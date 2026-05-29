// ====================================================================
// direction-a.jsx — "MISSION CONTROL"
// Top telemetry ribbon + command-palette nav (sidebar removed).
// ====================================================================

// shared chrome ------------------------------------------------------
function RibbonA({ clock = "17:38:05" }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 18, paddingBottom: 11, borderBottom: "1px solid var(--line)", marginBottom: 12 }}>
      <span style={{ display: "flex", alignItems: "center", gap: 9 }}>
        <LogoArt size="header" style={{ fontSize: 5, lineHeight: "5px" }} />
        <b style={{ letterSpacing: 2, fontSize: 12 }}>STACYVM</b>
        <span className="faint" style={{ fontSize: 9, letterSpacing: 2 }}>MISSION&nbsp;CONTROL</span>
      </span>
      <span style={{ flex: 1 }} />
      <span className="dim" style={{ fontSize: 11 }}>CPU <Spark data={[2,3,4,3,5,4,6,5]} /> <span className="hi">34%</span></span>
      <span className="dim" style={{ fontSize: 11 }}>MEM <Spark data={[4,4,5,6,6,7,6,7]} color="hi" /> <span className="hi">61%</span></span>
      <span className="dim" style={{ fontSize: 11 }}>{clock}</span>
      <span className="ok" style={{ fontSize: 11 }}>● ONLINE <span className="faint">v0.9.2</span></span>
    </div>
  );
}

function NavA({ active = "DASH" }) {
  const items = ["DASH", "SANDBOXES", "TEMPLATES", "PROVIDERS", "LOGS", "CONFIG"];
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 0, marginBottom: 14, fontSize: 10.5, letterSpacing: 1.5 }}>
      {items.map((it, i) => (
        <span key={it} style={{
          padding: "5px 12px", color: it === active ? "var(--orange)" : "var(--dim)",
          borderBottom: it === active ? "2px solid var(--orange)" : "2px solid transparent",
          background: it === active ? "rgba(255,166,12,0.07)" : "transparent",
        }}>
          <span className="faint">{i + 1} </span>{it}
        </span>
      ))}
      <span style={{ flex: 1 }} />
      <span className="dim" style={{ fontSize: 10.5 }}><Key>⌘</Key><Key>K</Key> <span className="faint">command</span></span>
    </div>
  );
}

function StatusLineA({ left, hints, msg, msgKind }) {
  return (
    <div style={{ display: "flex", alignItems: "center", marginTop: 14, paddingTop: 9, borderTop: "1px solid var(--line)", fontSize: 11 }}>
      <span className="faint">{left}</span>
      <span style={{ flex: 1 }} />
      {hints && <KeyHint items={hints} />}
      {msg && <span className={msgKind || "ok"} style={{ marginLeft: 14 }}>{msg}</span>}
    </div>
  );
}

