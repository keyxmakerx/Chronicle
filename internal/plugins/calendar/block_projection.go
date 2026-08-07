// block_projection.go — C-CALV4-SPINE-P2 (calendar-v4 wave 1, W-A).
//
// The ONE honest per-viewer projection: Calendar + its candidate events +
// world state → calendar_block.BlockData.
//
// THE LOAD-BEARING RULE. ViewerContext.TiedCount and ViewerContext.WholeCount
// MUST come from the SAME viewer-filtered pass. Computing one before the filter
// and one after turns the tie toggle into an ORACLE: the difference between the
// two numbers is exactly the count of events the viewer is not allowed to know
// about. The signed mockup's own tiedCount is that leak
// (mockups/calendar-v4.html :2378 counts ties off the raw EV list while
// wholeCount counts off visFor()); it is deliberately NOT ported.
//
// THE SLICE TRAP (COMMON §7). filterEventsByUser compacts IN PLACE —
// `filtered := events[:0]` (service.go :2394) — so it MUTATES the caller's
// backing array. A second filtering pass reads a corrupted slice. This file
// filters exactly once, at the top of projectBlock, drops the input slice on
// the floor immediately afterwards, and derives the counts, the cells and the
// intercalary marks from that single result.
package calendar

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// BlockViewer is everything per-viewer the projection needs. Chronicle's IDs
// are UUID strings (calendars.id / calendar_events.id / entities.id are
// VARCHAR(36)), so they travel as strings here — see the identity note on
// projectBlock for why the pinned BlockData's int64 identity fields cannot
// carry them.
type BlockViewer struct {
	UserID string
	Role   int
	// HostEntity is the entity whose page hosts the Block, "" when it is not
	// hosted on one. It is what "tied" means.
	HostEntity string
	// TieMode is "tied" | "whole". Anything else resolves to "whole" when the
	// Block is not entity-hosted and "tied" when it is (the signed default).
	TieMode string
	// Zone is the viewer's IANA zone, labelled on every real-world time (L15).
	Zone string
}

// Identity is the BlockViewer as the visibility filters see it. A BlockViewer
// is always request-derived, so it can only ever produce a RequestViewer — an
// empty UserID here means an ANONYMOUS visitor, never a trusted system caller
// (C-AUTHZ-EMPTY-USERID / ADR-049).
func (v BlockViewer) Identity() permissions.Viewer {
	return permissions.RequestViewer(v.Role, v.UserID)
}

// BlockProjectionInput is the complete input to one Block render.
//
// Calendar MUST be non-nil — see the contract on projectBlock.
//
// Events is the CANDIDATE set — the rows the repository already narrowed by
// base visibility in SQL, for the rendered month plus any intercalary months
// hanging off it. It is consumed destructively (see the file header); callers
// must not read it again after projectBlock returns.
type BlockProjectionInput struct {
	Calendar   *Calendar
	Events     []Event
	Viewer     BlockViewer
	MonthIndex int
	Year       int

	// TiedEventIDs is the set of event IDs tied to Viewer.HostEntity, from the
	// batched tie read. Nil when the Block is not entity-hosted.
	TiedEventIDs map[string]bool

	// Sync is the campaign-level sync pill (CalendarLinkStatus). The projection
	// only overrides State to "pause" for a calendar that tracks real time,
	// because a real-time calendar's date is never pushed.
	Sync calblock.SyncPill

	// IsActive marks the viewer's currently-active calendar.
	IsActive bool
	// LedgerHidden / ShelfHidden dock the zones but hide them — the real-world
	// Block on the Bench renders with noShelf.
	LedgerHidden bool
	ShelfHidden  bool
	// MoonCap bounds the per-day discs; the ceiling is announced once in the
	// Nameplate, never per cell.
	MoonCap int
	// LayerPrefs is the VIEWER's stored layer set and their persistence
	// endpoint, resolved once per page by the caller (C-CALV4-LAYERS-P9).
	// The zero value is "no stored row, nowhere to persist", which renders DEF
	// with the ⋯ invoker inert — the wave-1/2 behaviour exactly.
	LayerPrefs blockLayerPrefs
}

