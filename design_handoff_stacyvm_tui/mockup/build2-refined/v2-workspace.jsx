// ====================================================================
// v2-workspace.jsx — Sandbox Workspace: lazytree + vim + in-VM terminal
// Three focusable panes. Tab cycles focus; the focused pane glows.
// ====================================================================

const TREE = [
  { d: 0, n: "workspace", t: "dir", open: true, g: null },
  { d: 1, n: "src", t: "dir", open: true, g: null },
  { d: 2, n: "main.py", t: "file", g: "m", sel: true },
  { d: 2, n: "utils.py", t: "file", g: null },
  { d: 2, n: "model.py", t: "file", g: "a" },
  { d: 1, n: "tests", t: "dir", open: false, g: null },
  { d: 1, n: "data", t: "dir", open: false, g: null },
  { d: 1, n: "requirements.txt", t: "file", g: "m" },
  { d: 1, n: "run.sh", t: "file", g: "a" },
  { d: 1, n: "README.md", t: "file", g: null },
];

const CODE = [
  ["import", " os, sys", "kw"],
  ["from", " pathlib ", "kw"],
  ["", "", "blank"],
  ["def", " main():", "kw", "fn", "main"],
  ["body", '    data = load("/data/in.csv")', "str"],
  ["body", "    result = transform(data)", "plain"],
  ["body", "    print(result.head())", "plain"],
  ["", "", "blank"],
  ["cm", "# entrypoint", "cm"],
  ["if", '__name__ == "__main__":', "kw"],
  ["body", "    main()", "plain"],
];

function TreePane({ focused }) {
  const gi = { m: ["M", "m"], a: ["A", "a"], d: ["D", "d"] };
  return (
    <BFrame title="◧ FILES · lazytree" right={focused ? "FOCUS" : ""} accent={focused} style={{ minWidth: 188 }}>
      <div className="tree">
        {TREE.map((r, i) => {
          const pad = r.d * 13;
          const icon = r.t === "dir" ? (r.open ? "▾" : "▸") : "·";
          return (
            <div key={i} className={"trow" + (r.sel ? " sel" : "")} style={{ paddingLeft: 6 + pad }}>
              <span className="gi" style={{ color: r.t === "dir" ? "var(--orange)" : "var(--faint)" }}>{icon}</span>
              <span className="nm" style={{ color: r.t === "dir" ? "var(--ink)" : undefined }}>{r.n}</span>
              {r.g && <span className={"gi " + r.g}>{gi[r.g][0]}</span>}
            </div>
          );
        })}
      </div>
      <div className="keyrow" style={{ marginTop: 10, fontSize: 10 }}>
        <span><Key>j</Key><Key>k</Key> move</span><span><Key>↵</Key> open</span><span><Key>a</Key> new</span>
      </div>
    </BFrame>
  );
}

function VimPane({ focused, mode }) {
  return (
    <BFrame title="◨ /workspace/src/main.py" right={mode === "INSERT" ? "INSERT" : "NORMAL"} accent={focused} style={{ flex: 1, minWidth: 0 }}>
      <div className="vim">
        {CODE.map((l, i) => {
          const cl = i === 5;
          if (l[2] === "blank") return <div key={i} className="vline"><span className="g">{i + 1}</span><span> </span></div>;
          let body;
          if (l[2] === "kw" && l[3] === "fn") body = <span><span className="kw">{l[0]}</span> <span className="fn">main</span><span className="plain">():</span></span>;
          else if (l[2] === "kw") body = <span><span className="kw">{l[0]}</span><span className="plain">{l[1]}</span></span>;
          else if (l[2] === "str") body = <span className="plain">    data = <span className="fn">load</span>(<span className="str">"/data/in.csv"</span>)</span>;
          else if (l[2] === "cm") body = <span className="cm">{l[1]}</span>;
          else body = <span className="plain">{l[1]}</span>;
          return (
            <div key={i} className={"vline" + (cl ? " cl" : "")}>
              <span className="g">{i + 1}</span>
              <span>{body}{cl && mode === "INSERT" && <Cur />}</span>
            </div>
          );
        })}
      </div>
      <div className="modeline">
        <span className={"vmode" + (mode === "INSERT" ? " insert" : "")}>{mode === "INSERT" ? "-- INSERT --" : "NORMAL"}</span>
        <span className="dim">main.py</span>
        <span className="faint">utf-8 · py · unix</span>
        <span style={{ flex: 1 }} />
        <span className="dim">6:18</span>
        <span className="faint">{mode === "INSERT" ? "esc → normal" : "i insert · :w save · :q close"}</span>
      </div>
    </BFrame>
  );
}

