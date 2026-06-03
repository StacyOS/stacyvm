//go:build unix

package providers

import (
	"context"
	"io"
	"strings"
	"testing"
)

// asPTYProvider fails the test if p does not implement the optional PTYProvider
// capability.
func asPTYProvider(t *testing.T, p Provider) PTYProvider {
	t.Helper()
	pp, ok := p.(PTYProvider)
	if !ok {
		t.Fatalf("provider %q does not implement PTYProvider", p.Name())
	}
	return pp
}

func TestMockProviderOpenPTYRunsCommandAndReportsExit(t *testing.T) {
	p := NewMockProvider()
	pp := asPTYProvider(t, p)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{Image: "mock"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	t.Cleanup(func() { _ = p.Destroy(context.Background(), id) })

	sess, err := pp.OpenPTY(ctx, id, PTYOptions{
		Cmd:  []string{"/bin/sh", "-c", "printf hello; exit 7"},
		Cols: 80,
		Rows: 24,
	})
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}

	out, err := io.ReadAll(sess)
	if err != nil {
		t.Fatalf("read pty output: %v", err)
	}
	if !strings.Contains(string(out), "hello") {
		t.Fatalf("pty output = %q, want it to contain %q", string(out), "hello")
	}

	code, err := sess.Wait()
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
}

func TestMockProviderOpenPTYHonorsInitialWindowSize(t *testing.T) {
	p := NewMockProvider()
	pp := asPTYProvider(t, p)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{Image: "mock"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	t.Cleanup(func() { _ = p.Destroy(context.Background(), id) })

	// `stty size` prints "<rows> <cols>" for the controlling terminal.
	sess, err := pp.OpenPTY(ctx, id, PTYOptions{
		Cmd:  []string{"/bin/sh", "-c", "stty size"},
		Cols: 120,
		Rows: 40,
	})
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}

	out, _ := io.ReadAll(sess)
	if _, err := sess.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !strings.Contains(string(out), "40 120") {
		t.Fatalf("stty size output = %q, want it to contain %q", string(out), "40 120")
	}
}

func TestDockerIntegrationOpenPTYRunsCommandAndReportsExit(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	pp := asPTYProvider(t, p)
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	sess, err := pp.OpenPTY(ctx, id, PTYOptions{
		Cmd:  []string{"/bin/sh", "-c", "printf hello; exit 7"},
		Cols: 80,
		Rows: 24,
	})
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}

	out, err := io.ReadAll(sess)
	if err != nil {
		t.Fatalf("read pty output: %v", err)
	}
	if !strings.Contains(string(out), "hello") {
		t.Fatalf("pty output = %q, want it to contain %q", string(out), "hello")
	}

	code, err := sess.Wait()
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
}

func TestDockerIntegrationOpenPTYHonorsInitialWindowSize(t *testing.T) {
	skipIfNoDocker(t)
	p, err := newTestDockerProvider(t)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	pp := asPTYProvider(t, p)
	id := spawnTestSandbox(t, p)
	ctx := context.Background()

	sess, err := pp.OpenPTY(ctx, id, PTYOptions{
		Cmd:  []string{"/bin/sh", "-c", "stty size"},
		Cols: 120,
		Rows: 40,
	})
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}

	out, _ := io.ReadAll(sess)
	if _, err := sess.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !strings.Contains(string(out), "40 120") {
		t.Fatalf("stty size output = %q, want it to contain %q", string(out), "40 120")
	}
}

func TestMockProviderOpenPTYResizeAndSignal(t *testing.T) {
	p := NewMockProvider()
	pp := asPTYProvider(t, p)
	ctx := context.Background()

	id, err := p.Spawn(ctx, SpawnOptions{Image: "mock"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	t.Cleanup(func() { _ = p.Destroy(context.Background(), id) })

	// A long-lived process so the session stays open across control ops.
	sess, err := pp.OpenPTY(ctx, id, PTYOptions{
		Cmd:  []string{"/bin/cat"},
		Cols: 80,
		Rows: 24,
	})
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}

	if err := sess.Resize(100, 30); err != nil {
		t.Fatalf("resize: %v", err)
	}

	// Terminate the process via signal and confirm the session unblocks.
	if err := sess.Signal("SIGTERM"); err != nil {
		t.Fatalf("signal: %v", err)
	}

	_, _ = io.Copy(io.Discard, sess)
	if _, err := sess.Wait(); err != nil {
		// A signal-terminated process surfaces as a non-nil wait error only when
		// we cannot determine an exit code; SIGTERM should yield a clean code.
		t.Fatalf("wait after signal: %v", err)
	}
}
