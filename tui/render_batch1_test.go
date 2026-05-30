package tui

import (
	"os"
	"strings"
	"testing"
	"time"
)

func seedModel() Model {
	m := NewModel("http://localhost:7423", "")
	m.width, m.height = 198, 52
	exp := time.Now().Add(24 * time.Minute)
	m.sandboxes = []sandboxData{
		{ID: "sb-7f3a91", State: "running", Provider: "docker", Image: "python:3.12-alpine", MemoryMB: 1024, VCPUs: 1, CreatedAt: time.Now().Add(-24 * time.Minute), ExpiresAt: exp},
		{ID: "sb-2c8e04", State: "running", Provider: "docker", Image: "node:20", ExpiresAt: exp},
		{ID: "sb-9d11ba", State: "creating", Provider: "firecracker", Image: "alpine:latest", ExpiresAt: exp},
	}
	m.sbStats["sb-7f3a91"] = sandboxStat{cpuPct: 71, memBytes: 480 << 20, memLimit: 1024 << 20, supported: true}
	m.providerList = []providerData{
		{Name: "docker", IsDefault: true, Healthy: true, LatencyMS: 12},
		{Name: "firecracker", Healthy: false, LatencyMS: 8},
	}
	m.templateList = []templateData{
		{Name: "data-science", Image: "python:3.12", MemoryMB: 512, CPUCores: 1, TTLSeconds: 300, PoolSize: 3, Description: "Python 3.12 with numpy, pandas, scikit-learn pre-warmed."},
		{Name: "web-build", Image: "node:20", MemoryMB: 1024, CPUCores: 2, TTLSeconds: 300, PoolSize: 0},
	}
	m.host = hostSnapshot{cpuPct: 34, memPct: 61, diskPct: 22, ok: true}
	m.health = &healthData{Version: "0.9.2", Uptime: "4h2m"}
	return m
}

func snap(t *testing.T, name, out string, wants ...string) {
	t.Helper()
	if dir := os.Getenv("STACY_SNAPSHOT_DIR"); dir != "" {
		_ = os.WriteFile(dir+"/"+name+".txt", []byte(out), 0o644)
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("%s: missing %q", name, w)
		}
	}
}

func TestBatch1Renders(t *testing.T) {
	// Sandboxes list + inspect drawer
	m := seedModel()
	m.activeTab = tabSandboxes
	snap(t, "sandboxes", m.View(), "FLEET", "INSPECT", "sb-7f3a91", "exec")

	// Spawn modal
	m = seedModel()
	m.activeTab = tabSandboxes
	m.mode = modeSpawn
	m.inputs[0].SetValue("python:3.12-alpine")
	snap(t, "spawn-modal", m.View(), "SPAWN SANDBOX", "provider", "docker", "template")

	// Animated spawn sequence (mid-flight)
	m = seedModel()
	m.activeTab = tabSandboxes
	m.mode = modeSpawning
	m.spawn = spawnState{active: true, phase: 1, progress: 42, phaseStart: time.Now(), req: spawnReq{image: "python:3.12-alpine", provider: "docker", ttl: "30m"}}
	snap(t, "spawn-seq", m.View(), "PROVISIONING", "SEQUENCE", "pull image", "SPAWN REQUEST")

	// Exec with framed output
	m = seedModel()
	m.activeTab = tabSandboxes
	m.mode = modeExec
	m.lastExecCmd = "python -m pytest -q"
	m.lastExec = &execResultData{ExitCode: 0, Stdout: "........ [100%]\n24 passed in 1.83s", Duration: "1.9s"}
	snap(t, "exec", m.View(), "EXEC", "exit", "1.9s")

	// Files browser (READ mode)
	m = seedModel()
	m.activeTab = tabSandboxes
	m.mode = modeInput
	m.files = fileState{sandboxID: "sb-7f3a91", dir: "/workspace", openPath: "/workspace/main.py",
		content: "import os\n\ndef main():\n    print(\"hello\")  # entry\n",
		nodes: []fileNode{{name: "src", fpath: "/workspace/src", isDir: true}, {name: "main.py", fpath: "/workspace/main.py"}}}
	snap(t, "files", m.View(), "TREE", "READ mode", "main.py")

	// Templates table + detail
	m = seedModel()
	m.activeTab = tabTemplates
	snap(t, "templates", m.View(), "TEMPLATES", "POOL", "data-science")

	// Create-template modal
	m = seedModel()
	m.activeTab = tabTemplates
	m.mode = modeCreateTemplate
	snap(t, "tmpl-create", m.View(), "CREATE TEMPLATE", "name", "image")
}
