package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestSQLiteSSHKeyCRUD(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	key := &SSHKeyRecord{
		ID:          "key-1",
		OwnerID:     "user-alice",
		TenantID:    "tenant-acme",
		Fingerprint: "SHA256:abc123",
		PublicKey:   "ssh-ed25519 AAAAC3Nz... alice@host",
		Label:       "laptop",
	}
	if err := s.CreateSSHKey(ctx, key); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.GetSSHKeyByFingerprint(ctx, "SHA256:abc123")
	if err != nil {
		t.Fatalf("get by fingerprint: %v", err)
	}
	if got.ID != "key-1" || got.OwnerID != "user-alice" || got.TenantID != "tenant-acme" {
		t.Fatalf("got = %+v, want id=key-1 owner=user-alice tenant=tenant-acme", got)
	}
	if got.PublicKey != key.PublicKey || got.Label != "laptop" {
		t.Fatalf("got = %+v, want public key + label preserved", got)
	}

	list, err := s.ListSSHKeysByOwner(ctx, "user-alice")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Fingerprint != "SHA256:abc123" {
		t.Fatalf("list = %+v, want exactly one key", list)
	}

	if err := s.DeleteSSHKey(ctx, "key-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetSSHKeyByFingerprint(ctx, "SHA256:abc123"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete error = %v, want ErrNotFound", err)
	}
}

func TestSQLiteSSHKeyDuplicateFingerprintConflicts(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	first := &SSHKeyRecord{ID: "key-1", OwnerID: "u", Fingerprint: "SHA256:dup", PublicKey: "ssh-ed25519 A"}
	if err := s.CreateSSHKey(ctx, first); err != nil {
		t.Fatalf("create first: %v", err)
	}
	second := &SSHKeyRecord{ID: "key-2", OwnerID: "u", Fingerprint: "SHA256:dup", PublicKey: "ssh-ed25519 B"}
	if err := s.CreateSSHKey(ctx, second); !errors.Is(err, ErrConflict) {
		t.Fatalf("create duplicate fingerprint error = %v, want ErrConflict", err)
	}
}
