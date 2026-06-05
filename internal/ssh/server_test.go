package ssh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/rs/zerolog"
	gossh "golang.org/x/crypto/ssh"
)

// fakePTY is a minimal PTYSession: it emits fixed output then EOF, records
// resize calls, and reports a fixed exit code.
type fakePTY struct {
	out     *bytes.Reader
	exit    int
	resizes chan [2]uint16
}

func newFakePTY(output string, exit int) *fakePTY {
	return &fakePTY{out: bytes.NewReader([]byte(output)), exit: exit, resizes: make(chan [2]uint16, 8)}
}

func (f *fakePTY) Read(p []byte) (int, error)  { return f.out.Read(p) }
func (f *fakePTY) Write(p []byte) (int, error) { return len(p), nil }
func (f *fakePTY) Close() error                { return nil }
func (f *fakePTY) Signal(string) error         { return nil }
func (f *fakePTY) Wait() (int, error)          { return f.exit, nil }
func (f *fakePTY) Resize(cols, rows uint16) error {
	select {
	case f.resizes <- [2]uint16{cols, rows}:
	default:
	}
	return nil
}

type fakeBackend struct {
	allowedFP string
	identity  Identity

	mu      sync.Mutex
	gotSB   string
	gotOpts providers.PTYOptions
	pty     *fakePTY
	openErr error
}

func (b *fakeBackend) LookupKey(_ context.Context, fp string) (Identity, error) {
	if fp != b.allowedFP {
		return Identity{}, errUnknownKey
	}
	return b.identity, nil
}

func (b *fakeBackend) OpenPTY(_ context.Context, _ Identity, sandboxID string, opts providers.PTYOptions) (providers.PTYSession, error) {
	if b.openErr != nil {
		return nil, b.openErr
	}
	b.mu.Lock()
	b.gotSB = sandboxID
	b.gotOpts = opts
	b.mu.Unlock()
	return b.pty, nil
}

func newTestSigner(t *testing.T) gossh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen host key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	return signer
}

func startTestServer(t *testing.T, b Backend) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := NewServer(b, newTestSigner(t), zerolog.Nop(), ServerConfig{})
	go srv.Serve(ln)
	t.Cleanup(func() { ln.Close() })
	return ln
}

func TestServerInteractiveShell(t *testing.T) {
	_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	clientSigner, _ := gossh.NewSignerFromKey(clientPriv)
	fp := gossh.FingerprintSHA256(clientSigner.PublicKey())

	pty := newFakePTY("hi", 5)
	backend := &fakeBackend{
		allowedFP: fp,
		identity:  Identity{Subject: "alice", TenantID: "tenant-acme", OwnerID: "alice"},
		pty:       pty,
	}
	ln := startTestServer(t, backend)

	client, err := gossh.Dial("tcp", ln.Addr().String(), &gossh.ClientConfig{
		User:            "sb-test",
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
		t.Fatalf("new session: %v", err)
	}
	defer sess.Close()

	if err := sess.RequestPty("xterm-256color", 24, 80, gossh.TerminalModes{}); err != nil {
		t.Fatalf("request pty: %v", err)
	}

	var out bytes.Buffer
	sess.Stdout = &out
	if err := sess.Shell(); err != nil {
		t.Fatalf("shell: %v", err)
	}

	werr := sess.Wait()
	exitErr, ok := werr.(*gossh.ExitError)
	if !ok {
		t.Fatalf("wait error = %v (%T), want *ssh.ExitError", werr, werr)
	}
	if exitErr.ExitStatus() != 5 {
		t.Fatalf("exit status = %d, want 5", exitErr.ExitStatus())
	}
	if !bytes.Contains(out.Bytes(), []byte("hi")) {
		t.Fatalf("output = %q, want it to contain %q", out.String(), "hi")
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.gotSB != "sb-test" {
		t.Fatalf("backend sandbox = %q, want sb-test", backend.gotSB)
	}
	if backend.gotOpts.Cols != 80 || backend.gotOpts.Rows != 24 {
		t.Fatalf("pty size = %dx%d, want 80x24", backend.gotOpts.Cols, backend.gotOpts.Rows)
	}
	if backend.gotOpts.Term != "xterm-256color" {
		t.Fatalf("pty term = %q, want xterm-256color", backend.gotOpts.Term)
	}
}

func TestServerRejectsUnknownKey(t *testing.T) {
	backend := &fakeBackend{allowedFP: "SHA256:does-not-match", pty: newFakePTY("", 0)}
	ln := startTestServer(t, backend)

	_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	clientSigner, _ := gossh.NewSignerFromKey(clientPriv)

	_, err := gossh.Dial("tcp", ln.Addr().String(), &gossh.ClientConfig{
		User:            "sb-test",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(clientSigner)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err == nil {
		t.Fatal("dial succeeded, want auth failure for unknown key")
	}
}
