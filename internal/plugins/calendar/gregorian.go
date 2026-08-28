package calendar

import "time"

// gregorian.go — the two real-world date helpers the domain model depends on.
//
// CALV5 SALVAGE: both are recovered verbatim from the pre-deletion tree
// (22ac88a~1), where they lived in handler.go and worldstate.go respectively —
// a presentation file and a service file, neither of which V5 is bringing back.
// They are pure, they have no dependencies beyond time, and Calendar's own
// methods call them, so the domain is where they belong. Moving them here is
// the only change; the arithmetic is untouched.
//
// See cordinator/plans/2026-08-21-calendar-v5-salvage-manifest.md.

// daysInGregorianMonth returns the length of a real-world month, leap years
// included. `month` is 0-based, matching the model's real-time month index —
// time.Date normalises day 0 of month+1 to the last day of month.
func daysInGregorianMonth(year, month int) int {
	return time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
}

// gregorianJDN is the Julian Day Number for a proleptic-Gregorian date (the
// standard Fliegel–Van Flandern integer formula). Local + exact.
//
// It is the shared true-day counter for real-time calendars: the real-Moon
// phase math, the display weekday column and recurrence expansion all count
// real-time days through it, so none of them drifts across a Gregorian leap
// day. Keeping ONE counter is the point — the pre-deletion plugin's worst
// structural bug was two independent day counts that disagreed by a day per
// leap year, so a moon disc could contradict the grid cell it sat in.
func gregorianJDN(y, m, d int) int {
	a := (14 - m) / 12
	yy := y + 4800 - a
	mm := m + 12*a - 3
	return d + (153*mm+2)/5 + 365*yy + yy/4 - yy/100 + yy/400 - 32045
}