// DASHBOARD (hero) ---------------------------------------------------
function DashA() {
  const boxes = [
    ["sb-7f3a91", "run", "docker", "python:3.12", 71, "24m"],
    ["sb-2c8e04", "run", "docker", "node:20", 38, "12m"],
    ["sb-9d11ba", "creating", "firecracker", "alpine:latest", 0, "30m"],
    ["sb-4a77e2", "run", "proot", "ubuntu:22.04", 12, "08m"],
    ["sb-1e90cf", "idle", "docker", "rust:1.78", 3, "55m"],
  ];
  return (
    <Screen label="stacyvm tui — dashboard" right="alt-screen · 198×52">
      <RibbonA />
      <NavA active="DASH" />
      <div style={{ display: "grid", gridTemplateColumns: "1.55fr 1fr", gap: 11 }}>
        {/* left: fleet */}
        <Mod title="ACTIVE SANDBOXES" right="5 RUNNING · 1 CREATING" accent>
          <table className="tbl">
            <thead><tr><th>ID</th><th>STATE</th><th>PROVIDER</th><th>IMAGE</th><th>CPU</th><th>TTL</th></tr></thead>
            <tbody>
              {boxes.map((b, i) => (
                <tr key={b[0]} className={i === 0 ? "sel" : ""}>
                  <td className="b">{b[0]}</td>
                  <td><Dot state={b[1]} /> <span className={b[1] === "creating" ? "hi" : b[1] === "idle" ? "dim" : "ok"}>{b[1]}</span></td>
                  <td className="dim">{b[2]}</td>
                  <td className="dim">{b[3]}</td>
                  <td><Meter val={b[4]} width={6} showPct={false} color={b[4] > 60 ? "hi" : "ok"} /></td>
                  <td className="dim">{b[5]}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <div style={{ marginTop: 11, display: "flex", gap: 16 }} className="keyrow">
            <span><Key>s</Key> spawn</span><span><Key>e</Key> exec</span><span><Key>f</Key> files</span><span><Key>d</Key> kill</span><span><Key>↵</Key> inspect</span>
          </div>
        </Mod>
        {/* right column */}
        <div style={{ display: "grid", gap: 11, gridTemplateRows: "auto auto 1fr" }}>
          <Mod title="FLEET TELEMETRY" right="3s">
            <div style={{ display: "grid", gap: 6, fontSize: 11.5 }}>
              <div className="dim">CPU&nbsp;&nbsp;<Meter val={34} width={12} /></div>
              <div className="dim">MEM&nbsp;&nbsp;<Meter val={61} width={12} /></div>
              <div className="dim">DISK&nbsp;<Meter val={22} width={12} color="ok" /></div>
              <div className="dim" style={{ marginTop: 3 }}>LOAD <Spark data={[3,5,4,6,5,7,6,5,4,6,7,8]} /> <span className="faint">5m</span></div>
            </div>
          </Mod>
          <Mod title="PROVIDERS">
            <div style={{ fontSize: 11.5, lineHeight: 1.9 }}>
              <div><Ok>●</Ok> docker <span className="faint">· default · runsc</span></div>
              <div><Ok>●</Ok> firecracker <span className="faint">· kvm ok</span></div>
              <div><span className="dim">○</span> proot <span className="faint">· standby</span></div>
            </div>
          </Mod>
          <Mod title="EVENT STREAM" right="live">
            <div style={{ fontSize: 10.5, lineHeight: 1.75 }}>
              <div><span className="faint">17:38:04</span> <Hi>SPAWN</Hi> sb-7f3a91 docker</div>
              <div><span className="faint">17:38:02</span> <Ok>EXEC</Ok> sb-2c8e04 exit 0</div>
              <div><span className="faint">17:37:51</span> <span className="steel">WRITE</span> /workspace/main.py</div>
              <div><span className="faint">17:37:30</span> <span className="mint">TEMPLATE</span> data-science</div>
            </div>
          </Mod>
        </div>
      </div>
      <StatusLineA left="5 sandboxes · 4 templates · 2 providers" msg="✓ sb-7f3a91 ready" msgKind="ok"
        hints={[["⌘K", "command"], ["?", "help"], ["q", "quit"]]} />
    </Screen>
  );
}

// SANDBOXES list + detail drawer ------------------------------------
function SandboxesA() {
  return (
    <Screen label="sandboxes" right="6 total">
      <NavA active="SANDBOXES" />
      <div style={{ display: "grid", gridTemplateColumns: "1.4fr 1fr", gap: 11 }}>
        <Mod title="FLEET" right="filter: running">
          <table className="tbl">
            <thead><tr><th>ID</th><th>STATE</th><th>IMAGE</th><th>TTL</th></tr></thead>
            <tbody>
              <tr className="sel"><td className="b">sb-7f3a91</td><td><Ok>●</Ok> run</td><td className="dim">python:3.12</td><td className="dim">24m</td></tr>
              <tr><td>sb-2c8e04</td><td><Ok>●</Ok> run</td><td className="dim">node:20</td><td className="dim">12m</td></tr>
              <tr><td>sb-9d11ba</td><td><Hi>◐</Hi> create</td><td className="dim">alpine</td><td className="dim">30m</td></tr>
              <tr><td>sb-4a77e2</td><td><Ok>●</Ok> run</td><td className="dim">ubuntu</td><td className="dim">08m</td></tr>
            </tbody>
          </table>
        </Mod>
        <Mod title="◂ sb-7f3a91" right="INSPECT" accent>
          <div style={{ fontSize: 11.5, lineHeight: 1.85 }}>
            <div className="dim">state&nbsp;&nbsp;&nbsp;&nbsp;<Ok>● running</Ok></div>
            <div className="dim">image&nbsp;&nbsp;&nbsp;&nbsp;<span className="ink">python:3.12-alpine</span></div>
            <div className="dim">provider&nbsp;docker <span className="faint">(runsc)</span></div>
            <div className="dim">created&nbsp;&nbsp;17:14:09 <span className="faint">· 24m ago</span></div>
            <div className="dim">expires&nbsp;&nbsp;in 06m <Meter val={80} width={8} color="hi" showPct={false} /></div>
            <div style={{ marginTop: 9 }} className="dim">CPU <Meter val={71} width={10} /></div>
            <div className="dim">MEM <Meter val={48} width={10} color="ok" /></div>
          </div>
          <div style={{ marginTop: 11 }} className="keyrow">
            <span><Key>e</Key> exec</span><span><Key>f</Key> files</span><span><Key>l</Key> logs</span><span><Key>d</Key> kill</span>
          </div>
        </Mod>
      </div>
      <StatusLineA left="6 sandboxes" hints={[["j/k", "move"], ["↵", "inspect"], ["s", "spawn"]]} />
    </Screen>
  );
}

// SPAWN modal (floating) --------------------------------------------
function SpawnA() {
  return (
    <Screen label="sandboxes › spawn">
      <NavA active="SANDBOXES" />
      <div style={{ position: "relative" }}>
        <div style={{ opacity: 0.28, filter: "grayscale(1)" }}>
          <table className="tbl">
            <tbody>
              <tr><td>sb-7f3a91</td><td className="dim">python</td><td className="dim">24m</td></tr>
              <tr><td>sb-2c8e04</td><td className="dim">node</td><td className="dim">12m</td></tr>
              <tr><td>sb-9d11ba</td><td className="dim">alpine</td><td className="dim">30m</td></tr>
            </tbody>
          </table>
        </div>
        <div style={{ position: "absolute", inset: "-6px 12% auto 12%", top: -4 }}>
          <Mod title="◇ SPAWN SANDBOX" accent style={{ background: "var(--panel-2)", boxShadow: "0 24px 50px -16px rgba(0,0,0,.9), inset 0 0 30px rgba(255,166,12,.05)" }}>
            <div style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "10px 14px", fontSize: 12, alignItems: "center" }}>
              <span className="dim">image</span>
              <span style={{ border: "1px solid var(--line-2)", borderLeft: "2px solid var(--orange)", padding: "5px 9px", borderRadius: 2 }}>python:3.12-alpine<span className="hi" style={{ marginLeft: 1 }}>▏</span></span>
              <span className="dim">template</span>
              <span style={{ border: "1px solid var(--line-2)", padding: "5px 9px", borderRadius: 2 }} className="dim">data-science <span className="faint">▾</span></span>
              <span className="dim">ttl</span>
              <span style={{ border: "1px solid var(--line-2)", padding: "5px 9px", borderRadius: 2 }} className="dim">30m</span>
              <span className="dim">provider</span>
              <span className="keyrow"><span style={{ color: "var(--orange)", border: "1px solid var(--orange)", padding: "2px 8px", borderRadius: 2 }}>docker</span><span style={{ padding: "2px 8px" }}>firecracker</span><span style={{ padding: "2px 8px" }}>proot</span></span>
            </div>
            <div style={{ marginTop: 11 }} className="keyrow">
              <span><Key>tab</Key> next field</span><span><Key>↵</Key> spawn</span><span><Key>esc</Key> cancel</span>
            </div>
          </Mod>
        </div>
      </div>
      <div style={{ height: 30 }} />
    </Screen>
  );
}

// EXEC + FILES (split) ----------------------------------------------
function ExecA() {
  return (
    <Screen label="sandboxes › sb-7f3a91 › exec">
      <NavA active="SANDBOXES" />
      <Mod title="◇ EXEC · sb-7f3a91" accent>
        <div style={{ fontSize: 12 }}>
          <span className="ok">$</span> <span className="ink">python -m pytest -q</span>
        </div>
        <div style={{ fontSize: 11.5, lineHeight: 1.7, marginTop: 7 }} className="mint">
          <div>........................ <Ok>[100%]</Ok></div>
          <div className="dim">24 passed in 1.83s</div>
          <div className="faint">─ exit 0 · 1.9s ─</div>
        </div>
      </Mod>
      <div style={{ marginTop: 11 }} className="keyrow"><span><Key>↵</Key> run</span><span><Key>↑</Key> history</span><span><Key>esc</Key> back</span></div>
    </Screen>
  );
}

function FilesA() {
  return (
    <Screen label="sandboxes › sb-7f3a91 › files">
      <NavA active="SANDBOXES" />
      <div style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: 11 }}>
        <Mod title="TREE" style={{ minWidth: 150 }}>
          <div style={{ fontSize: 11.5, lineHeight: 1.85 }}>
            <div className="dim">▸ /workspace</div>
            <div style={{ paddingLeft: 12 }} className="hi">▾ src</div>
            <div style={{ paddingLeft: 24 }}>main.py</div>
            <div style={{ paddingLeft: 24 }} className="dim">utils.py</div>
            <div style={{ paddingLeft: 12 }} className="dim">README.md</div>
          </div>
        </Mod>
        <Mod title="◇ /workspace/src/main.py" right="EDIT" accent>
          <div style={{ fontSize: 11, lineHeight: 1.7 }}>
            <div><span className="faint">1</span> <span className="hi">import</span> os</div>
            <div><span className="faint">2</span> <span className="hi">def</span> <span className="mint">main</span>():</div>
            <div><span className="faint">3</span> &nbsp;&nbsp;print(<span className="ok">"hello"</span>)<span className="hi">▏</span></div>
          </div>
          <div style={{ marginTop: 9 }} className="keyrow">
            <span style={{ color: "var(--orange)" }}>● WRITE mode</span><span><Key>^o</Key> read</span><span><Key>^s</Key> save</span>
          </div>
        </Mod>
      </div>
    </Screen>
  );
}

