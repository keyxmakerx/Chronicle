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

// BlockProjectionInput is the complete input to one Block render.
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
}

// blockDefaultLayers is DEF — the default surface is a month with its moon
// phases and nothing else (L29). HasSwitchboard is false in wave 1: layer
// preferences are per-viewer and PERSISTED (L20/L26/L29) and that store does
// not exist yet, so the Block renders DEF and emits the ⋯ invoker while W-F
// builds the switchboard.
func blockDefaultLayers() calblock.LayerState {
	return calblock.LayerState{Enabled: []string{"moons"}, HasSwitchboard: false}
}

// projectBlock turns a calendar + its candidate events into one BlockData.
//
// IDENTITY — RESOLVED (pin r50). The four identity fields were typed int64
// while every one of those identities is a VARCHAR(36) UUID in Chronicle, so
// this producer used to zero them and smuggle the calendar's id through
// CalendarSlug. The pin was amended and they are now `string`; the real ids
// are carried directly. CalendarSlug goes back to meaning the slug.
func projectBlock(in BlockProjectionInput) calblock.BlockData {
	cal := in.Calendar
	viewer := in.Viewer

	// ───────────────────────────────────────────────────────────────────────
	// THE ONE PASS. Everything below reads `visible`; `in.Events` is dead from
	// this line on (filterEventsByUser compacted it in place) and is nilled so
	// a later edit cannot accidentally re-read the corrupted backing array.
	// ───────────────────────────────────────────────────────────────────────
	visible := filterEventsByUser(in.Events, viewer.Role, viewer.UserID)
	in.Events = nil

	geo := buildMonthGeometry(cal, blockMonthGeometryInput{
		MonthIndex: in.MonthIndex,
		Year:       in.Year,
		ShowMoons:  true,
		MoonCap:    in.MoonCap,
	})

	data := calblock.BlockData{
		CalendarID:   cal.ID,
		CalendarSlug: blockCalendarSlug(cal),
		Name:         blockCalendarName(cal),
		CalHue:       blockCalHue(cal),
		Pattern:      blockCalPattern(cal),
		Letter:       blockCalLetter(cal),
		IsRealWorld:  cal != nil && cal.IsRealLife(),
		IsDefault:    cal != nil && cal.IsDefault,
		IsActive:     in.IsActive,
		Month:        geo,
		Sync:         blockSyncForCalendar(in.Sync, cal),
		Layers:       blockDefaultLayers(),
		Ledger:       calblock.LedgerStub{NeedsBackend: true, Hidden: in.LedgerHidden},
		Shelf:        calblock.ShelfStub{NeedsBackend: true, Hidden: in.ShelfHidden},
	}
	data.DateLabel, data.Fault = blockDateLine(cal)
	data.SeasonLabel, data.EraLabel = blockSeasonEraLabels(cal)

	// Counts and cells, both derived from `visible` and nothing else.
	tieMode := blockResolveTieMode(viewer)
	counts := blockCountEvents(cal, visible, in, tieMode)
	data.Viewer = calblock.ViewerContext{
		IsGM:       permissions.CanSeeDmOnly(viewer.Role),
		UserID:     viewer.UserID,
		HostEntity: viewer.HostEntity,
		TiedCount:  counts.tied,
		WholeCount: counts.whole,
		TieMode:    tieMode,
		Zone:       blockViewerZone(cal, viewer),
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

// blockCounts is the pair that must never be differenceable.
type blockCounts struct{ tied, whole int }

// blockCountEvents counts DISTINCT viewer-visible events that the Block draws —
// the rendered month plus its intercalary rows. Both numbers come off the same
// `visible` slice, so their difference is the number of tied events among the
// events the viewer can already see, and nothing else. A viewer who cannot see
// event X contributes X to NEITHER number, so X cannot be recovered from the
// pair. TestBlockCountsAreNotAnOracle pins exactly that.
func blockCountEvents(cal *Calendar, visible []Event, in BlockProjectionInput, tieMode string) blockCounts {
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

// blockDecorateCells layers the per-viewer marks onto the geometry.
//
// TieMode changes which marks are emitted, exactly as the signed contract does
// (mockups/calendar-v4.html VIEWS.entity: tied mode passes `filter = e =>
// e.ties`). DayCell.Tied is set from the same pass in BOTH modes, so a cell
// that carries a tie is identifiable whichever way the toggle sits.
//
// KNOWN CONTRACT GAP, flagged to the coordinator: in "whole" mode the signed
// render dims the individual untied CHIPS (opacity .7 + a hairline rail), but
// the pinned Mark struct carries no per-mark tie flag — only DayCell.Tied,
// which is per DAY. A day with one tied and one untied event cannot express
// that distinction in wave 1.
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
	for r := range data.Month.Rows {
		for c := range data.Month.Rows[r].Cells {
			cell := &data.Month.Rows[r].Cells[c]
			if cell.Day == 0 {
				continue
			}
			marks, tied := blockMarksForDate(cal, visible, in, month, cell.Day)
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
			marks, _ := blockMarksForDate(cal, visible, in, mi+1, d)
			data.Month.Intercalary[idx].Marks, _ = blockCapMarks(marks)
			idx++
		}
	}
}

// blockMarksForDate builds the marks for one date and reports whether the date
// carries at least one TIED event. Both come from the same walk over `visible`.
func blockMarksForDate(cal *Calendar, visible []Event, in BlockProjectionInput, month, day int) ([]calblock.Mark, bool) {
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
		marks = append(marks, blockMarkFor(cal, e, isGM, evTied))
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
func blockMarkFor(cal *Calendar, e *Event, isGM, tied bool) calblock.Mark {
	key, color, icon := blockEventAxisKey(cal, e)
	return calblock.Mark{
		EventID:  e.ID,
		Title:    e.Name,
		Axis:     color,
		Pattern:  blockPatternFor(key),
		Glyph:    icon,
		Named:    strings.TrimSpace(e.Name) != "",
		Tied:     tied,
		Audience: blockAudienceFor(e, isGM),
	}
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
