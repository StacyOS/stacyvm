package providers

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/rs/zerolog"
)

// DockerProviderConfig holds all configuration for the Docker provider.
type DockerProviderConfig struct {
	Socket         string
	Runtime        string
	DefaultImage   string
	NetworkMode    string
	SeccompProfile string
	ReadOnlyRootfs bool
	Memory         string
	CPUs           string
	PidsLimit      int64
	User           string
	DroppedCaps    []string
	AddedCaps      []string
	Tmpfs          map[string]string
	PoolSecurity   PoolSecurityProviderConfig
	PreviewDomain  string
}

// PoolSecurityProviderConfig controls per-user isolation within a shared container.
type PoolSecurityProviderConfig struct {
	PerUserUID           bool
	PIDNamespace         bool
	WorkspacePermissions bool
	HidePID              bool
}

type dockerSandbox struct {
	id    string
	image string
	state string
}

// ctxChanWriter sends written bytes as StreamChunks to a channel, respecting context cancellation.
type ctxChanWriter struct {
	ch     chan<- StreamChunk
	stream string
	ctx    context.Context
}

func (w *ctxChanWriter) Write(p []byte) (int, error) {
	data := make([]byte, len(p))
	copy(data, p)
	select {
	case w.ch <- StreamChunk{Stream: w.stream, Data: string(data)}:
		return len(p), nil
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	}
}

// tarFileReader extracts a single file from a tar archive returned by CopyFromContainer.
type tarFileReader struct {
	*tar.Reader
	closer io.Closer
}

func (r *tarFileReader) Close() error { return r.closer.Close() }

// DockerProvider implements Provider using Docker containers.
type DockerProvider struct {
	mu        sync.RWMutex
	sandboxes map[string]*dockerSandbox
	cli       *client.Client
	config    DockerProviderConfig
	logger    zerolog.Logger
}

// NewDockerProvider creates a new Docker provider connected to the Docker daemon.
func NewDockerProvider(cfg DockerProviderConfig, logger zerolog.Logger) (*DockerProvider, error) {
	// Negotiate the API version with the daemon instead of pinning one. A fixed
	// version breaks both ways: too-new daemons reject an old pin ("client version
	// 1.41 is too old, minimum supported is 1.44") while too-old daemons reject a
	// new pin. Negotiation picks the highest mutually supported version.
	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}
	if cfg.Socket != "" {
		opts = append(opts, client.WithHost(cfg.Socket))
	} else {
		// No explicit socket configured: honor DOCKER_HOST and fall back to the
		// platform-default daemon host (unix socket on Linux/macOS, named pipe on
		// Windows). Forcing the unix path here previously broke Windows, where the
		// client rejects the unix protocol with "protocol not available".
		opts = append(opts, client.FromEnv)
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &DockerProvider{
		sandboxes: make(map[string]*dockerSandbox),
		cli:       cli,
		config:    cfg,
		logger:    logger,
	}, nil
}

func (d *DockerProvider) Name() string { return "docker" }

// ProviderConfig exposes the docker provider configuration (runtime, network).
func (d *DockerProvider) ProviderConfig() DockerProviderConfig { return d.config }

func (d *DockerProvider) Healthy(ctx context.Context) bool {
	_, err := d.cli.Ping(ctx)
	return err == nil
}

