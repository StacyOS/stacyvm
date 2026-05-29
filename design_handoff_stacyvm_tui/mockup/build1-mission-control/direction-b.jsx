// ====================================================================
// direction-b.jsx — "HUD MODULAR GRID"
// Slim icon dock + corner-bracket tactical frames + mono-grid rules.
// ====================================================================

// bracket-framed panel ----------------------------------------------
function BFrame({ title, right, accent, children, style }) {
  const c = accent ? "var(--orange)" : "var(--steel)";
  const corner = (pos) => {
    const s = { position: "absolute", color: c, fontSize: 12, lineHeight: "10px", opacity: accent ? 0.95 : 0.6, pointerEvents: "none" };
    if (pos === "tl") return { ...s, top: 2, left: 4 };
    if (pos === "tr") return { ...s, top: 2, right: 4 };
    if (pos === "bl") return { ...s, bottom: 2, left: 4 };
    return { ...s, bottom: 2, right: 4 };
  };
  return (
    <div style={{ position: "relative", border: "1px solid " + (accent ? "rgba(255,166,12,.3)" : "var(--line)"), padding: "12px 13px 11px", background: "rgba(255,255,255,.01)", ...style }}>
      <span style={corner("tl")}>⌜</span><span style={corner("tr")}>⌝</span>
      <span style={corner("bl")}>⌞</span><span style={corner("br")}>⌟</span>
      {title && (
        <div style={{ display: "flex", alignItems: "center", gap: 7, marginBottom: 9, fontSize: 9.5, letterSpacing: 2, textTransform: "uppercase", color: accent ? "var(--orange)" : "var(--dim)" }}>
          <span>{title}</span>
          <span style={{ flex: 1, height: 1, background: "var(--line)" }} />
          {right && <span className="faint" style={{ letterSpacing: 1 }}>{right}</span>}
        </div>
      )}
      {children}
    </div>
  );
}

function DockB({ active = 0 }) {
  const items = [["◈", "DASH"], ["▤", "SBX"], ["◆", "TPL"], ["⬡", "PRV"], ["≡", "LOG"], ["⌗", "CFG"]];
  return (
    <div style={{ width: 58, flex: "none", borderRight: "1px solid var(--line)", paddingRight: 11, display: "flex", flexDirection: "column", gap: 4, alignItems: "stretch" }}>
      <div style={{ marginBottom: 8, display: "flex", justifyContent: "center" }}>
        <LogoArt size="header" style={{ fontSize: 4, lineHeight: "4px" }} />
      </div>
      {items.map((it, i) => (
        <div key={i} style={{
          position: "relative", textAlign: "center", padding: "7px 0", borderRadius: 2,
          color: i === active ? "var(--orange)" : "var(--dim)",
          background: i === active ? "rgba(255,166,12,.08)" : "transparent",
        }}>
          {i === active && <span style={{ position: "absolute", left: 0, top: 6, bottom: 6, width: 2, background: "var(--orange)" }} />}
          <div style={{ fontSize: 15, lineHeight: "16px" }}>{it[0]}</div>
          <div style={{ fontSize: 7.5, letterSpacing: 1, marginTop: 2 }}>{i + 1}·{it[1]}</div>
        </div>
      ))}
      <div style={{ flex: 1 }} />
      <div style={{ textAlign: "center", fontSize: 9 }}><Ok>●</Ok></div>
    </div>
  );
}

function HeadB({ title, clock = "17:38" }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 13, fontSize: 11, letterSpacing: 2 }}>
      <span className="hi b">▶ {title}</span>
      <span style={{ flex: 1, height: 1, background: "var(--line)" }} />
      <span className="dim">{clock}</span>
      <span className="ok">● ONLINE</span>
      <span className="faint">v0.9.2</span>
    </div>
  );
}

function FootB({ left, hints }) {
  return (
    <div style={{ display: "flex", alignItems: "center", marginTop: 13, paddingTop: 9, borderTop: "1px solid var(--line)", fontSize: 11 }}>
      <span className="faint">{left}</span><span style={{ flex: 1 }} />
      {hints && <KeyHint items={hints} />}
    </div>
  );
}

