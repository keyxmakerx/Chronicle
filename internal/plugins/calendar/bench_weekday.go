// bench_weekday.go — C-CALV4-WEEKDAY-VIEWS: the v4 WEEK view and DAY view.
//
// ── WHAT THIS FILE IS ──────────────────────────────────────────────────────
//
// V2's shell carried the product's only week view and only day view
// (handler_v2.go's `case "week", "day"`). C-CALV4-V2SUNSET R2-4 retired the V1
// entry points at those views and pointed /week and /day at the Bench's MONTH,
// which was a live feature loss booked as this slice and named as a prerequisite
// of C-CALV4-SHELL-REMOVAL. This file is the replacement.
//
// ── THE SEAT: A QUERY PARAM ON THE BENCH, NOT A ROUTE ──────────────────────
//
// The Bench's whole cursor contract is `?y=` / `?m=` on the EXISTING
// GET /campaigns/:id/apps/calendar ([GR-1] signed it that way: no preference
// row, no session value, no fifth benchSectionKeys entry). This extends the
// SAME family with `?view=week|day` and `?d=<day>` and adds NO route, so
// internal/wire/routes_snapshot.txt is byte-identical and
// C-CALV4-SHELL-REMOVAL's pre-computed 727 → 722 arithmetic is untouched.
//
// The three alternatives and why each is worse are recorded in the dispatch;
// the short form: a sibling /apps/calendar/week is a snapshot regeneration plus
// a second navigation model; /schedule is a REAL-WORLD availability matrix
// (ISO dates, wall-clock hour columns) and declaring it the week view's
// successor would be a category error — the standing STOP-AND-FLAG (4) in
// .ai/todo.md asks exactly that and the answer is NO, in writing; and a Block
// LAYER key is the wrong subject, because layers change what the month DRAWS
// and apply instantly and silently, while a view is a different surface.
//
// ── ONE PASS, AND WHY IT IS ITS OWN PASS ───────────────────────────────────
//
// Every count and every chip on a v4 surface comes from ONE viewer-filtered
// pass (block_projection.go's header states the law and the reason). This
// surface obeys it: candidateEventsForDates reads the months the WINDOW touches,
// filterEventsByUser runs EXACTLY ONCE over that union, and every column, every
// all-day chip and every hour block below is derived from that single result.
//
// It is a DIFFERENT pass from the month Block's, and that is not a second
// oracle: a week can straddle two months (and two years), so its window is a
// different date range and the month's pass simply does not cover it. What the
// law forbids is two passes over the SAME set differing — which is why the day
// card's payload for this surface is re-serialised from THESE columns
// (dayCardSource.Week) rather than read a second time.
//
// ── TEN-DAY WEEKS ARE NATIVE. THERE IS NO LITERAL 7 ────────────────────────
//
// The column count is blockWeekLen(cal) — len(cal.Weekdays), with
// blockDefaultWeekLen as the ONE named fallback — and the nav step is the SAME
// number. V2 had four live 7-literals here and the worst of them was v2Step's
// `day += 7 * dir`, which drew ten columns and stepped seven days, so
// consecutive "weeks" on a ten-day calendar overlapped by three. Nothing in
// this file, its template or its stylesheet writes a 7.
//
// ── WHAT WAS NOT NEEDED: THE PINNED-STRUCT AMENDMENT ───────────────────────
//
// The scouting report expected numeric hours on calblock.Mark — a byte-pinned
// data.go amendment (a STOP-AND-FLAG). It is NOT taken, and the reason is that
// the hour axis is a PLUGIN surface, not a Block zone: this producer holds the
// Event beside the Mark, so it reads Event.StartHour/EndHour directly and hands
// calblock.Mark the ink it already knows how to make. The widget package draws
// no hour axis and therefore needs no hour field. data.go is untouched by this
// slice.
package calendar

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// --- the view keys ----------------------------------------------------------

// The three values `?view=` may take. The empty string is the MONTH and is the
// zero value on purpose: an un-navigated Bench, every existing bookmark and
// every link in the product that does not name a view all resolve to the
// surface they have always resolved to.
const (
	benchViewMonth = ""
	benchViewWeek  = "week"
	benchViewDay   = "day"
)

// normalizeBenchView clamps an arbitrary `?view=` to a supported key.
//
// UNKNOWN IS THE MONTH, never an error page. It is the same discipline
// normalizeCalendarSort applies to `?sort=` and the same one atoiOr applies to
// `?y=`: a junk cursor reads as "no cursor", because a 400 on a mistyped URL
// costs the reader their whole page for a control they can simply click again.
func normalizeBenchView(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case benchViewWeek:
		return benchViewWeek
	case benchViewDay:
		return benchViewDay
	default:
		return benchViewMonth
	}
}

// benchWeekDefaultHoursPerDay is the NAMED fallback for a calendar that
// declares no hours-per-day, exactly as blockDefaultWeekLen is the named
// fallback for a calendar that declares no weekdays.
//
// It is named rather than written as a literal 24 inside an expression for the
// identical reason block_geometry.go gives: the number must never be *assumed*.
// V2's hourRows buried the same value as a bare fallback and its week/day grids
// buried three more literals beside it.
const benchWeekDefaultHoursPerDay = 24

// benchWeekHourRows is the calendar's own day length in hours.
func benchWeekHourRows(cal *Calendar) int {
	if cal == nil || cal.HoursPerDay <= 0 {
		return benchWeekDefaultHoursPerDay
	}
	return cal.HoursPerDay
}

// --- the view model ---------------------------------------------------------