// Stats reports live CPU% and memory for a sandbox via the Docker stats API.
// The container name equals the sandbox ID. Implements providers.StatsReporter.
func (d *DockerProvider) Stats(ctx context.Context, sandboxID string) (*SandboxStats, error) {
	if _, err := d.getSandbox(sandboxID); err != nil {
		return nil, err
	}
	resp, err := d.cli.ContainerStats(ctx, sandboxID, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var v container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}

	// CPU% = (Δcontainer / Δsystem) * onlineCPUs * 100, per the Docker docs.
	cpuPct := 0.0
	cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage) - float64(v.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(v.CPUStats.SystemUsage) - float64(v.PreCPUStats.SystemUsage)
	if cpuDelta > 0 && sysDelta > 0 {
		ncpu := float64(v.CPUStats.OnlineCPUs)
		if ncpu == 0 {
			ncpu = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
		}
		if ncpu == 0 {
			ncpu = 1
		}
		cpuPct = (cpuDelta / sysDelta) * ncpu * 100
	}

	// Memory: subtract page cache when reported (cgroup v1 "cache" / v2 "file").
	memUsage := v.MemoryStats.Usage
	if cache, ok := v.MemoryStats.Stats["cache"]; ok && cache <= memUsage {
		memUsage -= cache
	} else if file, ok := v.MemoryStats.Stats["file"]; ok && file <= memUsage {
		memUsage -= file
	}

	return &SandboxStats{
		CPUPercent:       cpuPct,
		MemoryBytes:      memUsage,
		MemoryLimitBytes: v.MemoryStats.Limit,
	}, nil
}

func (d *DockerProvider) Spawn(ctx context.Context, opts SpawnOptions) (string, error) {
	image := opts.Image
	if image == "" {
		image = d.config.DefaultImage
	}
	if image == "" {
		image = "alpine:latest"
	}

	if err := d.ensureImage(ctx, image); err != nil {
		return "", fmt.Errorf("ensure image %q: %w", image, err)
	}

	memBytes := parseMemoryBytes(d.config.Memory)
	if opts.MemoryMB > 0 {
		memBytes = int64(opts.MemoryMB) * 1024 * 1024
	}

	nanoCPUs := parseCPUs(d.config.CPUs)
	if opts.VCPUs > 0 {
		nanoCPUs = int64(opts.VCPUs) * 1e9
	}

	pidsLimit := d.config.PidsLimit
	if pidsLimit <= 0 {
		pidsLimit = 256
	}

	// Generate a unique ID for the sandbox.
	b := make([]byte, 4)
	rand.Read(b)
	sandboxID := fmt.Sprintf("sb-%x", b)

	labels := map[string]string{
		"stacyvm":          "true",
		"stacyvm.provider": "docker",
		"stacyvm.sandbox":  sandboxID,
		"stacyvm.image":    image,
	}
	if len(opts.Metadata) > 0 {
		if data, err := json.Marshal(opts.Metadata); err == nil {
			labels["stacyvm.metadata"] = string(data)
		}
	}

	if d.config.PreviewDomain != "" {
		labels["traefik.enable"] = "true"
		// Route for port 3000 (common for web apps)
		labels[fmt.Sprintf("traefik.http.routers.%s-3000.rule", sandboxID)] =
			fmt.Sprintf("Host(`3000-%s.%s`)", sandboxID, d.config.PreviewDomain)
		labels[fmt.Sprintf("traefik.http.services.%s-3000.loadbalancer.server.port", sandboxID)] = "3000"
	}

	containerCfg := &container.Config{
		Image:      image,
		Cmd:        []string{"sh", "-c", "sleep infinity 2>/dev/null || tail -f /dev/null"},
		WorkingDir: "/workspace",
		Tty:        false,
		Labels:     labels,
	}
	if d.config.User != "" {
		containerCfg.User = d.config.User
	}

	networkMode := d.config.NetworkMode
	if networkMode == "" {
		networkMode = "none"
	}

	hostCfg := &container.HostConfig{
		Runtime:        d.config.Runtime,
		NetworkMode:    container.NetworkMode(networkMode),
		ReadonlyRootfs: d.config.ReadOnlyRootfs,
		Resources: container.Resources{
			Memory:    memBytes,
			NanoCPUs:  nanoCPUs,
			PidsLimit: &pidsLimit,
		},
		SecurityOpt: d.buildSecurityOpts(),
		CapDrop:     d.config.DroppedCaps,
		CapAdd:      d.config.AddedCaps,
		Tmpfs:       d.config.Tmpfs,
		AutoRemove:  false,
	}

	resp, err := d.cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, sandboxID)
	if err != nil {
		return "", fmt.Errorf("creating container: %w", err)
	}

	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		d.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true}) //nolint:errcheck
		return "", fmt.Errorf("starting container: %w", err)
	}

	// Create workspace directory inside the container.
	if _, err := d.containerExec(ctx, sandboxID, "mkdir -p /workspace"); err != nil {
		d.logger.Warn().Err(err).Msg("could not create /workspace in container")
	}

	d.mu.Lock()
	d.sandboxes[sandboxID] = &dockerSandbox{id: sandboxID, image: image, state: "running"}
	d.mu.Unlock()

	d.logger.Info().Str("container", sandboxID).Str("image", image).Msg("docker sandbox spawned")
	return sandboxID, nil
}

