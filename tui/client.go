package tui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type apiClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func newAPIClient(baseURL, apiKey string) *apiClient {
	return &apiClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

type sandboxData struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`
	Provider  string    `json:"provider"`
	Image     string    `json:"image"`
	MemoryMB  int       `json:"memory_mb"`
	VCPUs     int       `json:"vcpus"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type execResultData struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Duration string `json:"duration"`
}

type healthData struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

type providerData struct {
	Name         string   `json:"name"`
	IsDefault    bool     `json:"default"`
	Healthy      bool     `json:"healthy"`
	LatencyMS    int64    `json:"latency_ms"`
	Capabilities []string `json:"capabilities"`
	RuntimeCount *int     `json:"runtime_count"`
}

type templateData struct {
	Name        string `json:"name"`
	Image       string `json:"image"`
	Description string `json:"description"`
	MemoryMB    int    `json:"memory_mb"`
	CPUCores    int    `json:"cpu_cores"`
	TTLSeconds  int    `json:"ttl_seconds"`
	PoolSize    int    `json:"pool_size"`
}

func (c *apiClient) do(method, path string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

func (c *apiClient) health() (*healthData, error) {
	data, _, err := c.do("GET", "/api/v1/health", nil)
	if err != nil {
		return nil, err
	}
	var h healthData
	json.Unmarshal(data, &h)
	return &h, nil
}

func (c *apiClient) listSandboxes() ([]sandboxData, error) {
	data, _, err := c.do("GET", "/api/v1/sandboxes", nil)
	if err != nil {
		return nil, err
	}
	var sbs []sandboxData
	json.Unmarshal(data, &sbs)
	return sbs, nil
}

func (c *apiClient) spawn(image, ttl string) (*sandboxData, error) {
	body := map[string]string{"image": image}
	if ttl != "" {
		body["ttl"] = ttl
	}
	data, code, err := c.do("POST", "/api/v1/sandboxes", body)
	if err != nil {
		return nil, err
	}
	if code != 201 {
		return nil, fmt.Errorf("spawn failed: %s", string(data))
	}
	var sb sandboxData
	json.Unmarshal(data, &sb)
	return &sb, nil
}

func (c *apiClient) spawnTemplate(name string) (*sandboxData, error) {
	data, code, err := c.do("POST", "/api/v1/templates/"+name+"/spawn", nil)
	if err != nil {
		return nil, err
	}
	if code != 201 {
		return nil, fmt.Errorf("spawn from template failed: %s", string(data))
	}
	var sb sandboxData
	json.Unmarshal(data, &sb)
	return &sb, nil
}

func (c *apiClient) destroy(id string) error {
	_, code, err := c.do("DELETE", "/api/v1/sandboxes/"+id, nil)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("destroy failed (HTTP %d)", code)
	}
	return nil
}

func (c *apiClient) exec(id, command string) (*execResultData, error) {
	data, code, err := c.do("POST", "/api/v1/sandboxes/"+id+"/exec", map[string]string{"command": command})
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("exec failed: %s", string(data))
	}
	var r execResultData
	json.Unmarshal(data, &r)
	return &r, nil
}

func (c *apiClient) writeFile(id, path, content string) error {
	_, code, err := c.do("POST", "/api/v1/sandboxes/"+id+"/files", map[string]string{
		"path": path, "content": content,
	})
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("write failed (HTTP %d)", code)
	}
	return nil
}

func (c *apiClient) readFile(id, path string) (string, error) {
	req, _ := http.NewRequest("GET", c.baseURL+"/api/v1/sandboxes/"+id+"/files?path="+path, nil)
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("read failed: %s", string(data))
	}
	return string(data), nil
}

func (c *apiClient) listProviders() ([]providerData, error) {
	data, _, err := c.do("GET", "/api/v1/providers", nil)
	if err != nil {
		return nil, err
	}
	var providers []providerData
	json.Unmarshal(data, &providers)
	return providers, nil
}

func (c *apiClient) listTemplates() ([]templateData, error) {
	data, _, err := c.do("GET", "/api/v1/templates", nil)
	if err != nil {
		return nil, err
	}
	var templates []templateData
	json.Unmarshal(data, &templates)
	return templates, nil
}

