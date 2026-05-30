# Workspace Editing & Navigation Overhaul Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the StacyVM TUI workspace UX — kill the nav rendering glitch, make panes fill their space, give the file tree a working scrolling viewport with auto-refresh, replace the ad-hoc editor with a small Vim-like modal editor behind a clean `Editor` interface, and make pane-vs-screen navigation consistent with on-screen key hints.

**Architecture:** Bubble Tea (`charmbracelet/bubbletea` + `lipgloss` + `bubbles`). The editor is **Approach A**: `bubbles/textarea` does all low-level editing; a thin `TextareaEditor` adds NORMAL/INSERT modes and a fixed command set, driving textarea only through its public API and synthesized key messages. The rest of the app depends on an `Editor` interface so the implementation can evolve later (`TextareaEditor → CustomEditor → Embedded Vim`) without app changes.

**Tech Stack:** Go 1.25, `github.com/charmbracelet/bubbletea` v1.3.10, `github.com/charmbracelet/bubbles` v1.0.0 (`textarea`, `textinput`), `github.com/charmbracelet/lipgloss` v1.1.1.

**Spec:** `docs/superpowers/specs/2026-05-30-workspace-editor-navigation-overhaul-design.md`

---

## File Structure

- `tui/editor.go` *(new)* — `Editor` interface + `TextareaEditor` modal layer (modes, command set, single-slot clipboard, bounded undo).
- `tui/editor_test.go` *(new)* — unit tests for the modal layer.
- `tui/kit.go` *(modify)* — add `padLines` + `panelH` (height-filling panel).
- `tui/dashboard.go` *(modify)* — fix `selectedRow` to not nest pre-styled content.
- `tui/chrome.go` *(modify)* — fix `renderNav` ANSI leak; add workspace-aware footer hints.
- `tui/files.go` *(modify)* — shared netrw-style tree view with scrolling viewport.
- `tui/workspace.go` *(modify)* — use `Editor`; remove `:` cmdline; pane focus + Blur/Focus; terminal toggle; height-filling layout; editor modeline.
- `tui/app.go` *(modify)* — navigation routing (arrows local, drop arrow tab-switch, `Tab`/`Shift+Tab` panes); push file content into the editor on read; re-list the tree dir after exec.
- `tui/render_batch2_test.go` *(modify)* — add a no-ANSI-leak regression assertion + workspace render checks.

Order of tasks leaves the app building and passing tests after every task.

---

## Task 1: Fix the nav ANSI leak and `selectedRow` nesting (bug #1 + glitch sweep)

**Files:**
- Modify: `tui/chrome.go:69-92` (`renderNav`)
- Modify: `tui/dashboard.go:144-147` (`selectedRow`)
- Modify: `tui/workspace.go:336-338` and `tui/files.go:181-185` (callers passing pre-styled rows)
- Test: `tui/render_batch2_test.go`

Root cause: a pre-styled string (containing ANSI escapes) is nested inside a *second* lipgloss style that sets `Background`/`Underline`; this lipgloss version strips the embedded `ESC` byte, leaking literal `[38;2;…m` text. Fix: never wrap pre-styled fragments — render each segment once from raw text.

- [ ] **Step 1: Set the test imports + write the failing regression test**

Update the import block of `tui/render_batch2_test.go` (currently `"testing"`, `"time"`) to:

```go
import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)
```

