// worldstate_test.go — C-CAL-WORLDSTATE-SERVER-MODEL.
//
// The load-bearing test is TestWorldStateSeed_ParityWithJSContract: it
// asserts the Go-emitted seed JSON has exactly the keys the showcase engine's
// worldState seed uses (static/js/cal-almanac.js). If the two drift, the
// Phase-2 port breaks — this test is the guard. The rest cover the seed math
// (moon phase / named-phase fallback), the role-gated dm_only celestial
// filtering, and the SetWorldState mood/time round-trip + WS emit.
package calendar

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// --- mockCalendarRepo world-state method stubs ---
//
// Defined here (same package) so the migration-008 repo methods can be
// injected into service tests without touching service_test.go.

func (m *mockCalendarRepo) GetDayWeather(ctx context.Context, calendarID string, year, month, day int) (*DayWeather, error) {
	if m.getDayWeatherFn != nil {
		return m.getDayWeatherFn(ctx, calendarID, year, month, day)
	}
	return nil, nil
}

func (m *mockCalendarRepo) SetDayWeather(ctx context.Context, calendarID string, year, month, day int, weatherType string) error {
	if m.setDayWeatherFn != nil {
		return m.setDayWeatherFn(ctx, calendarID, year, month, day, weatherType)
	}
	return nil
}

func (m *mockCalendarRepo) GetCelestialEvents(ctx context.Context, calendarID string, year, month, day int) ([]CelestialEvent, error) {
	if m.getCelestialEventsFn != nil {
		return m.getCelestialEventsFn(ctx, calendarID, year, month, day)
	}
	return nil, nil
}

func (m *mockCalendarRepo) AddCelestialEvent(ctx context.Context, ce CelestialEvent) error {
	if m.addCelestialEventFn != nil {
		return m.addCelestialEventFn(ctx, ce)
	}
	return nil
}

func (m *mockCalendarRepo) GetMoonPhasesForCalendar(ctx context.Context, calendarID string) (map[int][]MoonPhaseVocab, error) {
	if m.getMoonPhasesFn != nil {
		return m.getMoonPhasesFn(ctx, calendarID)
	}
	return nil, nil
}

func (m *mockCalendarRepo) GetSpecialDays(ctx context.Context, calendarID string, year, month, day int) ([]SpecialDay, error) {
	if m.getSpecialDaysFn != nil {
		return m.getSpecialDaysFn(ctx, calendarID, year, month, day)
	}
	return nil, nil
}

func (m *mockCalendarRepo) SetMoodTint(ctx context.Context, calendarID string, color *string, intensity *float64) error {
	if m.setMoodTintFn != nil {
		return m.setMoodTintFn(ctx, calendarID, color, intensity)
	}
	return nil
}

// sampleSeedCalendar builds a small calendar with two moons + a season for
// the seed tests. 12 months × 30 days, 24h day.
func sampleSeedCalendar() *Calendar {
	silverTint := "#c8d8ff"
	return &Calendar{
		ID:             "cal-1",
		CampaignID:     "camp-1",
		Name:           "Harptos",
		CurrentYear:    1492,
		CurrentMonth:   4,
		CurrentDay:     15,
		CurrentHour:    12,
		CurrentMinute:  0,
		HoursPerDay:      24,
		MinutesPerHour:   60,
		SecondsPerMinute: 60,
		Months: []Month{
			{Name: "Hammer", Days: 30}, {Name: "Alturiak", Days: 30}, {Name: "Ches", Days: 30},
			{Name: "Tarsakh", Days: 30}, {Name: "Mirtul", Days: 30}, {Name: "Kythorn", Days: 30},
			{Name: "Flamerule", Days: 30}, {Name: "Eleasis", Days: 30}, {Name: "Eleint", Days: 30},
			{Name: "Marpenoth", Days: 30}, {Name: "Uktar", Days: 30}, {Name: "Nightal", Days: 30},
		},
		Seasons: []Season{
			{Name: "Spring", StartMonth: 3, StartDay: 1, EndMonth: 5, EndDay: 30},
		},
		Moons: []Moon{
			{ID: 1, Name: "Selune", CycleDays: 30.4, PhaseOffset: 0, Color: "#ffffff",
				BaseDesign: "moon-realistic-selene", Tint: &silverTint, PhaseSource: "css-clip", Size: 1, OrbitSpeed: 1},
			{ID: 2, Name: "Shar", CycleDays: 30.4, PhaseOffset: 15, Color: "#222244",
				BaseDesign: "moon-realistic-selene", PhaseSource: "css-clip", Size: 0.7, OrbitSpeed: 1.2},
		},
	}
}

