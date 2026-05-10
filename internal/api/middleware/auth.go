package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type AuthRole string

const (
	AuthRoleAnonymous   AuthRole = "anonymous"
	AuthRoleViewer      AuthRole = "viewer"   // read-only sandbox/template list
	AuthRoleAPI         AuthRole = "api"      // standard API access (spawn/exec/files)
	AuthRoleOperator    AuthRole = "operator" // API + template/environment management
	AuthRoleAdmin       AuthRole = "admin"    // operator + quotas/workers/provider config
	AuthRoleTenantAdmin AuthRole = "tenant_admin" // admin within a specific tenant
	AuthRoleWorker      AuthRole = "worker"

	ScopeAPI             = "api:*"
	ScopeAdmin           = "admin:*"
	ScopeRead            = "read:*"
	ScopeOperator        = "operator:*"
	ScopeTenantAdmin     = "tenant:admin"
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
	// OIDC-populated fields
	Subject  string
	Email    string
	TenantID string
	Groups   []string
}

type authIdentityContextKey struct{}

const workerSignedTokenPrefix = "stacyvm-worker-v1"

var errInvalidWorkerTokenClaims = errors.New("invalid worker token claims")

const (
	WorkerTokenAudienceControlPlane = "worker:control-plane"
	WorkerTokenAudienceRPC          = "worker:rpc"
	MaxWorkerTokenTTL               = 15 * time.Minute
	WorkerTokenClockSkew            = 30 * time.Second
)

type WorkerAuthConfig struct {
	SharedToken     string
	WorkerTokens    map[string]string
	SigningKey      string
	SigningKeys     []string
	RevokedTokenIDs []string
	Now             func() time.Time
}

