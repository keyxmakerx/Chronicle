// Package timeutil provides DST-correct conversions between zone-local
// wall-clock times and absolute instants, plus helpers for projecting a
// recurring weekly pattern onto a concrete calendar week in a viewer's zone.
//
// It is deliberately dependency-free (standard library only) and imports no
// Chronicle plugin packages, so both the availability scheduler and the
// real-calendar lane can share one converter instead of maintaining two
// divergent DST implementations (see C-SCHED-AUDIT §A7).
//
// The load-bearing rule (RC-12.5): a *recurring* availability block is stored
// as a zone-local wall-clock (weekday + minute-of-local-midnight + IANA zone),
// NEVER as a UTC instant. It only becomes an absolute instant when projected
// onto a specific real-world date, because the UTC offset of "18:00 local"
// depends on whether that date is inside daylight-saving time. Resolving the
// offset against the real date is what makes the conversion DST-correct.
package timeutil

import "time"

// MinutesPerDay is the number of minutes in a 24-hour civil day. Availability
// minute-of-day values live in [0, MinutesPerDay]; an end value equal to
// MinutesPerDay means "up to local midnight".
const MinutesPerDay = 24 * 60

// LoadLocation returns the IANA location for name, or time.UTC when name is
// empty or not resolvable. Availability rows carry their own IANA zone; a
// missing or unknown zone degrades safely to UTC rather than failing the
// whole overlay render. Callers that need to *reject* a bad zone (e.g. on
// write) should use time.LoadLocation directly.
func LoadLocation(name string) *time.Location {
	if name == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// IsValidLocation reports whether name resolves to a real IANA zone. Empty is
// not valid (callers should require an explicit zone on write).
func IsValidLocation(name string) bool {
	if name == "" {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

// WallClockInstant resolves a zone-local wall-clock — a civil date plus a
// minute offset from local midnight — to an absolute instant. It is
// DST-correct: time.Date computes the zone offset against the real (y, m, d),
// so the same minuteOfDay maps to different absolute instants on either side
// of a daylight-saving transition.
//
// minuteOfDay may be 0..MinutesPerDay (or beyond); time.Date normalizes it,
// so MinutesPerDay correctly rolls to 00:00 of the following civil day —
// which is exactly what an end-of-day block boundary should mean.
func WallClockInstant(loc *time.Location, year int, month time.Month, day, minuteOfDay int) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	// Hour is 0 and the whole time-of-day is expressed as minutes so callers
	// never have to pre-split hours/minutes; time.Date normalizes overflow.
	return time.Date(year, month, day, 0, minuteOfDay, 0, 0, loc)
}

// LocalWallClock converts an absolute instant into the given zone and returns
// the local weekday and the minute-of-local-midnight. Used to place a UTC
// instant onto a viewer's weekly grid.
func LocalWallClock(t time.Time, loc *time.Location) (weekday time.Weekday, minuteOfDay int) {
	if loc == nil {
		loc = time.UTC
	}
	lt := t.In(loc)
	return lt.Weekday(), lt.Hour()*60 + lt.Minute()
}

// CivilDate is a timezone-independent calendar date (a real Gregorian day).
// A date's weekday is the same in every zone, which is why the overlay keys
// its columns on CivilDate rather than on an instant.
type CivilDate struct {
	Year  int
	Month time.Month
	Day   int
}

// ParseCivilDate parses a YYYY-MM-DD string into a CivilDate.
func ParseCivilDate(s string) (CivilDate, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return CivilDate{}, err
	}
	return CivilDate{Year: t.Year(), Month: t.Month(), Day: t.Day()}, nil
}

// String renders the date as YYYY-MM-DD.
func (d CivilDate) String() string {
	return d.midnightUTC().Format("2006-01-02")
}

// Weekday returns the day of week (Sunday=0..Saturday=6). Zone-independent.
func (d CivilDate) Weekday() time.Weekday {
	return d.midnightUTC().Weekday()
}

// AddDays returns the civil date n days after d (n may be negative). It
// normalizes across month and year boundaries via time arithmetic.
func (d CivilDate) AddDays(n int) CivilDate {
	t := d.midnightUTC().AddDate(0, 0, n)
	return CivilDate{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

// midnightUTC anchors the civil date at 00:00 UTC purely for calendar
// arithmetic and weekday derivation — never used as a real instant.
func (d CivilDate) midnightUTC() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}
