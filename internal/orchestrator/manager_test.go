package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/StacyOs/stacyvm/internal/worker"
	"github.com/rs/zerolog"
)

func setupManager(t *testing.T) *Manager {
	t.Helper()
	return setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
	})
}

func setupManagerWithConfig(t *testing.T, cfg ManagerConfig) *Manager {
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

	events := NewEventBus()
	logger := zerolog.Nop()

	m := NewManager(reg, st, events, logger, cfg)
	m.Start()
	t.Cleanup(func() { m.Stop() })
	return m
}

type slowSpawnProvider struct {
	providers.Provider
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *slowSpawnProvider) Spawn(ctx context.Context, opts providers.SpawnOptions) (string, error) {
	p.once.Do(func() { close(p.entered) })
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-p.release:
	}
	return p.Provider.Spawn(ctx, opts)
}

type cancellableStreamProvider struct {
	providers.Provider
	started chan struct{}
	filled  chan struct{}
	once    sync.Once
	fill    sync.Once
}

func (p *cancellableStreamProvider) ExecStream(ctx context.Context, sandboxID string, opts providers.ExecOptions) (<-chan providers.StreamChunk, error) {
	ch := make(chan providers.StreamChunk, 64)
	go func() {
		defer close(ch)
		p.once.Do(func() { close(p.started) })
		sent := 0
		for {
			select {
			case <-ctx.Done():
				return
			case ch <- providers.StreamChunk{Stream: "stdout", Data: "streaming\n"}:
				sent++
				if sent >= 128 {
					p.fill.Do(func() { close(p.filled) })
				}
			}
		}
	}()
	return ch, nil
}

func TestManager_SpawnAndGet(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, err := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if sb.State != StateRunning {
		t.Fatalf("expected running, got %s", sb.State)
	}

	got, err := m.Get(ctx, sb.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != sb.ID {
		t.Fatalf("ID mismatch")
	}
}

func TestManager_RemoteSpawnUsesWorkerRPC(t *testing.T) {
	remoteRegistry := providers.NewRegistry()
	remoteMock := providers.NewMockProvider()
	remoteRegistry.Register(remoteMock)
	if err := remoteRegistry.SetDefault("mock"); err != nil {
		t.Fatalf("set remote default: %v", err)
	}
	server := httptest.NewServer(worker.RPCServer{
		WorkerID: "worker-remote",
		Token:    "worker-secret",
		Registry: remoteRegistry,
	}.Handler())
	defer server.Close()

	m := setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
		WorkerToken:   "worker-secret",
	})
	providersJSON, _ := json.Marshal([]string{"mock"})
	capacityJSON, _ := json.Marshal(map[string]interface{}{
		"max_sandboxes": 10,
		"rpc_url":       server.URL,
	})
	now := time.Now().UTC()
	if err := m.store.SaveWorker(context.Background(), &store.WorkerRecord{
		ID:            "worker-remote",
		Hostname:      "remote-host",
		Status:        "online",
		Providers:     string(providersJSON),
		Capabilities:  `["remote_worker","spawn"]`,
		Capacity:      string(capacityJSON),
		LastHeartbeat: now,
	}); err != nil {
		t.Fatalf("save worker: %v", err)
	}

	sb, err := m.Spawn(context.Background(), SpawnRequest{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if sb.WorkerID != "worker-remote" {
		t.Fatalf("worker id = %q, want worker-remote", sb.WorkerID)
	}
	if sb.VMID == "" {
		t.Fatal("expected remote runtime id in VMID")
	}
	if _, err := remoteMock.Status(context.Background(), sb.VMID); err != nil {
		t.Fatalf("remote runtime status: %v", err)
	}
	rec, err := m.store.GetSandbox(context.Background(), sb.ID)
	if err != nil {
		t.Fatalf("get sandbox record: %v", err)
	}
	if rec.WorkerID != "worker-remote" || rec.VMID != sb.VMID {
		t.Fatalf("unexpected record ownership: %+v", rec)
	}
	lease, err := m.store.GetLease(context.Background(), sb.ID)
	if err != nil {
		t.Fatalf("get lease: %v", err)
	}
	if lease.HolderID != "worker-remote" {
		t.Fatalf("lease holder = %q, want worker-remote", lease.HolderID)
	}
}

func TestManager_List(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})
	m.Spawn(ctx, SpawnRequest{Image: "ubuntu:latest"})

	list, err := m.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
}

