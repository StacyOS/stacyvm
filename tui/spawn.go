package tui

// spawn.go — screens 6 + 3: the floating quick-spawn modal (Build 1 A) and the
// animated spawn sequence (Build 2, v2-spawn.jsx). The real spawn API call runs
// concurrently with the phase animation; the "ready" tile shows the real result.

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var spawnProviders = []string{"docker", "firecracker", "proot"}

type spawnReq struct {
	image, template, provider, ttl string
}

type spawnPhase struct {
	key, label, line string
	dur              time.Duration
	prog             bool
}

// Phase durations are the design's pacing (v2-spawn.jsx).
var spawnPhases = []spawnPhase{
	{"queue", "queue", "scheduling on %s…", 600 * time.Millisecond, false},
	{"pull", "pull image", "%s  pulling layers", 1500 * time.Millisecond, true},
	{"boot", "boot rootfs", "unpacking · mounting overlay · init", 1100 * time.Millisecond, true},
	{"network", "network", "veth up · assigning address · preview", 700 * time.Millisecond, false},
	{"ready", "ready", "sandbox live", 0, false},
}

type spawnState struct {
	active     bool
	phase      int
	progress   float64 // current-phase %, 0..100
	phaseStart time.Time
	req        spawnReq
	result     *sandboxData
	err        string
	done       bool
}

type spawnTickMsg time.Time

func spawnTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg { return spawnTickMsg(t) })
}

// startSpawn kicks off the real spawn + the animation.
func (m *Model) startSpawn(req spawnReq) tea.Cmd {
	m.spawn = spawnState{active: true, phase: 0, phaseStart: time.Now(), req: req}
	m.mode = modeSpawning
	var spawnCmd tea.Cmd
	if req.template != "" {
		spawnCmd = m.spawnFromTemplate(req.template)
	} else {
		spawnCmd = m.spawnSandbox(req.image, req.ttl, req.provider)
	}
	m.addEvent(eventEntry{ts: time.Now(), kind: "SPAWN", detail: "queued " + req.image})
	return tea.Batch(spawnCmd, spawnTick())
}

// handleSpawnModalKey drives the quick-spawn modal (fields + segmented controls).
func (m *Model) handleSpawnModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.mode = modeNormal
		m.blurAllInputs()
		return m, nil
	case "tab":
		m.spawnFieldFocus(1)
		return m, nil
	case "shift+tab":
		m.spawnFieldFocus(-1)
		return m, nil
	case "enter":
		return m, m.submitSpawnModal()
	case "left", "right":
		d := 1
		if key == "left" {
			d = -1
		}
		switch m.inputFocus {
		case 1: // template select (-1 = none)
			n := len(m.templateList)
			m.spawnTemplateIdx += d
			if m.spawnTemplateIdx < -1 {
				m.spawnTemplateIdx = n - 1
			}
			if m.spawnTemplateIdx >= n {
				m.spawnTemplateIdx = -1
			}
		case 3: // provider segmented
			m.spawnProviderIdx = (m.spawnProviderIdx + d + len(spawnProviders)) % len(spawnProviders)
		}
		return m, nil
	}
	// Text entry on the image (0) / ttl (2) fields.
	idx := -1
	switch m.inputFocus {
	case 0:
		idx = 0
	case 2:
		idx = 1
	}
	if idx >= 0 {
		var cmd tea.Cmd
		m.inputs[idx], cmd = m.inputs[idx].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) spawnFieldFocus(dir int) {
	m.blurAllInputs()
	m.inputFocus = (m.inputFocus + dir + 4) % 4
	switch m.inputFocus {
	case 0:
		m.inputs[0].Focus()
	case 2:
		m.inputs[1].Focus()
	}
}

func (m *Model) submitSpawnModal() tea.Cmd {
	image := m.inputs[0].Value()
	ttl := m.inputs[1].Value()
	if ttl == "" {
		ttl = "30m"
	}
	req := spawnReq{ttl: ttl, provider: spawnProviders[m.spawnProviderIdx]}
	if m.spawnTemplateIdx >= 0 && m.spawnTemplateIdx < len(m.templateList) {
		req.template = m.templateList[m.spawnTemplateIdx].Name
		req.image = m.templateList[m.spawnTemplateIdx].Image
	} else {
		if image == "" {
			image = "alpine:latest"
		}
		req.image = image
	}
	m.blurAllInputs()
	return m.startSpawn(req)
}

