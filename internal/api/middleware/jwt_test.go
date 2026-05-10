package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/store"
)

// generateTestRSAKey creates a 2048-bit RSA key for testing.
func generateTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

// mintTestJWT signs a JWT with RS256 using the provided key and claims.
func mintTestJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString(mustJSON(t, map[string]string{"alg": "RS256", "kid": "test-kid", "typ": "JWT"}))
	payload := base64.RawURLEncoding.EncodeToString(mustJSON(t, claims))
	signed := header + "." + payload
	h := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, 0, h[:])
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return b
}

// testJWKSCache returns a jwksCache pre-loaded with the given RSA key, bypassing HTTP.
func testJWKSCache(key *rsa.PrivateKey) *jwksCache {
	return &jwksCache{
		keys:    map[string]publicKey{"test-kid": {rsa: &key.PublicKey}},
		fetched: time.Now().Add(time.Hour),
		ttl:     time.Hour,
	}
}

// testOIDCMiddleware builds an OIDCAuth-equivalent middleware using a pre-loaded JWKS
// cache, allowing RS256 JWT tests without a real HTTP JWKS server.
func testOIDCMiddleware(key *rsa.PrivateKey, cfg OIDCConfig) func(http.Handler) http.Handler {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cache := testJWKSCache(key)
	adminGroups := groupSet(cfg.AdminGroups)
	operatorGroups := groupSet(cfg.OperatorGroups)
	viewerGroups := groupSet(cfg.ViewerGroups)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			claims, err := verifyJWT(token, cfg.Issuer, cfg.Audience, publicKey{}, cache, cfg.Now())
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"code": "UNAUTHORIZED", "message": err.Error()})
				return
			}
			role := oidcRole(claims.Groups, adminGroups, operatorGroups, viewerGroups)
			identity := AuthIdentity{
				Role:     role,
				Header:   "Authorization",
				Scopes:   scopesForRole(role),
				Subject:  claims.Subject,
				Email:    claims.Email,
				TenantID: claims.TenantID,
				Groups:   claims.Groups,
			}
			r = r.WithContext(WithAuthIdentity(r.Context(), identity))
			next.ServeHTTP(w, r)
		})
	}
}

func echoIdentityHandler(w http.ResponseWriter, r *http.Request) {
	identity := AuthIdentityFromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"role":      string(identity.Role),
		"subject":   identity.Subject,
		"email":     identity.Email,
		"tenant_id": identity.TenantID,
		"groups":    identity.Groups,
	})
}

func doOIDCRequest(t *testing.T, mw func(http.Handler) http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(echoIdentityHandler)).ServeHTTP(rr, req)
	return rr
}

func TestOIDCAuth_ValidToken_GetsAPIRole(t *testing.T) {
	key := generateTestRSAKey(t)
	now := time.Now()
	token := mintTestJWT(t, key, map[string]any{
		"sub":   "user-123",
		"iss":   "https://idp.example.com",
		"aud":   "stacyvm",
		"exp":   now.Add(5 * time.Minute).Unix(),
		"iat":   now.Unix(),
		"email": "alice@example.com",
	})

	rr := doOIDCRequest(t, testOIDCMiddleware(key, OIDCConfig{
		Issuer:   "https://idp.example.com",
		Audience: "stacyvm",
	}), token)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["role"] != "api" {
		t.Errorf("expected api role, got %v", resp["role"])
	}
	if resp["subject"] != "user-123" {
		t.Errorf("expected subject user-123, got %v", resp["subject"])
	}
	if resp["email"] != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %v", resp["email"])
	}
}

func TestOIDCAuth_AdminGroup_GetsAdminRole(t *testing.T) {
	key := generateTestRSAKey(t)
	now := time.Now()
	token := mintTestJWT(t, key, map[string]any{
		"sub":    "admin-user",
		"iss":    "https://idp.example.com",
		"aud":    "stacyvm",
		"exp":    now.Add(5 * time.Minute).Unix(),
		"iat":    now.Unix(),
		"groups": []string{"stacyvm-admins", "engineers"},
	})

	rr := doOIDCRequest(t, testOIDCMiddleware(key, OIDCConfig{
		Issuer:      "https://idp.example.com",
		Audience:    "stacyvm",
		AdminGroups: []string{"stacyvm-admins"},
	}), token)

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["role"] != "admin" {
		t.Errorf("expected admin role, got %v", resp["role"])
	}
}

func TestOIDCAuth_OperatorGroup_GetsOperatorRole(t *testing.T) {
	key := generateTestRSAKey(t)
	now := time.Now()
	token := mintTestJWT(t, key, map[string]any{
		"sub":    "op-user",
		"iss":    "https://idp.example.com",
		"aud":    "stacyvm",
		"exp":    now.Add(5 * time.Minute).Unix(),
		"iat":    now.Unix(),
		"groups": []string{"stacyvm-operators"},
	})

	rr := doOIDCRequest(t, testOIDCMiddleware(key, OIDCConfig{
		Issuer:         "https://idp.example.com",
		Audience:       "stacyvm",
		AdminGroups:    []string{"stacyvm-admins"},
		OperatorGroups: []string{"stacyvm-operators"},
	}), token)

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["role"] != "operator" {
		t.Errorf("expected operator role, got %v", resp["role"])
	}
}

func TestOIDCAuth_ExpiredToken_Returns401(t *testing.T) {
	key := generateTestRSAKey(t)
	token := mintTestJWT(t, key, map[string]any{
		"sub": "user",
		"iss": "https://idp.example.com",
		"aud": "stacyvm",
		"exp": time.Now().Add(-1 * time.Minute).Unix(),
		"iat": time.Now().Add(-10 * time.Minute).Unix(),
	})

	rr := doOIDCRequest(t, testOIDCMiddleware(key, OIDCConfig{
		Issuer:   "https://idp.example.com",
		Audience: "stacyvm",
	}), token)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", rr.Code)
	}
}

