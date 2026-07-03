// worldstate.go — C-CAL-WORLDSTATE-SERVER-MODEL (Phase 1, build-order step 2).
//
// Turns the showcase's browser-only `worldState` blob into a real,
// server-side concept. The DATA contract is the showcase engine's seed at
// static/js/cal-almanac.js ~L464-481 (CATALOG Part 8): one object that
// drives BOTH the calendar sky-band and the standalone hourglass. The
// production port (Phase 2) and the Foundry push (Phase 5b) both consume
// this shape, so the Go-emitted JSON must stay 1:1 with the JS contract —
// the parity test in worldstate_test.go is the load-bearing guard.
//
// timepieceFill and timeControl direction/speed are LIVE-control / session
// state, not stored: the seed emits timeControl with neutral defaults and
// omits timepieceFill entirely (the client adds it locally). See CATALOG
// Part 6.
package calendar

import (
	"math"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// --- persisted domain types (migration 008) ---

// DayWeather is a per-date authored weather row (calendar_day_weather).
// At most one exists per (calendar, date).
//
// The rich fields (migration 012 / cordinator#53 unification seam) carry the
// Calendaria-shaped state the legacy single-row calendar_weather held; nil
// means "unset". WeatherType doubles as the Calendaria preset id — the W1
// vocabulary-parity wave (chronicle#479) made the two id sets 1:1, so no
// separate preset_id field exists. The sky band renders WeatherType only;
// the /calendar/weather sync surface round-trips the full shape.
type DayWeather struct {
	ID          int    `json:"id"`
	CalendarID  string `json:"calendar_id"`
	Year        int    `json:"year"`
	Month       int    `json:"month"`
	Day         int    `json:"day"`
	WeatherType string `json:"weather_type"`

	PresetLabel            *string   `json:"preset_label,omitempty"`
	Icon                   *string   `json:"icon,omitempty"`
	Color                  *string   `json:"color,omitempty"`
	TemperatureCelsius     *float64  `json:"temperature_celsius,omitempty"`
	WindSpeedKPH           *float64  `json:"wind_speed_kph,omitempty"`
	WindSpeedTier          *string   `json:"wind_speed_tier,omitempty"`
	WindDirection          *string   `json:"wind_direction,omitempty"`
	WindDirectionDegrees   *int      `json:"wind_direction_degrees,omitempty"`
	PrecipitationType      *string   `json:"precipitation_type,omitempty"`
	PrecipitationIntensity *float64  `json:"precipitation_intensity,omitempty"`
	Description            *string   `json:"description,omitempty"`
	UpdatedAt              time.Time `json:"updated_at,omitempty"`
}

// CelestialEvent is a date-specific sky event (meteor shower, eclipse,
// blood moon, ...) from calendar_celestial_events. Distinct from the GM's
// narrative calendar_events. Visibility uses the same everyone/dm_only
// vocabulary so the seed can filter GM-only events for players.
type CelestialEvent struct {
	ID            int    `json:"id"`
	CalendarID    string `json:"calendar_id"`
	Year          int    `json:"year"`
	Month         int    `json:"month"`
	Day           int    `json:"day"`
	Type          string `json:"type"`
	StartHour     int    `json:"start_hour"`
	DurationHours int    `json:"duration_hours"`
	Name          string `json:"name"`
	Visibility    string `json:"visibility"`
}

// MoonPhaseVocab is one named span of a moon's phase cycle
// (calendar_moon_phases). start_pct/end_pct are 0..100; the showcase
// matches cyclePct*100 against [start_pct, end_pct) (with wrap support).
type MoonPhaseVocab struct {
	MoonID   int    `json:"moon_id"`
	Name     string `json:"name"`
	StartPct int    `json:"start_pct"`
	EndPct   int    `json:"end_pct"`
	Glyph    string `json:"glyph"`
}

// SpecialDay is a special-moon-day flag on a specific date
// (calendar_special_days). kind is a free-form treatment id.
type SpecialDay struct {
	CalendarID string `json:"calendar_id"`
	Year       int    `json:"year"`
	Month      int    `json:"month"`
	Day        int    `json:"day"`
	Kind       string `json:"kind"`
}

// --- the worldState seed shape (CATALOG Part 8) ---
//
// JSON tags are camelCase to match the JS contract exactly (the rest of the
// codebase uses snake_case, but this struct is a wire mirror of the showcase
// seed — keep it 1:1, the parity test enforces it).

// WorldStateSeed is the server-side mirror of the showcase `worldState`
// object. One seed drives both atmosphere surfaces.
type WorldStateSeed struct {
	TimeOfDay   float64               `json:"timeOfDay"` // 0..1 fraction of the day
	Season      string                `json:"season"`    // derived season label ("" if none)
	Date        WorldStateDate        `json:"date"`
	Sun         WorldStateSun         `json:"sun"`
	Moons       []WorldStateMoon      `json:"moons"`
	Weather     WorldStateWeather     `json:"weather"`
	Events      []WorldStateEvent     `json:"events"`
	MoodTint    WorldStateMoodTint    `json:"moodTint"`
	TimeControl WorldStateTimeControl `json:"timeControl"`
}

// WorldStateDate is the seed's {year, month, day}.
type WorldStateDate struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

// WorldStateSun carries the painted-sun tint. Null today: the showcase
// derives the tint client-side from time-of-day + weather (the JS seed also
// ships sun.tint = null). No real schema source exists yet — emit null
// rather than fabricate one (stop-and-flag note in the PR).
type WorldStateSun struct {
	Tint *string `json:"tint"`
}

// WorldStateMoon mirrors one entry of the showcase moons[] array.
// orbitOffset is the persisted phase_offset; phase/cyclePct/namedPhase are
// computed server-side from the real moon math (the JS seed leaves them null
// and fills them at render — the server can fill them because it owns the
// cycle math, which is more useful for non-rendering consumers like Foundry).
type WorldStateMoon struct {
	ID          int              `json:"id"`
	Name        string           `json:"name"`
	BaseDesign  string           `json:"baseDesign"`
	Tint        *string          `json:"tint"`
	PhaseSource string           `json:"phaseSource"`
	Size        float64          `json:"size"`
	OrbitSpeed  float64          `json:"orbitSpeed"`
	OrbitOffset float64          `json:"orbitOffset"`
	Phase       int              `json:"phase"`    // 0..7 phase index
	NamedPhase  string           `json:"namedPhase"`
	CyclePct    float64          `json:"cyclePct"` // 0..1 position in cycle
	NamedPhases []WorldStateMoonPhase `json:"namedPhases"`
}

// WorldStateMoonPhase mirrors one moon.namedPhases span (start_pct/end_pct
// 0..100). Field names match the showcase's DATA.moon_phases entries.
type WorldStateMoonPhase struct {
	Name     string `json:"name"`
	StartPct int    `json:"start_pct"`
	EndPct   int    `json:"end_pct"`
	Glyph    string `json:"glyph"`
}

// WorldStateWeather is the seed's {type, intensity}.
type WorldStateWeather struct {
	Type      string  `json:"type"`
	Intensity float64 `json:"intensity"`
}

// WorldStateEvent mirrors one celestial event in the showcase events[]
// array. Field names (start_time/duration) match what celestialFor() emits
// so the Phase-2 renderer wires straight through.
//
// Visibility is an ADDITIVE field for the Foundry W5 bridge (cordinator#34):
// the module's celestial-note projection maps dm_only → Calendaria's
// 'secret' note visibility. Player-role seeds never contain dm_only events
// at all (celestialSeeds strips them), so the field leaks nothing; the web
// engine ignores unknown keys.
type WorldStateEvent struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	StartTime  int    `json:"start_time"`
	Duration   int    `json:"duration"`
	Visibility string `json:"visibility,omitempty"`
}

