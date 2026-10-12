// Package calendar — view_helpers.go holds small, pure presentation-shaping
// helpers shared by the calendars list (and its cards' month peeks) and the
// new-calendar wizard Templ pages. None of these decide authorization or
// business rules (that stays in service.go) — they only reshape
// already-authorized data for rendering.
package calendar

import (
	"fmt"
	"strings"
)

// visibilityLabel is the calendars-list card's one-line "who sees this".
// A visibility_rules allow-list is reported only by count: resolving user
// ids to display names would need a users/membership lookup this plugin
// does not have wired (plugins reach each other only through service
// interfaces — see CLAUDE.md), so "Shared with <names>" from the signed
// mockup is deliberately simplified to a count here.
func visibilityLabel(cal Calendar) string {
	if cal.Visibility == "dm_only" {
		return "Director only"
	}
	if rules := ParseVisibilityRules(cal.VisibilityRules); rules != nil && len(rules.AllowedUsers) > 0 {
		if len(rules.AllowedUsers) == 1 {
			return "Restricted to 1 player"
		}
		return fmt.Sprintf("Restricted to %d players", len(rules.AllowedUsers))
	}
	return "Everyone in the campaign"
}

// mainMoon returns a calendar's "main" moon for card display — the
// first entry in Moons. The real schema has no explicit primary-moon flag
// (unlike the mockup's fictional mo.main), so "first in stored order" is the
// stand-in; a future structure editor letting an owner reorder moons would
// change which one that is.
func mainMoon(cal *Calendar) *Moon {
	if len(cal.Moons) == 0 {
		return nil
	}
	return &cal.Moons[0]
}

// monthGridCell is one day cell in a card's month peek, a simplified month
// grid (current month only, no prev/next paging, no festival banding — see
// calendar_peek.templ).
type monthGridCell struct {
	Day        int
	Blank      bool // a leading/trailing pad cell so every row is a full week
	IsToday    bool
	IsPast     bool
	HasMoon    bool
	MoonPhase  float64
	EventCount int
}

// buildMonthGrid lays out one month as full weeks of monthGridCell, so the
// template can range over rows without knowing the calendar's own week
// length. eventCounts is from countEventsByDay (nil/empty renders no dots).
// mainMoonPtr nil renders no per-day silhouette.
func buildMonthGrid(cal *Calendar, year, month int, eventCounts map[int]int, mainMoonPtr *Moon) [][]monthGridCell {
	wl := cal.WeekLength()
	if wl <= 0 {
		wl = 7
	}
	// clampCalendarStructure holds an import to maxCalendarMonthDays, but a
	// row can reach calendar_months.days some other way (a pre-fix import, a
	// future hand-made structure editor) — re-clamping here keeps this grid
	// from building a slice sized to whatever that column holds.
	days := cal.MonthDays(month-1, year)
	if days > maxCalendarMonthDays {
		days = maxCalendarMonthDays
	}
	lead := cal.WeekdayIndex(year, month, 1)
	if lead < 0 {
		// An intercalary month on a calendar whose months restart the week
		// sits outside the weekday cycle; its days run from the first column.
		lead = 0
	} else {
		lead = (lead - cal.GridFirstWeekday(wl) + wl) % wl
	}

	cells := make([]monthGridCell, 0, lead+days+wl)
	for i := 0; i < lead; i++ {
		cells = append(cells, monthGridCell{Blank: true})
	}
	for d := 1; d <= days; d++ {
		cell := monthGridCell{
			Day:        d,
			IsToday:    year == cal.CurrentYear && month == cal.CurrentMonth && d == cal.CurrentDay,
			IsPast:     dateLess(year, month, d, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay),
			EventCount: eventCounts[d],
		}
		if mainMoonPtr != nil {
			cell.HasMoon = true
			cell.MoonPhase = mainMoonPtr.MoonPhase(cal.absDayIndex(year, month, d))
		}
		cells = append(cells, cell)
	}
	for len(cells)%wl != 0 {
		cells = append(cells, monthGridCell{Blank: true})
	}

	weeks := make([][]monthGridCell, 0, len(cells)/wl)
	for i := 0; i < len(cells); i += wl {
		weeks = append(weeks, cells[i:i+wl])
	}
	return weeks
}

// countEventsByDay counts, per day-of-month, how many of events actually
// occur on that day of (year, month). An event that came from a month read
// carries its own dates (Occurrences) and is counted from them; any other is
// recurrence-aware via Event.OccursOn, so a weekly event's dot lands on the
// day it actually recurs to, not its stored base day.
func countEventsByDay(cal *Calendar, events []Event, year, month int) map[int]int {
	if len(events) == 0 {
		return nil
	}
	counts := make(map[int]int, len(events))
	// See buildMonthGrid's own comment: don't trust a stored days value past
	// the bound an import is held to, regardless of how a row got here.
	days := cal.MonthDays(month-1, year)
	if days > maxCalendarMonthDays {
		days = maxCalendarMonthDays
	}
	for _, e := range events {
		if e.hasExpansion() {
			// The month read already worked out this event's dates, with its
			// rule and its skips and moves applied; recounting them from
			// OccursOn would put a dot on a skipped or unmoved date.
			for _, d := range e.visibleOccurrenceDays(year, month, days) {
				counts[d]++
			}
			continue
		}
		for d := 1; d <= days; d++ {
			if e.OccursOn(cal, year, month, d) {
				counts[d]++
			}
		}
	}
	return counts
}

// presetFacts renders one preset/import's structural summary line, e.g.
// "12 months, 365 days · 10-day weeks · 4 moons · 2 seasons".
func presetFacts(ir *ImportResult) string {
	totalDays := 0
	for _, m := range ir.Months {
		totalDays += m.Days
	}
	parts := []string{
		fmt.Sprintf("%d %s, %d %s", len(ir.Months), nounFor(len(ir.Months), "month", "months"), totalDays, nounFor(totalDays, "day", "days")),
		fmt.Sprintf("%d-day weeks", len(ir.Weekdays)),
	}
	if n := len(ir.Moons); n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", n, nounFor(n, "moon", "moons")))
	}
	if n := len(ir.Seasons); n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", n, nounFor(n, "season", "seasons")))
	}
	return strings.Join(parts, " · ")
}

// nounFor is the noun to print after a count: one for exactly 1, many
// otherwise, so a count never reads "1 months".
func nounFor(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// monthBarWidths returns each month's share of the year as a percentage
// (summing to ~100), for the wizard's proportional "ybar" month-length bar.
func monthBarWidths(months []MonthInput) []float64 {
	total := 0
	for _, m := range months {
		total += m.Days
	}
	if total <= 0 {
		return nil
	}
	out := make([]float64, len(months))
	for i, m := range months {
		out[i] = float64(m.Days) / float64(total) * 100
	}
	return out
}
