// Package ssh implements the StacyVM SSH gateway: a stateless protocol
// terminator that authenticates SSH clients and relays an interactive PTY to a
// sandbox through a Backend. It never executes user commands on the host — the
// real kernel PTY lives next to the shell process inside the sandbox.
package ssh

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"

	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/rs/zerolog"
	gossh "golang.org/x/crypto/ssh"
)

var errUnknownKey = errors.New("unknown ssh key")

// Identity is the resolved caller identity behind an SSH connection.
type Identity struct {
	Subject  string
	TenantID string
	OwnerID  string
	Scopes   []string
}

// Backend supplies authentication and PTY attachment to the SSH gateway.
type Backend interface {
	// LookupKey resolves an offered public-key SHA256 fingerprint to an identity.
	LookupKey(ctx context.Context, fingerprint string) (Identity, error)
	// OpenPTY authorizes identity for sandboxID and attaches an interactive PTY.
	OpenPTY(ctx context.Context, identity Identity, sandboxID string, opts providers.PTYOptions) (providers.PTYSession, error)
}

// Dialer is an optional Backend capability enabling SSH port forwarding
// (direct-tcpip / `ssh -L`). It dials addr ("host:port") from inside the
// sandbox's network after authorizing identity. Backends that do not implement
// it cause forwarding requests to be rejected cleanly.
type Dialer interface {
	DialSandbox(ctx context.Context, identity Identity, sandboxID, addr string) (net.Conn, error)
}

// ServerConfig holds tunable gateway options.
type ServerConfig struct {
	// Metrics, when set, records gateway counters for Prometheus.
	Metrics *Metrics
	// UserCA, when set, is the public key of the deployment's SSH user CA;
	// clients presenting a certificate signed by it are authenticated from the
	// certificate (the `stacy ssh` ephemeral-cert flow), bypassing the key store.
	UserCA gossh.PublicKey
}

// Server is the SSH gateway.
type Server struct {
	backend Backend
	signer  gossh.Signer
	logger  zerolog.Logger
	cfg     ServerConfig
	metrics *Metrics
	userCA  gossh.PublicKey
}

// NewServer builds a gateway that authenticates with hostKey and serves via backend.
func NewServer(backend Backend, hostKey gossh.Signer, logger zerolog.Logger, cfg ServerConfig) *Server {
	return &Server{backend: backend, signer: hostKey, logger: logger, cfg: cfg, metrics: cfg.Metrics, userCA: cfg.UserCA}
}

// Serve accepts connections until ln is closed.
func (s *Server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.HandleConn(context.Background(), conn)
	}
}

// HandleConn performs the SSH handshake on an established transport connection
// (a TCP conn, or a WebSocket-wrapped net.Conn) and serves its session channels.
func (s *Server) HandleConn(ctx context.Context, nc net.Conn) {
	defer nc.Close()

	var identity Identity
	cfg := &gossh.ServerConfig{
		PublicKeyCallback: func(meta gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			// CA-signed certificate path (ephemeral `stacy ssh` flow).
			if cert, ok := key.(*gossh.Certificate); ok {
				id, err := s.identityFromValidCert(meta.User(), cert)
				if err != nil {
					s.metrics.authFailure()
					return nil, err
				}
				identity = id
				return &gossh.Permissions{Extensions: map[string]string{"subject": id.Subject}}, nil
			}
			// Registered public key path.
			id, err := s.backend.LookupKey(ctx, gossh.FingerprintSHA256(key))
			if err != nil {
				s.metrics.authFailure()
				return nil, errUnknownKey
			}
			identity = id
			return &gossh.Permissions{Extensions: map[string]string{"subject": id.Subject}}, nil
		},
	}
	cfg.AddHostKey(s.signer)

	sshConn, chans, reqs, err := gossh.NewServerConn(nc, cfg)
	if err != nil {
		s.logger.Debug().Err(err).Msg("ssh handshake failed")
		return
	}
	defer sshConn.Close()
	go gossh.DiscardRequests(reqs)

	sandboxID, _ := parseSandboxTarget(sshConn.User())
	s.logger.Info().
		Str("subject", identity.Subject).
		Str("tenant", identity.TenantID).
		Str("sandbox", sandboxID).
		Str("remote", nc.RemoteAddr().String()).
		Msg("ssh connection authenticated")

	for newChan := range chans {
		switch newChan.ChannelType() {
		case "session":
			go s.handleSession(ctx, newChan, identity, sandboxID)
		case "direct-tcpip":
			go s.handleDirectTCPIP(ctx, newChan, identity, sandboxID)
		default:
			newChan.Reject(gossh.UnknownChannelType, "unsupported channel type")
		}
	}
}

