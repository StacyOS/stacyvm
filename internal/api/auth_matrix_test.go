package api

// Auth matrix regression tests.
//
// Covers: API-key only, OIDC only, mixed, admin route access, worker tokens.
// Every case tests both the accept path and the reject path so regressions in
// either direction are caught immediately.

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/api/middleware"
)

// ── RSA JWT helpers ──────────────────────────────────────────────────────────

func matrixGenRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// matrixPEM returns the PKIX PEM encoding of an RSA public key.
func matrixPEM(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func matrixMintJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	hdr := base64.RawURLEncoding.EncodeToString(matrixJSON(t, map[string]string{
		"alg": "RS256", "kid": "k1", "typ": "JWT",
	}))
	pay := base64.RawURLEncoding.EncodeToString(matrixJSON(t, claims))
	signed := hdr + "." + pay
	h := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
	if err != nil {
		t.Fatal(err)
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func matrixJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func matrixDo(t *testing.T, srv *Server, method, path string, headers map[string]string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	return rr.Code
}

func matrixOIDC(key *rsa.PrivateKey, t *testing.T,
	adminGroups, operatorGroups, viewerGroups []string) middleware.OIDCConfig {
	return middleware.OIDCConfig{
		Issuer:         "https://idp.test",
		Audience:       "stacyvm",
		PublicKeyPEM:   matrixPEM(t, key),
		GroupsClaim:    "groups",
		AdminGroups:    adminGroups,
		OperatorGroups: operatorGroups,
		ViewerGroups:   viewerGroups,
	}
}

func jwtClaims(key *rsa.PrivateKey, t *testing.T, groups []string) string {
	now := time.Now()
	claims := map[string]any{
		"sub": "u1", "iss": "https://idp.test", "aud": "stacyvm",
		"exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix(),
	}
	if len(groups) > 0 {
		claims["groups"] = groups
	}
	return matrixMintJWT(t, key, claims)
}

// ── 1. API-key only ──────────────────────────────────────────────────────────

func TestAuthMatrix_APIKeyOnly_NormalRoute(t *testing.T) {
	const apiKey = "api-key-32-bytes-long-enough-!!"
	srv := setupTestServer(t, ServerConfig{Addr: "127.0.0.1:0", APIKey: apiKey, Version: "test"})

	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/sandboxes",
		map[string]string{"X-API-Key": apiKey}); code != http.StatusOK {
		t.Errorf("valid API key: want 200, got %d", code)
	}
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/sandboxes", nil); code != http.StatusUnauthorized {
		t.Errorf("missing API key: want 401, got %d", code)
	}
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/sandboxes",
		map[string]string{"X-API-Key": "wrong"}); code != http.StatusUnauthorized {
		t.Errorf("wrong API key: want 401, got %d", code)
	}
}

func TestAuthMatrix_APIKeyOnly_AdminRoute(t *testing.T) {
	const (
		apiKey   = "api-key-32-bytes-long-enough-!!"
		adminKey = "admin-key-32-bytes-long-enough-!"
	)
	srv := setupTestServer(t, ServerConfig{
		Addr: "127.0.0.1:0", APIKey: apiKey, AdminAPIKey: adminKey, Version: "test",
	})

	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/admin/diagnostics",
		map[string]string{"X-Admin-API-Key": adminKey}); code != http.StatusOK {
		t.Errorf("valid admin key: want 200, got %d", code)
	}
	// Regular API key must not reach admin routes.
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/admin/diagnostics",
		map[string]string{"X-API-Key": apiKey}); code != http.StatusForbidden {
		t.Errorf("regular key on admin route: want 403, got %d", code)
	}
	// No key at all → AuthAny returns 401 (unauthenticated) before reaching admin routes.
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/admin/diagnostics", nil); code != http.StatusUnauthorized {
		t.Errorf("no key on admin route: want 401, got %d", code)
	}
}

// ── 2. OIDC only ─────────────────────────────────────────────────────────────

func TestAuthMatrix_OIDCOnly_NormalRoute(t *testing.T) {
	key := matrixGenRSAKey(t)
	srv := setupTestServer(t, ServerConfig{
		Addr: "127.0.0.1:0", Version: "test",
		OIDC: matrixOIDC(key, t, []string{"admins"}, nil, nil),
	})

	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/sandboxes",
		map[string]string{"Authorization": "Bearer " + jwtClaims(key, t, nil)}); code != http.StatusOK {
		t.Errorf("valid OIDC bearer: want 200, got %d", code)
	}
	// No bearer in OIDC-only mode — anonymous identity has no scopes, 403.
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/sandboxes", nil); code != http.StatusForbidden {
		t.Errorf("no bearer in OIDC-only mode: want 403, got %d", code)
	}
	// Malformed token — rejected before scope check.
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/sandboxes",
		map[string]string{"Authorization": "Bearer not.a.jwt"}); code != http.StatusUnauthorized {
		t.Errorf("malformed bearer: want 401, got %d", code)
	}
}

func TestAuthMatrix_OIDCOnly_ViewerCannotSpawn(t *testing.T) {
	key := matrixGenRSAKey(t)
	srv := setupTestServer(t, ServerConfig{
		Addr: "127.0.0.1:0", Version: "test",
		OIDC: matrixOIDC(key, t, []string{"admins"}, nil, []string{"viewers"}),
	})

	viewerToken := jwtClaims(key, t, []string{"viewers"})
	// Viewer can list (read:*).
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/sandboxes",
		map[string]string{"Authorization": "Bearer " + viewerToken}); code != http.StatusOK {
		t.Errorf("viewer list: want 200, got %d", code)
	}
	// Viewer cannot spawn (requires api:*).
	if code := matrixDo(t, srv, http.MethodPost, "/api/v1/sandboxes",
		map[string]string{"Authorization": "Bearer " + viewerToken}); code != http.StatusForbidden {
		t.Errorf("viewer spawn: want 403, got %d", code)
	}
}

