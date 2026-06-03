//go:build unix

// Package ssh_test exercises the full local SSH pipeline end to end: a real SSH
// client -> gateway -> StoreBackend (real SQLite key lookup + authorization) ->
// orchestrator.Manager.OpenPTYSession -> Mock provider host PTY -> exit status.
package ssh_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/orchestrator"
	"github.com/StacyOs/stacyvm/internal/providers"
	stacyssh "github.com/StacyOs/stacyvm/internal/ssh"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/rs/zerolog"
	gossh "golang.org/x/crypto/ssh"
)

func TestEndToEndSSHIntoSandbox(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	st, err := store.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	reg := providers.NewRegistry()
	reg.Register(providers.NewMockProvider())
	reg.SetDefault("mock")

	mgr := orchestrator.NewManager(reg, st, orchestrator.NewEventBus(), zerolog.Nop(), orchestrator.ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
	})
	mgr.Start()
	t.Cleanup(func() { mgr.Stop() })

	sb, err := mgr.Spawn(ctx, orchestrator.SpawnRequest{Image: "alpine:latest", OwnerID: "alice"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	// Register alice's SSH key.
	_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	clientSigner, _ := gossh.NewSignerFromKey(clientPriv)
	fp := gossh.FingerprintSHA256(clientSigner.PublicKey())
	if err := st.CreateSSHKey(ctx, &store.SSHKeyRecord{
		ID:          "key-alice",
		OwnerID:     "alice",
		Fingerprint: fp,
		PublicKey:   string(gossh.MarshalAuthorizedKey(clientSigner.PublicKey())),
	}); err != nil {
		t.Fatalf("register key: %v", err)
	}

	hostKey, err := stacyssh.LoadOrCreateHostKey(filepath.Join(dir, "hostkey"))
	if err != nil {
		t.Fatalf("host key: %v", err)
	}
	backend := stacyssh.NewStoreBackend(st, st, mgr, zerolog.Nop())
	gateway := stacyssh.NewServer(backend, hostKey, zerolog.Nop(), stacyssh.ServerConfig{})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go gateway.Serve(ln)

	client, err := gossh.Dial("tcp", ln.Addr().String(), &gossh.ClientConfig{
		User:            sb.ID, // address the sandbox by id
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(clientSigner)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	defer sess.Close()
	_ = sess.RequestPty("xterm", 24, 80, gossh.TerminalModes{})

	var out bytes.Buffer
	sess.Stdout = &out
	if err := sess.Run("printf e2e-ok; exit 7"); err != nil {
		exitErr, ok := err.(*gossh.ExitError)
		if !ok {
			t.Fatalf("run error = %v (%T), want *ssh.ExitError", err, err)
		}
		if exitErr.ExitStatus() != 7 {
			t.Fatalf("exit status = %d, want 7", exitErr.ExitStatus())
		}
	} else {
		t.Fatal("expected non-zero exit (7), got success")
	}

	if !bytes.Contains(out.Bytes(), []byte("e2e-ok")) {
		t.Fatalf("output = %q, want it to contain %q", out.String(), "e2e-ok")
	}
}

func TestEndToEndSSHRejectsUnauthorizedSandbox(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	st, err := store.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	reg := providers.NewRegistry()
	reg.Register(providers.NewMockProvider())
	reg.SetDefault("mock")
	mgr := orchestrator.NewManager(reg, st, orchestrator.NewEventBus(), zerolog.Nop(), orchestrator.ManagerConfig{
		DefaultTTL: 5 * time.Minute, DefaultImage: "alpine:latest", DefaultMemory: 512, DefaultVCPUs: 1,
	})
	mgr.Start()
	t.Cleanup(func() { mgr.Stop() })

	// Sandbox owned by bob, in a different tenant.
	sb, err := mgr.Spawn(ctx, orchestrator.SpawnRequest{Image: "alpine:latest", OwnerID: "bob", TenantID: "other"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	clientSigner, _ := gossh.NewSignerFromKey(clientPriv)
	if err := st.CreateSSHKey(ctx, &store.SSHKeyRecord{
		ID: "key-alice", OwnerID: "alice", TenantID: "acme",
		Fingerprint: gossh.FingerprintSHA256(clientSigner.PublicKey()),
		PublicKey:   string(gossh.MarshalAuthorizedKey(clientSigner.PublicKey())),
	}); err != nil {
		t.Fatalf("register key: %v", err)
	}

	hostKey, _ := stacyssh.LoadOrCreateHostKey(filepath.Join(dir, "hostkey"))
	gateway := stacyssh.NewServer(stacyssh.NewStoreBackend(st, st, mgr, zerolog.Nop()), hostKey, zerolog.Nop(), stacyssh.ServerConfig{})
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { ln.Close() })
	go gateway.Serve(ln)

	// Auth succeeds (alice's key is valid) but opening bob's sandbox must fail.
	client, err := gossh.Dial("tcp", ln.Addr().String(), &gossh.ClientConfig{
		User:            sb.ID,
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(clientSigner)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	defer sess.Close()
	if err := sess.Shell(); err == nil {
		// Shell request should be rejected for an unauthorized sandbox.
		if werr := sess.Wait(); werr == nil {
			t.Fatal("unauthorized session unexpectedly succeeded")
		}
	}
}
