package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
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
	ScopeWorkerSpawn     = "worker:spawn"
	ScopeWorkerDestroy   = "worker:destroy"
	ScopeWorkerStatus    = "worker:status"
	ScopeWorkerExec      = "worker:exec"
	ScopeWorkerFiles     = "worker:files"
	ScopeWorkerLogs      = "worker:logs"
	ScopeWorkerLease     = "worker:lease"
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
	return WorkerAuthWithTokens(workerToken, nil)
}

func WorkerAuthWithTokens(sharedWorkerToken string, workerTokens map[string]string) func(http.Handler) http.Handler {
	cleanWorkerTokens := normalizeWorkerTokens(workerTokens)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.TrimSpace(sharedWorkerToken) == "" && len(cleanWorkerTokens) == 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(map[string]string{
					"code":    "UNAVAILABLE",
					"message": "worker token is not configured",
				})
				return
			}
			workerID := strings.TrimSpace(r.Header.Get("X-Worker-ID"))
			token, header := workerTokenFromRequest(r)
			if workerID == "" || !validWorkerToken(workerID, token, sharedWorkerToken, cleanWorkerTokens) {
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
				Header:   header,
				WorkerID: workerID,
				Scopes:   scopesForRole(AuthRoleWorker),
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
		return []string{
			ScopeWorkerHeartbeat,
			ScopeWorkerSpawn,
			ScopeWorkerDestroy,
			ScopeWorkerStatus,
			ScopeWorkerExec,
			ScopeWorkerFiles,
			ScopeWorkerLogs,
			ScopeWorkerLease,
		}
	default:
		return nil
	}
}

func workerTokenFromRequest(r *http.Request) (string, string) {
	if token := strings.TrimSpace(r.Header.Get("X-Worker-Token")); token != "" {
		return token, "X-Worker-Token"
	}
	const prefix = "Bearer "
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(authz) > len(prefix) && strings.EqualFold(authz[:len(prefix)], prefix) {
		return strings.TrimSpace(authz[len(prefix):]), "Authorization"
	}
	return "", ""
}

func validWorkerToken(workerID, token, sharedWorkerToken string, workerTokens map[string]string) bool {
	if token == "" {
		return false
	}
	if expected, ok := workerTokens[workerID]; ok {
		return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
	}
	sharedWorkerToken = strings.TrimSpace(sharedWorkerToken)
	return sharedWorkerToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(sharedWorkerToken)) == 1
}

func normalizeWorkerTokens(workerTokens map[string]string) map[string]string {
	if len(workerTokens) == 0 {
		return nil
	}
	cleaned := make(map[string]string, len(workerTokens))
	for workerID, token := range workerTokens {
		workerID = strings.TrimSpace(workerID)
		token = strings.TrimSpace(token)
		if workerID == "" || token == "" {
			continue
		}
		cleaned[workerID] = token
	}
	return cleaned
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