func TestManager_ReconcileMarksMissingRuntimeDestroyed(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := m.store.CreateSandbox(ctx, &store.SandboxRecord{
		ID:        "sb-missing-runtime",
		State:     string(StateRunning),
		Provider:  "mock",
		Image:     "alpine:latest",
		MemoryMB:  512,
		VCPUs:     1,
		Metadata:  "{}",
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create stale sandbox record: %v", err)
	}

	if err := m.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	rec, err := m.store.GetSandbox(ctx, "sb-missing-runtime")
	if err != nil {
		t.Fatalf("get reconciled sandbox: %v", err)
	}
	if rec.State != string(StateDestroyed) {
		t.Fatalf("expected destroyed after reconcile, got %s", rec.State)
	}
	assertEventType(t, m.events.History(10), EventReconcileAction)
}

type runtimeListerProvider struct {
	providers.Provider
	runtimes []providers.RuntimeSandbox
}

func (p *runtimeListerProvider) ListRuntimeSandboxes(ctx context.Context) ([]providers.RuntimeSandbox, error) {
	return p.runtimes, nil
}

func TestManager_ReconcileAdoptsProviderRuntime(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	reg := providers.NewRegistry()
	mock := &runtimeListerProvider{
		Provider: providers.NewMockProvider(),
		runtimes: []providers.RuntimeSandbox{{
			ID:        "sb-adopted-runtime",
			State:     string(StateRunning),
			Provider:  "mock",
			Image:     "alpine:latest",
			CreatedAt: time.Now().UTC(),
			Metadata:  map[string]string{"source": "runtime"},
		}},
	}
	reg.Register(mock)
	reg.SetDefault("mock")

	m := NewManager(reg, st, NewEventBus(), zerolog.Nop(), ManagerConfig{
		DefaultTTL:    time.Hour,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
	})

	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	rec, err := st.GetSandbox(context.Background(), "sb-adopted-runtime")
	if err != nil {
		t.Fatalf("get adopted sandbox: %v", err)
	}
	if rec.State != string(StateRunning) {
		t.Fatalf("expected running adopted state, got %s", rec.State)
	}
	if rec.Provider != "mock" {
		t.Fatalf("expected mock provider, got %s", rec.Provider)
	}
	assertEventType(t, m.events.History(10), EventReconcileAction)
}

func TestManager_Exec(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, _ := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})

	result, err := m.Exec(ctx, sb.ID, ExecRequest{Command: "echo hello from manager"})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if result.Stdout == "" {
		t.Fatal("expected stdout")
	}
}

