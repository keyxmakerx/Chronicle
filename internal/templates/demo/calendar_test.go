// calendar_test.go — C-CAL-SHOWCASE-DESIGN-1-ALMANAC.
//
// Light guards — the showcase isn't load-bearing for canon discipline.
// These pin: render smoke, route registered, JS externalized + no
// inline body, CSS self-contained (no @layer / @apply / canon tokens
// / transition:all), the design switcher renders (Almanac active +
// Linear/Compact scaffolded), and the mock dataset has the full
// fantasy-calendar surface (months/moons/seasons/festivals/events).

package demo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestDemoCalendar_RendersWithoutPanic(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendarAlmanac().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if buf.Len() < 2000 {
		t.Errorf("render too small (%d bytes)", buf.Len())
	}
	html := buf.String()
	if !strings.Contains(html, "cal-almanac-shell") {
		t.Errorf("almanac shell class missing")
	}
}

// TestDemoCalendar_RouteRegistered — the new /demo/calendar route is in
// the wire snapshot.
func TestDemoCalendar_RouteRegistered(t *testing.T) {
	root := calDemoRepoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "internal", "wire", "routes_snapshot.txt"))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if !strings.Contains(string(b), "/demo/calendar") {
		t.Errorf("wire snapshot missing /demo/calendar route")
	}
}

// TestCalAlmanacJS_ExternalFileLoaded — JS is the externalized file
// (carry-forward: never inline a demo-specific <script> body).
func TestCalAlmanacJS_ExternalFileLoaded(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendarAlmanac().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `src="/static/js/cal-almanac.js"`) {
		t.Errorf("calendar templ must load /static/js/cal-almanac.js via <script src=… defer>")
	}
	if !strings.Contains(html, "defer") {
		t.Errorf("script tag must carry defer")
	}
	if _, err := os.Stat(filepath.Join(calDemoRepoRoot(t), "static", "js", "cal-almanac.js")); err != nil {
		t.Errorf("cal-almanac.js missing on disk: %v", err)
	}
}

// TestCalAlmanacJS_NoInlineScript — the templ source must carry no
// inline <script> body of its own. The mock-data JSON node uses
// <script type="application/json"> which is data, not code, and is
// allowed (it doesn't execute).
func TestCalAlmanacJS_NoInlineScript(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "calendar.templ"))
	if err != nil {
		t.Fatalf("read templ: %v", err)
	}
	stripped := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(string(src), "")
	open := regexp.MustCompile(`(?i)<script\b([^>]*)>`)
	for _, m := range open.FindAllStringSubmatch(stripped, -1) {
		attrs := strings.ToLower(m[1])
		if strings.Contains(attrs, "src=") {
			continue // external script — fine
		}
		if strings.Contains(attrs, `type="application/json"`) {
			continue // mock data payload, not code
		}
		t.Errorf("inline <script> tag found in calendar.templ; attrs: %q", m[1])
	}
}

// TestCalAlmanacCSS_NoTransitionAll — canon D5 / §B2 binding: never
// `transition: all` (always list properties).
func TestCalAlmanacCSS_NoTransitionAll(t *testing.T) {
	src := stripCalCSSComments(readCalAlmanacCSS(t))
	forbidden := "trans" + "ition: all"
	if strings.Contains(src, forbidden) {
		t.Errorf("cal-almanac.css contains forbidden `transition: all` — §B2 violation")
	}
}

// TestCalAlmanacCSS_SelfContained — the entire reason for the
// .cal-almanac-* approach: no @layer, no Tailwind @apply, no canon
// (--chronicle-*) tokens. Hardcoded OKLCH only.
func TestCalAlmanacCSS_SelfContained(t *testing.T) {
	src := stripCalCSSComments(readCalAlmanacCSS(t))
	for _, forbidden := range []string{"@layer", "@apply", "--chronicle-"} {
		if strings.Contains(src, forbidden) {
			t.Errorf("cal-almanac.css must not contain %q (self-contained, no cascade fight)", forbidden)
		}
	}
	// Must carry real hover/transition rules (no dead-CSS regression).
	if !strings.Contains(src, ":hover") {
		t.Errorf("cal-almanac.css has no :hover rules")
	}
	if !strings.Contains(src, "transition:") {
		t.Errorf("cal-almanac.css has no transitions")
	}
}

