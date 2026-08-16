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
	Slug string `json:"slug"`
	Name string `json:"name"`
	// Glyph and Axis are two of the locked triple's three channels.
	Glyph string `json:"glyph,omitempty"`
	Axis  string `json:"axis,omitempty"`
	// Pattern is the THIRD, and it is the greyscale identity channel — the one
	// that survives hue removal, and therefore the one the whole "every hue
	// pairs with a pattern or a glyph" law actually rests on.
	//
	// ADDED AT STAGE 8, FIX-FORWARD, and its absence was a real defect rather
	// than an omission: the module's type rail falls back to `p1` when a
	// category carries no pattern, so EVERY option in the rail drew the same
	// solid stroke and the pattern channel — which is the point of the channel
	// — carried no information at all. Two types were separable by hue alone,
	// which is the thing this arc's build law forbids in terms.
	//
	// IT IS DERIVED FROM THE SAME KEY THE BLOCK USES, not invented here.
	// blockPatternFor(slug) is what blockMarkFor already locks a mark's pattern
	// to (block_projection.go), so the pattern a type wears in the type rail is
	// the pattern its events wear in the grid and in the Ledger. A second
	// derivation would have been two identities for one category.
	Pattern string `json:"pattern,omitempty"`
}

// dayCardMember is one campaign member as the RESTRICTED-AUDIENCE roster prints
// them, and it is C-CALV4-EDITOR-R2b's one addition to this file ([ER-3] SIGNED).
//
// ── WHY IT RIDES THE PAGE PAYLOAD, AND WHY THAT IS NOT A BREACH OF [DC-1] ──
//
// The editor's Restricted card draws one row per member — hue swatch + locked
// pattern + the ringed two-letter mark + the name + the role + an allow/deny
// pair — and to draw it at all you need user ids AND display names.
// `visibility_rules` stores ids; cal_visibility.js's rulesToChips labels a chip
// with the RAW ID today because it has nothing better.
//
// [DC-1]'s payload law governs the CARD'S PER-EVENT FIELD SET: "the Ledger row's
// own field set and NOTHING more". The roster is neither an event nor a per-day
// field — it is EDITOR SEED DATA on the same wrapper, exactly like
// dayCardCategory, which rides here for the same shape of reason ([DC-8](c)
// option ii). The law is therefore re-stated rather than widened, and
// daycard_test.go asserts the wrapper's top-level key set EXPLICITLY BY NAME so
// a third seed cannot arrive by inference.
//
// ── OWNER-ONLY, BECAUSE RESTRICTED IS OWNER-ONLY ───────────────────────────
//
// PUT …/events/:eid/visibility is RequireRole(RoleOwner) (routes.go:182), so a
// Scribe never renders the Restricted card and MUST NOT RECEIVE THE NAMES.
// The gate is DayCardMount.CanRestrict — an EXPLICIT Owner fact, never
// CanDelete borrowed for the day: two Owner floors that coincide today are one
// refactor away from not, and the coincidence would be silent.
//
// ── IT COSTS NO NEW READ AND NO NEW ROUTE ──────────────────────────────────
//
// BenchScheduleReader.BenchRoster is already called on EVERY Bench render, for
// every viewer, ungated (benchRsvpResolve, bench.go) — Step 0 confirmed it is
// not behind the RSVP panel's own gate. This is a re-serialisation of data the
// page already rendered, which is the same argument [DC-1] Option B was signed
// on. No route moved; routes_snapshot.txt is untouched.
//
// THE IDENTITY PAIR IS THE RSVP PANEL'S OWN, from benchRsvpIdentity /
// benchRsvpInitials / benchRoleLabel, on the SAME roster index. Two surfaces
// that draw the same people must not draw them in two colours, and a second
// derivation here is exactly how that happens.
type dayCardMember struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Role is benchRoleLabel's output — the one shim printed role names go
	// through, so the later game-system terminology slice stays a one-file
	// change.
	Role string `json:"role"`
	// Initials is the ringed two-letter mark. It is a RING, not a filled disc:
	// the stills index measured near-white on a raw owner hue at 1.72:1 and
	// retired the filled form by name. The ink is the sheet's, not the hue's.
	Initials string `json:"initials"`
	// Axis and Pattern are the locked identity pair — hue NEVER travels alone,
	// so a viewer who cannot separate the hues separates the people by dash.
	Axis    string `json:"axis"`
	Pattern string `json:"pattern"`
}

