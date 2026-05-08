package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