// TEMPLATES ----------------------------------------------------------
function TemplatesA() {
  return (
    <Screen label="templates" right="4 defined">
      <NavA active="TEMPLATES" />
      <div style={{ display: "grid", gridTemplateColumns: "1.3fr 1fr", gap: 11 }}>
        <Mod title="TEMPLATES">
          <table className="tbl">
            <thead><tr><th>NAME</th><th>IMAGE</th><th>MEM</th><th>CPU</th><th>POOL</th></tr></thead>
            <tbody>
              <tr className="sel"><td className="b">data-science</td><td className="dim">python:3.12</td><td className="dim">512</td><td className="dim">1</td><td className="ok">3</td></tr>
              <tr><td>web-build</td><td className="dim">node:20</td><td className="dim">1024</td><td className="dim">2</td><td className="dim">0</td></tr>
              <tr><td>rust-ci</td><td className="dim">rust:1.78</td><td className="dim">2048</td><td className="dim">4</td><td className="dim">1</td></tr>
            </tbody>
          </table>
        </Mod>
        <Mod title="◂ data-science" accent>
          <div style={{ fontSize: 11.5, lineHeight: 1.7 }} className="dim">
            <div className="ink b">Data Science Stack</div>
            <div style={{ marginTop: 5 }}>Python 3.12 with numpy, pandas, scikit-learn pre-warmed. Pool keeps <Hi>3</Hi> instances ready.</div>
            <div style={{ marginTop: 8 }}>mem <span className="ink">512MB</span> · cpu <span className="ink">1</span> · ttl <span className="ink">300s</span></div>
          </div>
          <div style={{ marginTop: 10 }} className="keyrow"><span><Key>s</Key> spawn</span><span><Key>n</Key> new</span><span><Key>d</Key> delete</span></div>
        </Mod>
      </div>
      <StatusLineA left="4 templates" hints={[["s", "spawn"], ["n", "new"]]} />
    </Screen>
  );
}