// TestDemoCalendar_PageSeparation — per the operator's page-separation
// directive (2026-06-03): each design lives on its OWN isolated route.
// The `/demo/calendar` index is plain links (no design CSS/JS); the
// Almanac lives at `/demo/calendar/almanac` and loads ONLY its own
// assets. The in-page tab switcher is gone (a bug in one design must
// never reach another). Pin: index routes registered, index has links
// to all three designs + carries no design assets, and the Almanac
// page carries a back-link instead of a tab strip.
func TestDemoCalendar_PageSeparation(t *testing.T) {
	// Wire snapshot has both the index and the almanac sub-route.
	root := calDemoRepoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "internal", "wire", "routes_snapshot.txt"))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	snap := string(b)
	if !strings.Contains(snap, "/demo/calendar/almanac") {
		t.Errorf("wire snapshot missing the isolated /demo/calendar/almanac route")
	}

	// Index page: plain links to each design, NO design CSS/JS.
	var idx bytes.Buffer
	if err := DemoCalendarIndex().Render(context.Background(), &idx); err != nil {
		t.Fatalf("render index: %v", err)
	}
	ih := idx.String()
	for _, link := range []string{`href="/demo/calendar/almanac"`, "Almanac", "Linear", "Compact"} {
		if !strings.Contains(ih, link) {
			t.Errorf("index page missing %q", link)
		}
	}
	if strings.Contains(ih, "cal-almanac.css") || strings.Contains(ih, "cal-almanac.js") {
		t.Errorf("index page must NOT load any design CSS/JS (page-separation isolation)")
	}

	// Almanac page: a back-link to the index, and NO in-page tab
	// switcher (the superseded approach).
	var alm bytes.Buffer
	if err := DemoCalendarAlmanac().Render(context.Background(), &alm); err != nil {
		t.Fatalf("render almanac: %v", err)
	}
	ah := alm.String()
	if !strings.Contains(ah, `href="/demo/calendar"`) {
		t.Errorf("almanac page missing the back-link to the designs index")
	}
	if strings.Contains(ah, "cal-almanac-switch") {
		t.Errorf("almanac page still has the superseded in-page tab switcher; navigation must be via the index")
	}
}

// TestDemoCalendar_MockDataComplete — the mock fantasy calendar has all
// the structures the design demonstrates (months, moons, seasons,
// festivals, events spread across tiers + categories).
func TestDemoCalendar_MockDataComplete(t *testing.T) {
	d := CalAlmanacMock()
	if len(d.Months) < 12 {
		t.Errorf("expected ≥12 months; got %d", len(d.Months))
	}
	if len(d.Weekdays) < 7 {
		t.Errorf("expected ≥7 weekdays; got %d", len(d.Weekdays))
	}
	if len(d.Moons) < 1 {
		t.Errorf("expected ≥1 moon; got %d", len(d.Moons))
	}
	if len(d.Seasons) < 4 {
		t.Errorf("expected 4 seasons; got %d", len(d.Seasons))
	}
	if len(d.Festivals) < 2 {
		t.Errorf("expected ≥2 festivals; got %d", len(d.Festivals))
	}
	if len(d.Tiers) != 3 {
		t.Errorf("expected the canon 3 tiers (major/standard/detail); got %d", len(d.Tiers))
	}
	if len(d.Events) < 12 {
		t.Errorf("expected ≥12 events; got %d", len(d.Events))
	}
	// At least one event of each tier so the visual coverage is honest.
	tiers := map[string]bool{}
	for _, e := range d.Events {
		tiers[e.Tier] = true
	}
	for _, want := range []string{"major", "standard", "detail"} {
		if !tiers[want] {
			t.Errorf("mock data missing any event of tier %q", want)
		}
	}
	// At least one multi-day event (so the multi-day-touches-day
	// expansion gets exercised).
	multi := 0
	for _, e := range d.Events {
		if e.EndMonth != 0 {
			multi++
		}
	}
	if multi == 0 {
		t.Errorf("expected ≥1 multi-day event in mock data")
	}
	// At least one specific-visibility event (drives the chip-row editor).
	specific := 0
	for _, e := range d.Events {
		if e.Visibility == "specific" {
			specific++
		}
	}
	if specific == 0 {
		t.Errorf("expected ≥1 specific-visibility event so the chip-row editor renders rules on click")
	}
}

