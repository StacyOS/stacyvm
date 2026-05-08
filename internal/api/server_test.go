package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/orchestrator"
	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/rs/zerolog"
)

type noopBuildStarter struct{}

func (noopBuildStarter) Enqueue(buildID string) error { return nil }

func setupTestServer(t *testing.T, cfg ServerConfig) *Server {
	t.Helper()

	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	registry := providers.NewRegistry()
	mock := providers.NewMockProvider()
	registry.Register(mock)
	if err := registry.SetDefault("mock"); err != nil {
		t.Fatalf("set default provider: %v", err)
	}

	events := orchestrator.NewEventBus()
	manager := orchestrator.NewManager(registry, st, events, zerolog.Nop(), orchestrator.ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
	})
	templates := orchestrator.NewTemplateRegistry(st)
	pool := orchestrator.NewPoolManager(manager, templates, zerolog.Nop())

	return NewServer(cfg, registry, manager, events, templates, pool, st, noopBuildStarter{}, zerolog.Nop())
}

func TestAdminRoutesRequireAdminAPIKeyWhenConfigured(t *testing.T) {
	srv := setupTestServer(t, ServerConfig{
		APIKey:      "client-key",
		AdminAPIKey: "admin-key",
		Version:     "test",
	})

	tests := []struct {
		name   string
		header string
		key    string
		want   int
	}{
		{name: "client key forbidden", header: "X-API-Key", key: "client-key", want: http.StatusForbidden},
		{name: "admin api header ok", header: "X-API-Key", key: "admin-key", want: http.StatusOK},
		{name: "admin header ok", header: "X-Admin-API-Key", key: "admin-key", want: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostics", nil)
			req.Header.Set(tt.header, tt.key)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.want, w.Body.String())
			}
		})
	}
}

func TestAdminRoutesFallbackToAPIKeyWhenAdminKeyUnset(t *testing.T) {
	srv := setupTestServer(t, ServerConfig{
		APIKey:  "client-key",
		Version: "test",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostics", nil)
	req.Header.Set("X-API-Key", "client-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestAdminAPIKeyCanAuthenticateRegularRoutes(t *testing.T) {
	srv := setupTestServer(t, ServerConfig{
		APIKey:      "client-key",
		AdminAPIKey: "admin-key",
		Version:     "test",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/live", nil)
	req.Header.Set("X-Admin-API-Key", "admin-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestAdminRoutesWriteAuditLog(t *testing.T) {
	srv := setupTestServer(t, ServerConfig{
		APIKey:      "client-key",
		AdminAPIKey: "admin-key",
		Version:     "test",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostics", nil)
	req.Header.Set("X-Admin-API-Key", "admin-key")
	req.Header.Set("X-User-ID", "operator-a")
	req.Header.Set("User-Agent", "stacyvm-test")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("diagnostics status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit?limit=10", nil)
	req.Header.Set("X-Admin-API-Key", "admin-key")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("audit status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var records []store.AdminAuditRecord
	if err := json.NewDecoder(w.Body).Decode(&records); err != nil {
		t.Fatalf("decode audit records: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected audit records")
	}
	var found bool
	for _, rec := range records {
		if rec.Path == "/api/v1/admin/diagnostics" {
			found = true
			if rec.Actor != "operator-a" || rec.Method != http.MethodGet || rec.Status != http.StatusOK {
				t.Fatalf("unexpected diagnostics audit record: %+v", rec)
			}
		}
	}
	if !found {
		t.Fatalf("diagnostics audit record not found: %+v", records)
	}
}