func TestAuthMatrix_OIDCOnly_AdminRouteRequiresAdminRole(t *testing.T) {
	key := matrixGenRSAKey(t)
	srv := setupTestServer(t, ServerConfig{
		Addr: "127.0.0.1:0", Version: "test",
		OIDC: matrixOIDC(key, t, []string{"admins"}, nil, nil),
	})

	// Anonymous must be blocked — this was the bug: admin routes were open in OIDC-only mode.
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/admin/diagnostics", nil); code != http.StatusForbidden {
		t.Errorf("anonymous on admin route (OIDC-only): want 403, got %d — regression: admin route was unprotected", code)
	}
	// Non-admin OIDC user must be blocked.
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/admin/diagnostics",
		map[string]string{"Authorization": "Bearer " + jwtClaims(key, t, nil)}); code != http.StatusForbidden {
		t.Errorf("non-admin OIDC on admin route: want 403, got %d", code)
	}
	// Admin-group OIDC user must pass.
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/admin/diagnostics",
		map[string]string{"Authorization": "Bearer " + jwtClaims(key, t, []string{"admins"})}); code != http.StatusOK {
		t.Errorf("admin OIDC on admin route: want 200, got %d", code)
	}
}

// ── 3. Mixed: OIDC + API key ─────────────────────────────────────────────────

func TestAuthMatrix_Mixed_EitherAuthWorks(t *testing.T) {
	key := matrixGenRSAKey(t)
	const apiKey = "api-key-32-bytes-long-enough-!!"
	srv := setupTestServer(t, ServerConfig{
		Addr: "127.0.0.1:0", APIKey: apiKey, Version: "test",
		OIDC: matrixOIDC(key, t, nil, nil, nil),
	})

	// API key works.
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/sandboxes",
		map[string]string{"X-API-Key": apiKey}); code != http.StatusOK {
		t.Errorf("API key in mixed mode: want 200, got %d", code)
	}
	// Valid OIDC bearer works.
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/sandboxes",
		map[string]string{"Authorization": "Bearer " + jwtClaims(key, t, nil)}); code != http.StatusOK {
		t.Errorf("OIDC bearer in mixed mode: want 200, got %d", code)
	}
	// Invalid bearer must be rejected — not silently downgraded to anonymous.
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/sandboxes",
		map[string]string{"Authorization": "Bearer not.a.jwt"}); code != http.StatusUnauthorized {
		t.Errorf("invalid bearer in mixed mode: want 401, got %d", code)
	}
	// No auth at all → rejected.
	if code := matrixDo(t, srv, http.MethodGet, "/api/v1/sandboxes", nil); code != http.StatusUnauthorized {
		t.Errorf("no auth in mixed mode: want 401, got %d", code)
	}
}

// ── 4. Worker tokens must not be treated as OIDC Bearer tokens ───────────────

func TestAuthMatrix_WorkerToken_NotAcceptedOnAPIRoute(t *testing.T) {
	const apiKey = "api-key-32-bytes-long-enough-!!"
	signingKey := "worker-signing-key-32-bytes-long!"
	// Configure both API key (so auth is active) and a worker signing key.
	srv := setupTestServer(t, ServerConfig{
		Addr:             "127.0.0.1:0",
		APIKey:           apiKey,
		WorkerSigningKey: signingKey,
		Version:          "test",
	})

	workerToken, err := middleware.SignWorkerToken(signingKey, middleware.WorkerTokenClaims{
		WorkerID:  "worker-a",
		Audience:  middleware.WorkerTokenAudienceControlPlane,
		ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
		IssuedAt:  time.Now().Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Worker token starts with "stacyvm-worker-v1" — bearerToken() excludes it
	// so it never reaches the OIDC verifier. AuthAny also won't accept it as an
	// API key. Result: 401 (not 200 that would indicate the token was accepted).
	code := matrixDo(t, srv, http.MethodGet, "/api/v1/sandboxes",
		map[string]string{"Authorization": "Bearer " + workerToken})
	if code == http.StatusOK {
		t.Error("worker token was accepted on a regular API route — bearerToken() must exclude stacyvm-worker-v1 tokens")
	}
}

// ── 5. Worker token issuer: non-worker scopes rejected ───────────────────────

func TestAuthMatrix_TokenIssuer_RejectsNonWorkerScopes(t *testing.T) {
	const adminKey = "admin-key-32-bytes-long-enough-!"
	const signingKey = "worker-signing-key-32-bytes-long!"
	srv := setupTestServer(t, ServerConfig{
		Addr:             "127.0.0.1:0",
		AdminAPIKey:      adminKey,
		WorkerSigningKey: signingKey,
		Version:          "test",
	})

	body := `{"worker_id":"worker-a","ttl":"5m","scopes":["admin:*"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/worker-tokens",
		mustBodyReader(t, body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-API-Key", adminKey)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("non-worker scope in token request: want 400, got %d: %s", rr.Code, rr.Body.String())
	}

	// Worker scope is allowed.
	body2 := `{"worker_id":"worker-a","ttl":"5m","scopes":["worker:spawn"]}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/worker-tokens",
		mustBodyReader(t, body2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Admin-API-Key", adminKey)
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("worker scope in token request: want 200, got %d: %s", rr2.Code, rr2.Body.String())
	}
}

func mustBodyReader(t *testing.T, body string) *strings.Reader {
	t.Helper()
	return strings.NewReader(body)
}