// blockDefaultLayers is DEF — the default surface is a month with its moon
// phases and nothing else (L29).
//
// DEF DID NOT CHANGE IN WAVE 3, and that is the point. C-CALV4-LAYERS-P9 built
// the per-viewer store that L20/L26/L29 always required, so this function is now
// a SEED rather than a verdict: a viewer with no stored row still gets exactly
// ["moons"], and a viewer who has chosen gets their own set. "Bare mode is not a
// degraded view; for reading a month it is the better one" is only defensible if
// the viewer can leave it and have the leaving stick — shipping the mechanism to
// leave a default is the opposite of changing it.
func blockDefaultLayers(prefs blockLayerPrefs) calblock.LayerState {
	return resolveBlockLayers([]string{"moons"}, prefs)
}

// projectBlock turns a calendar + its candidate events into one BlockData.
//
// CONTRACT: in.Calendar is NON-NIL. The sole production caller —
// BlockService.Block — sits downstream of requireVisibleCalendar, which
// returns a calendar or an error, never nil-and-no-error; the seam and
// projection tests all construct a calendar. The projection's fault path is a
// calendar that cannot resolve a date (a MONTHLESS one — see blockDateLine),
// not a nil one, so this function reads cal.ID bare and carries no cal != nil
// guards of its own (coordinator ruling 2026-07-28 §3, stage 16). The shared
// helpers it calls (blockCalendarSlug, blockDateLine, …) keep their internal
// nil checks because they are also used standalone.
//
// IDENTITY — RESOLVED (r50 retype, amended r51). The four identity fields
// were typed int64 while every one of those identities is a VARCHAR(36) UUID
// in Chronicle, so this producer used to zero them and smuggle the calendar's
// id through CalendarSlug. The r50 retype made them `string`; the pin now in
// force is its r51 amendment (Mark.Tied + MonthGeometry.MoonsDeclared, which
// the books cite). The real ids are carried directly and CalendarSlug goes
// back to meaning the slug.
func projectBlock(in BlockProjectionInput) calblock.BlockData {
	cal := in.Calendar
	viewer := in.Viewer

	// ───────────────────────────────────────────────────────────────────────
	// THE ONE PASS. Everything below reads `visible`; `in.Events` is dead from
	// this line on (filterEventsByUser compacted it in place) and is nilled so
	// a later edit cannot accidentally re-read the corrupted backing array.
	// ───────────────────────────────────────────────────────────────────────
	visible := filterEventsByUser(in.Events, viewer.Identity())
	in.Events = nil

	geo := buildMonthGeometry(cal, blockMonthGeometryInput{
		MonthIndex: in.MonthIndex,
		Year:       in.Year,
		ShowMoons:  true,
		MoonCap:    in.MoonCap,
		// W-E: the Almanac register is the Shelf's data, so a Block whose host
		// removed the zone builds none.
		ShelfHidden: in.ShelfHidden,
	})

	data := calblock.BlockData{
		CalendarID:   cal.ID,
		CalendarSlug: blockCalendarSlug(cal),
		Name:         blockCalendarName(cal),
		CalHue:       blockCalHue(cal),
		Pattern:      blockCalPattern(cal),
		Letter:       blockCalLetter(cal),
		IsRealWorld:  cal.IsRealLife(),
		IsDefault:    cal.IsDefault,
		IsActive:     in.IsActive,
		Month:        geo,
		Sync:         blockSyncForCalendar(in.Sync, cal),
		Layers:       blockDefaultLayers(in.LayerPrefs),
		// BOTH ZONES ARE FILLED, and both signed `needs backend` chips retire
		// here — by a producer flag, without a template edit, which is the
		// promise the flags existed to keep. W-B filled the Ledger
		// (C-CALV4-LEDGER-P6 §9); W-E fills the Shelf (C-CALV4-SHELF-P7), and
		// with that the Block's four zones are all real.
		//
		// THE FIELDS STAY. They are the HOST's honesty switch rather than a
		// wave marker: a host that docks a zone it cannot fill turns one back
		// on and gets the signed chip instead of an empty box.
		//
		// NO NEW ZONE FLAG IS MINTED (2026-07-28 DEF/zone-chip ruling §2).
		// legend / horizon / moongraph keep their UNCONDITIONAL chips inside
		// their layer gates (block.templ) because no filling wave is named for
		// them, and the unconditional chip is the honest state today.
		Ledger:       calblock.LedgerStub{NeedsBackend: false, Hidden: in.LedgerHidden},
		Shelf:        calblock.ShelfStub{NeedsBackend: false, Hidden: in.ShelfHidden},
	}
	data.DateLabel, data.Fault = blockDateLine(cal)
	data.SeasonLabel, data.EraLabel = blockSeasonEraLabels(cal)

	// Counts and cells, both derived from `visible` and nothing else.
	tieMode := blockResolveTieMode(viewer)
	isGM := permissions.CanSeeDmOnly(viewer.Role)
	counts := blockCountEvents(cal, visible, in, tieMode, isGM)
	data.Viewer = calblock.ViewerContext{
		IsGM:       isGM,
		UserID:     viewer.UserID,
		HostEntity: viewer.HostEntity,
		TiedCount:  counts.tied,
		WholeCount: counts.whole,
		TieMode:    tieMode,
		Zone:       blockViewerZone(cal, viewer),
		// r52 rule 1: populated ONLY for a GM, at the SOURCE. blockCountEvents
		// leaves it zero for every other viewer by construction rather than the
		// renderer filtering it out later — a renderer-side gate is a rule one
		// careless template edit can lose.
		HiddenCount: counts.hidden,
	}

	blockDecorateCells(&data, cal, visible, in)
	return data
}

