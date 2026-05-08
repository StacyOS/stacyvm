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

	ScopeAPI   = "api:*"
	ScopeAdmin = "admin:*"
)

type AuthIdentity struct {
	Role   AuthRole
	Header string
	Scopes []string
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

func AdminAuth(adminAPIKey, fallbackAPIKey string) func(http.Handler) http.Handler {
	candidates := []authCandidate{{Key: adminAPIKey, Role: AuthRoleAdmin}}
	if adminAPIKey == "" {
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
