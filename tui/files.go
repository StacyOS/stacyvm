package tui

// files.go — screen 8, "Files" (Build 1 A): a real file TREE on the left and a
// numbered, syntax-highlighted code pane on the right with an explicit
// READ / WRITE mode indicator so writes are never accidental.

import (
	"path"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type fileNode struct {
	name  string
	fpath string
	isDir bool
}

type fileState struct {
	sandboxID string
	dir       string
	nodes     []fileNode
	cursor    int
	openPath  string
	content   string
	write     bool
	editor    textarea.Model
	editorOn  bool
	err       string
}

type filesListedMsg struct {
	dir   string
	files []fileInfoData
}

// startFiles enters the files browser for a sandbox and lists its workspace.
func (m *Model) startFiles(sandboxID string) tea.Cmd {
	m.mode = modeInput
	m.files = fileState{sandboxID: sandboxID, dir: "/workspace"}
	return m.listFilesCmd(sandboxID, "/workspace")
}

func (m Model) listFilesCmd(id, dir string) tea.Cmd {
	return func() tea.Msg {
		files, err := m.client.listFiles(id, dir)
		if err != nil {
			return filesListedMsg{dir: dir, files: nil}
		}
		return filesListedMsg{dir: dir, files: files}
	}
}

// activeFiles returns the file-state in play: the Workspace's tree when the
// Workspace is open, otherwise the standalone Files browser.
func (m *Model) activeFiles() *fileState {
	if m.mode == modeWorkspace {
		return &m.workspace.files
	}
	return &m.files
}

// applyFilesListed populates the active tree from a directory listing.
func (m *Model) applyFilesListed(msg filesListedMsg) {
	f := m.activeFiles()
	f.dir = msg.dir
	nodes := make([]fileNode, 0, len(msg.files)+1)
	if msg.dir != "/" && msg.dir != "" {
		nodes = append(nodes, fileNode{name: "..", fpath: path.Dir(msg.dir), isDir: true})
	}
	for _, fi := range msg.files {
		nodes = append(nodes, fileNode{name: path.Base(fi.Path), fpath: fi.Path, isDir: fi.IsDir})
	}
	f.nodes = nodes
	if f.cursor >= len(nodes) {
		f.cursor = max(0, len(nodes)-1)
	}
}

// handleFilesKey drives the files browser (navigation + READ/WRITE toggle).
func (m *Model) handleFilesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// In WRITE mode the editor owns most keys.
	if m.files.write && m.files.editorOn {
		switch key {
		case "esc":
			m.files.write = false
			m.files.editorOn = false
			return m, nil
		case "ctrl+o":
			m.files.write = false
			m.files.editorOn = false
			if m.files.openPath != "" {
				return m, m.readFileCmd(m.files.sandboxID, m.files.openPath)
			}
			return m, nil
		case "ctrl+s":
			content := m.files.editor.Value()
			m.files.content = content
			return m, m.writeFileCmd(m.files.sandboxID, m.files.openPath, content)
		}
		var cmd tea.Cmd
		m.files.editor, cmd = m.files.editor.Update(msg)
		return m, cmd
	}

	switch key {
	case "esc":
		m.mode = modeNormal
		return m, nil
	case "j", "down":
		if m.files.cursor < len(m.files.nodes)-1 {
			m.files.cursor++
		}
	case "k", "up":
		if m.files.cursor > 0 {
			m.files.cursor--
		}
	case "enter":
		if m.files.cursor < len(m.files.nodes) {
			n := m.files.nodes[m.files.cursor]
			if n.isDir {
				m.files.cursor = 0
				return m, m.listFilesCmd(m.files.sandboxID, n.fpath)
			}
			m.files.openPath = n.fpath
			m.files.write = false
			return m, m.readFileCmd(m.files.sandboxID, n.fpath)
		}
	case "ctrl+o":
		m.files.write = false
		m.files.editorOn = false
		if m.files.openPath != "" {
			return m, m.readFileCmd(m.files.sandboxID, m.files.openPath)
		}
	case "ctrl+s":
		if m.files.openPath == "" {
			return m, nil
		}
		// Enter WRITE mode: load current content into an editor.
		ta := textarea.New()
		ta.SetValue(m.files.content)
		ta.Focus()
		m.files.editor = ta
		m.files.write = true
		m.files.editorOn = true
		return m, textarea.Blink
	}
	return m, nil
}

// ── render ──────────────────────────────────────────────────────────────────

