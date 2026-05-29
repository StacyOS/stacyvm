package tui

// theme.go — the single design-token layer for the Mission Control TUI.
//
// This file is the source of truth for the handoff's color/glyph/spacing
// table (design_handoff_stacyvm_tui/README.md). All screens style themselves
// through these tokens so the restyle stays consistent.

import "github.com/charmbracelet/lipgloss"

// ── Palette (truecolor hex; roles, not just values) ──────────────────────
//
// Orange is the single dominant accent — the focused / active / live thing.
// Green is the only other semantic color (health/success). Everything else
// lives in the warm grayscale ramp.
var (
	colBg     = lipgloss.Color("#08080a") // terminal background (near-black, warm)
	colPanel  = lipgloss.Color("#0c0c10") // panel / screen fill
	colPanel2 = lipgloss.Color("#101016") // chrome bars, keycaps, modeline, selected-row bg
	colInk    = lipgloss.Color("#ECE7DD") // primary foreground (warm off-white)
	colDim    = lipgloss.Color("#7d7d86") // secondary text / labels
	colFaint  = lipgloss.Color("#4a4a52") // tertiary text, empty meter cells, separators
	colLine   = lipgloss.Color("#23232b") // borders / dividers
	colLine2  = lipgloss.Color("#33333d") // stronger borders, keycap edges
	colOrange = lipgloss.Color("#FFA60C") // PRIMARY ACCENT — selection, focus, active nav, cursor, logo
	colOrange2 = lipgloss.Color("#FF7A1A") // secondary accent (active tab number)
	colGreen  = lipgloss.Color("#22C55E") // success / running / ready / ONLINE
	colRed    = lipgloss.Color("#FF4747") // errors, deletions, kill
	colMint   = lipgloss.Color("#D7F6E2") // function names in code, TEMPLATE events
	colSteel  = lipgloss.Color("#5b6b78") // paths, muted code, bracket-frame corners
)

// ── Glyph vocabulary (use these exact characters) ────────────────────────
const (
	glyphDotRun     = "●" // running / online / live
	glyphDotCreate  = "◐" // creating / booting / in-progress
	glyphDotIdle    = "○" // idle / standby / pending
	glyphDotLive    = "◉" // live (event stream "following")
	glyphCheck      = "✓" // done
	glyphCross      = "✗" // error

	glyphMeterFull  = "█"
	glyphMeterEmpty = "░"

	glyphTreeOpen   = "▾"
	glyphTreeClosed = "▸"
	glyphTreeFile   = "·"

	glyphCornerTL = "⌜"
	glyphCornerTR = "⌝"
	glyphCornerBL = "⌞"
	glyphCornerBR = "⌟"

	glyphChip       = "▣" // sandbox id chip
	glyphPaneFiles  = "◧"
	glyphPaneEditor = "◨"
	glyphPaneTerm   = "◰"
	glyphPaneSpawn  = "◇"
	glyphInspect    = "◂"

	glyphCmd   = "⌘"
	glyphEnter = "↵"
	glyphTab   = "⇥"
	glyphReplay = "↻"
)

// sparkRamp is the 8-step ramp mapped from value/max.
var sparkRamp = []rune("▁▂▃▄▅▆▇█")

// ── Base styles ───────────────────────────────────────────────────────────
var (
	stInk   = lipgloss.NewStyle().Foreground(colInk)
	stDim   = lipgloss.NewStyle().Foreground(colDim)
	stFaint = lipgloss.NewStyle().Foreground(colFaint)
	stSteel = lipgloss.NewStyle().Foreground(colSteel)
	stMint  = lipgloss.NewStyle().Foreground(colMint)

	stHi   = lipgloss.NewStyle().Foreground(colOrange)            // accent / highlight
	stHiB  = lipgloss.NewStyle().Foreground(colOrange).Bold(true) // bold accent (IDs, metrics)
	stOK   = lipgloss.NewStyle().Foreground(colGreen)
	stErr  = lipgloss.NewStyle().Foreground(colRed)
	stBold = lipgloss.NewStyle().Foreground(colInk).Bold(true)

	// label: UPPERCASE, dim, wide letter-spacing (approximated with spaces by callers).
	stLabel = lipgloss.NewStyle().Foreground(colDim)
	// accent label: module/tile titles when the module is the focused/accent one.
	stLabelHi = lipgloss.NewStyle().Foreground(colOrange)

	// keycap: bordered chip with panel-2 fill and a heavier bottom edge feel.
	stKey = lipgloss.NewStyle().
		Foreground(colInk).
		Background(colPanel2).
		Padding(0, 1)
)

// stateStyle returns the foreground style for a sandbox/provider state word.
func stateStyle(state string) lipgloss.Style {
	switch state {
	case "running", "run", "online", "healthy", "ready", "ok":
		return stOK
	case "creating", "booting", "warn", "creating…":
		return stHi
	case "idle", "standby", "stopped", "expired", "destroyed":
		return stDim
	case "error", "unhealthy", "offline":
		return stErr
	default:
		return stDim
	}
}

// stateDot returns the colored status dot glyph for a state.
func stateDot(state string) string {
	switch state {
	case "creating", "booting":
		return stHi.Render(glyphDotCreate)
	case "idle", "standby", "stopped", "expired", "destroyed":
		return stDim.Render(glyphDotIdle)
	case "error", "unhealthy", "offline":
		return stErr.Render(glyphCross)
	default: // running / run / online / healthy / ready
		return stOK.Render(glyphDotRun)
	}
}

// kindStyle colors an event-stream / log KIND token by category.
func kindStyle(kind string) lipgloss.Style {
	switch kind {
	case "SPAWN":
		return stHi
	case "EXEC":
		return stOK
	case "WRITE", "READ":
		return stSteel
	case "TEMPLATE":
		return stMint
	case "KILL", "ERROR":
		return stErr
	case "CONFIG":
		return stDim
	default:
		return stDim
	}
}
