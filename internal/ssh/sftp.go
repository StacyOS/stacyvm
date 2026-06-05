package ssh

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"github.com/pkg/sftp"
)

// FileProvider is an optional Backend capability enabling the SFTP subsystem
// (sftp/scp/rsync, VS Code file sync). It authorizes identity and returns a
// filesystem view scoped to the sandbox; backends that do not implement it
// cause the sftp subsystem to be rejected.
type FileProvider interface {
	SandboxFiles(ctx context.Context, identity Identity, sandboxID string) (SandboxFS, error)
}

// FileEntry describes a single sandbox filesystem entry for SFTP listings.
type FileEntry struct {
	Name    string
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
	IsDir   bool
}

// SandboxFS is a sandbox-scoped filesystem view used by the SFTP subsystem.
// Implementations route to the sandbox's own filesystem; the host filesystem is
// never exposed.
type SandboxFS interface {
	ReadFile(ctx context.Context, p string) ([]byte, error)
	WriteFile(ctx context.Context, p string, content []byte) error
	List(ctx context.Context, p string) ([]FileEntry, error)
	Stat(ctx context.Context, p string) (FileEntry, error)
	Remove(ctx context.Context, p string) error
	Rename(ctx context.Context, oldpath, newpath string) error
	Mkdir(ctx context.Context, p string) error
}

// serveSFTP runs an SFTP RequestServer over rwc backed by fs until the client
// disconnects.
func serveSFTP(ctx context.Context, rwc io.ReadWriteCloser, fs SandboxFS) error {
	h := sftpHandler{ctx: ctx, fs: fs}
	server := sftp.NewRequestServer(rwc, sftp.Handlers{
		FileGet:  h,
		FilePut:  h,
		FileCmd:  h,
		FileList: h,
	})
	defer server.Close()
	if err := server.Serve(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

type sftpHandler struct {
	ctx context.Context
	fs  SandboxFS
}

func (h sftpHandler) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	data, err := h.fs.ReadFile(h.ctx, r.Filepath)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (h sftpHandler) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	return &sftpWriteBuffer{ctx: h.ctx, fs: h.fs, path: r.Filepath}, nil
}

func (h sftpHandler) Filecmd(r *sftp.Request) error {
	switch r.Method {
	case "Rename", "PosixRename":
		return h.fs.Rename(h.ctx, r.Filepath, r.Target)
	case "Rmdir", "Remove":
		return h.fs.Remove(h.ctx, r.Filepath)
	case "Mkdir":
		return h.fs.Mkdir(h.ctx, r.Filepath)
	case "Setstat":
		// Accept chmod/utimes/truncate metadata changes as no-ops so uploads
		// (which finish with FSETSTAT) succeed across the whole-file backend.
		return nil
	default:
		return sftp.ErrSSHFxOpUnsupported
	}
}

func (h sftpHandler) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	switch r.Method {
	case "List":
		entries, err := h.fs.List(h.ctx, r.Filepath)
		if err != nil {
			return nil, err
		}
		infos := make([]os.FileInfo, 0, len(entries))
		for _, e := range entries {
			infos = append(infos, fileInfoFromEntry(e))
		}
		return listerat(infos), nil
	case "Stat":
		e, err := h.fs.Stat(h.ctx, r.Filepath)
		if err != nil {
			return nil, err
		}
		return listerat([]os.FileInfo{fileInfoFromEntry(e)}), nil
	default:
		return nil, sftp.ErrSSHFxOpUnsupported
	}
}

// sftpWriteBuffer accumulates an SFTP upload in memory and flushes the whole
// file to the sandbox on close, matching the whole-file orchestrator API.
type sftpWriteBuffer struct {
	ctx  context.Context
	fs   SandboxFS
	path string

	mu      sync.Mutex
	buf     []byte
	aborted bool
}

func (w *sftpWriteBuffer) WriteAt(p []byte, off int64) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	end := int(off) + len(p)
	if end > len(w.buf) {
		grown := make([]byte, end)
		copy(grown, w.buf)
		w.buf = grown
	}
	copy(w.buf[off:], p)
	return len(p), nil
}

func (w *sftpWriteBuffer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.aborted {
		return nil
	}
	return w.fs.WriteFile(w.ctx, w.path, w.buf)
}

// TransferError is called by pkg/sftp if the transfer is aborted; discard the
// partial buffer so we do not flush a truncated file.
func (w *sftpWriteBuffer) TransferError(error) {
	w.mu.Lock()
	w.aborted = true
	w.mu.Unlock()
}

type listerat []os.FileInfo

func (l listerat) ListAt(ls []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l)) {
		return 0, io.EOF
	}
	n := copy(ls, l[offset:])
	if n < len(ls) {
		return n, io.EOF
	}
	return n, nil
}

type sftpFileInfo struct {
	name string
	size int64
	mode os.FileMode
	mod  time.Time
	dir  bool
}

func (fi sftpFileInfo) Name() string { return fi.name }
func (fi sftpFileInfo) Size() int64  { return fi.size }
func (fi sftpFileInfo) Mode() os.FileMode {
	if fi.dir {
		return fi.mode | os.ModeDir
	}
	return fi.mode
}
func (fi sftpFileInfo) ModTime() time.Time { return fi.mod }
func (fi sftpFileInfo) IsDir() bool        { return fi.dir }
func (fi sftpFileInfo) Sys() any           { return nil }

func fileInfoFromEntry(e FileEntry) os.FileInfo {
	return sftpFileInfo{name: e.Name, size: e.Size, mode: e.Mode, mod: e.ModTime, dir: e.IsDir}
}