function Shell({ active, title, foot, children }) {
  return (
    <Screen label={"stacyvm › " + title.toLowerCase()} right="tactical hud">
      <div style={{ display: "flex", gap: 13 }}>
        <DockB active={active} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <HeadB title={title} />
          {children}
          {foot}
        </div>
      </div>
    </Screen>
  );
}

// DASHBOARD (hero) ---------------------------------------------------
function DashB() {
  const tiles = [["SANDBOXES", "5", "ok", [3,4,5,4,5,6,5]], ["TEMPLATES", "4", "hi", [2,2,3,3,4,4,4]], ["PROVIDERS", "2/3", "ok", [2,2,2,2,2,2,2]], ["UPTIME", "4d", "mint", [5,5,5,6,6,6,7]]];
  return (
    <Shell active={0} title="DASHBOARD" foot={<FootB left="5 sbx · 4 tpl · 2 prv" hints={[["1-6", "nav"], ["s", "spawn"], ["?", "help"]]} />}>
      <div className="grid4" style={{ marginBottom: 11 }}>
        {tiles.map((t, i) => (
          <BFrame key={i} title={t[0]} accent={i === 0}>
            <div style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between" }}>
              <span className={"metric " + t[2]}>{t[1]}</span>
              <Spark data={t[3]} color={t[2]} />
            </div>
          </BFrame>
        ))}
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "1.5fr 1fr", gap: 11 }}>
        <BFrame title="FLEET · RUNNING" right="↑↓ SELECT" accent>
          <table className="tbl">
            <thead><tr><th></th><th>ID</th><th>IMAGE</th><th>CPU</th><th>TTL</th></tr></thead>
            <tbody>
              <tr className="sel"><td className="hi">▶</td><td className="b">sb-7f3a91</td><td className="dim">python:3.12</td><td><Meter val={71} width={5} showPct={false} /></td><td className="dim">24m</td></tr>
              <tr><td></td><td>sb-2c8e04</td><td className="dim">node:20</td><td><Meter val={38} width={5} showPct={false} color="ok" /></td><td className="dim">12m</td></tr>
              <tr><td></td><td><Hi>sb-9d11ba</Hi></td><td className="dim">alpine</td><td className="hi">◐ boot</td><td className="dim">30m</td></tr>
              <tr><td></td><td>sb-4a77e2</td><td className="dim">ubuntu</td><td><Meter val={12} width={5} showPct={false} color="ok" /></td><td className="dim">08m</td></tr>
            </tbody>
          </table>
        </BFrame>
        <div style={{ display: "grid", gap: 11 }}>
          <BFrame title="HOST">
            <div style={{ fontSize: 11, display: "grid", gap: 5 }} className="dim">
              <div>CPU <Meter val={34} width={11} /></div>
              <div>MEM <Meter val={61} width={11} /></div>
              <div>NET <Spark data={[3,5,4,7,5,8,6,5,7,6]} /> <span className="ok">1.2MB/s</span></div>
            </div>
          </BFrame>
          <BFrame title="ACTIVITY">
            <div style={{ fontSize: 10, lineHeight: 1.75 }}>
              <div><Hi>SPAWN</Hi> <span className="dim">sb-7f3a91</span></div>
              <div><Ok>EXEC</Ok> <span className="dim">exit 0</span></div>
              <div><span className="steel">WRITE</span> <span className="dim">main.py</span></div>
            </div>
          </BFrame>
        </div>
      </div>
    </Shell>
  );
}