// WorldStateMoodTint is the player mood overlay. Color is null when no mood
// is set; intensity 0..1.
type WorldStateMoodTint struct {
	Color     *string `json:"color"`
	Intensity float64 `json:"intensity"`
}

// WorldStateTimeControl is the DM time-verb intent. NOT persisted — emitted
// with neutral defaults (direction +1, speed 1). The client owns the live
// value during play.
type WorldStateTimeControl struct {
	Direction int     `json:"direction"`
	Speed     float64 `json:"speed"`
}

// moonShortPhaseNames matches the showcase's procedural fallback labels
// (cal-almanac.js moonNamedPhase) — "New"/"Full", not "New Moon"/"Full
// Moon". Keep identical so an authored-vocab-less moon reads the same label
// on both surfaces.
var moonShortPhaseNames = []string{
	"New", "Waxing Crescent", "First Quarter", "Waxing Gibbous",
	"Full", "Waning Gibbous", "Last Quarter", "Waning Crescent",
}

// moonPhaseIndex maps a 0..1 cycle position to the 0..7 phase index, matching
// the showcase: ((round(pct*8) % 8) + 8) % 8.
func moonPhaseIndex(pct float64) int {
	idx := (int(roundHalfUp(pct*8))%8 + 8) % 8
	return idx
}

