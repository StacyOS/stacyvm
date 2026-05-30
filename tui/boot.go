package tui

// boot.go — screen 4, the boot splash (Build 2, v2-boot.jsx). The resting
// (finished) state is the default render; tick messages drive the intro, so a
// backgrounded/interrupted frame is never blank.

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type bootTickMsg time.Time

func bootTick() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg { return bootTickMsg(t) })
}

// advanceBoot fills the connect bar; it overfills past 100 to briefly hold the
// "ready" state before dismissing.
func (m *Model) advanceBoot() {
	m.bootProg += 4
	if m.bootProg >= 130 {
		m.booting = false
	}
}

func (m Model) renderBoot(width, height int) string {
	prog := m.bootProg
	if prog > 100 {
		prog = 100
	}

	// Logo mark (orange) + wordmark + tagline.
	logo := stHi.Render(strings.Join(logoHero, "\n"))
	wordmark := stBold.Render("S T A C Y V M")
	tagline := stDim.Render("microVM sandbox orchestrator for LLMs")

	// Connect bar + status line, stepped by progress.
	bar := pbar(prog, 26, stHi)
	var status string
	switch {
	case m.bootProg < 55:
		cur := ""
		if m.cursorOn {
			cur = stHi.Render("▏")
		}
		status = stDim.Render("connecting :7423") + cur
	case m.bootProg < 100:
		cur := ""
		if m.cursorOn {
			cur = stHi.Render("▏")
		}
		status = stDim.Render("handshake · loading fleet") + cur
	default:
		status = stOK.Render(glyphCheck + " ready · " + itoa(len(m.sandboxes)) + " sandboxes")
	}

	block := lipgloss.JoinVertical(lipgloss.Center,
		logo, "", wordmark, "", tagline, "", bar+"  "+status, "",
		stFaint.Render("press any key to skip"),
	)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, block)
}
