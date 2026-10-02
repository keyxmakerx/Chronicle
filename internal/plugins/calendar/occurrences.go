// Package calendar - occurrences.go works out the dates a repeating event
// lands on within a range: weekly/monthly/yearly events through
// Event.OccursOn, rule events through their RecurrenceRule, and both with
// their "this one only" skips and moves applied.
//
// Every day is read on the calendar's own day counter (absDayIndex, the one
// the grid, weekdays and moon phases use) and walked month by month through
// MonthDays, so leap days, intercalary months and Each-month-starts-a-new-
// week all come from the same place the grid gets them, and a rule can
// never drift from what the grid shows.
package calendar

import (
	"math"
	"sort"
	"time"
)

// Occurrence is one date a repeating event lands on. Skipped and MovedFrom
// are filled only for a viewer who may edit the event; anyone else simply
// never sees a skipped date, and sees a moved one at its new date.
type Occurrence struct {
	Year      int      `json:"year"`
	Month     int      `json:"month"`
	Day       int      `json:"day"`
	Skipped   bool     `json:"skipped,omitempty"`
	MovedFrom *DayDate `json:"moved_from,omitempty"`
}

// Override actions for one occurrence of a repeating event.
const (
	OverrideSkip = "skip"
	OverrideMove = "move"
)