// TestWorldStateSeed_ParityWithJSContract is the load-bearing test: the Go
// seed JSON must carry the same keys the showcase worldState seed uses.
func TestWorldStateSeed_ParityWithJSContract(t *testing.T) {
	cal := sampleSeedCalendar()
	phases := map[int][]MoonPhaseVocab{
		1: {{MoonID: 1, Name: "The Silver Crown", StartPct: 45, EndPct: 55, Glyph: "🌕"}},
	}
	celestials := []CelestialEvent{
		{Type: "meteor-shower", Name: "Tears of Selune", StartHour: 22, DurationHours: 4, Visibility: "everyone"},
	}
	seed := AssembleWorldStateSeed(cal, 1492, 4, 15, &DayWeather{WeatherType: "rain"}, celestials, phases, permissions.RoleOwner)

	// Marshal → generic map to read the emitted top-level + nested keys.
	raw, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("unmarshal seed: %v", err)
	}

	js := readShowcaseJS(t)

	// 1) Every top-level seed key must appear as a property in the showcase
	//    worldState block.
	for key := range top {
		if !strings.Contains(js, key+":") {
			t.Errorf("top-level seed key %q absent from showcase worldState contract", key)
		}
	}
	// 2) The expected top-level set is exactly the Part-8 shape (no extra, no
	//    missing) — guards against a silent drift in either direction.
	wantTop := []string{"timeOfDay", "season", "date", "sun", "moons", "weather", "events", "moodTint", "timeControl"}
	if len(top) != len(wantTop) {
		t.Errorf("seed has %d top-level keys, want %d (%v)", len(top), len(wantTop), wantTop)
	}
	for _, k := range wantTop {
		if _, ok := top[k]; !ok {
			t.Errorf("seed missing required top-level key %q", k)
		}
	}

	// 3) Moon-entry keys must match the showcase moon shape.
	var moons []map[string]json.RawMessage
	if err := json.Unmarshal(top["moons"], &moons); err != nil || len(moons) == 0 {
		t.Fatalf("seed moons did not decode: %v", err)
	}
	for moonKey := range moons[0] {
		// camelCase render keys live in the JS seed; snake_case span keys
		// (start_pct/end_pct) live in DATA.moon_phases — both are in the file.
		if !strings.Contains(js, moonKey) {
			t.Errorf("moon seed key %q absent from showcase JS contract", moonKey)
		}
	}
	for _, want := range []string{"baseDesign", "phaseSource", "orbitSpeed", "orbitOffset", "cyclePct", "namedPhase", "namedPhases"} {
		if _, ok := moons[0][want]; !ok {
			t.Errorf("moon seed missing required key %q", want)
		}
	}
}

// TestWorldStateSeed_MoonMathAndNamedPhase checks the cycle math + the named-
// phase vocab walk with the procedural fallback.
func TestWorldStateSeed_MoonMathAndNamedPhase(t *testing.T) {
	cal := sampleSeedCalendar()
	// Selune: full vocab span covering the full-moon window so a mid-cycle day
	// resolves to the authored name; Shar: no vocab → procedural fallback.
	phases := map[int][]MoonPhaseVocab{
		1: {{MoonID: 1, Name: "The Silver Crown", StartPct: 0, EndPct: 100, Glyph: "🌕"}},
	}
	seed := AssembleWorldStateSeed(cal, 1492, 4, 15, nil, nil, phases, int(permissions.RoleOwner))

	if len(seed.Moons) != 2 {
		t.Fatalf("want 2 moons, got %d", len(seed.Moons))
	}
	if seed.Moons[0].NamedPhase != "The Silver Crown" {
		t.Errorf("Selune should use authored vocab, got %q", seed.Moons[0].NamedPhase)
	}
	// cyclePct is 0..1; phase index 0..7.
	if seed.Moons[0].CyclePct < 0 || seed.Moons[0].CyclePct >= 1 {
		t.Errorf("cyclePct out of range: %v", seed.Moons[0].CyclePct)
	}
	if seed.Moons[0].Phase < 0 || seed.Moons[0].Phase > 7 {
		t.Errorf("phase index out of range: %d", seed.Moons[0].Phase)
	}
	// Shar has no vocab → one of the procedural short names.
	if !contains(moonShortPhaseNames, seed.Moons[1].NamedPhase) {
		t.Errorf("Shar should fall back to a procedural name, got %q", seed.Moons[1].NamedPhase)
	}
	// orbitOffset mirrors phase_offset; tint passes through.
	if seed.Moons[1].OrbitOffset != 15 {
		t.Errorf("orbitOffset should mirror phase_offset, got %v", seed.Moons[1].OrbitOffset)
	}
	if seed.Moons[0].Tint == nil || *seed.Moons[0].Tint != "#c8d8ff" {
		t.Errorf("Selune tint should pass through")
	}
	// namedPhases always emits a (possibly empty) array, never null.
	if seed.Moons[1].NamedPhases == nil {
		t.Errorf("namedPhases must be a non-nil slice for [] JSON")
	}
}