// BenchWeekEvent is one event as this surface draws it: the Block's own Mark
// (so the chip's hue, pattern, glyph, audience mark and formatted time are the
// SAME ones the month grid and the Ledger print) plus the hour geometry only
// this surface needs.
//
// THE MARK IS NOT REBUILT. It comes from blockMarkFor, the one mark producer,
// so an event cannot wear one identity in the month and another in the week.
// What is added here is numeric — the rows it occupies — and numbers are what
// the pinned calblock.Mark deliberately does not carry.
type BenchWeekEvent struct {
	Mark calblock.Mark
	// AllDay is true when the event carries no start hour. Untimed events land
	// in the strip above the grid rather than being dropped or parked at hour 0
	// — V2 split them the same way and the split is the honest one: an event
	// with no time does not have a time.
	AllDay bool
	// RowStart / RowSpan are 1-BASED hour rows within the day (row 1 is the
	// calendar's hour 0). Zero on an all-day event.
	RowStart int
	RowSpan  int
	// MultiDay marks an event whose stored [start, end] window covers more than
	// this one day. V2's week and day views dropped multi-day events ENTIRELY —
	// a three-day festival appeared nowhere, on either surface — and that was a
	// gap rather than a feature, so they are drawn here on every day they span,
	// which is what the month grid already does (blockEventSpansDate).
	MultiDay bool
	// TimeLabel is the event's own formatted range, or "" when it has none. It
	// is Event.FormatTimeRange, not a second formatter.
	TimeLabel string
}

// BenchWeekDay is one column of the week (and the whole body of the day view).
//
// IT CARRIES ITS OWN (Year, Month) AND THAT IS LOAD-BEARING. A week window can
// straddle a month boundary and a year boundary, so "the rendered month" is not
// a property of the surface — it is a property of each column. A column that
// inherited the cursor's month would file its events, and the editor it opens,
// under the wrong month.
type BenchWeekDay struct {
	Year  int // the column's own year
	Month int // 1-BASED, the column's own month
	Day   int
	// DayKey / DayOrd are the ANSWER keys (guard B4's `data-day`, and the day
	// card's `[data-day][data-day-ord]` trigger). See benchWeekDayKey for why
	// this surface mints a wider key than the month grid's `<slug>-<day>`.
	DayKey string
	DayOrd string
	// MonthName is printed in the column header so a week crossing a month
	// boundary NAMES BOTH MONTHS rather than silently renumbering.
	MonthName string
	Weekday   string
	// Header is the column's own line: "<Weekday> · <MonthName> <Day>", with the
	// weekday segment dropped when the calendar declares none.
	Header string
	// Label is the short dated form ("Mirtul 3") the day card's head prints.
	Label       string
	IsToday     bool
	IsRest      bool
	Intercalary bool
	// RealDate is the Gregorian date this in-world day falls on, ALREADY
	// FORMATTED, or "" when the calendar carries no anchor. Same field, same
	// producer (blockRealDateLabel) and same "empty means NOT KNOWN" contract as
	// DayCell.RealDate.
	RealDate string
	// Moons are the day's discs, from the same producer the month cells use.
	Moons []calblock.MoonDisc
	// Season / Era are the day's own context. V2's day view carried NONE of
	// this — no today marker, no rest-day tint, no celestial or era readout —
	// while the Bench the reader just left prints all of it, so a day view that
	// omitted it would be poorer than the surface it replaced.
	Season string
	Era    string
	EraHue string
	AllDay []BenchWeekEvent
	Timed  []BenchWeekEvent
	// Count is every viewer-visible event on this day, all-day and timed
	// together. It is len(AllDay)+len(Timed) by construction — the two lists
	// partition one walk — so it can never disagree with what is drawn.
	Count int
}

// BenchWeekMoon is one declared moon's reading for the DAY view's celestial
// line. It is projected from MonthGeometry.Almanac, which the Block already
// built for this month; nothing is re-derived.
type BenchWeekMoon struct {
	Name  string
	Phase string
	Illum int // percent, 0..100 — the Almanac's own fraction, rounded once
}

// BenchWeekView is the whole surface.
type BenchWeekView struct {
	// Kind is benchViewWeek or benchViewDay. It is carried rather than inferred
	// from len(Days): a one-day-week calendar is legal.
	Kind         string
	CalendarID   string
	CalendarSlug string
	// WeekLen is the COLUMN COUNT and it is the calendar's own week length.
	// It drives --week-len in the template; the stylesheet declares no count.
	WeekLen int
	// HourRows is the calendar's own day length. It drives --hour-rows.
	HourRows int
	// HourLabels is one label per row, already formatted for THIS calendar's
	// minute geometry (see benchWeekHourLabel).
	HourLabels []string
	// Heading names the span ("Mirtul 3 – 9, 1492 DR") with the epoch; SubLabel
	// is the era/season line beneath it.
	Heading  string
	SubLabel string
	// Fault is the honesty state, borrowed verbatim from the Block's contract:
	// when non-empty the surface prints the fault WHERE THE DATES WOULD GO and
	// draws no grid at all — not a zero, not a placeholder, not an em dash.
	Fault string
	Days  []BenchWeekDay
	// Moons is the DAY view's celestial line (empty in the week view, where the
	// per-column discs carry the same fact in less space).
	Moons []BenchWeekMoon
	// Switch is the Month | Week | Day control and Nav is the prev/today/next
	// trio, both computed server-side. See benchViewSwitch / benchNavForView.
	Switch BenchViewSwitch
}

// BenchViewSwitch is the Month | Week | Day control.
//
// THREE <a> ELEMENTS, exactly as [GR-2]'s nav trio is, and for the identical
// reasons: the hrefs are server-computed onto the SAME route, so the control
// needs no script (boot.js sets htmx.config.allowScriptTags = false, which
// DELETES a <script> inside a swapped fragment, and tools/check-page-scripts.sh
// is a whole-tree ratchet), and a URL is shareable, refreshable,
// back-buttonable and middle-clickable.
//
// GUARD B3: the control attribute is `data-view-pick`. `data-view` is literally
// one of the six <html> STATE_MARKERS in tools/check-calendar-v4-lints.sh, and
// reusing a state-marker noun on an interactive control is what made every
// click on the signed mockup re-navigate twice.
type BenchViewSwitch struct {
	// Live is false when there is nothing to switch between (a calendar with no
	// months resolves no dates, so a week of it is not a thing).
	Live      bool
	Active    string // benchViewMonth | benchViewWeek | benchViewDay
	MonthHref string
	WeekHref  string
	DayHref   string
}

