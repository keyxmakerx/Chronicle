// PINNED BY THE COORDINATOR — calendar-v4 wave 1. DO NOT EDIT THIS TYPE SET.
//
// This file is the ONE cross-slice contract of calendar-v4 wave 1. Three
// dispatches reference it (C-CALV4-FOUNDATION-P0, C-CALV4-BLOCK-P1,
// C-CALV4-SPINE-P2). It exists so that the renderer and the producer are not
// two chats independently guessing the same struct.
//
// Copy it to internal/widgets/calendar_block/data.go BYTE-IDENTICALLY,
// including this header. C-CALV4-FOUNDATION-P0 lands it. If it is not yet on
// main when your slice branches, create it yourself from this file — an
// identical file added on two branches merges without conflict (r31.1
// precedent). Changing a field name or type is a STOP-AND-FLAG, not a
// judgement call: it desynchronises a parallel chat you cannot see.
//
// AMENDED r53 (2026-07-28). The Almanac register for the wave-2 Shelf (W-E):
// MonthGeometry.Almanac []AlmanacMoon plus the AlmanacMoon and AlmanacDay
// types — the full celestial register, deliberately UNCAPPED by MoonCap (the
// second half of L21: the grid's three-moon ceiling is only legitimate because
// the Almanac carries every declared body at full width). See
// decisions/2026-07-28-calv4-shelf-pin-amendment.md. Its §4b
// (ShelfStub.Upcoming) was NOT signed — Upcoming is month-scoped (ruling S1).
//
// AMENDED r52 (2026-07-28). Three additive fields for the wave-2 Ledger (W-B):
// Mark.Time and Mark.AxisLabel (the row's time column and meta line, neither
// derivable in a plugin-agnostic widget) and ViewerContext.HiddenCount (the
// signed "<N> hidden" chip — GM-only BY CONSTRUCTION; see the field's own
// comment for the four rules that answer r51 §4's set-aside). See
// decisions/2026-07-28-calv4-ledger-p6-pin-amendment.md.
//
// AMENDED r51 (2026-07-27). Two additive fields: Mark.Tied and
// MonthGeometry.MoonsDeclared. See decisions/2026-07-27-calv4-tie-mark-emission.md.
// Mark.Tied makes the tie toggle an INK change instead of a membership change —
// the producer emits the whole viewer-visible set in both modes and flags each
// mark, so CSS can dim rather than the server dropping cells. MoonsDeclared
// states how many moons the calendar declares, since the grid draws at most
// three and a fourth would otherwise vanish silently.
//
// AMENDED r50 (2026-07-26) after the wave-1 gate. Four identity fields were
// int64 and are now string: Chronicle's ids are VARCHAR(36)/CHAR(36) UUIDs
// (calendars, calendar_events, users) and every Go model already uses string.
// The original pin was derived from the signed design, which shows no types.
// Also clarified, because two chats read them in opposite directions and both
// readings were defensible: CalHue, MoreCount, AudienceMark.Restricted,
// EraBand.Half, and the two NeedsBackend flags.
//
// Field set derived from the SIGNED contract, cordinator
// mockups/calendar-v4.html @ 37cdd6d + mockups/renders/v4-*.
// Rulings that shaped it are in the dispatch files under "Coordinator rulings".

// Package calendar_block renders the calendar Block: one component, four zones
// (Nameplate / Instrument / docked Ledger / Shelf), size class taken from HOST
// width and density taken from MEASURED COLUMN width, both in CSS container
// queries rather than Go. Ten-day weeks are native; there is no literal 7.
//
// The package is plugin-agnostic by construction — it imports nothing from
// internal/plugins/** — exactly as internal/widgets/calendar_v2 is.
package calendar_block