func TestManager_ExecArgvMode(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, _ := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})

	result, err := m.Exec(ctx, sb.ID, ExecRequest{
		Mode:    providers.ExecModeArgv,
		Command: "printf",
		Args:    []string{"%s", "$HOME && echo injected"},
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.Stdout != "$HOME && echo injected" {
		t.Fatalf("stdout = %q, want literal argv payload", result.Stdout)
	}
}

func TestManager_ExecRejectsUnsupportedMode(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, _ := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})

	_, err := m.Exec(ctx, sb.ID, ExecRequest{
		Mode:    "raw",
		Command: "echo nope",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestManager_OperationMetrics(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, err := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if _, err := m.Exec(ctx, sb.ID, ExecRequest{Command: "echo metrics"}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if err := m.WriteFile(ctx, sb.ID, FileWriteRequest{Path: "/workspace/metrics.txt", Content: "ok"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := m.Destroy(ctx, sb.ID); err != nil {
		t.Fatalf("destroy: %v", err)
	}

	metrics := m.OperationMetrics()
	assertOperationMetric(t, metrics, OperationSpawn, "mock")
	assertOperationMetric(t, metrics, OperationExec, "mock")
	assertOperationMetric(t, metrics, OperationFileWrite, "mock")
	assertOperationMetric(t, metrics, OperationDestroy, "mock")
}

func assertOperationMetric(t *testing.T, metrics []OperationMetrics, operation, provider string) {
	t.Helper()
	for _, metric := range metrics {
		if metric.Operation == operation && metric.Provider == provider {
			if metric.SuccessTotal == 0 {
				t.Fatalf("%s/%s success total = 0", operation, provider)
			}
			return
		}
	}
	t.Fatalf("operation metric %s/%s not found in %+v", operation, provider, metrics)
}

func TestManager_ExecTimeout(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, _ := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})

	_, err := m.Exec(ctx, sb.ID, ExecRequest{
		Command: "sleep 1",
		Timeout: "1ms",
	})
	if !errors.Is(err, ErrExecTimeout) {
		t.Fatalf("expected ErrExecTimeout, got %v", err)
	}
	assertEventType(t, m.events.History(10), EventExecTimeout)
}

func TestManager_MaxExecTimeoutLimit(t *testing.T) {
	m := setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
		Limits: OperationalLimits{
			MaxExecTimeout: 50 * time.Millisecond,
		},
	})
	sb, _ := m.Spawn(context.Background(), SpawnRequest{Image: "alpine:latest"})

	_, err := m.Exec(context.Background(), sb.ID, ExecRequest{
		Command: "echo nope",
		Timeout: "1s",
	})
	if !errors.Is(err, providers.ErrResourceLimit) {
		t.Fatalf("expected resource limit, got %v", err)
	}
	assertEventType(t, m.events.History(10), EventResourceLimit)
}

func TestManager_SpawnLimits(t *testing.T) {
	m := setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
		Limits: OperationalLimits{
			MaxSandboxes:         1,
			MaxSandboxesPerOwner: 1,
			MaxTTL:               time.Hour,
		},
	})

	if _, err := m.Spawn(context.Background(), SpawnRequest{OwnerID: "owner-a"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	_, err := m.Spawn(context.Background(), SpawnRequest{OwnerID: "owner-b"})
	if !errors.Is(err, providers.ErrResourceLimit) {
		t.Fatalf("expected total resource limit, got %v", err)
	}
	assertEventType(t, m.events.History(10), EventResourceLimit)
}

func TestManager_SpawnOwnerLimit(t *testing.T) {
	m := setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
		Limits: OperationalLimits{
			MaxSandboxesPerOwner: 1,
		},
	})

	if _, err := m.Spawn(context.Background(), SpawnRequest{OwnerID: "owner-a"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	_, err := m.Spawn(context.Background(), SpawnRequest{OwnerID: "owner-a"})
	if !errors.Is(err, providers.ErrResourceLimit) {
		t.Fatalf("expected owner resource limit, got %v", err)
	}
}

func TestManager_SpawnOwnerIDValidation(t *testing.T) {
	m := setupManager(t)

	sb, err := m.Spawn(context.Background(), SpawnRequest{OwnerID: " owner-trimmed "})
	if err != nil {
		t.Fatalf("spawn trimmed owner: %v", err)
	}
	if sb.OwnerID != "owner-trimmed" {
		t.Fatalf("owner_id = %q, want owner-trimmed", sb.OwnerID)
	}

	for _, ownerID := range []string{"owner/a", "owner a", strings.Repeat("a", 129)} {
		if _, err := m.Spawn(context.Background(), SpawnRequest{OwnerID: ownerID}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid owner for %q, got %v", ownerID, err)
		}
	}
}

func TestManager_PersistentOwnerQuotaLimit(t *testing.T) {
	m := setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
	})
	_, err := m.SaveOwnerQuota(context.Background(), OwnerQuota{
		OwnerID:        "owner-quota",
		MaxSandboxes:   1,
		MaxTTL:         "30m",
		MaxExecTimeout: "2s",
	})
	if err != nil {
		t.Fatalf("save owner quota: %v", err)
	}

	if _, err := m.Spawn(context.Background(), SpawnRequest{OwnerID: "owner-quota", TTL: "10m"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	_, err = m.Spawn(context.Background(), SpawnRequest{OwnerID: "owner-quota", TTL: "10m"})
	if !errors.Is(err, providers.ErrResourceLimit) {
		t.Fatalf("expected owner quota resource limit, got %v", err)
	}

	usage, err := m.OwnerUsage(context.Background(), "owner-quota")
	if err != nil {
		t.Fatalf("owner usage: %v", err)
	}
	if !usage.QuotaConfigured || usage.ActiveSandboxes != 1 || usage.MaxSandboxes != 1 {
		t.Fatalf("unexpected owner usage: %+v", usage)
	}
	assertEventType(t, m.events.History(20), EventQuotaSaved)
}

func TestManager_OwnerQuotaDeletePublishesEvent(t *testing.T) {
	m := setupManager(t)

	if _, err := m.SaveOwnerQuota(context.Background(), OwnerQuota{OwnerID: "owner-delete", MaxSandboxes: 1}); err != nil {
		t.Fatalf("save quota: %v", err)
	}
	if err := m.DeleteOwnerQuota(context.Background(), "owner-delete"); err != nil {
		t.Fatalf("delete quota: %v", err)
	}
	assertEventType(t, m.events.History(20), EventQuotaDeleted)
}

func TestManager_QuotaSummary(t *testing.T) {
	m := setupManager(t)

	if _, err := m.SaveOwnerQuota(context.Background(), OwnerQuota{
		OwnerID:      "owner-a",
		MaxSandboxes: 2,
		MaxTTL:       "30s",
	}); err != nil {
		t.Fatalf("save owner-a quota: %v", err)
	}
	if _, err := m.SaveOwnerQuota(context.Background(), OwnerQuota{
		OwnerID:        "owner-b",
		MaxExecTimeout: "5s",
	}); err != nil {
		t.Fatalf("save owner-b quota: %v", err)
	}

	summary, err := m.QuotaSummary(context.Background())
	if err != nil {
		t.Fatalf("quota summary: %v", err)
	}
	if summary.Total != 2 || summary.WithMaxSandboxes != 1 || summary.WithMaxTTL != 1 || summary.WithMaxExecTimeout != 1 {
		t.Fatalf("unexpected quota summary: %+v", summary)
	}
}

func TestManager_PersistentOwnerQuotaTTLLimit(t *testing.T) {
	m := setupManager(t)
	if _, err := m.SaveOwnerQuota(context.Background(), OwnerQuota{OwnerID: "owner-ttl", MaxTTL: "5m"}); err != nil {
		t.Fatalf("save quota: %v", err)
	}
	_, err := m.Spawn(context.Background(), SpawnRequest{OwnerID: "owner-ttl", TTL: "10m"})
	if !errors.Is(err, providers.ErrResourceLimit) {
		t.Fatalf("expected ttl quota resource limit, got %v", err)
	}
}

func TestManager_OwnerQuotaValidation(t *testing.T) {
	m := setupManager(t)

	tests := []OwnerQuota{
		{OwnerID: "   ", MaxSandboxes: 1},
		{OwnerID: "owner/a", MaxSandboxes: 1},
		{OwnerID: "owner a", MaxSandboxes: 1},
		{OwnerID: "owner-a", MaxSandboxes: -1},
		{OwnerID: "owner-a", MaxTTL: "500ms"},
		{OwnerID: "owner-a", MaxExecTimeout: "1.5s"},
	}
	for _, quota := range tests {
		if _, err := m.SaveOwnerQuota(context.Background(), quota); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for %+v, got %v", quota, err)
		}
	}

	saved, err := m.SaveOwnerQuota(context.Background(), OwnerQuota{
		OwnerID:      " owner-trimmed ",
		MaxSandboxes: 2,
		MaxTTL:       "10s",
	})
	if err != nil {
		t.Fatalf("save trimmed owner quota: %v", err)
	}
	if saved.OwnerID != "owner-trimmed" || saved.MaxTTL != "10s" {
		t.Fatalf("unexpected saved quota: %+v", saved)
	}
}

