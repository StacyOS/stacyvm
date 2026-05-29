package tui

// chrome.go — the app's real shell: a full-width telemetry ribbon, the
// horizontal nav ribbon, and the status footer. These wrap every screen
// (replacing the old left sidebar).

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// faint orange-tinted backgrounds approximating rgba(255,166,12,~0.07).
var colNavActiveBg = lipgloss.Color("#1d160a")

var navItems = []string{"DASH", "SANDBOXES", "TEMPLATES", "PROVIDERS", "LOGS", "CONFIG"}

// renderRibbon draws the full-width telemetry ribbon (one row + a bottom rule).
func (m Model) renderRibbon(width int) string {
	// Left: compact mark + wordmark + kicker.
	left := stHi.Render(ribbonMark) + " " +
		stBold.Render("STACYVM") + " " +
		stFaint.Render("MISSION CONTROL")

	// Right: live CPU/MEM/LOAD sparklines + clock + online badge.
	cpuPct := m.host.cpuPct
	memPct := m.host.memPct
	right := strings.Join([]string{
		stDim.Render("CPU") + " " + spark(m.teleCPU.slice(), 8, stHi) + " " + stHi.Render(itoa(int(cpuPct+0.5)) + "%"),
		stDim.Render("MEM") + " " + spark(m.teleMEM.slice(), 8, stHi) + " " + stHi.Render(itoa(int(memPct+0.5)) + "%"),
		stDim.Render("LOAD") + " " + spark(m.teleLOAD.slice(), 8, stOK),
		stDim.Render(m.clockString()),
		m.onlineBadge(),
	}, "  ")

	row := spread(left, right, width)
	rule := stFaint.Render(strings.Repeat("─", max(0, width)))
	return row + "\n" + rule
}

func (m Model) clockString() string {
	t := m.clock
	if t.IsZero() {
		t = time.Now()
	}
	return t.Format("15:04:05")
}

func (m Model) onlineBadge() string {
	if m.health != nil {
		v := m.health.Version
		if v == "" {
			v = "?"
		}
		return stOK.Render(glyphDotRun+" ONLINE") + " " + stFaint.Render("v"+v)
	}
	return stErr.Render(glyphDotRun + " OFFLINE")
}

// renderNav draws the horizontal nav ribbon. Active item = orange text +
// orange underline + faint-orange bg; numbers are accelerators.
func (m Model) renderNav(width int) string {
	var b strings.Builder
	for i, it := range navItems {
		num := stFaint.Render(itoa(i+1)) + " "
		if tab(i) == m.activeTab {
			seg := lipgloss.NewStyle().
				Foreground(colOrange).
				Background(colNavActiveBg).
				Underline(true).
				Padding(0, 1).
				Render(num + it)
			b.WriteString(seg)
		} else {
			seg := lipgloss.NewStyle().
				Foreground(colDim).
				Padding(0, 1).
				Render(num + it)
			b.WriteString(seg)
		}
	}
	left := b.String()
	right := keycap(glyphCmd) + keycap("K") + " " + stFaint.Render("command")
	return spread(left, right, width)
}

// renderFooter draws the status footer: a top rule + a row with a faint
// summary on the left, keyhints on the right, and an optional colored chip.
func renderFooter(width int, left string, hints []hint, chip string) string {
	rule := stFaint.Render(strings.Repeat("─", max(0, width)))
	right := keyHints(hints)
	if chip != "" {
		right += "   " + chip
	}
	return rule + "\n" + spread(stFaint.Render(left), right, width)
}

// renderStatusFooter builds the per-screen footer: fleet summary on the left,
// contextual keyhints on the right, and a success/error chip.
func (m Model) renderStatusFooter(width int) string {
	left := fmt.Sprintf("%d sandboxes · %d templates · %d providers",
		len(m.sandboxes), len(m.templateList), len(m.providerList))

	var hints []hint
	switch {
	case m.mode == modeConfirm:
		hints = []hint{{"y", "yes"}, {"n", "no"}}
	case m.mode != modeNormal:
		hints = []hint{{glyphTab, "next field"}, {glyphEnter, "submit"}, {"esc", "cancel"}}
	default:
		switch m.activeTab {
		case tabDashboard:
			hints = []hint{{glyphCmd + "K", "command"}, {"?", "help"}, {"q", "quit"}}
		case tabSandboxes:
			hints = []hint{{"j/k", "move"}, {glyphEnter, "inspect"}, {"s", "spawn"}, {"q", "quit"}}
		case tabTemplates:
			hints = []hint{{"j/k", "move"}, {"s", "spawn"}, {"n", "new"}, {"d", "delete"}}
		case tabProviders:
			hints = []hint{{"r", "refresh"}, {glyphEnter, "set default"}}
		case tabLogs:
			hints = []hint{{"/", "filter"}, {"g", "jump kind"}, {"c", "copy"}}
		case tabConfig:
			hints = []hint{{glyphEnter, "edit key"}, {"space", "apply"}}
		}
	}

	chip := ""
	switch {
	case m.mode == modeConfirm && m.confirmMsg != "":
		chip = stHi.Render(strings.TrimSpace(m.confirmMsg))
	case m.statusMsg != "":
		chip = stOK.Render(glyphCheck + " " + m.statusMsg)
	case m.lastError != "":
		chip = stErr.Render(truncate(m.lastError, 40))
	}

	return renderFooter(width, left, hints, chip)
}
