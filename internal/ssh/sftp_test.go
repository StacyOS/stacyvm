package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"os"
	"path"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
)

// memFS is an in-memory SandboxFS for exercising the SFTP handler mapping.
type memFS struct {
	mu    sync.Mutex
	files map[string][]byte
	dirs  map[string]bool
}

func newMemFS() *memFS {
	return &memFS{files: map[string][]byte{}, dirs: map[string]bool{"/": true, "/work": true}}
}

func (m *memFS) ReadFile(_ context.Context, p string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[p]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), b...), nil
}

func (m *memFS) WriteFile(_ context.Context, p string, content []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[p] = append([]byte(nil), content...)
	return nil
}

func (m *memFS) List(_ context.Context, dir string) ([]FileEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []FileEntry
	for p, b := range m.files {
		if path.Dir(p) == dir {
			out = append(out, FileEntry{Name: path.Base(p), Size: int64(len(b)), Mode: 0o644})
		}
	}
	for d := range m.dirs {
		if d != dir && path.Dir(d) == dir {
			out = append(out, FileEntry{Name: path.Base(d), Mode: 0o755, IsDir: true})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *memFS) Stat(_ context.Context, p string) (FileEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.files[p]; ok {
		return FileEntry{Name: path.Base(p), Size: int64(len(b)), Mode: 0o644}, nil
	}
	if m.dirs[p] {
		return FileEntry{Name: path.Base(p), Mode: 0o755, IsDir: true}, nil
	}
	return FileEntry{}, os.ErrNotExist
}

func (m *memFS) Remove(_ context.Context, p string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, p)
	delete(m.dirs, p)
	return nil
}

func (m *memFS) Rename(_ context.Context, o, n string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.files[o]; ok {
		m.files[n] = b
		delete(m.files, o)
		return nil
	}
	return os.ErrNotExist
}

func (m *memFS) Mkdir(_ context.Context, p string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirs[p] = true
	return nil
}

func TestSFTPRoundTrip(t *testing.T) {
	fs := newMemFS()
	cConn, sConn := net.Pipe()
	go func() { _ = serveSFTP(context.Background(), sConn, fs) }()

	client, err := sftp.NewClientPipe(cConn, cConn)
	if err != nil {
		t.Fatalf("sftp client: %v", err)
	}
	defer client.Close()

	// Upload a file.
	f, err := client.Create("/work/hello.txt")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.Write([]byte("hi there")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Read it back.
	rf, err := client.Open("/work/hello.txt")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	data, err := io.ReadAll(rf)
	rf.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hi there" {
		t.Fatalf("content = %q, want %q", data, "hi there")
	}

	// Mkdir + list.
	if err := client.Mkdir("/work/sub"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	infos, err := client.ReadDir("/work")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	names := map[string]bool{}
	for _, fi := range infos {
		names[fi.Name()] = true
	}
	if !names["hello.txt"] || !names["sub"] {
		t.Fatalf("listing = %v, want hello.txt and sub", names)
	}

	// Rename + remove.
	if err := client.Rename("/work/hello.txt", "/work/bye.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := client.Stat("/work/bye.txt"); err != nil {
		t.Fatalf("stat after rename: %v", err)
	}
	if err := client.Remove("/work/bye.txt"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := client.Stat("/work/bye.txt"); err == nil {
		t.Fatal("expected stat to fail after remove")
	}
}

// fakeFileBackend adds the FileProvider capability to fakeBackend for testing
// the sftp subsystem end to end through the gateway.
type fakeFileBackend struct {
	fakeBackend
	fs *memFS
}

func (b *fakeFileBackend) SandboxFiles(_ context.Context, _ Identity, _ string) (SandboxFS, error) {
	return b.fs, nil
}

func TestServerSFTPSubsystem(t *testing.T) {
	fs := newMemFS()
	fs.files["/work/readme.md"] = []byte("welcome")

	_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	clientSigner, _ := gossh.NewSignerFromKey(clientPriv)
	fp := gossh.FingerprintSHA256(clientSigner.PublicKey())

	backend := &fakeFileBackend{
		fakeBackend: fakeBackend{allowedFP: fp, identity: Identity{OwnerID: "alice"}, pty: newFakePTY("", 0)},
		fs:          fs,
	}
	ln := startTestServer(t, backend)

	sshClient, err := gossh.Dial("tcp", ln.Addr().String(), &gossh.ClientConfig{
		User:            "sb-test",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(clientSigner)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer sshClient.Close()

	sc, err := sftp.NewClient(sshClient)
	if err != nil {
		t.Fatalf("sftp.NewClient: %v", err)
	}
	defer sc.Close()

	rf, err := sc.Open("/work/readme.md")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	data, _ := io.ReadAll(rf)
	rf.Close()
	if string(data) != "welcome" {
		t.Fatalf("content = %q, want welcome", data)
	}

	wf, err := sc.Create("/work/new.txt")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _ = wf.Write([]byte("uploaded"))
	wf.Close()
	if got := string(fs.files["/work/new.txt"]); got != "uploaded" {
		t.Fatalf("uploaded content = %q, want uploaded", got)
	}
}
