package main

import (
	"testing"

	"github.com/StacyOs/stacyvm/internal/orchestrator"
)

func TestFileEntryFromInfo(t *testing.T) {
	e := fileEntryFromInfo(orchestrator.FileInfo{
		Path:    "/work/app.go",
		Size:    123,
		Mode:    "644",
		IsDir:   false,
		ModTime: "2026-06-05T10:00:00Z",
	})
	if e.Name != "app.go" {
		t.Fatalf("Name = %q, want app.go", e.Name)
	}
	if e.Size != 123 {
		t.Fatalf("Size = %d, want 123", e.Size)
	}
	if e.Mode.Perm() != 0o644 {
		t.Fatalf("Mode.Perm() = %o, want 644", e.Mode.Perm())
	}
	if e.IsDir {
		t.Fatal("IsDir = true, want false")
	}
	if e.ModTime.IsZero() {
		t.Fatal("ModTime should be parsed, got zero")
	}
}

func TestFileEntryFromInfoDir(t *testing.T) {
	e := fileEntryFromInfo(orchestrator.FileInfo{Path: "/work/sub", Mode: "755", IsDir: true})
	if e.Name != "sub" || !e.IsDir {
		t.Fatalf("entry = %+v, want name=sub dir=true", e)
	}
	if e.Mode.Perm() != 0o755 {
		t.Fatalf("Mode.Perm() = %o, want 755", e.Mode.Perm())
	}
}
