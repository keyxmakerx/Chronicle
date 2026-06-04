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
	// V4: the hourglass is now an SVG frame standing on a horizontal shelf.
	for _, h := range []string{
		"cal-almanac-hourglass__frame",
		"cal-almanac-hourglass__svg",
		"data-cal-hourglass-theme",
		"data-cal-hourglass-flipped",
		"data-cal-hourglass-canvas",
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
		t.Errorf("snowglobe markup should be removed")
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
	if !strings.Contains(js, "registerInitBlock('era-overlay'") {
		t.Errorf("era-overlay init block missing (v4 renamed era-vignette → era-overlay)")
	}
}

// ============================================================
// REFINEMENT-V4 tests
// ============================================================

// TestCalAlmanac_EraOverlayBounded — the era marker is small + capped and
// never overlaps the title bar (it lives inside the sky-band). Asserts on
// the CSS size cap + the structural placement (badge is the only hit
// target; the halo is pointer-events:none).
func TestCalAlmanac_EraOverlayBounded(t *testing.T) {
	css := stripCalCSSComments(readCalAlmanacCSS(t))
	// Hard 25% cap present on the era wrapper.
	if !strings.Contains(css, "max-width: 25%") || !strings.Contains(css, "max-height: 25%") {
		t.Errorf("era wrapper must hard-cap at 25%% of the calendar")
	}
	// Base size token <= 120px.
	if !strings.Contains(css, "--cal-era-size: 116px") {
		t.Errorf("era base size token should be <=120px (116px)")
	}
	// The halo is never a hit target; only the badge is.
	if !strings.Contains(css, ".cal-almanac-eravig__halo") || !strings.Contains(css, ".cal-almanac-eravig__badge") {
		t.Errorf("era must split into a pointer-events:none halo + a hit-target badge")
	}
	html := renderAlmanac(t)
	// The era marker renders inside the sky-band (animation window).
	if !strings.Contains(html, "data-cal-era-badge") || !strings.Contains(html, "data-cal-era-halo") {
		t.Errorf("era badge + halo markup missing")
	}
}

// TestCalAlmanac_EraEffectsRegistry — ERA_EFFECTS exists with the MUST
// era types + editable params (color + particleSpec).
func TestCalAlmanac_EraEffectsRegistry(t *testing.T) {
	js := readCalAlmanacJS(t)
	if !strings.Contains(js, "registerInitBlock('era-effects-registry'") {
		t.Errorf("era-effects-registry init block missing")
	}
	for _, id := range []string{"ERA_EFFECTS.golden", "ERA_EFFECTS.dark", "ERA_EFFECTS.war", "ERA_EFFECTS.mythic", "ERA_EFFECTS.ancient", "ERA_EFFECTS.neutral"} {
		if !strings.Contains(js, id) {
			t.Errorf("ERA_EFFECTS missing era type: %s", id)
		}
	}
	// Each entry carries an editable colour ramp + a particleSpec.
	if !strings.Contains(js, "particleSpec:") || !strings.Contains(js, "lightness:") {
		t.Errorf("ERA_EFFECTS entries must expose editable colour + particleSpec")
	}
}

// TestCalAlmanac_EraPointerEventsSafe — the era element must NOT intercept
// the time controls or day-cell clicks (the v3 root cause). Asserts the
// pointer-events discipline + z-index ordering in CSS.
func TestCalAlmanac_EraPointerEventsSafe(t *testing.T) {
	css := stripCalCSSComments(readCalAlmanacCSS(t))
	// Wrapper + halo are pointer-events:none; the sun + time controls sit
	// above the era z-index.
	if !strings.Contains(css, ".cal-almanac-eravig {") {
		t.Fatalf("era wrapper rule missing")
	}
	// The sky arc (sun) is z-index 5 (above era z3); time overlay z6.
	if !strings.Contains(css, "z-index: 5") || !strings.Contains(css, "z-index: 6") {
		t.Errorf("sun arc (z5) + time overlay (z6) must sit above the era marker (z3)")
	}
	// The halo explicitly disables pointer events.
	idx := strings.Index(css, ".cal-almanac-eravig__halo {")
	if idx < 0 {
		t.Fatalf("era halo rule missing")
	}
	if !strings.Contains(css[idx:idx+400], "pointer-events: none") {
		t.Errorf("era halo must be pointer-events:none")
	}
}

