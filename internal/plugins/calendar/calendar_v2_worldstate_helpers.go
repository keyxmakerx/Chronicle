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

// wsSkyTimeFloat returns the 0..1 time-of-day (default noon) for the SSR sky
// gradient. nil-safe so a seedless band still paints a sane base.
func wsSkyTimeFloat(data CalendarV2ViewData) float64 {
	if data.WorldState == nil {
		return 0.5
	}
	return data.WorldState.TimeOfDay
}

// SkybandGradient returns the server-rendered base sky gradient for a 0..1
// time-of-day. SHARED between the /demo showcase and the production
// calendar_v2 band / entity embeds (C-CAL-V2-WORLDSTATE-BAND-FINISHING Part A —
// promoted out of the demo package so the two can't drift). It is the FIRST
// PAINT base only; cal-almanac.js renderTimePipeline overwrites
// `style.background` with a color-mix gradient when time-of-day changes.
//
// Keyframes (top→bottom 2-stop gradient), snapped to the nearest quarter (the
// engine handles live interpolation):
//
//	midnight 0.00 → deep indigo · dawn 0.25 → coral/lavender ·
//	noon 0.50 → cyan-blue · dusk 0.75 → amber/rose · midnight 1.00 (wrap).
func SkybandGradient(t float64) string {
	type stop struct{ top, bot string }
	stops := []stop{
		{"oklch(0.18 0.05 270)", "oklch(0.10 0.04 270)"}, // midnight
		{"oklch(0.55 0.13 30)", "oklch(0.35 0.10 305)"},  // dawn
		{"oklch(0.78 0.13 220)", "oklch(0.62 0.10 230)"}, // noon
		{"oklch(0.62 0.16 60)", "oklch(0.38 0.12 350)"},  // dusk
		{"oklch(0.18 0.05 270)", "oklch(0.10 0.04 270)"}, // midnight (wrap)
	}
	// Snap to the nearest of the 5 quarter keyframes (0, .25, .5, .75, 1).
	q := int(t*4 + 0.5)
	if q < 0 {
		q = 0
	}
	if q > 4 {
		q = 4
	}
	s := stops[q]
	return "linear-gradient(180deg, " + s.top + " 0%, " + s.bot + " 100%)"
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
		// Year-AWARE weekday (C-CAL-V2-SKY-RENDER-COMPLETION FIX B): the shared
		// #428 core, so the band reads e.g. "Monday · 8 June" for real-life June
		// 2026 — consistent with the grid + mini-month. The old (Day-1)%n was
		// year- AND month-blind (Jan 8 and Jun 8 printed the same weekday).
		idx := v2WeekdayIndexFor(cal, ws.Date.Year, ws.Date.Month, ws.Date.Day)
		if idx >= 0 && idx < n {
			weekday = cal.Weekdays[idx].Name
		}
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

// gmMoodPreset is one selectable mood-tint swatch in the GM panel (4b),
// mirroring the showcase MOOD_PRESETS. Color is the OKLCH the wash applies —
// the swatch shows the canvas color it represents (a color-picker swatch, not
// themeable chrome), so the rendering-canvas exemption logic still holds.
type gmMoodPreset struct {
	Key       string
	Label     string
	Color     string
	Intensity float64
}

// gmMoodPresets returns the 8 showcase mood presets verbatim (cal-almanac.js
// MOOD_PRESETS) so the production picker == the validated showcase set.
func gmMoodPresets() []gmMoodPreset {
	return []gmMoodPreset{
		{"ominous-red", "Ominous", "oklch(0.55 0.22 25)", 0.45},
		{"eerie-green", "Eerie", "oklch(0.70 0.20 150)", 0.40},
		{"melancholy-blue", "Melancholy", "oklch(0.55 0.16 250)", 0.42},
		{"festive-gold", "Festive", "oklch(0.85 0.16 85)", 0.40},
		{"cursed-violet", "Cursed", "oklch(0.55 0.22 305)", 0.45},
		{"holy-white", "Holy", "oklch(0.97 0.02 95)", 0.40},
		{"void-black", "Void", "oklch(0.15 0.02 280)", 0.50},
		{"frostbite-cyan", "Frostbite", "oklch(0.85 0.12 200)", 0.40},
	}
}

// gmWeatherType is one selectable weather condition in the GM console.
// IDs match the engine's SKY_FX catalog (cal-almanac.js) 1:1 so every tile
// the panel offers genuinely renders on the band. Glyph is the tile icon;
// Category groups the catalog browser (Standard / Severe / Environmental /
// Fantasy — the CATALOG Part-3 taxonomy).
type gmWeatherType struct {
	ID       string
	Label    string
	Glyph    string
	Category string
}

// gmWeatherTypes returns the FULL weather catalog the GM can set
// (C-CAL-WORLDSTATE-GM-OVERHAUL → C-CAL-PARITY-W0W1). The engine renders all
// of these; server-side the weather column is a free string, so expanding this
// list is purely additive (stored calendar_day_weather rows keep rendering).
//
// Parity (C-CAL-PARITY-W0W1): the first four categories are the 42 Calendaria
// preset ids verbatim (the sync contract — match exactly), grouped in
// Calendaria's four categories. The fifth category "Chronicle" holds our
// native extras that Calendaria lacks (kept, never deleted). IDs here match the
// engine's SKY_FX catalog (cal-almanac.js) 1:1 — pinned by the completeness
// test (TestWeatherCatalogContract here + weather_catalog_completeness.test.mjs).
//
// Note on the axis ruling (parity plan §B): "meteor-shower" appears here on the
// WEATHER axis AND in gmCelestialTypes/knownCelestialTypes on the EVENT axis —
// they coexist by design (rain THROUGH a meteor shower survives the Foundry
// round-trip). Both render via the same SKY_FX["meteor-shower"].
func gmWeatherTypes() []gmWeatherType {
	return []gmWeatherType{
		// --- Standard (Calendaria, 13) ---
		{"clear", "Clear", "☀", "Standard"},
		{"partly-cloudy", "Partly Cloudy", "⛅", "Standard"},
		{"cloudy", "Cloudy", "☁", "Standard"},
		{"overcast", "Overcast", "🌥", "Standard"},
		{"drizzle", "Drizzle", "🌦", "Standard"},
		{"rain", "Rain", "🌧", "Standard"},
		{"fog", "Fog", "🌁", "Standard"},
		{"mist", "Mist", "🌫", "Standard"},
		{"windy", "Windy", "🌬", "Standard"},
		{"sunshower", "Sunshower", "🌦", "Standard"},
		{"snow", "Snow", "❄", "Standard"},
		{"sleet", "Sleet", "🌧", "Standard"},
		{"heat-wave", "Heat Wave", "🔆", "Standard"},
		// --- Severe (Calendaria, 7) ---
		// hail moves Standard→Severe to match Calendaria (id unchanged; grouping only).
		{"thunderstorm", "Thunderstorm", "⚡", "Severe"},
		{"blizzard", "Blizzard", "🌬", "Severe"},
		{"hail", "Hail", "🧊", "Severe"},
		{"tornado", "Tornado", "🌪", "Severe"},
		{"hurricane", "Hurricane", "🌀", "Severe"},
		{"ice-storm", "Ice Storm", "🧊", "Severe"},
		{"monsoon", "Monsoon", "🌧", "Severe"},
		// --- Environmental (Calendaria, 8) ---
		// sandstorm moves Severe→Environmental to match Calendaria (id unchanged).
		{"ashfall", "Ashfall", "🌋", "Environmental"},
		{"sandstorm", "Sandstorm", "🏜", "Environmental"},
		{"luminous-sky", "Luminous Sky", "🌌", "Environmental"},
		{"sakura-bloom", "Sakura Bloom", "🌸", "Environmental"},
		{"autumn-leaves", "Autumn Leaves", "🍁", "Environmental"},
		{"rolling-fog", "Rolling Fog", "🌫", "Environmental"},
		{"wildfire-smoke", "Wildfire Smoke", "🔥", "Environmental"},
		{"dust-devil", "Dust Devil", "🌪", "Environmental"},
		// --- Fantasy (Calendaria, 14) ---
		{"black-sun", "Black Sun", "🌑", "Fantasy"},
		{"ley-surge", "Ley Surge", "💜", "Fantasy"},
		{"aether-haze", "Aether Haze", "🌀", "Fantasy"},
		{"nullfront", "Nullfront", "⬛", "Fantasy"},
		{"permafrost-surge", "Permafrost Surge", "❄", "Fantasy"},
		{"gravewind", "Gravewind", "💀", "Fantasy"},
		{"veilfall", "Veilfall", "🌫", "Fantasy"},
		{"arcane-winds", "Arcane Winds", "🌀", "Fantasy"},
		{"acid-rain", "Acid Rain", "☣", "Fantasy"},
		{"blood-rain", "Blood Rain", "🩸", "Fantasy"},
		{"meteor-shower", "Meteor Shower", "☄", "Fantasy"},
		{"spore-cloud", "Spore Cloud", "🍄", "Fantasy"},
		{"divine-light", "Divine Light", "✨", "Fantasy"},
		{"plague-miasma", "Plague Miasma", "☠", "Fantasy"},
		// --- Chronicle-native extras (kept; Calendaria has no equivalent) ---
		{"heavy-rain", "Heavy Rain", "⛈", "Chronicle"},
		{"snow-flurries", "Snow Flurries", "🌨", "Chronicle"},
		{"ember-rain", "Ember Rain", "🔥", "Chronicle"},
		{"falling-leaves", "Falling Leaves", "🍂", "Chronicle"},
		{"pollen-drift", "Pollen Drift", "🌼", "Chronicle"},
		{"fireflies", "Fireflies", "✨", "Chronicle"},
		{"miasma", "Miasma", "☠", "Chronicle"},
	}
}

// gmCelestialType is one GM-triggerable world-event tile. IDs match the
// service's knownCelestialTypes AND the engine's SKY_FX renderers — every
// tile both persists and visibly renders.
type gmCelestialType struct {
	ID       string
	Label    string
	Glyph    string
	Category string
}

// gmCelestialTypes returns the FULL triggerable world-event catalog
// (mirrors knownCelestialTypes; the sky-events vs omens grouping is the
// console's browse taxonomy).
func gmCelestialTypes() []gmCelestialType {
	return []gmCelestialType{
		// Sky events
		{"shooting-star", "Shooting Star", "💫", "Sky"},
		{"meteor-shower", "Meteor Shower", "☄", "Sky"},
		{"meteor-storm", "Meteor Storm", "🌠", "Sky"},
		{"star-fall", "Star Fall", "✨", "Sky"},
		{"comet", "Comet", "☄", "Sky"},
		{"aurora", "Aurora", "🌌", "Sky"},
		{"arcane-aurora", "Arcane Aurora", "🔮", "Sky"},
		// Sun & moons
		{"eclipse-solar", "Solar Eclipse", "🌑", "Sun & Moons"},
		{"eclipse-lunar", "Lunar Eclipse", "🌒", "Sun & Moons"},
		{"blood-moon", "Blood Moon", "🔴", "Sun & Moons"},
		{"supermoon", "Supermoon", "🌕", "Sun & Moons"},
		{"harvest-moon", "Harvest Moon", "🌔", "Sun & Moons"},
		{"blue-moon", "Blue Moon", "🔵", "Sun & Moons"},
		// Omens
		{"volcanic", "Volcanic Unrest", "🌋", "Omens"},
		{"plague", "Plague Miasma", "🦠", "Omens"},
		{"ice-age", "Deep Freeze", "🧊", "Omens"},
	}
}

// gmCurrentWeather returns the current seed weather type for the select
// default ("clear" when none).
func gmCurrentWeather(data CalendarV2ViewData) string {
	if data.WorldState != nil && data.WorldState.Weather.Type != "" {
		return data.WorldState.Weather.Type
	}
	return "clear"
}

// titleCaseFirst renders a dashed weather id as words ("heavy-rain" →
// "Heavy Rain"; ASCII-simple — weather ids are lowercase ASCII).
func titleCaseFirst(s string) string {
	if s == "" {
		return ""
	}
	b := []byte(s)
	up := true
	for i := 0; i < len(b); i++ {
		if b[i] == '-' {
			b[i] = ' '
			up = true
			continue
		}
		if up && b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 32
		}
		up = false
	}
	return string(b)
}
