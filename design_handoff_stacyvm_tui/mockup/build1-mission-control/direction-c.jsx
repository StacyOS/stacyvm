// ====================================================================
// direction-c.jsx — "CONVERSATIONAL STREAM"  (Claude Code-inspired)
// No tabs. A persistent command line; results stream in as blocks.
// Slash menu = navigation + command palette in one.
// ====================================================================

function StreamHeader({ clock = "17:38:05" }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 10, paddingBottom: 10, borderBottom: "1px solid var(--line)", marginBottom: 14, fontSize: 11 }}>
      <LogoArt size="header" style={{ fontSize: 4.5, lineHeight: "4.5px" }} />
      <b style={{ letterSpacing: 2, fontSize: 12 }}>STACYVM</b>
      <span style={{ flex: 1 }} />
      <span className="dim">5 running · 4 tpl</span>
      <span className="dim">cpu <Hi>34%</Hi></span>
      <span className="dim">{clock}</span>
      <span className="ok">● ONLINE</span>
    </div>
  );
}

// a block in the stream ----------------------------------------------
function Block({ kind = "sys", label, children, style }) {
  const railc = { user: "var(--orange)", sys: "var(--steel)", ok: "var(--green)", err: "var(--red)", run: "var(--orange)" }[kind];
  return (
    <div style={{ display: "flex", gap: 12, marginBottom: 14, ...style }}>
      <div style={{ width: 2, flex: "none", background: railc, opacity: kind === "sys" ? 0.4 : 0.85, borderRadius: 2 }} />
      <div style={{ flex: 1, minWidth: 0 }}>
        {label && <div style={{ fontSize: 9, letterSpacing: 1.5, textTransform: "uppercase", color: railc, marginBottom: 5 }}>{label}</div>}
        {children}
      </div>
    </div>
  );
}

function UserCmd({ children }) {
  return <div style={{ marginBottom: 14, fontSize: 13 }}><span className="hi b">❯ </span><span className="ink">{children}</span></div>;
}

// slash menu popover -------------------------------------------------
function SlashMenu() {
  const cmds = [
    ["/spawn", "launch a sandbox", true],
    ["/sandboxes", "list & manage the fleet", false],
    ["/exec", "run a command in a sandbox", false],
    ["/files", "read or write files", false],
    ["/templates", "browse & create templates", false],
    ["/providers", "provider health", false],
    ["/logs", "filter the event stream", false],
    ["/config", "edit configuration", false],
    ["/help", "keymap & concepts", false],
  ];
  return (
    <div style={{ border: "1px solid rgba(255,166,12,.3)", borderRadius: 3, background: "var(--panel-2)", padding: 6, marginBottom: 8, boxShadow: "0 -18px 40px -18px rgba(0,0,0,.8)" }}>
      <div style={{ fontSize: 9, letterSpacing: 2, color: "var(--dim)", padding: "3px 8px 6px" }}>COMMANDS · fuzzy filter as you type</div>
      {cmds.map((c, i) => (
        <div key={c[0]} style={{ display: "flex", gap: 12, padding: "4px 9px", borderRadius: 2, background: c[2] ? "rgba(255,166,12,.1)" : "transparent", fontSize: 12 }}>
          <span className={c[2] ? "hi b" : "ink"} style={{ width: 96 }}>{c[0]}</span>
          <span className="dim">{c[1]}</span>
        </div>
      ))}
    </div>
  );
}

function InputBar({ value = "", placeholder = "type a command, or / for menu", menu, hint }) {
  return (
    <div style={{ marginTop: 16 }}>
      {menu && <SlashMenu />}
      <div style={{ border: "1px solid " + (menu ? "rgba(255,166,12,.4)" : "var(--line-2)"), borderRadius: 3, padding: "9px 12px", display: "flex", alignItems: "center", gap: 9, background: "var(--panel-2)" }}>
        <span className="hi b">❯</span>
        {value ? <span className="ink" style={{ fontSize: 13 }}>{value}<span className="hi">▏</span></span> : <span className="faint" style={{ fontSize: 13 }}>{placeholder}<span className="hi">▏</span></span>}
        <span style={{ flex: 1 }} />
        <span className="faint" style={{ fontSize: 10 }}>{hint || "/ menu · ↑ history · ⏎ run"}</span>
      </div>
    </div>
  );
}