// TestDemoCalendar_AlmanacHooks — pin the MUST-tier interactive hooks.
// Each is a data-* attribute the JS init blocks bind; missing any
// means the corresponding interaction won't fire.
func TestDemoCalendar_AlmanacHooks(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendarAlmanac().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, h := range []string{
		"data-cal-widget",
		"data-cal-drag-handle",
		"data-cal-resize",
		"data-cal-sky",
		"data-cal-sky-sun",
		"data-cal-sky-moon",
		"data-cal-sky-time-label", // v2: click-to-type time (replaced the scrubber)
		"data-cal-grid",
		"data-cal-cell",
		"data-cal-event-id",
		"data-cal-drag-ghost",
		"data-cal-editor",         // v2: expanded editor (replaced the side drawer)
		"data-cal-vis-mode",
		"data-cal-vis-rules",
		"data-cal-vis-chips",
	} {
		if !strings.Contains(html, h) {
			t.Errorf("required hook missing: %s", h)
		}
	}
	// The retired scrubber + side drawer must be gone (v2 replaced them).
	for _, gone := range []string{"data-cal-sky-scrub", "data-cal-drawer", "cal-almanac-eras"} {
		if strings.Contains(html, gone) {
			t.Errorf("retired hook still present: %s", gone)
		}
	}
}

// TestDemoCalendar_RefinementMockData — operator's refinement pass
// (post-PR-#385) adds named-vocabulary data to the mock so the demo
// reads like worldbuilding instead of procedural fill. Pin the
// presence of each new structure.
func TestDemoCalendar_RefinementMockData(t *testing.T) {
	d := CalAlmanacMock()
	if len(d.Eras) < 3 {
		t.Errorf("expected ≥3 eras (past + current + ancient); got %d", len(d.Eras))
	}
	var current int
	for _, e := range d.Eras {
		if eraContainsYear(e, d.CurrentYear) {
			current++
		}
	}
	if current == 0 {
		t.Errorf("no era contains the current year %d", d.CurrentYear)
	}
	if len(d.WeatherTypes) < 8 {
		t.Errorf("expected ≥8 named weather types across categories; got %d", len(d.WeatherTypes))
	}
	// Categories present.
	wcats := map[string]bool{}
	for _, w := range d.WeatherTypes {
		wcats[w.Category] = true
	}
	for _, want := range []string{"standard", "severe", "environmental", "fantasy"} {
		if !wcats[want] {
			t.Errorf("weather vocabulary missing category %q", want)
		}
	}
	if len(d.MoonPhases) < 8 {
		t.Errorf("expected ≥8 named moon phases for the primary moon; got %d", len(d.MoonPhases))
	}
	if len(d.DayWeather) < 10 {
		t.Errorf("expected ≥10 day-keyed weather entries; got %d", len(d.DayWeather))
	}
	if len(d.DayNotes) < 1 {
		t.Errorf("expected ≥1 day note in the showcase mock; got %d", len(d.DayNotes))
	}
	if len(d.Recurring) < 1 {
		t.Errorf("expected ≥1 recurring template demonstrating recurring + override pattern; got %d", len(d.Recurring))
	}
	// At least one recurring template must carry per-instance overrides.
	anyOverride := false
	for _, r := range d.Recurring {
		if len(r.Overrides) > 0 {
			anyOverride = true
			break
		}
	}
	if !anyOverride {
		t.Errorf("expected at least one recurring template to demonstrate per-instance overrides")
	}
	// Categories all carry an Icon (no empties).
	for _, c := range d.Categories {
		if c.Icon == "" {
			t.Errorf("category %q is missing an Icon glyph", c.ID)
		}
	}
}

// TestDemoCalendar_RefinementV2Hooks — the v2 refinement: ambient sky
// (weather + celestial layers, sun-drag, click-time), era corner
// vignette, snowglobe timepiece, two-tier popup, named weather/category
// chips. Pin the markup hooks the JS init blocks bind.
func TestDemoCalendar_RefinementV2Hooks(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendarAlmanac().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, h := range []string{
		// Ambient sky.
		"data-cal-sky-weather-layer",
		"data-cal-sky-celestial-layer",
		"data-cal-sky-happening",
		"data-cal-sky-time-label",
		// Era corner vignette.
		"data-cal-era-vignette",
		"cal-almanac-eravig",
		// Time-piece common hooks (v3 hourglass — replaces v2 snowglobe).
		"data-cal-time",
		"data-cal-time-tick",
		"data-cal-time-clock",
		// Named weather chip + category icons + recurring marker (carried).
		"cal-almanac-cell__wchip",
		"cal-almanac-chip__icon",
		"cal-almanac-chip--recurring",
	} {
		if !strings.Contains(html, h) {
			t.Errorf("v2 hook missing: %s", h)
		}
	}
}

