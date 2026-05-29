// ====================================================================
// shared.jsx — Overview, Logo Lab, UX Fixes (cross-cutting)
// ====================================================================

// ---------- OVERVIEW ----------
function Overview({ onPick }) {
  const cards = [
    { idx: 1, lt: "Direction A", h: "Mission Control", sub: "Telemetry ribbon on top, ⌘K command palette, dense HUD module grids. Built for watching a whole fleet.", nav: "⌘K palette + 1–6 tabs", sig: "Persistent vitals ribbon", feel: "Grafana / control room", to: "a" },
    { idx: 2, lt: "Direction B", h: "HUD Modular Grid", sub: "Slim icon dock, corner-bracket tactical frames, a selection reticle. The most terminal-native.", nav: "Numbered icon dock", sig: "⌜ ⌝ bracket frames + ▶", feel: "Brutalist / tactical", to: "b" },
    { idx: 3, lt: "Direction C", h: "Conversational Stream", sub: "No tabs — a command line drives everything, results stream back as blocks. Most dynamic.", nav: "/ slash menu (fuzzy)", sig: "Stream + persistent input", feel: "Claude Code / agentic", to: "c" },
  ];
  const probs = [
    ["1–6 shortcuts are invisible", "shown on every nav item / in the menu"],
    ["Dashboard Quick-Spawn is dead UI", "spawn reachable from anywhere"],
    ["Files write-by-accident", "explicit WRITE mode / separate verbs"],
    ["Errors truncated at 50 chars", "full errors + scrollback, no truncation"],
    ["Spawn progress only on one tab", "global progress feedback"],
    ["Config exposes only 6 keys", "type any config key path"],
    ["Generic r/q hints everywhere", "context-specific key hints"],
    ["No help overlay (?)", "? overlay + /help + palette"],
  ];
  return (
    <div>
      <div className="secthead">
        <div className="kicker">StacyVM · TUI Redesign</div>
        <h1>Three futuristic directions <span className="hand">— pick a feel</span></h1>
        <div className="lede">All three keep your brand (orange <b>#FFA60C</b> on charcoal), stay <b>OS-agnostic</b> (pure Unicode + ANSI, no images — buildable in Bubble&nbsp;Tea / lipgloss / harmonica), and bake in fixes for every known UX rough edge. They differ in <b>how you navigate</b> and <b>how alive it feels</b>. These are low-fi wireframes — structure & flow first, polish later.</div>
      </div>
      <div className="ovgrid">
        {cards.map((c) => (
          <div key={c.idx} className="ovcard" onClick={() => onPick(c.to)}>
            <span className="pill">Tab {c.idx + 1}</span>
            <div className="lt" style={{ marginTop: 12 }}>{c.lt}</div>
            <h3>{c.h}</h3>
            <div className="sub">{c.sub}</div>
            <div className="mini">
              <div>nav&nbsp;&nbsp;&nbsp;<b>{c.nav}</b></div>
              <div>signature&nbsp;<b>{c.sig}</b></div>
              <div>feel&nbsp;&nbsp;<b>{c.feel}</b></div>
            </div>
            <div className="hand" style={{ color: "var(--note)", marginTop: 12, fontSize: 14 }}>→ open this direction</div>
          </div>
        ))}
      </div>

      <Divider>what every direction fixes</Divider>
      <div className="problemlist">
        {probs.map((p, i) => (
          <div className="prob" key={i}>
            <span className="x">✗</span>
            <span><b>{p[0]}</b> <span className="arrow">→</span> <span className="ok">{p[1]}</span></span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ---------- LOGO LAB ----------
function LogoLab() {
  return (
    <div>
      <div className="secthead">
        <div className="kicker">Brand · In-TUI</div>
        <h1>The mark, as terminal art <span className="hand">— pure text, every OS</span></h1>
        <div className="lede">Your logo PNG is downsampled to <b>half-block characters</b> (▀ ▄ █) and tinted brand orange. The result is just a <code style={{ color: "var(--orange)" }}>[]string</code> you print with lipgloss — no image protocol, identical on macOS / Linux / Windows terminals.</div>
      </div>

      <div className="scrgrid">
        {/* boot splash */}
        <div>
          <Screen label="boot splash" right="stacyvm tui">
            <div style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "26px 0 22px", gap: 14 }}>
              <LogoArt size="hero" style={{ fontSize: 9, lineHeight: "9px" }} />
              <div style={{ textAlign: "center" }}>
                <div className="b" style={{ letterSpacing: 6, fontSize: 16 }}>STACYVM</div>
                <div className="dim" style={{ fontSize: 11, letterSpacing: 1, marginTop: 4 }}>microVM sandbox orchestrator</div>
              </div>
              <div style={{ fontSize: 12 }}><Hi>{"████████████"}</Hi><span className="faint">{"░░░░"}</span> <span className="dim">connecting :7423</span></div>
            </div>
          </Screen>
          <div className="caption"><span className="t">Boot / splash</span><span className="hand">hero size · reveal with a harmonica spring for a slick fade-in</span></div>
        </div>

        {/* sizes */}
        <div>
          <Screen label="responsive sizes" right="pick by terminal width">
            <div style={{ display: "grid", gridTemplateColumns: "auto auto auto", gap: 22, alignItems: "center", justifyItems: "center", padding: "14px 0" }}>
              <div style={{ textAlign: "center" }}><LogoArt size="hero" style={{ fontSize: 6, lineHeight: "6px" }} /><div className="faint" style={{ fontSize: 9, marginTop: 8 }}>hero · 56c</div></div>
              <div style={{ textAlign: "center" }}><LogoArt size="header" style={{ fontSize: 6, lineHeight: "6px" }} /><div className="faint" style={{ fontSize: 9, marginTop: 8 }}>header · 28c</div></div>
              <div style={{ textAlign: "center" }}><LogoArt size="small" style={{ fontSize: 7, lineHeight: "7px" }} /><div className="faint" style={{ fontSize: 9, marginTop: 8 }}>compact · 22c</div></div>
            </div>
            <div style={{ borderTop: "1px solid var(--line)", marginTop: 8, paddingTop: 11, display: "flex", alignItems: "center", gap: 10 }}>
              <LogoArt size="header" style={{ fontSize: 4.5, lineHeight: "4.5px" }} />
              <b style={{ letterSpacing: 2, fontSize: 12 }}>STACYVM</b>
              <span className="faint" style={{ fontSize: 10 }}>← inline header / sidebar lockup</span>
            </div>
          </Screen>
          <div className="caption"><span className="t">Three sizes</span><span className="hand">degrade to the wordmark pill on tiny terminals</span></div>
        </div>
      </div>

      <Divider>shipping it in Go</Divider>
      <div className="problemlist">
        <div className="prob"><span className="ok">›</span><span><b>Store as a constant.</b> <span className="dim">Bake the half-block strings into a <code style={{color:"var(--orange)"}}>var stacyArt []string</code> — generated once from the PNG.</span></span></div>
        <div className="prob"><span className="ok">›</span><span><b>Tint with lipgloss.</b> <span className="dim">Render each line with <code style={{color:"var(--orange)"}}>Foreground(lipgloss.Color("#FFA60C"))</code>.</span></span></div>
        <div className="prob"><span className="ok">›</span><span><b>Pick by width.</b> <span className="dim">Choose hero / header / compact from the current terminal width — same trick as the sidebar.</span></span></div>
        <div className="prob"><span className="ok">›</span><span><b>Animate the reveal.</b> <span className="dim">A harmonica spring on opacity/offset gives the splash a futuristic fade-in.</span></span></div>
      </div>
    </div>
  );
}

// ---------- UX FIXES ----------
function CmdPaletteMock() {
  const rows = [["spawn sandbox", "⌘ /spawn", true], ["exec in sb-7f3a91", "→", false], ["open files · sb-7f3a91", "→", false], ["go to · Logs", "5", false], ["set provider → firecracker", "config", false], ["kill all idle", "danger", false]];
  return (
    <Screen label="⌘K — command palette" right="fuzzy">
      <div style={{ border: "1px solid var(--line-2)", borderRadius: 3, padding: "8px 11px", marginBottom: 10, fontSize: 13 }}>
        <Hi>❯</Hi> <span className="dim">spaw</span><span className="hi">▏</span>
      </div>
      {rows.map((r, i) => (
        <div key={i} style={{ display: "flex", padding: "5px 9px", borderRadius: 2, background: r[2] ? "rgba(255,166,12,.1)" : "transparent", fontSize: 12 }}>
          <span className={r[2] ? "hi" : "ink"} style={{ flex: 1 }}>{r[2] ? "▶ " : "  "}{r[0]}</span>
          <span className="faint">{r[1]}</span>
        </div>
      ))}
    </Screen>
  );
}

function HelpOverlayMock() {
  const groups = [["GLOBAL", [["1-6", "jump tab"], ["⌘K", "command"], ["?", "help"], ["q", "quit"]]], ["FLEET", [["s", "spawn"], ["e", "exec"], ["f", "files"], ["d", "kill"]]], ["LISTS", [["j/k", "move"], ["↵", "open"], ["/", "filter"]]]];
  return (
    <Screen label="? — help overlay" right="press any key to close">
      <div className="grid3">
        {groups.map((g, i) => (
          <Mod key={i} title={g[0]}>
            <div style={{ display: "grid", gap: 6 }}>
              {g[1].map((k, j) => <div key={j} className="keyrow"><Key>{k[0]}</Key><span className="dim">{k[1]}</span></div>)}
            </div>
          </Mod>
        ))}
      </div>
      <div className="faint" style={{ fontSize: 11, marginTop: 11, textAlign: "center" }}>concepts: sandbox · template · provider · worker — <Hi>?c</Hi> for the glossary</div>
    </Screen>
  );
}

function UXFixes() {
  const fixes = [
    ["Hidden 1–6 shortcuts", "Sidebar showed only icon + name.", "Every nav item shows its number; palette & slash menu list them all."],
    ["Dead Quick-Spawn", "s did nothing on the Dashboard.", "Spawn is a global command — palette, slash, or s, from anywhere."],
    ["Files write-by-accident", "Empty content = read, text = write (silent).", "Explicit WRITE-mode banner (A/B) or distinct /read · /write verbs (C)."],
    ["Truncated errors", "Status bar cut errors at 50 chars.", "Errors are full blocks in Logs / the stream — scrollable, never cut."],
    ["Lonely spawn loader", "Progress only animated on the Sandboxes tab.", "Spawn progress shows globally — ribbon (A), inline block (C)."],
    ["Config dead-ends", "Only 6 hard-coded keys reachable.", "Type any config key path; common ones offered as chips."],
    ["Generic hints", "Every tab showed r:refresh q:quit.", "Context-specific key hints per screen & mode."],
    ["No help / glossary", "No ? overlay; concepts undocumented.", "? overlay with grouped keys + a sandbox/template/provider glossary."],
  ];
  return (
    <div>
      <div className="secthead">
        <div className="kicker">Cross-cutting</div>
        <h1>The rough edges, fixed <span className="hand">— same in all three</span></h1>
        <div className="lede">These come straight from your hand-off notes. They're solved the same way regardless of which visual direction you pick — mostly by adding a <b>command palette / slash menu</b>, a <b>help overlay</b>, and treating <b>status as history</b> rather than a single truncated line.</div>
      </div>
      <div className="scrgrid" style={{ marginBottom: 8 }}>
        <ScreenCard title="Universal command palette (⌘K / :)" hand="discoverability, fixed once" label="ALL"><CmdPaletteMock /></ScreenCard>
        <ScreenCard title="Help overlay (?) with a glossary" hand="no more hunting for shortcuts" label="ALL"><HelpOverlayMock /></ScreenCard>
      </div>
      <Divider>fix matrix</Divider>
      <div className="problemlist">
        {fixes.map((f, i) => (
          <div key={i} style={{ border: "1px solid var(--line)", borderRadius: 3, padding: "13px 15px", background: "var(--panel)" }}>
            <div className="b" style={{ fontSize: 13, marginBottom: 6 }}>{f[0]}</div>
            <div className="prob" style={{ marginBottom: 5 }}><span className="x">✗</span><span className="dim">{f[1]}</span></div>
            <div className="prob"><span className="ok">✓</span><span><span className="ok">{f[2]}</span></span></div>
          </div>
        ))}
      </div>
    </div>
  );
}

Object.assign(window, { Overview, LogoLab, UXFixes });