// HERO — the live stream --------------------------------------------
function StreamC() {
  return (
    <Screen label="stacyvm" right="conversational" dots={true}>
      <StreamHeader />
      <Block kind="sys">
        <div style={{ display: "flex", gap: 16, alignItems: "center" }}>
          <LogoArt size="hero" style={{ fontSize: 7, lineHeight: "7px" }} />
          <div>
            <div className="ink b" style={{ fontSize: 14, letterSpacing: 1 }}>StacyVM v0.9.2</div>
            <div className="dim" style={{ fontSize: 12, marginTop: 4 }}>microVM sandbox orchestrator for LLMs.</div>
            <div className="faint" style={{ fontSize: 11, marginTop: 6 }}>5 sandboxes running · type <Hi>/</Hi> to see what you can do, or just describe what you want.</div>
          </div>
        </div>
      </Block>

      <UserCmd>spawn a python box from data-science</UserCmd>
      <Block kind="run" label="spawning · sb-9d11ba">
        <div style={{ fontSize: 12 }} className="dim">
          docker · python:3.12-alpine · ttl 30m<br />
          <span className="hi">[</span><span className="hi">●●●●●●</span><span className="faint">●●●●</span><span className="hi">]</span> <span className="hi">booting</span> <span className="faint">pulling layers… 1.1s</span>
        </div>
      </Block>

      <UserCmd>/sandboxes</UserCmd>
      <Block kind="sys" label="fleet · 5 running">
        <table className="tbl">
          <thead><tr><th>ID</th><th>STATE</th><th>IMAGE</th><th>CPU</th><th>TTL</th></tr></thead>
          <tbody>
            <tr><td className="b">sb-7f3a91</td><td><Ok>● run</Ok></td><td className="dim">python:3.12</td><td><Meter val={71} width={6} showPct={false} /></td><td className="dim">24m</td></tr>
            <tr><td>sb-2c8e04</td><td><Ok>● run</Ok></td><td className="dim">node:20</td><td><Meter val={38} width={6} showPct={false} color="ok" /></td><td className="dim">12m</td></tr>
            <tr><td><Hi>sb-9d11ba</Hi></td><td><Hi>◐ boot</Hi></td><td className="dim">alpine</td><td className="faint">—</td><td className="dim">30m</td></tr>
          </tbody>
        </table>
        <div className="keyrow" style={{ marginTop: 8 }}><span className="faint">act on a row:</span><span><Key>e</Key> exec</span><span><Key>f</Key> files</span><span><Key>d</Key> kill</span></div>
      </Block>

      <InputBar menu={true} value="/" />
      <div className="keyrow" style={{ marginTop: 9, justifyContent: "center" }}>
        <span><Key>esc</Key> close</span><span><Key>↑↓</Key> select</span><span><Key>⏎</Key> run</span><span><Key>?</Key> help</span>
      </div>
    </Screen>
  );
}

// SPAWN flow ---------------------------------------------------------
function SpawnFlowC() {
  return (
    <Screen label="stacyvm › spawn flow">
      <StreamHeader />
      <UserCmd>/spawn</UserCmd>
      <Block kind="sys" label="spawn · fill in or accept defaults">
        <div style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "8px 14px", fontSize: 12, alignItems: "center" }}>
          <span className="dim">image</span><span style={{ borderBottom: "1px solid var(--orange)", paddingBottom: 2 }}>python:3.12-alpine<span className="hi">▏</span></span>
          <span className="dim">template</span><span className="dim">data-science <span className="faint">(↹ to change)</span></span>
          <span className="dim">ttl</span><span className="dim">30m</span>
        </div>
      </Block>
      <Block kind="ok" label="ready · 1.4s">
        <div style={{ fontSize: 12 }}><Ok>✓</Ok> <span className="ink b">sb-9d11ba</span> running · <span className="dim">docker(runsc) · python:3.12 · expires 30m</span></div>
        <div className="keyrow" style={{ marginTop: 7 }}><span><Key>e</Key> exec here</span><span><Key>f</Key> open files</span><span><Key>↵</Key> attach</span></div>
      </Block>
      <InputBar placeholder="exec a command in sb-9d11ba…" />
    </Screen>
  );
}