// PROVIDERS ----------------------------------------------------------
function ProvidersA() {
  return (
    <Screen label="providers" right="2 healthy">
      <NavA active="PROVIDERS" />
      <div className="grid3">
        <Mod title="DOCKER" right="DEFAULT" accent>
          <div style={{ fontSize: 12, lineHeight: 1.9 }}>
            <div><Ok>● healthy</Ok></div>
            <div className="dim">runtime <span className="ink">runsc</span></div>
            <div className="dim">sandboxes <span className="ink">4</span></div>
            <div className="dim">latency <span className="ok">12ms</span> <Spark data={[2,3,2,4,3,2,3]} color="ok" /></div>
          </div>
        </Mod>
        <Mod title="FIRECRACKER">
          <div style={{ fontSize: 12, lineHeight: 1.9 }}>
            <div><Ok>● healthy</Ok></div>
            <div className="dim">kvm <Ok>/dev/kvm ok</Ok></div>
            <div className="dim">sandboxes <span className="ink">1</span></div>
            <div className="dim">latency <span className="ok">8ms</span> <Spark data={[2,2,3,2,3,2,2]} color="ok" /></div>
          </div>
        </Mod>
        <Mod title="PROOT">
          <div style={{ fontSize: 12, lineHeight: 1.9 }}>
            <div className="dim">○ standby</div>
            <div className="dim">mode <span className="ink">userspace</span></div>
            <div className="dim">sandboxes <span className="ink">0</span></div>
            <div className="faint">no kvm required</div>
          </div>
        </Mod>
      </div>
      <StatusLineA left="2 providers healthy" hints={[["r", "refresh"], ["↵", "set default"]]} />
    </Screen>
  );
}