// TestCalAlmanac_ParticleEngineRegistryWiring — weather/celestial entries
// carry particleSpec data; the engine reads them.
func TestCalAlmanac_ParticleEngineRegistryWiring(t *testing.T) {
	js := readCalAlmanacJS(t)
	if !strings.Contains(js, "registerInitBlock('particle-engine'") {
		t.Errorf("particle-engine init block missing")
	}
	if !strings.Contains(js, "CalParticleEngine") || !strings.Contains(js, "createSurface") {
		t.Errorf("shared CalParticleEngine missing")
	}
	// Specs are data on the registries.
	for _, h := range []string{
		"WEATHER_EFFECTS.rain = {", "particleSpec: {",
		"CELESTIAL_EFFECTS['meteor-shower']",
		"feedSkyEngine", "setEmitters",
	} {
		if !strings.Contains(js, h) {
			t.Errorf("particle engine wiring missing: %s", h)
		}
	}
	// Perf guards (binding §B3).
	for _, g := range []string{"IntersectionObserver", "visibilitychange", "prefers-reduced-motion", "hardwareConcurrency", "devicePixelRatio"} {
		if !strings.Contains(js, g) {
			t.Errorf("particle engine perf guard missing: %s", g)
		}
	}
}

// TestCalAlmanac_TimepieceShelfMarkup — the timepiece is a horizontal
// shelf with the vertical hourglass centered + flanking text regions.
func TestCalAlmanac_TimepieceShelfMarkup(t *testing.T) {
	html := renderAlmanac(t)
	for _, h := range []string{
		"cal-almanac-shelf",
		"cal-almanac-shelf__stage",
		"cal-almanac-shelf__bar",
		"cal-almanac-shelf__flank--left",
		"cal-almanac-shelf__flank--right",
		"cal-almanac-shelf__plinth",
		"data-cal-shelf-frame",
	} {
		if !strings.Contains(html, h) {
			t.Errorf("shelf markup missing: %s", h)
		}
	}
	// The old vertical-only hourglass aside class should be gone.
	if strings.Contains(html, `class="cal-almanac-hourglass cal-almanac-sky`) {
		t.Errorf("v3 vertical hourglass root should be replaced by the shelf")
	}
}

// TestCalAlmanac_HourglassInternals — the glass shell (outline/rim/reflect) +
// the in-glass interior canvas are present; WAVE 1 drives the interior
// (heightmap sand + day/night) via the engine's per-surface frame hook.
func TestCalAlmanac_HourglassInternals(t *testing.T) {
	html := renderAlmanac(t)
	for _, h := range []string{
		"cal-almanac-hourglass__outline",
		"cal-almanac-hourglass__rim",
		"cal-almanac-hourglass__reflect",
		"data-cal-hourglass-canvas",
	} {
		if !strings.Contains(html, h) {
			t.Errorf("hourglass internals markup missing: %s", h)
		}
	}
	js := readCalAlmanacJS(t)
	if !strings.Contains(js, "registerInitBlock('hourglass-internals'") {
		t.Errorf("hourglass-internals init block missing")
	}
	if !strings.Contains(js, "feedHourglassStream") || !strings.Contains(js, "GLASS_SURFACE") {
		t.Errorf("in-glass canvas wiring missing")
	}
}

