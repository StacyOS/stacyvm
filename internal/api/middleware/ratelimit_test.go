package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimitByOwner(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 60,
		Burst:             1,
		KeyBy:             "owner",
		Now:               func() time.Time { return now },
	})

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil)
	req.Header.Set("X-User-ID", "owner-a")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("first owner-a status = %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil)
	req.Header.Set("X-User-ID", "owner-a")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second owner-a status = %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil)
	req.Header.Set("X-User-ID", "owner-b")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("owner-b status = %d", w.Code)
	}
}

func TestRateLimitRefills(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 60,
		Burst:             1,
		Now:               func() time.Time { return now },
	})

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil)
	req.RemoteAddr = "203.0.113.10:5000"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("first status = %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil)
	req.RemoteAddr = "203.0.113.10:5000"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d", w.Code)
	}

	now = now.Add(time.Second)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil)
	req.RemoteAddr = "203.0.113.10:5000"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("refilled status = %d", w.Code)
	}
}

func TestRateLimitDisabled(t *testing.T) {
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:           false,
		RequestsPerMinute: 1,
		Burst:             1,
	})

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("request %d status = %d", i, w.Code)
		}
	}
}

func TestRateLimitErrorBody(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 60,
		Burst:             1,
		Now:               func() time.Time { return now },
	})

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil)
		req.RemoteAddr = "203.0.113.20:5000"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if i == 1 {
			var body map[string]string
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["code"] != "RESOURCE_LIMIT" {
				t.Fatalf("unexpected body: %+v", body)
			}
		}
	}
}

func TestRateLimitStats(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 60,
		Burst:             1,
		KeyBy:             "ip",
		Now:               func() time.Time { return now },
	})

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil)
		req.RemoteAddr = "203.0.113.30:5000"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	stats := limiter.Stats()
	if !stats.Enabled || stats.RequestsPerMinute != 60 || stats.Burst != 1 || stats.KeyBy != "ip" {
		t.Fatalf("unexpected config stats: %+v", stats)
	}
	if stats.ActiveBuckets != 1 || stats.AllowedTotal != 1 || stats.LimitedTotal != 1 {
		t.Fatalf("unexpected counters: %+v", stats)
	}
	if stats.BucketTTL == "" || stats.CleanupInterval == "" {
		t.Fatalf("expected cleanup settings in stats: %+v", stats)
	}
}

func TestRateLimitEvictsInactiveBuckets(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 60,
		Burst:             1,
		KeyBy:             "ip",
		BucketTTL:         time.Minute,
		CleanupInterval:   time.Second,
		Now:               func() time.Time { return now },
	})

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil)
	req.RemoteAddr = "203.0.113.40:5000"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if limiter.Stats().ActiveBuckets != 1 {
		t.Fatalf("expected one active bucket: %+v", limiter.Stats())
	}

	now = now.Add(2 * time.Minute)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil)
	req.RemoteAddr = "203.0.113.41:5000"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	stats := limiter.Stats()
	if stats.ActiveBuckets != 1 || stats.EvictedTotal != 1 {
		t.Fatalf("unexpected cleanup stats: %+v", stats)
	}
}