func TestManager_SpawnMaxTTLLimit(t *testing.T) {
	m := setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
		Limits: OperationalLimits{
			MaxTTL: time.Hour,
		},
	})

	_, err := m.Spawn(context.Background(), SpawnRequest{TTL: "2h"})
	if !errors.Is(err, providers.ErrResourceLimit) {
		t.Fatalf("expected ttl resource limit, got %v", err)
	}
}

func TestManager_EvaluateSpawnAdmissionAllowsWhenUnderLimits(t *testing.T) {
	m := setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
		Limits: OperationalLimits{
			MaxSandboxes:         2,
			MaxSandboxesPerOwner: 2,
			MaxTTL:               time.Hour,
		},
	})

	decision, err := m.EvaluateSpawnAdmission(context.Background(), "owner-a", 30*time.Minute)
	if err != nil {
		t.Fatalf("evaluate admission: %v", err)
	}
	if !decision.Allowed || decision.Queueable || decision.Reason != "" {
		t.Fatalf("unexpected admission decision: %+v", decision)
	}
	if decision.MaxSandboxes != 2 || decision.MaxOwnerSandboxes != 2 || decision.MaxTTL != "1h0m0s" {
		t.Fatalf("unexpected admission limits: %+v", decision)
	}
}

func TestManager_EvaluateSpawnAdmissionDeniesQueueableCapacity(t *testing.T) {
	m := setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
		Limits: OperationalLimits{
			MaxSandboxes: 1,
		},
	})

	if _, err := m.Spawn(context.Background(), SpawnRequest{OwnerID: "owner-a"}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	decision, err := m.EvaluateSpawnAdmission(context.Background(), "owner-b", 5*time.Minute)
	if err != nil {
		t.Fatalf("evaluate admission: %v", err)
	}
	if decision.Allowed || !decision.Queueable || decision.Reason != "max_sandboxes" {
		t.Fatalf("unexpected admission decision: %+v", decision)
	}
	if decision.ActiveSandboxes != 1 || decision.MaxSandboxes != 1 {
		t.Fatalf("unexpected admission counts: %+v", decision)
	}
}

func TestManager_EvaluateSpawnAdmissionDeniesNonQueueableTTL(t *testing.T) {
	m := setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
		Limits: OperationalLimits{
			MaxTTL: time.Hour,
		},
	})

	decision, err := m.EvaluateSpawnAdmission(context.Background(), "owner-a", 2*time.Hour)
	if err != nil {
		t.Fatalf("evaluate admission: %v", err)
	}
	if decision.Allowed || decision.Queueable || decision.Reason != "max_ttl" {
		t.Fatalf("unexpected admission decision: %+v", decision)
	}
}

func TestManager_EvaluateSpawnRequestAdmissionSelectsLocalWorker(t *testing.T) {
	m := setupManager(t)
	now := time.Now().UTC()
	for _, worker := range []*store.WorkerRecord{
		{
			ID:            "local",
			Status:        "online",
			Providers:     `["mock"]`,
			Capabilities:  `["spawn"]`,
			Capacity:      `{"max_sandboxes":10}`,
			LastHeartbeat: now,
		},
		{
			ID:            "worker-b",
			Status:        "online",
			Providers:     `["mock"]`,
			Capabilities:  `["spawn"]`,
			Capacity:      `{"max_sandboxes":10}`,
			LastHeartbeat: now,
		},
	} {
		if err := m.store.SaveWorker(context.Background(), worker); err != nil {
			t.Fatalf("save worker: %v", err)
		}
	}

	decision, err := m.EvaluateSpawnRequestAdmission(context.Background(), SpawnRequest{Provider: "mock"})
	if err != nil {
		t.Fatalf("evaluate admission: %v", err)
	}
	if !decision.Allowed || decision.SelectedWorkerID != "local" || decision.EligibleWorkers != 2 {
		t.Fatalf("unexpected worker placement: %+v", decision)
	}
}

