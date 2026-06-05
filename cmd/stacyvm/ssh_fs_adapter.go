package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/StacyOs/stacyvm/internal/orchestrator"
	stacyssh "github.com/StacyOs/stacyvm/internal/ssh"
)

// managerFS adapts *orchestrator.Manager to the by-ID sandbox filesystem the
// SSH gateway's SFTP subsystem consumes. It maps each SFTP operation onto an
// existing orchestrator file operation so SFTP works on every provider.
type managerFS struct {
	mgr *orchestrator.Manager
}

func (a managerFS) ReadFile(ctx context.Context, sandboxID, path string) ([]byte, error) {
	return a.mgr.ReadFile(ctx, sandboxID, path)
}

func (a managerFS) WriteFile(ctx context.Context, sandboxID, path string, content []byte, mode string) error {
	return a.mgr.WriteFile(ctx, sandboxID, orchestrator.FileWriteRequest{
		Path:    path,
		Content: string(content),
		Mode:    mode,
	})
}

func (a managerFS) ListFiles(ctx context.Context, sandboxID, path string) ([]stacyssh.FileEntry, error) {
	infos, err := a.mgr.ListFiles(ctx, sandboxID, path)
	if err != nil {
		return nil, err
	}
	out := make([]stacyssh.FileEntry, 0, len(infos))
	for _, fi := range infos {
		out = append(out, fileEntryFromInfo(fi))
	}
	return out, nil
}

func (a managerFS) StatFile(ctx context.Context, sandboxID, path string) (stacyssh.FileEntry, error) {
	fi, err := a.mgr.StatFile(ctx, sandboxID, path)
	if err != nil {
		return stacyssh.FileEntry{}, err
	}
	return fileEntryFromInfo(*fi), nil
}

func (a managerFS) RemoveFile(ctx context.Context, sandboxID, path string, recursive bool) error {
	return a.mgr.DeleteFile(ctx, sandboxID, orchestrator.FileDeleteRequest{Path: path, Recursive: recursive})
}

func (a managerFS) RenameFile(ctx context.Context, sandboxID, oldpath, newpath string) error {
	return a.mgr.MoveFile(ctx, sandboxID, orchestrator.FileMoveRequest{OldPath: oldpath, NewPath: newpath})
}

func (a managerFS) MkdirFile(ctx context.Context, sandboxID, path string) error {
	_, err := a.mgr.Exec(ctx, sandboxID, orchestrator.ExecRequest{
		Mode:    "shell",
		Command: "mkdir -p " + shellQuoteArg(path),
	})
	return err
}

// fileEntryFromInfo converts an orchestrator FileInfo (octal mode string,
// RFC3339 "Z" mod time, full path) to the gateway's FileEntry.
func fileEntryFromInfo(fi orchestrator.FileInfo) stacyssh.FileEntry {
	mode := os.FileMode(0o644)
	if m, err := strconv.ParseUint(strings.TrimSpace(fi.Mode), 8, 32); err == nil {
		mode = os.FileMode(m)
	} else if fi.IsDir {
		mode = 0o755
	}
	modTime, _ := time.Parse("2006-01-02T15:04:05Z", fi.ModTime)
	return stacyssh.FileEntry{
		Name:    filepath.Base(fi.Path),
		Size:    fi.Size,
		Mode:    mode,
		ModTime: modTime,
		IsDir:   fi.IsDir,
	}
}

// shellQuoteArg single-quotes a path for safe shell interpolation.
func shellQuoteArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
