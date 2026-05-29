# Handoff: StacyVM TUI — Mission Control (refined build)

## Overview

StacyVM is a **microVM sandbox orchestrator for LLMs**. This handoff covers the
TUI (terminal UI) front-end: a cockpit for spawning, inspecting, and living
inside sandboxes.

The design landed in **two builds**, and the full app is the **union of both** —
implement everything below:

- **Build 1 — "Mission Control" (Direction A, the SELECTED direction).** The
  approved overall structure and the *complete* navigation: a full-width
  telemetry ribbon + a `DASH / SANDBOXES / TEMPLATES / PROVIDERS / LOGS /
  CONFIG` nav with a `⌘K` command palette. This build defines **all six nav
  destinations plus their sub-screens** (sandbox inspect drawer, spawn modal,
  exec, files). Two other explorations (B "HUD Grid", C "Stream") were
  *rejected* — they're included only for reference and should NOT be built.
- **Build 2 — "Refined".** A polish pass that took the four most important
  surfaces from Build 1 and made them live/animated and higher-fidelity:
  the **Dashboard**, a new deep **Sandbox Workspace** (tree + vim + terminal),
  the **Spawn sequence** animation, and the **Boot splash** animation.

**Where the two builds overlap (Dashboard, Spawn), Build 2 wins** — it's the
newer, refined spec. For everything Build 2 didn't touch (Templates, Providers,
Logs, Config, Sandboxes list/inspect, Exec, standalone Files), **Build 1
Direction A is the spec.**

### Complete screen inventory (build everything in this table)

| # | Screen | Nav home | Spec source |
|---|--------|----------|-------------|
| 1 | **Dashboard / Mission Control** | `DASH` | Build 2 (refined) → `v2-dashboard.jsx` |
| 2 | **Sandbox Workspace** (tree + vim + in-VM terminal) | `SANDBOXES` → open | Build 2 → `v2-workspace.jsx` |
| 3 | **Spawn sequence** (animated lifecycle) | `SANDBOXES` → spawn | Build 2 → `v2-spawn.jsx` |
| 4 | **Boot splash** (animated open) | app launch | Build 2 → `v2-boot.jsx` |
| 5 | **Sandboxes — list + inspect drawer** | `SANDBOXES` | Build 1 A → `direction-a.jsx` |
| 6 | **Spawn — floating modal** (quick form) | `SANDBOXES` → spawn | Build 1 A → `direction-a.jsx` |
| 7 | **Exec — command + framed output** | `SANDBOXES` → exec | Build 1 A → `direction-a.jsx` |
| 8 | **Files — explicit READ/WRITE mode** | `SANDBOXES` → files | Build 1 A → `direction-a.jsx` |
| 9 | **Templates — table + detail** | `TEMPLATES` | Build 1 A → `direction-a.jsx` |
| 10 | **Providers — health cards** | `PROVIDERS` | Build 1 A → `direction-a.jsx` |
| 11 | **Logs — filterable event stream** | `LOGS` | Build 1 A → `direction-a.jsx` |
| 12 | **Config — segmented controls, live patch** | `CONFIG` | Build 1 A → `direction-a.jsx` |

