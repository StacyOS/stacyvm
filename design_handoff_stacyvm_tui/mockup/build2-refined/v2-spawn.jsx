// ====================================================================
// v2-spawn.jsx — animated sandbox spawn sequence (state machine)
// Plays: form → queue → pull layers → boot rootfs → net → READY
// ====================================================================

const SPAWN_STEPS = [
  { k: "queue",  label: "queue",        line: "scheduling on docker(runsc)…",        dur: 600 },
  { k: "pull",   label: "pull image",   line: "python:3.12-alpine  pulling layers",  dur: 1500, prog: true },
  { k: "boot",   label: "boot rootfs",  line: "unpacking · mounting overlay · init", dur: 1100, prog: true },
  { k: "net",    label: "network",      line: "veth up · 10.88.0.7 · preview *.stacy.dev", dur: 700 },
  { k: "ready",  label: "ready",        line: "sandbox live",                         dur: 0 },
];

function SpawnSequence({ embedded }) {
  const [phase, setPhase] = useState(0);     // index into SPAWN_STEPS
  const [prog, setProg] = useState(0);       // 0..100 for current step
  const [done, setDone] = useState(false);
  const timers = useRef([]);

  const clearAll = () => { timers.current.forEach(clearTimeout); timers.current = []; };

  const run = useCallback(() => {
    clearAll(); setPhase(0); setProg(0); setDone(false);
    let t = 0;
    SPAWN_STEPS.forEach((s, i) => {
      timers.current.push(setTimeout(() => {
        setPhase(i); setProg(0);
        if (s.prog) {
          const start = Date.now();
          const tick = () => {
            const p = Math.min(100, ((Date.now() - start) / s.dur) * 100);
            setProg(p);
            if (p < 100) timers.current.push(setTimeout(tick, 60));
          };
          tick();
        } else { setProg(100); }
      }, t));
      t += s.dur;
    });
    timers.current.push(setTimeout(() => setDone(true), t));
  }, []);

  useEffect(() => { run(); return clearAll; }, [run]);

  const step = SPAWN_STEPS[phase];
  const glyph = (i) => i < phase ? <Ok>✓</Ok> : i === phase ? (done && phase === SPAWN_STEPS.length - 1 ? <Ok>✓</Ok> : <span className="hi">◐</span>) : <span className="faint">○</span>;

  return (
    <Screen label="stacyvm › sandboxes › spawn" right="live sequence">
      <Ribbon />
      <Nav active="SANDBOXES" />

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
        {/* request */}
        <BFrame title="◇ SPAWN REQUEST" accent>
          <div style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "8px 14px", fontSize: 12, alignItems: "center" }}>
            <span className="dim">image</span><span className="ink">python:3.12-alpine</span>
            <span className="dim">template</span><span className="ink">data-science</span>
            <span className="dim">provider</span><span className="ink">docker <span className="faint">· runsc</span></span>
            <span className="dim">ttl</span><span className="ink">30m</span>
          </div>
        </BFrame>

        {/* live readout */}
        <BFrame title="◐ PROVISIONING" right={done ? "DONE" : "RUNNING"} accent={done}>
          {!done ? (
            <div style={{ fontSize: 12 }}>
              <div className="hi b" style={{ letterSpacing: 1 }}>{step.label.toUpperCase()}</div>
              <div className="dim" style={{ marginTop: 4, minHeight: 18 }}>{step.line}<Cur /></div>
              <div style={{ marginTop: 10 }}>
                <PBar pct={step.prog ? prog : 100} width={24} />
                <span className="dim" style={{ marginLeft: 8, fontSize: 11 }}>{Math.round(step.prog ? prog : 100)}%</span>
              </div>
            </div>
          ) : (
            <div style={{ fontSize: 12 }}>
              <div className="ok b" style={{ fontSize: 14 }}>✓ sb-9d11ba READY</div>
              <div className="dim" style={{ marginTop: 6, lineHeight: 1.7 }}>
                python:3.12-alpine · 10.88.0.7<br />
                booted in <span className="ok">1.42s</span> · expires 30m
              </div>
            </div>
          )}
        </BFrame>
      </div>

      {/* timeline */}
      <Mod title="SEQUENCE" right="docker(runsc)" style={{ marginTop: 12 }}>
        <div style={{ display: "grid", gap: 7, fontSize: 12 }}>
          {SPAWN_STEPS.map((s, i) => (
            <div key={s.k} style={{ display: "flex", alignItems: "center", gap: 11, opacity: i > phase && !done ? 0.4 : 1, transition: "opacity .3s" }}>
              <span style={{ width: 14 }}>{glyph(i)}</span>
              <span style={{ width: 96, color: i === phase && !done ? "var(--orange)" : "var(--ink)" }}>{s.label}</span>
              <span className="dim" style={{ flex: 1 }}>{s.line}</span>
              {i < phase || (done) ? <span className="ok" style={{ fontSize: 10.5 }}>{(s.dur / 1000).toFixed(2)}s</span>
                : i === phase ? <span className="hi" style={{ fontSize: 10.5 }}>· · ·</span>
                : <span className="faint" style={{ fontSize: 10.5 }}>—</span>}
            </div>
          ))}
        </div>
      </Mod>

      <div style={{ display: "flex", alignItems: "center", marginTop: 12, paddingTop: 9, borderTop: "1px solid var(--line)" }}>
        <span className="faint" style={{ fontSize: 11 }}>spawn progress surfaces here AND in the ribbon — never hidden on another tab</span>
        <span style={{ flex: 1 }} />
        <button className="replay" onClick={run}>↻ REPLAY SEQUENCE</button>
      </div>
    </Screen>
  );
}

Object.assign(window, { SpawnSequence });