// TestCalAlmanac_SkyBandAmbientRegistry — the WEATHER_EFFECTS +
// CELESTIAL_EFFECTS registries exist in the JS with MUST entries +
// TBD stubs (dispatch §F).
func TestCalAlmanac_SkyBandAmbientRegistry(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"WEATHER_EFFECTS", "CELESTIAL_EFFECTS",
		"registerInitBlock('weather-registry'", "registerInitBlock('celestial-registry'",
		"registerInitBlock('sky-band-ambient'",
		// MUST weather renders.
		"WEATHER_EFFECTS.rain", "WEATHER_EFFECTS.snow", "WEATHER_EFFECTS.thunderstorm", "WEATHER_EFFECTS.fog",
		// MUST celestial renders.
		"meteor-shower", "eclipse-solar", "eclipse-lunar",
		// TBD stubs registered.
		"'volcanic'", "'ice-age'", "'plague'", "'aurora'", "'comet'",
	} {
		if !strings.Contains(js, m) {
			t.Errorf("registry marker missing in JS: %q", m)
		}
	}
	// Mock mirrors the registries (templ can reference effect metadata).
	d := CalAlmanacMock()
	must, tbd := 0, 0
	for _, e := range append(append([]CalAlmanacEffect{}, d.WeatherEffects...), d.CelestialEffects...) {
		switch e.Tier {
		case "must":
			must++
		case "tbd":
			tbd++
		}
	}
	if must < 6 {
		t.Errorf("expected ≥6 MUST-tier effects in the mock registries; got %d", must)
	}
	if tbd < 7 {
		t.Errorf("expected ≥7 TBD-stub effects (architecture wired, visuals deferred); got %d", tbd)
	}
}

// TestCalAlmanac_PopupSlidableTiers — the two-tier popup: quick-view +
// expanded editor, with the expand affordance wired.
func TestCalAlmanac_PopupSlidableTiers(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendarAlmanac().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, h := range []string{
		"data-cal-qv",          // quick-view tier
		"data-cal-qv-title",
		"data-cal-qv-expand",   // expand affordance
		"data-cal-editor",      // expanded tier
		`data-cal-editor-tab="detail"`,
		`data-cal-editor-tab="notes"`,
		`data-cal-editor-tab="vis"`,
		"data-cal-editor-collapse",
	} {
		if !strings.Contains(html, h) {
			t.Errorf("two-tier popup hook missing: %s", h)
		}
	}
	js := readCalAlmanacJS(t)
	for _, m := range []string{"registerInitBlock('popup-slidein'", "registerInitBlock('popup-expand'", "expandToEditor"} {
		if !strings.Contains(js, m) {
			t.Errorf("popup JS marker missing: %q", m)
		}
	}
}

// TestCalAlmanac_ActionMenu8Items — all 8 actions render in the
// expanded editor (dispatch §D / operator Q2 "Full").
func TestCalAlmanac_ActionMenu8Items(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendarAlmanac().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	actions := []string{"create-entity", "timeline", "permalink", "duplicate", "recurring", "override-weather", "history", "delete"}
	for _, a := range actions {
		if !strings.Contains(html, `data-cal-action="`+a+`"`) {
			t.Errorf("action-menu item missing: %s", a)
		}
	}
	if c := strings.Count(html, `data-cal-action="`); c != 8 {
		t.Errorf("expected exactly 8 action-menu items; got %d", c)
	}
	js := readCalAlmanacJS(t)
	if !strings.Contains(js, "registerInitBlock('action-menu'") {
		t.Errorf("action-menu init block missing")
	}
	// Create Entity From submenu must have the entity types.
	d := CalAlmanacMock()
	if len(d.EntityTypes) < 6 {
		t.Errorf("expected ≥6 entity types for the Create Entity From submenu; got %d", len(d.EntityTypes))
	}
}

