package tui

// workspace.go — screen 2, the Sandbox Workspace (Build 2, v2-workspace.jsx):
// three focusable panes (files tree · vim-style editor · in-VM terminal) for
// living inside one sandbox. Tab cycles focus; the focused pane is accented.

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	wsFocusTree = iota
	wsFocusEditor
	wsFocusTerm
)

type workspaceState struct {
	sandboxID string
	focus     int
	files     fileState // tree + content + INSERT-mode textarea
	cmdline   string    // vim ":" command buffer
	cmdlineOn bool
	termLines []string
	termInput textinput.Model
	termBusy  bool
}

// openWorkspace enters the Workspace for the selected sandbox.
func (m *Model) openWorkspace() tea.Cmd {
	if len(m.sandboxes) == 0 || m.cursor >= len(m.sandboxes) {
		return nil
	}
	id := m.sandboxes[m.cursor].ID
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "run a command…"
	ti.Focus()
	m.workspace = workspaceState{
		sandboxID: id,
		focus:     wsFocusTree,
		files:     fileState{sandboxID: id, dir: "/workspace"},
		termInput: ti,
		termLines: []string{stDim.Render("# in-VM shell · commands run for real via exec")},
	}
	m.mode = modeWorkspace
	m.activeTab = tabSandboxes
	return m.listFilesCmd(id, "/workspace")
}

func (m Model) findSandbox(id string) (sandboxData, bool) {
	for _, sb := range m.sandboxes {
		if sb.ID == id {
			return sb, true
		}
	}
	return sandboxData{}, false
}

// ── keys ────────────────────────────────────────────────────────────────────

func (m *Model) handleWorkspaceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	ws := &m.workspace

	// Global within the workspace.
	switch key {
	case "tab":
		ws.focus = (ws.focus + 1) % 3
		return m, nil
	case "ctrl+w":
		m.mode = modeNormal
		return m, nil
	}
	if !ws.files.write && !ws.cmdlineOn && ws.focus != wsFocusTerm {
		switch key {
		case "1":
			ws.focus = wsFocusTree
			return m, nil
		case "2":
			ws.focus = wsFocusEditor
			return m, nil
		case "3":
			ws.focus = wsFocusTerm
			return m, nil
		}
	}

	switch ws.focus {
	case wsFocusTree:
		return m.workspaceTreeKey(key)
	case wsFocusEditor:
		return m.workspaceEditorKey(msg)
	case wsFocusTerm:
		return m.workspaceTermKey(msg)
	}
	return m, nil
}

func (m *Model) workspaceTreeKey(key string) (tea.Model, tea.Cmd) {
	f := &m.workspace.files
	switch key {
	case "esc":
		m.mode = modeNormal
	case "j", "down":
		if f.cursor < len(f.nodes)-1 {
			f.cursor++
		}
	case "k", "up":
		if f.cursor > 0 {
			f.cursor--
		}
	case "enter":
		if f.cursor < len(f.nodes) {
			n := f.nodes[f.cursor]
			if n.isDir {
				f.cursor = 0
				return m, m.listFilesCmd(m.workspace.sandboxID, n.fpath)
			}
			f.openPath = n.fpath
			f.write = false
			m.workspace.focus = wsFocusEditor
			return m, m.readFileCmd(m.workspace.sandboxID, n.fpath)
		}
	}
	return m, nil
}

func (m *Model) workspaceEditorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.workspace.files
	key := msg.String()

	// Vim ":" command line.
	if m.workspace.cmdlineOn {
		switch key {
		case "esc":
			m.workspace.cmdlineOn = false
			m.workspace.cmdline = ""
		case "enter":
			cmd := m.workspace.cmdline
			m.workspace.cmdlineOn = false
			m.workspace.cmdline = ""
			return m, m.runVimCommand(cmd)
		case "backspace":
			if len(m.workspace.cmdline) > 1 {
				m.workspace.cmdline = m.workspace.cmdline[:len(m.workspace.cmdline)-1]
			}
		default:
			if len(key) == 1 {
				m.workspace.cmdline += key
			}
		}
		return m, nil
	}

	// INSERT mode: the textarea owns keys.
	if f.write {
		switch key {
		case "esc":
			f.content = f.editor.Value()
			f.write = false
			return m, nil
		}
		var cmd tea.Cmd
		f.editor, cmd = f.editor.Update(msg)
		return m, cmd
	}

	// NORMAL mode.
	switch key {
	case "esc":
		m.mode = modeNormal
	case "i":
		if f.openPath != "" {
			ta := textarea.New()
			ta.SetValue(f.content)
			ta.Focus()
			f.editor = ta
			f.write = true
			return m, textarea.Blink
		}
	case ":":
		m.workspace.cmdlineOn = true
		m.workspace.cmdline = ":"
	}
	return m, nil
}