// SANDBOXES ----------------------------------------------------------
function SandboxesB() {
  return (
    <Shell active={1} title="SANDBOXES" foot={<FootB left="6 sbx" hints={[["j/k", "move"], ["↵", "open"], ["d", "kill"]]} />}>
      <div style={{ display: "grid", gridTemplateColumns: "1.4fr 1fr", gap: 11 }}>
        <BFrame title="FLEET" right="6">
          <table className="tbl">
            <thead><tr><th></th><th>ID</th><th>STATE</th><th>IMAGE</th></tr></thead>
            <tbody>
              <tr className="sel"><td className="hi">▶</td><td className="b">sb-7f3a91</td><td><Ok>● run</Ok></td><td className="dim">python</td></tr>
              <tr><td></td><td>sb-2c8e04</td><td><Ok>● run</Ok></td><td className="dim">node</td></tr>
              <tr><td></td><td>sb-9d11ba</td><td><Hi>◐ boot</Hi></td><td className="dim">alpine</td></tr>
              <tr><td></td><td>sb-1e90cf</td><td className="dim">○ idle</td><td className="dim">rust</td></tr>
            </tbody>
          </table>
        </BFrame>
        <BFrame title="sb-7f3a91" right="INSPECT" accent>
          <div style={{ fontSize: 11, lineHeight: 1.85 }} className="dim">
            <div>state <Ok>● running</Ok></div>
            <div>image <span className="ink">python:3.12-alpine</span></div>
            <div>via <span className="ink">docker · runsc</span></div>
            <div>ttl <Meter val={80} width={8} color="hi" /></div>
            <div style={{ marginTop: 6 }}>cpu <Meter val={71} width={9} /></div>
            <div>mem <Meter val={48} width={9} color="ok" /></div>
          </div>
          <div style={{ marginTop: 9 }} className="keyrow"><span><Key>e</Key>exec</span><span><Key>f</Key>files</span><span><Key>d</Key>kill</span></div>
        </BFrame>
      </div>
    </Shell>
  );
}

// SPAWN --------------------------------------------------------------
function SpawnB() {
  return (
    <Shell active={1} title="SANDBOXES / SPAWN" foot={<FootB left="" hints={[["tab", "field"], ["↵", "spawn"], ["esc", "cancel"]]} />}>
      <BFrame title="◇ SPAWN SANDBOX" accent>
        <div style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "9px 14px", fontSize: 12, alignItems: "center" }}>
          <span className="dim">IMAGE</span>
          <span style={{ borderBottom: "1px solid var(--orange)", paddingBottom: 2 }}>python:3.12-alpine<span className="hi">▏</span></span>
          <span className="dim">TEMPLATE</span>
          <span className="dim" style={{ borderBottom: "1px solid var(--line-2)", paddingBottom: 2 }}>data-science <span className="faint">▾</span></span>
          <span className="dim">TTL</span>
          <span className="dim" style={{ borderBottom: "1px solid var(--line-2)", paddingBottom: 2 }}>30m</span>
          <span className="dim">PROVIDER</span>
          <span className="keyrow"><span className="hi" style={{ border: "1px solid var(--orange)", padding: "2px 8px" }}>◆ docker</span><span style={{ padding: "2px 8px" }} className="dim">firecracker</span><span style={{ padding: "2px 8px" }} className="dim">proot</span></span>
        </div>
      </BFrame>
      <BFrame title="PREVIEW" style={{ marginTop: 11 }}>
        <div className="dim" style={{ fontSize: 11 }}>spawn <span className="ink">python:3.12-alpine</span> via <span className="ink">docker(runsc)</span> · ttl <span className="ink">30m</span> · est. boot <span className="ok">~1.2s</span></div>
      </BFrame>
    </Shell>
  );
}

// EXEC / FILES -------------------------------------------------------
function ExecB() {
  return (
    <Shell active={1} title="SBX / sb-7f3a91 / EXEC" foot={<FootB left="" hints={[["↵", "run"], ["↑", "hist"], ["esc", "back"]]} />}>
      <BFrame title="◇ EXEC" accent>
        <div style={{ fontSize: 12 }}><Ok>$</Ok> <span className="ink">python -m pytest -q</span><span className="hi">▏</span></div>
      </BFrame>
      <BFrame title="OUTPUT" right="exit 0 · 1.9s" style={{ marginTop: 11 }}>
        <div className="mint" style={{ fontSize: 11.5, lineHeight: 1.7 }}>
          <div>........................ <Ok>[100%]</Ok></div>
          <div className="dim">24 passed in 1.83s</div>
        </div>
      </BFrame>
    </Shell>
  );
}