// blockResolveTieMode picks the mode. The signed default on an entity page is
// tie-filtered; off an entity page there is nothing to tie to, so "whole" is
// the only honest answer regardless of what the caller asked for.
func blockResolveTieMode(v BlockViewer) string {
	if v.HostEntity == "" {
		return "whole"
	}
	if v.TieMode == "whole" {
		return "whole"
	}
	return "tied"
}

// blockCounts is the pair that must never be differenceable, plus the GM-only
// hidden total r52 added.
//
// `hidden` is NOT a third member of the differenceable pair: a player receives
// it in NO form at all (r52 rule 3), so unlike tied/whole — which a player does
// receive, deliberately post-filter on both sides — there is nothing on a
// player's screen for it to be an oracle against.
type blockCounts struct{ tied, whole, hidden int }

// blockCountEvents counts DISTINCT viewer-visible events that the Block draws —
// the rendered month plus its intercalary rows. Both numbers come off the same
// `visible` slice, so their difference is the number of tied events among the
// events the viewer can already see, and nothing else. A viewer who cannot see
// event X contributes X to NEITHER number, so X cannot be recovered from the
// pair. TestBlockCountsAreNotAnOracle pins exactly that.
// r52 folds the hidden-event total into THIS SAME WALK (rule 2). It is the GM's
// own pass — a GM's `visible` is unfiltered by construction (filterEventsByUser
// returns early for a role that can see dm_only), so there is no
// pre-filter/post-filter pair anywhere in the function to difference. isGM
// gates the counter itself, so every other viewer gets a structural zero rather
// than a filtered-out number.
func blockCountEvents(cal *Calendar, visible []Event, in BlockProjectionInput, tieMode string, isGM bool) blockCounts {
	_ = tieMode // counts are mode-INDEPENDENT: both are always shown on the toggle
	var out blockCounts
	if cal == nil {
		return out
	}
	months := append([]int{in.MonthIndex}, blockIntercalaryMonths(cal, in.MonthIndex)...)
	seen := make(map[string]bool, len(visible))
	for i := range visible {
		e := &visible[i]
		if seen[e.ID] || !blockEventInMonths(cal, e, in.Year, months) {
			continue
		}
		seen[e.ID] = true
		out.whole++
		if blockEventTied(e, in) {
			out.tied++
		}
		// DISTINCT events, on the same dedupe: a recurring dm_only event
		// occurring on six days of the month is ONE hidden event, and counting
		// its marks would inflate the chip past anything a GM could reconcile.
		if isGM && e.Visibility == "dm_only" {
			out.hidden++
		}
	}
	return out
}