// EXEC ---------------------------------------------------------------
function ExecC() {
  return (
    <Screen label="stacyvm › exec">
      <StreamHeader />
      <UserCmd>exec sb-7f3a91 — python -m pytest -q</UserCmd>
      <Block kind="ok" label="sb-7f3a91 · exit 0 · 1.9s">
        <div className="mint" style={{ fontSize: 11.5, lineHeight: 1.7 }}>
          <div>........................ <Ok>[100%]</Ok></div>
          <div className="dim">24 passed in 1.83s</div>
        </div>
      </Block>
      <UserCmd>exec sb-7f3a91 — cat /missing</UserCmd>
      <Block kind="err" label="sb-7f3a91 · exit 1">
        <div className="err" style={{ fontSize: 11.5 }}>cat: /missing: No such file or directory</div>
        <div className="faint" style={{ fontSize: 11, marginTop: 4 }}>full error kept in scrollback — never truncated.</div>
      </Block>
      <InputBar placeholder="run another command…" />
    </Screen>
  );
}

// FILES --------------------------------------------------------------
function FilesC() {
  return (
    <Screen label="stacyvm › files">
      <StreamHeader />
      <UserCmd>/read sb-7f3a91 /workspace/src/main.py</UserCmd>
      <Block kind="sys" label="read · 412 B">
        <div style={{ fontSize: 11, lineHeight: 1.7 }}>
          <div><span className="faint">1</span> <Hi>import</Hi> os</div>
          <div><span className="faint">2</span> <Hi>def</Hi> <span className="mint">main</span>(): print(<Ok>"hi"</Ok>)</div>
        </div>
      </Block>
      <UserCmd>/write sb-7f3a91 /workspace/run.sh</UserCmd>
      <Block kind="ok" label="write · separate command — never accidental">
        <div style={{ fontSize: 12 }}><Ok>✓</Ok> <span className="dim">wrote 64 B to /workspace/run.sh</span></div>
      </Block>
      <InputBar placeholder="/read or /write …" hint="read & write are distinct verbs" />
    </Screen>
  );
}

// TEMPLATES ----------------------------------------------------------
function TemplatesC() {
  return (
    <Screen label="stacyvm › templates">
      <StreamHeader />
      <UserCmd>/templates</UserCmd>
      <Block kind="sys" label="templates · 4">
        <table className="tbl">
          <thead><tr><th>NAME</th><th>IMAGE</th><th>POOL</th></tr></thead>
          <tbody>
            <tr><td className="b">data-science</td><td className="dim">python:3.12</td><td className="ok">3 warm</td></tr>
            <tr><td>web-build</td><td className="dim">node:20</td><td className="dim">0</td></tr>
            <tr><td>rust-ci</td><td className="dim">rust:1.78</td><td className="dim">1 warm</td></tr>
          </tbody>
        </table>
      </Block>
      <Block kind="sys" label="data-science">
        <div className="dim" style={{ fontSize: 11.5 }}>numpy · pandas · sklearn pre-warmed · 512MB / 1 cpu / 300s</div>
      </Block>
      <InputBar placeholder="/spawn data-science  or  /template new …" />
    </Screen>
  );
}

// PROVIDERS ----------------------------------------------------------
function ProvidersC() {
  return (
    <Screen label="stacyvm › providers">
      <StreamHeader />
      <UserCmd>/providers</UserCmd>
      <Block kind="sys" label="providers · 2 healthy">
        <div style={{ fontSize: 12, lineHeight: 1.95 }}>
          <div><Ok>●</Ok> <span className="ink">docker</span> <span className="faint">default · runsc</span> · 4 sbx · <span className="ok">12ms</span> <Spark data={[2,3,2,4,3,2]} color="ok" /></div>
          <div><Ok>●</Ok> <span className="ink">firecracker</span> <span className="faint">kvm ok</span> · 1 sbx · <span className="ok">8ms</span> <Spark data={[2,2,3,2,3]} color="ok" /></div>
          <div><span className="dim">○ proot</span> <span className="faint">standby · userspace · no kvm</span></div>
        </div>
      </Block>
      <InputBar placeholder="/config providers.default=…" />
    </Screen>
  );
}

