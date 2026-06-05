package ssh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/rs/zerolog"
)

const defaultKeepAliveInterval = time.Minute

// RenewFunc extends a sandbox's TTL so an active SSH session keeps it alive.
type RenewFunc func(ctx context.Context, sandboxID string) error

// ErrPermissionDenied is returned when an identity may not access a sandbox.
var ErrPermissionDenied = errors.New("permission denied for sandbox")

// ErrSSHDisabled is returned when a sandbox opts out of SSH via policy.
var ErrSSHDisabled = errors.New("ssh disabled for sandbox")

// ErrPortForwardDisabled is returned when a sandbox disables port forwarding.
var ErrPortForwardDisabled = errors.New("port forwarding disabled for sandbox")

// ErrPortForwardUnavailable is returned when no dialer is wired into the backend.
var ErrPortForwardUnavailable = errors.New("port forwarding not configured")

// ErrSFTPUnavailable is returned when the SFTP subsystem is not configured.
var ErrSFTPUnavailable = errors.New("sftp not configured")

type keyStore interface {
	GetSSHKeyByFingerprint(ctx context.Context, fingerprint string) (*store.SSHKeyRecord, error)
}

type sandboxLookup interface {
	GetSandbox(ctx context.Context, id string) (*store.SandboxRecord, error)
}

type ptyOpener interface {
	OpenPTYSession(ctx context.Context, sandboxID string, opts providers.PTYOptions) (providers.PTYSession, error)
}

// sandboxDialer dials a TCP address from inside a sandbox's network. The
// orchestrator Manager provides this for port forwarding (`ssh -L`).
type sandboxDialer interface {
	DialSandbox(ctx context.Context, sandboxID, addr string) (net.Conn, error)
}

// sandboxFileOps is the by-ID sandbox filesystem the orchestrator exposes for
// the SFTP subsystem. An adapter over *orchestrator.Manager satisfies it.
type sandboxFileOps interface {
	ReadFile(ctx context.Context, sandboxID, path string) ([]byte, error)
	WriteFile(ctx context.Context, sandboxID, path string, content []byte, mode string) error
	ListFiles(ctx context.Context, sandboxID, path string) ([]FileEntry, error)
	StatFile(ctx context.Context, sandboxID, path string) (FileEntry, error)
	RemoveFile(ctx context.Context, sandboxID, path string, recursive bool) error
	RenameFile(ctx context.Context, sandboxID, oldpath, newpath string) error
	MkdirFile(ctx context.Context, sandboxID, path string) error
}

// StoreBackend is the production Backend: it resolves keys from the store and
// attaches PTYs via the orchestrator, enforcing owner/tenant authorization.
// store.Store satisfies keyStore + sandboxLookup; *orchestrator.Manager
// satisfies ptyOpener.
type StoreBackend struct {
	keys      keyStore
	sandboxes sandboxLookup
	pty       ptyOpener
	logger    zerolog.Logger

	renew     RenewFunc
	keepAlive time.Duration
	recorder  RecorderFunc
	dialer    sandboxDialer
	files     sandboxFileOps
}

func NewStoreBackend(keys keyStore, sandboxes sandboxLookup, pty ptyOpener, logger zerolog.Logger) *StoreBackend {
	return &StoreBackend{keys: keys, sandboxes: sandboxes, pty: pty, logger: logger, keepAlive: defaultKeepAliveInterval}
}

// WithTTLRenewal makes active SSH sessions periodically extend the sandbox TTL
// via renew, so an in-use sandbox is not reaped. interval <= 0 keeps the default.
func (b *StoreBackend) WithTTLRenewal(renew RenewFunc, interval time.Duration) *StoreBackend {
	b.renew = renew
	if interval > 0 {
		b.keepAlive = interval
	}
	return b
}

// WithSessionRecording records each PTY session as an asciinema cast via rec.
func (b *StoreBackend) WithSessionRecording(rec RecorderFunc) *StoreBackend {
	b.recorder = rec
	return b
}

// WithPortForwarding enables SSH port forwarding (`ssh -L`) by routing
// authorized direct-tcpip channels through dialer.
func (b *StoreBackend) WithPortForwarding(dialer sandboxDialer) *StoreBackend {
	b.dialer = dialer
	return b
}

