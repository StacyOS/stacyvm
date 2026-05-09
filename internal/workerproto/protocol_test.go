package workerproto

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestValidateRequestSpawnRequiresLease(t *testing.T) {
	params, _ := json.Marshal(SpawnParams{
		SandboxID: "sb-1",
		Image:     "alpine:latest",
		Provider:  "mock",
		MemoryMB:  512,
		VCPUs:     1,
		TTL:       "5m",
	})
	err := ValidateRequest(Request{
		ID:       "req-1",
		Method:   MethodSpawn,
		WorkerID: "worker-a",
		Params:   params,
	})
	if !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("validate err = %v, want ErrInvalidMessage", err)
	}
}

func TestValidateRequestAllowsLeasedSpawn(t *testing.T) {
	params, _ := json.Marshal(SpawnParams{
		SandboxID: "sb-1",
		Image:     "alpine:latest",
		Provider:  "mock",
		MemoryMB:  512,
		VCPUs:     1,
		TTL:       "5m",
	})
	err := ValidateRequest(Request{
		ID:       "req-1",
		Method:   MethodSpawn,
		WorkerID: "worker-a",
		Lease: &LeaseToken{
			ResourceID: "sb-1",
			HolderID:   "worker-a",
			Generation: 1,
			ExpiresAt:  time.Now().Add(time.Minute),
		},
		Params: params,
	})
	if err != nil {
		t.Fatalf("validate spawn: %v", err)
	}
}

func TestValidateRequestRejectsUnknownMethod(t *testing.T) {
	err := ValidateRequest(Request{
		ID:       "req-1",
		Method:   "worker.nope",
		WorkerID: "worker-a",
	})
	if !errors.Is(err, ErrUnknownMethod) {
		t.Fatalf("validate err = %v, want ErrUnknownMethod", err)
	}
}

func TestAuthClaimsHasScope(t *testing.T) {
	claims := AuthClaims{
		WorkerID: "worker-a",
		Scopes:   []string{ScopeHeartbeat, ScopeLease},
		Expires:  time.Now().Add(time.Hour),
	}
	if !claims.HasScope(ScopeLease) {
		t.Fatal("expected lease scope")
	}
	if claims.HasScope(ScopeDestroy) {
		t.Fatal("unexpected destroy scope")
	}
}
