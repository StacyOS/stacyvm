package ssh

import (
	"context"
	"testing"

	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/rs/zerolog"
)

type fakeKeyStore struct {
	rec *store.SSHKeyRecord
	err error
}

func (f fakeKeyStore) GetSSHKeyByFingerprint(context.Context, string) (*store.SSHKeyRecord, error) {
	return f.rec, f.err
}

type fakeSandboxLookup struct {
	rec *store.SandboxRecord
	err error
}

func (f fakeSandboxLookup) GetSandbox(context.Context, string) (*store.SandboxRecord, error) {
	return f.rec, f.err
}

type recordingOpener struct{ called bool }

func (r *recordingOpener) OpenPTYSession(context.Context, string, providers.PTYOptions) (providers.PTYSession, error) {
	r.called = true
	return newFakePTY("", 0), nil
}

func TestStoreBackendLookupKey(t *testing.T) {
	be := NewStoreBackend(
		fakeKeyStore{rec: &store.SSHKeyRecord{OwnerID: "alice", TenantID: "acme"}},
		fakeSandboxLookup{}, &recordingOpener{}, zerolog.Nop())

	id, err := be.LookupKey(context.Background(), "SHA256:x")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if id.OwnerID != "alice" || id.TenantID != "acme" || id.Subject != "alice" {
		t.Fatalf("identity = %+v, want owner/subject=alice tenant=acme", id)
	}
}

func TestStoreBackendLookupKeyUnknown(t *testing.T) {
	be := NewStoreBackend(fakeKeyStore{err: store.ErrNotFound}, fakeSandboxLookup{}, &recordingOpener{}, zerolog.Nop())
	if _, err := be.LookupKey(context.Background(), "SHA256:x"); err == nil {
		t.Fatal("want error for unknown key")
	}
}

func TestStoreBackendOpenPTYAuthorization(t *testing.T) {
	tests := []struct {
		name      string
		sandbox   *store.SandboxRecord
		identity  Identity
		wantAllow bool
	}{
		{"owner match", &store.SandboxRecord{ID: "sb1", OwnerID: "alice"}, Identity{OwnerID: "alice"}, true},
		{"tenant match", &store.SandboxRecord{ID: "sb1", OwnerID: "bob", TenantID: "acme"}, Identity{OwnerID: "alice", TenantID: "acme"}, true},
		{"ownerless self-host", &store.SandboxRecord{ID: "sb1"}, Identity{OwnerID: "alice"}, true},
		{"stranger denied", &store.SandboxRecord{ID: "sb1", OwnerID: "bob", TenantID: "other"}, Identity{OwnerID: "alice", TenantID: "acme"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opener := &recordingOpener{}
			be := NewStoreBackend(fakeKeyStore{}, fakeSandboxLookup{rec: tt.sandbox}, opener, zerolog.Nop())
			_, err := be.OpenPTY(context.Background(), tt.identity, "sb1", providers.PTYOptions{})
			if tt.wantAllow {
				if err != nil {
					t.Fatalf("want allowed, got error %v", err)
				}
				if !opener.called {
					t.Fatal("opener should have been called")
				}
			} else {
				if err == nil {
					t.Fatal("want permission denied")
				}
				if opener.called {
					t.Fatal("opener must not be called when denied")
				}
			}
		})
	}
}
