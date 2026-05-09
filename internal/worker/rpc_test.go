package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/workerproto"
)

type fakeLeaseRenewer struct {
	resourceID string
	ttl        string
	lease      workerproto.LeaseToken
}

func (f *fakeLeaseRenewer) RenewLease(ctx context.Context, resourceID, ttl string) (workerproto.LeaseToken, error) {
	f.resourceID = resourceID
	f.ttl = ttl
	return f.lease, nil
}

func TestRPCServerStatus(t *testing.T) {
	registry := providers.NewRegistry()
	mock := providers.NewMockProvider()
	registry.Register(mock)
	if err := registry.SetDefault("mock"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	runtimeID, err := mock.Spawn(t.Context(), providers.SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	handler := (&RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: registry}).Handler()
	params, _ := json.Marshal(workerproto.StatusParams{
		SandboxID: "sb-control-plane",
		Provider:  "mock",
		RuntimeID: runtimeID,
	})
	reqBody, _ := json.Marshal(workerproto.Request{
		ID:       "req-1",
		Method:   workerproto.MethodStatus,
		WorkerID: "worker-a",
		Params:   params,
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(reqBody))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp workerproto.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var result workerproto.StatusResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.SandboxID != "sb-control-plane" || result.WorkerID != "worker-a" || result.State == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRPCServerExec(t *testing.T) {
	registry := providers.NewRegistry()
	mock := providers.NewMockProvider()
	registry.Register(mock)
	if err := registry.SetDefault("mock"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	runtimeID, err := mock.Spawn(t.Context(), providers.SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	handler := (&RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: registry}).Handler()
	params, _ := json.Marshal(workerproto.ExecParams{
		SandboxID: "sb-control-plane",
		Provider:  "mock",
		RuntimeID: runtimeID,
		Command:   "echo worker exec",
	})
	reqBody, _ := json.Marshal(workerproto.Request{
		ID:       "req-1",
		Method:   workerproto.MethodExec,
		WorkerID: "worker-a",
		Params:   params,
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(reqBody))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp workerproto.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var result workerproto.ExecResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.SandboxID != "sb-control-plane" || result.ExitCode != 0 || result.Stdout != "worker exec\n" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRPCServerExecStream(t *testing.T) {
	registry := providers.NewRegistry()
	mock := providers.NewMockProvider()
	registry.Register(mock)
	if err := registry.SetDefault("mock"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	runtimeID, err := mock.Spawn(t.Context(), providers.SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	handler := (&RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: registry}).Handler()
	params, _ := json.Marshal(workerproto.ExecParams{
		SandboxID: "sb-control-plane",
		Provider:  "mock",
		RuntimeID: runtimeID,
		Command:   "echo worker stream",
	})
	reqBody, _ := json.Marshal(workerproto.Request{
		ID:       "req-1",
		Method:   workerproto.MethodExecStream,
		WorkerID: "worker-a",
		Params:   params,
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(reqBody))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp workerproto.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var result workerproto.ExecStreamResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.SandboxID != "sb-control-plane" || len(result.Chunks) != 1 || result.Chunks[0].Data != "worker stream\n" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRPCServerRenewLease(t *testing.T) {
	renewed := workerproto.LeaseToken{
		ResourceID: "sb-1",
		HolderID:   "worker-a",
		Generation: 3,
		ExpiresAt:  time.Now().UTC().Add(2 * time.Minute),
	}
	renewer := &fakeLeaseRenewer{lease: renewed}
	handler := (&RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: providers.NewRegistry(), LeaseRenewer: renewer}).Handler()
	params, _ := json.Marshal(workerproto.RenewLeaseParams{ResourceID: "sb-1", TTL: "30s"})
	reqBody, _ := json.Marshal(workerproto.Request{
		ID:       "req-1",
		Method:   workerproto.MethodRenewLease,
		WorkerID: "worker-a",
		Lease: &workerproto.LeaseToken{
			ResourceID: "sb-1",
			HolderID:   "worker-a",
			Generation: 2,
			ExpiresAt:  time.Now().UTC().Add(time.Minute),
		},
		Params: params,
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(reqBody))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if renewer.resourceID != "sb-1" || renewer.ttl != "30s" {
		t.Fatalf("unexpected renewal call: resource=%q ttl=%q", renewer.resourceID, renewer.ttl)
	}
	var resp workerproto.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var result workerproto.RenewLeaseResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Lease.Generation != 3 {
		t.Fatalf("unexpected lease: %+v", result.Lease)
	}
}

func TestRPCServerSpawn(t *testing.T) {
	registry := providers.NewRegistry()
	mock := providers.NewMockProvider()
	registry.Register(mock)
	if err := registry.SetDefault("mock"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	handler := (&RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: registry}).Handler()
	params, _ := json.Marshal(workerproto.SpawnParams{
		SandboxID: "sb-control-plane",
		Image:     "alpine:latest",
		Provider:  "mock",
		MemoryMB:  512,
		VCPUs:     1,
		TTL:       "5m",
		Metadata:  map[string]string{"purpose": "test"},
	})
	reqBody, _ := json.Marshal(workerproto.Request{
		ID:       "req-1",
		Method:   workerproto.MethodSpawn,
		WorkerID: "worker-a",
		Lease: &workerproto.LeaseToken{
			ResourceID: "sb-control-plane",
			HolderID:   "worker-a",
			Generation: 1,
			ExpiresAt:  time.Now().UTC().Add(time.Minute),
		},
		Params: params,
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(reqBody))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp workerproto.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var result workerproto.SpawnResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.SandboxID != "sb-control-plane" || result.RuntimeID == "" || result.WorkerID != "worker-a" || result.State != "running" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := mock.Status(t.Context(), result.RuntimeID); err != nil {
		t.Fatalf("spawned runtime status: %v", err)
	}
}

func TestRPCServerSpawnRejectsLeaseMismatch(t *testing.T) {
	registry := providers.NewRegistry()
	registry.Register(providers.NewMockProvider())
	handler := (&RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: registry}).Handler()
	params, _ := json.Marshal(workerproto.SpawnParams{
		SandboxID: "sb-control-plane",
		Image:     "alpine:latest",
		Provider:  "mock",
		MemoryMB:  512,
		VCPUs:     1,
		TTL:       "5m",
	})
	reqBody, _ := json.Marshal(workerproto.Request{
		ID:       "req-1",
		Method:   workerproto.MethodSpawn,
		WorkerID: "worker-a",
		Lease: &workerproto.LeaseToken{
			ResourceID: "sb-other",
			HolderID:   "worker-a",
			Generation: 1,
			ExpiresAt:  time.Now().UTC().Add(time.Minute),
		},
		Params: params,
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(reqBody))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRPCServerShutdownDrainsWorker(t *testing.T) {
	registry := providers.NewRegistry()
	registry.Register(providers.NewMockProvider())
	if err := registry.SetDefault("mock"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	rpcServer := &RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: registry}
	handler := rpcServer.Handler()
	shutdownBody, _ := json.Marshal(workerproto.Request{
		ID:       "req-shutdown",
		Method:   workerproto.MethodShutdown,
		WorkerID: "worker-a",
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(shutdownBody))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("shutdown status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !rpcServer.Draining() {
		t.Fatal("expected worker to be draining")
	}

	params, _ := json.Marshal(workerproto.SpawnParams{
		SandboxID: "sb-control-plane",
		Image:     "alpine:latest",
		Provider:  "mock",
		MemoryMB:  512,
		VCPUs:     1,
		TTL:       "5m",
	})
	spawnBody, _ := json.Marshal(workerproto.Request{
		ID:       "req-spawn",
		Method:   workerproto.MethodSpawn,
		WorkerID: "worker-a",
		Lease: &workerproto.LeaseToken{
			ResourceID: "sb-control-plane",
			HolderID:   "worker-a",
			Generation: 1,
			ExpiresAt:  time.Now().UTC().Add(time.Minute),
		},
		Params: params,
	})
	req = httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(spawnBody))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("spawn status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestRPCServerDestroy(t *testing.T) {
	registry := providers.NewRegistry()
	mock := providers.NewMockProvider()
	registry.Register(mock)
	if err := registry.SetDefault("mock"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	runtimeID, err := mock.Spawn(t.Context(), providers.SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	handler := (&RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: registry}).Handler()
	params, _ := json.Marshal(workerproto.DestroyParams{
		SandboxID: "sb-control-plane",
		Provider:  "mock",
		RuntimeID: runtimeID,
	})
	reqBody, _ := json.Marshal(workerproto.Request{
		ID:       "req-1",
		Method:   workerproto.MethodDestroy,
		WorkerID: "worker-a",
		Lease: &workerproto.LeaseToken{
			ResourceID: "sb-control-plane",
			HolderID:   "worker-a",
			Generation: 1,
			ExpiresAt:  time.Now().UTC().Add(time.Minute),
		},
		Params: params,
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(reqBody))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	status, err := mock.Status(t.Context(), runtimeID)
	if err != nil {
		t.Fatalf("status after destroy: %v", err)
	}
	if status.State != "destroyed" {
		t.Fatalf("state after destroy = %q, want destroyed", status.State)
	}
}

func TestRPCServerDestroyRejectsLeaseMismatch(t *testing.T) {
	registry := providers.NewRegistry()
	registry.Register(providers.NewMockProvider())
	handler := (&RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: registry}).Handler()
	params, _ := json.Marshal(workerproto.DestroyParams{
		SandboxID: "sb-control-plane",
		Provider:  "mock",
		RuntimeID: "runtime-1",
	})
	reqBody, _ := json.Marshal(workerproto.Request{
		ID:       "req-1",
		Method:   workerproto.MethodDestroy,
		WorkerID: "worker-a",
		Lease: &workerproto.LeaseToken{
			ResourceID: "sb-other",
			HolderID:   "worker-a",
			Generation: 1,
			ExpiresAt:  time.Now().UTC().Add(time.Minute),
		},
		Params: params,
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(reqBody))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRPCServerRenewLeaseRejectsExpiredToken(t *testing.T) {
	renewer := &fakeLeaseRenewer{}
	handler := (&RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: providers.NewRegistry(), LeaseRenewer: renewer}).Handler()
	params, _ := json.Marshal(workerproto.RenewLeaseParams{ResourceID: "sb-1", TTL: "30s"})
	reqBody, _ := json.Marshal(workerproto.Request{
		ID:       "req-1",
		Method:   workerproto.MethodRenewLease,
		WorkerID: "worker-a",
		Lease: &workerproto.LeaseToken{
			ResourceID: "sb-1",
			HolderID:   "worker-a",
			Generation: 2,
			ExpiresAt:  time.Now().UTC().Add(-time.Second),
		},
		Params: params,
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(reqBody))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	if renewer.resourceID != "" {
		t.Fatalf("renewal should not be called, got resource %q", renewer.resourceID)
	}
}

func TestRPCServerRejectsWrongWorker(t *testing.T) {
	handler := (&RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: providers.NewRegistry()}).Handler()
	reqBody, _ := json.Marshal(workerproto.Request{
		ID:       "req-1",
		Method:   workerproto.MethodShutdown,
		WorkerID: "worker-b",
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(reqBody))
	req.Header.Set("X-Worker-ID", "worker-a")
	req.Header.Set("X-Worker-Token", "worker-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRPCServerRejectsMissingCredentials(t *testing.T) {
	handler := (&RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: providers.NewRegistry()}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
