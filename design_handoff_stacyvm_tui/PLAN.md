# StacyVM TUI — Mission Control restyle (real-data, full build)

## Context

The repo has a working Go/Bubble Tea TUI (`tui/` package, launched via `stacyvm tui`)
that already talks to the live StacyVM HTTP API. The design handoff in
`design_handoff_stacyvm_tui/` specifies a high-fidelity "Mission Control" look:
a full-width telemetry ribbon + horizontal nav + status footer, KPI tiles,
bracket-framed panels, meters/sparklines, a deep Sandbox Workspace, animated
Spawn + Boot sequences, and a ⌘K command palette — 12 screens total.

Goal: recreate the handoff's look **cell-perfectly** in our existing Charm stack
while **wiring every metric to a real data source** (user requirement: no
simulated numbers). Existing functionality and key bindings are preserved except
where the README explicitly changes them.

**Decisions locked with the user:**
- **Scope:** Full build — all 12 screens incl. net-new (Boot, Workspace, animated Spawn, ⌘K palette).
- **Data:** Real sources only. Poll on a ~1s tick into per-metric **client-side ring buffers** and render that real history as the sparklines. `—` placeholder only where no source exists (called out below).
- **Terminal (Workspace):** Real **command-runner** via existing WS/exec-stream (type command → real streamed output); editor edits a **real file** via the files API. Full bidirectional PTY (stdin/resize/signals) is a tracked **follow-up**, not in this effort.
- **Sequencing:** **Backend-data-source per screen** — each screen slice ships the backend it needs so it's fully real when reviewed.
- **Review:** Live server (`stacyvm serve` + `stacyvm tui`), eyeballed screen-by-screen.

## Current state (what exists)

- One big `tui/app.go` (1457 lines): root `Model`, `Update`, `View` (sidebar + content + statusbar), all `view*` funcs; `tui/config_tab.go`; `tui/client.go` (HTTP client + data structs).
- `tab` enum (dashboard/sandboxes/templates/providers/logs/config); `mode` enum (normal/input/confirm/exec/spawn/spawning/sandboxAction/spawnTemplate/createTemplate/configEdit).
- Inline lipgloss styles at `app.go:17-59` with ad-hoc colors (`#0C0C0C`, `#F9F7F3`, `#888888`, `#FF3333`) — to be replaced by the token layer.
- Deps already present: `bubbletea`, `bubbles` (textinput, table), `lipgloss`, `harmonica`, `glamour`, `docker/docker`, `nhooyr.io/websocket`. **To add:** `github.com/shirou/gopsutil/v4` (host stats).

## Data-source map (real vs. needs-backend vs. placeholder)

| Design element | Source today | Action |
|---|---|---|
| Events (dashboard stream + Logs screen) | `GET /events` **SSE bus**, 18 types + history | Wire TUI SSE consumer |
| Provider health / latency / counts (Providers cards, dashboard providers) | `GET /providers` (`LatencyMS`,`RuntimeCount`,caps) + `/providers/{name}` (`sandbox_count`,config) | Wire client (no backend change) |
| File tree (Files, Workspace) | `GET /sandboxes/{id}/files/list|stat|glob` | Wire client |
| Sandbox detail / TTL (inspect, workspace ctx bar) | `GET /sandboxes/{id}` (`created_at`,`expires_at`,…) | Wire client |
| Health / uptime / version (ribbon) | `GET /health` | Already wired |
| Terminal output (Workspace) | `GET /sandboxes/{id}/exec/ws` + `POST /exec` (stream) | Wire client; command-runner only |
| **Host CPU/MEM/DISK/NET + LOAD** (ribbon + HOST TELEMETRY) | **none** (no gopsutil/proc) | **New:** gopsutil collector + `GET /api/v1/system/stats` |
| **Per-sandbox CPU% / live mem** (dashboard CPU col, inspect meters) | **none** (`Provider.Stats()` missing; Docker `ContainerStats` unused) | **New:** optional `StatsReporter` iface (Docker `ContainerStats`) + `GET /api/v1/sandboxes/{id}/stats` |
| Firecracker "kvm /dev/kvm ok" | Healthy() only checks file exists | Small: stat `/dev/kvm`; else `—` |
| "312 spawns" uptime sub-stat | no spawn counter found | Derive from event history count, else `—` |
| Full interactive PTY / in-pane vim keystrokes | agentproto has no PTY | **Out of scope** (follow-up) — terminal is command-runner |

