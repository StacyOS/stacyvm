package tui

// logs.go — screen 11, "Logs" (Build 1 A): the full, following, color-coded
// event stream behind the dashboard's compact module. Fed by the real SSE bus.

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

var logKinds = []string{"", "SPAWN", "EXEC", "WRITE", "TEMPLATE", "KILL", "CONFIG"}

func (m *Model) handleLogsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "/", "g":
		// cycle the KIND filter
		cur := 0
		for i, k := range logKinds {
			if k == m.logFilter {
				cur = i
				break
			}
		}
		m.logFilter = logKinds[(cur+1)%len(logKinds)]
	case "c":
		m.statusMsg = fmt.Sprintf("copied %d events", len(m.filteredEvents()))
	}
	return m, nil
}

func (m Model) filteredEvents() []eventEntry {
	if m.logFilter == "" {
		return m.events
	}
	out := make([]eventEntry, 0, len(m.events))
	for _, e := range m.events {
		if e.kind == m.logFilter {
			out = append(out, e)
		}
	}
	return out
}

func (m Model) viewLogs(height, width int) string {
	inner := width - 4
	evs := m.filteredEvents()

	var rows []string
	if len(evs) == 0 {
		rows = append(rows, stFaint.Render("waiting for events…"))
	}
	// Show the most recent events that fit.
	maxRows := height - 4
	if maxRows < 3 {
		maxRows = 3
	}
	start := len(evs) - maxRows
	if start < 0 {
		start = 0
	}
	for _, e := range evs[start:] {
		ts := stFaint.Render(e.ts.Format("15:04:05"))
		kind := kindStyle(e.kind).Render(padRight(e.kind, 9))
		detail := stDim.Render(truncate(e.detail, max(1, inner-22)))
		rows = append(rows, ts+"  "+kind+" "+detail)
	}

	filt := "all"
	if m.logFilter != "" {
		filt = m.logFilter
	}
	hint := glyphDotLive + " following · filter: " + filt + " · " + itoa(len(evs)) + " events"
	return panel("EVENT STREAM", hint, strings.Join(rows, "\n"), width, false)
}
