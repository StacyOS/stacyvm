package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"nhooyr.io/websocket"
)

// sshConfigBlock describes a managed ~/.ssh/config Host block for a sandbox.
// In proxy mode the local ssh client reaches the gateway through a
// ProxyCommand that relays the SSH transport over the authenticated WebSocket;
// in direct mode it dials the native SSH port.
type sshConfigBlock struct {
	Alias    string // Host alias, e.g. "stacy-sb-123"
	Sandbox  string // SSH username = sandbox id
	ProxyCmd string // full ProxyCommand line (proxy mode)
	HostName string // gateway host (direct mode)
	Port     int    // gateway SSH port (direct mode)
	Identity string // optional IdentityFile path
}

func sshConfigMarkers(alias string) (begin, end string) {
	return "# >>> stacyvm managed: " + alias + " >>>",
		"# <<< stacyvm managed: " + alias + " <<<"
}

// renderSSHConfigBlock produces a marker-wrapped Host block so plain ssh, scp,
// rsync, and VS Code Remote-SSH can reach a sandbox via `ssh <alias>`.
func renderSSHConfigBlock(b sshConfigBlock) string {
	begin, end := sshConfigMarkers(b.Alias)
	var sb strings.Builder
	sb.WriteString(begin)
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "Host %s\n", b.Alias)
	if b.HostName != "" {
		fmt.Fprintf(&sb, "    HostName %s\n", b.HostName)
	}
	if b.Port != 0 {
		fmt.Fprintf(&sb, "    Port %d\n", b.Port)
	}
	fmt.Fprintf(&sb, "    User %s\n", b.Sandbox)
	if b.ProxyCmd != "" {
		fmt.Fprintf(&sb, "    ProxyCommand %s\n", b.ProxyCmd)
	}
	if b.Identity != "" {
		fmt.Fprintf(&sb, "    IdentityFile %s\n", b.Identity)
		// A managed cert/identity should not be mixed with the agent's keys.
		sb.WriteString("    IdentitiesOnly yes\n")
	}
	sb.WriteString("    StrictHostKeyChecking accept-new\n")
	sb.WriteString(end)
	return sb.String()
}

// upsertSSHConfigBlock inserts or replaces the managed block for alias within
// existing config text, leaving all other content untouched. Idempotent.
func upsertSSHConfigBlock(existing, alias, block string) string {
	begin, end := sshConfigMarkers(alias)
	startIdx := strings.Index(existing, begin)
	if startIdx >= 0 {
		endIdx := strings.Index(existing[startIdx:], end)
		if endIdx >= 0 {
			endIdx = startIdx + endIdx + len(end)
			return existing[:startIdx] + block + existing[endIdx:]
		}
	}
	trimmed := strings.TrimRight(existing, "\n")
	if trimmed == "" {
		return block + "\n"
	}
	return trimmed + "\n\n" + block + "\n"
}

// proxyRelay copies bytes bidirectionally between an established transport conn
// and the local stdio (in/out), returning once either direction closes. This is
// the body of the `stacy ssh proxy` ProxyCommand.
func proxyRelay(conn io.ReadWriteCloser, in io.Reader, out io.Writer) error {
	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(conn, in)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(out, conn)
		errc <- err
	}()
	err := <-errc
	_ = conn.Close()
	return err
}

type sshCertResponse struct {
	Certificate string `json:"certificate"`
	SandboxID   string `json:"sandbox_id"`
}

// requestSSHCert asks the control plane to mint a short-lived user certificate
// for publicKey bound to sandboxID, authenticated by the caller's API key/OIDC.
func requestSSHCert(c *httpClient, sandboxID, publicKey string, ttl time.Duration) (string, error) {
	body := map[string]any{
		"public_key":  publicKey,
		"sandbox_id":  sandboxID,
		"ttl_seconds": int(ttl.Seconds()),
	}
	resp, err := c.do("POST", "/api/v1/ssh/certs", body)
	if err != nil {
		return "", err
	}
	var out sshCertResponse
	if err := c.decodeJSON(resp, &out); err != nil {
		return "", err
	}
	if out.Certificate == "" {
		return "", fmt.Errorf("server returned an empty certificate")
	}
	return out.Certificate, nil
}

// buildCertSigner combines a private-key signer with an issued user certificate
// (authorized-keys line) into an ssh.Signer that presents the certificate.
func buildCertSigner(signer gossh.Signer, certLine string) (gossh.Signer, error) {
	pk, _, _, _, err := gossh.ParseAuthorizedKey([]byte(certLine))
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}
	cert, ok := pk.(*gossh.Certificate)
	if !ok {
		return nil, fmt.Errorf("issued credential is not an SSH certificate")
	}
	return gossh.NewCertSigner(cert, signer)
}

// wsURLFromServer derives the WebSocket URL for an SSH-over-WS endpoint from
// the configured HTTP server URL, mapping http→ws and https→wss.
func wsURLFromServer(serverURL, path string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http", "ws":
		u.Scheme = "ws"
	case "https", "wss":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported server scheme %q", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawQuery = ""
	return u.String(), nil
}

// dialWSTunnel opens the SSH-over-WebSocket tunnel at /api/v1/ssh/connect,
// authenticating with the API key, and returns it as a net.Conn carrying the
// raw SSH transport. The returned conn outlives the dial context.
func dialWSTunnel(ctx context.Context, serverURL, apiKey string) (net.Conn, error) {
	wsURL, err := wsURLFromServer(serverURL, "/api/v1/ssh/connect")
	if err != nil {
		return nil, err
	}
	hdr := http.Header{}
	if apiKey != "" {
		hdr.Set("X-API-Key", apiKey)
	}
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		return nil, err
	}
	return websocket.NetConn(context.Background(), c, websocket.MessageBinary), nil
}

// generateEphemeralSigner mints a throwaway ed25519 keypair for a single
// `stacy ssh` session, returning the signer and its authorized-keys line.
func generateEphemeralSigner() (gossh.Signer, string, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", err
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		return nil, "", err
	}
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		return nil, "", err
	}
	authLine := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(sshPub)))
	return signer, authLine, nil
}
