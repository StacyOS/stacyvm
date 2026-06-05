package ssh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	gossh "golang.org/x/crypto/ssh"
	"nhooyr.io/websocket"
)

func TestServeWebSocketTunnel(t *testing.T) {
	_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	clientSigner, _ := gossh.NewSignerFromKey(clientPriv)
	fp := gossh.FingerprintSHA256(clientSigner.PublicKey())

	backend := &fakeBackend{
		allowedFP: fp,
		identity:  Identity{Subject: "alice", OwnerID: "alice"},
		pty:       newFakePTY("hi", 0),
	}
	srv := NewServer(backend, newTestSigner(t), zerolog.Nop(), ServerConfig{})

	httpSrv := httptest.NewServer(http.HandlerFunc(srv.ServeWebSocket))
	defer httpSrv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http")
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	nc := websocket.NetConn(ctx, c, websocket.MessageBinary)

	sshConn, chans, reqs, err := gossh.NewClientConn(nc, "sb-test", &gossh.ClientConfig{
		User:            "sb-test",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(clientSigner)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("ssh handshake over ws: %v", err)
	}
	client := gossh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	defer sess.Close()

	var out bytes.Buffer
	sess.Stdout = &out
	if err := sess.Run("true"); err != nil {
		t.Fatalf("run over ws tunnel: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("hi")) {
		t.Fatalf("output = %q, want it to contain %q", out.String(), "hi")
	}
	if backend.gotSB != "sb-test" {
		t.Fatalf("backend sandbox = %q, want sb-test", backend.gotSB)
	}
}