// TestCalAlmanac_CreateEditorOpens — the #5 regression guard: the empty-day
// click path mounts the create popover + escalates to the editor.
func TestCalAlmanac_CreateEditorOpens(t *testing.T) {
	html := renderAlmanac(t)
	for _, h := range []string{"data-cal-create", "data-cal-create-commit", "data-cal-create-expand"} {
		if !strings.Contains(html, h) {
			t.Errorf("create popover markup missing: %s", h)
		}
	}
	js := readCalAlmanacJS(t)
	for _, h := range []string{"registerInitBlock('popup-create-flow'", "openCreatePopup", "expandToEditor"} {
		if !strings.Contains(js, h) {
			t.Errorf("create-flow JS missing: %s", h)
		}
	}
}

// TestCalAlmanac_DemoControlsPanel — the beta-test panel is present on the
// demo route (it lives in the demo templ only).
func TestCalAlmanac_DemoControlsPanel(t *testing.T) {
	html := renderAlmanac(t)
	for _, h := range []string{
		"data-cal-democtl",
		"data-cal-democtl-era",
		"data-cal-democtl-weather",
		"data-cal-democtl-celestial",
		"data-cal-democtl-time",
		"data-cal-democtl-frame",
		"data-cal-democtl-profile",
	} {
		if !strings.Contains(html, h) {
			t.Errorf("demo-controls markup missing: %s", h)
		}
	}
	js := readCalAlmanacJS(t)
	if !strings.Contains(js, "registerInitBlock('demo-controls'") {
		t.Errorf("demo-controls init block missing")
	}
}

// TestCalAlmanac_ReducedMotionEngineGuard — the engine must not auto-start
// under reduced-motion; it renders one static frame instead.
func TestCalAlmanac_ReducedMotionEngineGuard(t *testing.T) {
	js := readCalAlmanacJS(t)
	if !strings.Contains(js, "drawStaticFrame") {
		t.Errorf("engine must have a static-frame path for reduced-motion")
	}
	if !strings.Contains(js, "reducedNow") {
		t.Errorf("engine must gate on reduced-motion (reducedNow)")
	}
	css := stripCalCSSComments(readCalAlmanacCSS(t))
	if !strings.Contains(css, "prefers-reduced-motion: reduce") {
		t.Errorf("reduced-motion media query missing")
	}
}

// TestCalAlmanac_V4ProofClasses — the dispatch's forced-state classes are
// all defined for the headless gate.
func TestCalAlmanac_V4ProofClasses(t *testing.T) {
	css := stripCalCSSComments(readCalAlmanacCSS(t))
	for _, klass := range []string{
		".cal-almanac--proof-era-rest",
		".cal-almanac--proof-era-hover",
		".cal-almanac--proof-era-responsive-small",
		".cal-almanac--proof-weather-rain-animated",
		".cal-almanac--proof-weather-fog-animated",
		".cal-almanac--proof-weather-snow-animated",
		".cal-almanac--proof-celestial-meteor-slow",
		".cal-almanac--proof-celestial-eclipse",
		".cal-almanac--proof-timepiece-shelf",
		".cal-almanac--proof-hourglass-internals",
		".cal-almanac--proof-time-edit-reachable",
		".cal-almanac--proof-create-editor-open",
		".cal-almanac--proof-create-editor-expanded",
		".cal-almanac--proof-demo-controls",
	} {
		if !strings.Contains(css, klass) {
			t.Errorf("v4 proof class missing: %s", klass)
		}
	}
}

// ============================================================
// REFINEMENT-V5 — painted sun prototype tests
// ============================================================

// TestCalAlmanacSun_StateResolution — the resolveSunState() JS pure
// function picks the right state across the precedence ladder
// (eclipse > special > dawn/dusk window > default). We assert against
// the embedded JS source rather than spinning a runtime; the resolver
// is tiny and visible.
func TestCalAlmanacSun_StateResolution(t *testing.T) {
	js := readCalAlmanacJS(t)
	if !strings.Contains(js, "function resolveSunState(") {
		t.Fatalf("resolveSunState() missing")
	}
	// Precedence checks present + bounds match dispatch spec.
	for _, h := range []string{
		`activeCelestial === 'eclipse-solar'`, `return 'eclipse'`,
		`isSpecialMoonDay`, `return 'special'`,
		`timeFrac > 0.20 && timeFrac < 0.32`, `return 'dawn'`,
		`timeFrac > 0.68 && timeFrac < 0.80`, `return 'dusk'`,
		`return 'default'`,
	} {
		if !strings.Contains(js, h) {
			t.Errorf("sun-state resolution missing: %s", h)
		}
	}
}