// blockEventInMonths reports whether an event occurs on any day of any of the
// given 0-based month indices in `year`, expanding recurrence through the
// single OccursOn predicate.
func blockEventInMonths(cal *Calendar, e *Event, year int, months []int) bool {
	for _, mi := range months {
		days := cal.MonthDays(mi, year)
		for d := 1; d <= days; d++ {
			if e.OccursOn(cal, year, mi+1, d) {
				return true
			}
		}
	}
	return false
}

// blockEventTied reports whether an event is tied to the host entity. Two
// primitives express that tie and BOTH count: the event's own entity_id column
// (calendar_events.entity_id, the original one-entity link) and the
// entity_event_links many-to-many table (migration 009), read in batch and
// handed in as TiedEventIDs. Off an entity page nothing is tied.
func blockEventTied(e *Event, in BlockProjectionInput) bool {
	if in.Viewer.HostEntity == "" {
		return false
	}
	if e.EntityID != nil && *e.EntityID == in.Viewer.HostEntity {
		return true
	}
	return in.TiedEventIDs[e.ID]
}

// blockDecorateCells fills every cell's marks and tie flag.
//
// It takes NO tie mode, and that absence is the r51 contract made structural:
// after the ink-not-membership ruling nothing about a cell's contents depends
// on which way the toggle is set. A mode parameter here could only be used to
// reintroduce the defect.
func blockDecorateCells(data *calblock.BlockData, cal *Calendar, visible []Event, in BlockProjectionInput) {
	if cal == nil {
		return
	}
	month := in.MonthIndex + 1
	// The time formatter is resolved ONCE per Block: time.LoadLocation reads the
	// zoneinfo database, and a month of dense cells would otherwise re-open it
	// per mark. Same discipline as the one-pass rule above it.
	times := blockMarkTimes(cal, in.Viewer)
	for r := range data.Month.Rows {
		for c := range data.Month.Rows[r].Cells {
			cell := &data.Month.Rows[r].Cells[c]
			if cell.Day == 0 {
				continue
			}
			marks, tied := blockMarksForDate(cal, visible, in, month, cell.Day, times)
			cell.Tied = tied
			cell.Marks, cell.MoreCount = blockCapMarks(marks)
		}
	}

	// Intercalary rows carry marks too — they are days of a real month, just one
	// with no cell in the tenday grid.
	inter := blockIntercalaryMonths(cal, in.MonthIndex)
	idx := 0
	for _, mi := range inter {
		days := cal.MonthDays(mi, in.Year)
		for d := 1; d <= days && idx < len(data.Month.Intercalary); d++ {
			marks, _ := blockMarksForDate(cal, visible, in, mi+1, d, times)
			data.Month.Intercalary[idx].Marks, _ = blockCapMarks(marks)
			idx++
		}
	}
}

// blockMarksForDate builds the marks for one date and reports whether the date
// carries at least one TIED event. Both come from the same walk over `visible`.
func blockMarksForDate(cal *Calendar, visible []Event, in BlockProjectionInput, month, day int, times blockMarkTimeFormatter) ([]calblock.Mark, bool) {
	var marks []calblock.Mark
	tied := false
	isGM := permissions.CanSeeDmOnly(in.Viewer.Role)
	for i := range visible {
		e := &visible[i]
		if !e.OccursOn(cal, in.Year, month, day) {
			continue
		}
		evTied := blockEventTied(e, in)
		if evTied {
			tied = true
		}
		// TieMode does NOT decide membership (r51). Every viewer-visible mark is
		// emitted in BOTH modes and flagged; the renderer stamps a class and CSS
		// changes the ink. Dropping untied marks here changed a cell's contents
		// and therefore its height, which broke the no-motion rule the toggle
		// depends on, and left the CSS-only toggle nothing to re-ink — CSS cannot
		// restore a mark the server never sent.
		marks = append(marks, blockMarkFor(cal, e, isGM, evTied, times))
	}
	return marks, tied
}

