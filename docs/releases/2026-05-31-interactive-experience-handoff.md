# Docs Handoff — Interactive Experience (CLI / TUI / Web UI / Desktop)

**Range:** `0e54925` → `5d708d8` (2026-05-28 → 2026-05-31)
**Audience:** the docs writer. This file is *context*, not docs. It tells you
what shipped, how it behaves, and exactly which doc pages to create/update.
Everything below was verified against the code on 2026-05-31.

---

## 1. The one-line summary

StacyVM gained its entire **operator-facing surface** in this range: a styled
CLI, a full terminal UI (TUI), a Next.js web dashboard embedded in the binary,
a Wails desktop app, and an `npx` one-command installer. The sandbox/runtime
engine did **not** change — do not re-document providers/runtime behavior here.

---

## 2. Install & first-run flow (document this first)

There are now **three** ways to get StacyVM. Beginners should be pointed at #1.

### 2.1 One-command installer (recommended for new users)

```bash
npx stacyvm-setup@latest
```

What it does, in order (`scripts/npm-setup.mjs`):
1. Clones the repo into `./stacyvm` (shallow, `--branch main`).
2. Checks host prerequisites: **Go**, **Docker** (daemon reachable), git.
3. Builds the **web UI** (`npm install` + `npm run build` → `web/out`).
4. Downloads Go deps and builds the **`stacyvm` binary** (web UI embedded).
5. Installs the binary onto the PATH (`~/.local/bin` or `/usr/local/bin`;
   on Windows `%USERPROFILE%\.stacyvm\bin`).
6. **Deletes the cloned repo** (only after a successful install — otherwise it
   keeps it so the user always has a working binary).
7. Runs `stacyvm setup`, starts `stacyvm serve` in the background, and launches
   `stacyvm web-ui`.

Flags worth documenting: `--no-start` (build + install only), `--check-only`
(host check only), `--skip-docker-check`, `--skip-node-deps` (only safe if
`web/out` already exists), `--uninstall`, `--dir`, `--branch`, `--repo`.

> **Prerequisites callout (important):** Go, Node.js/npm, Docker, and git must
> be installed. The installer fails fast with host-specific guidance if not.

### 2.2 From source

```bash
make build          # builds web/out then the stacyvm binary
./stacyvm setup
./stacyvm serve
```

### 2.3 Desktop app

See §5.

**Docs to update:** `docs/getting-started/installation.mdx`,
`docs/getting-started/quickstart.mdx`, `docs/getting-started/prerequisites.mdx`.

---

## 3. CLI reference (commands registered in `cmd/stacyvm/main.go`)

Root: `stacyvm` — persistent flags `--server` (default `http://localhost:7423`)
and `--api-key` (env `STACYVM_API_KEY`). Styled with `charmbracelet/fang` and
the StacyVM brand theme (orange `#FFA60C`).

**New or changed in this range:**

| Command | What it does | Notes |
|---|---|---|
| `stacyvm setup` | Interactive setup wizard | Provider (Docker/Firecracker/PRoot), Docker runtime (runc/runsc/kata), preview domain, optional shell completion. Writes `~/.stacyvm/config.yaml`. |
| `stacyvm tui` | Launch the terminal UI | See §4. |
| `stacyvm web-ui` (alias `ui`) | Serve embedded web dashboard + open browser | `--port`/`-p`, default **5749**. |
| `stacyvm update` | Self-update the installed binary | |
| `stacyvm uninstall` | Remove binaries + `~/.stacyvm` | |
| `stacyvm config ...` | View/edit configuration | Mirrors the TUI Config screen. |

Pre-existing commands still present: `serve`, `worker`, `spawn`, `exec`, `kill`,
`list`, `version`, `build-image`, `doctor`, `db`, `upgrade`, `support`.

**Ports to state explicitly:** API server `serve` → **7423**; `web-ui` → **5749**.

**Docs to update:** add a CLI command reference (there isn't a dedicated one yet
under `docs/`); cross-link from quickstart.

---

## 4. The TUI (`stacyvm tui`) — `tui/` package

Bubble Tea full-screen app. Source of truth for behavior: `tui/app.go`.

**Six screens (tabs), switched with number keys `1`–`6`:**
1. **Dashboard** (`dashboard.go`) — overview + active sandboxes table.
2. **Sandboxes** (`sandboxes.go`) — list/manage sandboxes.
3. **Templates** (`templates.go`) — template management.
4. **Providers** (`providers.go`) — provider configuration/status.
5. **Logs** (`logs.go`) — event/log stream.
6. **Config** (`config_tab.go`) — view/edit configuration.

