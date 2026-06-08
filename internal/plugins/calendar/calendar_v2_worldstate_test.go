// calendar_v2_worldstate_test.go — C-CAL-WORLDSTATE-PRODUCTION-PORT 2a.
//
// Guards the live ambient band on calendar_v2: the seed embeds + the engine
// scaffold renders, the band is read-only (no demo/control affordances ship),
// it no-ops without a seed, and the engine source carries the production
// seams (so the same engine drives demo + prod).
package calendar

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func sampleV2WorldStateData() CalendarV2ViewData {
	cal := &Calendar{
		ID: "cal-1", Name: "Harptos", HoursPerDay: 24, MinutesPerHour: 60,
		CurrentYear: 1492, CurrentMonth: 4, CurrentDay: 15,
		Months:   []Month{{Name: "Hammer", Days: 30}, {Name: "Alturiak", Days: 30}, {Name: "Ches", Days: 30}, {Name: "Tarsakh", Days: 30}},
		Weekdays: []Weekday{{Name: "Sul"}, {Name: "Mol"}, {Name: "Zor"}},
	}
	tint := "#c8d8ff"
	seed := &WorldStateSeed{
		TimeOfDay: 0.5,
		Season:    "Spring",
		Date:      WorldStateDate{Year: 1492, Month: 4, Day: 15},
		Sun:       WorldStateSun{},
		Moons: []WorldStateMoon{
			{ID: 1, Name: "Selune", BaseDesign: "moon-realistic-selene", Tint: &tint, PhaseSource: "css-clip", Size: 1, OrbitSpeed: 1, CyclePct: 0.5, NamedPhase: "Full", NamedPhases: []WorldStateMoonPhase{}},
		},
		Weather:  WorldStateWeather{Type: "rain", Intensity: 1},
		Events:   []WorldStateEvent{{Type: "meteor-shower", Name: "Tears of Selune", StartTime: 22, Duration: 4}},
		MoodTint: WorldStateMoodTint{},
	}
	return CalendarV2ViewData{
		ActiveCalendar: cal,
		WorldState:     seed,
		WorldStateJSON: `{"timeOfDay":0.5,"season":"Spring","date":{"year":1492,"month":4,"day":15}}`,
		Year:           1492, Month: 4, Day: 15,
	}
}

func renderBand(t *testing.T, data CalendarV2ViewData) string {
	t.Helper()
	var sb strings.Builder
	if err := worldStateBandV2(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render band: %v", err)
	}
	return sb.String()
}

// TestWorldStateBandV2_EmbedsSeedAndEngine: the band embeds the seed blob +
// the engine scaffold the renderer binds to, and loads the shared assets.
func TestWorldStateBandV2_EmbedsSeedAndEngine(t *testing.T) {
	html := renderBand(t, sampleV2WorldStateData())
	for _, want := range []string{
		`id="cal-v2-worldstate"`,              // the production seed element
		`data-cal-worldstate=`,                // the Part-8 blob the engine reads
		`&#34;timeOfDay&#34;:0.5`,             // the seed JSON actually embedded (html-escaped)
		"data-cal-sky", "data-cal-sky-canvas", // sky scaffold the engine paints
		"data-cal-sky-weather-layer", "data-cal-sky-celestial-layer",
		"data-cal-sky-sun",            // passive sun (engine positions it)
		"background: linear-gradient", // Part A: the SSR base sky gradient
		"/static/css/cal-almanac.css", // rendering-canvas CSS
		"/static/js/cal-almanac.js",   // the shared engine
		"Tears of Selune",             // the celestial event chip
		"Spring", "Rain",              // read-only labels from the seed
	} {
		if !strings.Contains(html, want) {
			t.Errorf("band missing %q", want)
		}
	}
	// Part C: the full-page band is SKY ONLY — the hourglass/shelf is the
	// compact embed variant, not rendered here.
	if strings.Contains(html, "cal-almanac-shelf") || strings.Contains(html, "data-cal-hourglass-canvas") {
		t.Errorf("the full-page band must be sky-only (no hourglass/shelf)")
	}
	// The engine CREATES moon elements from the seed — the templ must NOT
	// hand-render a moon loop (guards against double-rendering).
	if strings.Contains(html, "data-cal-sky-moon") {
		t.Errorf("templ should not render moon elements; the engine creates them")
	}
}