// blockCapMarks applies the signed per-cell ceiling and returns the overflow
// count. The FULL viewer-visible mark list is kept — the "+n more" popover
// needs it — and MoreCount tells the renderer how many of the tail to fold:
// it draws len(Marks)-MoreCount chips plus the affordance. Three marks, or two
// plus "+n more", never both, so the cell height is fixed and a mark count can
// never change a row's geometry.
func blockCapMarks(marks []calblock.Mark) ([]calblock.Mark, int) {
	if len(marks) <= blockCellMarkCap {
		return marks, 0
	}
	return marks, len(marks) - blockCellMarkCapOverflow
}

// blockMarkFor projects one event into a mark.
//
// The (hue, pattern) pair is LOCKED to one key so colour is never load-bearing
// on its own: the pattern is derived from the same key that chooses the hue, so
// two events that share a hue share a pattern and a viewer who cannot separate
// the hues can still separate the marks. Axis is a concrete colour value from
// the operator's own data and NEVER references --accent (which is the campaign
// accent and would collide with the surface's own axis channel).
// r52 widens it by two fields, both of which the widget package CANNOT derive:
// the formatted time (the widget has no clock, no calendar hour/minute geometry
// and no zone rules) and the event TYPE's display name (Axis is a hue token and
// the hue→name mapping lives in the campaign's data). Both drop their Ledger
// segment when empty rather than printing an empty element — the
// blockSyncStrings idiom.
func blockMarkFor(cal *Calendar, e *Event, isGM, tied bool, times blockMarkTimeFormatter) calblock.Mark {
	key, color, icon := blockEventAxisKey(cal, e)
	return calblock.Mark{
		EventID:   e.ID,
		Title:     e.Name,
		Axis:      color,
		Pattern:   blockPatternFor(key),
		Glyph:     icon,
		Named:     strings.TrimSpace(e.Name) != "",
		Tied:      tied,
		Time:      times.format(e),
		AxisLabel: blockEventAxisLabel(cal, e),
		Audience:  blockAudienceFor(e, isGM),
	}
}

// blockEventAxisLabel resolves the event TYPE's DISPLAY NAME — "Quest",
// "Festival" — for the Ledger row's meta line (r52, Mark.AxisLabel).
//
// It is deliberately a separate function rather than a fourth return value on
// blockEventAxisKey: the Bench's own axis read (bench.go) consumes that
// signature, and widening it would drag a file this slice does not own into the
// diff for no behavioural reason.
//
// It resolves ONLY through the calendar's declared categories. An event whose
// category slug matches no declared category yields "" and the meta line drops
// the segment, because a slug is an identifier and printing it would be the
// widget inventing a display name from data that does not carry one — the same
// refusal block.templ records for the legend layer.
func blockEventAxisLabel(cal *Calendar, e *Event) string {
	if cal == nil || e == nil || e.Category == nil {
		return ""
	}
	slug := strings.TrimSpace(*e.Category)
	if slug == "" {
		return ""
	}
	for i := range cal.EventCategories {
		if cal.EventCategories[i].Slug == slug {
			return strings.TrimSpace(cal.EventCategories[i].Name)
		}
	}
	return ""
}

// blockMarkTimeFormatter formats Mark.Time (r52). It is resolved ONCE per Block
// and carries the two facts a per-mark call would otherwise re-derive: whether
// this CALENDAR is real-world, and — only then — the viewer's resolved zone.
//
// THE ZONE LABEL IS FOLDED INTO THE STRING, and only on a real-world calendar.
// L15 requires every real-world time to carry its zone; the same rule forbids a
// zone-labelled time on an in-world calendar, where the label would be a claim
// about a clock that does not exist. Keeping the two in one formatter with one
// `realWorld` flag — taken from the same Calendar the renderer reads
// BlockData.IsRealWorld from — is what makes the two statements structurally
// unable to disagree. That is why r52 §5 refused a per-mark real-world flag and
// a separate ViewerContext.ZoneAbbr.
type blockMarkTimeFormatter struct {
	realWorld bool
	loc       *time.Location
}

// blockMarkTimes resolves the formatter for one Block.
func blockMarkTimes(cal *Calendar, v BlockViewer) blockMarkTimeFormatter {
	if cal == nil || !cal.IsRealLife() {
		return blockMarkTimeFormatter{}
	}
	f := blockMarkTimeFormatter{realWorld: true}
	if zone := strings.TrimSpace(blockViewerZone(cal, v)); zone != "" {
		if loc, err := time.LoadLocation(zone); err == nil {
			f.loc = loc
		}
	}
	return f
}

