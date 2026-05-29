// ====================================================================
// v2-boot.jsx — boot splash. Resting state is ALWAYS visible; the
// entrance is timer-driven (no rAF) so it never gets stuck hidden.
// ====================================================================

function BootSplash() {
  const [pre, setPre] = useState(false);   // false = visible resting state
  const [pct, setPct] = useState(100);
  const [stage, setStage] = useState("ready");
  const timers = useRef([]);

  const clearAll = () => { timers.current.forEach(clearTimeout); timers.current = []; };

  const play = useCallback(() => {
    clearAll();
    setPre(true); setPct(0); setStage("connecting");
    // timer (not rAF) flips to visible — fires even if the tab is backgrounded
    timers.current.push(setTimeout(() => setPre(false), 60));
    const dur = 1600;
    timers.current.push(setTimeout(() => {
      const t0 = Date.now();
      const tick = () => {
        const p = Math.min(100, ((Date.now() - t0) / dur) * 100);
        setPct(p);
        setStage(p >= 100 ? "ready" : p > 55 ? "handshake" : "connecting");
        if (p < 100) timers.current.push(setTimeout(tick, 50));
      };
      tick();
    }, 760));
  }, []);

  // play once on mount, but only while actually visible (else stay at the
  // visible resting state so a backgrounded render is never blank)
  useEffect(() => {
    if (typeof document !== "undefined" && !document.hidden) play();
    return clearAll;
  }, [play]);

  const width = 26;
  const filled = Math.round((pct / 100) * width);
  return (
    <Screen label="boot splash" right="stacyvm tui">
      <div className={"boot" + (pre ? " pre" : "")}>
        <div className="mk"><LogoArt size="hero" style={{ fontSize: 10, lineHeight: "10px" }} /></div>
        <div className="wm">STACYVM</div>
        <div className="tag">microVM sandbox orchestrator for LLMs</div>
        <div className="barwrap">
          <span className="pbar"><span className="hi">{"█".repeat(filled)}</span><span className="faint">{"░".repeat(width - filled)}</span></span>
          <span className="dim" style={{ marginLeft: 10 }}>
            {stage === "connecting" && <>connecting :7423<Cur /></>}
            {stage === "handshake" && <>handshake · loading fleet<Cur /></>}
            {stage === "ready" && <span className="ok">✓ ready · 5 sandboxes</span>}
          </span>
        </div>
      </div>
      <div style={{ display: "flex", alignItems: "center", padding: "0 4px", borderTop: "1px solid var(--line)", paddingTop: 10 }}>
        <span className="faint" style={{ fontSize: 11 }}>logo springs in (harmonica), then the connect bar fills</span>
        <span style={{ flex: 1 }} />
        <button className="replay" onClick={play}>↻ REPLAY</button>
      </div>
    </Screen>
  );
}

Object.assign(window, { BootSplash });
