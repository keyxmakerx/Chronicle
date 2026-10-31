package calendar

import "math"

// Parts of the day, by how high the calendar's sun stands.
const (
	DayPartDawn  = "dawn"
	DayPartDay   = "day"
	DayPartDusk  = "dusk"
	DayPartNight = "night"
)

// twilight is how far below or above the horizon the sun is still dawn or
// dusk, in degrees: civil twilight, when the sky page is visibly lit but
// the sun not yet high.
const twilight = 6.0

// DayPart names the time of day at the calendar's own hour and minute on a
// date, from the sun the sky draws (sky_world.js makeSkym.sun): the same
// latitude, solstice on the year's first day, and an hour rescaled to a
// 24-hour face. Dawn is the morning's twilight, dusk the evening's.
func (c *Calendar) DayPart(year, month, day, hour, minute int) string {
	hpd, mph := c.HoursPerDay, c.MinutesPerHour
	if hpd <= 0 {
		hpd = 24
	}
	if mph <= 0 {
		mph = 60
	}
	h24 := (float64(hour) + float64(minute)/float64(mph)) / float64(hpd) * 24
	yl := float64(c.YearLength())
	if yl <= 0 {
		yl = 360
	}
	doy := day - 1
	for i := 0; i < month-1 && i < len(c.Months); i++ {
		doy += c.MonthDays(i, year)
	}
	lat := 45.0
	if c.Hemisphere != nil && *c.Hemisphere == HemisphereSouth {
		lat = -45
	}
	const d2r = math.Pi / 180
	lon := 1.5*math.Pi + 2*math.Pi*(float64(doy)+(h24-12)/24)/yl
	dec := math.Asin(math.Sin(23.44*d2r) * math.Sin(lon))
	ha := (h24 - 12) * 15 * d2r
	sa := math.Sin(lat*d2r)*math.Sin(dec) + math.Cos(lat*d2r)*math.Cos(dec)*math.Cos(ha)
	alt := math.Asin(math.Max(-1, math.Min(1, sa))) / d2r
	switch {
	case alt > twilight:
		return DayPartDay
	case alt < -twilight:
		return DayPartNight
	case h24 < 12:
		return DayPartDawn
	}
	return DayPartDusk
}