## Architecture / file layout (refactor `app.go` into cohesive files, same `package tui`)

- **`tui/theme.go`** — the single design-token layer (the README color/glyph/spacing table):
  - `Palette`: `lipgloss.Color` vars for all roles — `Bg #08080a`, `Panel #0c0c10`, `Panel2 #101016`, `Ink #ECE7DD`, `Dim #7d7d86`, `Faint #4a4a52`, `Line #23232b`, `Line2 #33333d`, `Orange #FFA60C`, `Orange2 #FF7A1A`, `Green #22C55E`, `Red #FF4747`, `Mint #D7F6E2`, `Steel #5b6b78`.
  - `Glyph` consts: dots `● ◐ ○ ◉`, meter `█ ░`, spark ramp `▁▂▃▄▅▆▇█`, tree `▾ ▸ ·`, checklist `✓ ◐ ○`, bracket corners `⌜⌝⌞⌟`, pane icons `◧ ◨ ◰ ◇ ▣`, chip `▣`.
  - Base style builders: `Label()` (UPPERCASE dim, letter-spaced), `Title()`, `KeyHint()`, `SelectedRow()`, `StateStyle(state)`, `KindStyle(kind)` (log colors). Replaces all of `app.go:17-59`.
- **`tui/logo.go`** — `STACY_LOGO_ART` (`hero`/`header`/`small`) as `[]string` from `mockup/logo-art.js`, tinted Orange; `header` → ribbon, `hero` → boot.
- **`tui/kit.go`** — primitives consuming the theme: `Meter(val,max,width,thresholdColor)`, `Spark(ring,max)`, `PBar(pct,width)`, `BracketFrame(title,body,focused)`, `StatusDot(state)`, `KPITile(label,metric,metricColor,sub,spark)`, `KeyHints([]hint)`, and a `ring` type (fixed-size float64 history with `Push`/`Slice`).
- **`tui/chrome.go`** — `TelemetryRibbon(...)` (logo+wordmark left; CPU/MEM/LOAD sparks + clock + `● ONLINE vX` right), `NavRibbon(active)` (`1 DASH · 2 SANDBOXES · … 6 CONFIG`, right `⌘K command`; active = orange text + underline + faint-orange bg), `StatusFooter(left,hints,chip)`. `View()` is rewritten to stack ribbon → nav → screen body → footer (the old fixed sidebar at `app.go:689-745` is removed).
- **Per-screen files** (split from `app.go`): `dashboard.go`, `sandboxes.go`, `spawn.go`, `workspace.go`, `boot.go`, `exec.go`, `files.go`, `templates.go`, `providers.go`, `logs.go`, `config_tab.go` (exists), `palette.go` (⌘K). `app.go` keeps `Model`, `Update`, key routing, commands.
- **`tui/telemetry.go`** — client-side: ring buffers (host cpu/mem/disk/net/load; per-sandbox cpu), the ~1s telemetry tick cmd, cursor-blink tick (~1.05s), and the SSE event subscription cmd (goroutine → channel → `tea.Msg`, re-issued per event).
- **`tui/client.go`** — add `systemStats()`, `sandboxStats(id)`, `sandboxDetail(id)`, `listFiles(id,path)`, `providerDetail(name)`, `subscribeEvents(ctx)` (SSE reader), and an `execWS(id,cmd)` streaming helper. New structs: `systemStatsData`, `sandboxStatsData`, `eventData`, `fileInfoData`, extend `providerData` (latency/runtimeCount/runtime), extend `healthData` if needed.

### Backend additions (Go server side)

