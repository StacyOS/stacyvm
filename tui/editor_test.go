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