// format returns the event's start time, or "" when it has none — in which case
// the Ledger row DROPS the .tm segment rather than printing an empty one.
func (f blockMarkTimeFormatter) format(e *Event) string {
	if e == nil {
		return ""
	}
	t := e.FormatTime()
	if t == "" || !f.realWorld || f.loc == nil {
		return t
	}
	// The abbreviation is resolved at the event's OWN instant, not at "now", so
	// an event either side of a DST boundary is labelled with the offset that
	// actually applies to it.
	inst := time.Date(e.Year, time.Month(e.Month), e.Day, *e.StartHour, *e.StartMinute, 0, 0, f.loc)
	if abbr := inst.Format("MST"); abbr != "" {
		return t + " " + abbr
	}
	return t
}

// blockEventAxisKey resolves the single key that drives an event's hue, pattern
// and glyph: its category when it has one, otherwise the event itself. An
// explicit per-event colour/icon overrides the category's, which is the
// existing V2 precedence.
func blockEventAxisKey(cal *Calendar, e *Event) (key, color, icon string) {
	if e.Category != nil {
		key = *e.Category
	}
	if cal != nil && key != "" {
		for i := range cal.EventCategories {
			c := &cal.EventCategories[i]
			if c.Slug == key {
				color, icon = c.Color, c.Icon
				break
			}
		}
	}
	if e.Color != nil && *e.Color != "" {
		color = *e.Color
	}
	if e.Icon != nil && *e.Icon != "" {
		icon = *e.Icon
	}
	if key == "" {
		key = e.ID
	}
	return key, strings.TrimSpace(color), strings.TrimSpace(icon)
}

// blockAudienceFor is the pair of gold GM marks (L22: circles are moons,
// permission is gold).
//
// WAVE-1 RULING (COMMON §6.2): composed tag+member audiences DO NOT EXIST on
// main. There is no member_tags/user_tags table anywhere — tags attach to
// entities only — and the shipped people-grouping primitive is campaign_groups
// + campaign_group_members. So this is populated ONLY from what exists:
// visibility == dm_only ("GM only") or a visibility_rules restriction
// ("Restricted"). The composed audience is W-G's.
//
// Restricted is the DISCRIMINATOR between the two signed marks, not a synonym
// for "hidden" (the AudienceMark ruling in data.go, closed at P5 §3.3):
// dm_only → Restricted false, the gold DOGEAR; a visibility_rules restriction
// → Restricted true, the gold DIAMOND. Setting it true on both branches drew
// the diamond on every dm_only day and the dogear nowhere in the product.
//
// It is nil for a player, always. Permission is ABSENCE: a player who can see
// an event sees an ordinary mark with no hint that anyone else cannot, and a
// player who cannot see it never receives the row at all.
func blockAudienceFor(e *Event, isGM bool) *calblock.AudienceMark {
	if !isGM {
		return nil
	}
	if e.Visibility == "dm_only" {
		return &calblock.AudienceMark{Label: "GM only", Restricted: false}
	}
	if r := e.ParseVisibilityRules(); r != nil && (len(r.AllowedUsers) > 0 || len(r.DeniedUsers) > 0) {
		return &calblock.AudienceMark{Label: "Restricted", Restricted: true}
	}
	return nil
}

// --- identity --------------------------------------------------------------

// blockCalendarSlug carries the calendar's STABLE identity. It is the row's
// UUID, not a display slug: see the identity note on projectBlock — the pinned
// int64 CalendarID cannot hold a VARCHAR(36), and this is the only string
// identity field the contract has.
func blockCalendarSlug(cal *Calendar) string {
	if cal == nil {
		return ""
	}
	return cal.ID
}

func blockCalendarName(cal *Calendar) string {
	if cal == nil {
		return ""
	}
	return cal.Name
}

// blockCalHueTokens is the CLOSED set of --cal channel tokens the signed
// stylesheet defines (mockups/calendar-v4.html :82-85). There are four; a
// campaign with more calendars reuses them, which is correct — the token is a
// hue channel, not an identity, and identity is carried by the pattern +
// letter + name together.
var blockCalHueTokens = []string{"harptos", "elven", "dwarven"}