// LOGS ---------------------------------------------------------------
function LogsA() {
  const rows = [
    ["17:38:04", "SPAWN", "hi", "sb-7f3a91 docker python:3.12 ttl=30m"],
    ["17:38:02", "EXEC", "ok", "sb-2c8e04 'pytest -q' exit=0 1.9s"],
    ["17:37:51", "WRITE", "steel", "sb-7f3a91 /workspace/src/main.py 412B"],
    ["17:37:30", "TEMPLATE", "mint", "created data-science pool=3"],
    ["17:36:58", "KILL", "err", "sb-0a31ff reason=ttl-expired"],
    ["17:36:12", "CONFIG", "dim", "providers.docker.runtime runc→runsc"],
  ];
  return (
    <Screen label="logs" right="filter: all · 100 cap">
      <NavA active="LOGS" />
      <Mod title="EVENT STREAM" right="◉ following">
        <div style={{ fontSize: 11.5, lineHeight: 1.85 }}>
          {rows.map((r, i) => (
            <div key={i}><span className="faint">{r[0]}</span> <span className={r[2] + " b"} style={{ display: "inline-block", width: 78 }}>{r[1]}</span> <span className="dim">{r[3]}</span></div>
          ))}
        </div>
      </Mod>
      <StatusLineA left="6 events" hints={[["/", "filter"], ["g", "jump kind"], ["c", "copy"]]} />
    </Screen>
  );
}

