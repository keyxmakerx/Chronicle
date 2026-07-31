package calendar

// daycard.go — C-CALV4-DAYCARD (calendar-v4 round 2, slice R2-2a): the day
// card's PAYLOAD, built once per render at the PRODUCER.
//
// WHY THIS FILE EXISTS. The operator's complaint was two sentences and both
// were mechanically true: clicking a day "just selects the date" (it sets the
// visually-hidden `data-day-pick` radio, and the generated ANSWER ladder
// filters the docked Ledger — the one sanctioned content change in the Block),
// and "nothing happens" (calendar-v4 has no way to create or edit an event at
// all). The card is the POINTER-FIRST answer on top of the CSS-only one, and
// this file is the half of it that decides WHAT the card may know.
//
// ── THE AGREEMENT LAW ([DC-2] SIGNED, dispatch §2) ─────────────────────────
//
//	For any day, the set of events the card lists is EXACTLY the set of .lrow
//	elements the ladder leaves visible for that day. Not a superset, not a
//	subset, not "close enough."
//
// It is provable here and nowhere else, because the card and the Ledger read
// ONE source: this builder walks the SAME calblock.BlockData the Block renders
// from — the single viewer-filtered pass block_projection.go performs — and
// never a second repository read and never a second filter. A card built from a
// second source that showed one more event than the Ledger would be a
// permission leak wearing a UI change's clothes. block_count_oracle_test.go
// joins the assertion to the count oracle (GM / Nissa / Bryn) rather than
// forking a second harness.
//
// ── THE PAYLOAD LAW ([DC-1] SIGNED, Option B) ──────────────────────────────
//
// The payload carries the LEDGER ROW's own field set and NOTHING more: event
// id, the day's two keys, title, time, the (axis, pattern, glyph) triple, the
// dm_only gold-rail flag and the audience label. That is ledger.templ's
// ledgerRow verbatim, minus the meta line the card does not print.
//
//	NO description. NO description_html. NO visibility_rules. NO recurrence.
//
// Those are the EDITOR's fields; they are the leak-sensitive ones; and they
// arrive over the editor's own route under the editor's own role floor. A
// payload that grew a description body would put every event's prose into every
// viewer's DOM for a card that never displays it. dayCardEvent has no field for
// any of them, deliberately, and daycard_payload_test.go asserts the shipped
// JSON key set is exactly this one.
//
// ── WHY A PAGE-EMITTED PAYLOAD AND NOT A ROUTE ─────────────────────────────
//
// Reading the rendered Ledger DOM (option A) is empty exactly when the card is
// most needed — a viewer who has switched the `ledger` layer off gets no Ledger
// rows AND no day radio at all (instrument.templ's dayPick is gated on
// ledgerDocked), which is the harder half of the operator's complaint. A
// per-day fragment route (option C) doubles the arc's most leak-sensitive
// surface and makes the card's contents wait on a round trip the register's
// 200ms open cannot. A page attribute is the shipped house precedent
// (event_grid.js:137 reads JSON.parse(root.dataset.calV2Events || '[]')) and it
// adds NO authorisation surface: it is a re-serialisation of data this viewer's
// page already rendered.

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// dayCardEvent is one row of the card, and it is ledgerRow's field set.
//
// Every field here is printed by internal/widgets/calendar_block/ledger.templ's
// ledgerRow (:155-190) for the same event. There is no field here the Ledger
// does not already print to this viewer, which is what makes the two surfaces
// non-differenceable: a card that carried one extra fact would be an oracle for
// it. `omitempty` mirrors the row's own DROP-RATHER-THAN-PRINT-EMPTY rule — a
// typeless event emits no glyph and an untimed one emits no time, in both
// surfaces, rather than an empty element.
type dayCardEvent struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Time  string `json:"time,omitempty"`
	// Axis is ALREADY NORMALISED to a token (see dayCardAxis): a bare value
	// never reaches a style attribute, here or in the widget.
	Axis    string `json:"axis"`
	Pattern string `json:"pattern"`
	Glyph   string `json:"glyph,omitempty"`
	// Gold is the dm_only GOLD RAIL + `GM` badge, and it splits exactly the way
	// ledgerShowsGoldRail splits: Audience non-nil AND NOT Restricted.
	// Audience.Restricted is the DISCRIMINATOR between the two signed GM marks,
	// not a synonym for "hidden" — a restricted-audience row gets the audience
	// chip alone. A card that drew the rail on both would be the identical
	// defect one layer down.
	Gold bool `json:"gold,omitempty"`
	// Audience is the chip's label ("GM only" / "Restricted"). It is nil for a
	// player at the producer, so this is "" for a player by construction:
	// permission is ABSENCE, not a greyed placeholder.
	Audience string `json:"audience,omitempty"`
}

