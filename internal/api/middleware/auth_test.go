package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthAnyAcceptsPrimaryOrAdminKey(t *testing.T) {
	var got AuthIdentity
	handler := AuthAny("primary-key", "admin-key")(identityHandler(&got))

	tests := []struct {
		name     string
		header   string
		key      string
		want     int
		wantRole AuthRole
		wantHead string
	}{
		{name: "primary", header: "X-API-Key", key: "primary-key", want: http.StatusOK, wantRole: AuthRoleAPI, wantHead: "X-API-Key"},
		{name: "admin via api header", header: "X-API-Key", key: "admin-key", want: http.StatusOK, wantRole: AuthRoleAdmin, wantHead: "X-API-Key"},
		{name: "admin via admin header", header: "X-Admin-API-Key", key: "admin-key", want: http.StatusOK, wantRole: AuthRoleAdmin, wantHead: "X-Admin-API-Key"},
		{name: "wrong", header: "X-API-Key", key: "wrong", want: http.StatusUnauthorized},
		{name: "missing", want: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got = AuthIdentity{}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set(tt.header, tt.key)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.want, w.Body.String())
			}
			if tt.want != http.StatusOK {
				return
			}
			if got.Role != tt.wantRole {
				t.Fatalf("role = %q, want %q", got.Role, tt.wantRole)
			}
			if got.Header != tt.wantHead {
				t.Fatalf("header = %q, want %q", got.Header, tt.wantHead)
			}
		})
	}
}

func TestAdminAuthRequiresAdminKeyWhenConfigured(t *testing.T) {
	var got AuthIdentity
	handler := AdminAuth("admin-key", "primary-key", true)(identityHandler(&got))

	tests := []struct {
		name     string
		header   string
		key      string
		want     int
		wantHead string
	}{
		{name: "admin via admin header", header: "X-Admin-API-Key", key: "admin-key", want: http.StatusOK, wantHead: "X-Admin-API-Key"},
		{name: "admin via api header", header: "X-API-Key", key: "admin-key", want: http.StatusOK, wantHead: "X-API-Key"},
		{name: "primary rejected", header: "X-API-Key", key: "primary-key", want: http.StatusForbidden},
		{name: "missing", want: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got = AuthIdentity{}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set(tt.header, tt.key)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.want, w.Body.String())
			}
			if tt.want != http.StatusOK {
				return
			}
			if got.Role != AuthRoleAdmin {
				t.Fatalf("role = %q, want %q", got.Role, AuthRoleAdmin)
			}
			if got.Header != tt.wantHead {
				t.Fatalf("header = %q, want %q", got.Header, tt.wantHead)
			}
			if !got.HasScope(ScopeAdmin) || !got.HasScope(ScopeAPI) {
				t.Fatalf("admin identity scopes = %#v, want admin and api scopes", got.Scopes)
			}
		})
	}
}

func TestAdminAuthFallsBackToPrimaryKey(t *testing.T) {
	handler := AdminAuth("", "primary-key", true)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "primary-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestAdminAuthCanDisablePrimaryKeyFallback(t *testing.T) {
	handler := AdminAuth("", "primary-key", false)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "primary-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d when no admin key is configured: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestAuthIdentityFromContextDefaultsToAnonymous(t *testing.T) {
	identity := AuthIdentityFromContext(context.Background())
	if identity.Role != AuthRoleAnonymous {
		t.Fatalf("role = %q, want %q", identity.Role, AuthRoleAnonymous)
	}
}

func TestWorkerAuthWithPerWorkerToken(t *testing.T) {
	var got AuthIdentity
	handler := WorkerAuthWithTokens("shared-token", map[string]string{
		"worker-a": "worker-a-token",
	})(identityHandler(&got))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-a-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if got.Role != AuthRoleWorker || got.WorkerID != "worker-a" {
		t.Fatalf("identity = %+v, want worker-a worker identity", got)
	}
	if !got.HasScope(ScopeWorkerHeartbeat) || !got.HasScope(ScopeWorkerLease) {
		t.Fatalf("worker identity scopes = %#v, want heartbeat and lease scopes", got.Scopes)
	}
}