// TestSkybandGradient_SnapsToKeyframes (C-CAL-V2-WORLDSTATE-BAND-FINISHING Part
// A): the SHARED SSR gradient snaps the 0..1 time-of-day to the nearest of the
// 5 keyframes, so the production band paints a sane base before the engine runs.
func TestSkybandGradient_SnapsToKeyframes(t *testing.T) {
	cases := map[float64]string{
		0.0:  "oklch(0.18 0.05 270)", // midnight
		0.25: "oklch(0.55 0.13 30)",  // dawn
		0.5:  "oklch(0.78 0.13 220)", // noon
		0.75: "oklch(0.62 0.16 60)",  // dusk
		1.0:  "oklch(0.18 0.05 270)", // midnight (wrap)
	}
	for tod, want := range cases {
		got := SkybandGradient(tod)
		if !strings.HasPrefix(got, "linear-gradient(180deg,") {
			t.Errorf("t=%.2f: not a gradient: %q", tod, got)
		}
		if !strings.Contains(got, want) {
			t.Errorf("t=%.2f: gradient %q missing keyframe %q", tod, got, want)
		}
	}
	// nil-safe accessor defaults to noon.
	if got := wsSkyTimeFloat(CalendarV2ViewData{}); got != 0.5 {
		t.Errorf("wsSkyTimeFloat(nil) = %v; want 0.5 (noon)", got)
	}
}

// TestWorldStateBandV2_ReadOnly: 2a ships no control affordances — no date
// setter, no draggable time slider, no demo controls. (Controls are 2b/2c.)
func TestWorldStateBandV2_ReadOnly(t *testing.T) {
	html := renderBand(t, sampleV2WorldStateData())
	for _, forbidden := range []string{
		"data-cal-datesetter", // R2 date-setter popover
		"data-cal-controls",   // demo control panel
		"cal-almanac-datesetter",
	} {
		if strings.Contains(html, forbidden) {
			t.Errorf("read-only band must not ship control markup %q", forbidden)
		}
	}
	// The date/time readouts are plain text, not the demo's <button> controls.
	if strings.Contains(html, `<button type="button" class="cal-almanac-sky__date"`) {
		t.Errorf("date readout must be read-only text in 2a, not a button")
	}
}

// TestWorldStateBandV2_NoSeedNoBand: without an active calendar + seed the
// band (and its engine assets) must not render at all.
func TestWorldStateBandV2_NoSeedNoBand(t *testing.T) {
	html := renderBand(t, CalendarV2ViewData{}) // no ActiveCalendar, no WorldState
	if strings.TrimSpace(html) != "" {
		t.Errorf("band must be empty without a seed, got: %q", html)
	}
}

// TestWorldStateHelpers covers the read-only label math.
func TestWorldStateHelpers(t *testing.T) {
	data := sampleV2WorldStateData()
	if got := wsClock(data); got != "12:00" {
		t.Errorf("wsClock at noon = %q, want 12:00", got)
	}
	if got := wsWeatherID(data); got != "rain" {
		t.Errorf("wsWeatherID = %q, want rain", got)
	}
	if got := wsSkyPhase(data); got != "Day" {
		t.Errorf("wsSkyPhase at noon = %q, want Day", got)
	}
	if got := wsDatePrimary(data); !strings.Contains(got, "Tarsakh") || !strings.Contains(got, "15") {
		t.Errorf("wsDatePrimary = %q, want the named month + day", got)
	}
	// Empty weather defaults to clear.
	empty := CalendarV2ViewData{WorldState: &WorldStateSeed{}}
	if got := wsWeatherID(empty); got != "clear" {
		t.Errorf("empty weather should default to clear, got %q", got)
	}
}

// TestEngineHasProductionSeams: the shared engine carries the 2a production
// wiring so demo + prod run off one source (additive; demo path unchanged).
func TestEngineHasProductionSeams(t *testing.T) {
	js := readEngineJS(t)
	for _, marker := range []string{
		"var PROD = false",                               // production flag
		"var PROD_SKIP = {",                              // the demo-block skip set
		"if (PROD && PROD_SKIP[b.name])",                 // runAll skips demo controls in prod
		"document.getElementById('cal-v2-worldstate')",   // prod seed detection
		"if (PROD) {",                                    // world-state block prod branch
		"if (PROD && typeof moon.cyclePct === 'number')", // real moon cycle in prod
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("engine missing production seam: %q", marker)
		}
	}
	// The demo controls are in the skip set (don't ship to prod).
	for _, demoBlock := range []string{"'demo-controls': 1", "'time-control': 1", "'date-setter': 1"} {
		if !strings.Contains(js, demoBlock) {
			t.Errorf("expected demo block %q in PROD_SKIP", demoBlock)
		}
	}
}

func readEngineJS(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	b, err := os.ReadFile(filepath.Join(root, "static", "js", "cal-almanac.js"))
	if err != nil {
		t.Fatalf("read cal-almanac.js: %v", err)
	}
	return string(b)
}
