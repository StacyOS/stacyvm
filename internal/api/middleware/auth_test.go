package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthAnyAcceptsPrimaryOrAdminKey(t *testing.T) {
	handler := AuthAny("primary-key", "admin-key")(okHandler())

	tests := []struct {
		name   string
		header string
		key    string
		want   int
	}{
		{name: "primary", header: "X-API-Key", key: "primary-key", want: http.StatusOK},
		{name: "admin via api header", header: "X-API-Key", key: "admin-key", want: http.StatusOK},
		{name: "admin via admin header", header: "X-Admin-API-Key", key: "admin-key", want: http.StatusOK},
		{name: "wrong", header: "X-API-Key", key: "wrong", want: http.StatusUnauthorized},
		{name: "missing", want: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set(tt.header, tt.key)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.want, w.Body.String())
			}
		})
	}
}

func TestAdminAuthRequiresAdminKeyWhenConfigured(t *testing.T) {
	handler := AdminAuth("admin-key", "primary-key")(okHandler())

	tests := []struct {
		name   string
		header string
		key    string
		want   int
	}{
		{name: "admin via admin header", header: "X-Admin-API-Key", key: "admin-key", want: http.StatusOK},
		{name: "admin via api header", header: "X-API-Key", key: "admin-key", want: http.StatusOK},
		{name: "primary rejected", header: "X-API-Key", key: "primary-key", want: http.StatusForbidden},
		{name: "missing", want: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set(tt.header, tt.key)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.want, w.Body.String())
			}
		})
	}
}

func TestAdminAuthFallsBackToPrimaryKey(t *testing.T) {
	handler := AdminAuth("", "primary-key")(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "primary-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
