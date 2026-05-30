package tui

// sandboxes.go — screen 5, "Sandboxes" (Build 1 Direction A): a FLEET table on
// the left and a live inspect drawer on the right. Also the dispatch point for
// the sandbox sub-screens (spawn modal, animated spawn, exec, files).

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) viewSandboxes(height, width int) string {
	switch m.mode {
	case modeSpawn:
		return m.renderSpawnModal(height, width)
	case modeSpawning:
		return m.renderSpawnSequence(height, width)
	case modeExec:
		return m.renderExec(height, width)
	case modeInput:
		return m.renderFiles(height, width)
	}

	// Normal: FLEET list + inspect drawer.
	leftW := (width - 1) * 14 / 24
	rightW := width - 1 - leftW
	left := m.fleetList(leftW)
	right := m.inspectDrawer(rightW)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

func (m Model) fleetList(width int) string {
	inner := width - 4
	wID, wState, wTTL := 11, 12, 6
	wImage := inner - wID - wState - wTTL - 3
	if wImage < 8 {
		wImage = 8
	}
	if wImage > 26 {
		wImage = 26
	}

	header := stLabel.Render(
		padRight("ID", wID) + " " + padRight("STATE", wState) + " " +
			padRight("IMAGE", wImage) + " " + padRight("TTL", wTTL))

	rows := []string{header}
	if len(m.sandboxes) == 0 {
		rows = append(rows, stDim.Render("no sandboxes — press s to spawn"))
	}
	for i, sb := range m.sandboxes {
		stateCell := stateDot(sb.State) + " " + stateStyle(sb.State).Render(sb.State)
		id := stBold.Render(sb.ID)
		if i == m.cursor {
			id = stHiB.Render(sb.ID)
		}
		row := padRight(id, wID) + " " +
			padRight(stateCell, wState) + " " +
			padRight(stDim.Render(truncate(sb.Image, wImage)), wImage) + " " +
			padRight(stDim.Render(ttlShort(sb.ExpiresAt)), wTTL)
		if i == m.cursor {
			rows = append(rows, selectedRow(row, inner))
		} else {
			rows = append(rows, row)
		}
	}
	return panel("FLEET", fmt.Sprintf("%d total", len(m.sandboxes)), strings.Join(rows, "\n"), width, false)
}

func (m Model) inspectDrawer(width int) string {
	if len(m.sandboxes) == 0 || m.cursor >= len(m.sandboxes) {
		return panel("INSPECT", "", stFaint.Render("select a sandbox"), width, true)
	}
	sb := m.sandboxes[m.cursor]
	inner := width - 4

	kv := func(k, v string) string {
		return stDim.Render(padRight(k, 10)) + v
	}

	created := sb.CreatedAt.Format("15:04:05")
	ago := humanizeSince(sb.CreatedAt)

	// Expires meter: fraction of the lease still remaining.
	expPct := 0.0
	total := sb.ExpiresAt.Sub(sb.CreatedAt)
	if total > 0 {
		expPct = float64(time.Until(sb.ExpiresAt)) / float64(total) * 100
		if expPct < 0 {
			expPct = 0
		}
	}

	lines := []string{
		kv("state", stateDot(sb.State)+" "+stateStyle(sb.State).Render(sb.State)),
		kv("image", stInk.Render(sb.Image)),
		kv("provider", stInk.Render(sb.Provider)),
		kv("created", stDim.Render(created+" · "+ago+" ago")),
		kv("expires", meter(expPct, 10, stHi, false)+" "+stDim.Render(ttlShort(sb.ExpiresAt))),
		"",
	}

	// Live CPU / MEM meters from real per-sandbox stats.
	st, ok := m.sbStats[sb.ID]
	if ok && st.supported {
		lines = append(lines,
			kv("CPU", meterFor(st.cpuPct, 10, true)),
			kv("MEM", meter(st.memPct(), 10, stOK, true)),
		)
	} else {
		lines = append(lines,
			kv("CPU", stFaint.Render("—")),
			kv("MEM", stFaint.Render("—")),
		)
	}

	lines = append(lines, "", keyHints([]hint{
		{"e", "exec"}, {"f", "files"}, {"l", "logs"}, {"d", "kill"},
	}))

	_ = inner
	return panel(glyphInspect+" "+sb.ID, "INSPECT", strings.Join(lines, "\n"), width, true)
}

func humanizeSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
