package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// CustomProvider forwards all Provider interface calls to a user-specified
// HTTP endpoint.  This allows users to bring their own sandbox backend.
//
// The remote endpoint is expected to expose the following paths:
//
//	POST   /spawn           — create a new sandbox
//	POST   /exec            — execute a command
//	POST   /files           — write a file
//	GET    /files            — read a file  (query: sandbox_id, path)
//	GET    /files/list       — list files   (query: sandbox_id, path)
//	GET    /status/:id       — sandbox status
//	DELETE /sandboxes/:id    — destroy a sandbox
//	GET    /health           — health check
type CustomProvider struct {
	name    string
	baseURL string
	apiKey  string
	client  *http.Client
}

// CustomProviderConfig holds the configuration for a custom HTTP provider.
type CustomProviderConfig struct {
	ProviderName string        `yaml:"name"     mapstructure:"name"`
	BaseURL      string        `yaml:"base_url" mapstructure:"base_url"`
	APIKey       string        `yaml:"api_key"  mapstructure:"api_key"`
	Timeout      time.Duration `yaml:"timeout"  mapstructure:"timeout"` // 0 means 60s default
}

// NewCustomProvider creates a new CustomProvider that forwards calls to the
// given HTTP endpoint.
func NewCustomProvider(cfg CustomProviderConfig) *CustomProvider {
	name := cfg.ProviderName
	if name == "" {
		name = "custom"
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	return &CustomProvider{
		name:    name,
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		client:  &http.Client{Timeout: timeout},
	}
}

func (p *CustomProvider) Name() string { return p.name }

func customHTTPError(operation string, code int, data []byte, sandboxID string) error {
	switch code {
	case http.StatusNotFound:
		return SandboxNotFoundError(sandboxID)
	case http.StatusGone:
		return SandboxDestroyedError(sandboxID)
	case http.StatusRequestTimeout:
		return ExecTimeoutError(sandboxID)
	case http.StatusTooManyRequests:
		return ResourceLimitError(operation)
	case http.StatusServiceUnavailable:
		return ProviderUnavailableError("custom", fmt.Errorf("%s", string(data)))
	default:
		return fmt.Errorf("%s failed (HTTP %d): %s", operation, code, string(data))
	}
}

// doRequest is a shared helper that builds, signs, and executes an HTTP
// request against the remote custom endpoint.
func (p *CustomProvider) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("X-API-Key", p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

// doStreamRequest is like doRequest but returns the raw response for streaming
// consumption. The caller is responsible for closing the response body.
func (p *CustomProvider) doStreamRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")
	if p.apiKey != "" {
		req.Header.Set("X-API-Key", p.apiKey)
	}

	return p.client.Do(req)
}

// Spawn creates a new sandbox on the remote endpoint.
// POST /spawn  { image, memory_mb, vcpus, disk_size_mb, metadata }
// Expects response: { "id": "..." }
func (p *CustomProvider) Spawn(ctx context.Context, opts SpawnOptions) (string, error) {
	body := map[string]interface{}{
		"image": opts.Image,
	}
	if opts.MemoryMB > 0 {
		body["memory_mb"] = opts.MemoryMB
	}
	if opts.VCPUs > 0 {
		body["vcpus"] = opts.VCPUs
	}
	if opts.DiskSizeMB > 0 {
		body["disk_size_mb"] = opts.DiskSizeMB
	}
	if opts.Metadata != nil {
		body["metadata"] = opts.Metadata
	}

	data, code, err := p.doRequest(ctx, "POST", "/spawn", body)
	if err != nil {
		return "", fmt.Errorf("custom spawn: %w", err)
	}
	if code >= 400 {
		return "", customHTTPError("custom spawn", code, data, "")
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("custom spawn: invalid response: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("custom spawn: no sandbox ID in response")
	}
	return result.ID, nil
}

// Exec runs a command in the sandbox.
// POST /exec  { sandbox_id, command, args, env, workdir }
// Expects response: { "exit_code": 0, "stdout": "...", "stderr": "..." }
func (p *CustomProvider) Exec(ctx context.Context, sandboxID string, opts ExecOptions) (*ExecResult, error) {
	body := map[string]interface{}{
		"sandbox_id": sandboxID,
		"command":    opts.Command,
	}
	if len(opts.Args) > 0 {
		body["args"] = opts.Args
	}
	if opts.Env != nil {
		body["env"] = opts.Env
	}
	if opts.WorkDir != "" {
		body["workdir"] = opts.WorkDir
	}

	data, code, err := p.doRequest(ctx, "POST", "/exec", body)
	if err != nil {
		return nil, fmt.Errorf("custom exec: %w", err)
	}
	if code >= 400 {
		return nil, customHTTPError("custom exec", code, data, sandboxID)
	}

	var result struct {
		ExitCode int    `json:"exit_code"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("custom exec: invalid response: %w", err)
	}
	return &ExecResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}, nil
}

// ExecStream runs a command and streams NDJSON output chunks.
// POST /exec  { sandbox_id, command, args, env, workdir, stream: true }
// Expects NDJSON response: { "stream": "stdout"|"stderr", "data": "..." }
func (p *CustomProvider) ExecStream(ctx context.Context, sandboxID string, opts ExecOptions) (<-chan StreamChunk, error) {
	body := map[string]interface{}{
		"sandbox_id": sandboxID,
		"command":    opts.Command,
		"stream":     true,
	}
	if len(opts.Args) > 0 {
		body["args"] = opts.Args
	}
	if opts.Env != nil {
		body["env"] = opts.Env
	}
	if opts.WorkDir != "" {
		body["workdir"] = opts.WorkDir
	}

	resp, err := p.doStreamRequest(ctx, "POST", "/exec", body)
	if err != nil {
		return nil, fmt.Errorf("custom exec stream: %w", err)
	}
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, customHTTPError("custom exec stream", resp.StatusCode, data, sandboxID)
	}

	ch := make(chan StreamChunk, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var chunk StreamChunk
			if err := json.Unmarshal(line, &chunk); err == nil {
				ch <- chunk
			}
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

// WriteFile writes content to a file in the sandbox.
// POST /files  { sandbox_id, path, content, mode }
func (p *CustomProvider) WriteFile(ctx context.Context, sandboxID string, path string, content io.Reader, mode string) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("reading content: %w", err)
	}

	body := map[string]string{
		"sandbox_id": sandboxID,
		"path":       path,
		"content":    string(data),
	}
	if mode != "" {
		body["mode"] = mode
	}

	respData, code, err := p.doRequest(ctx, "POST", "/files", body)
	if err != nil {
		return fmt.Errorf("custom write: %w", err)
	}
	if code >= 400 {
		return customHTTPError("custom write", code, respData, sandboxID)
	}
	return nil
}

// ReadFile reads a file from the sandbox.
// GET /files?sandbox_id=...&path=...
func (p *CustomProvider) ReadFile(ctx context.Context, sandboxID string, path string) (io.ReadCloser, error) {
	params := url.Values{}
	params.Set("sandbox_id", sandboxID)
	params.Set("path", path)

	data, code, err := p.doRequest(ctx, "GET", "/files?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("custom read: %w", err)
	}
	if code >= 400 {
		return nil, customHTTPError("custom read", code, data, sandboxID)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// ListFiles lists files at the given path in the sandbox.
// GET /files/list?sandbox_id=...&path=...
func (p *CustomProvider) ListFiles(ctx context.Context, sandboxID string, path string) ([]FileInfo, error) {
	params := url.Values{}
	params.Set("sandbox_id", sandboxID)
	params.Set("path", path)

	data, code, err := p.doRequest(ctx, "GET", "/files/list?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("custom list: %w", err)
	}
	if code >= 400 {
		return nil, customHTTPError("custom list", code, data, sandboxID)
	}

	var files []FileInfo
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("custom list: invalid response: %w", err)
	}
	return files, nil
}

func (p *CustomProvider) DeleteFile(ctx context.Context, sandboxID string, path string, recursive bool) error {
	body := map[string]interface{}{
		"sandbox_id": sandboxID,
		"path":       path,
		"recursive":  recursive,
	}
	data, code, err := p.doRequest(ctx, "DELETE", "/files", body)
	if err != nil {
		return fmt.Errorf("custom delete: %w", err)
	}
	if code >= 400 {
		return customHTTPError("custom delete", code, data, sandboxID)
	}
	return nil
}

func (p *CustomProvider) MoveFile(ctx context.Context, sandboxID string, oldPath, newPath string) error {
	body := map[string]interface{}{
		"sandbox_id": sandboxID,
		"old_path":   oldPath,
		"new_path":   newPath,
	}
	data, code, err := p.doRequest(ctx, "POST", "/files/move", body)
	if err != nil {
		return fmt.Errorf("custom move: %w", err)
	}
	if code >= 400 {
		return customHTTPError("custom move", code, data, sandboxID)
	}
	return nil
}

func (p *CustomProvider) ChmodFile(ctx context.Context, sandboxID string, path string, mode string) error {
	body := map[string]interface{}{
		"sandbox_id": sandboxID,
		"path":       path,
		"mode":       mode,
	}
	data, code, err := p.doRequest(ctx, "POST", "/files/chmod", body)
	if err != nil {
		return fmt.Errorf("custom chmod: %w", err)
	}
	if code >= 400 {
		return customHTTPError("custom chmod", code, data, sandboxID)
	}
	return nil
}

func (p *CustomProvider) StatFile(ctx context.Context, sandboxID string, path string) (*FileInfo, error) {
	params := url.Values{}
	params.Set("sandbox_id", sandboxID)
	params.Set("path", path)

	data, code, err := p.doRequest(ctx, "GET", "/files/stat?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("custom stat: %w", err)
	}
	if code >= 400 {
		return nil, customHTTPError("custom stat", code, data, sandboxID)
	}

	var fi FileInfo
	if err := json.Unmarshal(data, &fi); err != nil {
		return nil, fmt.Errorf("custom stat: invalid response: %w", err)
	}
	return &fi, nil
}

func (p *CustomProvider) GlobFiles(ctx context.Context, sandboxID string, pattern string) ([]string, error) {
	params := url.Values{}
	params.Set("sandbox_id", sandboxID)
	params.Set("pattern", pattern)

	data, code, err := p.doRequest(ctx, "GET", "/files/glob?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("custom glob: %w", err)
	}
	if code >= 400 {
		return nil, customHTTPError("custom glob", code, data, sandboxID)
	}

	var matches []string
	if err := json.Unmarshal(data, &matches); err != nil {
		return nil, fmt.Errorf("custom glob: invalid response: %w", err)
	}
	return matches, nil
}

// Status returns the current status of a sandbox.
// GET /status/:id
func (p *CustomProvider) Status(ctx context.Context, sandboxID string) (*SandboxStatus, error) {
	data, code, err := p.doRequest(ctx, "GET", "/status/"+url.PathEscape(sandboxID), nil)
	if err != nil {
		return nil, fmt.Errorf("custom status: %w", err)
	}
	if code == 404 {
		return nil, SandboxNotFoundError(sandboxID)
	}
	if code >= 400 {
		return nil, customHTTPError("custom status", code, data, sandboxID)
	}

	var result struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("custom status: invalid response: %w", err)
	}

	id := result.ID
	if id == "" {
		id = sandboxID
	}
	state := result.State
	if state == "" {
		state = "running"
	}

	return &SandboxStatus{ID: id, State: state}, nil
}

// Destroy tears down a sandbox.
// DELETE /sandboxes/:id
func (p *CustomProvider) Destroy(ctx context.Context, sandboxID string) error {
	data, code, err := p.doRequest(ctx, "DELETE", "/sandboxes/"+url.PathEscape(sandboxID), nil)
	if err != nil {
		return fmt.Errorf("custom destroy: %w", err)
	}
	if code >= 400 && code != 404 {
		return customHTTPError("custom destroy", code, data, sandboxID)
	}
	return nil
}

func (p *CustomProvider) ConsoleLog(_ context.Context, _ string, _ int) ([]string, error) {
	return []string{"[INFO] console logs not available for custom provider"}, nil
}

// Healthy returns true if the remote provider endpoint is reachable and
// reports itself as healthy.
// GET /health
func (p *CustomProvider) Healthy(ctx context.Context) bool {
	_, code, err := p.doRequest(ctx, "GET", "/health", nil)
	return err == nil && code < 400
}
