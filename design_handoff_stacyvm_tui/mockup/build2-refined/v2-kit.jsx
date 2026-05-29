// ====================================================================
// v2-kit.jsx — primitives for the refined Mission Control build
// ====================================================================
const { useState, useEffect, useRef, useCallback } = React;

function LogoArt({ size = "header", style }) {
  const art = (window.STACY_LOGO_ART && window.STACY_LOGO_ART[size]) || [];
  return <pre className="brandmark" style={style}>{art.join("\n")}</pre>;
}

const Dim = (p) => <span className="dim">{p.children}</span>;
const Hi = (p) => <span className="hi">{p.children}</span>;
const Ok = (p) => <span className="ok">{p.children}</span>;
const Err = (p) => <span className="err">{p.children}</span>;
const Faint = (p) => <span className="faint">{p.children}</span>;
const Steel = (p) => <span className="steel">{p.children}</span>;

function Screen({ label, right, children, style, dots = true }) {
  return (
    <div className="screen" style={style}>
      <div className="scr-chrome">
        {dots && <span className="scr-dots"><i className="o" /><i /><i /></span>}
        <span>{label}</span><span style={{ flex: 1 }} />
        <span className="faint">{right}</span>
      </div>
      <div className="scr-body">{children}</div>
    </div>
  );
}

function Mod({ title, right, accent, children, style }) {
  return (
    <div className={"mod" + (accent ? " accent" : "")} style={style}>
      {title && <div className="mod-t"><span>{title}</span><span className="faint">{right}</span></div>}
      {children}
    </div>
  );
}

// bracketed tile (Direction B signature)
function BFrame({ title, right, accent, children, style }) {
  return (
    <div className={"bframe" + (accent ? " accent" : "")} style={style}>
      <span className="c tl">⌜</span><span className="c tr">⌝</span>
      <span className="c bl">⌞</span><span className="c br">⌟</span>
      {title && <div className="bt"><span>{title}</span><span className="ln" />{right && <span className="faint">{right}</span>}</div>}
      {children}
    </div>
  );
}

const Key = (p) => <span className="key">{p.children}</span>;
function KeyHint({ items }) {
  return (
    <span className="keyrow">
      {items.map((it, i) => (
        <span key={i} style={{ display: "inline-flex", gap: 5, alignItems: "center" }}>
          <Key>{it[0]}</Key><span className="dim">{it[1]}</span>
        </span>
      ))}
    </span>
  );
}

const SPARK = "▁▂▃▄▅▆▇█";
function Spark({ data, color = "hi" }) {
  const max = Math.max(...data, 1);
  return <span className={"spark " + color}>{data.map((d) => SPARK[Math.min(7, Math.round((d / max) * 7))]).join("")}</span>;
}

function Meter({ val, width = 14, color = "hi", showPct = true }) {
  const filled = Math.round((val / 100) * width);
  return (
    <span className="meter">
      <span className={color}>{"█".repeat(filled)}</span>
      <span className="faint">{"░".repeat(Math.max(0, width - filled))}</span>
      {showPct && <span className="dim"> {val}%</span>}
    </span>
  );
}

function Note({ tag, children }) {
  return <div className="note">{tag && <span className="tagn">{tag}</span>}{children}</div>;
}
function ScreenCard({ title, hand, children, label, right }) {
  return (
    <div>
      <Screen label={label} right={right}>{children}</Screen>
      <div className="caption"><span className="t">{title}</span>{hand && <span className="hand">{hand}</span>}</div>
    </div>
  );
}
const Divider = (p) => <div className="divider">{p.children}</div>;
function Move({ t, children }) { return <div className="move"><div className="mt">{t}</div><div className="md">{children}</div></div>; }
const Cur = ({ block }) => <span className={"cur" + (block ? " block" : "")} />;

// live-ish animated spark hook (drifts a window of values)
function useDrift(seed, n = 12, lo = 2, hi = 8) {
  const [data, setData] = useState(() => Array.from({ length: n }, (_, i) => lo + ((seed * (i + 3)) % (hi - lo))));
  useEffect(() => {
    const id = setInterval(() => {
      setData((d) => [...d.slice(1), lo + Math.floor(Math.random() * (hi - lo + 1))]);
    }, 1100);
    return () => clearInterval(id);
  }, []);
  return data;
}

// progress bar built from block chars
function PBar({ pct, width = 22, color = "hi" }) {
  const filled = Math.round((pct / 100) * width);
  return (
    <span className="pbar">
      <span className={color}>{"█".repeat(filled)}</span>
      <span className="faint">{"░".repeat(Math.max(0, width - filled))}</span>
    </span>
  );
}

Object.assign(window, {
  LogoArt, Dim, Hi, Ok, Err, Faint, Steel, Screen, Mod, BFrame, Key, KeyHint,
  Spark, Meter, Note, ScreenCard, Divider, Move, Cur, useDrift, PBar,
});
