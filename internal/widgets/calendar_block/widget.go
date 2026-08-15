// widget.go — the calendar Block's public surface and the registries the
// contract tests pin against.
//
// The package renders ONE component with four zones:
//
//	A NAMEPLATE   identity, the date line (or the fault where the date would go),
//	              the sync pill, the era stamp, the layers invoker
//	B INSTRUMENT  the month grid — ten-day weeks native, the era as a soft tint
//	              on the day cells (C-CALV4-TILES §3: no caption row, no era text
//	              in the grid at all), the five-column rule, the intercalary row,
//	              the sub-mini tick rule
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

// LayerRow is one row of the switchboard, and the shape is EXACTLY the signed
// registry's: {k, n, where[, inside]} (mockups/calendar-v4.html:1186-1195).
//
// NO FIFTH FIELD, EVER. A `gmOnly:` or `need:` was proposed and refused by name
// — "a PERMISSION dimension bolted onto a per-viewer DISPLAY PREFERENCE
// registry … a struct field invented to make a narrative work"
// (mockups/v4-proposed/REVIEW.md:264-268). Permission is expressed as ABSENCE
// at the surface the key controls, never as a column here; build status is
// expressed by the zone's own chip. The denominator this registry defines —
// "N of 8 on" — is IDENTICAL for owner, co-DM and player, and only the on-set
// differs.
type LayerRow struct {
	Key string
	// Name is the row's label ("Moon phases").
	Name string
	// Where says where the layer lands ("in the dates", "beside the month").
	// It is the row's whole explanation and it is why no tooltip is needed.
	Where string
	// Inside marks the three layers that change the MONTH'S OWN GEOMETRY and
	// therefore apply instantly and silently (canon A8 / L-M2). It is also what
	// splits the sheet into its two groups: "In the month" (instant · no
	// confirm, no animation) and "Below the month" (opens a section).
	Inside bool
}

// LayerRows is the switchboard's registry, pinned against the signed table.
//
// It lives BESIDE LayerKeys rather than travelling on BlockData (r54 §5): these
// are contract constants, identical for every viewer of every calendar, and a
// copy on the wire would be eight rows of static text re-serialised per Block.
// LayerKeys stays the ordering authority and a test holds the two in lockstep,
// so a key added to one and forgotten in the other is a red build rather than a
// row that silently vanishes from the sheet.
var LayerRows = []LayerRow{
	{Key: "moons", Name: "Moon phases", Where: "in the dates", Inside: true},
	{Key: "eras", Name: "Era & season", Where: "above each week", Inside: true},
	{Key: "weeknums", Name: "Week numbers", Where: "left margin", Inside: true},
	{Key: "ledger", Name: "Event list", Where: "beside the month"},
	{Key: "moongraph", Name: "Illumination graph", Where: "foot of the month"},
	{Key: "legend", Name: "Legend", Where: "under the month"},
	{Key: "horizon", Name: "Knowledge horizon", Where: "under the legend"},
	{Key: "shelf", Name: "Shelf", Where: "foot of the block"},
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
