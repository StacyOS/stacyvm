package main

import (
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/api/middleware"
)

func TestIssueWorkerTokenSignsExpectedClaims(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	token, err := issueWorkerToken(workerTokenIssueOptions{
		WorkerID:   " worker-a ",
		SigningKey: "worker-signing-key-with-at-least-32-bytes",
		TTL:        "10m",
		Scopes:     []string{middleware.ScopeWorkerHeartbeat},
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("issue worker token: %v", err)
	}

	claims, ok := middleware.VerifyWorkerToken("worker-signing-key-with-at-least-32-bytes", token, now.Add(time.Minute))
	if !ok {
		t.Fatal("issued token did not verify")
	}
	if claims.WorkerID != "worker-a" {
		t.Fatalf("worker id = %q, want worker-a", claims.WorkerID)
	}
	if claims.Audience != middleware.WorkerTokenAudienceControlPlane {
		t.Fatalf("audience = %q, want %q", claims.Audience, middleware.WorkerTokenAudienceControlPlane)
	}
	if claims.IssuedAt != now.Unix() {
		t.Fatalf("issued at = %d, want %d", claims.IssuedAt, now.Unix())
	}
	if claims.ExpiresAt != now.Add(10*time.Minute).Unix() {
		t.Fatalf("expires at = %d, want %d", claims.ExpiresAt, now.Add(10*time.Minute).Unix())
	}
	if len(claims.Scopes) != 1 || claims.Scopes[0] != middleware.ScopeWorkerHeartbeat {
		t.Fatalf("scopes = %#v, want heartbeat scope", claims.Scopes)
	}
}

func TestIssueWorkerTokenRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		opts workerTokenIssueOptions
	}{
		{name: "missing worker", opts: workerTokenIssueOptions{SigningKey: "worker-signing-key-with-at-least-32-bytes", TTL: "5m"}},
		{name: "missing signing key", opts: workerTokenIssueOptions{WorkerID: "worker-a", TTL: "5m"}},
		{name: "bad ttl", opts: workerTokenIssueOptions{WorkerID: "worker-a", SigningKey: "worker-signing-key-with-at-least-32-bytes", TTL: "soon"}},
		{name: "zero ttl", opts: workerTokenIssueOptions{WorkerID: "worker-a", SigningKey: "worker-signing-key-with-at-least-32-bytes", TTL: "0s"}},
		{name: "bad audience", opts: workerTokenIssueOptions{WorkerID: "worker-a", SigningKey: "worker-signing-key-with-at-least-32-bytes", TTL: "5m", Audience: "admin"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := issueWorkerToken(tt.opts); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
