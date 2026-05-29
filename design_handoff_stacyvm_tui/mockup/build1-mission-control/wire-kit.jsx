// ====================================================================
// wire-kit.jsx — shared primitives for the StacyVM TUI wireframes
// ====================================================================
const { useState, useEffect, useRef } = React;

// ---- logo art -------------------------------------------------------
function LogoArt({ size = "header", style }) {
  const art = (window.STACY_LOGO_ART && window.STACY_LOGO_ART[size]) || [];
  return (
    <pre className="brandmark" style={{ fontSize: size === "hero" ? 9 : 6, lineHeight: size === "hero" ? "9px" : "6px", ...style }}>
      {art.join("\n")}
    </pre>
  );
}

// ---- text spans -----------------------------------------------------
const Dim = (p) => <span className="dim">{p.children}</span>;
const Hi = (p) => <span className="hi">{p.children}</span>;
const Ok = (p) => <span className="ok">{p.children}</span>;
const Err = (p) => <span className="err">{p.children}</span>;
const Faint = (p) => <span className="faint">{p.children}</span>;
const Steel = (p) => <span className="steel">{p.children}</span>;

// ---- terminal screen frame -----------------------------------------
function Screen({ label, right, children, style, dots = true }) {
  return (
    <div className="screen" style={style}>
      <div className="scr-chrome">
        {dots && (
          <span className="scr-dots"><i className="o" /><i /><i /></span>
        )}
        <span>{label}</span>
        <span style={{ flex: 1 }} />
        <span className="faint">{right}</span>
      </div>
      <div className="scr-body">{children}</div>
    </div>
  );
}

// ---- HUD module -----------------------------------------------------
function Mod({ title, right, accent, children, style }) {
  return (
    <div className={"mod" + (accent ? " accent" : "")} style={style}>
      {title && <div className="mod-t"><span>{title}</span><span className="faint">{right}</span></div>}
      {children}
    </div>
  );
}

// ---- keycap ---------------------------------------------------------
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

// ---- status dot -----------------------------------------------------
function Dot({ state }) {
  const map = { run: "ok", running: "ok", online: "ok", ok: "ok", idle: "dim", creating: "hi", warn: "hi", error: "err", off: "err", offline: "err" };
  const glyph = { creating: "◐", idle: "○", error: "✗", off: "●" };
  return <span className={map[state] || "dim"}>{glyph[state] || "●"}</span>;
}

// ---- block-char meter ----------------------------------------------
function Meter({ val, width = 14, color = "hi", showPct = true }) {
  const filled = Math.round((val / 100) * width);
  return (
    <span className="meter">
      <span className={color}>{"█".repeat(filled)}</span>
      <span className="faint">{"░".repeat(width - filled)}</span>
      {showPct && <span className="dim"> {val}%</span>}
    </span>
  );
}

// ---- sparkline ------------------------------------------------------
const SPARK = "▁▂▃▄▅▆▇█";
function Spark({ data, color = "hi" }) {
  const max = Math.max(...data, 1);
  return (
    <span className={"spark " + color}>
      {data.map((d) => SPARK[Math.min(7, Math.round((d / max) * 7))]).join("")}
    </span>
  );
}

// ---- annotation note ------------------------------------------------
function Note({ tag, children }) {
  return (
    <div className="note">
      {tag && <span className="tagn">{tag}</span>}
      {children}
    </div>
  );
}

// ---- screen + caption (for grid) -----------------------------------
function ScreenCard({ title, hand, children, label, right }) {
  return (
    <div>
      <Screen label={label} right={right}>{children}</Screen>
      <div className="caption">
        <span className="t">{title}</span>
        {hand && <span className="hand">{hand}</span>}
      </div>
    </div>
  );
}

// ---- section divider ------------------------------------------------
const Divider = (p) => <div className="divider">{p.children}</div>;

// ---- move chip ------------------------------------------------------
function Move({ t, children }) {
  return (
    <div className="move">
      <div className="mt">{t}</div>
      <div className="md">{children}</div>
    </div>
  );
}

// expose
Object.assign(window, {
  LogoArt, Dim, Hi, Ok, Err, Faint, Steel,
  Screen, Mod, Key, KeyHint, Dot, Meter, Spark, Note, ScreenCard, Divider, Move,
});