// TestCalAlmanac_SnowglobeTimepiece — the timepiece is now a snowglobe
// dome, not the old horizontal strip.
// TestCalAlmanac_HourglassRenders — REFINEMENT-V3: the snowglobe is
// replaced by a vertical hourglass. Markup hooks required for JS
// chamber/stream rendering + theme attribute + flip data-attribute.
func TestCalAlmanac_HourglassRenders(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendarAlmanac().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, h := range []string{
		"cal-almanac-hourglass",
		"cal-almanac-hourglass__frame",
		"data-cal-hourglass-chambers",
		"data-cal-hourglass-stream",
		`data-cal-hourglass-fill="top"`,
		`data-cal-hourglass-fill="bot"`,
		`data-cal-hourglass-chamber="top"`,
		`data-cal-hourglass-chamber="bot"`,
		"data-cal-hourglass-theme",
		"data-cal-hourglass-flipped",
		"cal-almanac-hourglass__waist",
		"cal-almanac-hourglass__sparkle",
		// drag handle + tick + clock preserved across the swap.
		"data-cal-time-drag",
		"data-cal-time-tick",
		"data-cal-time-clock",
	} {
		if !strings.Contains(html, h) {
			t.Errorf("hourglass markup missing: %s", h)
		}
	}
	// Snowglobe markup should be gone — the dome class set in particular.
	if strings.Contains(html, "cal-almanac-globe__dome") || strings.Contains(html, `data-cal-globe-dome`) {
		t.Errorf("snowglobe markup should be removed in v3")
	}
}

// TestCalAlmanac_HourglassFlipMechanism — the flip is tied to sunrise /
// sunset on the mock data, with a JS helper picking the orientation
// from VIEW.timeFrac. Pin the helper + the mock fields exist.
func TestCalAlmanac_HourglassFlipMechanism(t *testing.T) {
	m := CalAlmanacMock()
	if m.Sunrise <= 0 || m.Sunrise >= 1 {
		t.Errorf("CalAlmanacMockData.Sunrise expected (0,1), got %v", m.Sunrise)
	}
	if m.Sunset <= 0 || m.Sunset >= 1 {
		t.Errorf("CalAlmanacMockData.Sunset expected (0,1), got %v", m.Sunset)
	}
	if m.Sunset <= m.Sunrise {
		t.Errorf("expected Sunset > Sunrise on the default mock day, got rise=%v set=%v", m.Sunrise, m.Sunset)
	}
	// isNightPhase honors the interval.
	noon := CalAlmanacMockData{SkyTime: 0.50, Sunrise: 0.25, Sunset: 0.75}
	midnight := CalAlmanacMockData{SkyTime: 0.05, Sunrise: 0.25, Sunset: 0.75}
	if isNightPhase(noon) {
		t.Errorf("noon should be day phase")
	}
	if !isNightPhase(midnight) {
		t.Errorf("midnight should be night phase")
	}
	js := readCalAlmanacJS(t)
	for _, h := range []string{
		"registerInitBlock('hourglass-render'",
		"registerInitBlock('hourglass-flip'",
		"isNightFrac",
		"applyHourglassFlip",
		"applyHourglassLevels",
	} {
		if !strings.Contains(js, h) {
			t.Errorf("hourglass flip-mechanism JS missing: %s", h)
		}
	}
}

// TestCalAlmanac_SandRendererRegistry — WEATHER_EFFECTS + CELESTIAL_EFFECTS
// gain a sandRender hook for every MUST-tier entry. TBD-stub entries
// also get a placeholder hook (default golden / tinted via CSS).
func TestCalAlmanac_SandRendererRegistry(t *testing.T) {
	js := readCalAlmanacJS(t)
	// Hook installer present + a themed-sand init block.
	for _, h := range []string{
		"registerInitBlock('hourglass-themed-sand'",
		"hookSandRenderers",
		"defaultSandRender",
		"streakSandRender",
		"bigDropSandRender",
		"applySandTheme",
	} {
		if !strings.Contains(js, h) {
			t.Errorf("sand-renderer registry hook missing: %s", h)
		}
	}
	// MUST-tier weather sandRender assignments present.
	for _, key := range []string{"WEATHER_EFFECTS.clear", "WEATHER_EFFECTS.rain", "WEATHER_EFFECTS.snow", "WEATHER_EFFECTS.thunderstorm", "WEATHER_EFFECTS.fog", "WEATHER_EFFECTS.cloudy"} {
		if !strings.Contains(js, key+".sandRender") {
			t.Errorf("expected sandRender assigned on %s", key)
		}
	}
	// MUST-tier celestial sandRender assignments.
	for _, key := range []string{"CELESTIAL_EFFECTS['meteor-shower']", "CELESTIAL_EFFECTS['eclipse-solar']", "CELESTIAL_EFFECTS['eclipse-lunar']"} {
		if !strings.Contains(js, key+".sandRender") {
			t.Errorf("expected sandRender assigned on %s", key)
		}
	}
}