func (d *DockerProvider) Exec(ctx context.Context, sandboxID string, opts ExecOptions) (*ExecResult, error) {
	if _, err := d.getSandbox(sandboxID); err != nil {
		return nil, err
	}

	workDir := opts.WorkDir
	if workDir == "" {
		workDir = "/workspace"
	}

	cmd, err := buildExecCommand(opts)
	if err != nil {
		return nil, err
	}

	execCfg := container.ExecOptions{
		Cmd:          cmd,
		WorkingDir:   workDir,
		AttachStdout: true,
		AttachStderr: true,
		Env:          mapToEnvSlice(opts.Env),
	}

	execID, err := d.cli.ContainerExecCreate(ctx, sandboxID, execCfg)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ExecTimeoutError(sandboxID)
		}
		return nil, fmt.Errorf("exec create: %w", err)
	}

	resp, err := d.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ExecTimeoutError(sandboxID)
		}
		return nil, fmt.Errorf("exec attach: %w", err)
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, resp.Reader); err != nil && err != io.EOF {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ExecTimeoutError(sandboxID)
		}
		d.logger.Debug().Err(err).Msg("stdcopy exec error")
	}

	inspect, err := d.cli.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ExecTimeoutError(sandboxID)
		}
		return nil, fmt.Errorf("exec inspect: %w", err)
	}

	return &ExecResult{
		ExitCode: inspect.ExitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

func (d *DockerProvider) ExecStream(ctx context.Context, sandboxID string, opts ExecOptions) (<-chan StreamChunk, error) {
	if _, err := d.getSandbox(sandboxID); err != nil {
		return nil, err
	}

	workDir := opts.WorkDir
	if workDir == "" {
		workDir = "/workspace"
	}

	cmd, err := buildExecCommand(opts)
	if err != nil {
		return nil, err
	}

	execCfg := container.ExecOptions{
		Cmd:          cmd,
		WorkingDir:   workDir,
		AttachStdout: true,
		AttachStderr: true,
		Env:          mapToEnvSlice(opts.Env),
	}

	execID, err := d.cli.ContainerExecCreate(ctx, sandboxID, execCfg)
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}

	resp, err := d.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return nil, fmt.Errorf("exec attach: %w", err)
	}

	ch := make(chan StreamChunk, 64)
	go func() {
		defer close(ch)
		defer resp.Close()
		stdoutW := &ctxChanWriter{ch: ch, stream: "stdout", ctx: ctx}
		stderrW := &ctxChanWriter{ch: ch, stream: "stderr", ctx: ctx}
		if _, err := stdcopy.StdCopy(stdoutW, stderrW, resp.Reader); err != nil && err != io.EOF {
			d.logger.Debug().Err(err).Msg("stdcopy stream error")
		}
		if ctx.Err() == context.DeadlineExceeded {
			select {
			case ch <- StreamChunk{Stream: "stderr", Data: ExecTimeoutError(sandboxID).Error()}:
			default:
			}
		}
	}()

	return ch, nil
}

