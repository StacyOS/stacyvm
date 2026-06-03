package ssh

import (
	"context"
	"errors"
	"fmt"

	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/rs/zerolog"
)

// ErrPermissionDenied is returned when an identity may not access a sandbox.
var ErrPermissionDenied = errors.New("permission denied for sandbox")

type keyStore interface {
	GetSSHKeyByFingerprint(ctx context.Context, fingerprint string) (*store.SSHKeyRecord, error)
}

type sandboxLookup interface {
	GetSandbox(ctx context.Context, id string) (*store.SandboxRecord, error)
}

type ptyOpener interface {
	OpenPTYSession(ctx context.Context, sandboxID string, opts providers.PTYOptions) (providers.PTYSession, error)
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
}

func NewStoreBackend(keys keyStore, sandboxes sandboxLookup, pty ptyOpener, logger zerolog.Logger) *StoreBackend {
	return &StoreBackend{keys: keys, sandboxes: sandboxes, pty: pty, logger: logger}
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
	if !authorized(identity, sb) {
		b.logger.Warn().
			Str("subject", identity.Subject).
			Str("sandbox", sandboxID).
			Msg("ssh authorization denied")
		return nil, ErrPermissionDenied
	}
	return b.pty.OpenPTYSession(ctx, sandboxID, opts)
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
