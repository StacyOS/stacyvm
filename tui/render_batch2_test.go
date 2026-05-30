package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// TestNoAnsiLeak guards against the styled-in-styled nesting bug: when a
// pre-styled fragment is wrapped inside another Background/Underline style,
// this lipgloss version re-styles the inner escape's bytes as individual cells,
// leaking the literal characters of the escape (e.g. "[38;2;..m2[0m") into the
// VISIBLE text. We strip the real escapes and assert no such fragment remains.
func TestNoAnsiLeak(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor) // force color so the leak reproduces
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	m := seedModel()
	for _, tb := range []tab{tabDashboard, tabSandboxes, tabTemplates, tabProviders, tabLogs, tabConfig} {
		m.activeTab = tb
		visible := ansi.Strip(m.View())
		for _, frag := range []string{"[38;2", "[48;2"} {
			if strings.Contains(visible, frag) {
				t.Errorf("tab %d: leaked escape fragment %q into visible text:\n%s", tb, frag, visible)
			}
		}
	}
}

func TestPanelHFillsHeight(t *testing.T) {
	// A one-line body in a 10-row panel must still produce a 10-row box.
	out := panelH("TITLE", "", "one line", 40, 10, false)
	if got := lipgloss.Height(out); got != 10 {
		t.Errorf("panelH height = %d, want 10\n%s", got, out)
	}
	// Over-long bodies are truncated to fit, not overflow.
	body := strings.Repeat("x\n", 50)
	out = panelH("TITLE", "", body, 40, 8, false)
	if got := lipgloss.Height(out); got != 8 {
		t.Errorf("panelH height (truncate) = %d, want 8", got)
	}
}

func TestWorkspaceRenders(t *testing.T) {
	m := seedModel()
	m.cursor = 0
	if cmd := m.openWorkspace(); cmd != nil {
		_ = cmd // listFiles cmd not run in test
	}
	m.workspace.files.openPath = "/workspace/main.py"
	m.workspace.editor.SetContent("print('hi')\n")
	m.setWSFocus(wsFocusEditor)
	out := m.View()
	for _, want := range []string{"FILES", "TERMINAL", "NORMAL", "main.py", "^s save"} {
		if !strings.Contains(out, want) {
			t.Errorf("workspace render missing %q", want)
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
	ed := NewTextareaEditor()
	ed.SetContent("import os\n\ndef main():\n    print(\"hi\")  # go\n")
	m.workspace = workspaceState{
		sandboxID: "sb-7f3a91",
		focus:     wsFocusEditor,
		editor:    ed,
		showTerm:  true,
		files: fileState{
			sandboxID: "sb-7f3a91", dir: "/workspace", openPath: "/workspace/main.py",
			nodes: []fileNode{{name: "src", fpath: "/workspace/src", isDir: true}, {name: "main.py", fpath: "/workspace/main.py"}},
		},
		termLines: []string{"$ python -m pytest -q", "24 passed in 1.83s"},
	}
	snap(t, "workspace", m.View(), "FILES", "TERMINAL", "NORMAL", "sb-7f3a91")
}
