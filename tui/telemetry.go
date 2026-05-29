package tui

// telemetry.go — real-data plumbing for the live ribbon/dashboard: a ~1s
// telemetry tick that polls host + per-sandbox stats into client-side ring
// buffers, a cursor-blink tick, and the SSE event-stream consumer.

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// hostSnapshot is the latest real host telemetry from GET /api/v1/system/stats.
type hostSnapshot struct {
	cpuPct   float64
	memPct   float64
	diskPct  float64
	netRxBps float64
	netTxBps float64
	load1    float64
	ok       bool
}

func (h hostSnapshot) netRate() string { return formatRate(h.netRxBps + h.netTxBps) }

// sandboxStat is the latest real per-sandbox stats from /sandboxes/{id}/stats.
type sandboxStat struct {
	cpuPct    float64
	memBytes  uint64
	memLimit  uint64
	supported bool
}

func (s sandboxStat) memPct() float64 {
	if s.memLimit == 0 {
		return 0
	}
	return float64(s.memBytes) / float64(s.memLimit) * 100
}

// eventEntry is one row of the live event stream (from the SSE bus).
type eventEntry struct {
	ts     time.Time
	kind   string
	detail string
}

// ── messages ──────────────────────────────────────────────────────────────
type (
	teleTickMsg     time.Time
	blinkMsg        time.Time
	hostStatsMsg    hostSnapshot
	sandboxStatsMsg struct {
		id   string
		stat sandboxStat
	}
	eventMsg eventEntry
)

// ── tick commands ──────────────────────────────────────────────────────────

// teleTick fires ~1s — drifts the telemetry window + clock.
func teleTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return teleTickMsg(t) })
}

// blinkTick fires ~1.05s — toggles the cursor on/off (README cadence).
func blinkTick() tea.Cmd {
	return tea.Tick(1050*time.Millisecond, func(t time.Time) tea.Msg { return blinkMsg(t) })
}

func (m Model) fetchHostStats() tea.Cmd {
	return func() tea.Msg {
		h, err := m.client.systemStats()
		if err != nil {
			return hostStatsMsg(hostSnapshot{ok: false})
		}
		return hostStatsMsg(h)
	}
}

func (m Model) fetchSandboxStats(id string) tea.Cmd {
	return func() tea.Msg {
		s, err := m.client.sandboxStats(id)
		if err != nil {
			return sandboxStatsMsg{id: id, stat: sandboxStat{supported: false}}
		}
		return sandboxStatsMsg{id: id, stat: s}
	}
}

// fetchAllSandboxStats polls stats for every running sandbox.
func (m Model) fetchAllSandboxStats() tea.Cmd {
	var cmds []tea.Cmd
	for _, sb := range m.sandboxes {
		if sb.State == "running" {
			cmds = append(cmds, m.fetchSandboxStats(sb.ID))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// ── event stream (SSE) ──────────────────────────────────────────────────────

// streamEvents launches the long-lived SSE reader goroutine once.
func (m Model) streamEvents() tea.Cmd {
	return func() tea.Msg {
		go m.client.subscribeEvents(m.eventCh)
		return nil
	}
}

// waitEvent blocks for the next event from the SSE channel.
func (m Model) waitEvent() tea.Cmd {
	return func() tea.Msg {
		e, ok := <-m.eventCh
		if !ok {
			return nil
		}
		return eventMsg(e)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────

// pushTelemetry pushes the latest host snapshot into the ribbon/host rings.
func (m *Model) pushTelemetry() {
	if !m.host.ok {
		return
	}
	m.teleCPU.push(m.host.cpuPct)
	m.teleMEM.push(m.host.memPct)
	m.teleLOAD.push(m.host.load1)
	m.teleDISK.push(m.host.diskPct)
	m.teleNET.push(m.host.netRxBps + m.host.netTxBps)
}

// pushKPI samples the KPI counts into their rings each tick (real history).
func (m *Model) pushKPI() {
	running := 0
	for _, sb := range m.sandboxes {
		if sb.State == "running" {
			running++
		}
	}
	warm := 0
	for _, t := range m.templateList {
		warm += t.PoolSize
	}
	healthy := 0
	for _, p := range m.providerList {
		if p.Healthy {
			healthy++
		}
	}
	m.kpiSB.push(float64(running))
	m.kpiTmpl.push(float64(warm))
	m.kpiProv.push(float64(healthy))
}

// addEvent records an event-stream entry (capped).
func (m *Model) addEvent(e eventEntry) {
	m.events = append(m.events, e)
	if len(m.events) > 200 {
		m.events = m.events[len(m.events)-200:]
	}
}

func formatRate(bps float64) string {
	switch {
	case bps >= 1024*1024:
		return fmt.Sprintf("%.1fMB/s", bps/1024/1024)
	case bps >= 1024:
		return fmt.Sprintf("%.0fKB/s", bps/1024)
	default:
		return fmt.Sprintf("%.0fB/s", bps)
	}
}

// kindFromEventType maps a server event type to a TUI event KIND + detail.
func kindFromEventType(typ, sandboxID string) (kind, detail string) {
	switch {
	case typ == "sandbox.created", typ == "sandbox.running", typ == "spawn.queued", typ == "spawn.dequeued":
		kind = "SPAWN"
	case strings.HasPrefix(typ, "exec."):
		kind = "EXEC"
	case typ == "file.written":
		kind = "WRITE"
	case typ == "file.read":
		kind = "READ"
	case typ == "sandbox.destroyed":
		kind = "KILL"
	case typ == "quota.saved", typ == "quota.deleted":
		kind = "CONFIG"
	case strings.HasSuffix(typ, ".failed"), typ == "sandbox.error", typ == "provider.failed":
		kind = "ERROR"
	default:
		kind = "EVENT"
	}
	detail = sandboxID
	if detail == "" {
		detail = typ
	} else if kind == "EVENT" || kind == "ERROR" {
		detail = sandboxID + " " + typ
	}
	return kind, detail
}
