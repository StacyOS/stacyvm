package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
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