function FilesB() {
  return (
    <Shell active={1} title="SBX / sb-7f3a91 / FILES" foot={<FootB left="WRITE mode" hints={[["^o", "read"], ["^s", "save"]]} />}>
      <div style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: 11 }}>
        <BFrame title="TREE" style={{ minWidth: 145 }}>
          <div style={{ fontSize: 11, lineHeight: 1.85 }}>
            <div className="dim">▾ workspace</div>
            <div style={{ paddingLeft: 10 }} className="hi">▾ src</div>
            <div style={{ paddingLeft: 22 }}>main.py</div>
            <div style={{ paddingLeft: 22 }} className="dim">utils.py</div>
          </div>
        </BFrame>
        <BFrame title="main.py · WRITE" accent>
          <div style={{ fontSize: 11, lineHeight: 1.7 }}>
            <div><span className="faint">1</span> <Hi>import</Hi> os</div>
            <div><span className="faint">2</span> <Hi>def</Hi> <span className="mint">main</span>():</div>
            <div><span className="faint">3</span> &nbsp;print(<Ok>"hi"</Ok>)<span className="hi">▏</span></div>
          </div>
        </BFrame>
      </div>
    </Shell>
  );
}

// TEMPLATES ----------------------------------------------------------
function TemplatesB() {
  return (
    <Shell active={2} title="TEMPLATES" foot={<FootB left="4 tpl" hints={[["s", "spawn"], ["n", "new"]]} />}>
      <div style={{ display: "grid", gridTemplateColumns: "1.3fr 1fr", gap: 11 }}>
        <BFrame title="DEFINED" right="4">
          <table className="tbl">
            <thead><tr><th></th><th>NAME</th><th>MEM</th><th>POOL</th></tr></thead>
            <tbody>
              <tr className="sel"><td className="hi">▶</td><td className="b">data-science</td><td className="dim">512</td><td className="ok">3</td></tr>
              <tr><td></td><td>web-build</td><td className="dim">1024</td><td className="dim">0</td></tr>
              <tr><td></td><td>rust-ci</td><td className="dim">2048</td><td className="dim">1</td></tr>
            </tbody>
          </table>
        </BFrame>
        <BFrame title="data-science" accent>
          <div className="dim" style={{ fontSize: 11, lineHeight: 1.7 }}>
            <span className="ink b">Data Science Stack</span><br />numpy · pandas · sklearn pre-warmed. Pool keeps <Hi>3</Hi> ready.
            <div style={{ marginTop: 7 }}>512MB · 1 cpu · 300s</div>
          </div>
        </BFrame>
      </div>
    </Shell>
  );
}

// PROVIDERS ----------------------------------------------------------
function ProvidersB() {
  return (
    <Shell active={3} title="PROVIDERS" foot={<FootB left="2 healthy" hints={[["r", "refresh"], ["↵", "default"]]} />}>
      <div className="grid3">
        <BFrame title="DOCKER" right="DEFAULT" accent>
          <div style={{ fontSize: 11, lineHeight: 1.85 }}><Ok>● healthy</Ok><div className="dim">runsc · 4 sbx</div><div className="dim">12ms <Spark data={[2,3,2,4,3,2]} color="ok" /></div></div>
        </BFrame>
        <BFrame title="FIRECRACKER">
          <div style={{ fontSize: 11, lineHeight: 1.85 }}><Ok>● healthy</Ok><div className="dim">kvm ok · 1 sbx</div><div className="dim">8ms <Spark data={[2,2,3,2,3,2]} color="ok" /></div></div>
        </BFrame>
        <BFrame title="PROOT">
          <div style={{ fontSize: 11, lineHeight: 1.85 }} className="dim">○ standby<div>userspace · 0 sbx</div><div className="faint">no kvm</div></div>
        </BFrame>
      </div>
    </Shell>
  );
}

// LOGS ---------------------------------------------------------------
function LogsB() {
  const rows = [["17:38:04", "SPAWN", "hi"], ["17:38:02", "EXEC", "ok"], ["17:37:51", "WRITE", "steel"], ["17:37:30", "TEMPLATE", "mint"], ["17:36:58", "KILL", "err"]];
  const txt = ["sb-7f3a91 docker python:3.12", "sb-2c8e04 exit=0 1.9s", "/workspace/src/main.py", "data-science pool=3", "sb-0a31ff ttl-expired"];
  return (
    <Shell active={4} title="LOGS" foot={<FootB left="5 events" hints={[["/", "filter"], ["c", "copy"]]} />}>
      <BFrame title="EVENT STREAM" right="◉ FOLLOW" accent>
        <div style={{ fontSize: 11, lineHeight: 1.9 }}>
          {rows.map((r, i) => <div key={i}><span className="faint">{r[0]}</span> <span className={r[2] + " b"} style={{ display: "inline-block", width: 74 }}>{r[1]}</span> <span className="dim">{txt[i]}</span></div>)}
        </div>
      </BFrame>
    </Shell>
  );
}

