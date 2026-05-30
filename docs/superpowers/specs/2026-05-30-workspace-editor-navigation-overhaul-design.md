# Workspace Editing & Navigation Overhaul — Design

Date: 2026-05-30
Status: Approved (pending spec review)
Area: `tui/` (Bubble Tea TUI)

## 1. Context & Problem

The StacyVM TUI is a Bubble Tea app (`charmbracelet/bubbletea` + `lipgloss` +
`bubbles`). The Sandbox Workspace screen (`tui/workspace.go`) hosts three panes —
file tree, a "vim-style" editor, and an in-VM terminal. Several UX defects and a
feature request were raised:

1. **Active nav tab renders garbage.** The selected top-nav item shows literal
   text like `[38;2;73;73;81m2[0m SANDBOXES` instead of a styled label.
2. **Wasted vertical space.** The editor/terminal/tree boxes do not fill their
   allotted height, leaving large empty gaps.
3. **File tree gets "stuck" going down and doesn't show files.** Navigation past
   the visible region appears frozen.
4. **Tree doesn't refresh** when files are created/changed from the terminal.
5. **Editor should be a better, familiar Vim-like experience**, bigger, and fast.
6. **Pane-vs-tab navigation confusion.** On multi-pane screens, pressing the
   left arrow to move to the left *pane* instead switches the left *tab*.

### Root causes (confirmed in code)

- **#1 ANSI leak:** `renderNav` (`tui/chrome.go:69`) pre-renders the tab number
  into a styled string (embedded ANSI), then wraps that already-styled string in
  a *second* lipgloss style with `Underline(true)` + `Background`. This lipgloss
  version mangles the embedded escape (strips the `ESC` byte) when re-applying
  underline/background over pre-styled content. Root cause: **styled-in-styled
  nesting**.
- **#2 wasted space:** `panel()` (`tui/kit.go:207`) borders whatever content it
  is given and **never pads to a target height**; pane bodies emit only as many
  lines as they have content, so short files produce short boxes.
- **#3 stuck tree:** `workspaceTree` (`tui/workspace.go:325`) renders **all**
  nodes with **no scrolling viewport**; when the listing exceeds the pane the
  outer `MaxHeight` clamp (`tui/app.go:917`) clips it and the cursor moves out of
  view.
- **#4 no refresh:** the `execMsg` handler (`tui/app.go:457`) never re-lists the
  directory after a command runs.
- **#6 navigation:** `handleKey` (`tui/app.go:563`) binds `left`/`right` arrows
  to switching `m.activeTab`, colliding with the expectation that arrows move
  between panes.

## 2. Goals & Non-Goals

### Goals

- A **stable, familiar, Vim-like** auxiliary editor with **minimal maintenance
  burden** — "~80% of the perceived Vim experience for ~20% of the effort."
- Fix all six issues above with **no UI glitches**.
- A **consistent navigation model** across every multi-pane screen, with
  **on-screen key hints** so new users are never confused.
- A clean **`Editor` interface** seam so the implementation can later evolve
  (`TextareaEditor → CustomEditor → Embedded Vim`) without touching the rest of
  the app.

### Non-Goals (explicitly out of scope)

Do **not** build a full Vim clone or a custom editor engine. Excluded:
registers, macros, dot-repeat, text objects (`ciw`/`diw`), Visual mode, Visual
block, marks, Ex commands (`:`), multiple buffers, splits, plugin systems, full
Vim compatibility. Do **not** implement a custom text buffer, custom viewport, or
custom cursor-sync logic — `textarea` owns all low-level editing.

The editor is **auxiliary**, not the primary product. Priorities, in order:
**stability, maintainability, predictable behavior, clean architecture.** Do not
optimize for Vim completeness.

## 3. Architecture: Editor Abstraction + Modal Layer (Approach A)

The app depends only on this interface; `textarea` does all low-level editing.

```go
type Editor interface {
    SetContent(string)
    Value() string
    SetSize(width, height int)   // fills the pane (fixes wasted space)
    Update(tea.Msg) tea.Cmd      // returns cmd so textarea blink/scroll work
    View() string
    Focus()
    Blur()
}
```

(`SetSize` and the `tea.Cmd` return on `Update` are approved additions to the
originally-proposed interface; both are required for correct behavior.)

### `TextareaEditor` (first and only implementation for now)

Wraps a `bubbles/textarea.Model` plus:

- `mode` — `normal` | `insert`.
- `clipboard string` — a **single slot** for `yy`/`p` (not a register file).
- `undo []string` — a **bounded snapshot stack** for `u`. Snapshots are taken
  **per mutating NORMAL-mode operation** (before `x`, `dd`, `p`, and on entering
  INSERT via `i` — restored as one unit on `Esc`), **not per keystroke**. The
  stack is capped (e.g. 50 entries) to bound memory.

All editing is driven through `textarea`'s public API — confirmed available:
`SetValue`, `InsertString`, `Value`, `Line`, `LineCount`, `CursorUp`,
`CursorDown`, `CursorStart`, `CursorEnd`, `SetCursor`, `LineInfo`, `SetWidth`,
`SetHeight`, `Focus`, `Blur`. No custom buffer/viewport/cursor-sync.

#### Modes & commands

**INSERT mode** — `textarea` owns all keys (text entry, Backspace, Enter, arrow
keys). `Esc` → NORMAL.

**NORMAL mode** — the modal layer intercepts keys and drives `textarea`:

| Key   | Action                          | Implementation |
|-------|---------------------------------|----------------|
| `h`   | cursor left                     | synth `left` key / `SetCursor` |
| `l`   | cursor right                    | synth `right` key / `SetCursor` |
| `j`   | cursor down                     | `CursorDown()` |
| `k`   | cursor up                       | `CursorUp()` |
| `w`   | next word start (within line)   | compute column from current line, `SetCursor` |
| `b`   | prev word start (within line)   | compute column from current line, `SetCursor` |
| `0`   | line start                      | `CursorStart()` |
| `$`   | line end                        | `CursorEnd()` |
| `x`   | delete char under cursor        | snapshot → synth `delete` |
| `dd`  | delete current line → clipboard | snapshot → splice via `Value()/SetValue()` |
| `yy`  | yank current line → clipboard   | copy line text |
| `p`   | paste clipboard below line      | snapshot → splice via `Value()/SetValue()` |
| `u`   | undo last operation             | pop snapshot stack → `SetValue` |
| `i`   | enter INSERT                    | snapshot → `mode = insert` |
| `Esc` | (already NORMAL) no-op          | — |

`dd`/`yy` use a two-key pending state (`d`/`y` then the second key); any other
key cancels the pending operator. Word motion (`w`/`b`) is a simple within-line
boundary scan (covers the common case); crossing lines is acceptable to omit.

#### Saving & leaving

Ex commands are out of scope, so:

- **Save** = `Ctrl+S` (writes the file to the sandbox; triggers a tree refresh).
- **Leave the editor pane** = `Esc` to NORMAL, then `Tab` to move focus.

The existing `:w`/`:q` command-line in `tui/workspace.go` is **removed**.

## 4. netrw-style File Tree

Keep the single-directory, netrw-style listing (navigate **into** directories
rather than an always-expanded tree), but make it robust:

- **Scrolling viewport:** maintain a scroll offset and window the visible rows
  around the cursor so the selection is always on-screen — fixes "stuck."
- **Keys:** `j`/`k` move; `Enter` or `l` opens a file (focus moves to editor) or
  descends into a directory; `-` or `h` goes to the parent; `R` refreshes.
- **Auto-refresh** the current directory after: (a) a successful terminal
  `exec`, and (b) a file save. Fixes "created a file, tree didn't update."
- Body is padded to **fill the pane height** (see §5).

## 5. Layout / Space

- Add a **height-filling panel helper** (or a `height` parameter to `panel()`)
  that pads the body to an exact line count so each box spans its full allotted
  height. Root-cause fix for the empty gaps.
- **Rebalance the right column:** give the **editor the majority** of the
  height; the terminal becomes a compact strip. Add a key to **toggle the
  terminal off** for a full-height editor.
- `Editor.SetSize(w, h)` sizes the underlying `textarea` to fill the pane →
  "bigger vim space."

## 6. Navigation Model

One consistent model on every multi-pane screen (sandboxes `list|inspect`,
files `tree|editor`, workspace `tree|editor|terminal`):

- **Switch screens (top tabs):** number keys `1`–`6` jump directly.
  **Arrow keys no longer switch tabs** (removes the bug). Prev/next-screen
  bracket keys are **not** included in v1.
- **Move between panes within a screen:** `Tab` / `Shift+Tab` cycle the focus
  ring. The focused pane keeps its accent border + `FOCUS` tag. (This is tview's
  focus model implemented natively.)