// dayCardDay is one selectable day and its events, keyed BOTH ways.
//
// Key is the ANSWER key (dayKey / intercalaryKey — the `data-day` namespace
// guard B4 pins) and Ord is the LADDER key (`data-day-ord`). The card is opened
// from a cell that carries both, and the `Open in the Ledger` door needs Ord to
// find the day's radio, so carrying one and deriving the other would put the
// widget's two key namespaces in a second place.
type dayCardDay struct {
	Key     string `json:"key"`
	Ord     string `json:"ord"`
	Day     int    `json:"day"`
	Label   string `json:"label"`
	Weekday string `json:"weekday,omitempty"`
	// Year / Month are the day's REAL calendar coordinates — the ones
	// POST/PUT .../events take — and they are NOT derivable from Day.
	//
	// An intercalary day is the reason. Midwinter 1 is not Deepwinter 1: it
	// lives in its own is_intercalary Month that hangs off the rendered one,
	// and its Day is an ordinal WITHIN that month. An editor that pre-filled
	// the rendered month's index for it would create the event on the wrong
	// date, silently, on the calendars that have festival days — which is
	// most fantasy calendars. The producer resolves the coordinates through
	// blockIntercalaryMonths, which block_geometry.go names as the SINGLE
	// definition of that adjacency rule.
	//
	// Month is 1-BASED, matching the API's month number rather than the
	// geometry's 0-based index.
	Year   int            `json:"year"`
	Month  int            `json:"month"`
	Events []dayCardEvent `json:"events"`
}

// dayCardCategory is one entry of the event-type palette.
//
// IT RIDES THE PAGE PAYLOAD BECAUSE THE ROUTE IS OWNER-ONLY ([DC-8](c),
// resolved to option ii). `GET /calendars/:calId/event-categories` sits behind
// RequireRole(RoleOwner) (routes.go:131), so the palette a SCRIBE needs to pick
// a type on a new event is out of their reach. The two ways out were widening
// that GET's floor or answering from the producer; the arc's standing
// preference is not to widen an auth surface when a producer can answer, so
// this is the producer answering. No route moved and routes_snapshot.txt is
// untouched by it.
//
// IT IS SCRIBE+ ONLY. A player has no editor and therefore no type picker, so
// carrying the palette to them would be payload with no consumer.
type dayCardCategory struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Glyph string `json:"glyph,omitempty"`
	Axis  string `json:"axis,omitempty"`
}

// dayCardCalendar is one Block's worth of days.
//
// LedgerDocked IS CARRIED, NOT RE-DERIVED. The `Open in the Ledger` door is
// emitted only when the Ledger is actually docked for this viewer, and the JS
// must not infer that from the DOM's absence because ABSENCE HAS TWO CAUSES: a
// host that never docked the zone, and a viewer who switched the layer off. A
// link to a column that is not on the page is the exact class of dishonesty
// this arc keeps killing, so the producer states the fact.
type dayCardCalendar struct {
	CalendarID   string            `json:"id"`
	Slug         string            `json:"slug"`
	LedgerDocked bool              `json:"ledgerDocked"`
	Categories   []dayCardCategory `json:"categories,omitempty"`
	Days         []dayCardDay      `json:"days"`
}

// dayCardPayload is the whole page attribute: every Block on the surface.
//
// ONE CARD, N BLOCKS. The Bench stacks a primary Block and a real-world Block,
// and a card per Block would be N popovers, N listeners and N payload copies.
// The card is a page-level singleton positioned per click, so the payload is
// keyed by calendar id and the module picks the entry off the clicked cell's
// own `[data-bench-block][data-calendar-id]` ancestor.
type dayCardPayload struct {
	Calendars []dayCardCalendar `json:"calendars"`
}

