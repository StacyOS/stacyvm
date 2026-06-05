//go:build unix

package providers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/creack/pty"
)

// hostPTYSession bridges a PTYSession to a process running under a host pty.
// It is shared by providers that execute the sandbox process on the host
// (mock today, proot later).
type hostPTYSession struct {
	f   *os.File
	cmd *exec.Cmd
}

func (s *hostPTYSession) Read(p []byte) (int, error) {
	n, err := s.f.Read(p)
	if err != nil && ptyReadIsEOF(err) {
		return n, io.EOF
	}
	return n, err
}

func (s *hostPTYSession) Write(p []byte) (int, error) { return s.f.Write(p) }

func (s *hostPTYSession) Close() error { return s.f.Close() }

func (s *hostPTYSession) Resize(cols, rows uint16) error {
	return pty.Setsize(s.f, &pty.Winsize{Rows: rows, Cols: cols})
}

func (s *hostPTYSession) Signal(sig string) error {
	sg, ok := signalByName(sig)
	if !ok {
		return fmt.Errorf("unknown signal %q", sig)
	}
	if s.cmd.Process == nil {
		return errors.New("process not started")
	}
	return s.cmd.Process.Signal(sg)
}

func (s *hostPTYSession) Wait() (int, error) {
	err := s.cmd.Wait()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return -1, err
}

// startHostPTY starts cmd attached to a freshly allocated pty with the given
// initial window size (defaulting to 80x24 when unset).
func startHostPTY(cmd *exec.Cmd, cols, rows uint16) (*hostPTYSession, error) {
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		return nil, err
	}
	return &hostPTYSession{f: f, cmd: cmd}, nil
}

// ptyReadIsEOF reports whether a pty-master read error means the session ended.
// After the child exits, reading the master returns EIO on Linux.
func ptyReadIsEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) || errors.Is(err, os.ErrClosed)
}

func signalByName(name string) (os.Signal, bool) {
	switch name {
	case "SIGINT", "INT":
		return syscall.SIGINT, true
	case "SIGTERM", "TERM":
		return syscall.SIGTERM, true
	case "SIGKILL", "KILL":
		return syscall.SIGKILL, true
	case "SIGHUP", "HUP":
		return syscall.SIGHUP, true
	case "SIGQUIT", "QUIT":
		return syscall.SIGQUIT, true
	case "SIGWINCH", "WINCH":
		return syscall.SIGWINCH, true
	case "SIGUSR1", "USR1":
		return syscall.SIGUSR1, true
	case "SIGUSR2", "USR2":
		return syscall.SIGUSR2, true
	default:
		return nil, false
	}
}

// ptyExecOptions maps interactive PTYOptions onto ExecOptions in argv mode
// (an interactive shell must exec the program directly, not via `sh -c`).
func ptyExecOptions(opts PTYOptions) ExecOptions {
	cmdline := opts.Cmd
	if len(cmdline) == 0 {
		cmdline = []string{"/bin/sh"}
	}
	env := make(map[string]string, len(opts.Env)+1)
	for k, v := range opts.Env {
		env[k] = v
	}
	if opts.Term != "" {
		env["TERM"] = opts.Term
	}
	return ExecOptions{
		Mode:    ExecModeArgv,
		Command: cmdline[0],
		Args:    cmdline[1:],
		WorkDir: opts.WorkDir,
		Env:     env,
	}
}

// OpenPTY implements PTYProvider for PRootProvider: it builds the same
// proot-jailed command as Exec but attaches it to a host pty.
func (p *PRootProvider) OpenPTY(ctx context.Context, sandboxID string, opts PTYOptions) (PTYSession, error) {
	sb, err := p.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}
	cmd, err := p.BuildCommand(ctx, sb, ptyExecOptions(opts))
	if err != nil {
		return nil, err
	}
	return startHostPTY(cmd, opts.Cols, opts.Rows)
}

// OpenPTY implements PTYProvider for MockProvider by running the requested
// command on the host under a real pty, scoped to the sandbox's temp root.
func (m *MockProvider) OpenPTY(ctx context.Context, sandboxID string, opts PTYOptions) (PTYSession, error) {
	sb, err := m.getSandbox(sandboxID)
	if err != nil {
		return nil, err
	}

	cmdline := opts.Cmd
	if len(cmdline) == 0 {
		cmdline = []string{"/bin/sh"}
	}

	c := exec.CommandContext(ctx, cmdline[0], cmdline[1:]...)
	c.Dir = filepath.Join(sb.root, "workspace")
	if opts.WorkDir != "" {
		c.Dir = filepath.Join(sb.root, opts.WorkDir)
	}
	c.Env = os.Environ()
	c.Env = append(c.Env,
		"HOME="+sb.root,
		"WORKSPACE="+filepath.Join(sb.root, "workspace"),
		"SANDBOX_ROOT="+sb.root,
	)
	if opts.Term != "" {
		c.Env = append(c.Env, "TERM="+opts.Term)
	}
	for k, v := range opts.Env {
		c.Env = append(c.Env, k+"="+v)
	}

	return startHostPTY(c, opts.Cols, opts.Rows)
}