- **Within the focused pane:** arrows / `hjkl` act locally (selection or editor
  cursor); `Enter` activates; `Esc` goes up a level. Editor INSERT mode owns all
  keys, including arrows.

**Workspace is a focused sub-view.** While the workspace is open it owns all keys
(`tui/app.go:548`), so global `1`–`6` screen-switching is suspended there: pane
movement is `Tab` / `Shift+Tab` and `Esc` returns to the sandbox list. The
existing `1`/`2`/`3` direct-pane jumps are retained as a convenience inside the
workspace only (no screen is reachable from inside it), so there is no clash with
the top-level `1`–`6` screen accelerators. Top-level screens (dashboard,
sandboxes list, templates, etc.) use `1`–`6` to switch and `Tab` to cycle panes.

### On-screen navigation hints (required)

Users must always see what keys do. A **context-aware key-hint bar** reflects the
current focus and mode:

- The status footer (`renderStatusFooter`, `tui/chrome.go:107`) shows hints for
  the active screen/pane. In the workspace it reflects the **focused pane** and,
  for the editor, the **NORMAL vs INSERT** mode, e.g.:
  - Tree focus: `j/k move · ↵ open · - up · R refresh · Tab pane · 1-6 screen`
  - Editor NORMAL: `h/j/k/l move · i insert · x del · dd/yy/p · u undo · ^s save · Esc · Tab pane`
  - Editor INSERT: `type to edit · Esc normal`
  - Terminal focus: `↵ run · Esc back · Tab pane`
- The editor **modeline** (in-pane) shows the current mode badge and its key
  affordances (replacing the old `i insert · :w save · :q close` text with the
  real bindings: `i insert · ^s save · Esc normal`).

## 7. tview Decision

**Do not adopt `rivo/tview`.** It is built on `tcell`, a different stack from the
existing app (Bubble Tea + lipgloss + bubbles). The two frameworks each own the
event loop and rendering and cannot interoperate — adopting tview means a **full
TUI rewrite**, contrary to the stability / minimal-maintenance goals. The value
tview offers here is its **focus management**, which §6 provides natively.

## 8. Bug #1 + Glitch Sweep

- **Nav fix:** render each nav segment **once from raw (unstyled) text** so no
  pre-styled fragment is nested inside a `Background`/`Underline` style.
- **Audit** `selectedRow` and the context-bar / footer builders for the same
  `style-over-pre-styled` pattern; fix any that can leak.

## 9. Components & Affected Files

- `tui/editor.go` *(new)* — `Editor` interface + `TextareaEditor` (modal layer,
  clipboard, undo stack).
- `tui/workspace.go` — use `Editor`; remove `:` cmdline; pane focus ring;
  terminal toggle; height-filling layout; refresh-after-exec wiring.
- `tui/files.go` — netrw tree viewport + scroll offset + refresh triggers;
  optionally share the tree-render helper with the workspace.
- `tui/kit.go` — height-filling panel helper.
- `tui/chrome.go` — nav segment fix; context-aware footer hints.
- `tui/app.go` — navigation routing (arrows local, `1`–`6` screens, `Tab`
  pane focus); re-list dir after `execMsg` in the workspace.

## 10. Testing

Extend the existing render tests (`tui/render_test.go`,
`tui/render_batch1_test.go`, `tui/render_batch2_test.go`):

- **Editor modal layer (unit):** mode transitions; each NORMAL command's effect
  on `Value()`; `yy`/`p` clipboard; `u` restores the prior snapshot; `Ctrl+S`
  emits a write.
- **Tree viewport:** with more nodes than rows, the cursor stays visible while
  moving down; never clips/sticks.
- **Navigation routing:** arrow keys do **not** change `activeTab`; `Tab`
  cycles pane focus; `1`–`6` switch screens.
- **Glitch regression:** rendered nav contains **no literal `\x1b[` / `[38;2`**
  as visible text; panes fill their allotted height.
- **Hint bar:** the footer shows the correct hints for each focused pane / editor
  mode.

## 11. Risks & Mitigations

- **Word motion fidelity (`w`/`b`):** kept simple (within-line). Acceptable for an
  auxiliary editor; documented as a known limitation.
- **textarea API drift:** we depend on documented public methods only; pinned at
  `bubbles v1.0.0`.
- **Focus-ring consistency:** centralize the pane-focus logic so all multi-pane
  screens share one implementation, avoiding per-screen divergence.
