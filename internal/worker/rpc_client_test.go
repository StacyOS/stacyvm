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

func TestRPCClientExecStream(t *testing.T) {
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
	result, err := client.ExecStream(context.Background(), "req-1", workerproto.ExecParams{
		SandboxID: "sb-control-plane",
		RuntimeID: runtimeID,
		Provider:  "mock",
		Command:   "echo client stream",
	})
	if err != nil {
		t.Fatalf("exec stream: %v", err)
	}
	if result.SandboxID != "sb-control-plane" || len(result.Chunks) != 1 || result.Chunks[0].Data != "client stream\n" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRPCClientFileOperations(t *testing.T) {
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
	base := workerproto.FileParams{
		SandboxID: "sb-control-plane",
		RuntimeID: runtimeID,
		Provider:  "mock",
	}
	write := base
	write.Path = "/workspace/client.txt"
	write.Content = []byte("client file")
	write.Mode = "0644"
	if err := client.FileWrite(context.Background(), "req-write", write); err != nil {
		t.Fatalf("write: %v", err)
	}
	read := base
	read.Path = "/workspace/client.txt"
	readResult, err := client.FileRead(context.Background(), "req-read", read)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(readResult.Content) != "client file" {
		t.Fatalf("content = %q, want client file", string(readResult.Content))
	}
	list := base
	list.Path = "/workspace"
	listResult, err := client.FileList(context.Background(), "req-list", list)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listResult.Files) == 0 {
		t.Fatal("expected listed files")
	}
	stat := base
	stat.Path = "/workspace/client.txt"
	statResult, err := client.FileStat(context.Background(), "req-stat", stat)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if statResult.File.Size != int64(len("client file")) {
		t.Fatalf("stat size = %d, want %d", statResult.File.Size, len("client file"))
	}
	glob := base
	glob.Pattern = "/workspace/*.txt"
	globResult, err := client.FileGlob(context.Background(), "req-glob", glob)
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(globResult.Matches) != 1 {
		t.Fatalf("matches = %+v, want one match", globResult.Matches)
	}
	move := base
	move.OldPath = "/workspace/client.txt"
	move.NewPath = "/workspace/client-moved.txt"
	if err := client.FileMove(context.Background(), "req-move", move); err != nil {
		t.Fatalf("move: %v", err)
	}
	chmod := base
	chmod.Path = "/workspace/client-moved.txt"
	chmod.Mode = "0755"
	if err := client.FileChmod(context.Background(), "req-chmod", chmod); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	deleteParams := base
	deleteParams.Path = "/workspace/client-moved.txt"
	if err := client.FileDelete(context.Background(), "req-delete", deleteParams); err != nil {
		t.Fatalf("delete: %v", err)
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