// CONFIG -------------------------------------------------------------
function ConfigA() {
  return (
    <Screen label="config" right="live patch">
      <NavA active="CONFIG" />
      <div className="grid2">
        <Mod title="◇ PROVIDERS" accent>
          <div style={{ fontSize: 12, lineHeight: 2 }}>
            <div className="dim">default provider</div>
            <div className="keyrow" style={{ gap: 6 }}>
              <span style={{ color: "var(--orange)", border: "1px solid var(--orange)", padding: "2px 9px", borderRadius: 2 }}>docker</span>
              <span style={{ padding: "2px 9px" }}>firecracker</span>
              <span style={{ padding: "2px 9px" }}>proot</span>
            </div>
            <div className="dim" style={{ marginTop: 8 }}>docker runtime</div>
            <div className="keyrow" style={{ gap: 6 }}>
              <span style={{ padding: "2px 9px" }}>runc</span>
              <span style={{ color: "var(--orange)", border: "1px solid var(--orange)", padding: "2px 9px", borderRadius: 2 }}>runsc</span>
              <span style={{ padding: "2px 9px" }}>kata</span>
            </div>
          </div>
        </Mod>
        <Mod title="SERVER">
          <div style={{ fontSize: 12, lineHeight: 2 }} className="dim">
            <div>address <span className="ink">localhost:7423</span></div>
            <div>log format <span className="ink">pretty</span></div>
            <div>preview domain <span className="ink">*.stacy.dev</span></div>
            <div className="faint" style={{ marginTop: 6 }}>↵ edit any key · changes patch live</div>
          </div>
        </Mod>
      </div>
      <StatusLineA left="config" msg="✓ patched providers.docker.runtime=runsc" hints={[["space", "apply"]]} />
    </Screen>
  );
}

function DirectionA() {
  return (
    <div>
      <div className="secthead">
        <div className="kicker">Direction A</div>
        <h1>Mission Control <span className="hand">— the cockpit</span></h1>
        <div className="lede">The sidebar is gone. A <b>full-width telemetry ribbon</b> pins fleet vitals to the top of every screen, navigation collapses into a <b>⌘K command palette</b> + number tabs, and every tab is a grid of live <b>HUD modules</b>. Built for an operator watching many sandboxes at once.</div>
      </div>
      <div className="moves">
        <Move t="Nav model">⌘K palette is primary; 1–6 number tabs always visible in the bar (fixes the hidden-hint problem).</Move>
        <Move t="Signature">Persistent telemetry ribbon: CPU / MEM / load sparklines + ONLINE badge, on every screen.</Move>
        <Move t="Density">Information-dense, multi-panel. Closest to NASA/Grafana energy.</Move>
      </div>
      <div className="herorow">
        <DashA />
        <div className="notes">
          <Note tag="Ribbon">Fleet vitals live at the top <b>everywhere</b> — spawn progress & health are never off-screen (fixes "progress only on Sandboxes tab").</Note>
          <Note tag="Nav">Number tabs are visible <b>and</b> ⌘K opens a fuzzy command palette. No more guessing that 1–6 jump tabs.</Note>
          <Note tag="Modules">Each panel is a self-contained HUD card with its own title + readout. Easy to rearrange in lipgloss.</Note>
          <Note tag="Status">Success/error shows as a colored chip with no truncation — full history lives in Logs.</Note>
        </div>
      </div>
      <Divider>secondary screens &amp; modes</Divider>
      <div className="scrgrid">
        <ScreenCard title="Sandboxes — list + inspect drawer" hand="↵ opens a detail panel instead of a dead row" label="A">
          <SandboxesA />
        </ScreenCard>
        <ScreenCard title="Spawn — floating modal over the fleet" hand="real overlay, not inline text" label="A">
          <SpawnA />
        </ScreenCard>
        <ScreenCard title="Exec — command + framed output" hand="exit code & duration always shown" label="A">
          <ExecA />
        </ScreenCard>
        <ScreenCard title="Files — explicit READ / WRITE mode" hand="no more accidental writes!" label="A">
          <FilesA />
        </ScreenCard>
        <ScreenCard title="Templates — table + markdown detail" label="A"><TemplatesA /></ScreenCard>
        <ScreenCard title="Providers — health cards w/ latency" label="A"><ProvidersA /></ScreenCard>
        <ScreenCard title="Logs — filterable event stream" hand="colored kinds, /-filter, copy mode" label="A"><LogsA /></ScreenCard>
        <ScreenCard title="Config — segmented controls, live patch" label="A"><ConfigA /></ScreenCard>
      </div>
    </div>
  );
}

window.DirectionA = DirectionA;