// --- the cursor -------------------------------------------------------------

// benchWeekCursor resolves the requested (year, month, day) onto a real date.
//
// THE YEAR/MONTH HALF IS resolveView'S AND IS NOT DUPLICATED. resolveView is
// the sole clamp authority ([GR-1] forbids this slice from editing it and this
// slice does not): it clamps a bad month into a real one, keeps a navigated
// year when only the month is junk, and answers the calendar's own current
// month for a fully unset view. Only the DAY is clamped here, because
// resolveView has no day to clamp.
//
// THE DAY'S OWN DEFAULT IS THE CALENDAR'S CURRENT DAY, but only when the
// resolved month IS the calendar's current month in the calendar's current
// year. Defaulting to the current day of an arbitrary navigated month would
// answer "the 14th" in a month the reader is only browsing, which is the same
// class of small lie as a Today button that is not today.
func (s *BlockService) benchWeekCursor(cal *Calendar, v BlockDate) BlockDate {
	monthIdx, year := s.resolveView(cal, v)
	month := monthIdx + 1
	days := blockMonthDays(cal, monthIdx, year)
	day := v.Day
	if day < 1 {
		if year == cal.CurrentYear && month == cal.CurrentMonth {
			day = cal.CurrentDay
		} else {
			day = 1
		}
	}
	if day < 1 {
		day = 1
	}
	if days > 0 && day > days {
		day = days
	}
	return BlockDate{Year: year, Month: month, Day: day}
}

// benchWeekStepDate moves a date by n of the calendar's OWN days, rolling over
// month and year boundaries against cal.Months and the LEAP-AWARE MonthDays.
//
// IT IS GUARDED AT THE TOP AND THAT IS NOT DEFENSIVE PADDING. V2's
// stepDaysBackward has exactly this shape and NO guard: on a calendar with
// weekdays but zero months it does `month--` → `month = len(cal.Months)` → 0 →
// `cal.Months[-1]` → PANIC. addDaysSimple in the same package carries an
// explicit guard for precisely that case (handler_v2.go) and its sibling does
// not. The unguarded shape is not ported.
//
// Intercalary months are stepped THROUGH, not over: they are ordinary entries
// in cal.Months with is_intercalary set, so a week that runs into Midwinter
// draws Midwinter as a column instead of skipping a day.
func benchWeekStepDate(cal *Calendar, d BlockDate, n int) BlockDate {
	if cal == nil || len(cal.Months) == 0 {
		return d
	}
	mc := len(cal.Months)
	y, m, day := d.Year, d.Month, d.Day
	if m < 1 {
		m = 1
	}
	if m > mc {
		m = mc
	}
	for n > 0 {
		days := cal.MonthDays(m-1, y)
		if days <= 0 {
			days = 1
		}
		if day+n <= days {
			day += n
			break
		}
		n -= days - day + 1
		day = 1
		m++
		if m > mc {
			m = 1
			y++
		}
	}
	for n < 0 {
		if day+n >= 1 {
			day += n
			break
		}
		n += day
		m--
		if m < 1 {
			m = mc
			y--
		}
		day = cal.MonthDays(m-1, y)
		if day <= 0 {
			day = 1
		}
	}
	return BlockDate{Year: y, Month: m, Day: day}
}

// benchWeekWindow returns the calendar week CONTAINING the cursor, aligned to
// the calendar's own weekday cycle.
//
// ALIGNED, NOT CENTRED — and this is a deliberate departure from V2, which
// centred its window on the cursor (`start = cursor − floor(cols/2)`). Two
// reasons, both measured:
//
//  1. AGREEMENT. An aligned week is one row of the month grid the reader just
//     left, so a day sits in the same column on both surfaces. A centred window
//     puts the same day under a different weekday name depending on which day
//     you navigated from, which is indefensible on a product whose thesis is
//     that the week is the calendar's own.
//
//  2. THE STEP. Prev/next move by exactly one week and consecutive weeks
//     tile — no overlap, no gap. V2's centred window plus its hardcoded
//     `day += 7 * dir` overlapped by three days on every ten-day calendar.
//
// The alignment uses v2WeekdayIndexFor, the SHARED year-aware weekday index the
// month grid's own lead cell is computed from (block_geometry.go names it the
// deliberate sharing point), so the two cannot disagree.
func benchWeekWindow(cal *Calendar, cursor BlockDate, cols int) []BlockDate {
	if cal == nil || len(cal.Months) == 0 || cols < 1 {
		return nil
	}
	idx := v2WeekdayIndexFor(cal, cursor.Year, cursor.Month, cursor.Day)
	if idx < 0 {
		idx = 0
	}
	if idx >= cols {
		idx %= cols
	}
	start := benchWeekStepDate(cal, cursor, -idx)
	out := make([]BlockDate, 0, cols)
	cur := start
	for i := 0; i < cols; i++ {
		out = append(out, cur)
		cur = benchWeekStepDate(cal, cur, 1)
	}
	return out
}

// --- the ANSWER keys --------------------------------------------------------