> Screens 1–4 are documented in detail under **"Build 2 screens"**; screens
> 5–12 under **"Build 1 (Direction A) screens"**. Screens 2/3 (Workspace,
> animated Spawn) are the refined evolution of Build 1's Files/Exec and Spawn
> modal — keep the richer Build 2 versions as the primary experience, and treat
> the Build 1 Spawn modal (#6) as the lightweight "quick spawn" form that kicks
> off the #3 animation.

---

## ⚠️ Read this first — the target is a terminal app, not a web page

**The files in `mockup/` are design references created in HTML/React.** They
are prototypes that show the intended *look, layout, color, and motion* of the
TUI — they are **not** production code to copy.

This is a **terminal application**. The design notes throughout the mockup
explicitly reference the **Charm / Bubble Tea** Go stack:

- **[Bubble Tea](https://github.com/charmbracelet/bubbletea)** — the Elm-style
  TUI runtime (`Model` / `Update` / `View`, `tea.Msg`, `tea.Cmd`).
- **[Lip Gloss](https://github.com/charmbracelet/lipgloss)** — styling: colors,
  borders, padding, layout (`JoinHorizontal` / `JoinVertical`).
- **[Bubbles](https://github.com/charmbracelet/bubbles)** — ready components:
  `table`, `viewport`, `textinput`, `spinner`, `progress`, `filetree`-style
  list, `help`, `key`.
- **[Harmonica](https://github.com/charmbracelet/harmonica)** — spring physics
  for the boot/spawn animations.

**The task is to recreate these designs as a Bubble Tea TUI** using that stack
(or, if the team has already chosen a different TUI toolkit, that one). Map each
mockup screen to a Bubble Tea `Model`. Treat all the HTML/CSS as a visual spec
to match in the terminal — exact glyphs, color roles, alignment, and the
animation choreography.

### What in the mockup is NOT part of the TUI (presentation scaffolding only)

These exist only so the design can be reviewed in a browser. **Do not build
them into the terminal app:**

- The sticky **top bar** with the "ANNOTATIONS / SCANLINES" toggle buttons.
- The **segmented tab strip** (`1 DASHBOARD · 2 SANDBOX WORKSPACE · …`) that
  switches between the four mockup screens — this is a demo selector, not an
  app nav. (The app's *real* nav is the in-screen `DASH / SANDBOXES / TEMPLATES
  / …` ribbon row — see Dashboard below.)
- The **section headers, kickers, and lede paragraphs** above each screen.
- The handwritten **annotation notes** (Kalam cursive font, orange corner
  ticks). The `Kalam` font is *only* for these notes — the TUI itself is 100%
  monospace.
- Browser-only chrome on the `.screen` frame: drop shadows, the page background
  radial gradients + dotted grid, `backdrop-filter` blur, rounded corners, and
  the macOS-style corner brackets/dots. A terminal has none of these — render
  edge-to-edge in the terminal cell grid.
- The optional **CRT scanline** overlay (`body.crt`) is a fun toggle, not a
  requirement.

Everything *inside* the `.scr-body` (the dark panel content) — the ribbon,
tables, meters, sparklines, tree, editor, terminal, modeline, progress bars —
**is** the TUI and should be reproduced faithfully.

---

## Fidelity

**High-fidelity.** Colors, glyphs, spacing intent, layout proportions, and the
animation choreography are final. Match them. Because this is a terminal,
"pixel-perfect" means **cell-perfect**: same characters, same color roles, same
column alignment, same relative panel sizing.

---

## Design tokens

### Color palette (ANSI / truecolor)

Terminals should use truecolor (24-bit) hex where the emulator supports it, with
a sensible 256-color fallback. Roles, not just values:

| Token        | Hex       | Role |
|--------------|-----------|------|
| `bg`         | `#08080a` | Terminal background (near-black, faint warm tint) |
| `panel`      | `#0c0c10` | Panel / screen fill |
| `panel-2`    | `#101016` | Chrome bars, keycaps, modeline, selected rows bg |
| `ink`        | `#ECE7DD` | Primary foreground text (warm off-white) |
| `dim`        | `#7d7d86` | Secondary text / labels |
| `faint`      | `#4a4a52` | Tertiary text, empty meter cells, separators |
| `line`       | `#23232b` | Borders / dividers |
| `line-2`     | `#33333d` | Stronger borders, keycap edges |
| **`orange`** | `#FFA60C` | **Primary accent** — selection, focus, active nav, KPI highlights, cursor, logo |
| `orange-2`   | `#FF7A1A` | Secondary accent (active tab number) |
| `green` (ok) | `#22C55E` | Success / running / ready / "ONLINE" |
| `red` (err)  | `#FF4747` | Errors, deletions (git `D`), kill |
| `mint`       | `#D7F6E2` | Function names in code, "TEMPLATE" events |
| `steel`      | `#5b6b78` | Paths, muted code, bracket-frame corners |
| `note`       | `#E9C893` | Annotation text only (NOT in TUI) |

Orange is the single dominant accent — used sparingly for *the focused / active
/ live* thing. Green is the only other semantic color (health/success). Keep
everything else in the warm grayscale ramp.

### Typography
- The entire TUI is **monospace**. Mockup uses **JetBrains Mono** as the
  reference cell font, but in a terminal the font is the user's terminal font —
  you only control *weight emphasis* (bold) and *color*, not the typeface.
- Use **bold** for: IDs (`sb-7f3a91`), big KPI metrics, active prompts, vim
  mode badge, "READY".
- The mockup's px font-sizes (e.g. metric `32px`, body `12.5px`) express
  *relative hierarchy*, not literal sizes. In a terminal, hierarchy comes from
  **bold + color + spacing + UPPERCASE labels with letter-spacing**, not point
  size. Translate accordingly (e.g. the "big metric" becomes a bold colored
  number sitting alone on its line inside the tile).

### Glyph vocabulary (use these exact characters)
- Status dots: `●` running/online · `◐` creating/booting/in-progress ·
  `○` idle/standby · `◉` live.
- Meters / progress bars: filled `█`, empty `░`. Width is in cells
  (e.g. host telemetry meters are 12 cells; KPI/ttl meters 6–7; progress bars
  22–26).
- Sparklines: the 8-step ramp `▁▂▃▄▅▆▇█` mapped from value/max.
- Tree: `▾` open dir · `▸` closed dir · `·` file. Git flags: `M` (orange),
  `A` (green), `D` (red).
- Checklist: `✓` done (green) · `◐` active (orange) · `○` pending (faint) ·
  `· · ·` in-progress · `—` not-started.
- Bracket-frame corners (Workspace/Spawn tiles): `⌜ ⌝ ⌞ ⌟` (steel; orange when
  the tile is focused/accent).
- Pane title icons: `◧` files · `◨` editor · `◰` terminal · `◇`/`◐` spawn tiles
  · `▣` sandbox id chip.
- Cursor: a solid block that **blinks** (≈1.05s step). Bar cursor in insert
  fields; block cursor `▉`-style in the terminal prompt.

### Spacing & layout
- Panels separate by single-cell borders (`line` color) and one blank row of
  breathing room. Section labels are UPPERCASE with wide letter-spacing in
  `dim`.
- Reference terminal size shown in chrome: **198×52** (cols×rows), alt-screen.
  Design for ~180–200 cols but degrade gracefully (see Responsive).

---

## Build 2 screens (refined, animated — the primary experience)

### 1. Dashboard — "Mission Control"
**File:** `mockup/v2-dashboard.jsx`
**Purpose:** Fleet overview + entry point into any sandbox.

**Layout (top → bottom):**
1. **Telemetry ribbon** (full width): left = logo mark + `STACYVM` +
   `MISSION CONTROL`; right = `CPU ▁▂▃… 34%`, `MEM … 61%`, `LOAD …` (green
   spark), live `HH:MM:SS` clock, `● ONLINE v0.9.2` (green). The CPU/MEM/LOAD
   sparklines and the clock **tick live** — in Bubble Tea this is a
   `tea.Tick` loop emitting a `tickMsg` (~1s) that drifts a windowed slice of
   values and re-renders.
2. **Nav ribbon:** `1 DASH · 2 SANDBOXES · 3 TEMPLATES · 4 PROVIDERS ·
   5 LOGS · 6 CONFIG`, right-aligned `⌘K command`. Active item = orange text +
   orange bottom underline + faint orange bg. **This is the app's real
   navigation** (numbers are accelerators).
3. **KPI strip** — 4 equal bracket-framed tiles:
   - `SANDBOXES` → `5` (green), sub "4 running · 1 booting" *(accent/orange
     frame)*
   - `TEMPLATES` → `4` (orange), sub "3 warm in pool"
   - `PROVIDERS` → `2/3` (green), sub "docker · firecracker"
   - `UPTIME` → `4d`, sub "since 17:14 · 312 spawns"
   Each tile shows the big metric on the left and a live sparkline on the right.
4. **Two-column body** (`~1.55fr / 1fr`):
   - **Left — ACTIVE SANDBOXES** (orange/accent module, header right hint
     `↵ OPEN WORKSPACE`): a table with columns `ID · STATE · PROVIDER · IMAGE ·
     CPU · TTL`. Row 0 is selected (orange bg + left orange bar). STATE shows
     dot+label colored by state (`● run` green, `◐ creating` orange,
     `○ idle` dim). CPU column is a 6-cell mini-meter (orange if >60%, else
     green; `—` while creating). Below the table, a keyhint row:
     `s spawn · e exec · f files · d kill · ↵ open workspace`.
     **Selecting a row (↵ or click) opens that sandbox's Workspace.**
   - **Right — stacked modules:** `HOST TELEMETRY` (CPU/MEM/DISK 12-cell meters
     + NET sparkline `1.2MB/s`), `PROVIDERS` (docker default/runsc 12ms;
     firecracker kvm 8ms; proot standby), `EVENT STREAM` (`◉ live`, timestamped
     lines: `SPAWN` orange, `EXEC` green, `WRITE` steel, `TEMPLATE` mint).
5. **Status footer:** `5 sandboxes · 4 templates · 2 providers` … keyhints
   `⌘K command · ? help · q quit` … `✓ sb-9d11ba ready` (green).

**Sample data** (use verbatim as seed data):
```
sb-7f3a91  run       docker        python:3.12     71%  24m   (selected)
sb-2c8e04  run       docker        node:20         38%  12m
sb-9d11ba  creating  firecracker   alpine:latest   —    30m
sb-4a77e2  run       proot         ubuntu:22.04    12%  08m
sb-1e90cf  idle      docker        rust:1.78        3%  55m
```

### 2. Sandbox Workspace
**File:** `mockup/v2-workspace.jsx`
**Purpose:** Live inside one sandbox. Three **focusable panes**; `Tab` cycles
focus, the focused pane gets the orange bracket-frame glow + a `FOCUS` tag.

**Layout:**
- Ribbon + nav (nav active = `SANDBOXES`).
- **Context bar:** `▣ sb-7f3a91` (orange bold) · `● running` (green) ·
  `python:3.12-alpine` · `via docker(runsc)` · `ttl ███░░░ 06m`. Right side:
  pane selectors `1 files · 2 editor · 3 terminal · ⇥ cycle` (active one
  orange).
- **Pane grid** (`gridTemplateColumns: auto 1fr`, two rows; tree spans both
  rows on the left):
  - **① Files — `◧ FILES · lazytree`:** indented tree (13px/2-space indent per
    depth). Dirs use `▾/▸` (orange icon, ink name), files use `·` (faint).
    Right-aligned git flags `M/A/D`. Selected row (`main.py`) = orange bg +
    left orange bar + orange name. Footer keyhint `j k move · ↵ open · a new`.
  - **② Editor — `◨ /workspace/src/main.py`:** vim-style. Right-aligned line
    numbers (faint; current line orange). Current line (line 6) has a subtle
    highlight bg. Python syntax colors: keywords orange (`kw`), strings green
    (`str`), function names mint (`fn`), comments faint (`cm`). A **modeline**
    sits at the bottom: a mode badge — `NORMAL` (orange bg) or `-- INSERT --`
    (green bg) — then `main.py · utf-8 · py · unix`, right side `6:18` +
    contextual hint (`i insert · :w save · :q close`, or `esc → normal`).
    Pressing **`i`** (when editor focused) → INSERT (shows blinking bar cursor
    on current line); **`Esc`** → NORMAL.
  - **③ Terminal — `◰ TERMINAL · sb-7f3a91 (in-VM)`:** a real shell scrollback
    *inside the VM*. Prompt parts colored: user `stacy` orange, `@` steel,
    host `sb-7f3a91` green, `:` steel, path `/workspace` steel, `$` steel.
    Sample session: `python -m pytest -q` → `........ [100%]` →
    `24 passed in 1.83s`; `./run.sh --epochs 3` → loss lines →
    `✓ saved model.pkl (2.4MB)`. When focused, the prompt shows a blinking
    block cursor awaiting input.
- **Footer:** `workspace · 3 panes · <focus> focused` … keyhints
  `⇥ cycle pane · i insert · esc normal · ⌘K command`.

### 3. Spawn Sequence (animated)
**File:** `mockup/v2-spawn.jsx`
**Purpose:** Show provisioning lifecycle as a state machine driving both a
progress bar and a checklist timeline.

**State machine** (drive with `tea.Tick`; durations are the design's pacing):
```
queue        scheduling on docker(runsc)…                 600ms
pull image   python:3.12-alpine  pulling layers          1500ms  (progress)
boot rootfs  unpacking · mounting overlay · init         1100ms  (progress)
network      veth up · 10.88.0.7 · preview *.stacy.dev    700ms
ready        sandbox live                                   —
```
**Layout:**
- Two top tiles: `◇ SPAWN REQUEST` (image / template / provider / ttl summary)
  and `◐ PROVISIONING` (live readout: current phase UPPERCASE + orange,
  detail line + blinking cursor, a 24-cell progress bar with `%`). On
  completion the right tile flips to `✓ sb-9d11ba READY` (green) with
  `python:3.12-alpine · 10.88.0.7 · booted in 1.42s · expires 30m`.
- **SEQUENCE timeline module:** one row per phase — glyph (`✓`/`◐`/`○`),
  label (orange while active), detail line, and timing (`0.60s` once done,
  `· · ·` while active, `—` if pending). Future rows are dimmed to 0.4 opacity.
- Footer note + a `↻ REPLAY SEQUENCE` button (re-runs the machine).
- **Important behavior:** spawn progress *also* surfaces in the telemetry
  ribbon, so navigating away never hides it. Model spawn as a global/background
  command whose progress is observable from any screen.

### 4. Boot Splash (animated)
**File:** `mockup/v2-boot.jsx`
**Purpose:** The app's opening animation.

**Choreography:**
1. Logo **mark** springs in (translate up + de-blur + scale) — use
   **harmonica** spring, not linear.
2. Wordmark **`STACYVM`** (wide letter-spacing) fades/translates up
   (~+0.30s stagger).
3. Tagline **`microVM sandbox orchestrator for LLMs`** fades up (~+0.45s).
4. **Connect bar** (26-cell `█`/`░`) fills over ~1.6s while the status line
   steps: `connecting :7423` → (>55%) `handshake · loading fleet` →
   (100%) `✓ ready · 5 sandboxes` (green).

**Critical implementation note from the mockup:** the *resting* (finished) state
must always be the default render — the entrance is timer/tick-driven so a
backgrounded or interrupted render is never left blank. Build the View to render
the final state by default and let animation messages drive the *intro*, not the
other way around.

---

## Build 1 (Direction A) screens — the rest of the app

These come from the **selected** Mission Control direction (`direction-a.jsx`,
shown as tab 2 "MISSION CONTROL" in `build1-mission-control/index.html`). They
share the same ribbon + nav + status-line chrome as the Dashboard. Build them
with the same tokens, glyphs, and Lip Gloss approach.

All of them use a common **status line** footer: left = a faint summary, right =
a `KeyHint` row, and optionally a colored result chip (e.g. `✓ patched …`).

### 5. Sandboxes — list + inspect drawer
**Purpose:** Browse the full fleet; selecting a row opens an **inspect drawer**
beside the list (not a dead row). In Build 2 the drawer's "deep" actions evolve
into the full **Workspace** (#2) — keep this list→inspect as the quick triage
view, with `↵`/open escalating to the Workspace.

**Layout:** two columns (`~1.4fr / 1fr`).
- **Left — `FLEET`** (header right hint `filter: running`): table columns
  `ID · STATE · IMAGE · TTL`. Row 0 selected (orange). States use the dot
  glyphs (`●` run green, `◐` create orange).
- **Right — `◂ sb-7f3a91` (accent, hint `INSPECT`):** key/value readout —
  `state ● running`, `image python:3.12-alpine`, `provider docker (runsc)`,
  `created 17:14:09 · 24m ago`, `expires in 06m ███████░ `, then `CPU` 10-cell
  meter and `MEM` 10-cell meter (green). Keyhint: `e exec · f files · l logs ·
  d kill`.
- Status line: `6 sandboxes` · hints `j/k move · ↵ inspect · s spawn`.

### 6. Spawn — floating modal (quick spawn form)
**Purpose:** A lightweight overlay form to launch a sandbox; submitting it
starts the animated **Spawn sequence** (#3).

**Layout:** the fleet list sits behind, dimmed (~0.28 opacity, grayscaled). A
centered modal panel (`◇ SPAWN SANDBOX`, accent, with a heavy drop shadow in the
mockup — in the TUI render it as a bordered overlay box). Fields as a
label/control grid:
- `image` → text input showing `python:3.12-alpine` with a cursor `▏`
  (orange left-border marks the focused field).
- `template` → select `data-science ▾`.
- `ttl` → `30m`.
- `provider` → segmented control: `docker` (selected, orange outline) ·
  `firecracker` · `proot`.
- Keyhint: `tab next field · ↵ spawn · esc cancel`.

### 7. Exec — command + framed output
**Purpose:** Run a one-off command in a sandbox and see framed output with exit
code + duration **always shown** (a fix for output that used to truncate).

**Layout:** single accent module `◇ EXEC · sb-7f3a91`:
- Prompt line: `$ python -m pytest -q` (`$` green, command ink).
- Output block (mint/dim): `........................ [100%]`, `24 passed in
  1.83s`, then a framed footer `─ exit 0 · 1.9s ─` (faint).
- Keyhint: `↵ run · ↑ history · esc back`.

### 8. Files — explicit READ / WRITE mode
**Purpose:** Browse + edit sandbox files with an **explicit mode indicator** so
writes are never accidental. (Build 2's Workspace #2 is the richer evolution of
this — but the explicit READ/WRITE affordance must survive into it.)

**Layout:** two columns (`auto / 1fr`).
- **Left — `TREE`:** indented file tree; current dir orange (`▾ src`),
  files dim.
- **Right — `◇ /workspace/src/main.py` (accent, hint `EDIT`):** numbered code
  lines with syntax color (keywords orange, strings green, fn names mint), a
  cursor `▏`. Footer shows a bold orange `● WRITE mode` chip + `^o read ·
  ^s save`.

### 9. Templates — table + detail
**Purpose:** Manage reusable sandbox templates with pre-warmed pools.

**Layout:** two columns (`~1.3fr / 1fr`).
- **Left — `TEMPLATES`:** table `NAME · IMAGE · MEM · CPU · POOL`. Row 0
  (`data-science`) selected. POOL count is green when warm (>0), else dim.
  Sample rows: `data-science python:3.12 512 1 3` · `web-build node:20 1024 2 0`
  · `rust-ci rust:1.78 2048 4 1`.
- **Right — `◂ data-science` (accent):** bold title "Data Science Stack",
  description ("Python 3.12 with numpy, pandas, scikit-learn pre-warmed. Pool
  keeps **3** instances ready."), then `mem 512MB · cpu 1 · ttl 300s`. Keyhint:
  `s spawn · n new · d delete`.
- Status line: `4 templates` · hints `s spawn · n new`.

### 10. Providers — health cards
**Purpose:** Show each sandbox runtime backend's health + latency.

**Layout:** three equal cards (`grid3`).
- **`DOCKER` (accent, hint `DEFAULT`):** `● healthy` (green), `runtime runsc`,
  `sandboxes 4`, `latency 12ms` + green sparkline.
- **`FIRECRACKER`:** `● healthy`, `kvm /dev/kvm ok`, `sandboxes 1`,
  `latency 8ms` + sparkline.
- **`PROOT`:** `○ standby` (dim), `mode userspace`, `sandboxes 0`, faint
  `no kvm required`.
- Status line: `2 providers healthy` · hints `r refresh · ↵ set default`.

### 11. Logs — filterable event stream
**Purpose:** A following, color-coded, filterable event log (the full history
behind the Dashboard's compact EVENT STREAM module).

**Layout:** single module `EVENT STREAM` (hint `◉ following`). Each row:
`HH:MM:SS` (faint) · KIND (bold, fixed ~78px/cols column, colored by kind) ·
detail (dim). Kind colors: `SPAWN` orange, `EXEC` green, `WRITE` steel,
`TEMPLATE` mint, `KILL` red, `CONFIG` dim. Sample rows:
```
17:38:04  SPAWN     sb-7f3a91 docker python:3.12 ttl=30m
17:38:02  EXEC      sb-2c8e04 'pytest -q' exit=0 1.9s
17:37:51  WRITE     sb-7f3a91 /workspace/src/main.py 412B
17:37:30  TEMPLATE  created data-science pool=3
17:36:58  KILL      sb-0a31ff reason=ttl-expired
17:36:12  CONFIG    providers.docker.runtime runc→runsc
```
- Header note: `filter: all · 100 cap`. Status line hints: `/ filter ·
  g jump kind · c copy`.

### 12. Config — segmented controls, live patch
**Purpose:** Edit server/provider config with segmented toggles that **patch
live**.

**Layout:** two columns (`grid2`).
- **Left — `◇ PROVIDERS` (accent):** "default provider" segmented control
  (`docker` selected · `firecracker` · `proot`); "docker runtime" segmented
  control (`runc` · `runsc` selected · `kata`). Selected = orange outline.
- **Right — `SERVER`:** key/value — `address localhost:7423`,
  `log format pretty`, `preview domain *.stacy.dev`, faint hint `↵ edit any key
  · changes patch live`.
- Status line: result chip `✓ patched providers.docker.runtime=runsc` · hint
  `space apply`.

> **Rejected explorations (do NOT build):** `direction-b.jsx` ("HUD Grid") and
> `direction-c.jsx` ("Stream") are alternative layouts that were not chosen.
> They remain in `build1-mission-control/` only so the decision record is
> complete. Build Direction A.

---

## Interactions & behavior (key bindings)

Global / per-screen bindings the TUI must implement (the mockup's tab-number
switching `1–4` is a *demo* affordance — ignore it; the real bindings are
below):

| Context | Key | Action |
|---|---|---|
| Global | `⌘K` (or `Ctrl+K`) | Command palette (fuzzy jump to any screen/action) |
| Global | `1`–`6` | Jump to nav destination: `1 DASH · 2 SANDBOXES · 3 TEMPLATES · 4 PROVIDERS · 5 LOGS · 6 CONFIG` |
| Global | `?` | Help |
| Global | `q` | Quit |
| Dashboard | `↑/↓` or `j/k` | Move table selection |
| Dashboard | `↵` | Open selected sandbox → Workspace |
| Dashboard | `s` | Spawn · `e` exec · `f` files · `d` kill |
| Sandboxes | `j/k` | Move · `↵` inspect · `s` spawn |
| Sandboxes (inspect) | `e/f/l/d` | exec / files / logs / kill |
| Spawn modal | `tab` | Next field · `↵` spawn · `esc` cancel |
| Exec | `↵` run · `↑` history · `esc` back | |
| Files | `^o` read · `^s` save | toggle READ/WRITE explicitly |
| Templates | `s` spawn · `n` new · `d` delete | |
| Providers | `r` refresh · `↵` set default | |
| Logs | `/` filter · `g` jump kind · `c` copy | |
| Config | `↵` edit key · `space` apply | segmented controls patch live |
| Workspace | `⇥ Tab` | Cycle focus: files → editor → terminal → files |
| Workspace | `1/2/3` | Focus files / editor / terminal directly |
| Workspace (editor) | `i` | Enter INSERT mode |
| Workspace (editor) | `Esc` | Return to NORMAL mode |
| Workspace (editor) | `:w` / `:q` | Save / close |
| Spawn / Boot | — | `↻ Replay` re-runs the animation (dev affordance) |

### Animations / timing summary
- **Cursor blink:** ~1.05s, step (on ~55%, off ~45%).
- **Live telemetry tick:** ~1s (sparkline drift + clock).
- **Spawn phases:** see state machine table above; progress bars tween over the
  phase duration.
- **Boot:** spring mark-in; staggered fades at +0.30/+0.45/+0.60s; bar fill
  ~1.6s starting ~0.76s in.
- Prefer **spring physics (harmonica)** for the boot mark and any "pop"
  (READY), and eased tweens for bar fills — avoid purely linear motion.

---

## State management (Bubble Tea model sketch)

A top-level `Model` with a `screen` enum (`dashboard | workspace | spawn |
boot`) and sub-models:

- `dashboard`: `sandboxes []Sandbox`, `selected int`, `telemetry` (windowed
  ring buffers for cpu/mem/load/net + host cpu/mem/disk), `clock time.Time`,
  `events []Event`, `providers []Provider`.
- `workspace`: `sandboxID string`, `focus enum{tree,editor,term}`,
  `editorMode enum{normal,insert}`, `tree []TreeNode` (depth, name, kind,
  gitFlag, selected/open), `editor` (lines + syntax spans + cursor `line:col`),
  `term` (scrollback lines), `ttl`.
- `spawn`: `phase int`, `progress float64`, `done bool`, `req SpawnRequest`,
  plus the global progress hook so the ribbon can read it.
- `boot`: `progress float64`, `stage enum{connecting,handshake,ready}`, spring
  state for the mark.

Messages: `tickMsg` (telemetry/clock), `spawnPhaseMsg`/`spawnProgressMsg`,
`bootProgressMsg`, `keyMsg` routed by current screen + focus. Data fetching
(real fleet, telemetry, file tree, terminal I/O) is wired through `tea.Cmd`s;
the mockup uses the seed data above as stand-ins.

---

## Responsive / degradation
- Reference layout assumes ~198 cols. Below that, collapse the Dashboard's
  right-hand module column under the table, and reduce meter/sparkline widths
  before truncating labels. The Workspace can stack panes vertically on narrow
  terminals. Use Lip Gloss width measurement to reflow rather than hard-wrap.

## Assets
- **Logo art** (`mockup/logo-art.js`): the `STACY_LOGO_ART` object holds the
  ASCII/half-block renderings of the mark in three sizes (`hero`, `header`,
  `small`) built from half-block chars (`▄ █ ▀`). These are pure text and tint
  orange — drop them straight into the TUI (the `hero` size is the boot mark,
  `header` is the ribbon mark). No image files required.
- No other binary assets. Fonts are the user's terminal font (mockup only loads
  JetBrains Mono + Kalam for browser review).

## Files in this bundle
```
README.md                          ← this document

mockup/
  build2-refined/                  ← BUILD 2 — refined, animated (screens 1–4)
    StacyVM TUI v2.html            ← open in a browser: 4 screens + live motion
    styles.css                     ← reference visual spec (colors, glyphs, layout)
    logo-art.js                    ← ASCII logo art (drop into the TUI)
    v2-kit.jsx                     ← shared primitives (meters, sparks, frames…)
    v2-app.jsx                     ← demo shell (tabs/toggles = scaffolding, ignore)
    v2-dashboard.jsx               ← Dashboard / Mission Control (screen 1)
    v2-workspace.jsx               ← Sandbox Workspace: tree + vim + terminal (2)
    v2-spawn.jsx                   ← Spawn sequence state machine (3)
    v2-boot.jsx                    ← Boot splash animation (4)

  build1-mission-control/          ← BUILD 1 — selected Direction A (screens 5–12)
    index.html                     ← open in a browser; use TAB 2 "MISSION CONTROL"
    direction-a.jsx                ← ★ THE SELECTED DIRECTION — all of screens 5–12
    wire-kit.jsx                   ← Build 1 shared primitives
    shared.jsx, app.jsx            ← demo shell + overview (scaffolding)
    logo-art.js                    ← ASCII logo art
    direction-b.jsx, direction-c.jsx ← REJECTED explorations — do NOT build
```

**How to review:**
- **Build 2** — open `build2-refined/StacyVM TUI v2.html`; the top tab strip
  walks the 4 refined screens, and sparklines / clock / spawn / boot all animate
  live so you can see intended motion.
- **Build 1** — open `build1-mission-control/index.html` and click **tab 2
  "MISSION CONTROL"** (Direction A). Scroll past the hero Dashboard to the
  "secondary screens & modes" grid — that's screens 5–12 (Sandboxes, Spawn,
  Exec, Files, Templates, Providers, Logs, Config). Ignore tabs 3 "HUD GRID"
  and 4 "STREAM" (rejected) and the OVERVIEW/LOGO/UX-FIXES tabs (process notes).
