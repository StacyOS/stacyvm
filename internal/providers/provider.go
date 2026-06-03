package providers

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	ExecModeShell = "shell"
	ExecModeArgv  = "argv"
)

type SpawnOptions struct {
	Image      string
	MemoryMB   int
	VCPUs      int
	DiskSizeMB int
	Metadata   map[string]string
}

type ExecOptions struct {
	Command string
	Args    []string
	Mode    string
	Env     map[string]string
	WorkDir string
}

type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type StreamChunk struct {
	Stream string `json:"stream"` // "stdout" or "stderr"
	Data   string `json:"data"`
}

type FileInfo struct {
	Path    string
	Size    int64
	Mode    string
	IsDir   bool
	ModTime string
}

type SandboxStatus struct {
	ID    string
	State string
}

// RuntimeSandbox describes a provider-owned runtime discovered outside the
// orchestrator's in-memory state, usually during startup reconciliation.
type RuntimeSandbox struct {
	ID        string
	State     string
	Provider  string
	Image     string
	CreatedAt time.Time
	Metadata  map[string]string
}

// Provider defines the interface for sandbox execution backends.
//
// Implementations must satisfy the behavior documented in
// docs/provider-contract.md and exercised by provider_conformance_test.go.
type Provider interface {
	// Name returns the unique provider identifier.
	Name() string

	// Spawn creates a new sandbox and returns its ID.
	Spawn(ctx context.Context, opts SpawnOptions) (string, error)

	// Exec runs a command in the sandbox and returns the result.
	Exec(ctx context.Context, sandboxID string, opts ExecOptions) (*ExecResult, error)

	// ExecStream runs a command and streams output chunks.
	ExecStream(ctx context.Context, sandboxID string, opts ExecOptions) (<-chan StreamChunk, error)

	// WriteFile writes content to a file in the sandbox.
	WriteFile(ctx context.Context, sandboxID string, path string, content io.Reader, mode string) error

	// ReadFile reads a file from the sandbox.
	ReadFile(ctx context.Context, sandboxID string, path string) (io.ReadCloser, error)

	// ListFiles lists files at the given path in the sandbox.
	ListFiles(ctx context.Context, sandboxID string, path string) ([]FileInfo, error)

	// DeleteFile deletes a file or directory in the sandbox.
	DeleteFile(ctx context.Context, sandboxID string, path string, recursive bool) error

	// MoveFile moves/renames a file in the sandbox.
	MoveFile(ctx context.Context, sandboxID string, oldPath, newPath string) error

	// ChmodFile changes file permissions in the sandbox.
	ChmodFile(ctx context.Context, sandboxID string, path string, mode string) error

	// StatFile returns info about a single file in the sandbox.
	StatFile(ctx context.Context, sandboxID string, path string) (*FileInfo, error)

	// GlobFiles returns paths matching a glob pattern in the sandbox.
	GlobFiles(ctx context.Context, sandboxID string, pattern string) ([]string, error)

	// Status returns the current status of a sandbox.
	Status(ctx context.Context, sandboxID string) (*SandboxStatus, error)

	// Destroy tears down a sandbox.
	Destroy(ctx context.Context, sandboxID string) error

	// ConsoleLog returns the last n lines of the sandbox's console output.
	ConsoleLog(ctx context.Context, sandboxID string, lines int) ([]string, error)

	// Healthy returns true if the provider is operational.
	Healthy(ctx context.Context) bool
}

// PTYOptions configures an interactive pseudo-terminal session.
type PTYOptions struct {
	Cmd     []string          // command + args; empty means the sandbox's default shell
	Env     map[string]string // extra environment variables
	WorkDir string            // working directory inside the sandbox
	Term    string            // TERM value, e.g. "xterm-256color"
	Cols    uint16            // initial terminal width in columns
	Rows    uint16            // initial terminal height in rows
}

// PTYSession is a live interactive terminal attached to a process inside a
// sandbox. Reads return terminal output; writes deliver keystrokes/stdin to the
// process. The byte stream is binary-safe end to end.
type PTYSession interface {
	io.ReadWriteCloser
	// Resize updates the terminal window size.
	Resize(cols, rows uint16) error
	// Signal delivers a signal (e.g. "SIGINT") to the foreground process.
	Signal(sig string) error
	// Wait blocks until the attached process exits and returns its exit code.
	Wait() (exitCode int, err error)
}

// PTYProvider is an optional capability for providers that can attach an
// interactive PTY to a process inside a sandbox; the SSH gateway depends on it.
// Providers that cannot allocate a PTY simply do not implement it, and callers
// surface ErrPTYUnsupported.
type PTYProvider interface {
	OpenPTY(ctx context.Context, sandboxID string, opts PTYOptions) (PTYSession, error)
}

// SnapshotSummary describes a pre-built VM snapshot available for fast restore.
type SnapshotSummary struct {
	Image     string    `json:"image"`
	Provider  string    `json:"provider"`
	CreatedAt time.Time `json:"created_at"`
}

// SnapshotLister is an optional interface that providers can implement
// to expose their cached snapshots.
type SnapshotLister interface {
	ListSnapshots() []SnapshotSummary
}

// RuntimeSandboxLister is implemented by providers that can enumerate
// already-running runtimes for startup reconciliation.
type RuntimeSandboxLister interface {
	ListRuntimeSandboxes(ctx context.Context) ([]RuntimeSandbox, error)
}

// SandboxStats holds live resource usage for a single sandbox.
type SandboxStats struct {
	CPUPercent       float64
	MemoryBytes      uint64
	MemoryLimitBytes uint64
}

// StatsReporter is an optional interface for providers that can report live
// per-sandbox resource usage (CPU%, memory). Providers that cannot (proot,
// firecracker today) simply don't implement it; callers fall back to "—".
type StatsReporter interface {
	Stats(ctx context.Context, sandboxID string) (*SandboxStats, error)
}

func normalizeExecMode(mode string) (string, error) {
	switch strings.TrimSpace(mode) {
	case "", ExecModeShell:
		return ExecModeShell, nil
	case ExecModeArgv:
		return ExecModeArgv, nil
	default:
		return "", fmt.Errorf("unsupported exec mode %q", mode)
	}
}

func shellQuoteArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func buildExecCommand(opts ExecOptions) ([]string, error) {
	mode, err := normalizeExecMode(opts.Mode)
	if err != nil {
		return nil, err
	}
	if mode == ExecModeArgv {
		if strings.TrimSpace(opts.Command) == "" {
			return nil, fmt.Errorf("argv exec mode requires command")
		}
		return append([]string{opts.Command}, opts.Args...), nil
	}

	shellCmd := opts.Command
	for _, arg := range opts.Args {
		shellCmd += " " + shellQuoteArg(arg)
	}
	return []string{"sh", "-c", shellCmd}, nil
}