// TestCalAlmanacSun_InlineIcon — WAVE 1 supersession: the sun renders as an
// inline lorc/sun.svg icon (recolored per state via CSS), with the native
// lorc/eclipse.svg for the eclipse state. The painted-<picture>/WebP/PNG
// machinery is gone.
func TestCalAlmanacSun_InlineIcon(t *testing.T) {
	html := renderAlmanac(t)
	for _, frag := range []string{
		`data-cal-sun-state="default"`,
		`data-cal-sun-icon="sun"`,
		`data-cal-sun-icon="eclipse"`,
		`viewBox="0 0 512 512"`,
		`fill="currentColor"`,
	} {
		if !strings.Contains(html, frag) {
			t.Errorf("inline-SVG sun markup missing fragment: %s", frag)
		}
	}
	for _, gone := range []string{
		"<picture",
		"/static/img/cal-almanac/celestial/sun-",
		"data-cal-sun-state-img",
	} {
		if strings.Contains(html, gone) {
			t.Errorf("superseded painted-sun markup still present: %s", gone)
		}
	}
}

// TestCalAlmanacSun_VendoredIcons — the CC-BY icons are vendored locally (no
// runtime CDN) and attributed in CREDITS.md.
func TestCalAlmanacSun_VendoredIcons(t *testing.T) {
	root := calDemoRepoRoot(t)
	for _, p := range []string{
		filepath.Join(root, "static", "vendor", "game-icons", "lorc", "sun.svg"),
		filepath.Join(root, "static", "vendor", "game-icons", "lorc", "eclipse.svg"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("vendored icon missing: %s (%v)", p, err)
		}
	}
	credits, err := os.ReadFile(filepath.Join(root, "CREDITS.md"))
	if err != nil {
		t.Fatalf("CREDITS.md missing: %v", err)
	}
	for _, must := range []string{"game-icons", "CC-BY", "lorc/sun.svg"} {
		if !strings.Contains(string(credits), must) {
			t.Errorf("CREDITS.md missing attribution marker: %q", must)
		}
	}
	// No runtime CDN reference for the icons (inlined/vendored only).
	html := renderAlmanac(t)
	if strings.Contains(html, "jsdelivr") || strings.Contains(html, "cdn.jsdelivr") {
		t.Errorf("sun icon must be inlined/vendored, not hot-loaded from a CDN")
	}
}

// TestCalAlmanacSun_PaintedAssetsRemoved — the abandoned painted placeholders
// are gone from the repo (supersession cleanup).
func TestCalAlmanacSun_PaintedAssetsRemoved(t *testing.T) {
	dir := filepath.Join(calDemoRepoRoot(t), "static", "img", "cal-almanac", "celestial")
	for _, state := range []string{"default", "dawn", "dusk", "eclipse", "special"} {
		for _, ext := range []string{"webp", "png"} {
			p := filepath.Join(dir, "sun-"+state+"."+ext)
			if _, err := os.Stat(p); err == nil {
				t.Errorf("abandoned painted sun asset still present: %s", p)
			}
		}
	}
}

