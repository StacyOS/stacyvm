package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	gossh "golang.org/x/crypto/ssh"
)

// LoadOrCreateHostKey returns the SSH host key at path, generating and
// persisting a new ed25519 key (0600) on first use. A stable host key keeps
// clients' known_hosts valid across restarts and HA replicas.
func LoadOrCreateHostKey(path string) (gossh.Signer, error) {
	if data, err := os.ReadFile(path); err == nil {
		return gossh.ParsePrivateKey(data)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	block, err := gossh.MarshalPrivateKey(priv, "stacyvm-ssh-gateway")
	if err != nil {
		return nil, err
	}
	data := pem.EncodeToMemory(block)

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	return gossh.ParsePrivateKey(data)
}
