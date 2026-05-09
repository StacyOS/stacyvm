package worker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/workerproto"
)

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
	handler := RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: registry}.Handler()
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

func TestRPCServerRejectsWrongWorker(t *testing.T) {
	handler := RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: providers.NewRegistry()}.Handler()
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
	handler := RPCServer{WorkerID: "worker-a", Token: "worker-secret", Registry: providers.NewRegistry()}.Handler()
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