// roundHalfUp rounds to nearest, .5 away from zero — matches JS Math.round
// for the non-negative pct values used here.
func roundHalfUp(v float64) float64 {
	if v >= 0 {
		return float64(int(v + 0.5))
	}
	return float64(int(v - 0.5))
}

// moonNamedPhase resolves the display label for a moon at cycle position pct
// (0..1). It walks the authored namedPhases spans first (with wrap support,
// matching the showcase), then falls back to the procedural short names.
func moonNamedPhase(pct float64, spans []WorldStateMoonPhase) string {
	p := pct * 100
	for _, s := range spans {
		a, b := float64(s.StartPct), float64(s.EndPct)
		if a <= b {
			if p >= a && p < b {
				return s.Name
			}
		} else { // wrap-around span (e.g. 90..10)
			if p >= a || p < b {
				return s.Name
			}
		}
	}
	return moonShortPhaseNames[moonPhaseIndex(pct)]
}

// AssembleWorldStateSeed builds the Part-8 seed from already-loaded inputs.
// It is pure (no I/O) so the parity + filtering tests can exercise it
// directly; the service method BuildWorldStateSeed does the repo loads and
// calls this. role gates GM-only celestial events: a non-DM role drops
// dm_only events so players never see them.
//
// dayWeather may be nil (no authored weather for the date → "clear").
// moonPhases is keyed by moon id; a moon with no authored vocab falls back to
// procedural phase names.
func AssembleWorldStateSeed(
	cal *Calendar,
	year, month, day int,
	dayWeather *DayWeather,
	celestials []CelestialEvent,
	moonPhases map[int][]MoonPhaseVocab,
	role int,
) *WorldStateSeed {
	seed := &WorldStateSeed{
		Date:        WorldStateDate{Year: year, Month: month, Day: day},
		TimeOfDay:   timeOfDayFraction(cal),
		Season:      seasonLabel(cal, month, day),
		Sun:         WorldStateSun{Tint: nil}, // client-derived; no schema source (see type doc)
		Weather:     weatherSeed(dayWeather),
		MoodTint:    moodTintSeed(cal),
		TimeControl: WorldStateTimeControl{Direction: 1, Speed: 1}, // ephemeral defaults
		Moons:       moonSeeds(cal, year, month, day, moonPhases),
		Events:      celestialSeeds(celestials, role),
	}
	return seed
}

// timeOfDayFraction converts the calendar's current hour/minute into a 0..1
// fraction of the day. Guards against a zero-length day.
func timeOfDayFraction(cal *Calendar) float64 {
	minutesPerDay := cal.HoursPerDay * cal.MinutesPerHour
	if minutesPerDay <= 0 {
		return 0
	}
	elapsed := cal.CurrentHour*cal.MinutesPerHour + cal.CurrentMinute
	return float64(elapsed) / float64(minutesPerDay)
}

// seasonLabel returns the season name containing the date, or "" if none.
func seasonLabel(cal *Calendar, month, day int) string {
	if s := cal.SeasonForDate(month, day); s != nil {
		return s.Name
	}
	return ""
}

// weatherSeed maps the per-date authored weather (or "clear") into the seed.
// Intensity is always 1 — the showcase DATA has no per-weather intensity and
// the seed mirrors that (the engine scales intensity itself).
func weatherSeed(dw *DayWeather) WorldStateWeather {
	t := "clear"
	if dw != nil && dw.WeatherType != "" {
		t = dw.WeatherType
	}
	return WorldStateWeather{Type: t, Intensity: 1}
}

// moodTintSeed reads the persisted live-mood columns. Null intensity (no
// mood set) reads as 0.
func moodTintSeed(cal *Calendar) WorldStateMoodTint {
	out := WorldStateMoodTint{Color: cal.MoodTintColor}
	if cal.MoodTintIntensity != nil {
		out.Intensity = *cal.MoodTintIntensity
	}
	return out
}