func (m Model) renderFiles(height, width int) string {
	leftW := 34
	if width < 120 {
		leftW = 26
	}
	rightW := width - 1 - leftW

	tree := m.filesTree(leftW)
	editor := m.filesEditor(rightW, height)
	return joinH(tree, editor)
}

func (m Model) filesTree(width int) string {
	var rows []string
	rows = append(rows, stDim.Render(truncate(m.files.dir, width-4)))
	for i, n := range m.files.nodes {
		icon := stFaint.Render(glyphTreeFile)
		name := stDim.Render(n.name)
		if n.isDir {
			icon = stHi.Render(glyphTreeClosed)
			name = stInk.Render(n.name)
		}
		row := icon + " " + name
		if i == m.files.cursor {
			rows = append(rows, selectedRow(n.name, width-4))
		} else {
			rows = append(rows, row)
		}
	}
	if len(m.files.nodes) == 0 {
		rows = append(rows, stFaint.Render("(empty)"))
	}
	body := strings.Join(rows, "\n")
	body += "\n\n" + keyHints([]hint{{"j/k", "move"}, {glyphEnter, "open"}})
	return panel("TREE", "", body, width, false)
}

func (m Model) filesEditor(width, height int) string {
	title := glyphPaneSpawn + " " + orDash(m.files.openPath)
	if m.files.openPath == "" {
		return panel(title, "EDIT", stFaint.Render("select a file in the tree"), width, true)
	}

	var body string
	if m.files.write && m.files.editorOn {
		m.files.editor.SetWidth(width - 6)
		m.files.editor.SetHeight(max(3, height-8))
		body = m.files.editor.View()
	} else {
		lines := strings.Split(strings.TrimRight(m.files.content, "\n"), "\n")
		maxLines := height - 8
		if maxLines < 3 {
			maxLines = 3
		}
		var b strings.Builder
		for i, ln := range lines {
			if i >= maxLines {
				b.WriteString(stFaint.Render("  …\n"))
				break
			}
			num := stFaint.Render(padLeft(itoa(i+1), 3))
			b.WriteString(num + "  " + highlightLine(ln) + "\n")
		}
		body = strings.TrimRight(b.String(), "\n")
	}

	// Explicit mode chip + key affordances.
	var chip string
	if m.files.write {
		chip = stHiB.Render(glyphDotRun + " WRITE mode")
	} else {
		chip = stDim.Render(glyphDotIdle + " READ mode")
	}
	footer := chip + "   " + keyHints([]hint{{"^o", "read"}, {"^s", "save"}, {"esc", "back"}})
	return panel(title, "EDIT", body+"\n\n"+footer, width, true)
}

// joinH places two bordered panes side by side with a one-cell gap.
func joinH(left, right string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

// ── lightweight syntax highlighter ─────────────────────────────────────────
//
// Heuristic Python-ish coloring (keywords orange, strings green, comments
// faint, def/class names mint) — enough to read code in the pane.

var codeKeywords = map[string]bool{
	"import": true, "from": true, "def": true, "class": true, "return": true,
	"if": true, "elif": true, "else": true, "for": true, "while": true, "in": true,
	"and": true, "or": true, "not": true, "with": true, "as": true, "try": true,
	"except": true, "finally": true, "lambda": true, "None": true, "True": true,
	"False": true, "pass": true, "break": true, "continue": true, "raise": true,
	"yield": true, "global": true, "async": true, "await": true, "func": true,
	"package": true, "var": true, "const": true, "type": true, "struct": true,
}

func isWordRune(r byte) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func highlightLine(s string) string {
	var b strings.Builder
	prevWord := ""
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '#': // comment to end of line
			b.WriteString(stFaint.Render(s[i:]))
			return b.String()
		case c == '"' || c == '\'': // string literal
			q := c
			j := i + 1
			for j < len(s) && s[j] != q {
				j++
			}
			if j < len(s) {
				j++ // include closing quote
			}
			b.WriteString(stOK.Render(s[i:j]))
			i = j
			prevWord = ""
		case isWordRune(c): // word: keyword / def-name / plain
			j := i
			for j < len(s) && isWordRune(s[j]) {
				j++
			}
			word := s[i:j]
			switch {
			case codeKeywords[word]:
				b.WriteString(stHi.Render(word))
			case prevWord == "def" || prevWord == "class" || prevWord == "func":
				b.WriteString(stMint.Render(word))
			default:
				b.WriteString(stInk.Render(word))
			}
			prevWord = word
			i = j
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}
