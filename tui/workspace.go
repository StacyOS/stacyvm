package tui

// workspace.go — screen 2, the Sandbox Workspace (Build 2, v2-workspace.jsx):
// three focusable panes (files tree · vim-style editor · in-VM terminal) for
// living inside one sandbox. Tab cycles focus; the focused pane is accented.

import (
	"path"
	"strings"
	"time"

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
	files     fileState // tree + current dir + open file content
	editor    Editor    // the modal editor for the open file
	termLines []string
	termInput textinput.Model
	termBusy  bool
	showTerm  bool // terminal pane visible (toggle with ctrl+t)
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
	ed := NewTextareaEditor()
	m.workspace = workspaceState{
		sandboxID: id,
		focus:     wsFocusTree,
		files:     fileState{sandboxID: id, dir: "/workspace"},
		editor:    ed,
		termInput: ti,
		showTerm:  true,
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

// setWSFocus moves pane focus and keeps the editor's textarea focus in sync.
func (m *Model) setWSFocus(n int) {
	ws := &m.workspace
	ws.focus = n
	if ws.editor == nil {
		return
	}
	if n == wsFocusEditor {
		ws.editor.Focus()
	} else {
		ws.editor.Blur()
	}
}

func (m *Model) handleWorkspaceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	ws := &m.workspace

	// Pane focus ring (consistent across multi-pane screens).
	switch key {
	case "tab":
		next := (ws.focus + 1) % m.wsPaneCount()
		m.setWSFocus(next)
		return m, nil
	case "shift+tab":
		next := (ws.focus - 1 + m.wsPaneCount()) % m.wsPaneCount()
		m.setWSFocus(next)
		return m, nil
	case "ctrl+w":
		m.mode = modeNormal
		return m, nil
	case "ctrl+t":
		ws.showTerm = !ws.showTerm
		if !ws.showTerm && ws.focus == wsFocusTerm {
			m.setWSFocus(wsFocusEditor)
		}
		return m, nil
	}

	// Direct pane jumps (workspace-only; global 1-6 screen switching is
	// suspended while the workspace owns keys). Editor keeps digits in INSERT.
	inInsert := false
	if me, ok := ws.editor.(modalEditor); ok && me.Mode() == editorInsert {
		inInsert = true
	}
	if !inInsert {
		switch key {
		case "1":
			m.setWSFocus(wsFocusTree)
			return m, nil
		case "2":
			m.setWSFocus(wsFocusEditor)
			return m, nil
		case "3":
			if ws.showTerm {
				m.setWSFocus(wsFocusTerm)
			}
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

// wsPaneCount is 3 with the terminal visible, else 2 (tree + editor).
func (m Model) wsPaneCount() int {
	if m.workspace.showTerm {
		return 3
	}
	return 2
}

func (m *Model) workspaceTreeKey(key string) (tea.Model, tea.Cmd) {
	f := &m.workspace.files
	switch key {
	case "esc":
		m.mode = modeNormal
	case "-", "h", "left":
		parent := path.Dir(f.dir)
		f.cursor = 0
		return m, m.listFilesCmd(m.workspace.sandboxID, parent)
	case "R":
		return m, m.listFilesCmd(m.workspace.sandboxID, f.dir)
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
			m.setWSFocus(wsFocusEditor)
			return m, m.readFileCmd(m.workspace.sandboxID, n.fpath)
		}
	}
	return m, nil
}

func (m *Model) workspaceEditorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ws := &m.workspace
	switch msg.String() {
	case "ctrl+s":
		if ws.files.openPath == "" {
			return m, nil
		}
		content := ws.editor.Value()
		ws.files.content = content
		return m, m.writeFileCmd(ws.sandboxID, ws.files.openPath, content)
	case "esc":
		// Esc returns INSERT->NORMAL inside the editor; a second Esc (in
		// NORMAL) leaves the workspace.
		if me, ok := ws.editor.(modalEditor); ok && me.Mode() == editorInsert {
			return m, ws.editor.Update(msg)
		}
		m.mode = modeNormal
		return m, nil
	}
	return m, ws.editor.Update(msg)
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
	// panelH reserves 2 border rows + 1 title row; treeRows fills the rest.
	rows := treeRows(&f, width, height-3)
	hint := ""
	if m.workspace.focus == wsFocusTree {
		hint = "FOCUS"
	}
	return panelH(glyphPaneFiles+" FILES · netrw", hint,
		strings.Join(rows, "\n"), width, height, m.workspace.focus == wsFocusTree)
}

func (m Model) workspaceEditor(width, height int) string {
	ws := m.workspace
	focused := ws.focus == wsFocusEditor
	title := glyphPaneEditor + " " + orDash(ws.files.openPath)

	// Reserve 2 rows inside the box for the title + modeline; editor fills rest.
	editorH := height - 4
	if editorH < 1 {
		editorH = 1
	}
	var body string
	if ws.files.openPath == "" {
		body = stFaint.Render("open a file from the tree (↵)")
		body = padLines(body, editorH)
	} else {
		ws.editor.SetSize(width-4, editorH)
		body = ws.editor.View()
	}

	mode := editorNormal
	if me, ok := ws.editor.(modalEditor); ok {
		mode = me.Mode()
	}
	var badge, hints string
	if mode == editorInsert {
		badge = lipgloss.NewStyle().Foreground(colBg).Background(colGreen).Render(" -- INSERT -- ")
		hints = stDim.Render("type to edit · esc normal")
	} else {
		badge = lipgloss.NewStyle().Foreground(colBg).Background(colOrange).Render(" NORMAL ")
		hints = stDim.Render("i insert · ^s save · esc back")
	}
	info := stDim.Render(" " + filename(ws.files.openPath) + " · utf-8 · unix")
	modeline := spread(badge+info, hints, width-4)

	hint := ""
	if focused {
		hint = "FOCUS"
	}
	return panelH(title, hint, body+"\n"+modeline, width, height, focused)
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