// moonSeeds computes each moon's render+phase state for the date using the
// real cycle math (Moon.MoonPhase over the absolute day), then resolves the
// named phase from the authored vocab or the procedural fallback.
func moonSeeds(cal *Calendar, year, month, day int, moonPhases map[int][]MoonPhaseVocab) []WorldStateMoon {
	abs := cal.AbsoluteDay(year, month, day)
	out := make([]WorldStateMoon, 0, len(cal.Moons))
	for i := range cal.Moons {
		m := &cal.Moons[i]
		spans := toSeedPhases(moonPhases[m.ID])
		pct := m.MoonPhase(abs)
		out = append(out, WorldStateMoon{
			ID:          m.ID,
			Name:        m.Name,
			BaseDesign:  m.BaseDesign,
			Tint:        m.Tint,
			PhaseSource: m.PhaseSource,
			Size:        m.Size,
			OrbitSpeed:  m.OrbitSpeed,
			OrbitOffset: m.PhaseOffset,
			Phase:       moonPhaseIndex(pct),
			CyclePct:    pct,
			NamedPhase:  moonNamedPhase(pct, spans),
			NamedPhases: spans,
		})
	}
	// C-CAL-SKY-COMPLETION A: a real-life (Gregorian) calendar with no authored
	// moons still gets THE Moon — its phase computed LOCALLY from the real
	// synodic cycle for the shown date (no API, no location: phase is global, the
	// date is already timezone-synced). Advancing the day moves it through the
	// real ~29.53-day cycle. Fantasy calendars respect their own config (a moon
	// the operator adds in settings replaces this default, since len(Moons) > 0).
	if cal.Mode == ModeRealLife && len(out) == 0 {
		pct := gregorianMoonPhase(year, month, day)
		out = append(out, WorldStateMoon{
			ID:          0,
			Name:        "Moon",
			BaseDesign:  "moon-realistic-selene",
			PhaseSource: "css-clip",
			Size:        1,
			OrbitSpeed:  1,
			// OrbitOffset positions the disc on the night/day arc by phase (the
			// engine places it at time-0.5+offset): a full moon (pct 0.5) sits
			// opposite the sun → up at night; a new moon (pct 0) rides with the
			// sun → up by day. So it's only visible at the realistic times — at
			// night for most phases — not floating in the noon sky.
			OrbitOffset: 0.5 - pct,
			Phase:       moonPhaseIndex(pct),
			CyclePct:    pct,
			NamedPhase:  moonNamedPhase(pct, nil),
			NamedPhases: []WorldStateMoonPhase{},
		})
	}
	return out
}

// gregorianMoonPhase returns the real Moon's cycle position (0..1, 0 = new moon)
// for a Gregorian date, computed locally from the synodic month — NO API, NO
// location (C-CAL-SKY-COMPLETION A). The illuminated phase is global at a given
// instant; only the date matters, which a real-life calendar already tracks in
// the user's timezone. Reference: JDN 2451550.1 (2000-01-06) ≈ a new moon.
func gregorianMoonPhase(year, month, day int) float64 {
	const synodicMonth = 29.530588853
	const refNewMoonJDN = 2451550.1
	days := float64(gregorianJDN(year, month, day)) - refNewMoonJDN
	phase := math.Mod(days/synodicMonth, 1)
	if phase < 0 {
		phase += 1
	}
	return phase
}

// gregorianJDN is the Julian Day Number for a proleptic-Gregorian date (the
// standard Fliegel–Van Flandern integer formula). Local + exact; used only for
// the real-Moon phase math.
func gregorianJDN(y, m, d int) int {
	a := (14 - m) / 12
	yy := y + 4800 - a
	mm := m + 12*a - 3
	return d + (153*mm+2)/5 + 365*yy + yy/4 - yy/100 + yy/400 - 32045
}

// toSeedPhases converts persisted MoonPhaseVocab rows into the seed's
// namedPhases span shape. Returns a non-nil empty slice so the JSON emits
// [] rather than null (matching the showcase seed's always-array shape).
func toSeedPhases(vocab []MoonPhaseVocab) []WorldStateMoonPhase {
	out := make([]WorldStateMoonPhase, 0, len(vocab))
	for _, v := range vocab {
		out = append(out, WorldStateMoonPhase{
			Name:     v.Name,
			StartPct: v.StartPct,
			EndPct:   v.EndPct,
			Glyph:    v.Glyph,
		})
	}
	return out
}

// celestialSeeds maps the date's celestial events into the seed, dropping
// dm_only events for non-DM roles so players don't see GM-only sky events.
func celestialSeeds(celestials []CelestialEvent, role int) []WorldStateEvent {
	out := make([]WorldStateEvent, 0, len(celestials))
	canSeeDM := permissions.CanSeeDmOnly(role)
	for _, ce := range celestials {
		if ce.Visibility == storageVisibilityDMOnly && !canSeeDM {
			continue
		}
		out = append(out, WorldStateEvent{
			Type:       ce.Type,
			Name:       ce.Name,
			StartTime:  ce.StartHour,
			Duration:   ce.DurationHours,
			Visibility: ce.Visibility,
		})
	}
	return out
}
