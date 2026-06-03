package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// OpenPTY implements PTYProvider for DockerProvider. The command runs in a
// docker exec with a TTY allocated inside the container, so the attached
// connection is a single binary-safe full-duplex stream: writes deliver stdin
// (keystrokes), reads return terminal output. No stdcopy multiplexing applies
// under TTY mode.
func (d *DockerProvider) OpenPTY(ctx context.Context, sandboxID string, opts PTYOptions) (PTYSession, error) {
	if _, err := d.getSandbox(sandboxID); err != nil {
		return nil, err
	}

	workDir := opts.WorkDir
	if workDir == "" {
		workDir = "/workspace"
	}

	cmd := opts.Cmd
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}

	env := mapToEnvSlice(opts.Env)
	if opts.Term != "" {
		env = append(env, "TERM="+opts.Term)
	}

	execCfg := container.ExecOptions{
		Cmd:          cmd,
		WorkingDir:   workDir,
		Env:          env,
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := d.cli.ContainerExecCreate(ctx, sandboxID, execCfg)
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}

	rows, cols := opts.Rows, opts.Cols
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}

	hr, err := d.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{
		Tty:         true,
		ConsoleSize: &[2]uint{uint(rows), uint(cols)},
	})
	if err != nil {
		return nil, fmt.Errorf("exec attach: %w", err)
	}

	d.logger.Debug().Str("container", sandboxID).Str("exec", execID.ID).Msg("docker pty opened")
	return &dockerPTYSession{cli: d.cli, execID: execID.ID, hr: hr, ctx: ctx}, nil
}

// dockerPTYSession bridges a PTYSession to a Docker exec attached in TTY mode.
type dockerPTYSession struct {
	cli    *client.Client
	execID string
	hr     types.HijackedResponse
	ctx    context.Context
}

func (s *dockerPTYSession) Read(p []byte) (int, error)  { return s.hr.Reader.Read(p) }
func (s *dockerPTYSession) Write(p []byte) (int, error) { return s.hr.Conn.Write(p) }

func (s *dockerPTYSession) Close() error {
	s.hr.Close()
	return nil
}

func (s *dockerPTYSession) Resize(cols, rows uint16) error {
	return s.cli.ContainerExecResize(s.ctx, s.execID, container.ResizeOptions{
		Height: uint(rows),
		Width:  uint(cols),
	})
}

// Signal is a no-op for the Docker provider: the Docker API has no per-exec
// signal call, so interactive signals (Ctrl-C, etc.) reach the process through
// the TTY line discipline when the client writes the corresponding control
// bytes to the stream.
func (s *dockerPTYSession) Signal(sig string) error { return nil }

func (s *dockerPTYSession) Wait() (int, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		inspect, err := s.cli.ContainerExecInspect(s.ctx, s.execID)
		if err != nil {
			return -1, err
		}
		if !inspect.Running {
			return inspect.ExitCode, nil
		}
		select {
		case <-s.ctx.Done():
			return -1, s.ctx.Err()
		case <-ticker.C:
		}
	}
}