func TestWorkerAuthPerWorkerTokenOverridesSharedToken(t *testing.T) {
	handler := WorkerAuthWithTokens("shared-token", map[string]string{
		"worker-a": "worker-a-token",
	})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "shared-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d when shared token is used for a worker-specific credential: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestWorkerAuthFallsBackToSharedTokenForUnmappedWorker(t *testing.T) {
	handler := WorkerAuthWithTokens("shared-token", map[string]string{
		"worker-a": "worker-a-token",
	})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-b")
	req.Header.Set("Authorization", "Bearer shared-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for unmapped worker using shared token: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestWorkerAuthAcceptsSignedWorkerToken(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	token, err := SignWorkerToken("0123456789abcdef0123456789abcdef", WorkerTokenClaims{
		WorkerID:  "worker-a",
		Audience:  WorkerTokenAudienceControlPlane,
		Scopes:    []string{ScopeWorkerHeartbeat, ScopeWorkerLease, "admin:*"},
		ExpiresAt: now.Add(time.Minute).Unix(),
		IssuedAt:  now.Add(-time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("sign worker token: %v", err)
	}

	var got AuthIdentity
	handler := WorkerAuthWithConfig(WorkerAuthConfig{
		SigningKey: "0123456789abcdef0123456789abcdef",
		Now:        func() time.Time { return now },
	})(identityHandler(&got))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if got.Role != AuthRoleWorker || got.WorkerID != "worker-a" || got.Header != "Authorization" {
		t.Fatalf("identity = %+v, want signed worker-a identity from Authorization", got)
	}
	if !got.HasScope(ScopeWorkerHeartbeat) || !got.HasScope(ScopeWorkerLease) {
		t.Fatalf("worker identity scopes = %#v, want signed worker scopes", got.Scopes)
	}
	if got.HasScope(ScopeAdmin) {
		t.Fatalf("worker identity scopes = %#v, signed token should not grant non-worker scopes", got.Scopes)
	}
}

func TestWorkerAuthRejectsSignedWorkerTokenForWrongWorker(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	token, err := SignWorkerToken("0123456789abcdef0123456789abcdef", WorkerTokenClaims{
		WorkerID:  "worker-a",
		Audience:  WorkerTokenAudienceControlPlane,
		ExpiresAt: now.Add(time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("sign worker token: %v", err)
	}
	handler := WorkerAuthWithConfig(WorkerAuthConfig{
		SigningKey: "0123456789abcdef0123456789abcdef",
		Now:        func() time.Time { return now },
	})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-b")
	req.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d for signed worker token with mismatched worker ID: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestWorkerAuthRejectsExpiredSignedWorkerToken(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	token, err := SignWorkerToken("0123456789abcdef0123456789abcdef", WorkerTokenClaims{
		WorkerID:  "worker-a",
		Audience:  WorkerTokenAudienceControlPlane,
		ExpiresAt: now.Add(-time.Second).Unix(),
	})
	if err != nil {
		t.Fatalf("sign worker token: %v", err)
	}
	handler := WorkerAuthWithConfig(WorkerAuthConfig{
		SigningKey: "0123456789abcdef0123456789abcdef",
		Now:        func() time.Time { return now },
	})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d for expired signed worker token: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestWorkerAuthRejectsNotYetValidSignedWorkerToken(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	token, err := SignWorkerToken("0123456789abcdef0123456789abcdef", WorkerTokenClaims{
		WorkerID:  "worker-a",
		Audience:  WorkerTokenAudienceControlPlane,
		IssuedAt:  now.Unix(),
		NotBefore: now.Add(2 * time.Minute).Unix(),
		ExpiresAt: now.Add(5 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("sign worker token: %v", err)
	}
	handler := WorkerAuthWithConfig(WorkerAuthConfig{
		SigningKey: "0123456789abcdef0123456789abcdef",
		Now:        func() time.Time { return now },
	})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d for not-yet-valid signed worker token: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestWorkerAuthAllowsSmallSignedTokenClockSkew(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	token, err := SignWorkerToken("0123456789abcdef0123456789abcdef", WorkerTokenClaims{
		WorkerID:  "worker-a",
		Audience:  WorkerTokenAudienceControlPlane,
		IssuedAt:  now.Add(10 * time.Second).Unix(),
		NotBefore: now.Add(10 * time.Second).Unix(),
		ExpiresAt: now.Add(5 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("sign worker token: %v", err)
	}
	handler := WorkerAuthWithConfig(WorkerAuthConfig{
		SigningKey: "0123456789abcdef0123456789abcdef",
		Now:        func() time.Time { return now },
	})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for signed worker token inside clock skew: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestWorkerAuthRejectsSignedWorkerTokenExceedingMaxTTL(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	token, err := SignWorkerToken("0123456789abcdef0123456789abcdef", WorkerTokenClaims{
		WorkerID:  "worker-a",
		Audience:  WorkerTokenAudienceControlPlane,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(MaxWorkerTokenTTL + time.Second).Unix(),
	})
	if err != nil {
		t.Fatalf("sign worker token: %v", err)
	}
	handler := WorkerAuthWithConfig(WorkerAuthConfig{
		SigningKey: "0123456789abcdef0123456789abcdef",
		Now:        func() time.Time { return now },
	})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d for signed worker token exceeding max TTL: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestWorkerAuthRejectsRevokedSignedWorkerToken(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	token, err := SignWorkerToken("0123456789abcdef0123456789abcdef", WorkerTokenClaims{
		WorkerID:  "worker-a",
		TokenID:   "revoked-token-id",
		Audience:  WorkerTokenAudienceControlPlane,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(5 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("sign worker token: %v", err)
	}
	handler := WorkerAuthWithConfig(WorkerAuthConfig{
		SigningKey:      "0123456789abcdef0123456789abcdef",
		RevokedTokenIDs: []string{"revoked-token-id"},
		Now:             func() time.Time { return now },
	})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d for revoked signed worker token: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestWorkerAuthRejectsRPCAudienceSignedWorkerToken(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	token, err := SignWorkerToken("0123456789abcdef0123456789abcdef", WorkerTokenClaims{
		WorkerID:  "worker-a",
		Audience:  WorkerTokenAudienceRPC,
		ExpiresAt: now.Add(time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("sign worker token: %v", err)
	}
	handler := WorkerAuthWithConfig(WorkerAuthConfig{
		SigningKey: "0123456789abcdef0123456789abcdef",
		Now:        func() time.Time { return now },
	})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d for RPC-audience token on control-plane route: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestWorkerAuthKeepsStaticTokenFallbackWithSigningKey(t *testing.T) {
	handler := WorkerAuthWithConfig(WorkerAuthConfig{
		SharedToken: "shared-token",
		SigningKey:  "0123456789abcdef0123456789abcdef",
	})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "shared-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for static fallback with signing key configured: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestWorkerAuthAcceptsSignedWorkerTokenFromRotationKey(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	token, err := SignWorkerToken("old-worker-signing-key-with-at-least-32-bytes", WorkerTokenClaims{
		WorkerID:  "worker-a",
		Audience:  WorkerTokenAudienceControlPlane,
		ExpiresAt: now.Add(time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("sign worker token: %v", err)
	}
	handler := WorkerAuthWithConfig(WorkerAuthConfig{
		SigningKey:  "new-worker-signing-key-with-at-least-32-bytes",
		SigningKeys: []string{"old-worker-signing-key-with-at-least-32-bytes"},
		Now:         func() time.Time { return now },
	})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for token signed by rotation key: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestWorkerAuthSigningKeyCountsAsConfiguredCredentials(t *testing.T) {
	handler := WorkerAuthWithConfig(WorkerAuthConfig{
		SigningKey: "0123456789abcdef0123456789abcdef",
	})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "invalid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d when signing key is configured but token is invalid: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestRequireScopeAllowsMatchingScope(t *testing.T) {
	handler := RequireScope(ScopeAdmin)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(WithAuthIdentity(req.Context(), AuthIdentity{
		Role:   AuthRoleAdmin,
		Scopes: []string{ScopeAPI, ScopeAdmin},
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestRequireScopeRejectsMissingScope(t *testing.T) {
	handler := RequireScope(ScopeAdmin)(okHandler())

	tests := []struct {
		name     string
		identity AuthIdentity
	}{
		{name: "regular api identity", identity: AuthIdentity{Role: AuthRoleAPI, Scopes: []string{ScopeAPI}}},
		{name: "anonymous identity", identity: AuthIdentity{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req = req.WithContext(WithAuthIdentity(req.Context(), tt.identity))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusForbidden, w.Body.String())
			}
		})
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func identityHandler(dst *AuthIdentity) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*dst = AuthIdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
}
