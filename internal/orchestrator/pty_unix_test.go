//go:build unix

package orchestrator

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/rs/zerolog"
)

func TestManagerOpenPTYSessionRunsCommand(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()

	sb, err := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	sess, err := m.OpenPTYSession(ctx, sb.ID, providers.PTYOptions{
		Cmd:  []string{"/bin/sh", "-c", "printf hi; exit 3"},
		Cols: 80,
		Rows: 24,
	})
	if err != nil {
		t.Fatalf("open pty session: %v", err)
	}

	out, err := io.ReadAll(sess)
	if err != nil {
		t.Fatalf("read pty output: %v", err)
	}
	if !strings.Contains(string(out), "hi") {
		t.Fatalf("pty output = %q, want it to contain %q", string(out), "hi")
	}

	code, err := sess.Wait()
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
}

// noPTYProvider wraps a Provider but deliberately does not implement
// PTYProvider (embedding the interface only promotes Provider's methods).
type noPTYProvider struct {
	providers.Provider
}

func TestManagerOpenPTYSessionUnsupportedProvider(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	reg := providers.NewRegistry()
	reg.Register(noPTYProvider{providers.NewMockProvider()})
	reg.SetDefault("mock")

	m := NewManager(reg, st, NewEventBus(), zerolog.Nop(), ManagerConfig{
		DefaultTTL:    5 * time.Minute,
		DefaultImage:  "alpine:latest",
		DefaultMemory: 512,
		DefaultVCPUs:  1,
	})
	m.Start()
	t.Cleanup(func() { m.Stop() })

	ctx := context.Background()
	sb, err := m.Spawn(ctx, SpawnRequest{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	_, err = m.OpenPTYSession(ctx, sb.ID, providers.PTYOptions{Cmd: []string{"/bin/sh"}})
	if !errors.Is(err, providers.ErrPTYUnsupported) {
		t.Fatalf("OpenPTYSession error = %v, want ErrPTYUnsupported", err)
	}
}
