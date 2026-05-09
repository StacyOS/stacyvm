package worker

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/workerproto"
)

func TestRPCClientSpawn(t *testing.T) {
	registry := providers.NewRegistry()
	registry.Register(providers.NewMockProvider())
	if err := registry.SetDefault("mock"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	server := httptest.NewServer((&RPCServer{
		WorkerID: "worker-a",
		Token:    "worker-secret",
		Registry: registry,
	}).Handler())
	defer server.Close()

	client := RPCClient{
		BaseURL:  server.URL,
		WorkerID: "worker-a",
		Token:    "worker-secret",
	}
	result, err := client.Spawn(context.Background(), "req-1", workerproto.LeaseToken{
		ResourceID: "sb-control-plane",
		HolderID:   "worker-a",
		Generation: 1,
		ExpiresAt:  time.Now().UTC().Add(time.Minute),
	}, workerproto.SpawnParams{
		SandboxID: "sb-control-plane",
		Image:     "alpine:latest",
		Provider:  "mock",
		MemoryMB:  512,
		VCPUs:     1,
		TTL:       "5m",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if result.SandboxID != "sb-control-plane" || result.RuntimeID == "" || result.WorkerID != "worker-a" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRPCClientStatus(t *testing.T) {
	registry := providers.NewRegistry()
	mock := providers.NewMockProvider()
	registry.Register(mock)
	if err := registry.SetDefault("mock"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	runtimeID, err := mock.Spawn(context.Background(), providers.SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn mock: %v", err)
	}
	server := httptest.NewServer((&RPCServer{
		WorkerID: "worker-a",
		Token:    "worker-secret",
		Registry: registry,
	}).Handler())
	defer server.Close()

	client := RPCClient{
		BaseURL:  server.URL,
		WorkerID: "worker-a",
		Token:    "worker-secret",
	}
	result, err := client.Status(context.Background(), "req-1", workerproto.StatusParams{
		SandboxID: "sb-control-plane",
		RuntimeID: runtimeID,
		Provider:  "mock",
	})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if result.SandboxID != "sb-control-plane" || result.State == "" || result.WorkerID != "worker-a" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRPCClientExec(t *testing.T) {
	registry := providers.NewRegistry()
	mock := providers.NewMockProvider()
	registry.Register(mock)
	if err := registry.SetDefault("mock"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	runtimeID, err := mock.Spawn(context.Background(), providers.SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn mock: %v", err)
	}
	server := httptest.NewServer((&RPCServer{
		WorkerID: "worker-a",
		Token:    "worker-secret",
		Registry: registry,
	}).Handler())
	defer server.Close()

	client := RPCClient{
		BaseURL:  server.URL,
		WorkerID: "worker-a",
		Token:    "worker-secret",
	}
	result, err := client.Exec(context.Background(), "req-1", workerproto.ExecParams{
		SandboxID: "sb-control-plane",
		RuntimeID: runtimeID,
		Provider:  "mock",
		Command:   "echo client exec",
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if result.SandboxID != "sb-control-plane" || result.ExitCode != 0 || result.Stdout != "client exec\n" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRPCClientDestroy(t *testing.T) {
	registry := providers.NewRegistry()
	mock := providers.NewMockProvider()
	registry.Register(mock)
	if err := registry.SetDefault("mock"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	runtimeID, err := mock.Spawn(context.Background(), providers.SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn mock: %v", err)
	}
	server := httptest.NewServer((&RPCServer{
		WorkerID: "worker-a",
		Token:    "worker-secret",
		Registry: registry,
	}).Handler())
	defer server.Close()

	client := RPCClient{
		BaseURL:  server.URL,
		WorkerID: "worker-a",
		Token:    "worker-secret",
	}
	err = client.Destroy(context.Background(), "req-1", workerproto.LeaseToken{
		ResourceID: "sb-control-plane",
		HolderID:   "worker-a",
		Generation: 1,
		ExpiresAt:  time.Now().UTC().Add(time.Minute),
	}, workerproto.DestroyParams{
		SandboxID: "sb-control-plane",
		RuntimeID: runtimeID,
		Provider:  "mock",
	})
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}
	status, err := mock.Status(context.Background(), runtimeID)
	if err != nil {
		t.Fatalf("status after destroy: %v", err)
	}
	if status.State != "destroyed" {
		t.Fatalf("state after destroy = %q, want destroyed", status.State)
	}
}