func (d *DockerProvider) WriteFile(ctx context.Context, sandboxID string, path string, content io.Reader, mode string) error {
	if _, err := d.getSandbox(sandboxID); err != nil {
		return err
	}

	path = cleanContainerPath(path)
	dir := filepath.Dir(path)

	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("reading content: %w", err)
	}

	fileMode := int64(0644)
	if mode != "" {
		if parsed, err := strconv.ParseInt(mode, 8, 32); err == nil {
			fileMode = parsed
		}
	}

	// Ensure parent directory exists.
	if _, err := d.containerExec(ctx, sandboxID, "mkdir -p "+shellQuoteDocker(dir)); err != nil {
		d.logger.Warn().Err(err).Str("dir", dir).Msg("mkdir -p failed")
	}

	// Build tar archive in memory containing a single file.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name:     filepath.Base(path),
		Mode:     fileMode,
		Size:     int64(len(data)),
		Typeflag: tar.TypeReg,
		ModTime:  time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("tar header: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("tar write: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("tar close: %w", err)
	}

	return d.cli.CopyToContainer(ctx, sandboxID, dir, &buf, container.CopyToContainerOptions{})
}

func (d *DockerProvider) ReadFile(ctx context.Context, sandboxID string, path string) (io.ReadCloser, error) {
	if _, err := d.getSandbox(sandboxID); err != nil {
		return nil, err
	}

	path = cleanContainerPath(path)

	rc, _, err := d.cli.CopyFromContainer(ctx, sandboxID, path)
	if err != nil {
		return nil, fmt.Errorf("copy from container: %w", err)
	}

	// CopyFromContainer returns a tar archive; extract the first entry.
	tr := tar.NewReader(rc)
	if _, err := tr.Next(); err != nil {
		rc.Close()
		return nil, fmt.Errorf("reading tar entry: %w", err)
	}
	return &tarFileReader{Reader: tr, closer: rc}, nil
}

func (d *DockerProvider) ListFiles(ctx context.Context, sandboxID string, path string) ([]FileInfo, error) {
	if _, err := d.getSandbox(sandboxID); err != nil {
		return nil, err
	}

	path = cleanContainerPath(path)

	// Use find + stat with | separator for structured output: name|size|octal-mode|type|mtime-epoch
	cmd := fmt.Sprintf(
		`find %s -maxdepth 1 -mindepth 1 2>/dev/null | while IFS= read -r f; do stat -c '%%n|%%s|%%a|%%F|%%Y' "$f" 2>/dev/null; done`,
		shellQuoteDocker(path),
	)
	result, err := d.containerExec(ctx, sandboxID, cmd)
	if err != nil {
		return nil, fmt.Errorf("listing directory: %w", err)
	}

	return parseStatOutput(result.Stdout), nil
}