// benchWeekDayKey is this surface's `data-day` key, and it is WIDER than the
// month grid's on purpose.
//
// The grid's dayKey is "<slug>-<day>" because a month grid holds each ordinal
// exactly ONCE. A week window does not: it can cross a month boundary, and on a
// calendar with one-day intercalary months (canonical Harptos has four) it can
// hold "day 1" twice inside ten columns. An ordinal is therefore not a key
// here, and a colliding key would silently make two columns open the same day
// card.
//
// IT STAYS IN THE `data-day` NAMESPACE, which is what matters: guard B4 keys on
// the attribute and calendar_daycard.js triggers on `[data-day][data-day-ord]`,
// so the shipped card opens on these columns with no JS change at all.
func benchWeekDayKey(slug string, d BlockDate) string {
	if slug == "" {
		slug = "cal"
	}
	return slug + "-" + strconv.Itoa(d.Year) + "-" + strconv.Itoa(d.Month) + "-" + strconv.Itoa(d.Day)
}

// benchWeekDayOrd is the LADDER key (`data-day-ord`). The day card reads its
// first character to decide whether a day is intercalary
// (calendar_daycard.js's isIntercalary), which is why the `i` prefix is kept
// rather than folded into the wider key.
func benchWeekDayOrd(intercalary bool, d BlockDate) string {
	s := strconv.Itoa(d.Year) + "-" + strconv.Itoa(d.Month) + "-" + strconv.Itoa(d.Day)
	if intercalary {
		return "i" + s
	}
	return s
}

// --- hour geometry ----------------------------------------------------------

// benchWeekHourLabel prints one row of the hour axis.
//
// IT RESPECTS THE CALENDAR'S OWN MINUTE GEOMETRY, which V2's fmtHourLabel did
// not: that helper hardcoded a two-digit hour and a literal ":00", so a
// calendar with 1000-minute hours printed a label that claimed a 60-minute one.
// Both fields are padded to the width THIS calendar's maxima need, so a
// 30-hour day prints "07:00" and a 1000-minute hour prints "07:000".
func benchWeekHourLabel(cal *Calendar, hour int) string {
	hw := benchWeekDigits(benchWeekHourRows(cal) - 1)
	mpw := benchWeekDefaultMinutes
	if cal != nil && cal.MinutesPerHour > 0 {
		mpw = cal.MinutesPerHour
	}
	mw := benchWeekDigits(mpw - 1)
	return fmt.Sprintf("%0*d:%0*d", hw, hour, mw, 0)
}

// benchWeekDefaultMinutes is the NAMED fallback minute count, on the same rule
// as benchWeekDefaultHoursPerDay: the number is never assumed inline.
const benchWeekDefaultMinutes = 60

// benchWeekDigits is how many decimal digits n needs, floored at two so an
// ordinary 24×60 calendar prints the familiar "07:00" rather than "7:0".
func benchWeekDigits(n int) int {
	w := 1
	for n >= 10 {
		n /= 10
		w++
	}
	if w < 2 {
		return 2
	}
	return w
}

// benchWeekHourSpan places one timed event on the hour axis of ONE day.
//
// V2's eventHourRange is the arithmetic this is measured against and it is
// EXTENDED IN ONE DIRECTION ONLY: V2 read *EndHour only when the event's end
// date equalled its start date and otherwise drew start..start+1, because V2's
// week and day views excluded multi-day events entirely and never had to place
// one. This surface draws them, so a spanned day resolves as:
//
//	the START day of a span  → its own start hour, running to the day's end
//	a MIDDLE day             → the whole day
//	the END day              → the day's start, running to its stored end hour
//	a single-day event       → start .. (stored end hour, else start+1)
//
// Everything is clipped to the calendar's own HoursPerDay and an end at or
// before its start is forced to exactly one row, which is V2's rule kept.
func benchWeekHourSpan(cal *Calendar, e *Event, d BlockDate, rows int) (start, span int) {
	if e == nil || e.StartHour == nil || rows < 1 {
		return 0, 0
	}
	onStart := e.Year == d.Year && e.Month == d.Month && e.Day == d.Day
	onEnd := true
	if e.EndYear != nil && e.EndMonth != nil && e.EndDay != nil {
		onEnd = *e.EndYear == d.Year && *e.EndMonth == d.Month && *e.EndDay == d.Day
	}

	s := 0
	if onStart {
		s = *e.StartHour
	}
	end := rows
	if onEnd {
		switch {
		case e.EndHour != nil:
			end = *e.EndHour
		case onStart:
			// A single-day event with no stored end hour occupies exactly one
			// row. Running it to the end of the day would be the surface
			// asserting a duration the operator never entered.
			end = s + 1
		}
	}

	if s < 0 {
		s = 0
	}
	if s > rows-1 {
		s = rows - 1
	}
	if end > rows {
		end = rows
	}
	if end <= s {
		end = s + 1
	}
	return s + 1, end - s
}

// --- the per-day event walk -------------------------------------------------

// benchWeekEventsForDate builds one day's event lists from the ALREADY-FILTERED
// candidate slice.
//
// IT REUSES THE MONTH GRID'S THREE PRIMITIVES — blockEventSpansDate for
// multi-day membership, blockEventTied for the tie flag and blockMarkFor for the
// mark itself — so the ink, the audience mark and the formatted time are the
// grid's, not a second derivation.
//
// IT IS NOT blockMarksForDate ITSELF, and the reason is one line of that
// function: it takes the year from BlockProjectionInput.Year, so every date it
// tests belongs to the rendered month's year. A week window can cross a YEAR
// boundary, so this walk passes each column's OWN (year, month, day). Calling
// the month helper here would have silently tested the wrong year for the
// columns on the far side of a new year — visible only as "the last three days
// of the week are always empty".
//
// THE SPLIT IS `StartHour == nil`, which is V2's own allDayEventsForDay
// predicate. An event with no time does not have a time, and parking it at hour
// 0 would put a "some time today" item in the 00:00 slot as though it were a
// midnight appointment.
func benchWeekEventsForDate(cal *Calendar, visible []Event, viewer BlockViewer, tied map[string]bool,
	d BlockDate, rows int, times blockMarkTimeFormatter, isGM bool) (allDay, timed []BenchWeekEvent) {
	in := BlockProjectionInput{Calendar: cal, Viewer: viewer, TiedEventIDs: tied, Year: d.Year}
	for i := range visible {
		e := &visible[i]
		if !e.OccursOn(cal, d.Year, d.Month, d.Day) && !blockEventSpansDate(cal, e, d.Year, d.Month, d.Day) {
			continue
		}
		ev := BenchWeekEvent{
			Mark:      blockMarkFor(cal, e, isGM, blockEventTied(e, in), times),
			MultiDay:  e.IsMultiDay(),
			TimeLabel: e.FormatTimeRange(),
		}
		if e.StartHour == nil {
			ev.AllDay = true
			allDay = append(allDay, ev)
			continue
		}
		ev.RowStart, ev.RowSpan = benchWeekHourSpan(cal, e, d, rows)
		timed = append(timed, ev)
	}
	return allDay, timed
}