func (m *Model) workspaceTermKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.mode = modeNormal
		return m, nil
	case "enter":
		cmd := strings.TrimSpace(m.workspace.termInput.Value())
		if cmd == "" {
			return m, nil
		}
		m.workspace.termLines = append(m.workspace.termLines, m.termPrompt()+cmd)
		m.workspace.termInput.SetValue("")
		m.workspace.termBusy = true
		return m, m.execCommand(m.workspace.sandboxID, cmd)
	}
	var cmd tea.Cmd
	m.workspace.termInput, cmd = m.workspace.termInput.Update(msg)
	return m, cmd
}

// runVimCommand handles :w (save), :q (close), :wq.
func (m *Model) runVimCommand(cmd string) tea.Cmd {
	f := &m.workspace.files
	switch strings.TrimPrefix(cmd, ":") {
	case "w":
		if f.openPath != "" {
			content := f.content
			if f.write {
				content = f.editor.Value()
			}
			f.content = content
			return m.writeFileCmd(m.workspace.sandboxID, f.openPath, content)
		}
	case "q":
		m.mode = modeNormal
	case "wq", "x":
		m.mode = modeNormal
		if f.openPath != "" {
			content := f.content
			if f.write {
				content = f.editor.Value()
			}
			return m.writeFileCmd(m.workspace.sandboxID, f.openPath, content)
		}
	}
	return nil
}

func (m Model) termPrompt() string {
	return stHi.Render("stacy") + stSteel.Render("@") + stOK.Render(m.workspace.sandboxID) +
		stSteel.Render(":") + stSteel.Render("/workspace") + stSteel.Render("$ ")
}

// appendTermResult feeds a real exec result into the terminal scrollback.
func (m *Model) appendTermResult(r *execResultData) {
	for _, ln := range strings.Split(strings.TrimRight(r.Stdout, "\n"), "\n") {
		if ln != "" {
			m.workspace.termLines = append(m.workspace.termLines, stDim.Render(ln))
		}
	}
	if r.Stderr != "" {
		for _, ln := range strings.Split(strings.TrimRight(r.Stderr, "\n"), "\n") {
			m.workspace.termLines = append(m.workspace.termLines, stErr.Render(ln))
		}
	}
	m.workspace.termBusy = false
	if len(m.workspace.termLines) > 200 {
		m.workspace.termLines = m.workspace.termLines[len(m.workspace.termLines)-200:]
	}
}

// ── render ────────────────────────────────────────────────────────────────

func (m Model) renderWorkspace(height, width int) string {
	ctx := m.workspaceContextBar(width)

	paneH := height - 2
	if paneH < 8 {
		paneH = 8
	}
	treeW := 32
	if width < 130 {
		treeW = 26
	}
	rightW := width - 1 - treeW

	tree := m.workspaceTree(treeW, paneH)
	editorH := paneH * 3 / 5
	termH := paneH - editorH - 1
	editor := m.workspaceEditor(rightW, editorH)
	term := m.workspaceTerminal(rightW, termH)
	right := lipgloss.JoinVertical(lipgloss.Left, editor, term)
	panes := lipgloss.JoinHorizontal(lipgloss.Top, tree, " ", right)

	return ctx + "\n" + panes
}

func (m Model) workspaceContextBar(width int) string {
	sb, ok := m.findSandbox(m.workspace.sandboxID)
	left := stHiB.Render(glyphChip+" "+m.workspace.sandboxID) + "  "
	if ok {
		left += stateDot(sb.State) + " " + stateStyle(sb.State).Render(sb.State) + "  " +
			stDim.Render(sb.Image) + "  " + stFaint.Render("via "+sb.Provider) + "  " +
			stDim.Render("ttl ") + meter(ttlPct(sb), 6, stHi, false) + " " + stDim.Render(ttlShort(sb.ExpiresAt))
	}
	sel := func(n int, lab string) string {
		if m.workspace.focus == n {
			return stHi.Render(lab)
		}
		return stDim.Render(lab)
	}
	right := sel(0, "1 files") + " · " + sel(1, "2 editor") + " · " + sel(2, "3 terminal") + " · " + stFaint.Render(glyphTab+" cycle")
	return spread(left, right, width)
}