// BlockData is the complete render input for one Block. Everything the Block
// draws is in here; the Block performs no queries and holds no request state
// (widgetbindings renders bound blocks with context.Background()).
type BlockData struct {
	// --- Identity (Nameplate zone A) ---
	CalendarID   string // UUID (calendars.id is VARCHAR(36))
	CalendarSlug string // stable identity; drives the --cal channel
	Name         string
	// CalHue is a TOKEN NAME, not a colour value: "harptos" | "real" | "elven" |
	// "dwarven" | … The RENDERER maps it to var(--cal-<token>) through a fixed
	// allowlist and falls back to the neutral structural rule for an unknown
	// token. A renderer that whitelists only colour VALUES will grey out every
	// calendar's identity channel — which is exactly what happened in wave 1.
	CalHue       string
	Pattern      string // p1..p8 — the GREYSCALE identity channel, never colour alone
	Letter       string // single-character calendar mark
	IsRealWorld  bool
	IsDefault    bool
	IsActive     bool

	// --- The date line ---
	// Fault is the honesty state. When non-empty the Block prints the fault
	// WHERE THE DATE WOULD GO and emits no date element at all — it does not
	// print a zero, a placeholder, or an em dash. (Signed: the Dwarven
	// .calrow.warnrow, "Needs eras — 0 eras defined, dates cannot resolve".)
	DateLabel   string // "Sithrel 9, Cycle 218"
	SeasonLabel string
	EraLabel    string
	Fault       string

	// --- Instrument (zone B) ---
	Month MonthGeometry

	// --- Zone state ---
	Sync   SyncPill
	Layers LayerState
	Ledger LedgerStub
	Shelf  ShelfStub

	// --- Viewer ---
	Viewer ViewerContext
}

// MonthGeometry is one month, already resolved for a specific year. Days is
// LEAP-AWARE (Calendar.MonthDays(idx, year)), not the raw Months[i].Days the
// V2 helpers read.
type MonthGeometry struct {
	Index       int
	Year        int
	Name        string
	WeekLen     int // --week-len. NEVER defaulted to 7 anywhere, CSS or Go.
	Lead        int // blank leading cells before day 1
	Days        int // days in THIS month in THIS year
	RowCount    int // ceil((Lead+Days)/WeekLen)
	Weekdays    []Weekday
	Rows        []WeekRow
	Intercalary []IntercalaryDay // rendered at full tier only
	TodayDay    int              // 0 when today is not in this month

	// MoonsDeclared is how many moons the CALENDAR declares, which is not how many
	// the grid draws. The grid's ceiling is three (moonCap) so a month can never
	// grow with the fiction; a calendar declaring four leaves one drawn nowhere.
	// Without this the omission is silent and a builder wonders why the moon they
	// configured never appears. len(Moons) per cell cannot supply it — already capped.
	MoonsDeclared int

	// Almanac is the full celestial register for THIS month: every moon the
	// calendar declares, at full width, deliberately UNCAPPED by MoonCap.
	//
	// It is the data behind contract §8 item 2 (L5) — "a dedicated filtered
	// Almanac list in the shelf widget" — and it is the second half of L21. The
	// grid's three-moon ceiling is only legitimate because "the Almanac carries
	// every declared body at full width"; without this field the fourth declared
	// moon exists in the database, is counted by MoonsDeclared, and is drawn
	// nowhere.
	//
	// It CANNOT be derived from DayCell.Moons: that list is capped at moonCap,
	// so the overflow body has no illumination anywhere in BlockData, and a
	// MoonDisc is one day's disc geometry with no period, no turn and no drift.
	//
	// Empty when the calendar declares no moons, or when no Shelf is reachable
	// in this render. Never partially filled — a moon that appears here appears
	// with its whole month, because the Month lane, the Tonight readout and the
	// Moons arithmetic are three views of one pass.
	Almanac []AlmanacMoon
}