// --- the cross-month read ---------------------------------------------------

// candidateEventsForDates reads every (year, month) the given dates touch as ONE
// candidate slice holding each event exactly once.
//
// IT IS candidateEvents WIDENED, NOT A NEW REPOSITORY METHOD. The month spine's
// own read (candidateEvents) is fixed to one month plus its intercalary
// siblings, which is right for a month grid and cannot serve a week that
// straddles a boundary. The service already exposes ListEventsForDateRange, but
// that one is SAME-YEAR ONLY and so cannot serve a week across a new year
// either.
//
// THE DEDUPE IS THE SAME DEDUPE AND IT LIVES AT THE SOURCE. Every
// ListEventsForMonth call returns every recurring candidate regardless of the
// month asked for (the C-CAL-EDITOR-EXPANSION PR2 widening — OccursOn decides
// placement in Go), so a two-month union yields two copies of each recurring
// row. Downstream assumes each event appears once: an undeduped union draws
// duplicate chips on every column and doubles the day's count while the counts
// stay internally consistent, which is exactly the failure candidateEvents's own
// header warns about.
func (s *BlockService) candidateEventsForDates(ctx context.Context, cal *Calendar, dates []BlockDate, role int) ([]Event, error) {
	type ym struct{ y, m int }
	seenMonth := make(map[ym]bool, len(dates))
	var out []Event
	seen := make(map[string]bool)
	for _, d := range dates {
		k := ym{d.Year, d.Month}
		if seenMonth[k] {
			continue
		}
		seenMonth[k] = true
		evs, err := s.repo.ListEventsForMonth(ctx, cal.ID, d.Year, d.Month, role)
		if err != nil {
			return nil, fmt.Errorf("week events for %d-%d: %w", d.Year, d.Month, err)
		}
		for i := range evs {
			if seen[evs[i].ID] {
				continue
			}
			seen[evs[i].ID] = true
			out = append(out, evs[i])
		}
	}
	return out, nil
}

// --- the producer -----------------------------------------------------------

// BenchWeekRequest is one week/day render.
type BenchWeekRequest struct {
	CalendarID string
	Viewer     BlockViewer
	// Kind is benchViewWeek or benchViewDay.
	Kind string
	// Cursor is the requested date, UNVALIDATED — `?y=&m=&d=` straight off the
	// URL, exactly as [GR-1] passes the month cursor. benchWeekCursor clamps it
	// through resolveView.
	Cursor BlockDate
	// MoonCap bounds the per-day discs, mirroring the Block's own ceiling so a
	// day column and a month cell draw the same number of moons.
	MoonCap int
}

// WeekDayView builds the week or day surface.
//
// THE ORDER IS THE SPINE'S OWN ORDER, for the same reasons Block states: resolve
// the calendar through requireVisibleCalendar (so a calendar the viewer may not
// see is as unanswerable as a nonexistent one, with the SAME not-found shape),
// read the window's candidate events, filter ONCE, resolve ties in one batched
// query over the FILTERED set — TiedEventIDsForEntity applies no visibility
// filter of its own and relies on that ordering — then derive every column from
// the single result.
func (s *BlockService) WeekDayView(ctx context.Context, req BenchWeekRequest) (BenchWeekView, error) {
	cal, err := s.requireVisibleCalendar(ctx, req.CalendarID, req.Viewer)
	if err != nil {
		return BenchWeekView{}, err
	}

	kind := benchViewWeek
	if req.Kind == benchViewDay {
		kind = benchViewDay
	}
	cols := blockWeekLen(cal)
	if kind == benchViewDay {
		cols = 1
	}
	rows := benchWeekHourRows(cal)

	out := BenchWeekView{
		Kind:         kind,
		CalendarID:   cal.ID,
		CalendarSlug: blockCalendarSlug(cal),
		// WeekLen is the calendar's week length even in the DAY view, where only
		// one column is drawn: the stylesheet's column track reads --week-len and
		// the day view sets its own span, so the number stays the calendar's own
		// fact rather than becoming a render artefact.
		WeekLen:  blockWeekLen(cal),
		HourRows: rows,
	}
	out.HourLabels = make([]string, 0, rows)
	for h := 0; h < rows; h++ {
		out.HourLabels = append(out.HourLabels, benchWeekHourLabel(cal, h))
	}

	// THE FAULT PATH IS THE BLOCK'S, VERBATIM. A calendar that cannot resolve a
	// date prints the fault where the dates would go and draws nothing — the
	// signed .calrow.warnrow behaviour — rather than a grid of zeroes.
	if _, fault := blockDateLine(cal); fault != "" {
		out.Fault = fault
		return out, nil
	}

	cursor := s.benchWeekCursor(cal, req.Cursor)
	var window []BlockDate
	if kind == benchViewDay {
		window = []BlockDate{cursor}
	} else {
		window = benchWeekWindow(cal, cursor, cols)
	}
	if len(window) == 0 {
		out.Fault = "Needs months — 0 months defined, dates cannot resolve"
		return out, nil
	}

	events, err := s.candidateEventsForDates(ctx, cal, window, req.Viewer.Role)
	if err != nil {
		return BenchWeekView{}, err
	}
	// ── THE ONE PASS. `events` is compacted IN PLACE by filterEventsByUser
	// (service.go's `filtered := events[:0]`), so it is dead from here and every
	// column below reads `visible` and nothing else.
	visible := filterEventsByUser(events, req.Viewer.Identity())

	tied, err := s.tiedEventIDs(ctx, req.Viewer.HostEntity, visible)
	if err != nil {
		return BenchWeekView{}, err
	}

	out.Days = benchWeekColumns(cal, visible, req, window, tied, rows)
	out.Heading = benchWeekHeading(cal, out.Days, kind)
	out.SubLabel = benchWeekSubLabel(cal, out.Days)
	if kind == benchViewDay {
		out.Moons = benchWeekMoonLine(cal, cursor, req.MoonCap)
	}
	return out, nil
}

