package routes

import (
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

func setupSystemRoutes(t *testing.T, withProvider bool) (*SystemRoutes, *orchestrator.Manager) {
	t.Helper()

	dir := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	registry := providers.NewRegistry()
	if withProvider {
		mock := providers.NewMockProvider()
		registry.Register(mock)
		if err := registry.SetDefault("mock"); err != nil {
			t.Fatalf("set default provider: %v", err)
		}
	}

	events := orchestrator.NewEventBus()
	manager := orchestrator.NewManager(registry, st, events, zerolog.Nop(), orchestrator.ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
	})

	return NewSystemRoutes(registry, manager, events, st, "test-version"), manager
}

func TestSystemRoutes_Live(t *testing.T) {
	routes, _ := setupSystemRoutes(t, true)
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	w := httptest.NewRecorder()

	routes.Live(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body map[string]interface{}
	decodeSystemResponse(t, w, &body)
	if body["status"] != "alive" {
		t.Fatalf("status = %v, want alive", body["status"])
	}
}

func TestSystemRoutes_Ready(t *testing.T) {
	routes, _ := setupSystemRoutes(t, true)
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	routes.Ready(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body map[string]interface{}
	decodeSystemResponse(t, w, &body)
	if body["status"] != "ready" {
		t.Fatalf("status = %v, want ready", body["status"])
	}
	if body["ready_providers"].(float64) != 1 {
		t.Fatalf("ready providers = %v, want 1", body["ready_providers"])
	}
	providersBody := body["providers"].([]interface{})
	firstProvider := providersBody[0].(map[string]interface{})
	for _, field := range []string{"latency_ms", "last_checked", "capabilities"} {
		if _, ok := firstProvider[field]; !ok {
			t.Fatalf("provider health missing %s: %#v", field, firstProvider)
		}
	}
}

func TestSystemRoutes_ReadyNoProviders(t *testing.T) {
	routes, _ := setupSystemRoutes(t, false)
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	routes.Ready(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	var body map[string]interface{}
	decodeSystemResponse(t, w, &body)
	if body["status"] != "not_ready" {
		t.Fatalf("status = %v, want not_ready", body["status"])
	}
}

func TestSystemRoutes_Diagnostics(t *testing.T) {
	routes, manager := setupSystemRoutes(t, true)
	if _, err := manager.SaveOwnerQuota(context.Background(), orchestrator.OwnerQuota{
		OwnerID:        "team-a",
		MaxSandboxes:   3,
		MaxTTL:         "30m",
		MaxExecTimeout: "10s",
	}); err != nil {
		t.Fatalf("save quota: %v", err)
	}
	if _, err := manager.Spawn(context.Background(), orchestrator.SpawnRequest{Image: "alpine:latest"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/diagnostics", nil)
	w := httptest.NewRecorder()

	routes.Diagnostics(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body map[string]interface{}
	decodeSystemResponse(t, w, &body)
	for _, field := range []string{"generated_at", "build", "process", "store", "limits", "scheduler", "quotas", "rate_limit", "providers", "sandboxes", "events", "operations", "redactions"} {
		if _, ok := body[field]; !ok {
			t.Fatalf("diagnostics missing %s: %#v", field, body)
		}
	}
	quotas := body["quotas"].(map[string]interface{})
	if quotas["total"].(float64) != 1 || quotas["with_max_sandboxes"].(float64) != 1 {
		t.Fatalf("unexpected quota summary: %#v", quotas)
	}
	storeBody := body["store"].(map[string]interface{})
	if storeBody["healthy"] != true {
		t.Fatalf("store healthy = %v, want true", storeBody["healthy"])
	}
	if strings.Contains(w.Body.String(), "X-API-Key") {
		t.Fatal("diagnostics response leaked API key header name")
	}
}

func TestSystemRoutes_MetricsIncludesOperationalBreakdown(t *testing.T) {
	routes, manager := setupSystemRoutes(t, true)
	if _, err := manager.SaveOwnerQuota(context.Background(), orchestrator.OwnerQuota{
		OwnerID:      "team-a",
		MaxSandboxes: 2,
	}); err != nil {
		t.Fatalf("save quota: %v", err)
	}
	if _, err := manager.Spawn(context.Background(), orchestrator.SpawnRequest{Image: "alpine:latest"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	routes.Metrics(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body map[string]interface{}
	decodeSystemResponse(t, w, &body)

	sandboxes := body["sandboxes"].(map[string]interface{})
	if sandboxes["total"].(float64) != 1 {
		t.Fatalf("sandbox total = %v, want 1", sandboxes["total"])
	}
	providersBody := body["providers"].(map[string]interface{})
	if providersBody["healthy"].(float64) != 1 {
		t.Fatalf("healthy providers = %v, want 1", providersBody["healthy"])
	}
	if _, ok := body["events"].(map[string]interface{}); !ok {
		t.Fatal("expected events metrics")
	}
	if _, ok := body["scheduler"].(map[string]interface{}); !ok {
		t.Fatal("expected scheduler metrics")
	}
	quotas := body["quotas"].(map[string]interface{})
	if quotas["total"].(float64) != 1 {
		t.Fatalf("unexpected quota metrics: %#v", quotas)
	}
	if _, ok := body["rate_limit"].(map[string]interface{}); !ok {
		t.Fatal("expected rate limit metrics")
	}
	operations := body["operations"].([]interface{})
	if len(operations) == 0 {
		t.Fatal("expected operation metrics")
	}
}

func TestSystemRoutes_PrometheusMetrics(t *testing.T) {
	routes, manager := setupSystemRoutes(t, true)
	if _, err := manager.SaveOwnerQuota(context.Background(), orchestrator.OwnerQuota{
		OwnerID:      "team-a",
		MaxSandboxes: 2,
	}); err != nil {
		t.Fatalf("save quota: %v", err)
	}
	sb, err := manager.Spawn(context.Background(), orchestrator.SpawnRequest{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if _, err := manager.Exec(context.Background(), sb.ID, orchestrator.ExecRequest{Command: "echo prometheus"}); err != nil {
		t.Fatalf("exec: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics/prometheus", nil)
	w := httptest.NewRecorder()

	routes.PrometheusMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("content type = %q, want text/plain", got)
	}
	body := w.Body.String()
	for _, want := range []string{
		"stacyvm_uptime_seconds",
		"stacyvm_provider_healthy",
		"stacyvm_provider_health_latency_milliseconds",
		"stacyvm_spawn_queue_depth",
		"stacyvm_owner_quotas_total",
		`type="max_sandboxes"`,
		"stacyvm_rate_limit_allowed_total",
		"stacyvm_operation_success_total",
		`operation="spawn"`,
		`operation="exec"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("prometheus body missing %q:\n%s", want, body)
		}
	}
}

func decodeSystemResponse(t *testing.T, w *httptest.ResponseRecorder, dst interface{}) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
