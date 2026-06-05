package ssh

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"testing"
	"time"

	"github.com/rs/zerolog"
	gossh "golang.org/x/crypto/ssh"
)

func TestSignAndCheckUserCertificate(t *testing.T) {
	caSigner := newTestSigner(t)
	_, userPriv, _ := ed25519.GenerateKey(rand.Reader)
	userSigner, _ := gossh.NewSignerFromKey(userPriv)

	cert, err := SignUserCertificate(caSigner, userSigner.PublicKey(),
		Identity{Subject: "alice", OwnerID: "alice", TenantID: "acme"}, "sb-test", 10*time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	checker := &gossh.CertChecker{
		IsUserAuthority: func(auth gossh.PublicKey) bool {
			return bytes.Equal(auth.Marshal(), caSigner.PublicKey().Marshal())
		},
	}
	if err := checker.CheckCert("sb-test", cert); err != nil {
		t.Fatalf("valid cert rejected: %v", err)
	}
	// Wrong sandbox principal must be rejected.
	if err := checker.CheckCert("sb-other", cert); err == nil {
		t.Fatal("cert accepted for wrong sandbox principal")
	}

	id := identityFromCert(cert)
	if id.Subject != "alice" || id.OwnerID != "alice" || id.TenantID != "acme" {
		t.Fatalf("identity = %+v, want alice/alice/acme", id)
	}
}

func startTestServerWithCA(t *testing.T, b Backend, ca gossh.PublicKey) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := NewServer(b, newTestSigner(t), zerolog.Nop(), ServerConfig{UserCA: ca})
	go srv.Serve(ln)
	t.Cleanup(func() { ln.Close() })
	return ln
}

func TestServerAuthenticatesViaCertificate(t *testing.T) {
	caSigner := newTestSigner(t)

	_, userPriv, _ := ed25519.GenerateKey(rand.Reader)
	userSigner, _ := gossh.NewSignerFromKey(userPriv)
	cert, err := SignUserCertificate(caSigner, userSigner.PublicKey(),
		Identity{Subject: "alice", OwnerID: "alice"}, "sb-test", 10*time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	certSigner, err := gossh.NewCertSigner(cert, userSigner)
	if err != nil {
		t.Fatalf("cert signer: %v", err)
	}

	// Backend has NO registered keys: auth must succeed purely via the cert.
	backend := &fakeBackend{allowedFP: "none", pty: newFakePTY("hi", 0)}
	ln := startTestServerWithCA(t, backend, caSigner.PublicKey())

	client, err := gossh.Dial("tcp", ln.Addr().String(), &gossh.ClientConfig{
		User:            "sb-test",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(certSigner)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial with cert: %v", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	defer sess.Close()
	if err := sess.Run("true"); err != nil {
		t.Fatalf("run: %v", err)
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.gotSB != "sb-test" {
		t.Fatalf("backend sandbox = %q, want sb-test", backend.gotSB)
	}
}
