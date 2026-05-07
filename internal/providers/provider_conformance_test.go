package providers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type providerFactory func(t *testing.T) Provider

func runProviderConformance(t *testing.T, factory providerFactory) {
	t.Helper()

	t.Run("spawn status and destroy", func(t *testing.T) {
		p := factory(t)
		ctx := context.Background()

		id, err := p.Spawn(ctx, SpawnOptions{Image: "alpine:latest"})
		if err != nil {
			t.Fatalf("spawn: %v", err)
		}
		if id == "" {
			t.Fatal("spawn returned an empty sandbox ID")
		}

		status, err := p.Status(ctx, id)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if status.ID != id {
			t.Fatalf("status ID = %q, want %q", status.ID, id)
		}
		if status.State != "running" {
			t.Fatalf("status state = %q, want running", status.State)
		}

		if err := p.Destroy(ctx, id); err != nil {
			t.Fatalf("destroy: %v", err)
		}

		_, err = p.Exec(ctx, id, ExecOptions{Command: "echo after destroy"})
		if !errors.Is(err, ErrSandboxDestroyed) && !errors.Is(err, ErrSandboxNotFound) {
			t.Fatalf("exec after destroy error = %v, want sandbox lifecycle error", err)
		}
	})

	t.Run("exec captures success and nonzero exit", func(t *testing.T) {
		p := factory(t)
		ctx := context.Background()
		id := spawnConformanceSandbox(t, p)
		t.Cleanup(func() { _ = p.Destroy(context.Background(), id) })

		result, err := p.Exec(ctx, id, ExecOptions{Command: "echo conformance-ok"})
		if err != nil {
			t.Fatalf("exec success: %v", err)
		}
		if result.ExitCode != 0 {
			t.Fatalf("exit code = %d, want 0", result.ExitCode)
		}
		if !strings.Contains(result.Stdout, "conformance-ok") {
			t.Fatalf("stdout = %q, want conformance-ok", result.Stdout)
		}

		result, err = p.Exec(ctx, id, ExecOptions{Command: "exit 7"})
		if err != nil {
			t.Fatalf("exec nonzero: %v", err)
		}
		if result.ExitCode != 7 {
			t.Fatalf("exit code = %d, want 7", result.ExitCode)
		}
	})

	t.Run("exec stream emits output", func(t *testing.T) {
		p := factory(t)
		ctx := context.Background()
		id := spawnConformanceSandbox(t, p)
		t.Cleanup(func() { _ = p.Destroy(context.Background(), id) })

		ch, err := p.ExecStream(ctx, id, ExecOptions{Command: "printf stream-ok"})
		if err != nil {
			t.Fatalf("exec stream: %v", err)
		}
		var out strings.Builder
		for chunk := range ch {
			out.WriteString(chunk.Data)
		}
		if !strings.Contains(out.String(), "stream-ok") {
			t.Fatalf("stream output = %q, want stream-ok", out.String())
		}
	})

	t.Run("file operations round trip", func(t *testing.T) {
		p := factory(t)
		ctx := context.Background()
		id := spawnConformanceSandbox(t, p)
		t.Cleanup(func() { _ = p.Destroy(context.Background(), id) })

		const originalPath = "/workspace/contract.txt"
		const movedPath = "/workspace/contract-moved.txt"
		const content = "provider contract file"

		if err := p.WriteFile(ctx, id, originalPath, bytes.NewReader([]byte(content)), "0644"); err != nil {
			t.Fatalf("write file: %v", err)
		}

		rc, err := p.ReadFile(ctx, id, originalPath)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read content: %v", err)
		}
		if string(data) != content {
			t.Fatalf("content = %q, want %q", string(data), content)
		}

		if _, err := p.StatFile(ctx, id, originalPath); err != nil {
			t.Fatalf("stat file: %v", err)
		}

		matches, err := p.GlobFiles(ctx, id, "/workspace/contract*.txt")
		if err != nil {
			t.Fatalf("glob files: %v", err)
		}
		if len(matches) == 0 {
			t.Fatal("glob files returned no matches")
		}

		if err := p.MoveFile(ctx, id, originalPath, movedPath); err != nil {
			t.Fatalf("move file: %v", err)
		}
		if err := p.ChmodFile(ctx, id, movedPath, "0755"); err != nil {
			t.Fatalf("chmod file: %v", err)
		}
		if err := p.DeleteFile(ctx, id, movedPath, false); err != nil {
			t.Fatalf("delete file: %v", err)
		}
	})
}

func spawnConformanceSandbox(t *testing.T, p Provider) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	id, err := p.Spawn(ctx, SpawnOptions{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	return id
}
