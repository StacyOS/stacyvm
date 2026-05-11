package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/api/middleware"
)

func TestIssueWorkerTokenSignsExpectedClaims(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	result, err := issueWorkerToken(workerTokenIssueOptions{
		WorkerID:   " worker-a ",
		SigningKey: "worker-signing-key-with-at-least-32-bytes",
		TTL:        "10m",
		Scopes:     []string{middleware.ScopeWorkerHeartbeat},
		TokenID:    "token-id-1",
		NotBefore:  "2m",
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("issue worker token: %v", err)
	}

	if result.TokenID != "token-id-1" || result.NotBefore != now.Add(2*time.Minute).Format(time.RFC3339) {
		t.Fatalf("unexpected token issue result: %+v", result)
	}
	claims, ok := middleware.VerifyWorkerToken("worker-signing-key-with-at-least-32-bytes", result.Token, now.Add(3*time.Minute))
	if !ok {
		t.Fatal("issued token did not verify after not-before")
	}
	if claims.WorkerID != "worker-a" {
		t.Fatalf("worker id = %q, want worker-a", claims.WorkerID)
	}
	if claims.TokenID != "token-id-1" {
		t.Fatalf("token id = %q, want token-id-1", claims.TokenID)
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
	if claims.NotBefore != now.Add(2*time.Minute).Unix() {
		t.Fatalf("not before = %d, want %d", claims.NotBefore, now.Add(2*time.Minute).Unix())
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
		{name: "too long ttl", opts: workerTokenIssueOptions{WorkerID: "worker-a", SigningKey: "worker-signing-key-with-at-least-32-bytes", TTL: (middleware.MaxWorkerTokenTTL + time.Second).String()}},
		{name: "bad audience", opts: workerTokenIssueOptions{WorkerID: "worker-a", SigningKey: "worker-signing-key-with-at-least-32-bytes", TTL: "5m", Audience: "admin"}},
		{name: "bad not before", opts: workerTokenIssueOptions{WorkerID: "worker-a", SigningKey: "worker-signing-key-with-at-least-32-bytes", TTL: "5m", NotBefore: "later"}},
		{name: "negative not before", opts: workerTokenIssueOptions{WorkerID: "worker-a", SigningKey: "worker-signing-key-with-at-least-32-bytes", TTL: "5m", NotBefore: "-1s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := issueWorkerToken(tt.opts); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestWorkerTokenCommandReadsSigningKeyFile(t *testing.T) {
	keyPath := writeTestSecret(t, "worker-signing-key-with-at-least-32-bytes\n")
	cmd := newWorkerTokenCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"worker-a", "--signing-key-file", keyPath, "--token-id", "token-id-1", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker token command: %v", err)
	}
	var result workerTokenIssueResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode worker token json: %v", err)
	}
	if result.TokenID != "token-id-1" || result.WorkerID != "worker-a" {
		t.Fatalf("unexpected token result: %+v", result)
	}
	if _, ok := middleware.VerifyWorkerToken("worker-signing-key-with-at-least-32-bytes", result.Token, time.Now().UTC()); !ok {
		t.Fatal("issued token did not verify with signing key from file")
	}
}

func TestInspectWorkerTokenShowsUnverifiedClaims(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	issued, err := issueWorkerToken(workerTokenIssueOptions{
		WorkerID:   "worker-a",
		SigningKey: "worker-signing-key-with-at-least-32-bytes",
		TTL:        "5m",
		Scopes:     []string{middleware.ScopeWorkerHeartbeat},
		Audience:   middleware.WorkerTokenAudienceRPC,
		TokenID:    "token-id-1",
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("issue worker token: %v", err)
	}

	inspected, err := inspectWorkerToken(issued.Token)
	if err != nil {
		t.Fatalf("inspect worker token: %v", err)
	}
	if inspected.SignatureVerified {
		t.Fatal("inspect result should not report signature verification")
	}
	if inspected.WorkerID != "worker-a" || inspected.TokenID != "token-id-1" || inspected.Audience != middleware.WorkerTokenAudienceRPC {
		t.Fatalf("unexpected inspect result: %+v", inspected)
	}
	if inspected.IssuedAt != now.Format(time.RFC3339) || inspected.ExpiresAt != now.Add(5*time.Minute).Format(time.RFC3339) {
		t.Fatalf("unexpected inspect timestamps: %+v", inspected)
	}
	if len(inspected.Scopes) != 1 || inspected.Scopes[0] != middleware.ScopeWorkerHeartbeat {
		t.Fatalf("scopes = %#v, want heartbeat scope", inspected.Scopes)
	}
}

func TestInspectWorkerTokenRejectsInvalidFormat(t *testing.T) {
	if _, err := inspectWorkerToken("not-a-worker-token"); err == nil {
		t.Fatal("expected error")
	}
}

func TestVerifyWorkerTokenAcceptsRotationKey(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	issued, err := issueWorkerToken(workerTokenIssueOptions{
		WorkerID:   "worker-a",
		SigningKey: "old-worker-signing-key-with-at-least-32-bytes",
		TTL:        "5m",
		Audience:   middleware.WorkerTokenAudienceRPC,
		TokenID:    "token-id-1",
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("issue worker token: %v", err)
	}

	verified, err := verifyWorkerToken(workerTokenVerifyOptions{
		Token:           issued.Token,
		SigningKey:      "new-worker-signing-key-with-at-least-32-bytes",
		VerificationKey: []string{"old-worker-signing-key-with-at-least-32-bytes"},
		Audience:        middleware.WorkerTokenAudienceRPC,
		WorkerID:        "worker-a",
		Now:             func() time.Time { return now.Add(time.Minute) },
	})
	if err != nil {
		t.Fatalf("verify worker token: %v", err)
	}
	if !verified.SignatureVerified {
		t.Fatal("verify result should report signature verification")
	}
	if verified.TokenID != "token-id-1" || verified.WorkerID != "worker-a" || verified.Audience != middleware.WorkerTokenAudienceRPC {
		t.Fatalf("unexpected verify result: %+v", verified)
	}
}

func TestVerifyWorkerTokenRejectsRevokedTokenID(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	issued, err := issueWorkerToken(workerTokenIssueOptions{
		WorkerID:   "worker-a",
		SigningKey: "worker-signing-key-with-at-least-32-bytes",
		TTL:        "5m",
		TokenID:    "revoked-token-id",
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("issue worker token: %v", err)
	}

	if _, err := verifyWorkerToken(workerTokenVerifyOptions{
		Token:           issued.Token,
		SigningKey:      "worker-signing-key-with-at-least-32-bytes",
		RevokedTokenIDs: []string{"revoked-token-id"},
		Now:             func() time.Time { return now.Add(time.Minute) },
	}); err == nil {
		t.Fatal("expected revoked token to fail verification")
	}
}

func TestVerifyWorkerTokenRejectsWrongAudienceAndWorker(t *testing.T) {
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	issued, err := issueWorkerToken(workerTokenIssueOptions{
		WorkerID:   "worker-a",
		SigningKey: "worker-signing-key-with-at-least-32-bytes",
		TTL:        "5m",
		Audience:   middleware.WorkerTokenAudienceControlPlane,
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("issue worker token: %v", err)
	}

	if _, err := verifyWorkerToken(workerTokenVerifyOptions{
		Token:      issued.Token,
		SigningKey: "worker-signing-key-with-at-least-32-bytes",
		Audience:   middleware.WorkerTokenAudienceRPC,
		Now:        func() time.Time { return now.Add(time.Minute) },
	}); err == nil {
		t.Fatal("expected wrong audience to fail verification")
	}
	if _, err := verifyWorkerToken(workerTokenVerifyOptions{
		Token:      issued.Token,
		SigningKey: "worker-signing-key-with-at-least-32-bytes",
		WorkerID:   "worker-b",
		Now:        func() time.Time { return now.Add(time.Minute) },
	}); err == nil {
		t.Fatal("expected wrong worker to fail verification")
	}
}

func TestWorkerTokenVerifyCommandReadsSigningKeyFiles(t *testing.T) {
	now := time.Now().UTC()
	issued, err := issueWorkerToken(workerTokenIssueOptions{
		WorkerID:   "worker-a",
		SigningKey: "old-worker-signing-key-with-at-least-32-bytes",
		TTL:        "5m",
		Audience:   middleware.WorkerTokenAudienceRPC,
		TokenID:    "token-id-1",
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("issue worker token: %v", err)
	}
	activeKeyPath := writeTestSecret(t, "new-worker-signing-key-with-at-least-32-bytes")
	oldKeyPath := writeTestSecret(t, "old-worker-signing-key-with-at-least-32-bytes\n")
	cmd := newWorkerTokenVerifyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		issued.Token,
		"--signing-key-file", activeKeyPath,
		"--verification-key-file", oldKeyPath,
		"--worker-id", "worker-a",
		"--audience", middleware.WorkerTokenAudienceRPC,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker token verify command: %v", err)
	}
	var result workerTokenInspectResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode worker token verify json: %v", err)
	}
	if !result.SignatureVerified || result.TokenID != "token-id-1" {
		t.Fatalf("unexpected verify result: %+v", result)
	}
}

func TestWorkerTokenRotationPlanCommandOutputsJSON(t *testing.T) {
	cmd := newWorkerTokenRotationPlanCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"--new-key-ref", "/run/secrets/worker-signing-key-new",
		"--previous-key-ref", "/run/secrets/worker-signing-key-old",
		"--ttl", "10m",
		"--format", "json",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker token rotation-plan command: %v", err)
	}
	var result workerTokenRotationPlanResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode rotation plan json: %v", err)
	}
	if result.NewKeyRef != "/run/secrets/worker-signing-key-new" || result.PreviousKeyRef != "/run/secrets/worker-signing-key-old" {
		t.Fatalf("unexpected key refs: %+v", result)
	}
	if result.MaxTokenTTL != "10m0s" {
		t.Fatalf("max token ttl = %q, want 10m0s", result.MaxTokenTTL)
	}
	if len(result.Steps) != 5 || len(result.Validation) != 3 || result.ConfigSnippet == "" {
		t.Fatalf("unexpected rotation plan: %+v", result)
	}
}

func TestWorkerTokenRotationPlanRejectsInvalidTTL(t *testing.T) {
	if _, err := workerTokenRotationPlan(workerTokenRotationPlanOptions{
		NewKeyRef:      "new",
		PreviousKeyRef: "old",
		TTL:            (middleware.MaxWorkerTokenTTL + time.Second).String(),
	}); err == nil {
		t.Fatal("expected overlong ttl to fail")
	}
}

func TestReadSecretFileRejectsEmptySecret(t *testing.T) {
	path := writeTestSecret(t, "\n")
	if _, err := readSecretFile(path); err == nil {
		t.Fatal("expected empty secret file to fail")
	}
}

func TestFileWorkerTokenFuncReloadsTokenFile(t *testing.T) {
	path := writeTestSecret(t, "first-token\n")
	tokenFunc := fileWorkerTokenFunc(path)

	got, err := tokenFunc()
	if err != nil {
		t.Fatalf("read first token: %v", err)
	}
	if got != "first-token" {
		t.Fatalf("first token = %q, want first-token", got)
	}

	if err := os.WriteFile(path, []byte("second-token\n"), 0o600); err != nil {
		t.Fatalf("rotate token file: %v", err)
	}
	got, err = tokenFunc()
	if err != nil {
		t.Fatalf("read rotated token: %v", err)
	}
	if got != "second-token" {
		t.Fatalf("rotated token = %q, want second-token", got)
	}
}

func writeTestSecret(t *testing.T, value string) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "secret-*")
	if err != nil {
		t.Fatalf("create secret file: %v", err)
	}
	if _, err := file.WriteString(value); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close secret file: %v", err)
	}
	return file.Name()
}
