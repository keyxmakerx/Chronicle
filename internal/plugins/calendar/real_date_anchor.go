// real_date_anchor.go — the one bridge between a campaign's in-world calendar
// and the Gregorian axis everything schedulable is stored on.
//
// ── WHY A BRIDGE IS NEEDED AT ALL ──────────────────────────────────────────
//
// Chronicle keeps availability on a REAL axis and always has:
// `member_availability` is keyed to a Gregorian `day_of_week` and
// `availability_exceptions` to a Gregorian `on_date` (sessions/migrations/002),
// both minute-accurate and DST-correct. The calendar Block renders FANTASY
// days. Nothing joined the two, so on a fantasy calendar the availability strip
// had nothing to draw from — and the ruling has been, throughout this arc, that
// a surface which cannot answer says so rather than inventing a figure.
//
// `epoch_name` is a LABEL, not a date. `tracks_real_time` / `real_time_zone`
// (migration 012) apply only to `reallife` mode and are about deriving the
// current date from the wall clock, not about mapping arbitrary days. So there
// was no candidate to reuse and this is genuinely new.
//
// ── THE WHOLE IDEA ─────────────────────────────────────────────────────────
//
// One stored pair per calendar: an in-world date, and the real date it equals.
// Every other day follows by counting, because `Calendar.AbsoluteDay` already
// turns any (year, month, day) into an exact day index over the calendar's own
// month lengths and leap rule. Two subtractions and an `AddDate`:
//
//	realDate(d)  = anchorReal + (AbsoluteDay(d)      - AbsoluteDay(anchor)) days
//	inWorld(t)   = fromAbsolute(AbsoluteDay(anchor) + (t - anchorReal) days)
//
// ── WHAT IS DELIBERATELY NOT HERE ──────────────────────────────────────────
//
// NO TIME OF DAY. The mapping is day-granular. Availability windows carry their
// own minutes in their own IANA zone and are resolved against a real date by
// internal/timeutil, which is the code that already knows about DST; a
// half-second opinion from the calendar plugin would be a second, worse one.
//
// NO GUESSED EPOCH. An un-anchored calendar returns ok=false everywhere. There
// is no defensible default for someone else's world, and a plausible-looking
// wrong date is the failure mode this whole arc has been correcting.
//
// NO WEEKDAY MAPPING. A ten-day fantasy week does not correspond to a Gregorian
// one and this file does not pretend otherwise. What a caller gets is a real
// DATE; if it then wants a Gregorian weekday it takes `.Weekday()` on that,
// which is the real world's answer to a real world's question.
package calendar

import (
	"fmt"
	"time"
)

// RealDateAnchor is the anchor as ONE value, for the writers.
//
// The Calendar struct carries the four fields as separate nullable pointers
// because that is the shape the row has. Every WRITE path takes this struct
// instead, so "three of the four" is unrepresentable at the boundary rather
// than something a validator has to catch after the fact — which is the
// difference between a rule and a hope.
type RealDateAnchor struct {
	// Year, Month, Day are the IN-WORLD date, in this calendar's own terms.
	// Month is 1-based, matching Calendar.CurrentMonth and AbsoluteDay.
	Year, Month, Day int
	// RealDate is the Gregorian date that in-world day equals. Time-of-day and
	// zone are discarded on write — see this file's header on why.
	RealDate time.Time
}

// realDateAnchorOf reads the anchor back off a Calendar, or nil when it carries
// none. The inverse of the four-pointer storage shape.
func realDateAnchorOf(c *Calendar) *RealDateAnchor {
	if !c.HasRealAnchor() {
		return nil
	}
	return &RealDateAnchor{
		Year: *c.AnchorYear, Month: *c.AnchorMonth, Day: *c.AnchorDay,
		RealDate: dateOnlyUTC(*c.AnchorRealDate),
	}
}

