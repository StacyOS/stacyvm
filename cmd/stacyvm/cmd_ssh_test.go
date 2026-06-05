package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	stacyssh "github.com/StacyOs/stacyvm/internal/ssh"
	gossh "golang.org/x/crypto/ssh"
	"nhooyr.io/websocket"
)

func wsTestAccept(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	return websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
}

func wsTestNetConn(ctx context.Context, c *websocket.Conn) net.Conn {
	return websocket.NetConn(ctx, c, websocket.MessageBinary)
}

func TestRenderSSHConfigBlockProxy(t *testing.T) {
	block := renderSSHConfigBlock(sshConfigBlock{
		Alias:    "stacy-sb-123",
		Sandbox:  "sb-123",
		ProxyCmd: "stacyvm ssh proxy sb-123 --server http://localhost:7423",
		Identity: "/home/u/.ssh/id_stacy",
	})

	for _, want := range []string{
		"# >>> stacyvm managed: stacy-sb-123 >>>",
		"# <<< stacyvm managed: stacy-sb-123 <<<",
		"Host stacy-sb-123",
		"User sb-123",
		"ProxyCommand stacyvm ssh proxy sb-123 --server http://localhost:7423",
		"IdentityFile /home/u/.ssh/id_stacy",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("rendered block missing %q\n---\n%s", want, block)
		}
	}
}

func TestRenderSSHConfigBlockDirect(t *testing.T) {
	block := renderSSHConfigBlock(sshConfigBlock{
		Alias:    "stacy-sb-9",
		Sandbox:  "sb-9",
		HostName: "gw.example.com",
		Port:     2222,
	})
	if !strings.Contains(block, "HostName gw.example.com") {
		t.Fatalf("direct block missing HostName:\n%s", block)
	}
	if !strings.Contains(block, "Port 2222") {
		t.Fatalf("direct block missing Port:\n%s", block)
	}
	if strings.Contains(block, "ProxyCommand") {
		t.Fatalf("direct block should not contain ProxyCommand:\n%s", block)
	}
}

func TestUpsertSSHConfigBlockAppends(t *testing.T) {
	existing := "Host other\n    HostName 10.0.0.1\n"
	block := renderSSHConfigBlock(sshConfigBlock{Alias: "stacy-a", Sandbox: "a", HostName: "h", Port: 22})

	out := upsertSSHConfigBlock(existing, "stacy-a", block)

	if !strings.Contains(out, "Host other") {
		t.Fatalf("upsert dropped unrelated content:\n%s", out)
	}
	if !strings.Contains(out, "Host stacy-a") {
		t.Fatalf("upsert did not add new block:\n%s", out)
	}
}

func TestUpsertSSHConfigBlockReplacesInPlace(t *testing.T) {
	first := renderSSHConfigBlock(sshConfigBlock{Alias: "stacy-a", Sandbox: "a", HostName: "old", Port: 22})
	existing := "Host keep\n    HostName 1.2.3.4\n\n" + first + "\nHost keep2\n    HostName 5.6.7.8\n"

	second := renderSSHConfigBlock(sshConfigBlock{Alias: "stacy-a", Sandbox: "a", HostName: "new", Port: 22})
	out := upsertSSHConfigBlock(existing, "stacy-a", second)

	if strings.Contains(out, "HostName old") {
		t.Fatalf("replace left stale content:\n%s", out)
	}
	if !strings.Contains(out, "HostName new") {
		t.Fatalf("replace missing new content:\n%s", out)
	}
	if strings.Contains(out, "HostName 1.2.3.4") == false || strings.Contains(out, "HostName 5.6.7.8") == false {
		t.Fatalf("replace clobbered surrounding blocks:\n%s", out)
	}
	if c := strings.Count(out, "# >>> stacyvm managed: stacy-a >>>"); c != 1 {
		t.Fatalf("expected exactly one managed begin marker, got %d:\n%s", c, out)
	}
}

func TestProxyRelayBidirectional(t *testing.T) {
	c1, c2 := net.Pipe()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	done := make(chan error, 1)
	go func() { done <- proxyRelay(c1, inR, outW) }()

	// stdin -> conn
	go func() { _, _ = inW.Write([]byte("ping")) }()
	buf := make([]byte, 4)
	if _, err := io.ReadFull(c2, buf); err != nil {
		t.Fatalf("read from conn: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("conn got %q, want ping", buf)
	}

	// conn -> stdout
	go func() { _, _ = c2.Write([]byte("pong")) }()
	obuf := make([]byte, 4)
	if _, err := io.ReadFull(outR, obuf); err != nil {
		t.Fatalf("read from out: %v", err)
	}
	if string(obuf) != "pong" {
		t.Fatalf("out got %q, want pong", obuf)
	}

	_ = inW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("proxyRelay returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proxyRelay did not return after stdin closed")
	}
}

