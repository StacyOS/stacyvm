package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
)

type AuthRole string

const (
	AuthRoleAnonymous AuthRole = "anonymous"
	AuthRoleAPI       AuthRole = "api"
	AuthRoleAdmin     AuthRole = "admin"
	AuthRoleWorker    AuthRole = "worker"

	ScopeAPI             = "api:*"
	ScopeAdmin           = "admin:*"
	ScopeWorkerHeartbeat = "worker:heartbeat"
)

type AuthIdentity struct {
	Role     AuthRole
	Header   string
	WorkerID string
	Scopes   []string
}

type authIdentityContextKey struct{}

func Auth(apiKey string) func(http.Handler) http.Handler {
	return AuthAny(apiKey)
}

func AuthAny(apiKeys ...string) func(http.Handler) http.Handler {
	candidates := make([]authCandidate, 0, len(apiKeys))
	for i, key := range apiKeys {
		role := AuthRoleAPI
		if i > 0 {
			role = AuthRoleAdmin
		}
		candidates = append(candidates, authCandidate{Key: key, Role: role})
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(nonEmptyKeys(apiKeys...)) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			identity, ok := authenticateRequest(r, candidates...)
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{
					"code":    "UNAUTHORIZED",
					"message": "invalid or missing API key",
				})
				return
			}

			r = r.WithContext(WithAuthIdentity(r.Context(), identity))
			next.ServeHTTP(w, r)
		})
	}
}

func AdminAuth(adminAPIKey, fallbackAPIKey string, fallbackEnabled bool) func(http.Handler) http.Handler {
	candidates := []authCandidate{{Key: adminAPIKey, Role: AuthRoleAdmin}}
	if adminAPIKey == "" && fallbackEnabled {
		adminAPIKey = fallbackAPIKey
		candidates = []authCandidate{{Key: fallbackAPIKey, Role: AuthRoleAdmin}}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if adminAPIKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			identity, ok := authenticateRequest(r, candidates...)
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{
					"code":    "FORBIDDEN",
					"message": "admin API key required",
				})
				return
			}

			r = r.WithContext(WithAuthIdentity(r.Context(), identity))
			next.ServeHTTP(w, r)
		})
	}
}

func WorkerAuth(workerToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if workerToken == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(map[string]string{
					"code":    "UNAVAILABLE",
					"message": "worker token is not configured",
				})
				return
			}
			workerID := r.Header.Get("X-Worker-ID")
			token := r.Header.Get("X-Worker-Token")
			if token == "" {
				const prefix = "Bearer "
				authz := r.Header.Get("Authorization")
				if len(authz) > len(prefix) && authz[:len(prefix)] == prefix {
					token = authz[len(prefix):]
				}
			}
			if workerID == "" || subtle.ConstantTimeCompare([]byte(token), []byte(workerToken)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{
					"code":    "UNAUTHORIZED",
					"message": "invalid or missing worker credentials",
				})
				return
			}
			r = r.WithContext(WithAuthIdentity(r.Context(), AuthIdentity{
				Role:     AuthRoleWorker,
				Header:   "X-Worker-Token",
				WorkerID: workerID,
				Scopes:   []string{ScopeWorkerHeartbeat},
			}))
			next.ServeHTTP(w, r)
		})
	}
}

func WithAuthIdentity(ctx context.Context, identity AuthIdentity) context.Context {
	return context.WithValue(ctx, authIdentityContextKey{}, identity)
}

func AuthIdentityFromContext(ctx context.Context) AuthIdentity {
	identity, ok := ctx.Value(authIdentityContextKey{}).(AuthIdentity)
	if !ok {
		return AuthIdentity{Role: AuthRoleAnonymous}
	}
	return identity
}

func (i AuthIdentity) HasScope(scope string) bool {
	for _, candidate := range i.Scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity := AuthIdentityFromContext(r.Context())
			if !identity.HasScope(scope) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{
					"code":    "FORBIDDEN",
					"message": "required authorization scope missing",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type authCandidate struct {
	Key  string
	Role AuthRole
}

func authenticateRequest(r *http.Request, candidates ...authCandidate) (AuthIdentity, bool) {
	for _, header := range []string{"X-Admin-API-Key", "X-API-Key"} {
		candidate := r.Header.Get(header)
		if candidate == "" {
			continue
		}
		for _, authCandidate := range candidates {
			if authCandidate.Key == "" {
				continue
			}
			if subtle.ConstantTimeCompare([]byte(candidate), []byte(authCandidate.Key)) == 1 {
				return AuthIdentity{
					Role:   authCandidate.Role,
					Header: header,
					Scopes: scopesForRole(authCandidate.Role),
				}, true
			}
		}
	}
	return AuthIdentity{}, false
}

func scopesForRole(role AuthRole) []string {
	switch role {
	case AuthRoleAdmin:
		return []string{ScopeAPI, ScopeAdmin}
	case AuthRoleAPI:
		return []string{ScopeAPI}
	case AuthRoleWorker:
		return []string{ScopeWorkerHeartbeat}
	default:
		return nil
	}
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