// advanceSpawn steps the phase machine. Entry into "ready" is gated on the real
// result (or error) arriving, so the animation never claims success early.
func (m *Model) advanceSpawn(now time.Time) {
	if m.spawn.done {
		return
	}
	cur := m.spawn.phase
	if cur >= len(spawnPhases)-1 {
		m.spawn.done = true
		m.spawn.progress = 100
		return
	}
	ph := spawnPhases[cur]
	elapsed := now.Sub(m.spawn.phaseStart)
	if ph.dur > 0 {
		m.spawn.progress = clampF(float64(elapsed)/float64(ph.dur)*100, 0, 100)
	} else {
		m.spawn.progress = 100
	}
	if elapsed < ph.dur {
		return
	}
	next := cur + 1
	if spawnPhases[next].key == "ready" {
		if m.spawn.result != nil || m.spawn.err != "" {
			m.spawn.phase = next
			m.spawn.done = true
			m.spawn.progress = 100
		} else {
			m.spawn.progress = 95 // hold until the real spawn returns
		}
		return
	}
	m.spawn.phase = next
	m.spawn.phaseStart = now
	m.spawn.progress = 0
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ── spawn modal (quick form) ───────────────────────────────────────────────

func (m Model) renderSpawnModal(height, width int) string {
	inner := 56
	focus := m.inputFocus

	field := func(idx int, k, v string) string {
		bar := "  "
		kst := stDim
		if idx == focus {
			bar = stHi.Render("▌ ")
			kst = stHi
		}
		return bar + kst.Render(padRight(k, 10)) + v
	}

	// template field value
	tmplVal := stDim.Render("none ▾")
	if m.spawnTemplateIdx >= 0 && m.spawnTemplateIdx < len(m.templateList) {
		tmplVal = stInk.Render(m.templateList[m.spawnTemplateIdx].Name + " ▾")
	}

	// provider segmented control
	var segs []string
	for i, p := range spawnProviders {
		if i == m.spawnProviderIdx {
			segs = append(segs, lipgloss.NewStyle().Foreground(colOrange).
				Border(lipgloss.NormalBorder(), false, true, false, true).
				BorderForeground(colOrange).Render(p))
		} else {
			segs = append(segs, stFaint.Render(" "+p+" "))
		}
	}
	provVal := strings.Join(segs, " ")

	lines := []string{
		field(0, "image", m.inputs[0].View()),
		field(1, "template", tmplVal),
		field(2, "ttl", m.inputs[1].View()),
		field(3, "provider", provVal),
		"",
		keyHints([]hint{{glyphTab, "next field"}, {glyphEnter, "spawn"}, {"esc", "cancel"}}),
	}
	body := strings.Join(lines, "\n")
	box := panel(glyphPaneSpawn+" SPAWN SANDBOX", "", body, inner, true)

	bg := stFaint.Render(fmt.Sprintf("fleet: %d sandboxes", len(m.sandboxes)))
	canvas := lipgloss.Place(width, height-1, lipgloss.Center, lipgloss.Center, box)
	return bg + "\n" + canvas
}

// ── animated spawn sequence ─────────────────────────────────────────────────

func (m Model) renderSpawnSequence(height, width int) string {
	colW := (width - 1) / 2
	req := m.spawn.req

	// Left tile: SPAWN REQUEST summary.
	prov := req.provider
	if prov == "" {
		prov = "default"
	}
	tmpl := req.template
	if tmpl == "" {
		tmpl = "—"
	}
	reqBody := strings.Join([]string{
		stDim.Render(padRight("image", 10)) + stInk.Render(orDash(req.image)),
		stDim.Render(padRight("template", 10)) + stInk.Render(tmpl),
		stDim.Render(padRight("provider", 10)) + stInk.Render(prov),
		stDim.Render(padRight("ttl", 10)) + stInk.Render(orDash(req.ttl)),
	}, "\n")
	leftTile := bracketFrame(glyphPaneSpawn+" SPAWN REQUEST", "", reqBody, colW, false)

	// Right tile: PROVISIONING readout, or READY on completion.
	var rightTile string
	if m.spawn.done && m.spawn.result != nil {
		sb := m.spawn.result
		body := strings.Join([]string{
			stOK.Bold(true).Render(glyphCheck + " " + sb.ID + " READY"),
			stDim.Render(sb.Image),
			stDim.Render("expires " + ttlShort(sb.ExpiresAt)),
		}, "\n")
		rightTile = bracketFrame(glyphCheck+" READY", "", body, colW, true)
	} else if m.spawn.err != "" {
		body := stErr.Render(glyphCross+" spawn failed") + "\n" + stDim.Render(truncate(m.spawn.err, colW-4))
		rightTile = bracketFrame(glyphCross+" ERROR", "", body, colW, true)
	} else {
		ph := spawnPhases[m.spawn.phase]
		cursor := ""
		if m.cursorOn {
			cursor = stHi.Render("▏")
		}
		body := strings.Join([]string{
			stHi.Bold(true).Render(strings.ToUpper(ph.label)),
			stDim.Render(fmtPhaseLine(ph, req)) + cursor,
			"",
			pbar(m.spawn.progress, 24, stHi) + stDim.Render(" "+itoa(int(m.spawn.progress))+"%"),
		}, "\n")
		rightTile = bracketFrame(glyphDotCreate+" PROVISIONING", "", body, colW, true)
	}

	tiles := lipgloss.JoinHorizontal(lipgloss.Top, leftTile, " ", rightTile)

	// SEQUENCE checklist timeline.
	var rows []string
	for i, ph := range spawnPhases {
		glyph, lab, timing := m.seqRowParts(i, ph)
		row := glyph + " " + padRight(lab, 14) + stDim.Render(padRight(fmtPhaseLine(ph, req), 44)) + timing
		rows = append(rows, row)
	}
	seq := panel("SEQUENCE", "", strings.Join(rows, "\n"), width, false)

	footer := stFaint.Render("spawning… ") + keyHints([]hint{{glyphReplay, "replay"}, {"esc", "back"}})
	return tiles + "\n\n" + seq + "\n\n" + footer
}

func (m Model) seqRowParts(i int, ph spawnPhase) (glyph, lab, timing string) {
	switch {
	case m.spawn.done || i < m.spawn.phase:
		return stOK.Render(glyphCheck), stOK.Render(ph.label), stOK.Render(seqTiming(ph))
	case i == m.spawn.phase:
		return stHi.Render(glyphDotCreate), stHi.Render(ph.label), stHi.Render("· · ·")
	default:
		return stFaint.Render(glyphDotIdle), stFaint.Render(ph.label), stFaint.Render("—")
	}
}

func seqTiming(ph spawnPhase) string {
	if ph.dur == 0 {
		return ""
	}
	return fmt.Sprintf("%.2fs", ph.dur.Seconds())
}

func fmtPhaseLine(ph spawnPhase, req spawnReq) string {
	if !strings.Contains(ph.line, "%s") {
		return ph.line
	}
	arg := req.image
	if ph.key == "queue" {
		arg = req.provider
		if arg == "" {
			arg = "docker"
		}
	}
	return fmt.Sprintf(ph.line, arg)
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// spawnRibbon is the compact spawn indicator shown in the telemetry ribbon
// while a spawn is in flight, so progress is visible from any screen.
func (m Model) spawnRibbon() string {
	if !m.spawn.active {
		return ""
	}
	if m.spawn.done {
		if m.spawn.result != nil {
			return stOK.Render(glyphCheck + " spawned")
		}
		if m.spawn.err != "" {
			return stErr.Render(glyphCross + " spawn failed")
		}
	}
	ph := spawnPhases[m.spawn.phase]
	return stHi.Render(glyphDotCreate+" SPAWN ") + stDim.Render(ph.label+" ") + stHi.Render(itoa(int(m.spawn.progress))+"%")
}