func (c *apiClient) createTemplate(name, image, desc, memStr, cpuStr, ttlStr string) error {
	mem := 512
	if memStr != "" {
		if v, err := strconv.Atoi(memStr); err == nil {
			mem = v
		}
	}
	cpus := 1
	if cpuStr != "" {
		if v, err := strconv.Atoi(cpuStr); err == nil {
			cpus = v
		}
	}
	ttl := 300
	if ttlStr != "" {
		if v, err := strconv.Atoi(ttlStr); err == nil {
			ttl = v
		}
	}

	body := map[string]interface{}{
		"name":        name,
		"image":       image,
		"description": desc,
		"memory_mb":   mem,
		"cpu_cores":   cpus,
		"ttl_seconds": ttl,
	}

	_, code, err := c.do("POST", "/api/v1/templates", body)
	if err != nil {
		return err
	}
	if code != 201 {
		return fmt.Errorf("create template failed (HTTP %d)", code)
	}
	return nil
}

func (c *apiClient) deleteTemplate(name string) error {
	_, code, err := c.do("DELETE", "/api/v1/templates/"+name, nil)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("delete template failed (HTTP %d)", code)
	}
	return nil
}

// systemStats fetches real host telemetry from GET /api/v1/system/stats.
func (c *apiClient) systemStats() (hostSnapshot, error) {
	data, code, err := c.do("GET", "/api/v1/system/stats", nil)
	if err != nil {
		return hostSnapshot{}, err
	}
	if code != 200 {
		return hostSnapshot{}, fmt.Errorf("system stats failed (HTTP %d)", code)
	}
	var raw struct {
		CPUPct   float64 `json:"cpu_pct"`
		MemPct   float64 `json:"mem_pct"`
		DiskPct  float64 `json:"disk_pct"`
		NetRxBps float64 `json:"net_rx_bps"`
		NetTxBps float64 `json:"net_tx_bps"`
		Load1    float64 `json:"load1"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return hostSnapshot{}, err
	}
	return hostSnapshot{
		cpuPct:   raw.CPUPct,
		memPct:   raw.MemPct,
		diskPct:  raw.DiskPct,
		netRxBps: raw.NetRxBps,
		netTxBps: raw.NetTxBps,
		load1:    raw.Load1,
		ok:       true,
	}, nil
}

// sandboxStats fetches real per-sandbox stats from /sandboxes/{id}/stats.
func (c *apiClient) sandboxStats(id string) (sandboxStat, error) {
	data, code, err := c.do("GET", "/api/v1/sandboxes/"+id+"/stats", nil)
	if err != nil {
		return sandboxStat{}, err
	}
	if code != 200 {
		return sandboxStat{}, fmt.Errorf("sandbox stats failed (HTTP %d)", code)
	}
	var raw struct {
		Supported   bool    `json:"supported"`
		CPUPct      float64 `json:"cpu_pct"`
		MemBytes    uint64  `json:"mem_bytes"`
		MemLimit    uint64  `json:"mem_limit_bytes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return sandboxStat{}, err
	}
	return sandboxStat{
		cpuPct:    raw.CPUPct,
		memBytes:  raw.MemBytes,
		memLimit:  raw.MemLimit,
		supported: raw.Supported,
	}, nil
}

// subscribeEvents connects to the SSE event bus and pushes parsed events onto
// ch. It blocks forever, reconnecting on error — run it in a goroutine.
func (c *apiClient) subscribeEvents(ch chan<- eventEntry) {
	for {
		c.readEventStream(ch)
		time.Sleep(2 * time.Second) // reconnect backoff
	}
}

func (c *apiClient) readEventStream(ch chan<- eventEntry) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/events", nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	// No client timeout for the stream.
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var ev struct {
			Type      string    `json:"type"`
			SandboxID string    `json:"sandbox_id"`
			Timestamp time.Time `json:"timestamp"`
		}
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		kind, detail := kindFromEventType(ev.Type, ev.SandboxID)
		ts := ev.Timestamp
		if ts.IsZero() {
			ts = time.Now()
		}
		ch <- eventEntry{ts: ts, kind: kind, detail: detail}
	}
}

func (c *apiClient) patchConfig(payload map[string]interface{}) (string, error) {
	data, code, err := c.do("PATCH", "/api/v1/system/config", payload)
	if err != nil {
		return "", err
	}
	if code != 200 {
		return "", fmt.Errorf("config patch failed (HTTP %d): %s", code, string(data))
	}
	var resp map[string]interface{}
	json.Unmarshal(data, &resp)
	if msg, ok := resp["message"].(string); ok {
		return msg, nil
	}
	return "Configuration updated", nil
}