func (d *DockerProvider) DeleteFile(ctx context.Context, sandboxID string, path string, recursive bool) error {
	if _, err := d.getSandbox(sandboxID); err != nil {
		return err
	}

	path = cleanContainerPath(path)
	flag := "-f"
	if recursive {
		flag = "-rf"
	}
	result, err := d.containerExec(ctx, sandboxID, "rm "+flag+" "+shellQuoteDocker(path))
	if err != nil {
		return fmt.Errorf("delete file: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("rm failed (exit %d): %s", result.ExitCode, result.Stderr)
	}
	return nil
}

func (d *DockerProvider) MoveFile(ctx context.Context, sandboxID string, oldPath, newPath string) error {
	if _, err := d.getSandbox(sandboxID); err != nil {
		return err
	}

	oldPath = cleanContainerPath(oldPath)
	newPath = cleanContainerPath(newPath)
	newDir := filepath.Dir(newPath)

	cmd := fmt.Sprintf("mkdir -p %s && mv %s %s", shellQuoteDocker(newDir), shellQuoteDocker(oldPath), shellQuoteDocker(newPath))
	result, err := d.containerExec(ctx, sandboxID, cmd)
	if err != nil {
		return fmt.Errorf("move file: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("mv failed (exit %d): %s", result.ExitCode, result.Stderr)
	}
	return nil
}

func (d *DockerProvider) ChmodFile(ctx context.Context, sandboxID string, path string, mode string) error {
	if _, err := d.getSandbox(sandboxID); err != nil {
		return err
	}

	path = cleanContainerPath(path)
	result, err := d.containerExec(ctx, sandboxID, "chmod "+shellQuoteDocker(mode)+" "+shellQuoteDocker(path))
	if err != nil {
		return fmt.Errorf("chmod file: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("chmod failed (exit %d): %s", result.ExitCode, result.Stderr)
	}
	return nil
}

func (d *DockerProvider) StatFile(ctx context.Context, sandboxID string, path string) (*FileInfo, error) {
	if _, err := d.getSandbox(sandboxID); err != nil {
		return nil, err
	}

	path = cleanContainerPath(path)
	// Use ContainerStatPath for metadata, then exec stat for mode details.
	stat, err := d.cli.ContainerStatPath(ctx, sandboxID, path)
	if err != nil {
		return nil, fmt.Errorf("stat path: %w", err)
	}

	// Get octal mode and type via exec stat.
	result, err := d.containerExec(ctx, sandboxID, "stat -c '%a %F %Y' "+shellQuoteDocker(path))
	modeStr := ""
	isDir := stat.Mode.IsDir()
	if err == nil && result.ExitCode == 0 {
		parts := strings.Fields(strings.TrimSpace(result.Stdout))
		if len(parts) >= 1 {
			modeStr = parts[0]
		}
	}

	return &FileInfo{
		Path:    path,
		Size:    stat.Size,
		Mode:    modeStr,
		IsDir:   isDir,
		ModTime: stat.Mtime.UTC().Format("2006-01-02T15:04:05Z"),
	}, nil
}

func (d *DockerProvider) GlobFiles(ctx context.Context, sandboxID string, pattern string) ([]string, error) {
	if _, err := d.getSandbox(sandboxID); err != nil {
		return nil, err
	}

	// Pass the pattern via an env variable to avoid shell interpolation of caller-supplied input.
	cmd := `sh -c 'for f in $GLOB_PATTERN; do [ -e "$f" ] && echo "$f"; done 2>/dev/null'`
	execCfg := container.ExecOptions{
		Cmd:          []string{"sh", "-c", cmd},
		AttachStdout: true,
		AttachStderr: true,
		Env:          []string{"GLOB_PATTERN=" + pattern},
	}
	execID, err := d.cli.ContainerExecCreate(ctx, sandboxID, execCfg)
	if err != nil {
		return nil, fmt.Errorf("glob exec create: %w", err)
	}
	resp, err := d.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return nil, fmt.Errorf("glob exec attach: %w", err)
	}
	defer resp.Close()

	var stdout bytes.Buffer
	stdcopy.StdCopy(&stdout, io.Discard, resp.Reader) //nolint:errcheck

	var matches []string
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			matches = append(matches, line)
		}
	}
	return matches, nil
}

func (d *DockerProvider) Status(ctx context.Context, sandboxID string) (*SandboxStatus, error) {
	info, err := d.cli.ContainerInspect(ctx, sandboxID)
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, SandboxNotFoundError(sandboxID)
		}
		return nil, fmt.Errorf("inspect container %q: %w", sandboxID, err)
	}

	state := "unknown"
	if info.State != nil {
		switch {
		case info.State.Running:
			state = "running"
		case info.State.Paused:
			state = "paused"
		case info.State.Restarting:
			state = "restarting"
		default:
			state = "stopped"
		}
	}
	return &SandboxStatus{ID: sandboxID, State: state}, nil
}

func (d *DockerProvider) Destroy(ctx context.Context, sandboxID string) error {
	// Always remove from the in-memory map, even if Docker removal fails,
	// so subsequent getSandbox calls return a clean "not found" error.
	d.mu.Lock()
	delete(d.sandboxes, sandboxID)
	d.mu.Unlock()

	timeout := 5
	if err := d.cli.ContainerStop(ctx, sandboxID, container.StopOptions{Timeout: &timeout}); err != nil {
		d.logger.Debug().Err(err).Msg("container stop (continuing with remove)")
	}
	if err := d.cli.ContainerRemove(ctx, sandboxID, container.RemoveOptions{Force: true}); err != nil {
		if client.IsErrNotFound(err) {
			return SandboxNotFoundError(sandboxID)
		}
		return fmt.Errorf("removing container %q: %w", sandboxID, err)
	}

	short := sandboxID
	if len(short) > 12 {
		short = short[:12]
	}
	d.logger.Info().Str("container", short).Msg("docker sandbox destroyed")
	return nil
}