// dayCardMembers projects the Bench roster for the audience list.
//
// It returns nil below the Owner floor — not an empty slice, so the key is
// omitted entirely rather than shipped as `"members":[]`, which would be a
// promise of a roster this viewer is simply not entitled to. Permission is
// ABSENCE, applied to a payload key.
func dayCardMembers(roster []BenchRosterMember, canRestrict bool) []dayCardMember {
	if !canRestrict || len(roster) == 0 {
		return nil
	}
	out := make([]dayCardMember, 0, len(roster))
	for i, m := range roster {
		id := strings.TrimSpace(m.UserID)
		if id == "" {
			continue
		}
		hue, pattern := benchRsvpIdentity(i)
		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = id
		}
		out = append(out, dayCardMember{
			ID:       id,
			Name:     name,
			Role:     benchRoleLabel(m.Role),
			Initials: benchRsvpInitials(name),
			Axis:     hue,
			Pattern:  pattern,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// dayCardCalendar is one Block's worth of days.
//
// LedgerDocked IS CARRIED, NOT RE-DERIVED. The `Open in the Ledger` door is
// emitted only when the Ledger is actually docked for this viewer, and the JS
// must not infer that from the DOM's absence because ABSENCE HAS TWO CAUSES: a
// host that never docked the zone, and a viewer who switched the layer off. A
// link to a column that is not on the page is the exact class of dishonesty
// this arc keeps killing, so the producer states the fact.
//
// IT IS THE NECESSARY HALF, NOT THE WHOLE CONDITION (calv4 fix R1, item 3). The
// door additionally requires the Ledger to be STACKED BELOW the month rather
// than docked beside it — a runtime rect, measured in the module
// (ledgerIsStacked), because the layout turns on a CONTAINER query over the
// Block's own width and the server does not know it. Docked, a day click has
// already selected that day in the column: the door would click a checked radio,
// scroll to something on screen, and close the card.
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
// THE WRAPPER CARRIES EXACTLY TWO TOP-LEVEL KEYS AND BOTH ARE ENUMERATED IN
// THE SUITE. `calendars` is the per-day event data [DC-1] governs; `members` is
// the Owner-only editor seed [ER-3] SIGNED added. A THIRD key is a decision,
// not a convenience, and TestDayCard_PayloadWrapperCarriesOnlyItsTwoSeeds
// fails on one.
type dayCardPayload struct {
	Calendars []dayCardCalendar `json:"calendars"`
	// Members is the restricted-audience roster ([ER-3]). Owner-only and
	// omitted entirely below that floor — see dayCardMember's header for why
	// this is editor seed data rather than a widening of the payload law.
	Members []dayCardMember `json:"members,omitempty"`
}

// ── the two key namespaces, mirrored ───────────────────────────────────────
//
// dayKey / intercalaryKey / dayOrdKey / intercalaryOrdKey are UNEXPORTED in
// internal/widgets/calendar_block (helpers.go:590-604, :913-917) and this slice
// opens NO file in that package — the Block's interior law is the bound this
// dispatch is fenced by, and exporting a helper to satisfy a consumer would be
// an edit to it. They are therefore mirrored here, byte-for-byte, and pinned
// against the RENDERED Block's own attributes rather than against these
// functions, so a drift in either direction fails.
//
// BOTH NAMESPACES ARE PINNED, and saying so is the point (DC-ICAL-2). The first
// cut of this comment claimed the mirror was guarded in either direction while
// only TestDayCard_KeysAgreeWithTheRenderedBlock existed — and that test walks
// ordinary days only, because the signed oracle fixture declares no
// is_intercalary month. So `slug-iN` / `iN` were minted by nobody's guard while
// a comment said otherwise, which is worse than an unguarded mirror: it is the
// sentence that stops the next hand from adding the test.
// TestDayCard_IntercalaryKeysAgreeWithTheRenderedBlock is the missing half, and
// it splices real intercalary months into the oracle fixture rather than
// asserting against a hand-built payload.

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
	// identical and the zip below cannot slip. That assumption is now CHECKED
	// rather than asserted: TestDayCard_IntercalaryDaysResolveTheirOWNMonth
	// zips two intercalary months of UNEQUAL length, which is the shape a
	// positional zip fails on. It is a zip rather than a
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
			Slug: slug,
			Name: strings.TrimSpace(ec.Name),
			// THE SAME KEY THE BLOCK LOCKS ITS MARKS TO. blockEventAxisKey
			// resolves an event's key to its category slug and blockMarkFor
			// takes blockPatternFor(key) — so this is the identical
			// derivation, and a type's rail in the editor wears the pattern
			// its events wear in the grid.
			Pattern: dayCardPattern(blockPatternFor(slug)),
			Glyph:   strings.TrimSpace(ec.Icon),
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

// buildDayCardCalendarFromWeek is the WEEK/DAY view's half of the same payload
// (C-CALV4-WEEKDAY-VIEWS).
//
// ── WHY THE PAYLOAD FOLLOWS THE VIEW ───────────────────────────────────────
//
// The card is opened from any element carrying `[data-day][data-day-ord]`
// inside a `[data-bench-block][data-calendar-id]`, and it looks the day up BY
// KEY. When the primary slot draws a week instead of a month, the keys on the
// page are the WEEK's — so a payload still keyed to the month would make every
// column a dead click, which is the exact "nothing happens" complaint the day
// card was built to answer.
//
// ── IT IS THE SAME SET, FROM THE SAME PASS ─────────────────────────────────
//
// [DC-2]'s agreement law says the card's set for a day is EXACTLY what the
// surface shows for that day. Here that is provable the same way it is for the
// month: these rows are re-serialised from the columns the surface just
// rendered — the single viewer-filtered pass WeekDayView performed — and not
// from a second read. AllDay and Timed are two views of ONE walk, so
// concatenating them cannot produce an event the column does not draw.
//
// ── AND IT CARRIES NO MORE THAN [DC-1] ALLOWS ──────────────────────────────
//
// dayCardEvents is the ONE mapper, reused verbatim: id, title, time, the
// (axis, pattern, glyph) triple, the gold rail and the audience label. No
// description, no recurrence, no visibility rules — the week view holds richer
// Event data than the month grid does (it reads hours off the row) and NONE of
// it reaches the payload.
func buildDayCardCalendarFromWeek(w BenchWeekView, cal *Calendar, canAuthor bool) dayCardCalendar {
	out := dayCardCalendar{
		CalendarID: w.CalendarID,
		Slug:       w.CalendarSlug,
		// FALSE, ALWAYS, AND IT IS A FACT RATHER THAN A DEFAULT. The `Open in
		// the Ledger` door is emitted only when a Ledger is docked for this
		// viewer, and the week/day surface docks none — it draws an hour axis
		// where the Block draws its four zones. A door to a column that is not
		// on the page is the dishonesty dayCardCalendar's own header refuses.
		LedgerDocked: false,
		Categories:   dayCardCategories(cal, canAuthor),
	}
	for _, d := range w.Days {
		events := make([]dayCardEvent, 0, d.Count)
		for _, e := range d.AllDay {
			events = append(events, dayCardEvents([]calblock.Mark{e.Mark})...)
		}
		for _, e := range d.Timed {
			events = append(events, dayCardEvents([]calblock.Mark{e.Mark})...)
		}
		out.Days = append(out.Days, dayCardDay{
			Key:     d.DayKey,
			Ord:     d.DayOrd,
			Day:     d.Day,
			Label:   dayCardWeekLabel(d),
			Weekday: d.Weekday,
			// THE COLUMN'S OWN COORDINATES, never the cursor's. A week can
			// straddle a month and a year, so an editor pre-filled from the
			// surface's month would write the event to the wrong date on every
			// column past the boundary — the same defect dayCardDay's Year/Month
			// comment already records for intercalary days, one unit larger.
			Year:   d.Year,
			Month:  d.Month,
			Events: events,
		})
	}
	return out
}

// dayCardWeekLabel is the card head's dated line for a week/day column. It
// prints the day, the month name and the year — the same three facts
// dayCardLabel prints, taken from the COLUMN rather than from a single rendered
// month, because these columns do not share one.
func dayCardWeekLabel(d BenchWeekDay) string {
	label := strconv.Itoa(d.Day)
	if name := strings.TrimSpace(d.MonthName); name != "" {
		label += " " + name
	}
	if d.Year != 0 {
		label += " " + strconv.Itoa(d.Year)
	}
	return label
}

// dayCardSeed is the VIEWER-LEVEL half of the payload: the two facts that
// decide what editor seed data this page carries, kept together so a call site
// cannot pass one and forget the other.
//
// CanAuthor is the Scribe floor (the type palette). CanRestrict is the Owner
// floor ([ER-3]) and it is EXPLICIT rather than inferred from CanAuthor or
// borrowed from CanDelete — the whole point of the ruling is that the two Owner
// gates are named separately.
type dayCardSeed struct {
	CanAuthor   bool
	CanRestrict bool
	Roster      []BenchRosterMember
}

// dayCardPayloadJSON serialises the surface's Blocks into the page attribute.
//
// It returns "" when there is nothing to carry, and the mount emits no
// attribute at all in that case — an empty payload attribute is a promise of a
// card that has no days to open on. THE ROSTER ALONE IS NOT SOMETHING TO CARRY:
// a page with no Block has no card, so no editor, so no audience list to seed.
//
// MARSHAL CANNOT FAIL HERE (every field is a string, bool, int or a slice of
// those), and a swallowed error would be a silent blank card, so the error is
// folded into the empty return rather than logged per render.
func dayCardPayloadJSON(seed dayCardSeed, sources ...dayCardSource) string {
	p := dayCardPayload{Members: dayCardMembers(seed.Roster, seed.CanRestrict)}
	for _, src := range sources {
		if src.Block == nil {
			continue
		}
		// THE VIEW DECIDES THE KEYS (C-CALV4-WEEKDAY-VIEWS). A Block drawing a
		// week/day surface put the WEEK's `data-day` keys on the page, so the
		// payload for that calendar must be the week's days or every column is a
		// dead click. One branch, at the one place the payload is assembled.
		if src.Block.Week != nil {
			p.Calendars = append(p.Calendars,
				buildDayCardCalendarFromWeek(*src.Block.Week, src.Calendar, seed.CanAuthor))
			continue
		}
		p.Calendars = append(p.Calendars, buildDayCardCalendar(src.Block.Data, src.Calendar, seed.CanAuthor))
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