function TermPane({ focused }) {
  return (
    <BFrame title="◰ TERMINAL · sb-7f3a91 (in-VM)" right={focused ? "FOCUS" : ""} accent={focused}>
      <div className="term">
        <div><span className="ph">stacy</span><span className="pp">@</span><span className="ps">sb-7f3a91</span><span className="pp">:</span><span className="steel">/workspace</span><span className="pp">$</span> <span className="ink">python -m pytest -q</span></div>
        <div className="out">........................ <Ok>[100%]</Ok></div>
        <div className="out">24 passed in 1.83s</div>
        <div><span className="ph">stacy</span><span className="pp">@</span><span className="ps">sb-7f3a91</span><span className="pp">:</span><span className="steel">/workspace</span><span className="pp">$</span> <span className="ink">./run.sh --epochs 3</span></div>
        <div className="out">epoch 1/3 loss=0.412  epoch 2/3 loss=0.281  epoch 3/3 loss=0.196</div>
        <div className="out"><Ok>✓</Ok> saved model.pkl <span className="faint">(2.4MB)</span></div>
        <div><span className="ph">stacy</span><span className="pp">@</span><span className="ps">sb-7f3a91</span><span className="pp">:</span><span className="steel">/workspace</span><span className="pp">$</span> <span className="ink">{focused ? "" : "ls -la"}</span>{focused && <Cur block />}</div>
      </div>
    </BFrame>
  );
}

// the live, interactive workspace
function Workspace({ embedded }) {
  const [focus, setFocus] = useState("tree"); // tree | editor | term
  const [mode, setMode] = useState("NORMAL");
  useEffect(() => {
    if (embedded) return;
    const onKey = (e) => {
      if (e.key === "Tab") { e.preventDefault(); setFocus((f) => f === "tree" ? "editor" : f === "editor" ? "term" : "tree"); }
      if (e.key === "i" && focus === "editor") setMode("INSERT");
      if (e.key === "Escape") setMode("NORMAL");
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [focus, embedded]);

  return (
    <Screen label="stacyvm › sandbox › sb-7f3a91 › workspace" right="docker · runsc">
      <Ribbon />
      <Nav active="SANDBOXES" />
      {/* context bar for the sandbox */}
      <div className="ctxbar">
        <span className="hi b">▣ sb-7f3a91</span>
        <span className="chip"><Ok>●</Ok> running</span>
        <span className="chip">python:3.12-alpine</span>
        <span className="chip">via docker<span className="faint">(runsc)</span></span>
        <span className="chip">ttl <Meter val={80} width={7} color="hi" showPct={false} /> <span className="dim">06m</span></span>
        <span style={{ flex: 1 }} />
        <span className="keyrow" style={{ fontSize: 10.5 }}>
          <span style={{ cursor: "pointer", color: focus === "tree" ? "var(--orange)" : undefined }} onClick={() => setFocus("tree")}><Key>1</Key> files</span>
          <span style={{ cursor: "pointer", color: focus === "editor" ? "var(--orange)" : undefined }} onClick={() => setFocus("editor")}><Key>2</Key> editor</span>
          <span style={{ cursor: "pointer", color: focus === "term" ? "var(--orange)" : undefined }} onClick={() => setFocus("term")}><Key>3</Key> terminal</span>
          <span><Key>⇥</Key> cycle</span>
        </span>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "auto 1fr", gridTemplateRows: "auto auto", gap: 11 }}>
        <div style={{ gridRow: "1 / 3" }}><TreePane focused={focus === "tree"} /></div>
        <VimPane focused={focus === "editor"} mode={mode} />
        <TermPane focused={focus === "term"} />
      </div>

      <div style={{ display: "flex", alignItems: "center", marginTop: 12, paddingTop: 9, borderTop: "1px solid var(--line)", fontSize: 11 }}>
        <span className="faint">workspace · 3 panes · {focus} focused</span>
        <span style={{ flex: 1 }} />
        <KeyHint items={[["⇥", "cycle pane"], ["i", "insert"], ["esc", "normal"], ["⌘K", "command"]]} />
      </div>
    </Screen>
  );
}

Object.assign(window, { Workspace, TreePane, VimPane, TermPane });
