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
}
