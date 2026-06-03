package ssh

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadOrCreateHostKeyGeneratesAndPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "ssh_host_ed25519_key")

	s1, err := LoadOrCreateHostKey(path)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if s1 == nil {
		t.Fatal("signer is nil")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode = %o, want 0600", info.Mode().Perm())
	}

	// Second load must return the same identity (stable host key across restarts).
	s2, err := LoadOrCreateHostKey(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if !bytes.Equal(s1.PublicKey().Marshal(), s2.PublicKey().Marshal()) {
		t.Fatal("host key changed across loads; known_hosts would break")
	}
}
