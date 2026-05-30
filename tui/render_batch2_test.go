package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestNoAnsiLeak guards against the styled-in-styled bug where an embedded
// escape's ESC byte is stripped, leaking a bare "[38;2;..m" into visible text.
// Every "[38;2;" must be part of a real escape sequence "\x1b[38;2;".
func TestNoAnsiLeak(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor) // force color so the leak reproduces
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) }) // restore for other tests
	m := seedModel()
	for _, tb := range []tab{tabDashboard, tabSandboxes, tabTemplates, tabProviders, tabLogs, tabConfig} {
		m.activeTab = tb
		out := m.View()
		total := strings.Count(out, "[38;2;")
		real := strings.Count(out, "\x1b[38;2;")
		if total != real {
			t.Errorf("tab %d: %d bare '[38;2;' leaked (total=%d real=%d)", tb, total-real, total, real)
		}
	}
}

func TestBatch2Renders(t *testing.T) {
	// Providers cards
	m := seedModel()
	m.activeTab = tabProviders
	m.provLatency["docker"] = newRing(8)
	m.provLatency["docker"].push(12)
	m.provLatency["firecracker"] = newRing(8)
	m.provLatency["firecracker"].push(8)
	m.providerDetails["docker"] = providerDetailData{Name: "docker", SandboxCount: 4, Config: map[string]string{"type": "docker", "runtime": "runsc"}}
	m.providerDetails["firecracker"] = providerDetailData{Name: "firecracker", SandboxCount: 1, Config: map[string]string{"type": "firecracker"}}
	snap(t, "providers", m.View(), "DOCKER", "runtime", "latency", "sandboxes")

	// Logs event stream
	m = seedModel()
	m.activeTab = tabLogs
	m.events = []eventEntry{
		{ts: time.Now(), kind: "SPAWN", detail: "sb-7f3a91 docker python:3.12"},
		{ts: time.Now(), kind: "EXEC", detail: "sb-2c8e04 exit=0"},
		{ts: time.Now(), kind: "WRITE", detail: "sb-7f3a91 /workspace/main.py"},
	}
	snap(t, "logs", m.View(), "EVENT STREAM", "SPAWN", "following")

	// Config segmented controls
	m = seedModel()
	m.activeTab = tabConfig
	m.configCursor = 4 // runsc
	snap(t, "config", m.View(), "PROVIDERS", "default provider", "docker runtime", "SERVER")

	// Command palette overlay
	m = seedModel()
	m.activeTab = tabDashboard
	m.paletteOpen = true
	m.paletteQuery = "spa"
	snap(t, "palette", m.View(), "COMMAND", "Spawn")

	// Boot splash
	m = seedModel()
	m.booting = true
	m.bootProg = 60
	snap(t, "boot", m.View(), "S T A C Y V M", "orchestrator", "handshake")

	// Workspace
	m = seedModel()
	m.activeTab = tabSandboxes
	m.mode = modeWorkspace
	m.workspace = workspaceState{
		sandboxID: "sb-7f3a91",
		focus:     wsFocusEditor,
		files: fileState{
			sandboxID: "sb-7f3a91", dir: "/workspace", openPath: "/workspace/main.py",
			content: "import os\n\ndef main():\n    print(\"hi\")  # go\n",
			nodes:   []fileNode{{name: "src", fpath: "/workspace/src", isDir: true}, {name: "main.py", fpath: "/workspace/main.py"}},
		},
		termLines: []string{"$ python -m pytest -q", "24 passed in 1.83s"},
	}
	snap(t, "workspace", m.View(), "FILES", "TERMINAL", "NORMAL", "sb-7f3a91")
}
