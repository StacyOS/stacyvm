package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// fakeDialBackend adds direct-tcpip dialing to fakeBackend, dialing a fixed
// local target (an echo server) regardless of the requested address, while
// recording what address the gateway asked for.
type fakeDialBackend struct {
	fakeBackend
	dialTarget string

	dmu     sync.Mutex
	gotAddr string
	dialErr error
}

func (b *fakeDialBackend) DialSandbox(_ context.Context, _ Identity, _ string, addr string) (net.Conn, error) {
	b.dmu.Lock()
	b.gotAddr = addr
	derr := b.dialErr
	b.dmu.Unlock()
	if derr != nil {
		return nil, derr
	}
	return net.Dial("tcp", b.dialTarget)
}

func TestServerDirectTCPIP(t *testing.T) {
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo listen: %v", err)
	}
	t.Cleanup(func() { echoLn.Close() })
	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c) }()
		}
	}()

	_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	clientSigner, _ := gossh.NewSignerFromKey(clientPriv)
	fp := gossh.FingerprintSHA256(clientSigner.PublicKey())

	backend := &fakeDialBackend{
		fakeBackend: fakeBackend{allowedFP: fp, identity: Identity{OwnerID: "alice"}, pty: newFakePTY("", 0)},
		dialTarget:  echoLn.Addr().String(),
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

	// Open a forwarded connection (ssh -L equivalent) to a service "inside" the
	// sandbox; the gateway routes it through the backend dialer.
	conn, err := client.Dial("tcp", "10.1.2.3:6379")
	if err != nil {
		t.Fatalf("direct-tcpip dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != "hello" {
		t.Fatalf("echo = %q, want hello", buf)
	}

	backend.dmu.Lock()
	got := backend.gotAddr
	backend.dmu.Unlock()
	if got != "10.1.2.3:6379" {
		t.Fatalf("backend asked to dial %q, want 10.1.2.3:6379", got)
	}
}

func TestServerDirectTCPIPRejectedWithoutDialer(t *testing.T) {
	_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	clientSigner, _ := gossh.NewSignerFromKey(clientPriv)
	fp := gossh.FingerprintSHA256(clientSigner.PublicKey())

	// Plain fakeBackend does NOT implement Dialer.
	backend := &fakeBackend{allowedFP: fp, identity: Identity{OwnerID: "alice"}, pty: newFakePTY("", 0)}
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

	if _, err := client.Dial("tcp", "10.1.2.3:6379"); err == nil {
		t.Fatal("expected direct-tcpip to be rejected when backend has no dialer")
	}
}