// ── the two key namespaces, mirrored ───────────────────────────────────────
//
// dayKey / intercalaryKey / dayOrdKey / intercalaryOrdKey are UNEXPORTED in
// internal/widgets/calendar_block (helpers.go:590-604, :913-917) and this slice
// opens NO file in that package — the Block's interior law is the bound this
// dispatch is fenced by, and exporting a helper to satisfy a consumer would be
// an edit to it. They are therefore mirrored here, byte-for-byte, and
// daycard_test.go pins the mirror against the RENDERED Block's own attributes
// rather than against these functions, so a drift in either direction fails.

func dayCardKey(slug string, day int) string {
	if slug == "" {
		slug = "cal"
	}
	return slug + "-" + strconv.Itoa(day)
}

func dayCardIntercalaryKey(slug string, day int) string {
	if slug == "" {
		slug = "cal"
	}
	return slug + "-i" + strconv.Itoa(day)
}

func dayCardOrd(day int) string { return strconv.Itoa(day) }

func dayCardIntercalaryOrd(day int) string { return "i" + strconv.Itoa(day) }

// ── the style channels, normalised at the producer ─────────────────────────

var (
	dayCardVarRef = regexp.MustCompile(`^var\(--[a-zA-Z0-9_-]+\)$`)
	dayCardHex    = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)
	dayCardOklch  = regexp.MustCompile(`^oklch\([0-9a-zA-Z.%/ +-]+\)$`)
	dayCardPat    = regexp.MustCompile(`^p[1-8]$`)
)

// dayCardAxis mirrors the widget's axisToken (helpers.go:56-64): a bare value
// NEVER reaches a style attribute. It matters more here than there, because the
// card's rows are built in JS and the value crosses a JSON boundary before it
// is written into a custom property — normalising at the producer means the
// module never has to be trusted with a sanitiser of its own.
func dayCardAxis(v string) string {
	v = strings.TrimSpace(v)
	if dayCardVarRef.MatchString(v) || dayCardHex.MatchString(v) || dayCardOklch.MatchString(v) {
		return v
	}
	return calblock.AxisFallback
}

// dayCardPattern mirrors patternClass (helpers.go:102-107). p1..p8 is a CLOSED
// set — the greyscale identity channel — and anything else is p1 rather than a
// class name the sheet has never heard of.
func dayCardPattern(p string) string {
	p = strings.TrimSpace(p)
	if dayCardPat.MatchString(p) {
		return p
	}
	return "p1"
}

// dayCardLedgerDocked mirrors ledgerDocked (helpers.go:979-981): the layer
// registry says yes AND the host has not hidden the zone. Both conditions, in
// one place, because the two causes of an absent Ledger must stay separable.
func dayCardLedgerDocked(d calblock.BlockData) bool {
	for _, k := range d.Layers.Enabled {
		if k == "ledger" {
			return !d.Ledger.Hidden
		}
	}
	return false
}