// blockCalHue assigns the --cal token. Real-world calendars take the signed
// "real" token and the campaign default takes the signed primary "harptos";
// every other calendar takes a token chosen by a STABLE hash of its UUID, so a
// calendar keeps its hue across renders, across reorders, and across restarts.
func blockCalHue(cal *Calendar) string {
	if cal == nil {
		return ""
	}
	if cal.IsRealLife() {
		return "real"
	}
	if cal.IsDefault {
		return blockCalHueTokens[0]
	}
	return blockCalHueTokens[blockStableHash(cal.ID)%uint32(len(blockCalHueTokens))]
}

// blockCalPattern assigns the GREYSCALE identity channel — never colour alone.
// The signed pairs are harptos/p1, real/p2, elven/p3, dwarven/p4; the default
// calendar and the real-world calendar take p1 and p2 to match, and the rest
// spread stably over p3..p8.
func blockCalPattern(cal *Calendar) string {
	if cal == nil {
		return ""
	}
	if cal.IsRealLife() {
		return "p2"
	}
	if cal.IsDefault {
		return "p1"
	}
	return fmt.Sprintf("p%d", 3+blockStableHash(cal.ID)%6)
}

// blockPatternFor locks a mark's pattern to its axis key.
func blockPatternFor(key string) string {
	if key == "" {
		return "p1"
	}
	return fmt.Sprintf("p%d", 1+blockStableHash(key)%8)
}

// blockCalLetter is the single-character calendar mark — the first rune of the
// name, upper-cased (signed: H, R, E, D).
func blockCalLetter(cal *Calendar) string {
	if cal == nil {
		return ""
	}
	for _, r := range strings.TrimSpace(cal.Name) {
		return strings.ToUpper(string(r))
	}
	return ""
}