func TestRequestSSHCert(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/ssh/certs" {
			t.Errorf("path = %s, want /api/v1/ssh/certs", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "secret" {
			t.Errorf("missing api key header")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["sandbox_id"] != "sb-7" {
			t.Errorf("sandbox_id = %v, want sb-7", body["sandbox_id"])
		}
		if body["public_key"] != "ssh-ed25519 AAAA..." {
			t.Errorf("public_key = %v", body["public_key"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"certificate": "ssh-ed25519-cert-v01@openssh.com AAAACERT",
			"sandbox_id":  "sb-7",
		})
	}))
	defer ts.Close()

	c := newHTTPClient(ts.URL, "secret")
	cert, err := requestSSHCert(c, "sb-7", "ssh-ed25519 AAAA...", 10*time.Minute)
	if err != nil {
		t.Fatalf("requestSSHCert: %v", err)
	}
	if cert != "ssh-ed25519-cert-v01@openssh.com AAAACERT" {
		t.Fatalf("cert = %q", cert)
	}
}

func TestGenerateEphemeralSigner(t *testing.T) {
	signer, authLine, err := generateEphemeralSigner()
	if err != nil {
		t.Fatalf("generateEphemeralSigner: %v", err)
	}
	parsed, _, _, _, err := gossh.ParseAuthorizedKey([]byte(authLine))
	if err != nil {
		t.Fatalf("parse authorized line %q: %v", authLine, err)
	}
	if string(parsed.Marshal()) != string(signer.PublicKey().Marshal()) {
		t.Fatal("authorized line does not match signer public key")
	}
}

func TestWSURLFromServer(t *testing.T) {
	cases := []struct{ in, want string }{
		{"http://localhost:7423", "ws://localhost:7423/api/v1/ssh/connect"},
		{"https://gw.example.com", "wss://gw.example.com/api/v1/ssh/connect"},
		{"http://localhost:7423/", "ws://localhost:7423/api/v1/ssh/connect"},
	}
	for _, tc := range cases {
		got, err := wsURLFromServer(tc.in, "/api/v1/ssh/connect")
		if err != nil {
			t.Fatalf("wsURLFromServer(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("wsURLFromServer(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDialWSTunnel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/ssh/connect" {
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		if r.Header.Get("X-API-Key") != "k" {
			http.Error(w, "no key", http.StatusUnauthorized)
			return
		}
		c, err := wsTestAccept(w, r)
		if err != nil {
			return
		}
		nc := wsTestNetConn(r.Context(), c)
		buf := make([]byte, 4)
		if _, err := io.ReadFull(nc, buf); err != nil {
			return
		}
		_, _ = nc.Write(buf)
		time.Sleep(50 * time.Millisecond)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := dialWSTunnel(ctx, ts.URL, "k")
	if err != nil {
		t.Fatalf("dialWSTunnel: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("echo = %q, want ping", buf)
	}
}

func TestBuildCertSigner(t *testing.T) {
	signer, authLine, err := generateEphemeralSigner()
	if err != nil {
		t.Fatalf("ephemeral: %v", err)
	}
	userKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(authLine))
	if err != nil {
		t.Fatalf("parse user key: %v", err)
	}
	_, caPriv, _ := ed25519.GenerateKey(rand.Reader)
	caSigner, _ := gossh.NewSignerFromKey(caPriv)
	cert, err := stacyssh.SignUserCertificate(caSigner, userKey, stacyssh.Identity{Subject: "u", OwnerID: "u"}, "sb-1", time.Minute)
	if err != nil {
		t.Fatalf("sign cert: %v", err)
	}
	certLine := string(gossh.MarshalAuthorizedKey(cert))

	cs, err := buildCertSigner(signer, certLine)
	if err != nil {
		t.Fatalf("buildCertSigner: %v", err)
	}
	if _, ok := cs.PublicKey().(*gossh.Certificate); !ok {
		t.Fatalf("cert signer public key is %T, want *ssh.Certificate", cs.PublicKey())
	}
}

func TestBuildCertSignerRejectsPlainKey(t *testing.T) {
	signer, authLine, _ := generateEphemeralSigner()
	if _, err := buildCertSigner(signer, authLine); err == nil {
		t.Fatal("expected error for non-certificate credential")
	}
}