// buildDayCardCalendar projects ONE Block's already-viewer-filtered BlockData
// into the card's per-calendar payload entry.
//
// IT WALKS THE SAME CELLS THE GRID DREW. DayCell.Marks is the FULL
// viewer-visible list for the day (MoreCount is overlapping, never additive —
// data.go says so at length), so the card lists what the Ledger lists, in the
// Ledger's own ordinal order: ordinary days ascending, then the intercalary
// days, which is newLedgerView's own walk (helpers.go:1149-1180).
//
// EVERY DAY WITH A CELL GETS AN ENTRY, INCLUDING AN EMPTY ONE. The card has a
// real empty state ("No events on this day.") and a card that silently refused
// to open on a quiet day would teach the operator the click is dead again — the
// exact complaint this slice answers. Days past the ANSWER ladder's bound are
// included too: the ladder cannot reach them (no radio is emitted) but the
// POINTER can, which is the card being the only answer there.
func buildDayCardCalendar(d calblock.BlockData, cal *Calendar, canAuthor bool) dayCardCalendar {
	out := dayCardCalendar{
		CalendarID:   d.CalendarID,
		Slug:         d.CalendarSlug,
		LedgerDocked: dayCardLedgerDocked(d),
		Categories:   dayCardCategories(cal, canAuthor),
	}

	// One pass over the grid, indexed by day so lead/trail cells (Day == 0)
	// and duplicated geometry cannot produce two entries for one date.
	cells := make(map[int]calblock.DayCell, d.Month.Days)
	order := make([]int, 0, d.Month.Days)
	for _, r := range d.Month.Rows {
		for _, c := range r.Cells {
			if c.Day <= 0 {
				continue
			}
			if _, seen := cells[c.Day]; !seen {
				order = append(order, c.Day)
			}
			cells[c.Day] = c
		}
	}
	sortAscending(order)

	for _, day := range order {
		c := cells[day]
		out.Days = append(out.Days, dayCardDay{
			Key:     dayCardKey(d.CalendarSlug, day),
			Ord:     dayCardOrd(day),
			Day:     day,
			Label:   dayCardLabel(d, day),
			Weekday: dayCardWeekday(d.Month, c),
			Year:    d.Month.Year,
			Month:   d.Month.Index + 1,
			Events:  dayCardEvents(c.Marks),
		})
	}

	// The intercalary days, with their OWN month resolved. The walk mirrors
	// blockIntercalary's exactly — every following is_intercalary Month, in
	// order, each day 1..MonthDays — so the two lists are positionally
	// identical and the zip below cannot slip. It is a zip rather than a
	// re-derivation because block_geometry.go's own comment says
	// blockIntercalaryMonths is the SINGLE definition of that adjacency rule
	// and "two independent copies of which months hang off this one is exactly
	// how a row and its events end up disagreeing".
	for i, ic := range d.Month.Intercalary {
		year, month := dayCardIntercalaryCoords(cal, d.Month.Index, d.Month.Year, i)
		out.Days = append(out.Days, dayCardDay{
			Key: dayCardIntercalaryKey(d.CalendarSlug, ic.Day),
			Ord: dayCardIntercalaryOrd(ic.Day),
			Day: ic.Day,
			// An intercalary day is NOT "day N of the month" — Midwinter 1 is
			// not Deepwinter 1, which is why the widget keeps two namespaces at
			// all. Its own name is the honest label.
			Label:  dayCardIntercalaryLabel(ic),
			Year:   year,
			Month:  month,
			Events: dayCardEvents(ic.Marks),
		})
	}
	return out
}

// dayCardIntercalaryCoords resolves the (year, 1-based month) of the nth
// intercalary day hanging off monthIdx.
//
// It returns month 0 when the calendar cannot be walked — a degraded spine, or
// a geometry the adjacency rule no longer explains. THAT IS THE HONEST ANSWER,
// not a fallback to the rendered month: an editor pre-filled with a month the
// day does not live in would write the event to the wrong date, and a zero
// month is a value the module can refuse to open on.
func dayCardIntercalaryCoords(cal *Calendar, monthIdx, year, nth int) (int, int) {
	if cal == nil {
		return year, 0
	}
	seen := 0
	for _, i := range blockIntercalaryMonths(cal, monthIdx) {
		days := cal.MonthDays(i, year)
		if nth < seen+days {
			return year, i + 1
		}
		seen += days
	}
	return year, 0
}

// dayCardCategories is the event-type palette, for an authoring viewer only.
//
// THE HUE IS NORMALISED AND THE GLYPH IS NOT INVENTED. A category with no icon
// emits none, and the card's row draws the pattern alone — every hue pairs with
// a pattern or a glyph, which is the rule the type rail hangs on, and the
// pattern channel is always there.
func dayCardCategories(cal *Calendar, canAuthor bool) []dayCardCategory {
	if cal == nil || !canAuthor || len(cal.EventCategories) == 0 {
		return nil
	}
	out := make([]dayCardCategory, 0, len(cal.EventCategories))
	for _, ec := range cal.EventCategories {
		slug := strings.TrimSpace(ec.Slug)
		if slug == "" {
			continue
		}
		cat := dayCardCategory{
			Slug:  slug,
			Name:  strings.TrimSpace(ec.Name),
			Glyph: strings.TrimSpace(ec.Icon),
		}
		if hue := strings.TrimSpace(ec.Color); hue != "" {
			cat.Axis = dayCardAxis(hue)
		}
		if cat.Name == "" {
			cat.Name = slug
		}
		out = append(out, cat)
	}
	return out
}