// LOGS ---------------------------------------------------------------
function LogsC() {
  const rows = [["17:38:04", "SPAWN", "hi", "sb-7f3a91 docker python:3.12"], ["17:38:02", "EXEC", "ok", "sb-2c8e04 exit=0 1.9s"], ["17:37:51", "WRITE", "steel", "/workspace/src/main.py"], ["17:36:58", "KILL", "err", "sb-0a31ff ttl-expired"]];
  return (
    <Screen label="stacyvm › logs">
      <StreamHeader />
      <UserCmd>/logs kind:spawn,exec,error</UserCmd>
      <Block kind="sys" label="event stream · filtered · following">
        <div style={{ fontSize: 11.5, lineHeight: 1.9 }}>
          {rows.map((r, i) => <div key={i}><span className="faint">{r[0]}</span> <span className={r[2] + " b"} style={{ display: "inline-block", width: 70 }}>{r[1]}</span> <span className="dim">{r[3]}</span></div>)}
        </div>
      </Block>
      <InputBar placeholder="/logs kind:… or text…" hint="filters are part of the command" />
    </Screen>
  );
}

// CONFIG -------------------------------------------------------------
function ConfigC() {
  return (
    <Screen label="stacyvm › config">
      <StreamHeader />
      <UserCmd>/config</UserCmd>
      <Block kind="sys" label="config · type a key or pick">
        <div className="dim" style={{ fontSize: 11.5 }}>providers.default</div>
        <div className="keyrow" style={{ gap: 6, marginBottom: 8 }}><span className="hi" style={{ border: "1px solid var(--orange)", padding: "2px 9px" }}>docker</span><span className="dim" style={{ padding: "2px 9px" }}>firecracker</span><span className="dim" style={{ padding: "2px 9px" }}>proot</span></div>
        <div className="faint" style={{ fontSize: 11 }}>…or type any key path: <span className="ink">providers.docker.runtime=kata</span></div>
      </Block>
      <Block kind="ok" label="patched"><div style={{ fontSize: 12 }}><Ok>✓</Ok> <span className="dim">providers.docker.runtime = runsc</span></div></Block>
      <InputBar value="config providers.docker.runtime=runsc" />
    </Screen>
  );
}

function DirectionC() {
  return (
    <div>
      <div className="secthead">
        <div className="kicker">Direction C</div>
        <h1>Conversational Stream <span className="hand">— the Claude Code one</span></h1>
        <div className="lede">No tabs at all. A single scrolling stream with a <b>persistent command line</b>. Type a command (or plain English) and the result streams back as a <b>block</b>. A <b>slash menu</b> doubles as navigation and command palette. Spawns animate inline; errors are just blocks you scroll back to — nothing is ever truncated. This is the most dynamic, and the most "LLM-operator" of the three.</div>
      </div>
      <div className="moves">
        <Move t="Nav model">There is no nav — <b>/</b> opens a fuzzy command menu. Everything is reachable by typing.</Move>
        <Move t="Signature">Stream of result blocks + an always-present input bar. Spawns show inline progress.</Move>
        <Move t="Feel">Dynamic & alive, like an agent session. Natural for an LLM driving the tool too.</Move>
      </div>
      <div className="herorow">
        <StreamC />
        <div className="notes">
          <Note tag="Input bar">One persistent prompt drives everything. <b>/</b> opens the fuzzy menu — discoverability solved, no memorizing 1–6.</Note>
          <Note tag="Inline progress">Spawning shows a live progress block <b>wherever you are</b> — never hidden on another tab.</Note>
          <Note tag="Scrollback">Results stay in the stream. Errors are full blocks you scroll back to — the 50-char truncation is gone.</Note>
          <Note tag="Verbs">Read and write are <b>separate commands</b> (/read, /write) — the accidental-write trap disappears.</Note>
        </div>
      </div>
      <Divider>command results &amp; states</Divider>
      <div className="scrgrid">
        <ScreenCard title="Spawn — flow with inline progress → ready" hand="progress feedback everywhere" label="C"><SpawnFlowC /></ScreenCard>
        <ScreenCard title="Exec — success & error blocks" hand="errors never truncated" label="C"><ExecC /></ScreenCard>
        <ScreenCard title="Files — /read and /write are distinct" hand="no accidental writes" label="C"><FilesC /></ScreenCard>
        <ScreenCard title="Templates — list + spawn from result" label="C"><TemplatesC /></ScreenCard>
        <ScreenCard title="Providers — health inline" label="C"><ProvidersC /></ScreenCard>
        <ScreenCard title="Logs — filters live in the command" label="C"><LogsC /></ScreenCard>
        <ScreenCard title="Config — type a key or pick chips" label="C"><ConfigC /></ScreenCard>
      </div>
    </div>
  );
}

window.DirectionC = DirectionC;