`termenv` is currently an indirect dependency; run `go mod tidy` once after adding this import so go.mod records it as direct (it is already in `go.sum`). (`lipgloss` is also used by Task 2's test.)

Then add this test. Two subtleties baked in:
1. **Force truecolor.** In a non-TTY `go test` run, lipgloss strips color, so the leak can't reproduce. `lipgloss.SetColorProfile(termenv.TrueColor)` forces the default renderer (which the package-level styles use) to emit truecolor so the bug manifests deterministically.
2. **Detection.** The substring `[38;2;` ALSO appears inside every *legitimate* escape `\x1b[38;2;…m`, so we can't just search for it. The leak is a `[38;2;` **not** preceded by the `ESC` byte — so assert every `[38;2;` is part of a real `\x1b[38;2;` escape (counts equal).

```go
// TestNoAnsiLeak guards against the styled-in-styled bug where an embedded
// escape's ESC byte is stripped, leaking a bare "[38;2;..m" into visible text.
// Every "[38;2;" must be part of a real escape sequence "\x1b[38;2;".
func TestNoAnsiLeak(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor) // force color so the leak reproduces
	m := seedModel()
	for _, tb := range []tab{tabDashboard, tabSandboxes, tabTemplates, tabProviders, tabLogs, tabConfig} {
		m.activeTab = tb
		out := m.View()
		total := strings.Count(out, "[38;2;")
		real := strings.Count(out, "\x1b[38;2;")
		if total != real {
			t.Errorf("tab %d: %d bare '[38;2;' leaked (total=%d real=%d)", tb, total-real, total, real)
		}
	}
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `go mod tidy && go test ./tui/ -run TestNoAnsiLeak -v`
Expected: FAIL — `bare '[38;2;' leaked` on every tab (the active nav segment leaks its stripped escape).

- [ ] **Step 3: Fix `renderNav` to render each segment once from raw text**

Replace the body of `renderNav` in `tui/chrome.go` (lines 69-92) with:

```go
func (m Model) renderNav(width int) string {
	var b strings.Builder
	for i, it := range navItems {
		// Build the raw segment text first; render ONCE so no pre-styled
		// fragment is nested inside another style (that nesting strips the
		// embedded ESC byte and leaks literal "[38;2;..m").
		seg := itoa(i+1) + " " + it
		if tab(i) == m.activeTab {
			b.WriteString(lipgloss.NewStyle().
				Foreground(colOrange).
				Background(colNavActiveBg).
				Underline(true).
				Padding(0, 1).
				Render(seg))
		} else {
			b.WriteString(lipgloss.NewStyle().
				Foreground(colDim).
				Padding(0, 1).
				Render(seg))
		}
	}
	left := b.String()
	right := keycap(glyphCmd) + keycap("K") + " " + stFaint.Render("command")
	return spread(left, right, width)
}
```

- [ ] **Step 4: Fix `selectedRow` to take raw text and style once**

Replace `selectedRow` in `tui/dashboard.go` (lines 144-147) with:

```go
// selectedRow renders a highlighted selection row from RAW text. It applies the
// accent foreground + selection background in a single style so no pre-styled
// fragment is nested (which would strip embedded escapes).
func selectedRow(row string, inner int) string {
	bar := stHi.Render("▌")
	body := lipgloss.NewStyle().
		Foreground(colOrange).
		Background(colNavActiveBg).
		Render(padRight(row, inner-1))
	return bar + body
}
```

- [ ] **Step 5: Update callers to pass raw text (not pre-styled)**

In `tui/workspace.go` line 337, change:

```go
			rows = append(rows, selectedRow(stHi.Render(n.name), width-4))
```
to:
```go
			rows = append(rows, selectedRow(n.name, width-4))
```

In `tui/files.go` line 183, change:

```go
			rows = append(rows, selectedRow(stHi.Render(n.name), width-4))
```
to:
```go
			rows = append(rows, selectedRow(n.name, width-4))
```

- [ ] **Step 6: Run the regression test + full suite**

Run: `go test ./tui/ -run TestNoAnsiLeak -v && go test ./tui/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add tui/chrome.go tui/dashboard.go tui/workspace.go tui/files.go tui/render_batch2_test.go
git commit -m "fix(tui): stop styled-in-styled nesting leaking raw ANSI in nav and selection rows"
```

---

## Task 2: Add a height-filling panel helper

**Files:**
- Modify: `tui/kit.go` (add `padLines` + `panelH` after `panel`)
- Test: `tui/render_batch2_test.go`

`panel()` borders whatever it's given and never pads to a target height — the root cause of wasted space. Add a wrapper that pads/truncates the body so the outer box is exactly `height` rows tall.

- [ ] **Step 1: Write the failing test**

Add to `tui/render_batch2_test.go`:

```go
func TestPanelHFillsHeight(t *testing.T) {
	// A one-line body in a 10-row panel must still produce a 10-row box.
	out := panelH("TITLE", "", "one line", 40, 10, false)
	if got := lipgloss.Height(out); got != 10 {
		t.Errorf("panelH height = %d, want 10\n%s", got, out)
	}
	// Over-long bodies are truncated to fit, not overflow.
	body := strings.Repeat("x\n", 50)
	out = panelH("TITLE", "", body, 40, 8, false)
	if got := lipgloss.Height(out); got != 8 {
		t.Errorf("panelH height (truncate) = %d, want 8", got)
	}
}
```

`lipgloss` and `strings` are already imported in `tui/render_batch2_test.go` (added in Task 1), so no import change is needed here — this test uses `lipgloss.Height` and `strings.Repeat`.

- [ ] **Step 2: Run it to confirm it fails**

Run: `go test ./tui/ -run TestPanelHFillsHeight -v`
Expected: FAIL — `panelH` undefined.

- [ ] **Step 3: Implement `padLines` + `panelH`**

Add to `tui/kit.go` immediately after the `panel` function (after line 234):

```go
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
```

- [ ] **Step 4: Run the test**

Run: `go test ./tui/ -run TestPanelHFillsHeight -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tui/kit.go tui/render_batch2_test.go
git commit -m "feat(tui): add panelH height-filling panel helper"
```

---

## Task 3: Editor interface + `TextareaEditor` modal layer

**Files:**
- Create: `tui/editor.go`
- Test: `tui/editor_test.go`

A self-contained widget. No app wiring yet — the app keeps building and passing. All editing goes through textarea's public API and synthesized key messages (confirmed bindings: `left`/`right` = char move, `alt+right`/`alt+left` = word move, `delete` = delete-forward; methods `CursorUp/Down/Start/End`, `Line`, `Value`/`SetValue`).

- [ ] **Step 1: Write the editor file**

Create `tui/editor.go`:

```go
package tui

// editor.go — the Editor seam plus TextareaEditor, a lightweight Vim-like modal
// layer over bubbles/textarea (Approach A). textarea owns ALL low-level editing
// (insert, delete, cursor, scroll, viewport, utf-8, multiline). This type only
// adds NORMAL/INSERT modes and a small, fixed command set. No custom buffer,
// viewport, or cursor-sync logic. Out of scope by design: registers, macros,
// dot-repeat, text objects, visual mode, marks, ex commands.

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// Editor is the interface the rest of the app depends on. Implementations may
// evolve (TextareaEditor -> CustomEditor -> Embedded Vim) without app changes.
type Editor interface {
	SetContent(string)
	Value() string
	SetSize(width, height int)
	Update(tea.Msg) tea.Cmd
	View() string
	Focus()
	Blur()
}

type editorMode int

const (
	editorNormal editorMode = iota
	editorInsert
)

const editorUndoCap = 50

// modalEditor is an optional capability used by the workspace to show the mode
// badge and to decide Esc behavior. Kept off the core Editor interface so that
// interface stays minimal.
type modalEditor interface {
	Mode() editorMode
}

// TextareaEditor is the Approach-A modal editor.
type TextareaEditor struct {
	ta        textarea.Model
	mode      editorMode
	clipboard string   // single slot for yy/p (NOT a register file)
	undo      []string // bounded snapshot stack for u (NOT an undo tree)
	pending   string   // pending operator: "d" or "y" ("" = none)
}

// NewTextareaEditor builds an editor in NORMAL mode with line numbers on.
func NewTextareaEditor() *TextareaEditor {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = true
	return &TextareaEditor{ta: ta, mode: editorNormal}
}

func (e *TextareaEditor) SetContent(s string) {
	e.ta.SetValue(s)
	e.undo = e.undo[:0]
	e.pending = ""
}

func (e *TextareaEditor) Value() string { return e.ta.Value() }

func (e *TextareaEditor) SetSize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	e.ta.SetWidth(w)
	e.ta.SetHeight(h)
}