// Validate reports why an anchor cannot be stored against this calendar, or nil
// if it can.
//
// IT IS CHECKED AGAINST THE CALENDAR, NOT IN THE ABSTRACT. "Month 14" is a
// perfectly good integer and a nonsense anchor on a twelve-month calendar; day
// 31 is fine in Hammer and not in a 30-day month. An anchor that names a day
// the calendar does not have would map every other day to a real date that is
// off by however many days the phantom is out, silently — the arithmetic never
// fails, it just answers wrongly. So the boundary is where this is caught.
func (a *RealDateAnchor) Validate(c *Calendar) error {
	switch {
	case a == nil:
		return fmt.Errorf("no anchor given")
	case c == nil || len(c.Months) == 0:
		return fmt.Errorf("this calendar declares no months, so no in-world date can be anchored")
	case c.YearLength() == 0:
		return fmt.Errorf("every month on this calendar declares 0 days, so no in-world date can be anchored")
	case a.Year < 0:
		return fmt.Errorf("year %d is before this calendar's reckoning begins", a.Year)
	case a.Month < 1 || a.Month > len(c.Months):
		return fmt.Errorf("month %d does not exist — this calendar has %d", a.Month, len(c.Months))
	case a.RealDate.IsZero():
		return fmt.Errorf("no real-world date given")
	}
	if md := c.MonthDays(a.Month-1, a.Year); a.Day < 1 || a.Day > md {
		return fmt.Errorf("%s %d does not exist in year %d — that month has %d days",
			c.Months[a.Month-1].Name, a.Day, a.Year, md)
	}
	return nil
}

// anchorYearSearchCap bounds the year search in FromAbsoluteDay.
//
// The search is a correction loop around an arithmetic estimate, so it normally
// runs zero or one times. The cap exists because the estimate divides by
// YearLength(), which an owner can edit: a calendar whose months were rewritten
// to a very different total between the anchor being stored and this call can
// start the loop far from the answer. A bounded loop that reports failure beats
// an unbounded one that hangs a page render, and this is called per rendered
// day.
const anchorYearSearchCap = 4096

// HasRealAnchor reports whether this calendar can map its days to real dates.
//
// ALL FOUR FIELDS OR NONE. A partial anchor is not a weaker anchor, it is an
// unanswerable one — an in-world date with no real date to equal, or the
// reverse, maps nothing. The service refuses to write a partial one; this is
// the read-side statement of the same rule, so a row hand-edited in the
// database degrades to "not anchored" rather than to a wrong answer.
//
// A calendar with no months cannot be anchored either, because AbsoluteDay has
// no structure to count over and every date would collapse onto the same day.
func (c *Calendar) HasRealAnchor() bool {
	return c != nil &&
		c.AnchorYear != nil && c.AnchorMonth != nil && c.AnchorDay != nil &&
		c.AnchorRealDate != nil &&
		len(c.Months) > 0 && c.YearLength() > 0
}

// anchorAbsoluteDay is the anchor's own day index, recomputed on every call
// rather than stored.
//
// THAT RECOMPUTATION IS THE POINT, and it is why migration 018 stores the named
// date instead of a day count. If the owner later edits the structure — adds a
// month, fixes a month's length — a stored count would hold the COUNT fixed and
// silently slide the in-world date the anchor names. Recomputing holds the
// NAMED DAY fixed, which is what "the campaign began on Marpenoth 14" means.
func (c *Calendar) anchorAbsoluteDay() int {
	return c.AbsoluteDay(*c.AnchorYear, *c.AnchorMonth, *c.AnchorDay)
}

// RealDateFor returns the Gregorian date an in-world date falls on.
//
// The returned time is midnight UTC on that date, which is a DATE carrier and
// not an instant — callers comparing it to `availability_exceptions.on_date`
// (also a bare DATE) are comparing like with like. Anything that needs a real
// instant must combine this date with a wall-clock and a zone through
// internal/timeutil; doing it here would bake a zone the calendar does not own.
//
// ok=false means this calendar carries no anchor. It is never a wrong date.
func (c *Calendar) RealDateFor(year, month, day int) (time.Time, bool) {
	if !c.HasRealAnchor() {
		return time.Time{}, false
	}
	delta := c.AbsoluteDay(year, month, day) - c.anchorAbsoluteDay()
	return c.AnchorRealDate.UTC().AddDate(0, 0, delta), true
}