func TestManager_EvaluateSpawnRequestAdmissionRejectsRemoteWorkerUntilRPC(t *testing.T) {
	m := setupManager(t)
	now := time.Now().UTC()
	for _, worker := range []*store.WorkerRecord{
		{
			ID:            "local",
			Status:        "draining",
			Providers:     `["mock"]`,
			Capabilities:  `["spawn"]`,
			Capacity:      `{"max_sandboxes":10}`,
			LastHeartbeat: now,
		},
		{
			ID:            "worker-b",
			Status:        "online",
			Providers:     `["mock"]`,
			Capabilities:  `["spawn"]`,
			Capacity:      `{"max_sandboxes":10}`,
			LastHeartbeat: now,
		},
	} {
		if err := m.store.SaveWorker(context.Background(), worker); err != nil {
			t.Fatalf("save worker: %v", err)
		}
	}

	decision, err := m.EvaluateSpawnRequestAdmission(context.Background(), SpawnRequest{Provider: "mock"})
	if err != nil {
		t.Fatalf("evaluate admission: %v", err)
	}
	if decision.Allowed || decision.Queueable || decision.SelectedWorkerID != "worker-b" || decision.Reason != "remote_worker_rpc_unavailable" {
		t.Fatalf("unexpected remote worker decision: %+v", decision)
	}
}

func TestManager_EvaluateSpawnRequestAdmissionRejectsStaleLocalWorker(t *testing.T) {
	m := setupManager(t)
	if err := m.store.SaveWorker(context.Background(), &store.WorkerRecord{
		ID:            "local",
		Status:        "online",
		Providers:     `["mock"]`,
		Capabilities:  `["spawn"]`,
		Capacity:      `{"max_sandboxes":10}`,
		LastHeartbeat: time.Now().UTC().Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("save worker: %v", err)
	}

	decision, err := m.EvaluateSpawnRequestAdmission(context.Background(), SpawnRequest{Provider: "mock"})
	if err != nil {
		t.Fatalf("evaluate admission: %v", err)
	}
	if decision.Allowed || decision.Reason != "worker_unavailable" || decision.EligibleWorkers != 0 {
		t.Fatalf("unexpected stale worker decision: %+v", decision)
	}
}

func TestManager_SpawnAdmissionSerializesConcurrentCreates(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	base := providers.NewMockProvider()
	slow := &slowSpawnProvider{
		Provider: base,
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	reg := providers.NewRegistry()
	reg.Register(slow)
	if err := reg.SetDefault("mock"); err != nil {
		t.Fatalf("set default provider: %v", err)
	}

	m := NewManager(reg, st, NewEventBus(), zerolog.Nop(), ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
		Limits: OperationalLimits{
			MaxSandboxes: 1,
		},
	})
	m.Start()
	t.Cleanup(func() { m.Stop() })

	firstCh := make(chan error, 1)
	go func() {
		_, err := m.Spawn(context.Background(), SpawnRequest{OwnerID: "owner-a"})
		firstCh <- err
	}()

	select {
	case <-slow.entered:
	case <-time.After(time.Second):
		t.Fatal("first spawn did not enter provider")
	}

	secondCh := make(chan error, 1)
	go func() {
		_, err := m.Spawn(context.Background(), SpawnRequest{OwnerID: "owner-b"})
		secondCh <- err
	}()

	select {
	case err := <-secondCh:
		t.Fatalf("second spawn completed before first persisted: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	close(slow.release)
	if err := <-firstCh; err != nil {
		t.Fatalf("first spawn: %v", err)
	}

	err = <-secondCh
	if !errors.Is(err, providers.ErrResourceLimit) {
		t.Fatalf("expected second spawn resource limit, got %v", err)
	}

	list, err := m.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one persisted sandbox, got %d", len(list))
	}
}

func TestManager_SpawnQueueWaitsForCapacity(t *testing.T) {
	m := setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
		Limits: OperationalLimits{
			MaxSandboxes:      1,
			SpawnOverflow:     "queue",
			SpawnQueueTimeout: 500 * time.Millisecond,
			MaxSpawnQueue:     2,
		},
	})
	ctx := context.Background()

	first, err := m.Spawn(ctx, SpawnRequest{OwnerID: "owner-a"})
	if err != nil {
		t.Fatalf("first spawn: %v", err)
	}

	type spawnResult struct {
		sb  *Sandbox
		err error
	}
	resultCh := make(chan spawnResult, 1)
	go func() {
		sb, err := m.Spawn(ctx, SpawnRequest{OwnerID: "owner-b"})
		resultCh <- spawnResult{sb: sb, err: err}
	}()

	select {
	case result := <-resultCh:
		t.Fatalf("second spawn returned before capacity opened: sb=%v err=%v", result.sb, result.err)
	case <-time.After(25 * time.Millisecond):
	}

	if err := m.Destroy(ctx, first.ID); err != nil {
		t.Fatalf("destroy first: %v", err)
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("second spawn: %v", result.err)
		}
		if result.sb == nil || result.sb.OwnerID != "owner-b" {
			t.Fatalf("unexpected second spawn: %+v", result.sb)
		}
	case <-time.After(time.Second):
		t.Fatal("second spawn did not resume after capacity opened")
	}

	events := m.events.History(20)
	assertEventType(t, events, EventSpawnQueued)
	assertEventType(t, events, EventSpawnDequeued)
	status := m.SchedulerStatus()
	if status.SpawnQueuedTotal != 1 || status.SpawnDequeuedTotal != 1 || status.SpawnQueueWaitCount != 1 {
		t.Fatalf("unexpected queue status: %+v", status)
	}
	if status.SpawnQueueWaitTotalMS <= 0 || status.SpawnQueueWaitMaxMS <= 0 || status.SpawnQueueWaitAvgMS <= 0 {
		t.Fatalf("expected positive queue wait metrics: %+v", status)
	}
}

