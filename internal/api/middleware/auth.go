package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
)

func Auth(apiKey string) func(http.Handler) http.Handler {
	return AuthAny(apiKey)
}

func AuthAny(apiKeys ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(nonEmptyKeys(apiKeys...)) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			if !matchesAnyKey(r.Header.Get("X-API-Key"), apiKeys...) && !matchesAnyKey(r.Header.Get("X-Admin-API-Key"), apiKeys...) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{
					"code":    "UNAUTHORIZED",
					"message": "invalid or missing API key",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func AdminAuth(adminAPIKey, fallbackAPIKey string) func(http.Handler) http.Handler {
	if adminAPIKey == "" {
		adminAPIKey = fallbackAPIKey
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if adminAPIKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			if !matchesAnyKey(r.Header.Get("X-Admin-API-Key"), adminAPIKey) && !matchesAnyKey(r.Header.Get("X-API-Key"), adminAPIKey) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{
					"code":    "FORBIDDEN",
					"message": "admin API key required",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func matchesAnyKey(candidate string, apiKeys ...string) bool {
	if candidate == "" {
		return false
	}
	for _, apiKey := range apiKeys {
		if apiKey == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(apiKey)) == 1 {
			return true
		}
	}
	return false
}

func nonEmptyKeys(apiKeys ...string) []string {
	keys := make([]string, 0, len(apiKeys))
	for _, key := range apiKeys {
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}