// blockStableHash is FNV-1a over a UUID. Deterministic across processes — the
// point is that a calendar's hue and pattern do not shuffle between page loads
// or between two servers rendering the same campaign.
func blockStableHash(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// --- the date line ---------------------------------------------------------

// blockDateLine returns (DateLabel, Fault).
//
// Fault is the honesty state: when it is non-empty the Block prints the fault
// WHERE THE DATE WOULD GO and emits no date element at all — not a zero, not a
// placeholder, not an em dash (signed: the Dwarven .calrow.warnrow). It is
// raised only for a STRUCTURAL failure to resolve a date, which on main means
// exactly three things: no months at all, a current month outside the month
// list, or a current day past the (leap-aware) length of that month.
//
// The signed Dwarven fault text — "Needs eras — 0 eras defined, dates cannot
// resolve" — is deliberately NOT synthesised: Chronicle has no era-RELATIVE
// year numbering, so a calendar with zero eras still resolves "Deepwinter 14,
// 1523" perfectly well and printing a fault there would blank the date on most
// real calendars. The fault MECHANISM ships; the era variant lands when the
// model gains era-relative reckoning. Flagged to the coordinator.
func blockDateLine(cal *Calendar) (label, fault string) {
	if cal == nil {
		return "", "Needs a calendar — none resolved"
	}
	if len(cal.Months) == 0 {
		return "", "Needs months — 0 months defined, dates cannot resolve"
	}
	if cal.CurrentMonth < 1 || cal.CurrentMonth > len(cal.Months) {
		return "", fmt.Sprintf("Date out of range — month %d of %d", cal.CurrentMonth, len(cal.Months))
	}
	dim := cal.MonthDays(cal.CurrentMonth-1, cal.CurrentYear)
	if cal.CurrentDay < 1 || cal.CurrentDay > dim {
		return "", fmt.Sprintf("Date out of range — day %d of %d in %s",
			cal.CurrentDay, dim, cal.Months[cal.CurrentMonth-1].Name)
	}
	return blockDateLabel(cal), ""
}

// blockDateLabel formats the calendar's CURRENT date — the Nameplate's date
// line, not the navigated cursor.
//
// Real-world calendars get the signed Gregorian form "Sat 25 Jul 2026", built
// from the calendar's OWN weekday and month names (so a renamed month still
// reads correctly) rather than from time.Format. In-world calendars get the
// signed "Sithrel 9, Cycle 218" form: month, day, then the reckoning label —
// the calendar's epoch_name when it has one, else the entry of a declared
// yearly cycle, else nothing but the year.
func blockDateLabel(cal *Calendar) string {
	month := cal.Months[cal.CurrentMonth-1].Name
	if cal.IsRealLife() {
		wd := ""
		if wl := blockWeekLen(cal); wl > 0 && len(cal.Weekdays) > 0 {
			idx := v2WeekdayIndexFor(cal, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay)
			if idx >= 0 && idx < len(cal.Weekdays) {
				wd = blockAbbrev(cal.Weekdays[idx].Name, 3) + " "
			}
		}
		return fmt.Sprintf("%s%d %s %d", wd, cal.CurrentDay, blockAbbrev(month, 3), cal.CurrentYear)
	}
	reckoning := ""
	if cal.EpochName != nil && strings.TrimSpace(*cal.EpochName) != "" {
		reckoning = strings.TrimSpace(*cal.EpochName) + " "
	} else {
		for i := range cal.Cycles {
			if e := CycleEntryForYear(&cal.Cycles[i], cal.CurrentYear); e != nil && e.Name != "" {
				reckoning = e.Name + " "
				break
			}
		}
	}
	return fmt.Sprintf("%s %d, %s%d", month, cal.CurrentDay, reckoning, cal.CurrentYear)
}

// blockSeasonEraLabels resolves the season and era of the calendar's CURRENT
// date. Both are "" when the calendar declares none — an absent label says
// "this calendar has no seasons", which is information; a fabricated one is not.
func blockSeasonEraLabels(cal *Calendar) (season, era string) {
	if cal == nil {
		return "", ""
	}
	if s := cal.CurrentSeason(); s != nil {
		season = s.Name
	}
	if e := cal.CurrentEra(); e != nil {
		era = e.Name
	}
	return season, era
}

// blockViewerZone labels the viewer's zone. A real-time calendar's configured
// anchor zone is the fallback when the viewer's own is unknown, because a
// real-world time with no zone label is the exact ambiguity L15 forbids.
func blockViewerZone(cal *Calendar, v BlockViewer) string {
	if strings.TrimSpace(v.Zone) != "" {
		return v.Zone
	}
	if cal != nil && cal.RealTimeZone != nil {
		return *cal.RealTimeZone
	}
	return ""
}

// blockSyncForCalendar folds the per-CALENDAR fact into the campaign-level
// pill: a calendar that tracks real time computes its date from the wall clock
// and rejects manual writes, so its date is never pushed — the signed state for
// that is "pause" ("date push paused — tracks real time"). Everything else in
// the pill is campaign-level and comes from CalendarLinkStatus.
func blockSyncForCalendar(pill calblock.SyncPill, cal *Calendar) calblock.SyncPill {
	if cal == nil || !cal.UsesRealTime() {
		return pill
	}
	pill.State = blockSyncStatePause
	pill.Full, pill.Compact = blockSyncStrings(blockSyncFacts{
		State:  blockSyncStatePause,
		Linked: pill.Linked,
		Total:  pill.Total,
	})
	return pill
}

// --- ordering helper shared with the service -------------------------------

// blockSortEventsStable gives a total, collation-independent order to a set of
// events: date, then start time, then name, then ID. Used wherever a list of
// events must be deterministic across two servers and two MariaDB collations.
func blockSortEventsStable(evs []Event) {
	sort.SliceStable(evs, func(i, j int) bool {
		a, b := &evs[i], &evs[j]
		if a.Year != b.Year {
			return a.Year < b.Year
		}
		if a.Month != b.Month {
			return a.Month < b.Month
		}
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		ah, bh := blockDeref(a.StartHour, 99), blockDeref(b.StartHour, 99)
		if ah != bh {
			return ah < bh
		}
		am, bm := blockDeref(a.StartMinute, 99), blockDeref(b.StartMinute, 99)
		if am != bm {
			return am < bm
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
}

// blockDeref reads an optional int with a sentinel default.
func blockDeref(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}
