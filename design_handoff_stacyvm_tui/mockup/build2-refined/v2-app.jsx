// ====================================================================
// v2-app.jsx — shell for the refined build
// ====================================================================
const V2_TABS = [
  ["dashboard", "DASHBOARD"],
  ["workspace", "SANDBOX WORKSPACE"],
  ["spawn", "SPAWN ANIM"],
  ["boot", "BOOT SPLASH"],
];

function V2App() {
  const [tab, setTab] = useState(() => location.hash.replace("#", "") || "dashboard");
  const [anno, setAnno] = useState(true);
  const [crt, setCrt] = useState(false);

  useEffect(() => {
    document.body.classList.toggle("noanno", !anno);
    document.body.classList.toggle("crt", crt);
  }, [anno, crt]);
  useEffect(() => { location.hash = tab; window.scrollTo({ top: 0 }); }, [tab]);
  useEffect(() => {
    const onKey = (e) => {
      if (e.target.tagName === "INPUT" || tab === "workspace") return; // workspace owns Tab/i/esc
      const n = parseInt(e.key, 10);
      if (n >= 1 && n <= V2_TABS.length) setTab(V2_TABS[n - 1][0]);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [tab]);

  const intro = {
    dashboard: { k: "View · Mission Control + B", h: "The merged dashboard", d: <>A's chrome — telemetry ribbon, number tabs, ⌘K — wrapped around <b>B's informative KPI strip</b>: four bracketed tiles (sandboxes / templates / providers / uptime) with live sparklines, sitting above the fleet table. Sparklines and the clock tick live. <b>Click any row</b> (or the ↵ hint) to open its workspace.</> },
    workspace: { k: "View · deep sandbox interaction", h: "Sandbox Workspace", d: <>One screen to live inside a sandbox: a <b>lazytree</b> file browser (git status flags), a <b>vim</b> editor with a real modeline, and an <b>in-VM terminal</b>. <b>Press <span style={{color:"var(--orange)"}}>Tab</span></b> to cycle which pane is focused (or click 1/2/3); press <b>i</b> in the editor for INSERT, <b>Esc</b> to leave.</> },
    spawn: { k: "View · animated", h: "Spawn sequence", d: <>The live provisioning animation: <b>queue → pull layers → boot rootfs → network → ready</b>, with a filling progress bar and a checked-off timeline. Hit <b>↻ Replay</b> to watch again. This same progress surfaces in the ribbon, so it's never hidden on another tab.</> },
    boot: { k: "View · animated", h: "Boot splash", d: <>The iconic open: the mark springs in, the wordmark and tagline fade up, then the connect bar fills to <span style={{color:"var(--green)"}}>ready</span>. <b>↻ Replay</b> to see it again.</> },
  }[tab];

  return (
    <div>
      <div className="topbar">
        <div className="brandwrap">
          <LogoArt size="header" style={{ fontSize: 6, lineHeight: "6px" }} />
          <div className="brandtxt"><b>STACYVM</b><span>TUI · REFINED — MISSION CONTROL</span></div>
        </div>
        <div className="topspacer" />
        <div className="toggles">
          <button className={"tg" + (anno ? " on" : "")} onClick={() => setAnno((v) => !v)}><span className="led" /> ANNOTATIONS</button>
          <button className={"tg" + (crt ? " on" : "")} onClick={() => setCrt((v) => !v)}><span className="led" /> SCANLINES</button>
        </div>
      </div>

      <div className="seg">
        {V2_TABS.map((t, i) => (
          <button key={t[0]} className={tab === t[0] ? "active" : ""} onClick={() => setTab(t[0])}>
            <span className="num">{i + 1}</span>{t[1]}
          </button>
        ))}
      </div>

      <div className="wrap">
        <div className="secthead">
          <div className="kicker">{intro.k}</div>
          <h1>{intro.h}</h1>
          <div className="lede">{intro.d}</div>
        </div>

        {tab === "dashboard" && (
          <div className="herorow">
            <Dashboard onOpenSandbox={() => setTab("workspace")} onSpawn={() => setTab("spawn")} />
            <div className="notes">
              <Note tag="From B">The four bracketed KPI tiles up top — the "informative" header you liked, now with live sparklines.</Note>
              <Note tag="From A">Telemetry ribbon, ⌘K, numbered tabs — the cockpit chrome on every screen.</Note>
              <Note tag="Live">CPU/MEM/LOAD sparklines + the clock actually tick. In Bubble Tea this is a <b>tea.Tick</b> loop.</Note>
              <Note tag="Try it">Click a sandbox row → it opens the Workspace.</Note>
            </div>
          </div>
        )}
        {tab === "workspace" && (
          <div className="herorow">
            <Workspace />
            <div className="notes">
              <Note tag="lazytree">Left pane is a collapsible tree with git flags (<span style={{color:"var(--orange)"}}>M</span>/<span style={{color:"var(--green)"}}>A</span>). j/k to move, ↵ to open.</Note>
              <Note tag="vim">Center is a real modal editor — NORMAL/INSERT modeline, line numbers, syntax. Press <b>i</b> then <b>Esc</b>.</Note>
              <Note tag="terminal">Right pane is a shell <b>inside the VM</b> — run anything, output stays in scrollback (no truncation).</Note>
              <Note tag="focus">⇥ cycles panes; the focused one gets the orange bracket glow. Click 1/2/3 too.</Note>
            </div>
          </div>
        )}
        {tab === "spawn" && (
          <div className="herorow">
            <SpawnSequence />
            <div className="notes">
              <Note tag="State machine">Five phases drive both the progress bar and the timeline checkmarks — easy to map to your spawn lifecycle events.</Note>
              <Note tag="harmonica">Use a spring for the bar fill &amp; the READY pop so it feels alive, not linear.</Note>
              <Note tag="Global">Because progress also lives in the ribbon, you can navigate away and still see it finish.</Note>
            </div>
          </div>
        )}
        {tab === "boot" && (
          <div className="herorow">
            <BootSplash />
            <div className="notes">
              <Note tag="Mark">Rendered from your PNG as half-block chars — pure text, tints orange, identical on every OS.</Note>
              <Note tag="Motion">Spring the mark in (translate + blur), stagger the wordmark/tagline, then fill the connect bar.</Note>
            </div>
          </div>
        )}

        <div className="footer">StacyVM TUI · refined · A + B dashboard · sandbox workspace · animated spawn &amp; boot · press 1–4</div>
      </div>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<V2App />);
