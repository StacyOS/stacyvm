package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

func setupTestServerWithStore(t *testing.T, cfg ServerConfig) (*Server, *store.SQLiteStore) {
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

	return NewServer(cfg, registry, manager, events, templates, pool, st, noopBuildStarter{}, zerolog.Nop()), st
}

func TestWorkerHeartbeatUsesWorkerTokenNotAPIKey(t *testing.T) {
	srv, st := setupTestServerWithStore(t, ServerConfig{
		APIKey:      "client-key",
		AdminAPIKey: "admin-key",
		WorkerToken: "worker-secret",
		Version:     "test",
	})
	body := []byte(`{"hostname":"worker-host","status":"online","providers":["mock"],"capabilities":["heartbeat"],"capacity":{"max_sandboxes":1}}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/worker/worker-a/heartbeat", bytes.NewReader(body))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	rec, err := st.GetWorker(context.Background(), "worker-a")
	if err != nil {
		t.Fatalf("get worker: %v", err)
	}
	if rec.Hostname != "worker-host" {
		t.Fatalf("hostname = %q, want worker-host", rec.Hostname)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/worker/worker-b/heartbeat", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "client-key")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("api key status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestWorkerHeartbeatRejectsWorkerIDMismatch(t *testing.T) {
	srv := setupTestServer(t, ServerConfig{
		WorkerToken: "worker-secret",
		Version:     "test",
	})
	body := []byte(`{"hostname":"worker-host"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/worker/worker-a/heartbeat", bytes.NewReader(body))
	req.Header.Set("X-Worker-ID", "worker-b")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestWorkerRenewLeaseUsesWorkerToken(t *testing.T) {
	srv, st := setupTestServerWithStore(t, ServerConfig{
		APIKey:      "client-key",
		AdminAPIKey: "admin-key",
		WorkerToken: "worker-secret",
		Version:     "test",
	})
	if _, err := st.AcquireLease(context.Background(), "sb-lease", "sandbox", "worker-a", time.Minute); err != nil {
		t.Fatalf("acquire lease: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/worker/worker-a/leases/sb-lease/renew", strings.NewReader(`{"ttl":"2m"}`))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var result struct {
		Lease struct {
			ResourceID string `json:"resource_id"`
			HolderID   string `json:"holder_id"`
			Generation int64  `json:"generation"`
		} `json:"lease"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Lease.ResourceID != "sb-lease" || result.Lease.HolderID != "worker-a" || result.Lease.Generation != 2 {
		t.Fatalf("unexpected lease result: %+v", result.Lease)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/worker/worker-a/leases/sb-lease/renew", strings.NewReader(`{"ttl":"2m"}`))
	req.Header.Set("X-API-Key", "client-key")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("api key status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
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

func TestAdminRoutesRejectAPIKeyWhenFallbackDisabled(t *testing.T) {
	srv := setupTestServer(t, ServerConfig{
		APIKey:                "client-key",
		AdminFallbackDisabled: true,
		Version:               "test",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostics", nil)
	req.Header.Set("X-API-Key", "client-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestAdminRoutesRemainOpenWhenAuthUnset(t *testing.T) {
	srv := setupTestServer(t, ServerConfig{
		Version: "test",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostics", nil)
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

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit?actor=operator-a&method=GET&status=200&path=diagnostics&format=csv", nil)
	req.Header.Set("X-Admin-API-Key", "admin-key")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("csv audit status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Fatalf("csv content type = %q", got)
	}
	if body := w.Body.String(); !strings.Contains(body, "/api/v1/admin/diagnostics") || !strings.Contains(body, "operator-a") {
		t.Fatalf("csv body missing filtered audit record: %s", body)
	}
}

func TestServerRegistersLocalWorkerAndExposesWorkers(t *testing.T) {
	srv := setupTestServer(t, ServerConfig{Version: "test"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workers", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("workers status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var workers []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&workers); err != nil {
		t.Fatalf("decode workers: %v", err)
	}
	if len(workers) != 1 || workers[0]["id"] != "local" {
		t.Fatalf("unexpected workers: %+v", workers)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/workers/worker-b/heartbeat", strings.NewReader(`{"hostname":"host-b","providers":["mock"],"capabilities":["spawn"],"capacity":{"max_sandboxes":5}}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var heartbeat map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&heartbeat); err != nil {
		t.Fatalf("decode heartbeat: %v", err)
	}
	if heartbeat["id"] != "worker-b" || heartbeat["status"] != "online" {
		t.Fatalf("unexpected heartbeat: %+v", heartbeat)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostics", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("diagnostics status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var diagnostics map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&diagnostics); err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	workerSummary := diagnostics["workers"].(map[string]interface{})
	if workerSummary["total"].(float64) != 2 {
		t.Fatalf("worker total = %v, want 2", workerSummary["total"])
	}
}

func TestServerRefreshesLocalWorkerHeartbeat(t *testing.T) {
	srv, st := setupTestServerWithStore(t, ServerConfig{
		Version:         "test",
		WorkerHeartbeat: 10 * time.Millisecond,
	})
	oldHeartbeat := time.Now().UTC().Add(-10 * time.Minute)
	if err := st.SaveWorker(context.Background(), &store.WorkerRecord{
		ID:            "local",
		Hostname:      "stale-host",
		Status:        "online",
		Providers:     `["mock"]`,
		Capabilities:  `["spawn"]`,
		Capacity:      `{}`,
		LastHeartbeat: oldHeartbeat,
	}); err != nil {
		t.Fatalf("save stale worker: %v", err)
	}

	srv.workerHeartbeat.start()
	t.Cleanup(func() { srv.workerHeartbeat.stop() })

	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		worker, err := st.GetWorker(context.Background(), "local")
		if err != nil {
			t.Fatalf("get local worker: %v", err)
		}
		if worker.LastHeartbeat.After(oldHeartbeat) && worker.Hostname != "stale-host" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	worker, _ := st.GetWorker(context.Background(), "local")
	t.Fatalf("local worker heartbeat was not refreshed: %+v", worker)
}

func TestAdminRoutesPruneAuditLogWithRetention(t *testing.T) {
	srv, st := setupTestServerWithStore(t, ServerConfig{
		APIKey:              "client-key",
		AdminAPIKey:         "admin-key",
		AdminAuditRetention: time.Hour,
		Version:             "test",
	})
	ctx := t.Context()
	if err := st.CreateAdminAudit(ctx, &store.AdminAuditRecord{
		Actor:     "old-operator",
		Method:    http.MethodGet,
		Path:      "/api/v1/admin/old",
		Status:    http.StatusOK,
		CreatedAt: time.Now().Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("create old audit: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostics", nil)
	req.Header.Set("X-Admin-API-Key", "admin-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("diagnostics status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	records, err := st.ListAdminAudit(ctx, store.AdminAuditQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	for _, rec := range records {
		if rec.Actor == "old-operator" {
			t.Fatalf("old audit record was not pruned: %+v", records)
		}
	}
}