func TestOIDCAuth_WrongIssuer_Returns401(t *testing.T) {
	key := generateTestRSAKey(t)
	now := time.Now()
	token := mintTestJWT(t, key, map[string]any{
		"sub": "user",
		"iss": "https://evil.com",
		"aud": "stacyvm",
		"exp": now.Add(5 * time.Minute).Unix(),
		"iat": now.Unix(),
	})

	rr := doOIDCRequest(t, testOIDCMiddleware(key, OIDCConfig{
		Issuer:   "https://idp.example.com",
		Audience: "stacyvm",
	}), token)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong issuer, got %d", rr.Code)
	}
}

func TestOIDCAuth_WrongAudience_Returns401(t *testing.T) {
	key := generateTestRSAKey(t)
	now := time.Now()
	token := mintTestJWT(t, key, map[string]any{
		"sub": "user",
		"iss": "https://idp.example.com",
		"aud": "other-service",
		"exp": now.Add(5 * time.Minute).Unix(),
		"iat": now.Unix(),
	})

	rr := doOIDCRequest(t, testOIDCMiddleware(key, OIDCConfig{
		Issuer:   "https://idp.example.com",
		Audience: "stacyvm",
	}), token)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong audience, got %d", rr.Code)
	}
}

func TestOIDCAuth_WrongSigningKey_Returns401(t *testing.T) {
	key1 := generateTestRSAKey(t)
	key2 := generateTestRSAKey(t)
	now := time.Now()
	// Token signed with key1, but middleware has key2.
	token := mintTestJWT(t, key1, map[string]any{
		"sub": "user",
		"iss": "https://idp.example.com",
		"aud": "stacyvm",
		"exp": now.Add(5 * time.Minute).Unix(),
		"iat": now.Unix(),
	})

	rr := doOIDCRequest(t, testOIDCMiddleware(key2, OIDCConfig{
		Issuer:   "https://idp.example.com",
		Audience: "stacyvm",
	}), token)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong signing key, got %d", rr.Code)
	}
}

func TestOIDCAuth_NoBearerToken_FallsThrough(t *testing.T) {
	key := generateTestRSAKey(t)
	mw := testOIDCMiddleware(key, OIDCConfig{
		Issuer:   "https://idp.example.com",
		Audience: "stacyvm",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	called := false
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if !called {
		t.Error("expected downstream handler to be called when no Bearer token present")
	}
}

func TestBearerToken_WorkerSignedTokenExcluded(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer stacyvm-worker-v1.payload.signature")
	got := bearerToken(req)
	if got != "" {
		t.Errorf("bearerToken should return empty for worker-signed tokens, got %q", got)
	}
}

func TestBearerToken_NormalBearerExtracted(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer eyJfoo.bar.baz")
	got := bearerToken(req)
	if got != "eyJfoo.bar.baz" {
		t.Errorf("unexpected token: %q", got)
	}
}

func TestOIDCRole_Priority(t *testing.T) {
	adminSet := groupSet([]string{"admins"})
	opSet := groupSet([]string{"operators"})
	viewerSet := groupSet([]string{"viewers"})

	cases := []struct {
		groups []string
		want   AuthRole
	}{
		{[]string{"admins", "operators"}, AuthRoleAdmin},    // admin wins when both match
		{[]string{"operators", "viewers"}, AuthRoleOperator}, // operator beats viewer
		{[]string{"viewers"}, AuthRoleViewer},
		{[]string{"other"}, AuthRoleAPI}, // no match → default api
		{nil, AuthRoleAPI},
	}
	for _, tc := range cases {
		got := oidcRole(tc.groups, adminSet, opSet, viewerSet)
		if got != tc.want {
			t.Errorf("oidcRole(%v) = %q, want %q", tc.groups, got, tc.want)
		}
	}
}

func TestAudUnmarshal_StringAndArray(t *testing.T) {
	var a1 aud
	if err := json.Unmarshal([]byte(`"stacyvm"`), &a1); err != nil {
		t.Fatal(err)
	}
	if len(a1) != 1 || a1[0] != "stacyvm" {
		t.Errorf("string aud parse failed: %v", a1)
	}

	var a2 aud
	if err := json.Unmarshal([]byte(`["stacyvm","other"]`), &a2); err != nil {
		t.Fatal(err)
	}
	if len(a2) != 2 {
		t.Errorf("array aud parse failed: %v", a2)
	}
}

func TestPolicyPermits(t *testing.T) {
	pol := func(effect, pattern string) *store.PolicyRecord {
		return &store.PolicyRecord{Effect: effect, Pattern: pattern, Priority: 10}
	}

	tests := []struct {
		name     string
		value    string
		policies []*store.PolicyRecord
		want     bool
	}{
		{"allow exact", "alpine:3.19", []*store.PolicyRecord{pol("allow", "alpine:3.19")}, true},
		{"allow glob", "alpine:3.19", []*store.PolicyRecord{pol("allow", "alpine:*")}, true},
		{"allow wildcard", "anything", []*store.PolicyRecord{pol("allow", "*")}, true},
		{"deny exact", "ubuntu:latest", []*store.PolicyRecord{pol("deny", "ubuntu:latest")}, false},
		{"deny glob", "ubuntu:22.04", []*store.PolicyRecord{pol("deny", "ubuntu:*")}, false},
		{"no policy default permit", "anything", nil, true},
		{"deny before allow", "bad:img", []*store.PolicyRecord{pol("deny", "bad:img"), pol("allow", "bad:img")}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := policyPermits(tc.value, tc.policies)
			if got != tc.want {
				t.Errorf("policyPermits(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