// OccurrenceOverride is a stored "this one only" change: the occurrence the
// event's own repeat put on Year/Month/Day is skipped, or moved to New*.
type OccurrenceOverride struct {
	EventID   string    `json:"event_id"`
	Year      int       `json:"year"`
	Month     int       `json:"month"`
	Day       int       `json:"day"`
	Action    string    `json:"action"`
	NewYear   *int      `json:"new_year,omitempty"`
	NewMonth  *int      `json:"new_month,omitempty"`
	NewDay    *int      `json:"new_day,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// date is the occurrence the override applies to.
func (o OccurrenceOverride) date() DayDate { return DayDate{Year: o.Year, Month: o.Month, Day: o.Day} }

// target is a move's new date; ok is false for a skip or a half-set row.
func (o OccurrenceOverride) target() (DayDate, bool) {
	if o.Action != OverrideMove || o.NewYear == nil || o.NewMonth == nil || o.NewDay == nil {
		return DayDate{}, false
	}
	return DayDate{Year: *o.NewYear, Month: *o.NewMonth, Day: *o.NewDay}, true
}

// OccurrenceOverrideInput is a request to skip, or move, one occurrence.
// Year/Month/Day are the new date and are used only for a move.
type OccurrenceOverrideInput struct {
	Action string
	Year   int
	Month  int
	Day    int
}

// before reports whether a sorts strictly before b.
func (a DayDate) before(b DayDate) bool {
	return dateLess(a.Year, a.Month, a.Day, b.Year, b.Month, b.Day)
}

// inRange reports whether d lies in [from, to].
func (d DayDate) inRange(from, to DayDate) bool { return !d.before(from) && !to.before(d) }

// Scan bounds. A rule is scanned day by day, so how far one request may walk
// is capped: ruleScanYears of the calendar's own years, held between the two
// absolute bounds so neither a tiny nor a vast year length escapes it.
const (
	ruleScanYears   = 5
	ruleScanMinDays = 400
	ruleScanMaxDays = 4000
)

// ruleScanCapDays is the most days one expansion of one event may walk.
func ruleScanCapDays(cal *Calendar) int {
	year := cal.YearLength() + cal.leapExtraDays()
	if cal.UsesRealTime() {
		year = 366
	}
	days := ruleScanYears * year
	if days < ruleScanMinDays {
		days = ruleScanMinDays
	}
	if days > ruleScanMaxDays {
		days = ruleScanMaxDays
	}
	return days
}

// dayCursor walks consecutive calendar days, skipping months that have no
// days this year (a leap-day month in a common year). abs is the day's
// absDayIndex, re-read at each month start so it stays exactly what the grid
// computes even across the counter's own seams.
type dayCursor struct {
	cal          *Calendar
	y, m, d, abs int
	mdays        int
	ok           bool
}

// maxEmptyMonthHops bounds the search for a month with days, so a calendar
// whose months have none can never spin.
func (c *dayCursor) maxEmptyMonthHops() int {
	leap := c.cal.LeapYearEvery
	if leap < 1 {
		leap = 1
	}
	return min(len(c.cal.Months)*(leap+1), 100000)
}

// newDayCursor starts at d. A date the calendar does not have that year (a
// leap day in a common year) starts on the next day it does have.
func newDayCursor(cal *Calendar, d DayDate) dayCursor {
	c := dayCursor{cal: cal, y: d.Year, m: d.Month, d: d.Day, ok: len(cal.Months) > 0}
	if !c.ok {
		return c
	}
	if c.m < 1 {
		c.m, c.d = 1, 1
	}
	if c.m > len(cal.Months) {
		c.m = len(cal.Months)
		c.toNextMonth()
		return c
	}
	if c.d < 1 {
		c.d = 1
	}
	c.mdays = cal.MonthDays(c.m-1, c.y)
	if c.d > c.mdays {
		c.toNextMonth()
		return c
	}
	c.abs = cal.absDayIndex(c.y, c.m, c.d)
	return c
}

func (c *dayCursor) toNextMonth() {
	for i := 0; i < c.maxEmptyMonthHops(); i++ {
		c.m++
		if c.m > len(c.cal.Months) {
			c.m = 1
			c.y++
		}
		c.mdays = c.cal.MonthDays(c.m-1, c.y)
		if c.mdays > 0 {
			c.d = 1
			c.abs = c.cal.absDayIndex(c.y, c.m, 1)
			return
		}
	}
	c.ok = false
}

func (c *dayCursor) toPrevMonth() {
	for i := 0; i < c.maxEmptyMonthHops(); i++ {
		c.m--
		if c.m < 1 {
			c.m = len(c.cal.Months)
			c.y--
		}
		c.mdays = c.cal.MonthDays(c.m-1, c.y)
		if c.mdays > 0 {
			c.d = c.mdays
			c.abs = c.cal.absDayIndex(c.y, c.m, c.d)
			return
		}
	}
	c.ok = false
}

func (c *dayCursor) next() {
	if !c.ok {
		return
	}
	if c.d < c.mdays {
		c.d++
		c.abs++
		return
	}
	c.toNextMonth()
}

func (c *dayCursor) prev() {
	if !c.ok {
		return
	}
	if c.d > 1 {
		c.d--
		c.abs--
		return
	}
	c.toPrevMonth()
}

// shift moves n days (negative: back).
func (c *dayCursor) shift(n int) {
	for ; n > 0 && c.ok; n-- {
		c.next()
	}
	for ; n < 0 && c.ok; n++ {
		c.prev()
	}
}

func (c *dayCursor) date() DayDate { return DayDate{Year: c.y, Month: c.m, Day: c.d} }

// shiftDate is d moved n calendar days.
func shiftDate(cal *Calendar, d DayDate, n int) (DayDate, bool) {
	c := newDayCursor(cal, d)
	c.shift(n)
	return c.date(), c.ok
}

// moonPhaseFalls reports whether the moment moon reaches the phase target
// (0 new … 0.75 last quarter) falls on the day whose counter is abs: the
// one day per cycle nearest that moment, so it is always a day the moon
// view names with that phase (for any cycle of eight days or more). A
// cycle shorter than a day reaches every phase every day.
func moonPhaseFalls(m *Moon, abs int, target float64) bool {
	if m == nil || m.CycleDays <= 0 {
		return false
	}
	x := m.MoonPhase(abs) - target
	x -= math.Floor(x + 0.5) // signed cycles since the turning point, in [-0.5, 0.5)
	daysSince := x * m.CycleDays
	return daysSince > -0.5 && daysSince <= 0.5
}

// expander expands events against one calendar, which must carry Months,
// Weekdays, and every Moon and Season (unfiltered: a rule on a hidden moon
// still has dates). anchors holds the events after_event conditions name;
// overrides holds stored skips and moves by event id.
type expander struct {
	cal       *Calendar
	anchors   map[string]*Event
	overrides map[string][]OccurrenceOverride
	capDays   int
}

func newExpander(cal *Calendar, anchors map[string]*Event, overrides map[string][]OccurrenceOverride) *expander {
	return &expander{cal: cal, anchors: anchors, overrides: overrides, capDays: ruleScanCapDays(cal)}
}

// occurrences returns e's occurrences in [from, to] with its overrides
// applied: a skipped one stays in the list flagged Skipped (the caller drops
// it for a viewer who may not edit), a moved one appears at its new date
// with MovedFrom set. A move whose original date is no longer an occurrence
// (the rule changed since) is ignored. truncated reports a scan that hit
// its bound.
func (x *expander) occurrences(e *Event, from, to DayDate, depth int) ([]Occurrence, bool) {
	nat, truncated := x.natural(e, from, to, 0, depth)
	ovs := x.overrides[e.ID]
	byDate := make(map[DayDate]OccurrenceOverride, len(ovs))
	for _, o := range ovs {
		byDate[o.date()] = o
	}
	natSet := make(map[DayDate]bool, len(nat))
	out := make([]Occurrence, 0, len(nat))
	for _, d := range nat {
		natSet[d] = true
		if o, ok := byDate[d]; ok {
			if o.Action == OverrideSkip {
				out = append(out, Occurrence{Year: d.Year, Month: d.Month, Day: d.Day, Skipped: true})
				continue
			}
			if _, isMove := o.target(); isMove {
				continue // shown at its new date below
			}
		}
		out = append(out, Occurrence{Year: d.Year, Month: d.Month, Day: d.Day})
	}
	for _, o := range ovs {
		t, ok := o.target()
		if !ok || !t.inRange(from, to) {
			continue
		}
		orig := o.date()
		if !natSet[orig] {
			if found, _ := x.natural(e, orig, orig, 1, depth); len(found) == 0 {
				continue
			}
		}
		out = append(out, Occurrence{Year: t.Year, Month: t.Month, Day: t.Day, MovedFrom: &orig})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a := DayDate{Year: out[i].Year, Month: out[i].Month, Day: out[i].Day}
		b := DayDate{Year: out[j].Year, Month: out[j].Month, Day: out[j].Day}
		return a.before(b)
	})
	return out, truncated
}

// natural returns the dates e's own repeat puts in [from, to], before any
// override, stopping after limit dates when limit > 0.
func (x *expander) natural(e *Event, from, to DayDate, limit, depth int) ([]DayDate, bool) {
	base := DayDate{Year: e.Year, Month: e.Month, Day: e.Day}
	if !e.isRepeating() {
		if base.inRange(from, to) {
			return []DayDate{base}, false
		}
		return nil, false
	}
	if e.usesRule() {
		return x.ruleNatural(e, from, to, limit, depth)
	}
	// Weekly/monthly/yearly: OccursOn decides each day. Nothing lands
	// before the base date, so the walk starts there at the earliest.
	start := from
	if start.before(base) {
		start = base
	}
	if to.before(start) {
		return nil, false
	}
	var out []DayDate
	c := newDayCursor(x.cal, start)
	for steps := 0; c.ok && !to.before(c.date()); steps++ {
		if steps >= x.capDays {
			return out, true
		}
		if e.OccursOn(x.cal, c.y, c.m, c.d) {
			out = append(out, c.date())
			if limit > 0 && len(out) >= limit {
				break
			}
		}
		c.next()
	}
	return out, false
}

// compiledCond is one rule condition ready to test a day cheaply.
type compiledCond struct {
	kind     string
	moon     *Moon
	target   float64
	set      map[int]bool
	n, wd    int
	day      int
	season   *Season
	anchorAt map[DayDate]bool // anchor occurrence dates, for after_event
	lag      *dayCursor       // walks `days` behind the scan, for after_event
}

// ruleNatural walks the days a rule's matches could fall on and returns the
// occurrences (match + offset_days) inside [from, to].
//
// With every > 1 or a max-occurrences cap, which match is which depends on
// every match since the event's start, so the walk starts there; otherwise
// it starts where the range does. Either way it walks at most capDays days,
// and reports truncated if that stopped it early.
func (x *expander) ruleNatural(e *Event, from, to DayDate, limit, depth int) ([]DayDate, bool) {
	r := e.RecurrenceRule
	// An anchor never follows its own anchor: one level only, so a stale
	// cycle in stored data can never loop.
	if depth > 0 && r.hasAfterEvent() {
		return nil, false
	}
	base := DayDate{Year: e.Year, Month: e.Month, Day: e.Day}
	k := r.OffsetDays
	every := r.every()
	maxOcc := e.RecurrenceMaxOccurrences
	if maxOcc != nil && *maxOcc <= 0 {
		return nil, false
	}
	counting := every > 1 || maxOcc != nil
	var end *DayDate
	if e.RecurrenceEndYear != nil && e.RecurrenceEndMonth != nil && e.RecurrenceEndDay != nil {
		end = &DayDate{Year: *e.RecurrenceEndYear, Month: *e.RecurrenceEndMonth, Day: *e.RecurrenceEndDay}
	}

	mFrom, ok1 := shiftDate(x.cal, from, -k)
	mTo, ok2 := shiftDate(x.cal, to, -k)
	if !ok1 || !ok2 {
		return nil, false
	}
	scanStart := mFrom
	if counting || mFrom.before(base) {
		scanStart = base
	}
	if mTo.before(scanStart) {
		return nil, false
	}

	main := newDayCursor(x.cal, scanStart)
	emit := newDayCursor(x.cal, scanStart)
	emit.shift(k)
	conds, truncated, ok := x.compileRule(r, scanStart, mTo, depth)
	if !ok {
		return nil, truncated
	}

	var out []DayDate
	matches := 0
	for steps := 0; main.ok && emit.ok && !mTo.before(main.date()); steps++ {
		if steps >= x.capDays {
			truncated = true
			break
		}
		if x.dayMatches(conds, &main) {
			if matches%every == 0 {
				if maxOcc != nil && matches/every >= *maxOcc {
					break
				}
				od := emit.date()
				if end != nil && end.before(od) {
					break
				}
				if !od.before(from) && !to.before(od) {
					out = append(out, od)
					if limit > 0 && len(out) >= limit {
						break
					}
				}
			}
			matches++
		}
		main.next()
		emit.next()
		for i := range conds {
			if conds[i].lag != nil {
				conds[i].lag.next()
			}
		}
	}
	return out, truncated
}

// compileRule resolves a rule's references (moons, seasons, anchor dates
// over the scan) once, so the day walk tests each condition cheaply. ok is
// false when a condition can never match (a deleted moon, say), which makes
// the whole rule match nothing.
func (x *expander) compileRule(r *RecurrenceRule, scanStart, scanEnd DayDate, depth int) ([]compiledCond, bool, bool) {
	conds := make([]compiledCond, 0, len(r.Match))
	truncated := false
	for _, c := range r.Match {
		cc := compiledCond{kind: c.Kind}
		switch c.Kind {
		case RuleMoonPhase:
			for i := range x.cal.Moons {
				if x.cal.Moons[i].ID == c.MoonID {
					cc.moon = &x.cal.Moons[i]
				}
			}
			if cc.moon == nil {
				return nil, false, false
			}
			cc.target = rulePhaseTargets[c.Phase]
		case RuleWeekday:
			if x.cal.WeekLength() <= 0 {
				return nil, false, false
			}
			cc.set = map[int]bool{}
			if c.Weekday != nil {
				cc.set[*c.Weekday] = true
			}
			for _, w := range c.Weekdays {
				cc.set[w] = true
			}
		case RuleNthWeekday:
			if x.cal.WeekLength() <= 0 || c.Weekday == nil {
				return nil, false, false
			}
			cc.n, cc.wd = c.N, *c.Weekday
		case RuleDayOfMonth:
			cc.day = c.Day
		case RuleMonths, RuleMonth:
			cc.kind = RuleMonths
			cc.set = map[int]bool{}
			if c.Month > 0 {
				cc.set[c.Month] = true
			}
			for _, m := range c.Months {
				cc.set[m] = true
			}
		case RuleSeason, RuleSeasonStart:
			for i := range x.cal.Seasons {
				if x.cal.Seasons[i].ID == c.SeasonID {
					cc.season = &x.cal.Seasons[i]
				}
			}
			if cc.season == nil {
				return nil, false, false
			}
		case RuleAfterEvent, RuleRelativeToEvent:
			cc.kind = RuleAfterEvent
			days := 0
			if c.Days != nil {
				days = *c.Days
			}
			anchor := x.anchors[c.EventID]
			if anchor == nil {
				return nil, false, false
			}
			lag := newDayCursor(x.cal, scanStart)
			lag.shift(-days)
			lagEnd, ok := shiftDate(x.cal, scanEnd, -days)
			if !lag.ok || !ok {
				return nil, false, false
			}
			occ, tr := x.occurrences(anchor, lag.date(), lagEnd, depth+1)
			truncated = truncated || tr
			cc.anchorAt = make(map[DayDate]bool, len(occ))
			for _, o := range occ {
				if !o.Skipped {
					cc.anchorAt[DayDate{Year: o.Year, Month: o.Month, Day: o.Day}] = true
				}
			}
			cc.lag = &lag
		default:
			return nil, false, false
		}
		conds = append(conds, cc)
	}
	return conds, truncated, true
}

// dayMatches reports whether every condition holds on the cursor's day.
func (x *expander) dayMatches(conds []compiledCond, c *dayCursor) bool {
	wl := x.cal.WeekLength()
	for i := range conds {
		cc := &conds[i]
		switch cc.kind {
		case RuleMoonPhase:
			if !moonPhaseFalls(cc.moon, c.abs, cc.target) {
				return false
			}
		case RuleWeekday:
			wd := x.cal.weekdayAt(c.m, c.d, c.abs)
			if wd < 0 || !cc.set[wd] {
				return false
			}
		case RuleNthWeekday:
			if x.cal.weekdayAt(c.m, c.d, c.abs) != cc.wd {
				return false
			}
			if cc.n == ruleLastOfMonth {
				if c.d+wl <= c.mdays {
					return false
				}
			} else if (c.d-1)/wl+1 != cc.n {
				return false
			}
		case RuleDayOfMonth:
			if cc.day == ruleLastOfMonth {
				if c.d != c.mdays {
					return false
				}
			} else if c.d != cc.day {
				return false
			}
		case RuleMonths:
			if !cc.set[c.m] {
				return false
			}
		case RuleSeason:
			if !cc.season.ContainsDate(c.m, c.d) {
				return false
			}
		case RuleSeasonStart:
			if c.m != cc.season.StartMonth || c.d != cc.season.StartDay {
				return false
			}
		case RuleAfterEvent:
			if cc.lag == nil || !cc.lag.ok || !cc.anchorAt[cc.lag.date()] {
				return false
			}
		}
	}
	return true
}