func ttlPct(sb sandboxData) float64 {
	total := sb.ExpiresAt.Sub(sb.CreatedAt)
	if total <= 0 {
		return 0
	}
	p := float64(time.Until(sb.ExpiresAt)) / float64(total) * 100
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func (m Model) workspaceTree(width, height int) string {
	f := m.workspace.files
	var rows []string
	rows = append(rows, stDim.Render(truncate(f.dir, width-4)))
	for i, n := range f.nodes {
		icon := stFaint.Render(glyphTreeFile)
		name := stDim.Render(n.name)
		if n.isDir {
			icon = stHi.Render(glyphTreeClosed)
			name = stInk.Render(n.name)
		}
		if i == f.cursor {
			rows = append(rows, selectedRow(stHi.Render(n.name), width-4))
		} else {
			rows = append(rows, icon+" "+name)
		}
	}
	hint := ""
	if m.workspace.focus == wsFocusTree {
		hint = "FOCUS"
	}
	return panel(glyphPaneFiles+" FILES · lazytree", hint, strings.Join(rows, "\n"), width, m.workspace.focus == wsFocusTree)
}

func (m Model) workspaceEditor(width, height int) string {
	f := m.workspace.files
	focused := m.workspace.focus == wsFocusEditor
	title := glyphPaneEditor + " " + orDash(f.openPath)

	var body string
	if f.openPath == "" {
		body = stFaint.Render("open a file from the tree (↵)")
	} else if f.write {
		f.editor.SetWidth(width - 6)
		f.editor.SetHeight(max(3, height-6))
		body = f.editor.View()
	} else {
		lines := strings.Split(strings.TrimRight(f.content, "\n"), "\n")
		maxLines := height - 6
		if maxLines < 2 {
			maxLines = 2
		}
		var b strings.Builder
		for i, ln := range lines {
			if i >= maxLines {
				b.WriteString(stFaint.Render("  …\n"))
				break
			}
			b.WriteString(stFaint.Render(padLeft(itoa(i+1), 3)) + "  " + highlightLine(ln) + "\n")
		}
		body = strings.TrimRight(b.String(), "\n")
	}

	// Modeline.
	var badge string
	if f.write {
		badge = lipgloss.NewStyle().Foreground(colBg).Background(colGreen).Render(" -- INSERT -- ")
	} else {
		badge = lipgloss.NewStyle().Foreground(colBg).Background(colOrange).Render(" NORMAL ")
	}
	mode := stDim.Render(" " + filename(f.openPath) + " · utf-8 · unix")
	right := stDim.Render("i insert · :w save · :q close")
	if m.workspace.cmdlineOn {
		right = stHi.Render(m.workspace.cmdline) + cursorBar(m.cursorOn)
	}
	modeline := spread(badge+mode, right, width-4)

	hint := ""
	if focused {
		hint = "FOCUS"
	}
	return panel(title, hint, body+"\n"+modeline, width, focused)
}

func (m Model) workspaceTerminal(width, height int) string {
	focused := m.workspace.focus == wsFocusTerm
	maxLines := height - 6
	if maxLines < 2 {
		maxLines = 2
	}
	start := len(m.workspace.termLines) - maxLines
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	for _, ln := range m.workspace.termLines[start:] {
		b.WriteString(ln + "\n")
	}
	// Active prompt line.
	prompt := m.termPrompt()
	if focused {
		prompt += m.workspace.termInput.View() + cursorBar(m.cursorOn)
	} else {
		prompt += stFaint.Render("(press 3 to focus)")
	}
	b.WriteString(prompt)

	hint := ""
	if focused {
		hint = "FOCUS"
	}
	return panel(glyphPaneTerm+" TERMINAL · "+m.workspace.sandboxID+" (in-VM)", hint, b.String(), width, focused)
}

func filename(p string) string {
	if p == "" {
		return "—"
	}
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func cursorBar(on bool) string {
	if on {
		return stHi.Render("▏")
	}
	return " "
}