// handleDirectTCPIP services an `ssh -L` port-forward channel by dialing the
// requested address from inside the sandbox (via the Backend's optional Dialer)
// and relaying bytes. Forwarding is rejected when the backend cannot dial.
func (s *Server) handleDirectTCPIP(ctx context.Context, newChan gossh.NewChannel, identity Identity, sandboxID string) {
	dialer, ok := s.backend.(Dialer)
	if !ok {
		newChan.Reject(gossh.Prohibited, "port forwarding not supported")
		return
	}
	host, port, ok := parseDirectTCPIP(newChan.ExtraData())
	if !ok {
		newChan.Reject(gossh.ConnectionFailed, "malformed direct-tcpip request")
		return
	}
	addr := net.JoinHostPort(host, strconv.Itoa(int(port)))
	conn, err := dialer.DialSandbox(ctx, identity, sandboxID, addr)
	if err != nil {
		s.logger.Debug().Err(err).Str("sandbox", sandboxID).Str("addr", addr).Msg("ssh forward dial failed")
		newChan.Reject(gossh.ConnectionFailed, err.Error())
		return
	}
	ch, reqs, err := newChan.Accept()
	if err != nil {
		_ = conn.Close()
		return
	}
	go gossh.DiscardRequests(reqs)
	s.relayTCP(ch, conn)
}

// relayTCP copies bytes bidirectionally between an SSH channel and a dialed
// connection until either side closes.
func (s *Server) relayTCP(ch gossh.Channel, conn net.Conn) {
	defer ch.Close()
	defer conn.Close()
	done := make(chan struct{}, 2)
	go func() {
		n, _ := io.Copy(conn, ch)
		s.metrics.addBytesIn(n)
		done <- struct{}{}
	}()
	go func() {
		n, _ := io.Copy(ch, conn)
		s.metrics.addBytesOut(n)
		done <- struct{}{}
	}()
	<-done
}

func parseDirectTCPIP(payload []byte) (host string, port uint32, ok bool) {
	var msg struct {
		DestHost string
		DestPort uint32
		OrigHost string
		OrigPort uint32
	}
	if err := gossh.Unmarshal(payload, &msg); err != nil {
		return "", 0, false
	}
	return msg.DestHost, msg.DestPort, true
}

// identityFromValidCert verifies a user certificate against the configured user
// CA for the given login (sandbox) and returns the identity it carries.
func (s *Server) identityFromValidCert(user string, cert *gossh.Certificate) (Identity, error) {
	if s.userCA == nil {
		return Identity{}, errUnknownKey
	}
	checker := &gossh.CertChecker{
		IsUserAuthority: func(auth gossh.PublicKey) bool {
			return bytes.Equal(auth.Marshal(), s.userCA.Marshal())
		},
	}
	if err := checker.CheckCert(user, cert); err != nil {
		return Identity{}, err
	}
	return identityFromCert(cert), nil
}