// TestCalAlmanac_CreatePopupFlow — click-empty-day → mini create popup.
// Pin the markup + the JS init block + the expand → editor escalation.
func TestCalAlmanac_CreatePopupFlow(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendarAlmanac().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, h := range []string{
		"data-cal-create",
		"data-cal-create-title",
		"data-cal-create-tier",
		"data-cal-create-cat",
		"data-cal-create-notes",
		"data-cal-create-commit",
		"data-cal-create-expand",
		"data-cal-create-close",
		"cal-almanac-qv--create",
	} {
		if !strings.Contains(html, h) {
			t.Errorf("create-popup markup missing: %s", h)
		}
	}
	js := readCalAlmanacJS(t)
	for _, h := range []string{
		"registerInitBlock('popup-create-flow'",
		"openCreatePopup",
		"closeCreatePopup",
		"readCreateForm",
		"mockCreateEvent",
		"expandToEditor", // escalation path target
	} {
		if !strings.Contains(js, h) {
			t.Errorf("create-popup JS missing: %s", h)
		}
	}
}

// TestCalAlmanac_MockCreateEvent — Go-side CreateEvent appends to the
// in-memory events slice with a generated id when none is provided.
func TestCalAlmanac_MockCreateEvent(t *testing.T) {
	m := CalAlmanacMock()
	before := len(m.Events)
	ev := CalAlmanacCreateEvent(&m, CalAlmanacEventDraft{
		Name:     "Battle of Five Armies",
		Month:    m.CurrentMonth,
		Day:      m.CurrentDay,
		Tier:     "major",
		Category: "session",
	})
	if len(m.Events) != before+1 {
		t.Fatalf("expected events slice to grow by 1, got %d -> %d", before, len(m.Events))
	}
	if ev.ID == "" {
		t.Errorf("CreateEvent should populate id; got empty")
	}
	if ev.Name != "Battle of Five Armies" {
		t.Errorf("name not round-tripped: %q", ev.Name)
	}
	if ev.Hour != -1 {
		t.Errorf("expected Hour -1 (all-day default), got %d", ev.Hour)
	}
	// Default tier coalescing.
	ev2 := CalAlmanacCreateEvent(&m, CalAlmanacEventDraft{Name: "Untiered", Month: m.CurrentMonth, Day: m.CurrentDay})
	if ev2.Tier != "standard" {
		t.Errorf("expected tier to default to 'standard', got %q", ev2.Tier)
	}
}

// TestCalAlmanac_V3ProofClasses — every forced-state proof class the
// dispatch enumerates is defined in the CSS so headless screenshots
// can lock the state.
func TestCalAlmanac_V3ProofClasses(t *testing.T) {
	src := stripCalCSSComments(readCalAlmanacCSS(t))
	for _, klass := range []string{
		".cal-almanac--proof-hourglass-rest",
		".cal-almanac--proof-hourglass-flip-active",
		".cal-almanac--proof-hourglass-rain",
		".cal-almanac--proof-hourglass-eclipse",
		".cal-almanac--proof-hourglass-meteor",
		".cal-almanac--proof-hourglass-snow",
		".cal-almanac--proof-popup-create-mini",
		".cal-almanac--proof-popup-create-expanded",
	} {
		if !strings.Contains(src, klass) {
			t.Errorf("v3 proof class missing in CSS: %s", klass)
		}
	}
}

// TestCalAlmanac_EraVignetteNoStrip — the horizontal era strip is gone,
// replaced by the corner vignette.
func TestCalAlmanac_EraVignetteNoStrip(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendarAlmanac().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if strings.Contains(html, "cal-almanac-eras__band") || strings.Contains(html, "data-cal-eras") {
		t.Errorf("the horizontal era strip should be removed in favour of the corner vignette")
	}
	if !strings.Contains(html, "cal-almanac-eravig") || !strings.Contains(html, "data-cal-era-vignette") {
		t.Errorf("era corner-vignette markup missing")
	}
	js := readCalAlmanacJS(t)
	if !strings.Contains(js, "registerInitBlock('era-vignette'") {
		t.Errorf("era-vignette init block missing")
	}
}

// ---- helpers ------------------------------------------------------

func stripCalCSSComments(src string) string {
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(src, "")
}
func calDemoRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}
func readCalAlmanacCSS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(calDemoRepoRoot(t), "static", "css", "cal-almanac.css"))
	if err != nil {
		t.Fatalf("read cal-almanac.css: %v", err)
	}
	return string(b)
}

func readCalAlmanacJS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(calDemoRepoRoot(t), "static", "js", "cal-almanac.js"))
	if err != nil {
		t.Fatalf("read cal-almanac.js: %v", err)
	}
	return string(b)
}