- **`internal/api/routes/system.go`**: add `GET /api/v1/system/stats` → `{cpu_pct, mem_pct, disk_pct, net_rx_bps, net_tx_bps, load1}`. Backed by a small **host sampler** (gopsutil) started in `cmd/stacyvm/cmd_serve.go` that refreshes ~1s and serves the latest snapshot (gopsutil CPU% needs interval sampling; a background sampler avoids per-request blocking). Register route alongside existing `/metrics`.
- **`internal/providers/provider.go`**: add optional `StatsReporter interface { Stats(ctx, sandboxID) (*SandboxStats, error) }` (`{CPUPct float64; MemBytes, MemLimitBytes uint64}`). **`internal/providers/docker.go`** implements it via Docker `ContainerStats` (cgroup deltas → CPU%). Firecracker/proot: not implemented → endpoint returns `supported:false`.
- **`internal/api/routes/sandboxes.go`**: add `GET /api/v1/sandboxes/{id}/stats` → calls provider's `StatsReporter` if available, else `{supported:false}` (TUI renders `—`).
- **Provider latency**: already in `ProviderHealth.LatencyMS` from timed health checks — no backend change, just surface it.
- Note: the local single-process path is the target; worker-distributed per-sandbox stats (workerproto `MethodStats`) and Firecracker vsock stats are **follow-ups** (TUI shows `—` there).

## Build sequence (one slice at a time; diff + run instructions after each)

> Slice 1 is the largest because it lays the token/kit/chrome foundation that every
> later screen reuses, plus the two telemetry endpoints the Dashboard needs. It can be
> reviewed in two passes (chrome+layout, then live telemetry) if preferred.

1. **Dashboard + foundation** (`v2-dashboard.jsx`). Build `theme.go`, `logo.go`, `kit.go`, `chrome.go`; rewrite `View()` to ribbon/nav/footer; build Dashboard: 4 KPI tiles, two-column body (left ACTIVE SANDBOXES table with `ID·STATE·PROVIDER·IMAGE·CPU·TTL`, row0 selected, 6-cell CPU mini-meter; right HOST TELEMETRY / PROVIDERS / EVENT STREAM). **Backend:** `/system/stats` (gopsutil) + `/sandboxes/{id}/stats` (docker) + SSE consumer + ring buffers + 1s tick. Keys: `↑/↓` `j/k` move, `↵` open Workspace, `s/e/f/d`, `1-6` nav.
2. **Sandboxes list + inspect drawer** (`direction-a.jsx`). Two-col `FLEET` table + side `◂ INSPECT` drawer (state/image/provider/created/expires-meter + CPU/MEM meters from sandbox-stats). `j/k` move, `↵` inspect, `s` spawn, drawer `e/f/l/d`; "open" escalates to Workspace.
3. **Spawn modal (quick form)** (`direction-a.jsx`). Bordered overlay over dimmed fleet: `image` input, `template` select, `ttl`, `provider` segmented (`docker·firecracker·proot`). Submitting starts the animated Spawn sequence. `tab` next, `↵` spawn, `esc` cancel.
4. **Spawn sequence (animated)** (`v2-spawn.jsx`). Phase state machine (queue 600ms · pull 1500ms · boot 1100ms · network 700ms · ready) driven by `tea.Tick`; `◇ SPAWN REQUEST` + `◐ PROVISIONING` tiles (progress bar + %), SEQUENCE checklist timeline, flips to `✓ … READY`. Real spawn via API; model spawn as **background state on the root Model** so progress shows in the ribbon from any screen. `↻ replay`.
5. **Exec** (`direction-a.jsx`). `◇ EXEC · sb-id`: `$ cmd` (real), mint/dim output, always-shown framed `─ exit N · Ns ─` footer. `↵` run, `↑` history, `esc` back.
6. **Files** (`direction-a.jsx`). Two-col: real `TREE` (from files/list) + `◇ /path EDIT` numbered syntax code + explicit `● WRITE`/READ mode chip. `^o` read, `^s` save (preserve explicit-mode affordance).
7. **Templates** (`direction-a.jsx`). `TEMPLATES` table (`NAME·IMAGE·MEM·CPU·POOL`, POOL green when >0) + `◂` detail (title, description via glamour, `mem·cpu·ttl`). `s` spawn, `n` new, `d` delete.
8. **Providers** (`direction-a.jsx`). 3 health cards (DOCKER default / FIRECRACKER / PROOT): `● healthy`, runtime, sandboxes count, `latency Nms` + spark (real from `/providers`). `r` refresh, `↵` set default.
9. **Logs** (`direction-a.jsx`). `EVENT STREAM` (`◉ following`) from SSE: `HH:MM:SS · KIND(colored) · detail`; kind colors SPAWN orange/EXEC green/WRITE steel/TEMPLATE mint/KILL red/CONFIG dim. `/` filter, `g` jump kind, `c` copy.
10. **Config** (`direction-a.jsx` / `config_tab.go`). Two-col segmented controls: default provider + docker runtime (selected = orange outline) patching live via `PATCH /config`; right `SERVER` key/values. `↵` edit key, `space` apply; result chip `✓ patched …`.
11. **Sandbox Workspace** (`v2-workspace.jsx`). Context bar (`▣ id · ● running · image · via provider · ttl meter`); 3 focusable panes — `◧ FILES` (real tree) / `◨ EDITOR` (real file, vim-style modeline NORMAL/INSERT, syntax colors, line numbers) / `◰ TERMINAL` (real command-runner via WS/exec-stream). `Tab` cycle, `1/2/3` focus, `i` insert, `Esc` normal, `:w/:q`. Focused pane gets orange bracket-frame + `FOCUS`.
12. **Boot splash** (`v2-boot.jsx`). Harmonica spring mark-in; staggered wordmark/tagline fades; 26-cell connect bar over ~1.6s stepping `connecting :7423` → `handshake · loading fleet` → `✓ ready · N sandboxes`. **Render final/resting state by default**, intro driven by tick msgs (never blank). Shown on launch before Dashboard.
13. **⌘K command palette** (global). `Ctrl+K` overlay: fuzzy list of nav destinations + actions (spawn/exec/files/kill/refresh); `↑/↓` select, `↵` run, `esc` close. Also add global `?` help.