// benchWeekColumns derives every column from the single filtered pass.
//
// A FREE FUNCTION, NOT A METHOD, and deliberately: it performs no query and
// holds no service state, so it is directly testable against a hand-built
// calendar and event slice with no repository at all.
func benchWeekColumns(cal *Calendar, visible []Event, req BenchWeekRequest,
	window []BlockDate, tied map[string]bool, rows int) []BenchWeekDay {
	slug := blockCalendarSlug(cal)
	// The time formatter is resolved ONCE for the whole surface, exactly as
	// blockDecorateCells resolves it once per Block: time.LoadLocation reads the
	// zoneinfo database and a ten-column week of dense days would otherwise
	// re-open it per mark.
	times := blockMarkTimes(cal, req.Viewer)
	isGM := permissions.CanSeeDmOnly(req.Viewer.Role)

	out := make([]BenchWeekDay, 0, len(window))
	for _, d := range window {
		mi := d.Month - 1
		col := BenchWeekDay{
			Year:      d.Year,
			Month:     d.Month,
			Day:       d.Day,
			MonthName: benchWeekMonthName(cal, mi),
			DayKey:    benchWeekDayKey(slug, d),
			RealDate:  blockRealDateLabel(cal, d.Year, d.Month, d.Day),
			IsToday:   d.Year == cal.CurrentYear && d.Month == cal.CurrentMonth && d.Day == cal.CurrentDay,
		}
		if mi >= 0 && mi < len(cal.Months) {
			col.Intercalary = cal.Months[mi].IsIntercalary
		}
		col.DayOrd = benchWeekDayOrd(col.Intercalary, d)
		col.Weekday, col.IsRest = benchWeekWeekdayFor(cal, d)
		col.Header = benchWeekColumnHeader(col)
		col.Label = strings.TrimSpace(fmt.Sprintf("%s %d", col.MonthName, col.Day))
		if sn := cal.SeasonForDate(d.Month, d.Day); sn != nil {
			col.Season = sn.Name
		}
		if e := cal.EraForYear(d.Year); e != nil {
			col.Era = e.Name
			col.EraHue = strings.TrimSpace(e.Color)
		}
		col.Moons = benchWeekMoonDiscs(cal, d, req.MoonCap)
		col.AllDay, col.Timed = benchWeekEventsForDate(cal, visible, req.Viewer, tied, d, rows, times, isGM)
		col.Count = len(col.AllDay) + len(col.Timed)
		out = append(out, col)
	}
	return out
}

// benchWeekMoonDiscs reuses the month grid's disc producer for ONE day.
//
// It pays one Calendar.AbsoluteDay per column rather than per cell, which is
// the cost monthBaseAbsoluteDay's header bounds: a week is at most a handful of
// columns, where a month is dozens of cells times three moons.
func benchWeekMoonDiscs(cal *Calendar, d BlockDate, moonCap int) []calblock.MoonDisc {
	if cal == nil || len(cal.Moons) == 0 {
		return nil
	}
	return moonDiscsForDay(cal.Moons, cal.AbsoluteDay(d.Year, d.Month, d.Day), 1, moonCap)
}

// benchWeekMoonLine is the DAY view's celestial readout, projected from the
// Almanac register the month geometry already builds.
//
// V2's day view carried NO celestial context at all, while the Ledger's day
// panel on the Bench prints every declared moon's phase — so a v4 day view that
// omitted it would have been poorer than the surface the reader just left. It
// is UNCAPPED, because the Almanac register is uncapped by [S5]: the grid's
// three-moon ceiling is only legitimate because the register carries every
// declared body.
func benchWeekMoonLine(cal *Calendar, d BlockDate, moonCap int) []BenchWeekMoon {
	if cal == nil || len(cal.Moons) == 0 || d.Month < 1 || d.Month > len(cal.Months) {
		return nil
	}
	mi := d.Month - 1
	reg := blockAlmanacRegister(cal.Moons, monthBaseAbsoluteDay(cal, mi, d.Year),
		blockMonthDays(cal, mi, d.Year), d.Day, moonCap)
	out := make([]BenchWeekMoon, 0, len(reg))
	for _, m := range reg {
		for _, ad := range m.Days {
			if ad.Day != d.Day {
				continue
			}
			out = append(out, BenchWeekMoon{
				Name:  m.Name,
				Phase: ad.Phase,
				Illum: int(ad.Illum*100 + 0.5),
			})
			break
		}
	}
	return out
}

