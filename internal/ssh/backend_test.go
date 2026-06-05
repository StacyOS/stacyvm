package ssh

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

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

func TestStoreBackendRespectsSandboxSSHPolicy(t *testing.T) {
	opener := &recordingOpener{}
	be := NewStoreBackend(
		fakeKeyStore{},
		fakeSandboxLookup{rec: &store.SandboxRecord{ID: "sb1", OwnerID: "alice", Metadata: `{"ssh":"off"}`}},
		opener, zerolog.Nop(),
	)
	_, err := be.OpenPTY(context.Background(), Identity{OwnerID: "alice"}, "sb1", providers.PTYOptions{})
	if err == nil {
		t.Fatal("expected SSH to be denied by sandbox policy")
	}
	if opener.called {
		t.Fatal("opener must not be called when SSH is disabled by policy")
	}
}

func TestStoreBackendRenewsTTLWhileSessionOpen(t *testing.T) {
	var calls int32
	renew := func(context.Context, string) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}
	be := NewStoreBackend(
		fakeKeyStore{},
		fakeSandboxLookup{rec: &store.SandboxRecord{ID: "sb1", OwnerID: "alice"}},
		&recordingOpener{}, zerolog.Nop(),
	).WithTTLRenewal(renew, 5*time.Millisecond)

	sess, err := be.OpenPTY(context.Background(), Identity{OwnerID: "alice"}, "sb1", providers.PTYOptions{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	if atomic.LoadInt32(&calls) < 1 {
		t.Fatal("expected at least one TTL renewal while session open")
	}

	_ = sess.Close()
	settled := atomic.LoadInt32(&calls)
	time.Sleep(30 * time.Millisecond)
	if grew := atomic.LoadInt32(&calls) - settled; grew > 1 {
		t.Fatalf("renewals continued after close (grew by %d)", grew)
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

type fakeSandboxDialer struct {
	called  bool
	gotAddr string
}

func (f *fakeSandboxDialer) DialSandbox(_ context.Context, _ string, addr string) (net.Conn, error) {
	f.called = true
	f.gotAddr = addr
	c, _ := net.Pipe()
	return c, nil
}

func TestStoreBackendDialSandboxAuthorized(t *testing.T) {
	d := &fakeSandboxDialer{}
	be := NewStoreBackend(
		fakeKeyStore{},
		fakeSandboxLookup{rec: &store.SandboxRecord{ID: "sb1", OwnerID: "alice"}},
		&recordingOpener{}, zerolog.Nop(),
	).WithPortForwarding(d)

	conn, err := be.DialSandbox(context.Background(), Identity{OwnerID: "alice"}, "sb1", "10.0.0.5:5432")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()
	if !d.called || d.gotAddr != "10.0.0.5:5432" {
		t.Fatalf("dialer called=%v addr=%q, want true 10.0.0.5:5432", d.called, d.gotAddr)
	}
}

func TestStoreBackendDialSandboxDeniedForStranger(t *testing.T) {
	d := &fakeSandboxDialer{}
	be := NewStoreBackend(
		fakeKeyStore{},
		fakeSandboxLookup{rec: &store.SandboxRecord{ID: "sb1", OwnerID: "bob", TenantID: "other"}},
		&recordingOpener{}, zerolog.Nop(),
	).WithPortForwarding(d)

	if _, err := be.DialSandbox(context.Background(), Identity{OwnerID: "alice", TenantID: "acme"}, "sb1", "x:1"); err == nil {
		t.Fatal("expected permission denied")
	}
	if d.called {
		t.Fatal("dialer must not be called when denied")
	}
}

func TestStoreBackendDialSandboxPolicyDisabled(t *testing.T) {
	d := &fakeSandboxDialer{}
	be := NewStoreBackend(
		fakeKeyStore{},
		fakeSandboxLookup{rec: &store.SandboxRecord{ID: "sb1", OwnerID: "alice", Metadata: `{"port_forward":"off"}`}},
		&recordingOpener{}, zerolog.Nop(),
	).WithPortForwarding(d)

	if _, err := be.DialSandbox(context.Background(), Identity{OwnerID: "alice"}, "sb1", "x:1"); err == nil {
		t.Fatal("expected port forwarding denied by policy")
	}
	if d.called {
		t.Fatal("dialer must not be called when policy denies forwarding")
	}
}

func TestStoreBackendDialSandboxNoDialerConfigured(t *testing.T) {
	be := NewStoreBackend(
		fakeKeyStore{},
		fakeSandboxLookup{rec: &store.SandboxRecord{ID: "sb1", OwnerID: "alice"}},
		&recordingOpener{}, zerolog.Nop(),
	)
	if _, err := be.DialSandbox(context.Background(), Identity{OwnerID: "alice"}, "sb1", "x:1"); err == nil {
		t.Fatal("expected error when no dialer is configured")
	}
}

type fakeFileOps struct{ reads int }

func (f *fakeFileOps) ReadFile(context.Context, string, string) ([]byte, error) {
	f.reads++
	return []byte("x"), nil
}
func (f *fakeFileOps) WriteFile(context.Context, string, string, []byte, string) error { return nil }
func (f *fakeFileOps) ListFiles(context.Context, string, string) ([]FileEntry, error) {
	return nil, nil
}
func (f *fakeFileOps) StatFile(context.Context, string, string) (FileEntry, error) {
	return FileEntry{}, nil
}
func (f *fakeFileOps) RemoveFile(context.Context, string, string, bool) error   { return nil }
func (f *fakeFileOps) RenameFile(context.Context, string, string, string) error { return nil }
func (f *fakeFileOps) MkdirFile(context.Context, string, string) error          { return nil }

func TestStoreBackendSandboxFilesAuthorized(t *testing.T) {
	ops := &fakeFileOps{}
	be := NewStoreBackend(
		fakeKeyStore{},
		fakeSandboxLookup{rec: &store.SandboxRecord{ID: "sb1", OwnerID: "alice"}},
		&recordingOpener{}, zerolog.Nop(),
	).WithSFTP(ops)

	fs, err := be.SandboxFiles(context.Background(), Identity{OwnerID: "alice"}, "sb1")
	if err != nil {
		t.Fatalf("SandboxFiles: %v", err)
	}
	if _, err := fs.ReadFile(context.Background(), "/work/x"); err != nil {
		t.Fatalf("read through bound fs: %v", err)
	}
	if ops.reads != 1 {
		t.Fatalf("expected delegated read, got %d", ops.reads)
	}
}

func TestStoreBackendSandboxFilesDeniedForStranger(t *testing.T) {
	be := NewStoreBackend(
		fakeKeyStore{},
		fakeSandboxLookup{rec: &store.SandboxRecord{ID: "sb1", OwnerID: "bob", TenantID: "other"}},
		&recordingOpener{}, zerolog.Nop(),
	).WithSFTP(&fakeFileOps{})

	if _, err := be.SandboxFiles(context.Background(), Identity{OwnerID: "alice", TenantID: "acme"}, "sb1"); err == nil {
		t.Fatal("expected permission denied")
	}
}

func TestStoreBackendSandboxFilesNotConfigured(t *testing.T) {
	be := NewStoreBackend(
		fakeKeyStore{},
		fakeSandboxLookup{rec: &store.SandboxRecord{ID: "sb1", OwnerID: "alice"}},
		&recordingOpener{}, zerolog.Nop(),
	)
	if _, err := be.SandboxFiles(context.Background(), Identity{OwnerID: "alice"}, "sb1"); err == nil {
		t.Fatal("expected error when sftp not configured")
	}
}
