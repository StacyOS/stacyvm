// ====================================================================
// v2-dashboard.jsx — A's chrome + B's informative KPI tiles, live.
// ====================================================================

function Ribbon() {
  const cpu = useDrift(3), mem = useDrift(7, 12, 4, 8), load = useDrift(5);
  const [clock, setClock] = useState("17:38:05");
  useEffect(() => {
    const id = setInterval(() => {
      const d = new Date();
      setClock(d.toTimeString().slice(0, 8));
    }, 1000);
    return () => clearInterval(id);
  }, []);
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 18, paddingBottom: 11, borderBottom: "1px solid var(--line)", marginBottom: 12 }}>
      <span style={{ display: "flex", alignItems: "center", gap: 9 }}>
        <LogoArt size="header" style={{ fontSize: 5, lineHeight: "5px" }} />
        <b style={{ letterSpacing: 2, fontSize: 12 }}>STACYVM</b>
        <span className="faint" style={{ fontSize: 9, letterSpacing: 2 }}>MISSION&nbsp;CONTROL</span>
      </span>
      <span style={{ flex: 1 }} />
      <span className="dim" style={{ fontSize: 11 }}>CPU <Spark data={cpu} /> <span className="hi">34%</span></span>
      <span className="dim" style={{ fontSize: 11 }}>MEM <Spark data={mem} /> <span className="hi">61%</span></span>
      <span className="dim" style={{ fontSize: 11 }}>LOAD <Spark data={load} color="ok" /></span>
      <span className="dim" style={{ fontSize: 11 }}>{clock}</span>
      <span className="ok" style={{ fontSize: 11 }}>● ONLINE <span className="faint">v0.9.2</span></span>
    </div>
  );
}

function Nav({ active = "DASH", onNav }) {
  const items = ["DASH", "SANDBOXES", "TEMPLATES", "PROVIDERS", "LOGS", "CONFIG"];
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 0, marginBottom: 14, fontSize: 10.5, letterSpacing: 1.5 }}>
      {items.map((it, i) => (
        <span key={it} onClick={() => onNav && onNav(it)} style={{
          padding: "5px 12px", cursor: onNav ? "pointer" : "default",
          color: it === active ? "var(--orange)" : "var(--dim)",
          borderBottom: it === active ? "2px solid var(--orange)" : "2px solid transparent",
          background: it === active ? "rgba(255,166,12,0.07)" : "transparent",
        }}><span className="faint">{i + 1} </span>{it}</span>
      ))}
      <span style={{ flex: 1 }} />
      <span className="dim" style={{ fontSize: 10.5 }}><Key>⌘</Key><Key>K</Key> <span className="faint">command</span></span>
    </div>
  );
}

// KPI tile (B's bracket frame + metric + spark)
function Kpi({ title, val, color, seed, accent, note }) {
  const d = useDrift(seed);
  return (
    <BFrame title={title} accent={accent}>
      <div style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between" }}>
        <span className={"metric " + (color || "")}>{val}</span>
        <Spark data={d} color={color === "ok" ? "ok" : "hi"} />
      </div>
      {note && <div className="faint" style={{ fontSize: 9.5, marginTop: 6, letterSpacing: .5 }}>{note}</div>}
    </BFrame>
  );
}

