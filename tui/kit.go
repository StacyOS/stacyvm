package tui

// kit.go — reusable rendering primitives that consume the theme tokens:
// meters, sparklines, progress bars, bracket-framed tiles, titled panels,
// keycaps/keyhints — plus small ANSI-aware layout helpers and a ring buffer.

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── ring: fixed-size float history feeding the sparklines ─────────────────
type ring struct {
	buf []float64
	cap int
}

func newRing(capacity int) *ring { return &ring{cap: capacity} }

func (r *ring) push(v float64) {
	if r == nil {
		return
	}
	if len(r.buf) >= r.cap {
		r.buf = r.buf[len(r.buf)-r.cap+1:]
	}
	r.buf = append(r.buf, v)
}

func (r *ring) slice() []float64 {
	if r == nil {
		return nil
	}
	return r.buf
}

// ── ANSI-aware layout helpers ─────────────────────────────────────────────

func ansiWidth(s string) int { return lipgloss.Width(s) }

// padRight pads (with spaces) the visible string up to w cells. It does not
// truncate styled text — callers control content width.
func padRight(s string, w int) string {
	gap := w - ansiWidth(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// padLeft right-aligns s within w cells.
func padLeft(s string, w int) string {
	gap := w - ansiWidth(s)
	if gap <= 0 {
		return s
	}
	return strings.Repeat(" ", gap) + s
}

// spread places left and right on one row of exactly w cells, right-aligning
// the right segment.
func spread(left, right string, w int) string {
	gap := w - ansiWidth(left) - ansiWidth(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// label renders a section label in the tracked dim/orange label color. Callers
// pass UPPERCASE literals for section headers; dynamic titles (IDs, paths,
// names) keep their own case so e.g. "◂ sb-7f3a91" isn't shouted.
func label(text string, accent bool) string {
	if accent {
		return stLabelHi.Render(text)
	}
	return stLabel.Render(text)
}

// ── meter / spark / progress ──────────────────────────────────────────────

// meter renders a `val`%-filled bar `width` cells wide. fill is the style for
// the filled cells (empty cells are faint). showPct appends ` NN%` in dim.
func meter(val float64, width int, fill lipgloss.Style, showPct bool) string {
	if val < 0 {
		val = 0
	}
	if val > 100 {
		val = 100
	}
	filled := int(math.Round(val / 100 * float64(width)))
	if filled > width {
		filled = width
	}
	out := fill.Render(strings.Repeat(glyphMeterFull, filled)) +
		stFaint.Render(strings.Repeat(glyphMeterEmpty, width-filled))
	if showPct {
		out += stDim.Render(" " + itoa(int(math.Round(val))) + "%")
	}
	return out
}

// meterFor picks orange when val > 60, else green (the dashboard CPU rule).
func meterFor(val float64, width int, showPct bool) string {
	fill := stOK
	if val > 60 {
		fill = stHi
	}
	return meter(val, width, fill, showPct)
}

// spark renders `data` as a sparkline `width` cells wide, left-padded with the
// lowest ramp glyph (faint) when there are fewer samples than width.
func spark(data []float64, width int, st lipgloss.Style) string {
	max := 1.0
	for _, d := range data {
		if d > max {
			max = d
		}
	}
	var b strings.Builder
	pad := width - len(data)
	if pad > 0 {
		b.WriteString(stFaint.Render(strings.Repeat(string(sparkRamp[0]), pad)))
	}
	start := 0
	if pad < 0 {
		start = -pad // keep the most recent `width` samples
	}
	var glyphs strings.Builder
	for _, d := range data[start:] {
		idx := int(math.Round(d / max * 7))
		if idx < 0 {
			idx = 0
		}
		if idx > 7 {
			idx = 7
		}
		glyphs.WriteRune(sparkRamp[idx])
	}
	b.WriteString(st.Render(glyphs.String()))
	return b.String()
}

// pbar is a thick progress bar (no percent suffix) for spawn/boot sequences.
func pbar(pct float64, width int, fill lipgloss.Style) string {
	return meter(pct, width, fill, false)
}

// ── bracket-framed tile (KPI / workspace / spawn tiles) ───────────────────
//
// Renders the signature corner-bracket frame:
//
//	⌜──────────────────────⌝
//	 LABEL ──────────── hint
//	 <body…>
//	⌞──────────────────────⌟
//
// Corners are steel normally, orange when accent/focused. No vertical side
// bars — the corner brackets are the frame (README "bracket-frame corners").
func bracketFrame(title, rightHint, body string, width int, accent bool) string {
	corner := stSteel
	if accent {
		corner = stHi
	}
	rule := width - 2
	if rule < 0 {
		rule = 0
	}
	top := corner.Render(glyphCornerTL) + stFaint.Render(strings.Repeat("─", rule)) + corner.Render(glyphCornerTR)
	bot := corner.Render(glyphCornerBL) + stFaint.Render(strings.Repeat("─", rule)) + corner.Render(glyphCornerBR)

	inner := width - 2 // one space of inset on each side
	var rows []string
	if title != "" {
		lab := label(title, accent)
		hint := ""
		if rightHint != "" {
			hint = stFaint.Render(rightHint)
		}
		dividerW := inner - ansiWidth(lab) - ansiWidth(hint) - 2
		if dividerW < 1 {
			dividerW = 1
		}
		titleRow := lab + " " + stFaint.Render(strings.Repeat("─", dividerW)) + " " + hint
		rows = append(rows, titleRow)
	}
	for _, ln := range strings.Split(body, "\n") {
		rows = append(rows, ln)
	}

	var b strings.Builder
	b.WriteString(top + "\n")
	for _, r := range rows {
		b.WriteString(" " + padRight(r, inner) + "\n")
	}
	b.WriteString(bot)
	return b.String()
}

// ── titled panel (the "mod" modules) ──────────────────────────────────────
//
// A bordered box with an inside title row (label + optional right hint) above
// the body. Accent modules get an orange-tinted border + orange title.
func panel(title, rightHint, body string, width int, accent bool) string {
	border := colLine2
	if accent {
		border = colOrange
	}
	inner := width - 4 // 2 border + 2 padding
	if inner < 1 {
		inner = 1
	}
	var head string
	if title != "" {
		lab := label(title, accent)
		hint := ""
		if rightHint != "" {
			hint = stFaint.Render(rightHint)
		}
		head = spread(lab, hint, inner) + "\n"
	}
	// Pre-pad the header to the exact text width so the bordered box always
	// spans `width` (border 2 + padding 2 + text inner). Body lines are sized
	// by callers; lipgloss pads shorter lines to the block width.
	content := head + body
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Render(content)
}

// padLines pads s with blank lines (or truncates) so it has exactly n lines.
func padLines(s string, n int) string {
	if n < 1 {
		n = 1
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// panelH is panel() whose outer bordered box is exactly `height` rows tall.
// It pads/truncates the body so callers can fill their allotted pane height
// (fixes the "underutilized space" gaps). height includes the 2 border rows.
func panelH(title, rightHint, body string, width, height int, accent bool) string {
	titleLines := 0
	if title != "" {
		titleLines = 1
	}
	bodyLines := height - 2 - titleLines // rows left for the body inside the box
	if bodyLines < 1 {
		bodyLines = 1
	}
	return panel(title, rightHint, padLines(body, bodyLines), width, accent)
}

// ── keycaps / keyhints ────────────────────────────────────────────────────

func keycap(k string) string { return stKey.Render(k) }

type hint struct{ key, desc string }

// keyHints renders a row of `key desc` pairs separated by spacing.
func keyHints(items []hint) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, keycap(it.key)+" "+stDim.Render(it.desc))
	}
	return strings.Join(parts, "   ")
}

// itoa is a tiny strconv.Itoa to avoid importing strconv everywhere here.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	s := string(digits[i:])
	if neg {
		s = "-" + s
	}
	return s
}