// handleSession negotiates a single session channel: it gathers the PTY request
// then, on shell/exec, attaches the sandbox PTY and relays bytes. The request
// loop keeps running during the session so window-change resizes apply live.
func (s *Server) handleSession(ctx context.Context, newChan gossh.NewChannel, identity Identity, sandboxID string) {
	ch, reqs, err := newChan.Accept()
	if err != nil {
		return
	}

	var (
		mu      sync.Mutex
		opts    providers.PTYOptions
		session providers.PTYSession
		started bool
	)

	startSession := func() bool {
		mu.Lock()
		if started {
			mu.Unlock()
			return false
		}
		startOpts := opts
		mu.Unlock()

		sess, err := s.backend.OpenPTY(ctx, identity, sandboxID, startOpts)
		if err != nil {
			io.WriteString(ch.Stderr(), "stacyvm: "+err.Error()+"\r\n")
			return false
		}

		mu.Lock()
		session = sess
		started = true
		mu.Unlock()

		go s.bridge(ch, sess)
		return true
	}

	for req := range reqs {
		switch req.Type {
		case "pty-req":
			term, cols, rows, ok := parsePtyReq(req.Payload)
			if ok {
				mu.Lock()
				opts.Term, opts.Cols, opts.Rows = term, cols, rows
				mu.Unlock()
			}
			req.Reply(ok, nil)
		case "window-change":
			cols, rows, ok := parseWindowChange(req.Payload)
			if ok {
				mu.Lock()
				if session != nil {
					_ = session.Resize(cols, rows)
				} else {
					opts.Cols, opts.Rows = cols, rows
				}
				mu.Unlock()
			}
			if req.WantReply {
				req.Reply(ok, nil)
			}
		case "shell":
			req.Reply(startSession(), nil)
		case "exec":
			if cmd, ok := parseExecCommand(req.Payload); ok && cmd != "" {
				mu.Lock()
				opts.Cmd = []string{"/bin/sh", "-c", cmd}
				mu.Unlock()
			}
			req.Reply(startSession(), nil)
		case "subsystem":
			if parseSubsystem(req.Payload) == "sftp" {
				req.Reply(s.startSFTP(ctx, ch, identity, sandboxID), nil)
			} else {
				req.Reply(false, nil)
			}
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

// bridge relays bytes between the SSH channel and the PTY session until the
// process exits or the connection drops, then reports the exit status.
func (s *Server) bridge(ch gossh.Channel, session providers.PTYSession) {
	s.metrics.sessionStarted()
	defer s.metrics.sessionEnded()
	defer ch.Close()
	defer session.Close()

	done := make(chan struct{})
	go func() {
		n, _ := io.Copy(session, ch) // client stdin -> pty
		s.metrics.addBytesIn(n)
	}()
	go func() {
		n, _ := io.Copy(ch, session) // pty output -> client
		s.metrics.addBytesOut(n)
		close(done)
	}()
	<-done

	code, _ := session.Wait()
	sendExitStatus(ch, code)
}

func parsePtyReq(payload []byte) (term string, cols, rows uint16, ok bool) {
	var msg struct {
		Term              string
		Columns, Rows     uint32
		WidthPx, HeightPx uint32
		Modes             string
	}
	if err := gossh.Unmarshal(payload, &msg); err != nil {
		return "", 0, 0, false
	}
	return msg.Term, uint16(msg.Columns), uint16(msg.Rows), true
}

func parseWindowChange(payload []byte) (cols, rows uint16, ok bool) {
	var msg struct {
		Columns, Rows     uint32
		WidthPx, HeightPx uint32
	}
	if err := gossh.Unmarshal(payload, &msg); err != nil {
		return 0, 0, false
	}
	return uint16(msg.Columns), uint16(msg.Rows), true
}

// startSFTP serves the SFTP subsystem on ch, backed by the sandbox filesystem
// when the Backend supports it. Returns false (rejecting the subsystem) when
// SFTP is unavailable or unauthorized.
func (s *Server) startSFTP(ctx context.Context, ch gossh.Channel, identity Identity, sandboxID string) bool {
	fp, ok := s.backend.(FileProvider)
	if !ok {
		return false
	}
	fs, err := fp.SandboxFiles(ctx, identity, sandboxID)
	if err != nil {
		io.WriteString(ch.Stderr(), "stacyvm: "+err.Error()+"\r\n")
		return false
	}
	go func() {
		s.metrics.sessionStarted()
		defer s.metrics.sessionEnded()
		_ = serveSFTP(ctx, ch, fs)
	}()
	return true
}

func parseSubsystem(payload []byte) string {
	var msg struct{ Name string }
	if err := gossh.Unmarshal(payload, &msg); err != nil {
		return ""
	}
	return msg.Name
}

func parseExecCommand(payload []byte) (string, bool) {
	var msg struct{ Command string }
	if err := gossh.Unmarshal(payload, &msg); err != nil {
		return "", false
	}
	return msg.Command, true
}

func sendExitStatus(ch gossh.Channel, code int) {
	var payload [4]byte
	binary.BigEndian.PutUint32(payload[:], uint32(code))
	_, _ = ch.SendRequest("exit-status", false, payload[:])
}