func TestManager_SpawnQueueResumesWhenQuotaChanges(t *testing.T) {
	m := setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
		Limits: OperationalLimits{
			SpawnOverflow:     "queue",
			SpawnQueueTimeout: 500 * time.Millisecond,
			MaxSpawnQueue:     2,
		},
	})
	ctx := context.Background()
	if _, err := m.SaveOwnerQuota(ctx, OwnerQuota{OwnerID: "owner-a", MaxSandboxes: 1}); err != nil {
		t.Fatalf("save initial quota: %v", err)
	}
	if _, err := m.Spawn(ctx, SpawnRequest{OwnerID: "owner-a"}); err != nil {
		t.Fatalf("first spawn: %v", err)
	}

	type spawnResult struct {
		sb  *Sandbox
		err error
	}
	resultCh := make(chan spawnResult, 1)
	go func() {
		sb, err := m.Spawn(ctx, SpawnRequest{OwnerID: "owner-a"})
		resultCh <- spawnResult{sb: sb, err: err}
	}()

	select {
	case result := <-resultCh:
		t.Fatalf("second spawn returned before quota changed: sb=%v err=%v", result.sb, result.err)
	case <-time.After(25 * time.Millisecond):
	}

	if _, err := m.SaveOwnerQuota(ctx, OwnerQuota{OwnerID: "owner-a", MaxSandboxes: 2}); err != nil {
		t.Fatalf("increase quota: %v", err)
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("second spawn: %v", result.err)
		}
		if result.sb == nil || result.sb.OwnerID != "owner-a" {
			t.Fatalf("unexpected second spawn: %+v", result.sb)
		}
	case <-time.After(time.Second):
		t.Fatal("second spawn did not resume after quota changed")
	}

	status := m.SchedulerStatus()
	if status.SpawnQueuedTotal != 1 || status.SpawnDequeuedTotal != 1 {
		t.Fatalf("unexpected queue status: %+v", status)
	}
}

func TestManager_SpawnQueueTimesOut(t *testing.T) {
	m := setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
		Limits: OperationalLimits{
			MaxSandboxes:      1,
			SpawnOverflow:     "queue",
			SpawnQueueTimeout: 20 * time.Millisecond,
			MaxSpawnQueue:     2,
		},
	})

	if _, err := m.Spawn(context.Background(), SpawnRequest{OwnerID: "owner-a"}); err != nil {
		t.Fatalf("first spawn: %v", err)
	}

	_, err := m.Spawn(context.Background(), SpawnRequest{OwnerID: "owner-b"})
	if !errors.Is(err, providers.ErrResourceLimit) {
		t.Fatalf("expected queue timeout resource limit, got %v", err)
	}
	assertEventType(t, m.events.History(20), EventSpawnQueueTimeout)
	status := m.SchedulerStatus()
	if status.SpawnQueuedTotal != 1 || status.SpawnQueueTimeouts != 1 || status.SpawnQueueWaitCount != 1 {
		t.Fatalf("unexpected timeout queue status: %+v", status)
	}
}

func TestManager_SchedulerStatus(t *testing.T) {
	m := setupManagerWithConfig(t, ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
		Limits: OperationalLimits{
			SpawnOverflow:     "queue",
			SpawnQueueTimeout: 10 * time.Second,
			MaxSpawnQueue:     7,
		},
	})

	status := m.SchedulerStatus()
	if status.SpawnOverflow != "queue" || status.MaxSpawnQueue != 7 || status.SpawnQueueTimeout != "10s" || status.AdmissionControl != "worker_aware_local" || status.WorkerID != "local" || status.SelectedWorkerID != "local" || status.EligibleWorkers != 1 {
		t.Fatalf("unexpected scheduler status: %+v", status)
	}
	if status.SpawnQueueDepth != 0 {
		t.Fatalf("queue depth = %d, want 0", status.SpawnQueueDepth)
	}
	if status.SpawnQueuedTotal != 0 || status.SpawnQueueWaitTotal != "0s" || status.SpawnQueueWaitAvg != "0s" {
		t.Fatalf("unexpected empty queue metrics: %+v", status)
	}
}

func TestManager_ExecStreamTimeoutEmitsErrorChunk(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, _ := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})

	ch, err := m.ExecStream(ctx, sb.ID, ExecRequest{
		Command: "sleep 1",
		Timeout: "1ms",
	})
	if err != nil {
		t.Fatalf("exec stream: %v", err)
	}

	var sawTimeout bool
	for chunk := range ch {
		if chunk.Stream == "stderr" && strings.Contains(chunk.Data, ErrExecTimeout.Error()) {
			sawTimeout = true
		}
	}
	if !sawTimeout {
		t.Fatal("expected timeout error chunk")
	}
	assertEventType(t, m.events.History(10), EventExecTimeout)
}