**Global keys:** `1`–`6` switch screens · `Ctrl+K` command palette
(`palette.go`) · `r` refresh · `q` / `Ctrl+C` quit.

**Sandbox actions (Dashboard + Sandboxes):** `j`/`k` or arrows move · `s` spawn
(`spawn.go`) · `e` exec (`exec.go`) · `f` workspace/files (`workspace.go`,
`files.go`) · `l` logs · `d`/`delete` destroy · `enter` open.

**Workspace + editor** (`workspace.go`, `editor.go`): netrw-style scrolling file
tree with parent/refresh navigation, a **modal text editor** (`Ctrl+S` saves,
`esc` closes), editor-majority layout with a toggleable terminal pane, and the
tree auto-refreshes after exec/save. Footer key hints are **context-aware** (they
change per screen/mode).

Other UI niceties: animated **boot splash** (`boot.go`), brand theme
(`theme.go`, `logo.go`, `palette.go`).

**Docs to update:** new page "Using the TUI" (screens + the keybinding tables
above). Recommend a screenshot/gif. Good home: `docs/getting-started/` or a new
`docs/tutorials/` entry.

---

## 5. Web dashboard (`web/`) + Desktop app (`desktop/`)

### 5.1 Web dashboard
- Next.js **16** app (the folder package name is `web-new`), static export
  (`output: 'export'` in `web/next.config.ts` → `web/out/`).
- Embedded into the CLI binary via `//go:embed all:out` (`web/embed.go`),
  served by `stacyvm web-ui` on port **5749**.
- Pages (`web/app/*/page.tsx`): Dashboard, Sandboxes, Templates, Providers,
  Environments, Operations, Tenants, Settings.
- Talks to the API at `http://127.0.0.1:7423/api/v1` (see `web/.env.local` /
  `NEXT_PUBLIC_API_URL`). **So `web-ui` alone is not enough — `serve` must be
  running** for live data. Call this out in docs.

### 5.2 Desktop app (Wails)
- `desktop/` is a **separate Go module** (`replace github.com/StacyOs/stacyvm
  => ../`) that embeds the web UI and runs the API daemon **in-process**
  (`desktop/daemon.go`), so the desktop app does not need a separate `serve`.
- **Build:** `make build-desktop` (= `cd desktop && wails build -tags
  webkit2_41`). Requires the Wails CLI and, on Linux, **webkit2gtk-4.1**.
- **Output / export location:** `desktop/build/bin/StacyVM`.
- **Still needs Docker** (or another provider) on the host to actually spawn
  sandboxes — the desktop app bundles the UI + API, not the isolation backend.

**Docs to update:** new "Desktop app" page (build + run + prerequisites +
distribution); note webkit2gtk on Linux and Docker as a runtime prerequisite.

### 5.3 Distribution notes (for the "download & run" goal)
- Wails generally must be built **on the target OS** (CGO + native webview), so
  releasing for macOS/Windows/Linux needs per-OS CI runners.
- "Just download and run" packaging per platform:
  - **Windows:** `wails build -nsis` → single-file installer (`.exe`).
  - **macOS:** produces `StacyVM.app`; wrap in a `.dmg` (codesign + notarize for
    Gatekeeper).
  - **Linux:** produces a bare ELF that depends on `libwebkit2gtk-4.1` at
    runtime → ship an **AppImage** (self-contained) or `.deb`/`.rpm` so beginners
    don't have to install system libs.

---

## 6. Branding

New logos, refreshed palette, and replaced icon placeholders across the web
frontend (`5d708d8`). CLI brand colors live in `getBrandTheme()` in
`cmd/stacyvm/main.go` (orange `#FFA60C`, green `#22C55E`, mint `#D7F6E2`). Keep
any docs screenshots/wordmarks consistent with these.

---

## 7. Removed / moved (don't document as current)

- Old **Vite** web frontend → moved to `deprecated/web/` (ignore it).
- An earlier standalone desktop frontend was removed in favor of the Wails app.

---

## 8. Suggested docs checklist

- [ ] `getting-started/installation.mdx` — add the `npx stacyvm-setup` flow + prerequisites.
- [ ] `getting-started/quickstart.mdx` — setup → serve → web-ui / tui first-run.
- [ ] `getting-started/prerequisites.mdx` — Go, Node/npm, Docker, git (+ webkit2gtk for desktop).
- [ ] New page: **CLI command reference** (table in §3, with flags/ports).
- [ ] New page: **Using the TUI** (screens + keybindings, §4).
- [ ] New page: **Web dashboard** (`web-ui`, port 5749, needs `serve`).
- [ ] New page: **Desktop app** (build, `desktop/build/bin/StacyVM`, distribution).
- [ ] Add all new nav entries to `docs.json` (Mintlify).
