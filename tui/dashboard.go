package tui

// dashboard.go — screen 1, "Mission Control" (Build 2, v2-dashboard.jsx).
// KPI strip + two-column body: ACTIVE SANDBOXES table (left) and stacked
// HOST TELEMETRY / PROVIDERS / EVENT STREAM modules (right). All real data.

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) viewDashboard(height, width int) string {
	kpi := m.dashKPIStrip(width)

	// Two-column body: ~1.55fr / 1fr with a one-cell gap.
	leftW := (width - 1) * 155 / 255
	rightW := width - 1 - leftW

	left := m.dashActiveSandboxes(leftW)
	right := m.dashRightColumn(rightW)
	bodyCols := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)

	return kpi + "\n\n" + bodyCols
}

// ── KPI strip ──────────────────────────────────────────────────────────────

func (m Model) dashKPIStrip(width int) string {
	running, booting := 0, 0
	for _, sb := range m.sandboxes {
		switch sb.State {
		case "running":
			running++
		case "creating":
			booting++
		}
	}
	healthy := 0
	provNames := make([]string, 0, len(m.providerList))
	for _, p := range m.providerList {
		if p.Healthy {
			healthy++
			provNames = append(provNames, p.Name)
		}
	}
	warm := 0
	for _, t := range m.templateList {
		warm += t.PoolSize
	}

	tileW := (width - 3) / 4

	sbNote := fmt.Sprintf("%d running · %d booting", running, booting)
	tmplNote := fmt.Sprintf("%d warm in pool", warm)
	provNote := strings.Join(provNames, " · ")
	if provNote == "" {
		provNote = stFaint.Render("—")
	}

	uptimeVal, uptimeNote := "—", "—"
	if m.health != nil {
		uptimeVal = uptimeShort(m.health.Uptime)
		if since, ok := uptimeSince(m.health.Uptime); ok {
			uptimeNote = "since " + since.Format("15:04")
		}
	}

	tiles := []string{
		m.kpiTile(tileW, "SANDBOXES", itoa(running), stOK.Bold(true), sbNote, m.kpiSB, stOK, true),
		m.kpiTile(tileW, "TEMPLATES", itoa(len(m.templateList)), stHiB, tmplNote, m.kpiTmpl, stHi, false),
		m.kpiTile(tileW, "PROVIDERS", fmt.Sprintf("%d/%d", healthy, len(m.providerList)), stOK.Bold(true), provNote, m.kpiProv, stOK, false),
		m.kpiTile(tileW, "UPTIME", uptimeVal, stBold, uptimeNote, m.teleLOAD, stOK, false),
	}
	return joinRow(tiles, width)
}

func (m Model) kpiTile(width int, title, metric string, metricSt lipgloss.Style, note string, r *ring, sparkSt lipgloss.Style, accent bool) string {
	inner := width - 2
	metricRow := spread(metricSt.Render(metric), spark(r.slice(), 8, sparkSt), inner)
	noteRow := stFaint.Render(truncate(note, inner))
	body := metricRow + "\n" + noteRow
	return bracketFrame(title, "", body, width, accent)
}

// ── left: ACTIVE SANDBOXES table ─────────────────────────────────────────

func (m Model) dashActiveSandboxes(width int) string {
	inner := width - 4 // panel border + padding

	// Column widths within the module.
	wID, wState, wProv, wCPU, wTTL := 10, 11, 12, 7, 5
	wImage := inner - wID - wState - wProv - wCPU - wTTL - 5 // gaps
	if wImage < 6 {
		wImage = 6
	}
	if wImage > 24 { // keep the table compact; don't stretch the image column
		wImage = 24
	}

	header := stLabel.Render(
		padRight("ID", wID) + " " + padRight("STATE", wState) + " " +
			padRight("PROVIDER", wProv) + " " + padRight("IMAGE", wImage) + " " +
			padRight("CPU", wCPU) + " " + padRight("TTL", wTTL))

	var rows []string
	rows = append(rows, header)
	if len(m.sandboxes) == 0 {
		rows = append(rows, stDim.Render("no sandboxes — press s to spawn"))
	}
	for i, sb := range m.sandboxes {
		stateCell := stateDot(sb.State) + " " + stateStyle(sb.State).Render(sb.State)
		row := padRight(selText(m, i, sb.ID), wID) + " " +
			padRight(stateCell, wState) + " " +
			padRight(stDim.Render(sb.Provider), wProv) + " " +
			padRight(stDim.Render(truncate(sb.Image, wImage)), wImage) + " " +
			padRight(m.cpuCell(sb, wCPU), wCPU) + " " +
			padRight(stDim.Render(ttlShort(sb.ExpiresAt)), wTTL)
		if i == m.cursor {
			rows = append(rows, selectedRow(row, inner))
		} else {
			rows = append(rows, row)
		}
	}

	keyrow := keyHints([]hint{
		{"s", "spawn"}, {"e", "exec"}, {"f", "files"}, {"d", "kill"}, {glyphEnter, "open workspace"},
	})
	body := strings.Join(rows, "\n") + "\n\n" + keyrow
	return panel("ACTIVE SANDBOXES", glyphEnter+" OPEN WORKSPACE", body, width, true)
}