// AlmanacMoon is one declared moon over one month.
//
// Drawn is whether the month grid draws this body. It is the ONE place the
// renderer learns which bodies the ceiling excluded; re-deriving it by counting
// DayCell.Moons would put moonCap's arithmetic in two places, and the nameplate
// badge already proves how that goes wrong (it states a total it cannot itself
// compute, which is why MoonsDeclared exists).
//
// There is deliberately NO Epithet. calendar.Moon has no epithet column
// (internal/plugins/calendar/model.go:461-473); the mockup's "the great pale
// moon" is fixture text. Wave 2 prints no epithet rather than inventing one.
type AlmanacMoon struct {
	Name       string  // the moon's configured name
	PeriodDays float64 // the declared cycle length, in days
	Drawn      bool    // whether the month grid draws this body (within moonCap)

	// TurnsThisMonth and DriftDays exist because the signed Moons panel prints
	// its own arithmetic — "the arithmetic is printed so it can be audited, no
	// date in the register was typed by hand". Both are computed from PeriodDays
	// against the month's REAL day count. The mockup's `30 % period` hardcodes a
	// thirty-day month and must not be ported into a product whose thesis is
	// arbitrary month lengths.
	TurnsThisMonth int
	DriftDays      float64

	// Days is one entry per day of this month, in ordinal order, always the
	// month's full length. Per-day illumination is where the VETOED composite
	// "how bright is tonight" scalar was relocated (L19/L24): a magnitude as an
	// explicit readout, never a glanceable claim. Deriving it in the renderer
	// would mean shipping the phase function into the widget package, which has
	// no calendar geometry and no leap rules.
	Days []AlmanacDay

	// NextNewDay / NextFullDay are the next turn of each kind at or after the
	// answered day, 0 when this month contains none. The Tonight readout prints
	// "next full 19". They ride along rather than being scanned out of Days
	// because the answered day is per-render state and the scan would otherwise
	// run in the template.
	NextNewDay  int
	NextFullDay int
}

// AlmanacDay is one moon on one day of the month.
//
// Day is the ordinal day, 1-based, matching DayCell.Day and the data-day key
// namespace (dayKey => "<slug>-<day>") — the Almanac's month lane is a DATED
// surface under guard B4 and an ANSWER partner of the grid and the Ledger.
type AlmanacDay struct {
	Day   int     // ordinal day of the month, 1-based
	Illum float64 // illuminated fraction, 0..1
	Phase string  // the phase word, e.g. "waxing gibbous"
	Turn  string  // "" | "new" | "full" — a turn landing on this day
	Node  bool    // inside the node window the .abr bracket draws
}

type Weekday struct {
	Name  string
	Abbr  string
	Half  bool // the five-column rule, on the header cell
	Index int
}

// WeekRow is one row of the grid. Era bands are PER WEEK ROW — an era spanning
// days 1..17 cannot span a subgrid across three rows (span 17 silently wrecks
// the grid; §9 deviation 2).
type WeekRow struct {
	Index int
	Bands []EraBand
	Cells []DayCell
}

type EraBand struct {
	Label     string
	Suffix    string // the season, folded in as a suffix on row 0 ONLY (§9 dev 3)
	StartCol  int    // 1-based, relative to the week gutter
	Span      int
	BandHue   string // --bandhue; driven by the ERA, never hardcoded per calendar
	OpenLeft  bool   // era continues before this row
	OpenRight bool   // era continues after this row
	Edge      bool   // editorial rule at a mid-month era boundary
	// Half means THIS BAND CONTAINS the half column. The rule itself is drawn by
	// the dedicated half-column ruler element at the half column — the band must
	// NOT also put a border on its own right edge, which lands the rule wherever
	// the band happens to end.
	Half      bool
}

type DayCell struct {
	Day         int // 0 = out of range (lead/trail)
	Col         int // 1-based within the week
	Half        bool
	IsToday     bool
	Intercalary bool
	Moons       []MoonDisc
	Marks       []Mark
	// MoreCount is OVERLAPPING, not additive: Marks holds the FULL viewer-visible
	// list for the day, and MoreCount is how many of those are not drawn as
	// chips. The day's event total is len(Marks) — NEVER len(Marks)+MoreCount.
	// Adding them double-counts and the Block prints a wrong total in its foot.
	// The ceiling is declared once, in the Nameplate (L30).
	MoreCount   int
	Tied        bool // this day carries at least one event tied to the host entity

	// Fogged is the knowledge-horizon layer.
	// WAVE-1 RULING: there is no queryable fog horizon on main — m.horizon is a
	// literal on the mockup's month object. Producers MUST leave this false and
	// the Horizon surfaces MUST render the signed `needs backend` chip. The
	// field exists so W-F does not have to re-touch every cell.
	Fogged bool
}