func (d *DockerProvider) ConsoleLog(ctx context.Context, sandboxID string, lines int) ([]string, error) {
	if _, err := d.getSandbox(sandboxID); err != nil {
		return nil, err
	}

	tail := "all"
	if lines > 0 {
		tail = strconv.Itoa(lines)
	}

	rc, err := d.cli.ContainerLogs(ctx, sandboxID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
	})
	if err != nil {
		return nil, fmt.Errorf("container logs: %w", err)
	}
	defer rc.Close()

	var stdout, stderr bytes.Buffer
	stdcopy.StdCopy(&stdout, &stderr, rc) //nolint:errcheck

	combined := stdout.String() + stderr.String()
	raw := strings.Split(strings.TrimRight(combined, "\n"), "\n")
	result := make([]string, 0, len(raw))
	for _, line := range raw {
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

func (d *DockerProvider) ListRuntimeSandboxes(ctx context.Context) ([]RuntimeSandbox, error) {
	args := filters.NewArgs(filters.Arg("label", "stacyvm=true"))
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return nil, fmt.Errorf("list stacyvm containers: %w", err)
	}

	out := make([]RuntimeSandbox, 0, len(containers))
	for _, c := range containers {
		id := c.Labels["stacyvm.sandbox"]
		if id == "" && len(c.Names) > 0 {
			id = strings.TrimPrefix(c.Names[0], "/")
		}
		if id == "" {
			id = c.ID
		}

		image := c.Labels["stacyvm.image"]
		if image == "" {
			image = c.Image
		}

		metadata := map[string]string{}
		if raw := c.Labels["stacyvm.metadata"]; raw != "" {
			_ = json.Unmarshal([]byte(raw), &metadata)
		}

		out = append(out, RuntimeSandbox{
			ID:        id,
			State:     dockerContainerState(c.State),
			Provider:  d.Name(),
			Image:     image,
			CreatedAt: time.Unix(c.Created, 0).UTC(),
			Metadata:  metadata,
		})
		d.rememberSandbox(id, image, dockerContainerState(c.State))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Private helpers
// ---------------------------------------------------------------------------

func (d *DockerProvider) getSandbox(id string) (*dockerSandbox, error) {
	d.mu.RLock()
	sb, ok := d.sandboxes[id]
	d.mu.RUnlock()
	if !ok {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		info, err := d.cli.ContainerInspect(ctx, id)
		if err != nil {
			if client.IsErrNotFound(err) {
				return nil, SandboxNotFoundError(id)
			}
			return nil, fmt.Errorf("inspect container %q: %w", id, err)
		}
		if info.Config == nil || info.Config.Labels["stacyvm"] != "true" {
			return nil, SandboxNotFoundError(id)
		}
		image := info.Config.Labels["stacyvm.image"]
		if image == "" {
			image = info.Config.Image
		}
		state := "unknown"
		if info.State != nil {
			state = dockerContainerState(info.State.Status)
		}
		return d.rememberSandbox(id, image, state), nil
	}
	return sb, nil
}

func (d *DockerProvider) rememberSandbox(id, image, state string) *dockerSandbox {
	if state == "" {
		state = "running"
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	sb := &dockerSandbox{id: id, image: image, state: state}
	d.sandboxes[id] = sb
	return sb
}

func dockerContainerState(state string) string {
	switch state {
	case "running":
		return "running"
	case "paused":
		return "paused"
	case "restarting":
		return "restarting"
	case "created":
		return "creating"
	case "exited", "dead", "removing":
		return "stopped"
	default:
		if state == "" {
			return "unknown"
		}
		return state
	}
}

// containerExec runs a one-shot command inside the container and returns the result.
func (d *DockerProvider) containerExec(ctx context.Context, containerID string, cmd string) (*ExecResult, error) {
	execCfg := container.ExecOptions{
		Cmd:          []string{"sh", "-c", cmd},
		AttachStdout: true,
		AttachStderr: true,
	}
	execID, err := d.cli.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return nil, err
	}
	resp, err := d.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return nil, err
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	stdcopy.StdCopy(&stdout, &stderr, resp.Reader) //nolint:errcheck

	inspect, err := d.cli.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return nil, err
	}
	return &ExecResult{
		ExitCode: inspect.ExitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

// ensureImage pulls the image if it is not already present in the local daemon cache.
func (d *DockerProvider) ensureImage(ctx context.Context, imageName string) error {
	// If no tag is provided, assume :latest for matching purposes
	searchName := imageName
	if !strings.Contains(searchName, ":") {
		searchName += ":latest"
	}

	// Check local cache first to avoid unnecessary registry calls.
	images, err := d.cli.ImageList(ctx, dockerimage.ListOptions{})
	if err == nil {
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if tag == imageName || tag == searchName {
					d.logger.Debug().Str("image", imageName).Msg("image found locally")
					return nil
				}
			}
		}
	}

	// Image not found locally, proceed to pull from registry.
	d.logger.Info().Str("image", imageName).Msg("image not found locally; pulling from registry")
	rc, err := d.cli.ImagePull(ctx, imageName, dockerimage.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image: %w", err)
	}
	defer rc.Close()

	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("consuming pull stream (partial pull): %w", err)
	}
	return nil
}