function Dashboard({ onOpenSandbox, onSpawn }) {
  const boxes = [
    ["sb-7f3a91", "run", "docker", "python:3.12", 71, "24m"],
    ["sb-2c8e04", "run", "docker", "node:20", 38, "12m"],
    ["sb-9d11ba", "creating", "firecracker", "alpine:latest", 0, "30m"],
    ["sb-4a77e2", "run", "proot", "ubuntu:22.04", 12, "08m"],
    ["sb-1e90cf", "idle", "docker", "rust:1.78", 3, "55m"],
  ];
  const cpu = useDrift(3), mem = useDrift(7, 12, 4, 8), net = useDrift(9), disk = useDrift(4);
  return (
    <Screen label="stacyvm tui — dashboard" right="alt-screen · 198×52">
      <Ribbon />
      <Nav active="DASH" onNav={(t) => t === "SANDBOXES" && onOpenSandbox && onOpenSandbox()} />

      {/* B's KPI strip — the "informative" header */}
      <div className="grid4" style={{ marginBottom: 12 }}>
        <Kpi title="SANDBOXES" val="5" color="ok" seed={2} accent note="4 running · 1 booting" />
        <Kpi title="TEMPLATES" val="4" color="hi" seed={5} note="3 warm in pool" />
        <Kpi title="PROVIDERS" val="2/3" color="ok" seed={1} note="docker · firecracker" />
        <Kpi title="UPTIME" val="4d" color="" seed={6} note="since 17:14 · 312 spawns" />
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1.55fr 1fr", gap: 11 }}>
        <Mod title="ACTIVE SANDBOXES" right="↵ OPEN WORKSPACE" accent>
          <table className="tbl">
            <thead><tr><th>ID</th><th>STATE</th><th>PROVIDER</th><th>IMAGE</th><th>CPU</th><th>TTL</th></tr></thead>
            <tbody>
              {boxes.map((b, i) => (
                <tr key={b[0]} className={i === 0 ? "sel" : ""} onClick={() => onOpenSandbox && onOpenSandbox(b[0])} style={{ cursor: "pointer" }}>
                  <td className="b">{b[0]}</td>
                  <td><span className={b[1] === "creating" ? "hi" : b[1] === "idle" ? "dim" : "ok"}>{b[1] === "creating" ? "◐" : b[1] === "idle" ? "○" : "●"} {b[1]}</span></td>
                  <td className="dim">{b[2]}</td>
                  <td className="dim">{b[3]}</td>
                  <td>{b[1] === "creating" ? <span className="faint">—</span> : <Meter val={b[4]} width={6} showPct={false} color={b[4] > 60 ? "hi" : "ok"} />}</td>
                  <td className="dim">{b[5]}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <div style={{ marginTop: 11, display: "flex", gap: 16 }} className="keyrow">
            <span style={{ cursor: "pointer" }} onClick={onSpawn}><Key>s</Key> spawn</span>
            <span><Key>e</Key> exec</span><span><Key>f</Key> files</span><span><Key>d</Key> kill</span>
            <span style={{ cursor: "pointer" }} onClick={() => onOpenSandbox && onOpenSandbox()}><Key>↵</Key> open workspace</span>
          </div>
        </Mod>

        <div style={{ display: "grid", gap: 11, gridTemplateRows: "auto auto 1fr" }}>
          <Mod title="HOST TELEMETRY" right="3s">
            <div style={{ display: "grid", gap: 6, fontSize: 11.5 }}>
              <div className="dim">CPU&nbsp;&nbsp;<Meter val={34} width={12} /></div>
              <div className="dim">MEM&nbsp;&nbsp;<Meter val={61} width={12} /></div>
              <div className="dim">DISK&nbsp;<Meter val={22} width={12} color="ok" /></div>
              <div className="dim" style={{ marginTop: 2 }}>NET&nbsp;&nbsp;<Spark data={net} /> <span className="ok">1.2MB/s</span></div>
            </div>
          </Mod>
          <Mod title="PROVIDERS">
            <div style={{ fontSize: 11.5, lineHeight: 1.85 }}>
              <div><Ok>●</Ok> docker <span className="faint">· default · runsc · 12ms</span></div>
              <div><Ok>●</Ok> firecracker <span className="faint">· kvm ok · 8ms</span></div>
              <div><span className="dim">○</span> proot <span className="faint">· standby</span></div>
            </div>
          </Mod>
          <Mod title="EVENT STREAM" right="◉ live">
            <div style={{ fontSize: 10.5, lineHeight: 1.7 }}>
              <div><span className="faint">17:38:04</span> <Hi>SPAWN</Hi> sb-9d11ba alpine</div>
              <div><span className="faint">17:38:02</span> <Ok>EXEC</Ok> sb-2c8e04 exit 0</div>
              <div><span className="faint">17:37:51</span> <span className="steel">WRITE</span> /workspace/main.py</div>
              <div><span className="faint">17:37:30</span> <span className="mint">TEMPLATE</span> data-science</div>
            </div>
          </Mod>
        </div>
      </div>

      <div style={{ display: "flex", alignItems: "center", marginTop: 14, paddingTop: 9, borderTop: "1px solid var(--line)", fontSize: 11 }}>
        <span className="faint">5 sandboxes · 4 templates · 2 providers</span>
        <span style={{ flex: 1 }} />
        <KeyHint items={[["⌘K", "command"], ["?", "help"], ["q", "quit"]]} />
        <span className="ok" style={{ marginLeft: 14 }}>✓ sb-9d11ba ready</span>
      </div>
    </Screen>
  );
}

Object.assign(window, { Dashboard, Ribbon, Nav });