type IntercalaryDay struct {
	Name  string
	Day   int
	Marks []Mark
}

// MoonDisc is a derived terminator, not a pie fill (L25). Illum is 0..1.
type MoonDisc struct {
	Name       string
	Illum      float64
	Waxing     bool
	Eclipse    bool
	Terminator string // the derived path/clip descriptor
}

type Mark struct {
	EventID  string // UUID (calendar_events.id is VARCHAR(36))
	Title    string
	Axis     string // --axis value. FORBIDDEN from referencing --accent.
	Pattern  string // p1..p8 — locked (hue, pattern) pair; colour is never load-bearing
	Glyph    string // ■ ▲ ✦ ◆ ● ☾ …
	Named    bool   // named chip vs underline; decided in CSS, this is the content hint

	// Tied is whether THIS event is tied to ViewerContext.HostEntity.
	//
	// The producer emits the WHOLE viewer-visible set in BOTH tie modes and sets
	// this per mark; TieMode never removes a mark from a cell. Dropping untied
	// marks in "tied" mode changes a cell's contents and therefore its height,
	// which breaks the no-motion rule the toggle depends on, and leaves a CSS-only
	// toggle nothing to re-ink. Ink is the renderer's job; membership is not
	// TieMode's.
	Tied     bool

	// Time is the event's time, ALREADY FORMATTED, or "" when the event has no
	// time — in which case the Ledger row DROPS the segment rather than printing
	// an empty one.
	//
	// Formatted in the producer because that is where the calendar's own
	// hour/minute geometry lives (it is not 24x60 on every calendar) and where
	// the viewer's zone can be resolved. The widget package is
	// plugin-agnostic: it has no clock, no calendar geometry and no zone rules.
	//
	// There is deliberately NO per-mark real-world flag. Whether this string is
	// a real-world time (rendered .tm, with a zone abbreviation appended by the
	// producer) or an in-world time (rendered .tm.mono, never zone-labelled) is
	// a property of the CALENDAR, and BlockData.IsRealWorld already carries it.
	// A second copy of one fact can disagree with itself, and the disagreement
	// is precisely L15's forbidden case: a zone-labelled real-world time
	// printed on an in-world calendar.
	Time string

	// AxisLabel is the event TYPE's display name ("Quest", "Festival") — the
	// first segment of the Ledger row's meta line. Empty drops the segment.
	//
	// It cannot be derived from Axis: Axis is a hue token, and the mapping from
	// hue to type name lives in the campaign's data, not in the widget. This is
	// the same absence block.templ already books as the reason the `legend`
	// layer cannot be built. Adding it here does NOT transfer the legend to
	// this wave — the legend zone's NeedsBackend flag stays unminted until the
	// wave that fills it (2026-07-28 zone-chip ruling §2).
	AxisLabel string

	Audience *AudienceMark
}

// AudienceMark is the gold diamond (L22: circles are moons, permission is a
// diamond). GM/co-DM only — for a player it is nil and the mark is an ordinary
// mark, because permission is ABSENCE, not a greyed placeholder.
//
// WAVE-1 RULING: composed tag+member audiences DO NOT EXIST on main (no
// member_tags table; the shipped people primitive is campaign_groups). Wave 1
// populates this ONLY from what exists: visibility == dm_only, or a
// visibility_rules restriction. Label is then "GM only" or "Restricted".
// The composed audience is W-G.
// Restricted is the DISCRIMINATOR between the two signed GM marks, not a
// synonym for "hidden":
//   Restricted == false -> the gold DOGEAR   (visibility == dm_only)
//   Restricted == true  -> the gold DIAMOND  (a visibility_rules restriction)
// Setting it true on both branches means the dogear never renders anywhere in
// the product. Label is "GM only" or "Restricted" to match.
type AudienceMark struct {
	Label      string
	Restricted bool
}