func (d *DockerProvider) buildSecurityOpts() []string {
	if d.config.SeccompProfile == "" || d.config.SeccompProfile == "default" {
		return nil
	}
	return []string{"seccomp=" + d.config.SeccompProfile}
}

// cleanContainerPath returns a cleaned absolute container path using POSIX path semantics
// (path.Clean, not filepath.Clean) since containers are always Linux.
func cleanContainerPath(p string) string {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

// scopedWorkspacePath returns a workspace path scoped to a sandbox, preventing traversal.
func scopedWorkspacePath(sandboxID, p string) string {
	base := "/workspace/" + sandboxID
	clean := path.Clean("/" + p)
	if strings.HasPrefix(clean, base+"/") || clean == base {
		return clean
	}
	return base + "/" + filepath.Base(clean)
}

// parseMemoryBytes converts a human-readable memory string to bytes.
func parseMemoryBytes(s string) int64 {
	if s == "" {
		return 0
	}
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "0" {
		return 0
	}
	multipliers := map[string]int64{
		"k": 1024,
		"m": 1024 * 1024,
		"g": 1024 * 1024 * 1024,
	}
	for suffix, mult := range multipliers {
		if strings.HasSuffix(s, suffix) {
			n, err := strconv.ParseInt(s[:len(s)-len(suffix)], 10, 64)
			if err != nil {
				return 0
			}
			return n * mult
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// parseCPUs converts a CPU count string to Docker NanoCPUs.
func parseCPUs(s string) int64 {
	if s == "" || s == "0" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f * 1e9)
}

// mapToEnvSlice converts a map of env vars to a KEY=VALUE slice.
func mapToEnvSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}

// shellQuoteDocker wraps a string in single quotes for safe shell interpolation.
func shellQuoteDocker(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// parseStatOutput parses output from:
//
//	find /path ... | while read -r f; do stat -c '%n|%s|%a|%F|%Y' "$f"; done
//
// Each line: name|size|octal-mode|type|mtime-epoch
// Uses SplitN(5) so that filenames containing '|' don't break parsing.
func parseStatOutput(output string) []FileInfo {
	var files []FileInfo
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}
		size, _ := strconv.ParseInt(parts[1], 10, 64)
		epoch, _ := strconv.ParseInt(parts[4], 10, 64)
		modTime := time.Unix(epoch, 0).UTC().Format("2006-01-02T15:04:05Z")
		isDir := strings.Contains(parts[3], "directory")
		files = append(files, FileInfo{
			Path:    parts[0],
			Size:    size,
			Mode:    parts[2],
			IsDir:   isDir,
			ModTime: modTime,
		})
	}
	return files
}
