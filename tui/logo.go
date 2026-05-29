package tui

// logo.go — STACY_LOGO_ART dropped in from mockup/logo-art.js.
// Pure text built from half-block chars; tinted orange in the TUI.
// `hero` is the boot mark, `header` the ribbon mark, `small` a compact mark.

var logoHero = []string{
	"    ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄",
	"  ███████████████████",
	" ████████████████████",
	"▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀",
	"▄▄▄▄▄▄▄▄▄▄▄",
	"███████████",
	"███████████",
	"           ██████████",
	"           ██████████",
	"           ▀▀▀▀▀▀▀▀▀▀",
	"▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄",
	"█████████████████████",
	"███████████████████▀",
	"▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀",
}

var logoHeader = []string{
	"██████████",
	"█████▀▀▀▀▀",
	"█████",
	"     █████",
	"▄▄▄▄▄█████",
	"█████████▀",
}

var logoSmall = []string{
	"▄███████",
	"▄▄▄▄",
	"▀▀▀▀▄▄▄▄",
	"    ▀▀▀▀",
	"███████▀",
}

// ribbonMark is the single-cell brand mark for the telemetry ribbon. The
// mockup renders the `header` art at ~5px (a tiny inline glyph); in a terminal
// the honest translation of that hierarchy is a compact one-cell mark rather
// than a 6-row block. The full art is reserved for the boot splash (logoHero).
const ribbonMark = "▞▚"
