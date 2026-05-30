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
