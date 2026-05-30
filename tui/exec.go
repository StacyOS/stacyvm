package tui

// exec.go — screen 7, "Exec" (Build 1 A): run a one-off command in a sandbox
// and show framed output with the exit code + duration ALWAYS shown.

import (
	"strings"
)

func (m Model) renderExec(height, width int) string {
	target := "(none)"
	if len(m.sandboxes) > 0 && m.cursor < len(m.sandboxes) {
		target = m.sandboxes[m.cursor].ID
	}
	inner := width - 4

	var lines []string

	// Prompt: the command being typed (or the last one that ran).
	prompt := stOK.Render("$") + " " + m.inputs[2].View()
	lines = append(lines, prompt)

	// Framed output block from the last exec result.
	if m.lastExec != nil {
		lines = append(lines, "")
		if m.lastExecCmd != "" {
			lines = append(lines, stFaint.Render(stOK.Render("$ ")+m.lastExecCmd))
		}
		out := m.lastExec.Stdout
		maxLines := height - 9
		if maxLines < 3 {
			maxLines = 3
		}
		printed := 0
		for _, ln := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			if printed >= maxLines {
				lines = append(lines, stFaint.Render("  …"))
				break
			}
			lines = append(lines, stMint.Render(ln))
			printed++
		}
		if m.lastExec.Stderr != "" {
			for _, ln := range strings.Split(strings.TrimRight(m.lastExec.Stderr, "\n"), "\n") {
				lines = append(lines, stErr.Render(ln))
			}
		}
		// Framed footer: ─ exit N · Dur ─  (always shown).
		exitStyle := stOK
		if m.lastExec.ExitCode != 0 {
			exitStyle = stErr
		}
		tag := " exit " + exitStyle.Render(itoa(m.lastExec.ExitCode)) + stFaint.Render(" · "+m.lastExec.Duration) + " "
		dashes := inner - ansiWidth(tag)
		if dashes < 2 {
			dashes = 2
		}
		left := dashes / 2
		lines = append(lines, stFaint.Render(strings.Repeat("─", left))+tag+stFaint.Render(strings.Repeat("─", dashes-left)))
	}

	lines = append(lines, "", keyHints([]hint{{glyphEnter, "run"}, {"↑", "history"}, {"esc", "back"}}))

	return panel(glyphPaneSpawn+" EXEC · "+target, "", strings.Join(lines, "\n"), width, true)
}