func (e *TextareaEditor) Focus() { e.ta.Focus() }
func (e *TextareaEditor) Blur()  { e.ta.Blur() }
func (e *TextareaEditor) View() string { return e.ta.View() }
func (e *TextareaEditor) Mode() editorMode { return e.mode }

// Update routes a message based on the current mode.
func (e *TextareaEditor) Update(msg tea.Msg) tea.Cmd {
	key, isKey := msg.(tea.KeyMsg)
	if !isKey {
		var cmd tea.Cmd
		e.ta, cmd = e.ta.Update(msg) // blink, etc.
		return cmd
	}
	if e.mode == editorInsert {
		return e.updateInsert(key)
	}
	return e.updateNormal(key)
}

func (e *TextareaEditor) updateInsert(key tea.KeyMsg) tea.Cmd {
	if key.Type == tea.KeyEsc {
		e.mode = editorNormal
		return nil
	}
	var cmd tea.Cmd
	e.ta, cmd = e.ta.Update(key)
	return cmd
}

func (e *TextareaEditor) updateNormal(key tea.KeyMsg) tea.Cmd {
	k := key.String()

	// Resolve a pending operator (dd / yy). Any other second key cancels it.
	if e.pending != "" {
		op := e.pending
		e.pending = ""
		switch {
		case op == "d" && k == "d":
			e.snapshot()
			e.deleteLine(true)
		case op == "y" && k == "y":
			e.yankLine()
		}
		return nil
	}

	switch k {
	case "h", "left":
		e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyLeft})
	case "l", "right":
		e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyRight})
	case "j", "down":
		e.ta.CursorDown()
	case "k", "up":
		e.ta.CursorUp()
	case "0":
		e.ta.CursorStart()
	case "$":
		e.ta.CursorEnd()
	case "w":
		e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	case "b":
		e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyLeft, Alt: true})
	case "x":
		e.snapshot()
		var cmd tea.Cmd
		e.ta, cmd = e.ta.Update(tea.KeyMsg{Type: tea.KeyDelete})
		return cmd
	case "d":
		e.pending = "d"
	case "y":
		e.pending = "y"
	case "p":
		e.snapshot()
		e.pasteBelow()
	case "u":
		e.undoOp()
	case "i":
		e.snapshot()
		e.mode = editorInsert
	}
	return nil
}

// ── line operations (via textarea Value/SetValue + Line) ───────────────────

func (e *TextareaEditor) snapshot() {
	e.undo = append(e.undo, e.ta.Value())
	if len(e.undo) > editorUndoCap {
		e.undo = e.undo[len(e.undo)-editorUndoCap:]
	}
}

func (e *TextareaEditor) undoOp() {
	if len(e.undo) == 0 {
		return
	}
	prev := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.ta.SetValue(prev)
}

// linesAndRow returns the buffer split into lines and the current row (clamped).
func (e *TextareaEditor) linesAndRow() ([]string, int) {
	ls := strings.Split(e.ta.Value(), "\n")
	row := e.ta.Line()
	if row < 0 {
		row = 0
	}
	if row >= len(ls) {
		row = len(ls) - 1
	}
	return ls, row
}

func (e *TextareaEditor) yankLine() {
	ls, row := e.linesAndRow()
	e.clipboard = ls[row]
}

func (e *TextareaEditor) deleteLine(cut bool) {
	ls, row := e.linesAndRow()
	if cut {
		e.clipboard = ls[row]
	}
	if len(ls) <= 1 {
		e.ta.SetValue("")
		return
	}
	ls = append(ls[:row], ls[row+1:]...)
	e.ta.SetValue(strings.Join(ls, "\n"))
}