// WithSFTP enables the SFTP subsystem (sftp/scp/rsync) backed by files.
func (b *StoreBackend) WithSFTP(files sandboxFileOps) *StoreBackend {
	b.files = files
	return b
}

// SandboxFiles authorizes identity for sandboxID and returns a sandbox-scoped
// filesystem view for SFTP. It satisfies the gateway's FileProvider.
func (b *StoreBackend) SandboxFiles(ctx context.Context, identity Identity, sandboxID string) (SandboxFS, error) {
	if b.files == nil {
		return nil, ErrSFTPUnavailable
	}
	sb, err := b.sandboxes.GetSandbox(ctx, sandboxID)
	if err != nil {
		return nil, fmt.Errorf("sandbox %s: %w", sandboxID, err)
	}
	if sshDisabledByPolicy(sb) {
		return nil, ErrSSHDisabled
	}
	if !authorized(identity, sb) {
		b.logger.Warn().
			Str("subject", identity.Subject).
			Str("sandbox", sandboxID).
			Msg("ssh sftp authorization denied")
		return nil, ErrPermissionDenied
	}
	return &boundSandboxFS{files: b.files, sandboxID: sandboxID}, nil
}

// DialSandbox authorizes identity for sandboxID and, if policy permits, dials
// addr from inside the sandbox's network. It satisfies the gateway's Dialer.
func (b *StoreBackend) DialSandbox(ctx context.Context, identity Identity, sandboxID, addr string) (net.Conn, error) {
	if b.dialer == nil {
		return nil, ErrPortForwardUnavailable
	}
	sb, err := b.sandboxes.GetSandbox(ctx, sandboxID)
	if err != nil {
		return nil, fmt.Errorf("sandbox %s: %w", sandboxID, err)
	}
	if sshDisabledByPolicy(sb) {
		return nil, ErrSSHDisabled
	}
	if portForwardDisabledByPolicy(sb) {
		return nil, ErrPortForwardDisabled
	}
	if !authorized(identity, sb) {
		b.logger.Warn().
			Str("subject", identity.Subject).
			Str("sandbox", sandboxID).
			Str("addr", addr).
			Msg("ssh port-forward authorization denied")
		return nil, ErrPermissionDenied
	}
	return b.dialer.DialSandbox(ctx, sandboxID, addr)
}

func (b *StoreBackend) LookupKey(ctx context.Context, fingerprint string) (Identity, error) {
	rec, err := b.keys.GetSSHKeyByFingerprint(ctx, fingerprint)
	if err != nil {
		return Identity{}, err
	}
	return Identity{Subject: rec.OwnerID, OwnerID: rec.OwnerID, TenantID: rec.TenantID}, nil
}

func (b *StoreBackend) OpenPTY(ctx context.Context, identity Identity, sandboxID string, opts providers.PTYOptions) (providers.PTYSession, error) {
	sb, err := b.sandboxes.GetSandbox(ctx, sandboxID)
	if err != nil {
		return nil, fmt.Errorf("sandbox %s: %w", sandboxID, err)
	}
	if sshDisabledByPolicy(sb) {
		return nil, ErrSSHDisabled
	}
	if !authorized(identity, sb) {
		b.logger.Warn().
			Str("subject", identity.Subject).
			Str("sandbox", sandboxID).
			Msg("ssh authorization denied")
		return nil, ErrPermissionDenied
	}

	sess, err := b.pty.OpenPTYSession(ctx, sandboxID, opts)
	if err != nil {
		return nil, err
	}

	// Record the session as an asciinema cast, if configured.
	if b.recorder != nil {
		if wc, rerr := b.recorder(sandboxID, identity); rerr == nil && wc != nil {
			if cr, cerr := newCastRecorder(wc, opts.Cols, opts.Rows); cerr == nil {
				sess = &recordingSession{PTYSession: sess, rec: cr}
			} else {
				_ = wc.Close()
			}
		} else if wc != nil {
			_ = wc.Close()
		}
	}

	if b.renew == nil {
		return sess, nil
	}

	// Keep the sandbox alive while the session is open.
	kaCtx, cancel := context.WithCancel(context.Background())
	go b.runKeepAlive(kaCtx, sandboxID)
	return &keepAliveSession{PTYSession: sess, stop: cancel}, nil
}

