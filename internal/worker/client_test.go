package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/workerproto"
)

func TestClientHeartbeatSendsWorkerCredentials(t *testing.T) {
	var gotPath string
	var gotWorkerID string
	var gotToken string
	var gotBody workerproto.HeartbeatParams
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotWorkerID = r.Header.Get("X-Worker-ID")
		gotToken = r.Header.Get("X-Worker-Token")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := Client{
		BaseURL:  server.URL,
		WorkerID: "worker-a",
		Token:    "worker-secret",
	}
	err := client.Heartbeat(context.Background(), workerproto.HeartbeatParams{
		Hostname:     "host-a",
		Status:       "online",
		Providers:    []string{"mock"},
		Capabilities: []string{"heartbeat"},
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if gotPath != "/api/v1/worker/worker-a/heartbeat" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotWorkerID != "worker-a" || gotToken != "worker-secret" {
		t.Fatalf("unexpected credentials: worker_id=%q token=%q", gotWorkerID, gotToken)
	}
	if gotBody.Hostname != "host-a" || gotBody.Providers[0] != "mock" {
		t.Fatalf("unexpected body: %+v", gotBody)
	}
}

func TestClientHeartbeatRejectsMissingConfig(t *testing.T) {
	if err := (Client{}).Heartbeat(context.Background(), workerproto.HeartbeatParams{}); err == nil {
		t.Fatal("expected missing config error")
	}
}

func TestClientRenewLeaseSendsWorkerCredentials(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Minute)
	var gotPath string
	var gotWorkerID string
	var gotToken string
	var gotBody workerproto.RenewLeaseParams
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotWorkerID = r.Header.Get("X-Worker-ID")
		gotToken = r.Header.Get("X-Worker-Token")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(workerproto.RenewLeaseResult{Lease: workerproto.LeaseToken{
			ResourceID: "sb-1",
			HolderID:   "worker-a",
			Generation: 2,
			ExpiresAt:  expiresAt,
		}})
	}))
	defer server.Close()

	client := Client{
		BaseURL:  server.URL,
		WorkerID: "worker-a",
		Token:    "worker-secret",
	}
	lease, err := client.RenewLease(context.Background(), "sb-1", "30s")
	if err != nil {
		t.Fatalf("renew lease: %v", err)
	}
	if gotPath != "/api/v1/worker/worker-a/leases/sb-1/renew" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotWorkerID != "worker-a" || gotToken != "worker-secret" {
		t.Fatalf("unexpected credentials: worker_id=%q token=%q", gotWorkerID, gotToken)
	}
	if gotBody.ResourceID != "sb-1" || gotBody.TTL != "30s" {
		t.Fatalf("unexpected body: %+v", gotBody)
	}
	if lease.Generation != 2 || !lease.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("unexpected lease: %+v", lease)
	}
}