// TestWorldStateSeed_DMOnlyCelestialFiltered: players never see dm_only
// celestial events; DMs see everything.
func TestWorldStateSeed_DMOnlyCelestialFiltered(t *testing.T) {
	cal := sampleSeedCalendar()
	celestials := []CelestialEvent{
		{Type: "meteor-shower", Name: "Public Shower", Visibility: "everyone"},
		{Type: "eclipse-solar", Name: "Secret Eclipse", Visibility: "dm_only"},
	}

	player := AssembleWorldStateSeed(cal, 1492, 4, 15, nil, celestials, nil, int(permissions.RolePlayer))
	if len(player.Events) != 1 || player.Events[0].Name != "Public Shower" {
		t.Errorf("player should see only the public event, got %+v", player.Events)
	}

	dm := AssembleWorldStateSeed(cal, 1492, 4, 15, nil, celestials, nil, int(permissions.RoleOwner))
	if len(dm.Events) != 2 {
		t.Errorf("DM should see both events, got %d", len(dm.Events))
	}
}

// TestWorldStateSeed_WeatherAndMood: authored weather + persisted mood feed
// the seed; absent weather → "clear", absent mood → intensity 0.
func TestWorldStateSeed_WeatherAndMood(t *testing.T) {
	cal := sampleSeedCalendar()
	color := "#8a2be2"
	intensity := 0.6
	cal.MoodTintColor = &color
	cal.MoodTintIntensity = &intensity

	withWeather := AssembleWorldStateSeed(cal, 1492, 4, 15, &DayWeather{WeatherType: "snow"}, nil, nil, int(permissions.RoleOwner))
	if withWeather.Weather.Type != "snow" || withWeather.Weather.Intensity != 1 {
		t.Errorf("authored weather not reflected: %+v", withWeather.Weather)
	}
	if withWeather.MoodTint.Color == nil || *withWeather.MoodTint.Color != color || withWeather.MoodTint.Intensity != intensity {
		t.Errorf("mood not reflected: %+v", withWeather.MoodTint)
	}
	if withWeather.Season != "Spring" {
		t.Errorf("season label wrong: %q", withWeather.Season)
	}
	if withWeather.TimeOfDay != 0.5 { // 12:00 of a 24h day
		t.Errorf("timeOfDay should be 0.5 at noon, got %v", withWeather.TimeOfDay)
	}

	noWeather := AssembleWorldStateSeed(sampleSeedCalendar(), 1492, 4, 15, nil, nil, nil, int(permissions.RoleOwner))
	if noWeather.Weather.Type != "clear" {
		t.Errorf("absent weather should default to clear, got %q", noWeather.Weather.Type)
	}
	if noWeather.MoodTint.Intensity != 0 || noWeather.MoodTint.Color != nil {
		t.Errorf("absent mood should be null/0, got %+v", noWeather.MoodTint)
	}
	// timeControl is emitted with ephemeral defaults, never persisted.
	if noWeather.TimeControl.Direction != 1 || noWeather.TimeControl.Speed != 1 {
		t.Errorf("timeControl defaults wrong: %+v", noWeather.TimeControl)
	}
}

// TestSetWorldState_RoundTripAndEmit: mood + time write through the service
// persists to the repo and emits calendar.worldstate.changed.
func TestSetWorldState_RoundTripAndEmit(t *testing.T) {
	cal := sampleSeedCalendar()
	var savedColor *string
	var savedIntensity *float64
	var savedYear, savedMonth, savedDay int
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return cal, nil },
		setMoodTintFn: func(_ context.Context, _ string, color *string, intensity *float64) error {
			savedColor, savedIntensity = color, intensity
			return nil
		},
		updateFn: func(_ context.Context, c *Calendar) error {
			savedYear, savedMonth, savedDay = c.CurrentYear, c.CurrentMonth, c.CurrentDay
			return nil
		},
	}
	pub := &recordingPublisher{}
	svc := NewCalendarService(repo)
	svc.SetEventPublisher(pub)

	color := "#112233"
	y, mo, d := 1493, 6, 1
	err := svc.SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{
		Mood: &WorldStateMoodTint{Color: &color, Intensity: 0.4},
		Time: &WorldStateTimeSet{Year: &y, Month: &mo, Day: &d},
	})
	if err != nil {
		t.Fatalf("SetWorldState: %v", err)
	}
	if savedColor == nil || *savedColor != color || savedIntensity == nil || *savedIntensity != 0.4 {
		t.Errorf("mood not persisted: color=%v intensity=%v", savedColor, savedIntensity)
	}
	if savedYear != y || savedMonth != mo || savedDay != d {
		t.Errorf("date not persisted: %d-%d-%d", savedYear, savedMonth, savedDay)
	}
	if !contains(pub.types(), "calendar.worldstate.changed") {
		t.Errorf("expected calendar.worldstate.changed emit, got %v", pub.types())
	}
}

// --- helpers ---
// (contains() is shared with ws_dotted_test.go.)

// readShowcaseJS reads the showcase engine source (repo-root relative).
func readShowcaseJS(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	// internal/plugins/calendar → repo root is three levels up.
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	b, err := os.ReadFile(filepath.Join(root, "static", "js", "cal-almanac.js"))
	if err != nil {
		t.Fatalf("read cal-almanac.js: %v", err)
	}
	return string(b)
}