func (e *TextareaEditor) pasteBelow() {
	if e.clipboard == "" {
		return
	}
	ls, row := e.linesAndRow()
	out := make([]string, 0, len(ls)+1)
	out = append(out, ls[:row+1]...)
	out = append(out, e.clipboard)
	out = append(out, ls[row+1:]...)
	e.ta.SetValue(strings.Join(out, "\n"))
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./tui/`
Expected: builds clean.

- [ ] **Step 3: Write the editor tests**

Create `tui/editor_test.go`:

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// newEd returns a focused, sized editor seeded with content in NORMAL mode.
func newEd(content string) *TextareaEditor {
	e := NewTextareaEditor()
	e.SetSize(40, 10)
	e.Focus()
	e.SetContent(content)
	return e
}

// gotoTop moves the cursor to line 0. textarea.SetValue leaves the cursor at the
// END of the content, so line-oriented tests must reposition deterministically.
func gotoTop(e *TextareaEditor) {
	for i := 0; i < 50; i++ {
		e.Update(runes("k")) // CursorUp clamps at the first line
	}
}

func TestEditorStartsInNormalMode(t *testing.T) {
	e := newEd("hello")
	if e.Mode() != editorNormal {
		t.Fatalf("want NORMAL mode at start")
	}
}

func TestEditorInsertAndEsc(t *testing.T) {
	e := newEd("")
	e.Update(runes("i")) // enter INSERT
	if e.Mode() != editorInsert {
		t.Fatalf("i did not enter INSERT")
	}
	e.Update(runes("a"))
	e.Update(runes("b"))
	e.Update(tea.KeyMsg{Type: tea.KeyEsc}) // back to NORMAL
	if e.Mode() != editorNormal {
		t.Fatalf("esc did not return to NORMAL")
	}
	if e.Value() != "ab" {
		t.Fatalf("insert produced %q, want %q", e.Value(), "ab")
	}
}

func TestEditorNormalKeysDoNotInsert(t *testing.T) {
	e := newEd("")
	e.Update(runes("z")) // unknown NORMAL key — must be ignored, not typed
	if e.Value() != "" {
		t.Fatalf("NORMAL ignored key leaked into buffer: %q", e.Value())
	}
}

func TestEditorDeleteLine(t *testing.T) {
	e := newEd("one\ntwo\nthree")
	gotoTop(e) // cursor to line 0
	e.Update(runes("d"))
	e.Update(runes("d"))
	if e.Value() != "two\nthree" {
		t.Fatalf("dd produced %q", e.Value())
	}
}

func TestEditorYankPaste(t *testing.T) {
	e := newEd("alpha\nbeta")
	gotoTop(e)
	e.Update(runes("y"))
	e.Update(runes("y")) // yank "alpha"
	e.Update(runes("p")) // paste below line 0
	if e.Value() != "alpha\nalpha\nbeta" {
		t.Fatalf("yy/p produced %q", e.Value())
	}
}

func TestEditorUndo(t *testing.T) {
	e := newEd("keep\ndrop")
	before := e.Value()
	gotoTop(e)
	e.Update(runes("d"))
	e.Update(runes("d")) // delete "keep"
	if e.Value() == before {
		t.Fatalf("dd did not change buffer")
	}
	e.Update(runes("u")) // undo
	if e.Value() != before {
		t.Fatalf("u did not restore; got %q want %q", e.Value(), before)
	}
}

func TestEditorSetContentClearsUndo(t *testing.T) {
	e := newEd("a")
	gotoTop(e)
	e.Update(runes("d"))
	e.Update(runes("d"))
	e.SetContent("fresh")
	e.Update(runes("u")) // nothing to undo after fresh content
	if e.Value() != "fresh" {
		t.Fatalf("undo after SetContent changed buffer: %q", e.Value())
	}
}
```

- [ ] **Step 4: Run the editor tests**

Run: `go test ./tui/ -run TestEditor -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add tui/editor.go tui/editor_test.go
git commit -m "feat(tui): add Editor interface + TextareaEditor modal layer"
```

---

## Task 4: Wire the Editor into the workspace; remove the `:` cmdline; Ctrl+S save

**Files:**
- Modify: `tui/workspace.go` (state, openWorkspace, key handling, render, helpers)
- Modify: `tui/app.go:476-485` (`fileReadMsg` pushes content into the editor)

Replace the inline textarea + vim `:` command line with the `Editor`. Save = `Ctrl+S`; leave pane = `Esc` (when NORMAL) / `Tab`.

- [ ] **Step 1: Replace workspace state fields**

In `tui/workspace.go`, replace the `workspaceState` struct (lines 23-32) with:

```go
type workspaceState struct {
	sandboxID string
	focus     int
	files     fileState // tree + current dir + open file content
	editor    Editor    // the modal editor for the open file
	termLines []string
	termInput textinput.Model
	termBusy  bool
	showTerm  bool // terminal pane visible (toggle with ctrl+t)
}
```

- [ ] **Step 2: Initialize the editor in `openWorkspace`**

In `tui/workspace.go`, replace the `m.workspace = workspaceState{...}` assignment in `openWorkspace` (lines 44-50) with:

```go
	ed := NewTextareaEditor()
	m.workspace = workspaceState{
		sandboxID: id,
		focus:     wsFocusTree,
		files:     fileState{sandboxID: id, dir: "/workspace"},
		editor:    ed,
		termInput: ti,
		showTerm:  true,
		termLines: []string{stDim.Render("# in-VM shell · commands run for real via exec")},
	}
```

- [ ] **Step 3: Add a focus helper and rewrite the key handler**

In `tui/workspace.go`, replace `handleWorkspaceKey` (lines 67-103) with:

```go
// setWSFocus moves pane focus and keeps the editor's textarea focus in sync.
func (m *Model) setWSFocus(n int) {
	ws := &m.workspace
	ws.focus = n
	if ws.editor == nil {
		return
	}
	if n == wsFocusEditor {
		ws.editor.Focus()
	} else {
		ws.editor.Blur()
	}
}

func (m *Model) handleWorkspaceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	ws := &m.workspace

	// Pane focus ring (consistent across multi-pane screens).
	switch key {
	case "tab":
		next := (ws.focus + 1) % m.wsPaneCount()
		m.setWSFocus(next)
		return m, nil
	case "shift+tab":
		next := (ws.focus - 1 + m.wsPaneCount()) % m.wsPaneCount()
		m.setWSFocus(next)
		return m, nil
	case "ctrl+w":
		m.mode = modeNormal
		return m, nil
	case "ctrl+t":
		ws.showTerm = !ws.showTerm
		if !ws.showTerm && ws.focus == wsFocusTerm {
			m.setWSFocus(wsFocusEditor)
		}
		return m, nil
	}

	// Direct pane jumps (workspace-only; global 1-6 screen switching is
	// suspended while the workspace owns keys). Editor keeps digits in INSERT.
	inInsert := false
	if me, ok := ws.editor.(modalEditor); ok && me.Mode() == editorInsert {
		inInsert = true
	}
	if !inInsert {
		switch key {
		case "1":
			m.setWSFocus(wsFocusTree)
			return m, nil
		case "2":
			m.setWSFocus(wsFocusEditor)
			return m, nil
		case "3":
			if ws.showTerm {
				m.setWSFocus(wsFocusTerm)
			}
			return m, nil
		}
	}

	switch ws.focus {
	case wsFocusTree:
		return m.workspaceTreeKey(key)
	case wsFocusEditor:
		return m.workspaceEditorKey(msg)
	case wsFocusTerm:
		return m.workspaceTermKey(msg)
	}
	return m, nil
}

// wsPaneCount is 3 with the terminal visible, else 2 (tree + editor).
func (m Model) wsPaneCount() int {
	if m.workspace.showTerm {
		return 3
	}
	return 2
}
```

- [ ] **Step 4: Rewrite `workspaceEditorKey` to delegate to the Editor**

In `tui/workspace.go`, replace the entire `workspaceEditorKey` function (lines 134-192) with:

```go
func (m *Model) workspaceEditorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ws := &m.workspace
	switch msg.String() {
	case "ctrl+s":
		if ws.files.openPath == "" {
			return m, nil
		}
		content := ws.editor.Value()
		ws.files.content = content
		return m, m.writeFileCmd(ws.sandboxID, ws.files.openPath, content)
	case "esc":
		// Esc returns INSERT->NORMAL inside the editor; a second Esc (in
		// NORMAL) leaves the workspace.
		if me, ok := ws.editor.(modalEditor); ok && me.Mode() == editorInsert {
			return m, ws.editor.Update(msg)
		}
		m.mode = modeNormal
		return m, nil
	}
	return m, ws.editor.Update(msg)
}
```

- [ ] **Step 5: Remove dead code and the now-unused `textarea` import**

In `tui/workspace.go`, delete the `runVimCommand` function (lines 215-241) entirely. (It is only referenced by the old `:` cmdline code removed in Step 4.)

The workspace no longer constructs a `textarea.Model` (the `Editor` owns it now), so remove the textarea import. The import block becomes:

```go
import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)
```

(`textinput`, `tea`, `lipgloss`, `strings`, `time` all remain in use. `path` is added later in Task 5, where it is first used.)

- [ ] **Step 6: Update the tree `enter`-opens-file path to load the editor**

In `tui/workspace.go` `workspaceTreeKey`, replace the file-open branch (lines 124-129) with:

```go
			f.openPath = n.fpath
			m.setWSFocus(wsFocusEditor)
			return m, m.readFileCmd(m.workspace.sandboxID, n.fpath)
```

(The `f.write = false` line is removed — there is no `write` flag in the editor flow.)

- [ ] **Step 7: Push file content into the editor on read**

In `tui/app.go`, replace the `fileReadMsg` case (lines 476-485) with:

```go
	case fileReadMsg:
		if m.mode == modeWorkspace {
			m.workspace.files.content = string(msg)
			if m.workspace.editor != nil {
				m.workspace.editor.SetContent(string(msg))
			}
		} else if m.mode == modeInput {
			f := m.activeFiles()
			f.content = string(msg)
			f.write = false
		} else {
			m.lastOutput = string(msg)
		}
		m.lastError = ""
		m.addLog("READ", fmt.Sprintf("%d bytes", len(msg)))
```

- [ ] **Step 8: Rewrite `workspaceEditor` render to use the Editor + modeline**

In `tui/workspace.go`, replace the `workspaceEditor` function (lines 349-397) with:

```go
func (m Model) workspaceEditor(width, height int) string {
	ws := m.workspace
	focused := ws.focus == wsFocusEditor
	title := glyphPaneEditor + " " + orDash(ws.files.openPath)

	// Reserve 2 rows inside the box for the title + modeline; editor fills rest.
	editorH := height - 4
	if editorH < 1 {
		editorH = 1
	}
	var body string
	if ws.files.openPath == "" {
		body = stFaint.Render("open a file from the tree (↵)")
		body = padLines(body, editorH)
	} else {
		ws.editor.SetSize(width-4, editorH)
		body = ws.editor.View()
	}

	mode := editorNormal
	if me, ok := ws.editor.(modalEditor); ok {
		mode = me.Mode()
	}
	var badge, hints string
	if mode == editorInsert {
		badge = lipgloss.NewStyle().Foreground(colBg).Background(colGreen).Render(" -- INSERT -- ")
		hints = stDim.Render("type to edit · esc normal")
	} else {
		badge = lipgloss.NewStyle().Foreground(colBg).Background(colOrange).Render(" NORMAL ")
		hints = stDim.Render("i insert · ^s save · esc back")
	}
	info := stDim.Render(" " + filename(ws.files.openPath) + " · utf-8 · unix")
	modeline := spread(badge+info, hints, width-4)

	hint := ""
	if focused {
		hint = "FOCUS"
	}
	return panelH(title, hint, body+"\n"+modeline, width, height, focused)
}
```

- [ ] **Step 9: Fix the existing workspace snapshot test, then build + run the suite**

The existing `TestBatch2Renders` in `tui/render_batch2_test.go` builds a `workspaceState` literal with no `editor` (now a nil interface → `renderWorkspace` would panic) and no `showTerm` (so the terminal pane would not render). Replace its "Workspace" block (the `m = seedModel()` … `snap(t, "workspace", …)` at the end of `TestBatch2Renders`) with:

```go
	// Workspace
	m = seedModel()
	m.activeTab = tabSandboxes
	m.mode = modeWorkspace
	ed := NewTextareaEditor()
	ed.SetContent("import os\n\ndef main():\n    print(\"hi\")  # go\n")
	m.workspace = workspaceState{
		sandboxID: "sb-7f3a91",
		focus:     wsFocusEditor,
		editor:    ed,
		showTerm:  true,
		files: fileState{
			sandboxID: "sb-7f3a91", dir: "/workspace", openPath: "/workspace/main.py",
			nodes: []fileNode{{name: "src", fpath: "/workspace/src", isDir: true}, {name: "main.py", fpath: "/workspace/main.py"}},
		},
		termLines: []string{"$ python -m pytest -q", "24 passed in 1.83s"},
	}
	snap(t, "workspace", m.View(), "FILES", "TERMINAL", "NORMAL", "sb-7f3a91")
```

Then run: `go build ./... && go test ./tui/`
Expected: builds; all tests pass. (The standalone `files` browser test uses `modeInput`, unaffected.)

- [ ] **Step 10: Add a workspace render smoke test**

Add to `tui/render_batch2_test.go`:

```go
func TestWorkspaceRenders(t *testing.T) {
	m := seedModel()
	m.cursor = 0
	if cmd := m.openWorkspace(); cmd != nil {
		_ = cmd // listFiles cmd not run in test
	}
	m.workspace.files.openPath = "/workspace/main.py"
	m.workspace.editor.SetContent("print('hi')\n")
	m.setWSFocus(wsFocusEditor)
	out := m.View()
	for _, want := range []string{"FILES", "TERMINAL", "NORMAL", "main.py", "^s save"} {
		if !strings.Contains(out, want) {
			t.Errorf("workspace render missing %q", want)
		}
	}
}
```

- [ ] **Step 11: Run it**

Run: `go test ./tui/ -run TestWorkspaceRenders -v`
Expected: PASS.

- [ ] **Step 12: Commit**

```bash
git add tui/workspace.go tui/app.go tui/render_batch2_test.go
git commit -m "feat(tui): use modal Editor in workspace; Ctrl+S save; drop vim : cmdline"
```

---

## Task 5: netrw-style tree with a scrolling viewport (fixes "stuck")

**Files:**
- Modify: `tui/files.go` (add a shared scrolling tree-render helper + scroll offset)
- Modify: `tui/workspace.go:325-347` (`workspaceTree` uses the shared helper)
- Test: `tui/editor_test.go` is unit-only; add tree tests to `tui/render_batch2_test.go`

Root cause: the tree renders all nodes with no viewport; the outer `MaxHeight` clamp then clips it and the cursor moves out of view. Add a scroll offset that windows rows around the cursor.

- [ ] **Step 1: Note — no struct change needed (pure viewport)**

`treeRows` derives the visible window **purely from the cursor** and the row budget. It does NOT store a scroll offset: `View()` runs on a value copy of the model, so a persisted `scroll` field could not survive between renders anyway. No change to `fileState` is required. Proceed to the test.

- [ ] **Step 2: Write the failing viewport test**

Add to `tui/render_batch2_test.go`:

```go
func TestTreeViewportKeepsCursorVisible(t *testing.T) {
	nodes := make([]fileNode, 40)
	for i := range nodes {
		nodes[i] = fileNode{name: "f" + itoa(i), fpath: "/workspace/f" + itoa(i)}
	}
	f := fileState{dir: "/workspace", nodes: nodes, cursor: 30}
	rows := treeRows(&f, 30, 10) // width 30, 10 visible rows
	// The selected node name must be present in the windowed output.
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "f30") {
		t.Errorf("cursor row f30 not visible in viewport:\n%s", joined)
	}
	if len(rows) > 10 {
		t.Errorf("viewport returned %d rows, want <= 10", len(rows))
	}
}
```

- [ ] **Step 3: Run it to confirm it fails**

Run: `go test ./tui/ -run TestTreeViewportKeepsCursorVisible -v`
Expected: FAIL — `treeRows` undefined.

- [ ] **Step 4: Implement the shared `treeRows` helper**

Add to `tui/files.go` (after `applyFilesListed`, around line 81):

```go
// treeRows renders the netrw-style listing for f as at most `visible` rows
// (including the directory-path header row), windowed around the cursor so the
// selection is always shown. Pure: the window is derived from f.cursor only.
func treeRows(f *fileState, width, visible int) []string {
	if visible < 2 {
		visible = 2
	}
	rowsForList := visible - 1 // first row shows the dir path
	if rowsForList < 1 {
		rowsForList = 1
	}
	// Window the list so the cursor is always visible (cursor sits at the
	// bottom of the window once we've scrolled past the first page).
	scroll := 0
	if f.cursor >= rowsForList {
		scroll = f.cursor - rowsForList + 1
	}

	out := []string{stDim.Render(truncate(f.dir, width-4))}
	end := scroll + rowsForList
	if end > len(f.nodes) {
		end = len(f.nodes)
	}
	for i := scroll; i < end; i++ {
		n := f.nodes[i]
		if i == f.cursor {
			out = append(out, selectedRow(n.name, width-4))
			continue
		}
		icon := stFaint.Render(glyphTreeFile)
		name := stDim.Render(n.name)
		if n.isDir {
			icon = stHi.Render(glyphTreeClosed)
			name = stInk.Render(n.name)
		}
		out = append(out, icon+" "+name)
	}
	if len(f.nodes) == 0 {
		out = append(out, stFaint.Render("(empty)"))
	}
	return out
}
```

- [ ] **Step 5: Run the viewport test**

Run: `go test ./tui/ -run TestTreeViewportKeepsCursorVisible -v`
Expected: PASS.

- [ ] **Step 6: Use `treeRows` in the workspace tree (with height fill)**

In `tui/workspace.go`, replace `workspaceTree` (lines 325-347) with:

```go
func (m Model) workspaceTree(width, height int) string {
	f := m.workspace.files
	// panelH reserves 2 border rows + 1 title row; treeRows fills the rest.
	rows := treeRows(&f, width, height-3)
	hint := ""
	if m.workspace.focus == wsFocusTree {
		hint = "FOCUS"
	}
	return panelH(glyphPaneFiles+" FILES · netrw", hint,
		strings.Join(rows, "\n"), width, height, m.workspace.focus == wsFocusTree)
}
```

- [ ] **Step 7: Add parent/refresh keys to the workspace tree handler**

In `tui/workspace.go` `workspaceTreeKey`, add cases for parent navigation and refresh. Replace the `switch key {` block opening (lines 107-110) so the switch includes:

```go
	switch key {
	case "esc":
		m.mode = modeNormal
	case "-", "h", "left":
		parent := path.Dir(f.dir)
		f.cursor = 0
		return m, m.listFilesCmd(m.workspace.sandboxID, parent)
	case "R":
		return m, m.listFilesCmd(m.workspace.sandboxID, f.dir)
	case "j", "down":
```

This is the first use of `path` in `tui/workspace.go`, so add it to the import block:

```go
import (
	"path"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)
```

- [ ] **Step 8: Build and run the suite**

Run: `go build ./... && go test ./tui/`
Expected: PASS. (`workspaceTree` no longer references the old per-node loop.)

- [ ] **Step 9: Commit**

```bash
git add tui/files.go tui/workspace.go tui/render_batch2_test.go
git commit -m "feat(tui): scrolling netrw-style tree viewport with parent/refresh keys"
```

---

## Task 6: Auto-refresh the tree after exec and after save

**Files:**
- Modify: `tui/app.go` (`execMsg` and `fileWrittenMsg` re-list the workspace dir)

Root cause: nothing re-lists the directory after a command or a save, so created files never appear.

- [ ] **Step 1: Re-list after a workspace exec**

In `tui/app.go`, replace the `execMsg` case (lines 457-465) with:

```go
	case execMsg:
		r := (*execResultData)(msg)
		if m.mode == modeWorkspace && m.workspace.termBusy {
			m.appendTermResult(r)
			m.lastError = ""
			m.addLog("EXEC", fmt.Sprintf("exit=%d dur=%s", r.ExitCode, r.Duration))
			// A command may have created/removed files — refresh the tree.
			return m, m.listFilesCmd(m.workspace.sandboxID, m.workspace.files.dir)
		}
		m.lastExec = r
		m.lastError = ""
		m.addLog("EXEC", fmt.Sprintf("exit=%d dur=%s", r.ExitCode, r.Duration))
```

- [ ] **Step 2: Re-list after a workspace save**

In `tui/app.go`, replace the `fileWrittenMsg` case (lines 467-474) with:

```go
	case fileWrittenMsg:
		m.statusMsg = "File written"
		m.lastError = ""
		if m.mode == modeInput {
			m.files.write = false
			m.files.editorOn = false
		}
		m.addLog("WRITE", "file written")
		if m.mode == modeWorkspace {
			return m, m.listFilesCmd(m.workspace.sandboxID, m.workspace.files.dir)
		}
```

- [ ] **Step 3: Build and test**

Run: `go build ./... && go test ./tui/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add tui/app.go
git commit -m "feat(tui): refresh workspace file tree after exec and save"
```

---

## Task 7: Workspace layout — fill height + rebalance + terminal toggle

**Files:**
- Modify: `tui/workspace.go` (`renderWorkspace`, `workspaceTerminal`)

Give the editor the majority of the right column, fill every pane to its allotted height, and honor the `showTerm` toggle from Task 4.

- [ ] **Step 1: Rewrite `renderWorkspace` for full-height panes + toggle**

In `tui/workspace.go`, replace `renderWorkspace` (lines 268-290) with:

```go
func (m Model) renderWorkspace(height, width int) string {
	ctx := m.workspaceContextBar(width)

	paneH := height - 2 // context bar + breathing row
	if paneH < 8 {
		paneH = 8
	}
	treeW := 32
	if width < 130 {
		treeW = 26
	}
	rightW := width - 1 - treeW

	tree := m.workspaceTree(treeW, paneH)

	var right string
	if m.workspace.showTerm {
		editorH := paneH * 7 / 10 // editor gets the majority
		termH := paneH - editorH
		editor := m.workspaceEditor(rightW, editorH)
		term := m.workspaceTerminal(rightW, termH)
		right = lipgloss.JoinVertical(lipgloss.Left, editor, term)
	} else {
		right = m.workspaceEditor(rightW, paneH) // full-height editor
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, tree, " ", right)
	return ctx + "\n" + panes
}
```

- [ ] **Step 2: Make the terminal pane fill its height**

In `tui/workspace.go`, replace `workspaceTerminal` (lines 399-427) with:

```go
func (m Model) workspaceTerminal(width, height int) string {
	focused := m.workspace.focus == wsFocusTerm
	maxLines := height - 3 // border (2) + active prompt (1)
	if maxLines < 1 {
		maxLines = 1
	}
	start := len(m.workspace.termLines) - maxLines
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	for _, ln := range m.workspace.termLines[start:] {
		b.WriteString(ln + "\n")
	}
	prompt := m.termPrompt()
	if focused {
		prompt += m.workspace.termInput.View() + cursorBar(m.cursorOn)
	} else {
		prompt += stFaint.Render("(press 3 to focus)")
	}
	b.WriteString(prompt)

	hint := ""
	if focused {
		hint = "FOCUS"
	}
	return panelH(glyphPaneTerm+" TERMINAL · "+m.workspace.sandboxID+" (in-VM)", hint,
		b.String(), width, height, focused)
}
```

- [ ] **Step 3: Build and test**

Run: `go build ./... && go test ./tui/`
Expected: PASS.

- [ ] **Step 4: Add a terminal-toggle test**

Add to `tui/render_batch2_test.go`:

```go
func TestWorkspaceTerminalToggle(t *testing.T) {
	m := seedModel()
	m.cursor = 0
	m.openWorkspace()
	withTerm := m.View()
	if !strings.Contains(withTerm, "TERMINAL") {
		t.Fatalf("terminal pane should be visible by default")
	}
	// Toggle the terminal off.
	m.handleWorkspaceKey(tea.KeyMsg{Type: tea.KeyCtrlT})
	if m.workspace.showTerm {
		t.Fatalf("ctrl+t did not hide the terminal")
	}
	without := m.View()
	if strings.Contains(without, "TERMINAL ·") {
		t.Errorf("terminal pane still rendered after toggle off")
	}
}
```

This is the first test using `tea`, so update the `tui/render_batch2_test.go` import block to:

```go
import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)
```

- [ ] **Step 5: Run it**

Run: `go test ./tui/ -run TestWorkspaceTerminalToggle -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add tui/workspace.go tui/render_batch2_test.go
git commit -m "feat(tui): full-height workspace panes, editor-majority layout, terminal toggle"
```

---

## Task 8: Navigation routing — arrows act locally, drop arrow tab-switching

**Files:**
- Modify: `tui/app.go:563-575` (remove `left`/`right` → tab switching)
- Test: `tui/render_batch2_test.go`

Root cause of the user's complaint: `left`/`right` arrows switch the top nav tab, hijacking in-screen navigation. `1`–`6` remain the screen accelerators; `Tab`/`Shift+Tab` move panes (Task 4 already wired this for the workspace).

- [ ] **Step 1: Write the failing routing test**

Add to `tui/render_batch2_test.go`:

```go
func TestArrowsDoNotSwitchTabs(t *testing.T) {
	m := seedModel()
	m.activeTab = tabSandboxes
	m.mode = modeNormal
	m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if m.activeTab != tabSandboxes {
		t.Errorf("left arrow switched tab to %d; arrows must act locally", m.activeTab)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRight})
	if m.activeTab != tabSandboxes {
		t.Errorf("right arrow switched tab to %d; arrows must act locally", m.activeTab)
	}
	// Number keys still switch screens.
	m.handleKey(runes("3"))
	if m.activeTab != tabTemplates {
		t.Errorf("number key 3 did not switch to templates; got %d", m.activeTab)
	}
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `go test ./tui/ -run TestArrowsDoNotSwitchTabs -v`
Expected: FAIL — left arrow moves `activeTab` to `tabDashboard`.

- [ ] **Step 3: Remove the arrow→tab branch**

In `tui/app.go`, delete the `left`/`right` block inside the `if m.mode == modeNormal` section (lines 563-575):

```go
		if key == "left" || key == "right" {
			newTab := m.activeTab
			if key == "left" && m.activeTab > 0 {
				newTab--
			} else if key == "right" && m.activeTab < tabConfig {
				newTab++
			}
			if newTab != m.activeTab {
				m.activeTab = newTab
				return m, nil
			}
			return m, nil
		}
```

Delete that entire block. Leave the `1`–`6` block (lines 554-561) intact.

- [ ] **Step 4: Run the routing test + suite**

Run: `go test ./tui/ -run TestArrowsDoNotSwitchTabs -v && go test ./tui/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tui/app.go tui/render_batch2_test.go
git commit -m "fix(tui): arrows act within the screen; 1-6 switch screens (no arrow tab-hijack)"
```

---

## Task 9: Context-aware on-screen key hints

**Files:**
- Modify: `tui/chrome.go` (`renderStatusFooter` workspace branch)

New users must always see what keys do. Make the footer reflect the focused workspace pane and the editor mode.

- [ ] **Step 1: Add workspace-aware hints to the footer**

In `tui/chrome.go` `renderStatusFooter`, add a `modeWorkspace` branch at the top of the `switch` (before `case m.mode == modeConfirm`, line 113). Insert:

```go
	case m.mode == modeWorkspace:
		switch m.workspace.focus {
		case wsFocusTree:
			hints = []hint{{"j/k", "move"}, {glyphEnter, "open"}, {"-", "up"}, {"R", "refresh"}, {glyphTab, "pane"}}
		case wsFocusEditor:
			if me, ok := m.workspace.editor.(modalEditor); ok && me.Mode() == editorInsert {
				hints = []hint{{"esc", "normal"}, {"type", "edit"}}
			} else {
				hints = []hint{{"i", "insert"}, {"x/dd", "del"}, {"yy/p", "copy"}, {"u", "undo"}, {"^s", "save"}, {glyphTab, "pane"}}
			}
		case wsFocusTerm:
			hints = []hint{{glyphEnter, "run"}, {"^t", "hide term"}, {"esc", "back"}, {glyphTab, "pane"}}
		}
```

- [ ] **Step 2: Build and run the suite**

Run: `go build ./... && go test ./tui/`
Expected: PASS.

- [ ] **Step 3: Add a hint-bar test**

Add to `tui/render_batch2_test.go`:

```go
func TestWorkspaceHintsAreContextual(t *testing.T) {
	m := seedModel()
	m.cursor = 0
	m.openWorkspace()

	m.setWSFocus(wsFocusTree)
	if out := m.View(); !strings.Contains(out, "refresh") {
		t.Errorf("tree-focus footer missing tree hints")
	}
	m.setWSFocus(wsFocusEditor)
	if out := m.View(); !strings.Contains(out, "insert") || !strings.Contains(out, "save") {
		t.Errorf("editor-focus footer missing editor hints")
	}
}
```

- [ ] **Step 4: Run it**

Run: `go test ./tui/ -run TestWorkspaceHintsAreContextual -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tui/chrome.go tui/render_batch2_test.go
git commit -m "feat(tui): context-aware workspace key hints in the footer"
```

---

## Task 10: Final verification

**Files:** none (verification only)

- [ ] **Step 1: Full build + vet + test**

Run: `go build ./... && go vet ./tui/ && go test ./tui/ -v`
Expected: builds clean, vet clean, all tests PASS.

- [ ] **Step 2: Manual smoke (optional, real server)**

If a StacyVM server is running, launch the TUI, open a sandbox workspace, and verify:
- Active nav tab shows a clean label (no `[38;2;…m`).
- Panes fill the screen (no large empty gaps).
- File tree scrolls past the visible region without sticking; `-` goes up; `R` refreshes.
- Creating a file in the terminal makes it appear in the tree.
- Editor: `i` to type, `Esc`, `x`/`dd`/`yy`/`p`/`u` behave; `Ctrl+S` saves.
- `Tab`/`Shift+Tab` move panes; `Ctrl+T` hides/shows the terminal; arrows no longer switch tabs.
- The footer hints change per focused pane and editor mode.

- [ ] **Step 3: Final commit (if any docs/notes changed)**

```bash
git add -A
git commit -m "docs(tui): note workspace editor & navigation behavior" || true
```

---

## Notes & Known Limitations

- Word motion (`w`/`b`) uses textarea's own word navigation; behavior matches textarea, not exact Vim word semantics. Acceptable for an auxiliary editor (per spec).
- `yy`/`p` use a single clipboard slot; `u` is a bounded per-operation snapshot stack — not Vim registers or an undo tree (out of scope by design).
- The standalone Files browser (`modeInput`) keeps its existing READ/WRITE textarea; only the Workspace adopts the modal `Editor` in this plan. Shared fixes (nav, `selectedRow`, tree viewport, `panelH`) still benefit it.
- `tview` is intentionally NOT adopted (incompatible stack — see spec §7); its focus model is implemented natively via the pane focus ring.