// benchWeekWeekdayFor names the column's weekday and reports whether it is a
// rest day, through the SHARED year-aware index. Both facts come from one
// lookup so a column cannot be tinted as a rest day under the wrong name.
func benchWeekWeekdayFor(cal *Calendar, d BlockDate) (string, bool) {
	if cal == nil || len(cal.Weekdays) == 0 {
		return "", false
	}
	idx := v2WeekdayIndexFor(cal, d.Year, d.Month, d.Day)
	if idx < 0 || idx >= len(cal.Weekdays) {
		return "", false
	}
	w := cal.Weekdays[idx]
	return w.Name, w.IsRestDay
}

func benchWeekMonthName(cal *Calendar, monthIdx int) string {
	if cal == nil || monthIdx < 0 || monthIdx >= len(cal.Months) {
		return ""
	}
	return cal.Months[monthIdx].Name
}

// benchWeekColumnHeader is V2's weekDayLabel: "<Weekday> · <MonthName> <Day>",
// dropping the weekday segment when the calendar declares none. A week crossing
// a month boundary therefore prints BOTH real month names, which is the whole
// reason the month name is in a column header at all.
func benchWeekColumnHeader(c BenchWeekDay) string {
	right := strings.TrimSpace(fmt.Sprintf("%s %d", c.MonthName, c.Day))
	if strings.TrimSpace(c.Weekday) == "" {
		return right
	}
	return c.Weekday + " · " + right
}

// benchWeekHeading names the span, with the epoch.
//
// The three forms, and the third is one V2 could not produce because its own
// stepper stopped at year end:
//
//	same month  "Mirtul 3 – 9, 1492 DR"
//	cross month "Mirtul 28 – Kythorn 4, 1492 DR"
//	cross year  "Nightal 28, 1492 – Hammer 4, 1493 DR"
//	day view    "Mirtul 3, 1492 DR"
func benchWeekHeading(cal *Calendar, days []BenchWeekDay, kind string) string {
	if len(days) == 0 {
		return ""
	}
	epoch := benchWeekEpoch(cal)
	a, b := days[0], days[len(days)-1]
	if kind == benchViewDay || len(days) == 1 {
		return strings.TrimSpace(fmt.Sprintf("%s %d, %d%s", a.MonthName, a.Day, a.Year, epoch))
	}
	switch {
	case a.Year != b.Year:
		return strings.TrimSpace(fmt.Sprintf("%s %d, %d – %s %d, %d%s",
			a.MonthName, a.Day, a.Year, b.MonthName, b.Day, b.Year, epoch))
	case a.Month != b.Month:
		return strings.TrimSpace(fmt.Sprintf("%s %d – %s %d, %d%s",
			a.MonthName, a.Day, b.MonthName, b.Day, a.Year, epoch))
	default:
		return strings.TrimSpace(fmt.Sprintf("%s %d – %d, %d%s",
			a.MonthName, a.Day, b.Day, a.Year, epoch))
	}
}

// benchWeekEpoch is the calendar's reckoning suffix (" DR"), or "" when it
// declares none. Absent rather than invented — the same rule
// blockSeasonEraLabels applies to a calendar with no seasons.
func benchWeekEpoch(cal *Calendar) string {
	if cal == nil || cal.EpochName == nil {
		return ""
	}
	if n := strings.TrimSpace(*cal.EpochName); n != "" {
		return " " + n
	}
	return ""
}

// benchWeekSubLabel is the era/season line beneath the heading. It names the
// era and season of the window's FIRST day and, when the window crosses into
// another era or season, says so rather than silently naming only one.
func benchWeekSubLabel(cal *Calendar, days []BenchWeekDay) string {
	if len(days) == 0 {
		return ""
	}
	var parts []string
	if era := benchWeekSpan(days, func(c BenchWeekDay) string { return c.Era }); era != "" {
		parts = append(parts, era)
	}
	if season := benchWeekSpan(days, func(c BenchWeekDay) string { return c.Season }); season != "" {
		parts = append(parts, season)
	}
	return strings.Join(parts, " · ")
}

// benchWeekSpan folds one per-day label over the window: the single value when
// every day agrees, "A → B" when the window crosses a boundary, and "" when no
// day carries one.
func benchWeekSpan(days []BenchWeekDay, get func(BenchWeekDay) string) string {
	first, last := "", ""
	for _, d := range days {
		v := strings.TrimSpace(get(d))
		if v == "" {
			continue
		}
		if first == "" {
			first = v
		}
		last = v
	}
	if first == "" || first == last {
		return first
	}
	return first + " → " + last
}

// --- the controls -----------------------------------------------------------

// benchViewHref builds one Bench URL in the SAME param family the month cursor
// already uses. The month form deliberately carries NO `view=` and NO `d=` — a
// URL that names the surface it is not is noise, and the empty view key is the
// month by definition.
func benchViewHref(campaignID, view string, d BlockDate) string {
	base := fmt.Sprintf("/campaigns/%s/apps/calendar", campaignID)
	var q []string
	if view != benchViewMonth {
		q = append(q, "view="+view)
	}
	// Year 0 is resolveView's "unset" SENTINEL (block_service.go), which [GR-1]
	// forbids this slice from editing, so year 0 is not addressable by the
	// cursor at all and is omitted rather than linked to.
	if d.Year != 0 {
		q = append(q, "y="+strconv.Itoa(d.Year))
	}
	if d.Month > 0 {
		q = append(q, "m="+strconv.Itoa(d.Month))
	}
	// The day rides only on the two surfaces that have a day. Carrying it into
	// the month href would make `Month` mean "this month, with a day answered",
	// which is a different page from the one the pill promises.
	if view != benchViewMonth && d.Day > 0 {
		q = append(q, "d="+strconv.Itoa(d.Day))
	}
	if len(q) == 0 {
		return base
	}
	return base + "?" + strings.Join(q, "&")
}