// SyncPill is an honesty state. The DENOMINATOR NEVER DROPS — only transport
// and timestamp do. Full and Compact are BOTH emitted; CSS container queries
// choose which is visible (full tier → Full, std → Compact, mini/submini →
// neither). Producers do not decide tier.
//
// WAVE-1 RULING on the numerator: "linked" is DEFINED, not queried.
// sync_mappings has no calendar type and every syncapi calendar endpoint
// resolves the campaign default, so Linked = 1 when a module is connected
// (the campaign-default calendar is the one reachable), else 0. Total = the
// number of calendars in the campaign. This is exactly what the signed
// "In sync · 1 of 4 linked" says. Do not invent a per-calendar linkage.
type SyncPill struct {
	State     string // "ok" | "drift" | "bad" | "pause" | "none"
	Full      string
	Compact   string
	Linked    int
	Total     int
	Transport string // "Foundry" — omitted from Compact
	PushedAgo string // "pushed 2m ago" — omitted from Compact
}

// LayerState is the eight-entry layer registry (L29 split moons into phases +
// graph). DEF = ["moons"]: the default surface is a month with its moon phases
// and nothing else.
//
// Valid keys: "moons" "eras" "weeknums" "ledger" "moongraph" "legend"
// "horizon" "shelf". The first three are INSIDE layers — they change the
// month's geometry and therefore apply INSTANTLY AND SILENTLY (canon A8/L-M2).
//
// WAVE-1 RULING: layer preferences are per-viewer and persisted (L20/L26/L29),
// and that store does not exist yet. Wave 1 renders DEF and emits the ⋯
// invoker; the switchboard itself is W-F. HasSwitchboard is false in wave 1.
type LayerState struct {
	Enabled        []string
	HasSwitchboard bool
}

// LedgerStub / ShelfStub: wave 1 docks these zones at the correct size and
// renders the signed `needs backend` chip. That is an honesty state the
// operator signed, not a shortcut. W-B and W-E fill them.
// NeedsBackend GATES the chip — the renderer must not emit it unconditionally.
// W-B sets it false when the Ledger is filled, and it must not have to edit the
// stub template to stop the chip rendering.
type LedgerStub struct {
	NeedsBackend bool
	Hidden       bool // the real-world Block on the Bench renders with noShelf
}

type ShelfStub struct {
	NeedsBackend bool
	Hidden       bool
}

// ViewerContext carries everything per-viewer. TiedCount and WholeCount MUST
// come from THE SAME viewer-filtered pass — computing one pre-filter and one
// post-filter turns the tie toggle into an oracle that differences hidden
// events out of the two numbers. The mockup's own tiedCount is exactly that
// leak; do not port it.
//
// TieMode changes ink LEVEL only. No cell may grow, move, or leave the DOM
// when it flips — that is what makes it legal under the no-motion rule and
// what makes the counts non-differenceable.
type ViewerContext struct {
	IsGM       bool
	UserID     string // UUID (users.id is CHAR(36))
	HostEntity string // UUID; EMPTY when the Block is not hosted on an entity page.
	//                   Renderers gate the tie toggle on HostEntity != "".
	TiedCount  int
	WholeCount int
	TieMode    string // "tied" | "whole"
	Zone       string // IANA zone, labelled on every real-world time (L15)

	// HiddenCount is how many events on this month are hidden from players —
	// the signed "<N> hidden" chip on the nameplate, whose own title makes it a
	// Ledger control.
	//
	// GM-ONLY BY CONSTRUCTION, and the construction is the safety argument:
	//   1. the producer sets it ONLY when IsGM; it is zero for every other
	//      viewer at the source, never filtered out at the renderer;
	//   2. it comes from the SAME single viewer-filtered pass as TiedCount and
	//      WholeCount — the GM's pass, which sees everything, so there is no
	//      pre-filter/post-filter pair to difference;
	//   3. it is NEVER rendered to a player in any form — no chip, no title, no
	//      aria label, and above all no zero. Permission is ABSENCE.
	// Unlike the tie pair (which a player DOES receive, deliberately
	// non-differenceable), a player receives this in no form at all, so there
	// is nothing for it to be an oracle against.
	//
	// It cannot be derived by the renderer: the hidden events are not in a
	// player's BlockData at all, and counting Audience != nil across a GM's
	// marks would conflate dm_only with visibility_rules and double-count a
	// recurring event.
	HiddenCount int
}
