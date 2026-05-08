package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RateLimitConfig struct {
	Enabled           bool
	RequestsPerMinute int
	Burst             int
	KeyBy             string
	BucketTTL         time.Duration
	CleanupInterval   time.Duration
	Now               func() time.Time
}

type rateBucket struct {
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

type RateLimiter struct {
	mu                sync.Mutex
	buckets           map[string]*rateBucket
	rate              float64
	requestsPerMinute int
	burst             float64
	keyBy             string
	now               func() time.Time
	disabled          bool
	allowedTotal      uint64
	limitedTotal      uint64
	evictedTotal      uint64
	bucketTTL         time.Duration
	cleanupInterval   time.Duration
	lastCleanup       time.Time
}

type RateLimitStats struct {
	Enabled           bool   `json:"enabled"`
	RequestsPerMinute int    `json:"requests_per_minute"`
	Burst             int    `json:"burst"`
	KeyBy             string `json:"key_by"`
	ActiveBuckets     int    `json:"active_buckets"`
	AllowedTotal      uint64 `json:"allowed_total"`
	LimitedTotal      uint64 `json:"limited_total"`
	EvictedTotal      uint64 `json:"evicted_total"`
	BucketTTL         string `json:"bucket_ttl"`
	CleanupInterval   string `json:"cleanup_interval"`
}

func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	if cfg.RequestsPerMinute < 0 {
		cfg.RequestsPerMinute = 0
	}
	if cfg.Burst <= 0 {
		cfg.Burst = cfg.RequestsPerMinute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.BucketTTL == 0 {
		cfg.BucketTTL = 15 * time.Minute
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = time.Minute
	}
	keyBy := strings.TrimSpace(strings.ToLower(cfg.KeyBy))
	if keyBy == "" {
		keyBy = "owner"
	}
	return &RateLimiter{
		buckets:           make(map[string]*rateBucket),
		rate:              float64(cfg.RequestsPerMinute) / 60.0,
		requestsPerMinute: cfg.RequestsPerMinute,
		burst:             float64(cfg.Burst),
		keyBy:             keyBy,
		now:               cfg.Now,
		disabled:          !cfg.Enabled || cfg.RequestsPerMinute == 0,
		bucketTTL:         cfg.BucketTTL,
		cleanupInterval:   cfg.CleanupInterval,
	}
}

func RateLimit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	return NewRateLimiter(cfg).Middleware
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rl.disabled {
			next.ServeHTTP(w, r)
			return
		}

		allowed, remaining, retryAfter := rl.allow(rl.key(r))
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(int(rl.burst)))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !allowed {
			seconds := int(math.Ceil(retryAfter.Seconds()))
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"code":    "RESOURCE_LIMIT",
				"message": fmt.Sprintf("rate limit exceeded; retry after %ds", seconds),
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) allow(key string) (bool, int, time.Duration) {
	now := rl.now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.cleanupExpiredLocked(now)

	bucket := rl.buckets[key]
	if bucket == nil {
		bucket = &rateBucket{tokens: rl.burst, lastRefill: now}
		rl.buckets[key] = bucket
	}

	elapsed := now.Sub(bucket.lastRefill).Seconds()
	if elapsed > 0 {
		bucket.tokens = math.Min(rl.burst, bucket.tokens+elapsed*rl.rate)
		bucket.lastRefill = now
	}
	bucket.lastSeen = now

	if bucket.tokens >= 1 {
		bucket.tokens--
		rl.allowedTotal++
		return true, int(math.Floor(bucket.tokens)), 0
	}

	needed := 1 - bucket.tokens
	retryAfter := time.Duration(math.Ceil(needed/rl.rate)) * time.Second
	rl.limitedTotal++
	return false, 0, retryAfter
}

func (rl *RateLimiter) Stats() RateLimitStats {
	if rl == nil {
		return RateLimitStats{}
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return RateLimitStats{
		Enabled:           !rl.disabled,
		RequestsPerMinute: rl.requestsPerMinute,
		Burst:             int(rl.burst),
		KeyBy:             rl.keyBy,
		ActiveBuckets:     len(rl.buckets),
		AllowedTotal:      rl.allowedTotal,
		LimitedTotal:      rl.limitedTotal,
		EvictedTotal:      rl.evictedTotal,
		BucketTTL:         rl.bucketTTL.String(),
		CleanupInterval:   rl.cleanupInterval.String(),
	}
}

func (rl *RateLimiter) cleanupExpiredLocked(now time.Time) {
	if rl.bucketTTL <= 0 || rl.cleanupInterval <= 0 {
		return
	}
	if !rl.lastCleanup.IsZero() && now.Sub(rl.lastCleanup) < rl.cleanupInterval {
		return
	}
	rl.lastCleanup = now
	for key, bucket := range rl.buckets {
		if now.Sub(bucket.lastSeen) > rl.bucketTTL {
			delete(rl.buckets, key)
			rl.evictedTotal++
		}
	}
}

func (rl *RateLimiter) key(r *http.Request) string {
	switch rl.keyBy {
	case "api_key":
		if apiKey := strings.TrimSpace(r.Header.Get("X-API-Key")); apiKey != "" {
			return bucketKey("api_key", apiKey)
		}
	case "ip":
		return bucketKey("ip", clientIP(r))
	default:
		if ownerID := strings.TrimSpace(r.Header.Get("X-User-ID")); ownerID != "" {
			return bucketKey("owner", ownerID)
		}
		if apiKey := strings.TrimSpace(r.Header.Get("X-API-Key")); apiKey != "" {
			return bucketKey("api_key", apiKey)
		}
	}
	return bucketKey("ip", clientIP(r))
}

func bucketKey(kind, value string) string {
	sum := sha256.Sum256([]byte(kind + ":" + value))
	return kind + ":" + hex.EncodeToString(sum[:])
}

func clientIP(r *http.Request) string {
	if forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		return strings.TrimSpace(parts[0])
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