// benchViewSwitch builds the Month | Week | Day control for one calendar.
//
// THE CURSOR IS CARRIED ACROSS THE SWITCH. Clicking `Week` from a month the GM
// navigated to lands on a week INSIDE that month, not on today — the same
// promise the month cursor makes, extended one unit down. Clicking `Month` from
// a week lands on the month the week's cursor day belongs to.
func benchViewSwitch(cal *Calendar, campaignID, active string, cursor BlockDate) BenchViewSwitch {
	if cal == nil || len(cal.Months) == 0 {
		return BenchViewSwitch{}
	}
	return BenchViewSwitch{
		Live:      true,
		Active:    active,
		MonthHref: benchViewHref(campaignID, benchViewMonth, BlockDate{Year: cursor.Year, Month: cursor.Month}),
		WeekHref:  benchViewHref(campaignID, benchViewWeek, cursor),
		DayHref:   benchViewHref(campaignID, benchViewDay, cursor),
	}
}

// benchNavForView is benchNav's week/day sibling: the prev / Today / next trio
// stepping in the ACTIVE UNIT.
//
// THE STEP IS THE CALENDAR'S OWN WEEK LENGTH, NOT SEVEN. V2's v2Step did
// `day += 7 * dir` for the week while its grid drew len(cal.Weekdays) columns,
// so on a ten-day calendar the grid showed ten days and Next moved seven —
// consecutive "weeks" overlapped by three days, every time, and nothing said so.
// Here the step and the column count are THE SAME EXPRESSION, so they cannot
// disagree.
//
// `Today` IS ALWAYS RENDERED AND NEVER DISABLED, exactly as [GR-2] rules for the
// month: it links back to the view with no cursor, which resolves to the
// calendar's own stored in-world date — not the OS clock. A control that
// vanishes when you are already there teaches the reader it is missing rather
// than satisfied.
func benchNavForView(cal *Calendar, campaignID, view string, cursor BlockDate) BenchNav {
	if cal == nil || len(cal.Months) == 0 || view == benchViewMonth {
		return BenchNav{}
	}
	step := 1
	if view == benchViewWeek {
		step = blockWeekLen(cal)
	}
	prev := benchWeekStepDate(cal, cursor, -step)
	next := benchWeekStepDate(cal, cursor, step)

	nav := BenchNav{
		Live:      true,
		Unit:      view,
		Month:     benchWeekMonthName(cal, cursor.Month-1),
		Year:      cursor.Year,
		TodayHref: benchViewHref(campaignID, view, BlockDate{}),
		NextHref:  benchViewHref(campaignID, view, next),
	}
	// Year 0 is resolveView's unset sentinel and cannot be linked to — the same
	// single direction BenchNav's header already names as the one that can be
	// absent. A calendar standing in the first week of year 1 therefore has no
	// expressible previous week, and no link is honest where a link that
	// silently lands on today is not.
	if prev.Year != 0 {
		nav.PrevHref = benchViewHref(campaignID, view, prev)
	}
	return nav
}

// benchWeekSeat fills the primary Block's view switch and, when a view was
// asked for, its week/day surface and its unit-stepping nav.
//
// IT MUTATES THE BenchBlock IN PLACE rather than returning a second value,
// because the switch has to be populated in EVERY view — including the month,
// where there is no week surface to return — and a builder that returned
// (view, switch, nav) would give three call sites the chance to wire two of
// them.
//
// A FAILED WEEK READ LEAVES THE MONTH STANDING. The spine's error path is the
// visibility gate's own not-found shape, which is deliberately
// indistinguishable from a hidden calendar (C-CALV4-SEAM-P5 stage 9), so this
// host must not branch on which — it simply does not seat a week, and the
// reader gets the month Block that was already built. A degraded view is a
// worse page; a blank one is a broken product.
func (h *Handler) benchWeekSeat(ctx context.Context, spine *BlockService, cal *Calendar,
	viewer BlockViewer, in benchInput, block *BenchBlock) {
	if spine == nil || cal == nil || block == nil {
		return
	}
	cursor := spine.benchWeekCursor(cal, in.View)
	view := normalizeBenchView(in.ViewKind)
	block.Switch = benchViewSwitch(cal, in.Campaign.ID, view, cursor)
	if view == benchViewMonth {
		return
	}
	w, err := spine.WeekDayView(ctx, BenchWeekRequest{
		CalendarID: cal.ID,
		Viewer:     viewer,
		Kind:       view,
		Cursor:     in.View,
		MoonCap:    benchMoonCap,
	})
	if err != nil {
		return
	}
	w.Switch = block.Switch
	block.Week = &w
	// The trio now steps in the ACTIVE UNIT. It REPLACES the month trio rather
	// than sitting beside it: two nav rows on one Block would be two answers to
	// "what does Next do", and the month is one click away on the switch.
	block.Nav = benchNavForView(cal, in.Campaign.ID, view, cursor)
}

// benchWeekColumnAria labels one column header for a screen reader: the day,
// then whether it is today, a rest day or a festival day, then how many events
// it carries.
//
// THE COUNT IS THE VIEWER'S OWN. BenchWeekDay.Count is len(AllDay)+len(Timed)
// off the single filtered pass, so it can only ever announce events this reader
// can already see — the same construction that makes the Block's tied/whole
// pair non-differenceable. It is stated rather than left to the visual chips
// because a column whose events are three coloured blocks announces nothing at
// all otherwise.
func benchWeekColumnAria(d BenchWeekDay) string {
	parts := []string{d.Header}
	if d.IsToday {
		parts = append(parts, "today")
	}
	if d.IsRest {
		parts = append(parts, "rest day")
	}
	if d.Intercalary {
		parts = append(parts, "festival day")
	}
	switch d.Count {
	case 0:
		parts = append(parts, "no events")
	case 1:
		parts = append(parts, "1 event")
	default:
		parts = append(parts, strconv.Itoa(d.Count)+" events")
	}
	return strings.Join(parts, ", ")
}
