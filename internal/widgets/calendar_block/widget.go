// widget.go — the calendar Block's public surface and the registries the
// contract tests pin against.
//
// The package renders ONE component with four zones:
//
//	A NAMEPLATE   identity, the date line (or the fault where the date would go),
//	              the sync pill, the era stamp, the layers invoker
//	B INSTRUMENT  the month grid — ten-day weeks native, per-week-row era bands,
//	              the five-column rule, the intercalary row, the sub-mini tick rule
//	C LEDGER      docked, never a drawer. Wave 1 ships it at its real width with
//	              the signed `needs backend` chip; W-B fills it
//	D SHELF       the foot. Same deal; W-E fills it
//
// There is no drawer. The Ledger IS the drawer, permanently docked — canon D9's
// drawer clause was struck by canon amendment A3 precisely so that clicking a
// day repaints a panel already on screen instead of sliding one in.
package calendar_block

// StylesheetPath is the Block's stylesheet. It is UNLAYERED and SELF-CONTAINED
// per the rendering-canvas CSS exemption
// (cordinator decisions/2026-06-05-rendering-canvas-css-exemption.md):
// input.css's content is layered, and an unlayered standalone sheet beats every
// layer, which is exactly why the two cascade regimes never fight.
//
// It is linked ONLY through layouts.AssetURL — a bare href="/static/…" in any
// .templ fails TestTemplatesUseAssetURL repo-wide.
const StylesheetPath = "/static/css/calendar-block.css"

// LayerKeys is the eight-entry layer registry (L29 split moons into phases +
// graph). The first three are INSIDE layers: they change the month's own
// geometry and therefore apply instantly and silently (canon A8 / L-M2).
//
// WAVE 1 renders moons / eras / weeknums for real, and dockeds the Ledger and
// Shelf zones as stubs regardless of their layer keys (the full-tier column
// arithmetic subtracts the Ledger's 300px unconditionally, so the zone has to be
// there for the geometry to be honest). moongraph / legend / horizon render
// their zone with the signed `needs backend` chip — the same honesty state, for
// the same reason: the switchboard and the per-viewer preference store are W-F.
var LayerKeys = []string{
	"moons", "eras", "weeknums", // inside — geometry
	"ledger", "moongraph", "legend", "horizon", "shelf", // beside / below
}

// PatternKeys are the eight locked stroke patterns. They are the GREYSCALE
// IDENTITY CHANNEL: colour is never load-bearing, so every (hue, pattern) pair
// must still resolve with the hue removed.
//
//	p1 solid · p2 dash 4-2 · p3 dot 1-2 · p4 dash 2-2
//	p5 double hairline · p6 dash 6-2 · p7 solid + notch · p8 fine dash 1-1
//
// p7 and p8 are unassigned headroom. The (hue, pattern) pairs lock PER AXIS:
//
//	type      session p5 ■ · quest p2 ▲ · festival p4 ✦ · social p1 ◆ ·
//	          downtime p3 ● · celestial p6 ☾
//	owner     kael p1 · bryn p2 · tam p3 · nissa p4 · rell p5, plus initials
//	calendar  harptos p1 · real p2 · elven p3 · dwarven p4, plus a letter
//
// The pairing is the PRODUCER's to apply (Mark.Pattern); this package only
// guarantees that whatever arrives resolves to one of the eight.
var PatternKeys = []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8"}

// SyncStates are the five honesty states of the sync pill. The denominator
// never drops; only transport and timestamp do.
var SyncStates = []string{"ok", "drift", "bad", "pause", "none"}