// CONFIG -------------------------------------------------------------
function ConfigB() {
  return (
    <Shell active={5} title="CONFIG" foot={<FootB left="live patch" hints={[["space", "apply"]]} />}>
      <BFrame title="◇ PROVIDERS" accent>
        <div className="dim" style={{ fontSize: 11.5 }}>default provider</div>
        <div className="keyrow" style={{ gap: 6, marginBottom: 8 }}><span className="hi" style={{ border: "1px solid var(--orange)", padding: "2px 9px" }}>docker</span><span style={{ padding: "2px 9px" }}>firecracker</span><span style={{ padding: "2px 9px" }}>proot</span></div>
        <div className="dim" style={{ fontSize: 11.5 }}>docker runtime</div>
        <div className="keyrow" style={{ gap: 6 }}><span style={{ padding: "2px 9px" }}>runc</span><span className="hi" style={{ border: "1px solid var(--orange)", padding: "2px 9px" }}>runsc</span><span style={{ padding: "2px 9px" }}>kata</span></div>
      </BFrame>
    </Shell>
  );
}

function DirectionB() {
  return (
    <div>
      <div className="secthead">
        <div className="kicker">Direction B</div>
        <h1>HUD Modular Grid <span className="hand">— tactical</span></h1>
        <div className="lede">A <b>slim icon dock</b> replaces the wide sidebar — six glyphs, numbered, collapsible. Content snaps to a strict <b>mono-grid of corner-bracketed modules</b> (⌜ ⌝ ⌞ ⌟) with a selection reticle (▶). The most "terminal-native" of the three: heavy box-drawing, structural, reads like a targeting console.</div>
      </div>
      <div className="moves">
        <Move t="Nav model">Vertical icon dock, each item numbered 1–6 (icon + number = discoverable shortcut). Collapses to glyphs on narrow terminals.</Move>
        <Move t="Signature">Corner-bracket frames + ▶ reticle selection. Everything lives on a visible grid.</Move>
        <Move t="Feel">Brutalist, structural, unmistakably a terminal. Cheapest to build in lipgloss.</Move>
      </div>
      <div className="herorow">
        <DashB />
        <div className="notes">
          <Note tag="Dock">Icon + number means the 1–6 shortcuts are finally <b>visible</b>. Collapses gracefully when the terminal is narrow.</Note>
          <Note tag="Tiles">Four bracketed KPI tiles with sparklines give an at-a-glance fleet read before you scan the table.</Note>
          <Note tag="Reticle">▶ marks the selected row across every list — one consistent selection language.</Note>
          <Note tag="Grid">Pure box-drawing + brackets — no images required, identical on every OS &amp; terminal.</Note>
        </div>
      </div>
      <Divider>secondary screens &amp; modes</Divider>
      <div className="scrgrid">
        <ScreenCard title="Sandboxes — fleet + inspect" label="B"><SandboxesB /></ScreenCard>
        <ScreenCard title="Spawn — bracketed form + preview" hand="shows what'll happen before ↵" label="B"><SpawnB /></ScreenCard>
        <ScreenCard title="Exec — command + output frame" label="B"><ExecB /></ScreenCard>
        <ScreenCard title="Files — WRITE mode is explicit" hand="mode label up top, no guessing" label="B"><FilesB /></ScreenCard>
        <ScreenCard title="Templates — list + detail" label="B"><TemplatesB /></ScreenCard>
        <ScreenCard title="Providers — bracketed health" label="B"><ProvidersB /></ScreenCard>
        <ScreenCard title="Logs — follow + filter" label="B"><LogsB /></ScreenCard>
        <ScreenCard title="Config — segmented toggles" label="B"><ConfigB /></ScreenCard>
      </div>
    </div>
  );
}

window.DirectionB = DirectionB;
