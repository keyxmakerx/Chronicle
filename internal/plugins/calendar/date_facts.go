package calendar

import "fmt"

// Reads of one date against the calendar, for other plugins (through
// adapters in internal/app) that keep dates of their own, such as a page's
// "Died on" field. Geometry must be loaded.

// ValidDate reports whether the calendar has that day: a month it has and
// a day that month has in that year, leap days included.
func (c *Calendar) ValidDate(year, month, day int) bool {
	return month >= 1 && month <= len(c.Months) && day >= 1 && day <= c.MonthDays(month-1, year)
}

// DateLabel formats a date the way FullDateLabel formats today.
func (c *Calendar) DateLabel(year, month, day int) string {
	return fmt.Sprintf("%s %d, %d", c.MonthName(month), day, year)
}

// DayNumber is a date on the day counter weekdays and moons use, so the
// days between two dates are the difference of their numbers.
func (c *Calendar) DayNumber(year, month, day int) int {
	return c.absDayIndex(year, month, day)
}

// AddDays steps a date n days forward (n >= 0) through the calendar's own
// months and leap rule; false when the calendar has no day to land on.
func (c *Calendar) AddDays(d DayDate, n int) (DayDate, bool) {
	return c.addDays(d, n)
}