// dayCardLabel is the card head's dated line: "12 Deepwinter 1523".
//
// The YEAR is the month geometry's OWN resolved year (MonthGeometry.Year is
// leap-aware and already resolved), not a second date read. A calendar that
// cannot resolve a date at all carries BlockData.Fault and renders no date
// element — that fault is the Block's to print, and the card does not restate
// it: the day's ordinal and the month's name are true regardless.
func dayCardLabel(d calblock.BlockData, day int) string {
	label := strconv.Itoa(day)
	if name := strings.TrimSpace(d.Month.Name); name != "" {
		label += " " + name
	}
	if d.Month.Year != 0 {
		label += " " + strconv.Itoa(d.Month.Year)
	}
	return label
}

// dayCardIntercalaryLabel prints the festival day's own name, falling back to
// its ordinal when the calendar declared none.
func dayCardIntercalaryLabel(ic calblock.IntercalaryDay) string {
	if n := strings.TrimSpace(ic.Name); n != "" {
		return n
	}
	return strconv.Itoa(ic.Day)
}

// dayCardWeekday names the day's weekday from the CELL's own column.
//
// THERE IS NO LITERAL WEEK LENGTH ANYWHERE IN THIS DERIVATION. DayCell.Col is
// the 1-based column the producer already placed the day in, and Weekdays is
// the calendar's own header row — ten-day weeks are native and a `% 7` here
// would be the exact defect css_contract_test.go:365 forbids one layer up.
// Empty when the calendar declares no weekdays, in which case the card's head
// simply drops the segment rather than printing a dangling separator.
func dayCardWeekday(m calblock.MonthGeometry, c calblock.DayCell) string {
	if c.Col < 1 || c.Col > len(m.Weekdays) {
		return ""
	}
	return strings.TrimSpace(m.Weekdays[c.Col-1].Name)
}

// dayCardEvents maps marks to rows, one for one, in the cell's own order.
func dayCardEvents(marks []calblock.Mark) []dayCardEvent {
	out := make([]dayCardEvent, 0, len(marks))
	for _, m := range marks {
		row := dayCardEvent{
			ID:      m.EventID,
			Title:   m.Title,
			Time:    m.Time,
			Axis:    dayCardAxis(m.Axis),
			Pattern: dayCardPattern(m.Pattern),
			Glyph:   m.Glyph,
		}
		if m.Audience != nil {
			row.Audience = m.Audience.Label
			// ledgerShowsGoldRail (helpers.go:1258-1260), mirrored exactly.
			row.Gold = !m.Audience.Restricted
		}
		out = append(out, row)
	}
	return out
}

// sortAscending is an insertion sort over the handful of day ordinals a month
// carries. It exists so this file needs no import of sort for a slice whose
// length is bounded by a month.
func sortAscending(xs []int) {
	for i := 1; i < len(xs); i++ {
		v := xs[i]
		j := i - 1
		for j >= 0 && xs[j] > v {
			xs[j+1] = xs[j]
			j--
		}
		xs[j+1] = v
	}
}

// dayCardSource pairs a rendered Block with the Calendar it was projected from.
//
// The CALENDAR is needed for two facts BlockData deliberately does not carry:
// the event-type palette ([DC-8](c) option ii) and the intercalary months'
// adjacency. Both are producer-side chrome rather than event data, so neither
// touches the payload law that governs dayCardEvent.
type dayCardSource struct {
	Block    *BenchBlock
	Calendar *Calendar
}

// dayCardPayloadJSON serialises the surface's Blocks into the page attribute.
//
// It returns "" when there is nothing to carry, and the mount emits no
// attribute at all in that case — an empty payload attribute is a promise of a
// card that has no days to open on.
//
// MARSHAL CANNOT FAIL HERE (every field is a string, bool, int or a slice of
// those), and a swallowed error would be a silent blank card, so the error is
// folded into the empty return rather than logged per render.
func dayCardPayloadJSON(canAuthor bool, sources ...dayCardSource) string {
	p := dayCardPayload{}
	for _, src := range sources {
		if src.Block == nil {
			continue
		}
		p.Calendars = append(p.Calendars, buildDayCardCalendar(src.Block.Data, src.Calendar, canAuthor))
	}
	if len(p.Calendars) == 0 {
		return ""
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return string(raw)
}