// TestCalAlmanacSun_ReducedMotion — under prefers-reduced-motion the sun icon
// freezes (rotation + pulse) and the canvas engine refuses to start its loop;
// the v5 state-resolution (resolveSunState) + canvas sun-bloom are preserved.
func TestCalAlmanacSun_ReducedMotion(t *testing.T) {
	css := stripCalCSSComments(readCalAlmanacCSS(t))
	if !strings.Contains(css, "@media (prefers-reduced-motion: reduce)") {
		t.Fatalf("reduced-motion media query missing")
	}
	if !strings.Contains(css, ".cal-almanac-sun__icon { animation: none !important") {
		t.Errorf("sun icon animation must be `animation: none !important` under reduced-motion")
	}
	js := readCalAlmanacJS(t)
	for _, keep := range []string{"reducedNow", "drawStaticFrame", "function resolveSunState", "sun-bloom"} {
		if !strings.Contains(js, keep) {
			t.Errorf("preserved engine/sun marker missing: %s", keep)
		}
	}
}

// TestCalAlmanacSun_BloomCelestialEntry — the sun-bloom celestial-effect
// entry exists, is alwaysActive, and ships a sunBloomSpec(state) variant.
func TestCalAlmanacSun_BloomCelestialEntry(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, h := range []string{
		`CELESTIAL_EFFECTS['sun-bloom']`,
		`alwaysActive: true`,
		`function sunBloomSpec(`,
	} {
		if !strings.Contains(js, h) {
			t.Errorf("sun-bloom celestial entry / spec missing: %s", h)
		}
	}
}

// TestCalAlmanacSun_BloomParticleCapSafe — the per-state max-alive
// parameter never exceeds the dispatch's cap of 14 for any sun state.
func TestCalAlmanacSun_BloomParticleCapSafe(t *testing.T) {
	js := readCalAlmanacJS(t)
	// The spec literal lives inside sunBloomSpec; pin the `maxAlive` lines.
	// Eclipse is the densest at 14; default/dawn/dusk/special ≤ 8.
	if !strings.Contains(js, "var maxAlive = state === 'eclipse' ? 14 : 8;") {
		t.Errorf("sun-bloom particle cap rule should pin 14 for eclipse, 8 otherwise")
	}
}

// TestCalAlmanacSun_DemoControlsDropdown — the showcase demo-controls panel
// gains a Sun-state dropdown so the operator can force any of the 5 states.
func TestCalAlmanacSun_DemoControlsDropdown(t *testing.T) {
	html := renderAlmanac(t)
	if !strings.Contains(html, "data-cal-democtl-sun") {
		t.Errorf("demo-controls sun-state dropdown missing")
	}
	for _, state := range []string{"default", "dawn", "dusk", "eclipse", "special"} {
		if !strings.Contains(html, `value="`+state+`"`) {
			t.Errorf("demo-controls dropdown option missing for state %q", state)
		}
	}
	js := readCalAlmanacJS(t)
	if !strings.Contains(js, "[data-cal-democtl-sun]") {
		t.Errorf("demo-controls JS handler for sun-state missing")
	}
}

// TestCalAlmanacSun_V5ProofClasses — all five painted-sun proof classes
// (one per state) exist in CSS for the headless screenshot gate.
func TestCalAlmanacSun_V5ProofClasses(t *testing.T) {
	css := stripCalCSSComments(readCalAlmanacCSS(t))
	for _, state := range []string{"default", "dawn", "dusk", "eclipse", "special"} {
		klass := ".cal-almanac--proof-sun-" + state
		if !strings.Contains(css, klass) {
			t.Errorf("v5 sun-state proof class missing: %s", klass)
		}
	}
}

// TestCalAlmanacSun_MockSpecialMoonDays — the mock data exposes a
// SpecialMoonDays list (the source for the painted sun-special state).
func TestCalAlmanacSun_MockSpecialMoonDays(t *testing.T) {
	m := CalAlmanacMock()
	if len(m.SpecialMoonDays) == 0 {
		t.Fatalf("CalAlmanacMockData.SpecialMoonDays should be seeded for the showcase")
	}
}

// ---- helpers ------------------------------------------------------

func stripCalCSSComments(src string) string {
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(src, "")
}
func renderAlmanac(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	if err := DemoCalendarAlmanac().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
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