// selText renders an ID cell, orange when it's the selected row.
func selText(m Model, i int, id string) string {
	if i == m.cursor {
		return stHiB.Render(id)
	}
	return stBold.Render(id)
}

// selectedRow tints a fully-composed row: leading orange bar + faint-orange bg.
func selectedRow(row string, inner int) string {
	bar := stHi.Render("▌")
	return bar + lipgloss.NewStyle().Background(colNavActiveBg).Render(padRight(row, inner-1))
}

func (m Model) cpuCell(sb sandboxData, w int) string {
	if sb.State != "running" {
		return stFaint.Render("—")
	}
	st, ok := m.sbStats[sb.ID]
	if !ok || !st.supported {
		return stFaint.Render("—")
	}
	return meterFor(st.cpuPct, 6, false)
}

// ── right column: HOST TELEMETRY / PROVIDERS / EVENT STREAM ───────────────

func (m Model) dashRightColumn(width int) string {
	host := m.dashHostTelemetry(width)
	provs := m.dashProviders(width)
	events := m.dashEventStream(width)
	return lipgloss.JoinVertical(lipgloss.Left, host, provs, events)
}

func (m Model) dashHostTelemetry(width int) string {
	h := m.host
	lines := []string{
		stDim.Render("CPU  ") + meter(h.cpuPct, 12, meterFillFor(h.cpuPct), true),
		stDim.Render("MEM  ") + meter(h.memPct, 12, meterFillFor(h.memPct), true),
		stDim.Render("DISK ") + meter(h.diskPct, 12, stOK, true),
		stDim.Render("NET  ") + spark(m.teleNET.slice(), 8, stHi) + " " + stOK.Render(h.netRate()),
	}
	hint := "3s"
	if !h.ok {
		hint = stFaint.Render("—")
	}
	return panel("HOST TELEMETRY", hint, strings.Join(lines, "\n"), width, false)
}

func meterFillFor(v float64) lipgloss.Style {
	if v > 60 {
		return stHi
	}
	return stOK
}

func (m Model) dashProviders(width int) string {
	if len(m.providerList) == 0 {
		return panel("PROVIDERS", "", stFaint.Render("no providers"), width, false)
	}
	var lines []string
	for _, p := range m.providerList {
		state := "running"
		if !p.Healthy {
			state = "idle"
		}
		meta := []string{}
		if p.IsDefault {
			meta = append(meta, "default")
		}
		if p.LatencyMS > 0 {
			meta = append(meta, fmt.Sprintf("%dms", p.LatencyMS))
		}
		metaStr := ""
		if len(meta) > 0 {
			metaStr = " " + stFaint.Render("· "+strings.Join(meta, " · "))
		}
		lines = append(lines, stateDot(state)+" "+stInk.Render(p.Name)+metaStr)
	}
	return panel("PROVIDERS", "", strings.Join(lines, "\n"), width, false)
}

func (m Model) dashEventStream(width int) string {
	inner := width - 4
	var lines []string
	n := len(m.events)
	start := n - 5
	if start < 0 {
		start = 0
	}
	if n == 0 {
		lines = append(lines, stFaint.Render("waiting for events…"))
	}
	for _, e := range m.events[start:] {
		ts := stFaint.Render(e.ts.Format("15:04:05"))
		kind := kindStyle(e.kind).Render(padRight(e.kind, 8))
		detail := stDim.Render(truncate(e.detail, max(1, inner-18)))
		lines = append(lines, ts+" "+kind+" "+detail)
	}
	return panel("EVENT STREAM", glyphDotLive+" live", strings.Join(lines, "\n"), width, false)
}

// ── small helpers ──────────────────────────────────────────────────────────

// joinRow places multi-line tiles side by side with single-cell gaps.
func joinRow(tiles []string, total int) string {
	if len(tiles) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tiles)*2-1)
	for i, t := range tiles {
		if i > 0 {
			parts = append(parts, " ")
		}
		parts = append(parts, t)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func ttlShort(exp time.Time) string {
	d := time.Until(exp)
	if d < 0 {
		return "exp"
	}
	mins := int(d.Minutes())
	if mins >= 60 {
		return fmt.Sprintf("%dh", mins/60)
	}
	return fmt.Sprintf("%02dm", mins)
}

func uptimeShort(s string) string {
	d, err := time.ParseDuration(s)
	if err != nil {
		return s
	}
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

func uptimeSince(s string) (time.Time, bool) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, false
	}
	return time.Now().Add(-d), true
}
