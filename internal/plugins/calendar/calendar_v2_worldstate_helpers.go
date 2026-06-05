// calendar_v2_worldstate_helpers.go — view helpers for the live ambient
// worldState band on calendar_v2 (C-CAL-WORLDSTATE-PRODUCTION-PORT, 2a).
//
// These produce the read-only first-paint labels + the weather-effect class
// the rendering-canvas CSS keys on. The engine (cal-almanac.js) repaints the
// animated layers from the embedded seed on load; these are the no-JS / SSR
// fallback values, mirroring the demo's pre-JS render.
package calendar

import "fmt"

// wsActive reports whether the ambient band should render (active calendar +
// a successfully-built seed).
func wsActive(data CalendarV2ViewData) bool {
	return data.WorldState != nil && data.ActiveCalendar != nil
}

// wsWeatherID returns the weather-effect id the canvas CSS + engine key on
// (the seed's weather type already is that id — "clear"/"rain"/"snow"/...).
// Empty defaults to "clear".
func wsWeatherID(data CalendarV2ViewData) string {
	if data.WorldState == nil || data.WorldState.Weather.Type == "" {
		return "clear"
	}
	return data.WorldState.Weather.Type
}

// wsTimeFrac formats the 0..1 time-of-day for the data attribute the engine
// reads as a first-paint hint.
func wsTimeFrac(data CalendarV2ViewData) string {
	if data.WorldState == nil {
		return "0.5"
	}
	return fmt.Sprintf("%.4f", data.WorldState.TimeOfDay)
}

// wsClock renders the in-world time as H:MM (or HH:MM) from the 0..1
// time-of-day and the calendar's hours-per-day, matching the demo's clock.
func wsClock(data CalendarV2ViewData) string {
	if data.WorldState == nil || data.ActiveCalendar == nil {
		return ""
	}
	hpd := data.ActiveCalendar.HoursPerDay
	if hpd <= 0 {
		hpd = 24
	}
	mins := int(data.WorldState.TimeOfDay * float64(hpd) * 60)
	h := (mins / 60) % hpd
	m := mins % 60
	return fmt.Sprintf("%d:%02d", h, m)
}

// wsDatePrimary renders "Weekday · Day MonthName" for the cursor date.
// Weekday uses the same simple day-modulo the demo uses (it is a label, not an
// astronomical weekday). Guards short month/weekday lists.
func wsDatePrimary(data CalendarV2ViewData) string {
	cal, ws := data.ActiveCalendar, data.WorldState
	if cal == nil || ws == nil {
		return ""
	}
	month := ""
	if ws.Date.Month >= 1 && ws.Date.Month <= len(cal.Months) {
		month = cal.Months[ws.Date.Month-1].Name
	}
	weekday := ""
	if n := len(cal.Weekdays); n > 0 && ws.Date.Day >= 1 {
		weekday = cal.Weekdays[(ws.Date.Day-1)%n].Name
	}
	if weekday != "" {
		return fmt.Sprintf("%s · %d %s", weekday, ws.Date.Day, month)
	}
	return fmt.Sprintf("%d %s", ws.Date.Day, month)
}

// wsSkyPhase labels the part of the day from the 0..1 time-of-day, mirroring
// the demo's dawn/day/dusk/night banding.
func wsSkyPhase(data CalendarV2ViewData) string {
	if data.WorldState == nil {
		return ""
	}
	t := data.WorldState.TimeOfDay
	switch {
	case t < 0.22:
		return "Night"
	case t < 0.30:
		return "Dawn"
	case t < 0.72:
		return "Day"
	case t < 0.80:
		return "Dusk"
	default:
		return "Night"
	}
}

// wsWeatherLabel is the human label for the weather type (capitalized id with
// a couple of multi-word special cases).
func wsWeatherLabel(data CalendarV2ViewData) string {
	switch wsWeatherID(data) {
	case "clear":
		return "Clear"
	case "rain":
		return "Rain"
	case "snow":
		return "Snow"
	case "fog":
		return "Fog"
	case "cloudy":
		return "Cloudy"
	case "thunderstorm":
		return "Thunderstorm"
	default:
		return titleCaseFirst(wsWeatherID(data))
	}
}

// wsYearLabel renders "Year N <epoch>" for the shelf flank.
func wsYearLabel(data CalendarV2ViewData) string {
	if data.WorldState == nil {
		return ""
	}
	epoch := ""
	if data.ActiveCalendar != nil && data.ActiveCalendar.EpochName != nil {
		epoch = " " + *data.ActiveCalendar.EpochName
	}
	return fmt.Sprintf("Year %d%s", data.WorldState.Date.Year, epoch)
}

// titleCaseFirst upper-cases the first rune of s (ASCII-simple; weather ids
// are lowercase ASCII).
func titleCaseFirst(s string) string {
	if s == "" {
		return ""
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 32
	}
	return string(b)
}