func (b *StoreBackend) runKeepAlive(ctx context.Context, sandboxID string) {
	interval := b.keepAlive
	if interval <= 0 {
		interval = defaultKeepAliveInterval
	}
	if err := b.renew(ctx, sandboxID); err != nil {
		b.logger.Debug().Err(err).Str("sandbox", sandboxID).Msg("ssh ttl renewal failed")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := b.renew(ctx, sandboxID); err != nil {
				b.logger.Debug().Err(err).Str("sandbox", sandboxID).Msg("ssh ttl renewal failed")
			}
		}
	}
}

// keepAliveSession stops TTL renewal when the PTY session closes.
type keepAliveSession struct {
	providers.PTYSession
	stop func()
}

func (s *keepAliveSession) Close() error {
	s.stop()
	return s.PTYSession.Close()
}

// boundSandboxFS adapts the by-ID sandboxFileOps to the sandbox-scoped
// SandboxFS the SFTP handler expects, fixing the sandbox ID.
type boundSandboxFS struct {
	files     sandboxFileOps
	sandboxID string
}

func (f *boundSandboxFS) ReadFile(ctx context.Context, p string) ([]byte, error) {
	return f.files.ReadFile(ctx, f.sandboxID, p)
}
func (f *boundSandboxFS) WriteFile(ctx context.Context, p string, content []byte) error {
	return f.files.WriteFile(ctx, f.sandboxID, p, content, "0644")
}
func (f *boundSandboxFS) List(ctx context.Context, p string) ([]FileEntry, error) {
	return f.files.ListFiles(ctx, f.sandboxID, p)
}
func (f *boundSandboxFS) Stat(ctx context.Context, p string) (FileEntry, error) {
	return f.files.StatFile(ctx, f.sandboxID, p)
}
func (f *boundSandboxFS) Remove(ctx context.Context, p string) error {
	return f.files.RemoveFile(ctx, f.sandboxID, p, true)
}
func (f *boundSandboxFS) Rename(ctx context.Context, oldpath, newpath string) error {
	return f.files.RenameFile(ctx, f.sandboxID, oldpath, newpath)
}
func (f *boundSandboxFS) Mkdir(ctx context.Context, p string) error {
	return f.files.MkdirFile(ctx, f.sandboxID, p)
}

// sshDisabledByPolicy reports whether a sandbox has opted out of SSH via its
// metadata (e.g. {"ssh":"off"}).
func sshDisabledByPolicy(sb *store.SandboxRecord) bool {
	if strings.TrimSpace(sb.Metadata) == "" {
		return false
	}
	var md map[string]string
	if err := json.Unmarshal([]byte(sb.Metadata), &md); err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(md["ssh"])) {
	case "off", "disabled", "deny", "false":
		return true
	default:
		return false
	}
}

// portForwardDisabledByPolicy reports whether a sandbox disables port
// forwarding via its metadata (e.g. {"port_forward":"off"}).
func portForwardDisabledByPolicy(sb *store.SandboxRecord) bool {
	if strings.TrimSpace(sb.Metadata) == "" {
		return false
	}
	var md map[string]string
	if err := json.Unmarshal([]byte(sb.Metadata), &md); err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(md["port_forward"])) {
	case "off", "disabled", "deny", "false":
		return true
	default:
		return false
	}
}

// authorized allows access when the identity owns the sandbox, shares its
// tenant, or — in the single-user self-hosted case — when the sandbox records
// no owner/tenant at all.
func authorized(id Identity, sb *store.SandboxRecord) bool {
	switch {
	case sb.OwnerID != "" && sb.OwnerID == id.OwnerID:
		return true
	case sb.TenantID != "" && id.TenantID != "" && sb.TenantID == id.TenantID:
		return true
	case sb.OwnerID == "" && sb.TenantID == "":
		return true
	default:
		return false
	}
}