func TestManager_ExecStreamCancellationDoesNotEmitTimeout(t *testing.T) {
	m := setupManager(t)
	base, err := m.registry.Get("mock")
	if err != nil {
		t.Fatalf("get mock provider: %v", err)
	}
	streamProvider := &cancellableStreamProvider{
		Provider: base,
		started:  make(chan struct{}),
		filled:   make(chan struct{}),
	}
	m.registry.Register(streamProvider)

	sb, err := m.Spawn(context.Background(), SpawnRequest{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := m.ExecStream(ctx, sb.ID, ExecRequest{Command: "stream forever"})
	if err != nil {
		t.Fatalf("exec stream: %v", err)
	}

	select {
	case <-streamProvider.filled:
	case <-time.After(time.Second):
		t.Fatal("stream provider did not fill the stream buffers")
	}
	deadline := time.After(time.Second)
	for len(ch) < cap(ch) {
		select {
		case <-deadline:
			t.Fatalf("stream output buffer length = %d, want %d", len(ch), cap(ch))
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	time.Sleep(100 * time.Millisecond)

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for range ch {
		}
	}()
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("stream did not close after cancellation")
	}

	for _, event := range m.events.History(20) {
		if event.Type == EventExecTimeout {
			t.Fatalf("unexpected timeout event after cancellation: %+v", event)
		}
	}
}

func TestManager_PublishesOperationFailureEvent(t *testing.T) {
	m := setupManager(t)

	if _, err := m.Exec(context.Background(), "sb-does-not-exist", ExecRequest{Command: "echo nope"}); err == nil {
		t.Fatal("expected exec error")
	}

	assertEventType(t, m.events.History(10), EventExecFailed)
}

func assertEventType(t *testing.T, events []Event, eventType EventType) Event {
	t.Helper()
	for _, event := range events {
		if event.Type == eventType {
			if event.ID == "" {
				t.Fatalf("event %s has empty ID", eventType)
			}
			if len(event.Data) == 0 {
				t.Fatalf("event %s has empty data", eventType)
			}
			return event
		}
	}
	t.Fatalf("event %s not found in %+v", eventType, events)
	return Event{}
}

func TestManager_WriteAndReadFile(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, _ := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})

	err := m.WriteFile(ctx, sb.ID, FileWriteRequest{
		Path:    "/workspace/greeting.txt",
		Content: "hello stacyvm",
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := m.ReadFile(ctx, sb.ID, "/workspace/greeting.txt")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello stacyvm" {
		t.Fatalf("expected 'hello stacyvm', got %q", string(data))
	}
}

func TestManager_PooledScopedPathRejectsTraversal(t *testing.T) {
	m := setupManager(t)
	sb := &Sandbox{ID: "sb-pool", VMID: "vm-shared"}

	for _, path := range []string{
		"../../etc/passwd",
		"../sb-other/secret.txt",
		"/../../../root/.ssh/id_rsa",
		"/workspace/../../../etc/shadow",
	} {
		if got, err := m.scopedPathForOperation(sb, path); err == nil {
			t.Fatalf("scopedPathForOperation(%q) = %q, want traversal error", path, got)
		}
	}
}

func TestManager_FileOperationsRejectPooledTraversal(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()
	sb := &Sandbox{ID: "sb-pool", VMID: "vm-shared", Provider: "mock", OwnerID: "owner-a"}

	m.mu.Lock()
	m.sandboxes[sb.ID] = sb
	m.mu.Unlock()

	cases := []struct {
		name string
		run  func() error
	}{
		{"write", func() error { return m.WriteFile(ctx, sb.ID, FileWriteRequest{Path: "../../escape.txt", Content: "x"}) }},
		{"read", func() error { _, err := m.ReadFile(ctx, sb.ID, "../../escape.txt"); return err }},
		{"list", func() error { _, err := m.ListFiles(ctx, sb.ID, "../../"); return err }},
		{"delete", func() error { return m.DeleteFile(ctx, sb.ID, FileDeleteRequest{Path: "../../escape.txt"}) }},
		{"move_old", func() error {
			return m.MoveFile(ctx, sb.ID, FileMoveRequest{OldPath: "../../escape.txt", NewPath: "ok.txt"})
		}},
		{"move_new", func() error {
			return m.MoveFile(ctx, sb.ID, FileMoveRequest{OldPath: "ok.txt", NewPath: "../../escape.txt"})
		}},
		{"chmod", func() error { return m.ChmodFile(ctx, sb.ID, FileChmodRequest{Path: "../../escape.txt", Mode: "0644"}) }},
		{"stat", func() error { _, err := m.StatFile(ctx, sb.ID, "../../escape.txt"); return err }},
		{"glob", func() error { _, err := m.GlobFiles(ctx, sb.ID, "../../*"); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestManager_OperationAuditForExecAndFile(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, _ := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest", OwnerID: "owner-a"})
	if _, err := m.Exec(ctx, sb.ID, ExecRequest{Command: "echo audit"}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if err := m.WriteFile(ctx, sb.ID, FileWriteRequest{Path: "/workspace/audit.txt", Content: "ok"}); err != nil {
		t.Fatalf("write: %v", err)
	}

	records, err := m.store.ListOperationAudit(ctx, store.OperationAuditQuery{SandboxID: sb.ID, Limit: 20})
	if err != nil {
		t.Fatalf("list operation audit: %v", err)
	}
	seen := map[string]bool{}
	for _, rec := range records {
		seen[rec.Action] = true
		if rec.Actor != "owner-a" {
			t.Fatalf("actor = %q, want owner-a in record %+v", rec.Actor, rec)
		}
	}
	for _, action := range []string{"sandbox.spawn", "exec", "file.write"} {
		if !seen[action] {
			t.Fatalf("missing audit action %s in %+v", action, records)
		}
	}
}

func TestManager_Destroy(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, _ := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})

	err := m.Destroy(ctx, sb.ID)
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}

	// Exec should fail on destroyed sandbox
	_, err = m.Exec(ctx, sb.ID, ExecRequest{Command: "echo"})
	if err == nil {
		t.Fatal("expected error after destroy")
	}
}

func TestManager_SpawnCreatesAndDestroyReleasesLease(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, err := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	lease, err := m.store.GetLease(ctx, sb.ID)
	if err != nil {
		t.Fatalf("get lease: %v", err)
	}
	if lease.HolderID != "local" || lease.ResourceType != "sandbox" {
		t.Fatalf("unexpected lease: %+v", lease)
	}

	if err := m.Destroy(ctx, sb.ID); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if _, err := m.store.GetLease(ctx, sb.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get released lease err = %v, want ErrNotFound", err)
	}
}

func TestManager_DestroyRequiresLease(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, err := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if err := m.store.ReleaseLease(ctx, sb.ID, "local"); err != nil {
		t.Fatalf("release local lease: %v", err)
	}
	if _, err := m.store.AcquireLease(ctx, sb.ID, "sandbox", "worker-b", time.Hour); err != nil {
		t.Fatalf("acquire competing lease: %v", err)
	}

	err = m.Destroy(ctx, sb.ID)
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("destroy err = %v, want ErrConflict", err)
	}
	got, err := m.Get(ctx, sb.ID)
	if err != nil {
		t.Fatalf("get sandbox after failed destroy: %v", err)
	}
	if got.State != StateRunning {
		t.Fatalf("sandbox state = %s, want running", got.State)
	}
}

func TestManager_TTLExpiry(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	// Spawn with very short TTL
	sb, err := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest", TTL: "1ms"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	// Wait for it to expire
	time.Sleep(10 * time.Millisecond)

	// Prune should clean it up
	count, err := m.Prune(ctx)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 pruned, got %d", count)
	}

	// Verify it's gone from listing
	list, _ := m.List(ctx)
	for _, s := range list {
		if s.ID == sb.ID {
			t.Fatal("expected sandbox to be pruned from list")
		}
	}
}

func TestManager_ExtendTTL(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, err := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest", TTL: "5m"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	originalExpiry := sb.ExpiresAt

	// Extend by 30 minutes (extends from now, not from original expiry)
	beforeExtend := time.Now()
	updated, err := m.ExtendTTL(ctx, sb.ID, 30*time.Minute)
	if err != nil {
		t.Fatalf("extend: %v", err)
	}

	// New expiry should be ~30m from now, which is later than the original 5m expiry
	if !updated.ExpiresAt.After(originalExpiry) {
		t.Fatalf("expected new expiry after original, got %v <= %v", updated.ExpiresAt, originalExpiry)
	}
	expectedMin := beforeExtend.Add(30 * time.Minute)
	if updated.ExpiresAt.Before(expectedMin.Add(-time.Second)) {
		t.Fatalf("expected expires_at >= ~%v, got %v", expectedMin, updated.ExpiresAt)
	}

	// Verify via Get
	got, _ := m.Get(ctx, sb.ID)
	if !got.ExpiresAt.Equal(updated.ExpiresAt) {
		t.Fatalf("Get: expected expires_at %v, got %v", updated.ExpiresAt, got.ExpiresAt)
	}
}

func TestManager_ExtendTTL_NotFound(t *testing.T) {
	m := setupManager(t)
	_, err := m.ExtendTTL(context.Background(), "sb-nope", 30*time.Minute)
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox")
	}
}

func TestManager_ExtendTTL_Destroyed(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, _ := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})
	m.Destroy(ctx, sb.ID)

	_, err := m.ExtendTTL(ctx, sb.ID, 30*time.Minute)
	if err == nil {
		t.Fatal("expected error extending destroyed sandbox")
	}
}

func TestManager_StateTransitions(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	// Creating → Running
	sb, err := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if sb.State != StateRunning {
		t.Fatalf("expected running after spawn, got %s", sb.State)
	}

	// Running → Destroyed
	m.Destroy(ctx, sb.ID)
	rec, err := m.store.GetSandbox(ctx, sb.ID)
	if err != nil {
		t.Fatalf("get from store: %v", err)
	}
	if rec.State != string(StateDestroyed) {
		t.Fatalf("expected destroyed in store, got %s", rec.State)
	}
}
