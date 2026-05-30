package tui

// palette.go — the global ⌘K / Ctrl+K command palette: a fuzzy jump to any
// screen or action, overlaid centered on the current screen.

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type paletteItem struct {
	label string
	run   func(m *Model) tea.Cmd
}

func (m Model) paletteAllItems() []paletteItem {
	return []paletteItem{
		{"Dashboard", func(m *Model) tea.Cmd { m.activeTab = tabDashboard; return nil }},
		{"Sandboxes", func(m *Model) tea.Cmd { m.activeTab = tabSandboxes; return nil }},
		{"Templates", func(m *Model) tea.Cmd { m.activeTab = tabTemplates; return nil }},
		{"Providers", func(m *Model) tea.Cmd { m.activeTab = tabProviders; return nil }},
		{"Logs", func(m *Model) tea.Cmd { m.activeTab = tabLogs; return nil }},
		{"Config", func(m *Model) tea.Cmd { m.activeTab = tabConfig; return nil }},
		{"Spawn sandbox", func(m *Model) tea.Cmd { m.activeTab = tabSandboxes; return m.openSpawnModal() }},
		{"Exec in selected", func(m *Model) tea.Cmd { m.activeTab = tabSandboxes; return m.openExec() }},
		{"Files of selected", func(m *Model) tea.Cmd { m.activeTab = tabSandboxes; return m.openFiles() }},
		{"Open workspace", func(m *Model) tea.Cmd { return m.openWorkspace() }},
		{"Kill selected", func(m *Model) tea.Cmd { m.confirmKill(); return nil }},
		{"Refresh", func(m *Model) tea.Cmd {
			return tea.Batch(m.fetchSandboxes(), m.fetchHealth(), m.fetchProviders(), m.fetchTemplates())
		}},
	}
}

func (m Model) paletteFiltered() []paletteItem {
	all := m.paletteAllItems()
	q := strings.ToLower(strings.TrimSpace(m.paletteQuery))
	if q == "" {
		return all
	}
	out := make([]paletteItem, 0, len(all))
	for _, it := range all {
		if strings.Contains(strings.ToLower(it.label), q) {
			out = append(out, it)
		}
	}
	return out
}

func (m *Model) openPalette() {
	m.paletteOpen = true
	m.paletteQuery = ""
	m.paletteCursor = 0
}

func (m *Model) handlePaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.paletteFiltered()
	switch msg.String() {
	case "esc", "ctrl+k":
		m.paletteOpen = false
		return m, nil
	case "up", "ctrl+p":
		if m.paletteCursor > 0 {
			m.paletteCursor--
		}
	case "down", "ctrl+n":
		if m.paletteCursor < len(items)-1 {
			m.paletteCursor++
		}
	case "enter":
		m.paletteOpen = false
		if m.paletteCursor < len(items) {
			return m, items[m.paletteCursor].run(m)
		}
	case "backspace":
		if len(m.paletteQuery) > 0 {
			m.paletteQuery = m.paletteQuery[:len(m.paletteQuery)-1]
			m.paletteCursor = 0
		}
	default:
		if len(msg.String()) == 1 {
			m.paletteQuery += msg.String()
			m.paletteCursor = 0
		}
	}
	return m, nil
}

func (m Model) renderPaletteOverlay(width, height int) string {
	boxW := 56
	items := m.paletteFiltered()

	cursorBlink := " "
	if m.cursorOn {
		cursorBlink = stHi.Render("▏")
	}
	queryLine := stHi.Render(glyphCmd+"K ") + stInk.Render(m.paletteQuery) + cursorBlink

	var rows []string
	rows = append(rows, queryLine, stFaint.Render(strings.Repeat("─", boxW-2)))
	maxItems := 10
	for i, it := range items {
		if i >= maxItems {
			break
		}
		if i == m.paletteCursor {
			rows = append(rows, stHi.Render("▸ "+it.label))
		} else {
			rows = append(rows, stDim.Render("  "+it.label))
		}
	}
	if len(items) == 0 {
		rows = append(rows, stFaint.Render("  no matches"))
	}
	rows = append(rows, "", keyHints([]hint{{glyphEnter, "run"}, {"esc", "close"}}))

	box := panel("COMMAND", "", strings.Join(rows, "\n"), boxW, true)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