## Key bindings (preserve + add)

- **Preserve:** `1-6` nav, `←/→` tab nav, `q`/`Ctrl+C` quit, `r` refresh, `j/k`/`↑↓` move, sandboxes `s/e/f/d`, templates `n/c/s/d`, modal `tab/shift+tab/enter/esc`, confirm `y/n`, config `space/enter`.
- **Add (README):** `Ctrl+K` palette, `?` help, Dashboard `↵` open Workspace, Sandboxes `↵` inspect (drawer) + open→Workspace, Logs `/` `g` `c`, Providers `↵` set default, Workspace `Tab`/`1/2/3`/`i`/`Esc`/`:w`/`:q`, Spawn/Boot `↻` replay.

## Animations / timing
- Telemetry+clock tick ~1s (drift real ring buffers); cursor blink ~1.05s; spawn phase durations per table with eased progress; boot spring mark + staggered fades (+0.30/+0.45/+0.60s) + ~1.6s bar. Reuse existing `harmonica` springs; keep `tickFrame()` 60fps only while animating.

## Verification (per slice)
1. Build: `go build ./...` (and `go vet ./tui/...`). Run any touched tests (`go test ./internal/api/... ./internal/providers/...`).
2. Run live: terminal A `go run ./cmd/stacyvm serve`; terminal B `go run ./cmd/stacyvm tui` (server `http://localhost:7423`; `STACYVM_API_KEY` if set). Resize to ≥~180 cols for full fidelity (graceful reflow below).
3. For each slice: spawn a sandbox, confirm the screen renders real data (telemetry moves, events stream in, CPU meters reflect real load, file tree matches the sandbox), and confirm preserved key bindings still work. Show the diff + these run steps; wait for go-ahead before the next slice.
4. New endpoints checked directly: `curl localhost:7423/api/v1/system/stats`, `curl localhost:7423/api/v1/sandboxes/<id>/stats`.

## Open gaps / explicit `—` placeholders & follow-ups
- **Full interactive PTY** (stdin/resize/signals; real in-pane vim, live shell) — follow-up; terminal is command-runner for now.
- **Per-sandbox stats** only on the local **Docker** path; Firecracker/proot and worker-distributed sandboxes show `—` (follow-up: vsock stats + `workerproto.MethodStats`).
- **KVM "ok"** = best-effort `/dev/kvm` stat; **"NNN spawns"** derived from event history if available, else `—`.
- Responsive degradation: collapse right module column under the table below ~180 cols; Workspace panes stack vertically on narrow terminals (Lip Gloss width measurement, no hard-wrap).