type WorkerTokenClaims struct {
	WorkerID  string   `json:"worker_id"`
	TokenID   string   `json:"jti,omitempty"`
	Audience  string   `json:"aud,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	ExpiresAt int64    `json:"exp"`
	IssuedAt  int64    `json:"iat,omitempty"`
	NotBefore int64    `json:"nbf,omitempty"`
}

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
	return WorkerAuthWithConfig(WorkerAuthConfig{
		SharedToken:  sharedWorkerToken,
		WorkerTokens: workerTokens,
	})
}

func WorkerAuthWithConfig(cfg WorkerAuthConfig) func(http.Handler) http.Handler {
	cleanWorkerTokens := normalizeWorkerTokens(cfg.WorkerTokens)
	sharedWorkerToken := strings.TrimSpace(cfg.SharedToken)
	signingKeys := normalizeSigningKeys(cfg.SigningKey, cfg.SigningKeys)
	revokedTokenIDs := normalizeRevokedTokenIDs(cfg.RevokedTokenIDs)
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sharedWorkerToken == "" && len(cleanWorkerTokens) == 0 && len(signingKeys) == 0 {
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
			scopes, ok := validateWorkerCredentials(workerID, token, sharedWorkerToken, cleanWorkerTokens, signingKeys, revokedTokenIDs, now)
			if workerID == "" || !ok {
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
				Scopes:   scopes,
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
				// Extract tenant hint from header if present (API key users may still scope to a tenant).
				tenantID := strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
				return AuthIdentity{
					Role:     authCandidate.Role,
					Header:   header,
					Scopes:   scopesForRole(authCandidate.Role),
					TenantID: tenantID,
				}, true
			}
		}
	}
	return AuthIdentity{}, false
}

func scopesForRole(role AuthRole) []string {
	switch role {
	case AuthRoleAdmin:
		return []string{ScopeRead, ScopeAPI, ScopeOperator, ScopeAdmin}
	case AuthRoleTenantAdmin:
		return []string{ScopeRead, ScopeAPI, ScopeOperator, ScopeTenantAdmin}
	case AuthRoleOperator:
		return []string{ScopeRead, ScopeAPI, ScopeOperator}
	case AuthRoleAPI:
		return []string{ScopeRead, ScopeAPI}
	case AuthRoleViewer:
		return []string{ScopeRead}
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

func SignWorkerToken(signingKey string, claims WorkerTokenClaims) (string, error) {
	signingKey = strings.TrimSpace(signingKey)
	claims.WorkerID = strings.TrimSpace(claims.WorkerID)
	if signingKey == "" || claims.WorkerID == "" || claims.ExpiresAt <= 0 {
		return "", errInvalidWorkerTokenClaims
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	signedPart := workerSignedTokenPrefix + "." + payloadB64
	signature := signWorkerToken(signingKey, signedPart)
	return signedPart + "." + signature, nil
}

func NewWorkerTokenID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func VerifyWorkerToken(signingKey, token string, now time.Time) (WorkerTokenClaims, bool) {
	return VerifyWorkerTokenForAudience(signingKey, token, "", now)
}

// DecodeWorkerTokenClaims decodes signed worker token metadata without verifying
// the signature. Callers must not use the returned claims as authenticated identity.
func DecodeWorkerTokenClaims(token string) (WorkerTokenClaims, bool) {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != workerSignedTokenPrefix {
		return WorkerTokenClaims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return WorkerTokenClaims{}, false
	}
	var claims WorkerTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return WorkerTokenClaims{}, false
	}
	claims.WorkerID = strings.TrimSpace(claims.WorkerID)
	claims.TokenID = strings.TrimSpace(claims.TokenID)
	claims.Audience = strings.TrimSpace(claims.Audience)
	return claims, true
}

func VerifyWorkerTokenForAudience(signingKey, token, audience string, now time.Time) (WorkerTokenClaims, bool) {
	signingKey = strings.TrimSpace(signingKey)
	token = strings.TrimSpace(token)
	if signingKey == "" || token == "" {
		return WorkerTokenClaims{}, false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != workerSignedTokenPrefix {
		return WorkerTokenClaims{}, false
	}
	signedPart := parts[0] + "." + parts[1]
	expectedSignature := signWorkerToken(signingKey, signedPart)
	if subtle.ConstantTimeCompare([]byte(parts[2]), []byte(expectedSignature)) != 1 {
		return WorkerTokenClaims{}, false
	}
	claims, ok := DecodeWorkerTokenClaims(token)
	if !ok {
		return WorkerTokenClaims{}, false
	}
	if claims.WorkerID == "" || claims.ExpiresAt <= 0 || !now.Before(time.Unix(claims.ExpiresAt, 0)) {
		return WorkerTokenClaims{}, false
	}
	if claims.NotBefore > 0 && now.Add(WorkerTokenClockSkew).Before(time.Unix(claims.NotBefore, 0)) {
		return WorkerTokenClaims{}, false
	}
	if claims.IssuedAt > 0 && now.Add(WorkerTokenClockSkew).Before(time.Unix(claims.IssuedAt, 0)) {
		return WorkerTokenClaims{}, false
	}
	if claims.IssuedAt > 0 && time.Unix(claims.ExpiresAt, 0).Sub(time.Unix(claims.IssuedAt, 0)) > MaxWorkerTokenTTL {
		return WorkerTokenClaims{}, false
	}
	if audience = strings.TrimSpace(audience); audience != "" && claims.Audience != "" && claims.Audience != audience {
		return WorkerTokenClaims{}, false
	}
	return claims, true
}

func validateWorkerCredentials(workerID, token, sharedWorkerToken string, workerTokens map[string]string, signingKeys []string, revokedTokenIDs map[string]struct{}, now func() time.Time) ([]string, bool) {
	if token == "" || workerID == "" {
		return nil, false
	}
	if claims, ok := verifyWorkerTokenWithAnyKey(signingKeys, token, WorkerTokenAudienceControlPlane, now().UTC()); ok {
		if claims.WorkerID != workerID {
			return nil, false
		}
		if isWorkerTokenRevoked(claims, revokedTokenIDs) {
			return nil, false
		}
		scopes := normalizeScopes(claims.Scopes)
		if len(scopes) == 0 {
			scopes = scopesForRole(AuthRoleWorker)
		}
		return scopes, true
	}
	if validWorkerToken(workerID, token, sharedWorkerToken, workerTokens) {
		return scopesForRole(AuthRoleWorker), true
	}
	return nil, false
}

func isWorkerTokenRevoked(claims WorkerTokenClaims, revokedTokenIDs map[string]struct{}) bool {
	if len(revokedTokenIDs) == 0 || claims.TokenID == "" {
		return false
	}
	_, ok := revokedTokenIDs[claims.TokenID]
	return ok
}

func verifyWorkerTokenWithAnyKey(signingKeys []string, token, audience string, now time.Time) (WorkerTokenClaims, bool) {
	for _, signingKey := range signingKeys {
		if claims, ok := VerifyWorkerTokenForAudience(signingKey, token, audience, now); ok {
			return claims, true
		}
	}
	return WorkerTokenClaims{}, false
}

func signWorkerToken(signingKey, signedPart string) string {
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(signedPart))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	allowed := map[string]struct{}{}
	for _, scope := range scopesForRole(AuthRoleWorker) {
		allowed[scope] = struct{}{}
	}
	cleaned := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if _, ok := allowed[scope]; ok {
			cleaned = append(cleaned, scope)
		}
	}
	return cleaned
}

func normalizeSigningKeys(primary string, additional []string) []string {
	seen := map[string]struct{}{}
	keys := make([]string, 0, len(additional)+1)
	for _, key := range append([]string{primary}, additional...) {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys
}

func normalizeRevokedTokenIDs(ids []string) map[string]struct{} {
	if len(ids) == 0 {
		return nil
	}
	revoked := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		revoked[id] = struct{}{}
	}
	return revoked
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
