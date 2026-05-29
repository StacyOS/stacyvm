// ====================================================================
// app.jsx — shell: top bar, toggles, direction tabs, routing
// ====================================================================
const TABS = [
  ["overview", "OVERVIEW", null],
  ["a", "MISSION CONTROL", "A"],
  ["b", "HUD GRID", "B"],
  ["c", "STREAM", "C"],
  ["logo", "LOGO ART", null],
  ["fixes", "UX FIXES", null],
];

function App() {
  const [tab, setTab] = useState(() => location.hash.replace("#", "") || "overview");
  const [anno, setAnno] = useState(true);
  const [crt, setCrt] = useState(false);

  useEffect(() => {
    document.body.classList.toggle("noanno", !anno);
    document.body.classList.toggle("crt", crt);
  }, [anno, crt]);

  useEffect(() => {
    location.hash = tab;
    window.scrollTo({ top: 0 });
  }, [tab]);

  useEffect(() => {
    const onKey = (e) => {
      if (e.target.tagName === "INPUT") return;
      const n = parseInt(e.key, 10);
      if (n >= 1 && n <= TABS.length) setTab(TABS[n - 1][0]);
      if (e.key === "a") setAnno((v) => !v);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div>
      <div className="topbar">
        <div className="brandwrap">
          <LogoArt size="header" style={{ fontSize: 6, lineHeight: "6px" }} />
          <div className="brandtxt">
            <b>STACYVM</b>
            <span>TUI · WIREFRAME EXPLORATION</span>
          </div>
        </div>
        <div className="topspacer" />
        <div className="toggles">
          <button className={"tg" + (anno ? " on" : "")} onClick={() => setAnno((v) => !v)}>
            <span className="led" /> ANNOTATIONS
          </button>
          <button className={"tg" + (crt ? " on" : "")} onClick={() => setCrt((v) => !v)}>
            <span className="led" /> SCANLINES
          </button>
        </div>
      </div>

      <div className="tabs">
        {TABS.map((t, i) => (
          <button key={t[0]} className={"tab" + (tab === t[0] ? " active" : "")} onClick={() => setTab(t[0])}>
            <span className="num">{i + 1}</span>{t[1]}
          </button>
        ))}
      </div>

      <div className="wrap">
        {tab === "overview" && <Overview onPick={setTab} />}
        {tab === "a" && <DirectionA />}
        {tab === "b" && <DirectionB />}
        {tab === "c" && <DirectionC />}
        {tab === "logo" && <LogoLab />}
        {tab === "fixes" && <UXFixes />}
        <div className="footer">StacyVM TUI · wireframe exploration · {TABS.length} views · press 1–{TABS.length} to switch · a toggles annotations</div>
      </div>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
