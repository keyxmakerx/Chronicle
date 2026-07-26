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
	CalendarID   int64
	CalendarSlug string // stable identity; drives the --cal channel
	Name         string
	CalHue       string // --cal token: "harptos" | "real" | "elven" | "dwarven" | …
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
	Half      bool   // the five-column rule reaches the band too
}

type DayCell struct {
	Day         int // 0 = out of range (lead/trail)
	Col         int // 1-based within the week
	Half        bool
	IsToday     bool
	Intercalary bool
	Moons       []MoonDisc
	Marks       []Mark
	MoreCount   int  // the "+n more" overflow; the ceiling is declared once, in the Nameplate (L30)
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
	EventID  int64
	Title    string
	Axis     string // --axis value. FORBIDDEN from referencing --accent.
	Pattern  string // p1..p8 — locked (hue, pattern) pair; colour is never load-bearing
	Glyph    string // ■ ▲ ✦ ◆ ● ☾ …
	Named    bool   // named chip vs underline; decided in CSS, this is the content hint
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
	UserID     int64
	HostEntity int64  // 0 when the Block is not hosted on an entity page
	TiedCount  int
	WholeCount int
	TieMode    string // "tied" | "whole"
	Zone       string // IANA zone, labelled on every real-world time (L15)
}