// InWorldDateFor is the inverse: which in-world date a real date falls on.
//
// It is the direction that answers "what is today, in-world" and "which day of
// the month does our next session land on", and it is the one the settings
// preview reads back so an owner can see their anchor's consequence rather than
// trust the arithmetic.
func (c *Calendar) InWorldDateFor(t time.Time) (year, month, day int, ok bool) {
	if !c.HasRealAnchor() {
		return 0, 0, 0, false
	}
	// Both sides normalised to midnight UTC before subtracting, so a caller
	// handing in a zoned instant mid-afternoon does not lose or gain a day to
	// truncation. This is a DATE difference, not a duration.
	from := dateOnlyUTC(*c.AnchorRealDate)
	to := dateOnlyUTC(t)
	delta := int(to.Sub(from).Hours() / 24)
	return c.FromAbsoluteDay(c.anchorAbsoluteDay() + delta)
}

// FromAbsoluteDay turns a day index back into (year, month, day) — the inverse
// of AbsoluteDay, over the same month lengths and the same leap rule.
//
// It is exported because the anchor is not its only caller in waiting: moon
// phases, season boundaries and era spans all already compute FORWARD to an
// absolute day, and every "which day is index N" question they cannot currently
// ask is this function.
//
// THE YEAR IS ESTIMATED THEN CORRECTED, not searched from zero. AbsoluteDay's
// full-years term is monotonic in `year`, so a divide by the mean year length
// lands within a year or two and the correction loops run at most a couple of
// times — but they are BOUNDED anyway (anchorYearSearchCap), because this runs
// once per rendered day and a structure edit can move the estimate.
//
// ok=false means the index cannot be resolved against this calendar's
// structure: no months, a zero-length year, or a search that did not converge.
// Callers must not substitute a fallback date for it.
func (c *Calendar) FromAbsoluteDay(abs int) (year, month, day int, ok bool) {
	if c == nil || len(c.Months) == 0 || c.YearLength() <= 0 {
		return 0, 0, 0, false
	}
	// startOfYear(y) is the absolute day of y's first day.
	startOfYear := func(y int) int { return c.AbsoluteDay(y, 1, 1) }

	year = abs / c.YearLength()
	if year < 0 {
		year = 0
	}
	steps := 0
	for startOfYear(year) > abs && year > 0 {
		year--
		if steps++; steps > anchorYearSearchCap {
			return 0, 0, 0, false
		}
	}
	for startOfYear(year+1) <= abs {
		year++
		if steps++; steps > anchorYearSearchCap {
			return 0, 0, 0, false
		}
	}
	// AbsoluteDay(y,1,1) = fullYears(y) + 1, so the day's 1-based offset into
	// the year is abs - fullYears(y) = abs - startOfYear(year) + 1.
	rem := abs - startOfYear(year) + 1
	if rem < 1 {
		// Only reachable for an index before year 0, which this calendar's
		// arithmetic does not model. Reported, never approximated.
		return 0, 0, 0, false
	}
	for i := 0; i < len(c.Months); i++ {
		md := c.MonthDays(i, year)
		if rem <= md {
			return year, i + 1, rem, true
		}
		rem -= md
	}
	// Past the last month: the year is shorter than the correction loop
	// believed, which means the structure changed under a stored index.
	return 0, 0, 0, false
}

// dateOnlyUTC strips a timestamp to midnight UTC on its own calendar date.
//
// TAKEN IN THE TIME'S OWN LOCATION FIRST. `t.UTC().Date()` would move a
// late-evening instant in a negative-offset zone onto the NEXT day before
// truncating, so a GM in America/Chicago asking "what is today, in-world" after
// 19:00 would be answered with tomorrow. The date a human means is the date in
// their own zone.
func dateOnlyUTC(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
