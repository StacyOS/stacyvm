package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/orchestrator"
	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

func setupQuotaRouter(t *testing.T) (chi.Router, *orchestrator.Manager) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	reg := providers.NewRegistry()
	mock := providers.NewMockProvider()
	reg.Register(mock)
	reg.SetDefault("mock")

	mgr := orchestrator.NewManager(reg, st, orchestrator.NewEventBus(), zerolog.Nop(), orchestrator.ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
	})

	r := chi.NewRouter()
	r.Mount("/api/v1/quotas", NewQuotaRoutes(mgr).Routes())
	return r, mgr
}

func TestQuotaRoutes_SaveGetUsageDelete(t *testing.T) {
	r, mgr := setupQuotaRouter(t)

	body := `{"max_sandboxes":1,"max_ttl":"30m","max_exec_timeout":"10s"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/quotas/owner-a", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("save status = %d: %s", w.Code, w.Body.String())
	}

	var quota orchestrator.OwnerQuota
	if err := json.NewDecoder(w.Body).Decode(&quota); err != nil {
		t.Fatalf("decode quota: %v", err)
	}
	if quota.OwnerID != "owner-a" || quota.MaxSandboxes != 1 {
		t.Fatalf("unexpected quota: %+v", quota)
	}

	if _, err := mgr.Spawn(req.Context(), orchestrator.SpawnRequest{OwnerID: "owner-a"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/quotas/owner-a/usage", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("usage status = %d: %s", w.Code, w.Body.String())
	}
	var usage orchestrator.OwnerUsage
	if err := json.NewDecoder(w.Body).Decode(&usage); err != nil {
		t.Fatalf("decode usage: %v", err)
	}
	if !usage.QuotaConfigured || usage.ActiveSandboxes != 1 || usage.MaxSandboxes != 1 {
		t.Fatalf("unexpected usage: %+v", usage)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/quotas/owner-a", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d: %s", w.Code, w.Body.String())
	}
}

func TestQuotaRoutes_InvalidQuotaReturnsBadRequest(t *testing.T) {
	r, _ := setupQuotaRouter(t)

	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "bad duration",
			path: "/api/v1/quotas/owner-a",
			body: `{"max_ttl":"500ms"}`,
		},
		{
			name: "negative sandboxes",
			path: "/api/v1/quotas/owner-a",
			body: `{"max_sandboxes":-1}`,
		},
		{
			name: "bad owner",
			path: "/api/v1/quotas/owner%20a",
			body: `{"max_sandboxes":1}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusBadRequest, w.Body.String())
			}
		})
	}
}
