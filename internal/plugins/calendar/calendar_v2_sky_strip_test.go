// calendar_v2_sky_strip_test.go — C-CAL-SKY-STRIP. Covers the strip's server
// render: collapsed-by-default markup, the expanded pane's skybox widget
// mount, the no-worldstate glyph degrade, and the sync chip's loading
// skeleton (the chip's in-sync/stale/drift/never-synced STATE LOGIC is a
// pure JS function — test/js/calendar_v2_sky_strip.test.mjs — since the
// actual presence fetch + paint happens client-side; see the .templ file's
// package doc for why).
package calendar

import (
	"context"
	"strings"
	"testing"
)

func skyStripTestData(withWorldState bool) CalendarV2ViewData {
	cal := gregorian2026()
	data := CalendarV2ViewData{ActiveCalendar: cal, AllCalendars: []Calendar{*cal}, View: "month", Year: 2026, Month: 6, Day: 8, CampaignID: "camp-1"}
	if withWorldState {
		data.WorldState = &WorldStateSeed{
			Moons: []WorldStateMoon{
				{Name: "Selûne", Phase: 5, NamedPhase: "Waning Gibbous", CyclePct: 0.82},
			},
			Events: []WorldStateEvent{
				{Type: "meteor-shower", Name: "Duskfall meteors"},
			},
		}
	}
	return data
}

func renderSkyStrip(t *testing.T, data CalendarV2ViewData) string {
	t.Helper()
	var sb strings.Builder
	if err := calendarV2SkyStrip(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render sky strip: %v", err)
	}
	return sb.String()
}

// --- Nil calendar: nothing renders ---

func TestSkyStrip_NilCalendar_RendersNothing(t *testing.T) {
	html := renderSkyStrip(t, CalendarV2ViewData{})
	if strings.TrimSpace(html) != "" {
		t.Errorf("expected no output without an active calendar; got:\n%s", html)
	}
}

// --- Collapsed by default ---

func TestSkyStrip_CollapsedByDefault(t *testing.T) {
	html := renderSkyStrip(t, skyStripTestData(true))
	for _, want := range []string{
		`data-cal-sky-strip`,
		`data-cal-sky-strip-toggle`,
		`aria-expanded="false"`,
		`data-cal-sky-chevron`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("strip missing %q", want)
		}
	}
	// The pane exists but starts hidden — collapse state is client-toggled
	// (localStorage), not server-rendered as two variants.
	if !strings.Contains(html, `id="cal-sky-strip-pane" class="hidden`) {
		t.Error("expected the expanded pane to render present-but-hidden by default (collapsed)")
	}
}

// --- Expanded pane mounts the skybox widget (second consumer) ---

func TestSkyStrip_ExpandedPaneMountsSkyboxWidget(t *testing.T) {
	html := renderSkyStrip(t, skyStripTestData(true))
	if !strings.Contains(html, `data-widget="skybox"`) {
		t.Error("expanded pane should mount data-widget=\"skybox\" (the #543 widget, second consumer)")
	}
}

// --- No-worldstate degrade ---

func TestSkyStrip_NoWorldState_ShowsNoSkyData(t *testing.T) {
	html := renderSkyStrip(t, skyStripTestData(false))
	if !strings.Contains(html, `data-cal-sky-empty`) || !strings.Contains(html, "No sky data") {
		t.Errorf("expected the no-worldstate fallback text; got:\n%s", html)
	}
}

func TestSkyStrip_WithWorldState_RendersMoonAndEventGlyphs(t *testing.T) {
	html := renderSkyStrip(t, skyStripTestData(true))
	for _, want := range []string{"🌖", "Waning Gibbous", "☄", "Duskfall meteors"} {
		if !strings.Contains(html, want) {
			t.Errorf("strip missing glyph content %q; got:\n%s", want, html)
		}
	}
}

// --- Sync chip: loading skeleton (state logic is JS-side; see the .test.mjs) ---

func TestSkyStrip_SyncChipRendersLoadingSkeleton(t *testing.T) {
	html := renderSkyStrip(t, skyStripTestData(true))
	for _, want := range []string{`data-cal-sky-sync-chip`, `data-cal-sky-sync-dot`, `data-cal-sky-sync-word`, `data-cal-sky-sync-detail`, "Checking"} {
		if !strings.Contains(html, want) {
			t.Errorf("sync chip missing %q", want)
		}
	}
	// Mobile collapse (dispatch item 4): the detail suffix is sm+ only; the
	// word alone is the mobile-collapsed form.
	if !strings.Contains(html, `class="hidden sm:inline" data-cal-sky-sync-detail`) {
		t.Error("sync chip detail suffix should be hidden below sm (mobile collapses to dot + word)")
	}
}

func TestSkyStrip_SyncNowButton_NoNetworkCallWiredServerSide(t *testing.T) {
	html := renderSkyStrip(t, skyStripTestData(true))
	if !strings.Contains(html, `data-cal-sky-sync-now`) {
		t.Error("missing the Sync-now button hook")
	}
	if !strings.Contains(html, `data-cal-sky-sync-help`) {
		t.Error("missing the copy-help text the button reveals (no push pipeline — dispatch scope)")
	}
}

// --- Helper unit tests ---

func TestSkyStripMoonPhaseGlyph_AllEightPhases(t *testing.T) {
	want := []string{"🌑", "🌒", "🌓", "🌔", "🌕", "🌖", "🌗", "🌘"}
	for i, g := range want {
		if got := skyStripMoonPhaseGlyph(i); got != g {
			t.Errorf("phase %d = %q; want %q", i, got, g)
		}
	}
}

func TestSkyStripMoonPhaseGlyph_OutOfRangeFallsBack(t *testing.T) {
	if got := skyStripMoonPhaseGlyph(99); got != "🌙" {
		t.Errorf("out-of-range phase = %q; want fallback 🌙", got)
	}
	if got := skyStripMoonPhaseGlyph(-1); got != "🌙" {
		t.Errorf("negative phase = %q; want fallback 🌙", got)
	}
}

func TestSkyStripEventGlyph_KnownTypes(t *testing.T) {
	cases := map[string]string{
		"meteor-shower": "☄",
		"eclipse-solar": "◑",
		"blood-moon":    "●",
		"supermoon":     "○",
		"aurora":        "✦",
		"totally-novel": "✦", // unlisted type falls back to the generic star
	}
	for typ, want := range cases {
		if got := skyStripEventGlyph(typ); got != want {
			t.Errorf("skyStripEventGlyph(%q) = %q; want %q", typ, got, want)
		}
	}
}

func TestSkyStripMoonGlyphs_NilWorldStateIsEmpty(t *testing.T) {
	data := CalendarV2ViewData{}
	if got := skyStripMoonGlyphs(data); got != nil {
		t.Errorf("expected nil for no worldstate; got %+v", got)
	}
}

func TestSkyStripEventGlyphs_SkipsUnnamedEvents(t *testing.T) {
	data := CalendarV2ViewData{WorldState: &WorldStateSeed{Events: []WorldStateEvent{
		{Type: "meteor-shower", Name: ""},
		{Type: "aurora", Name: "Northern lights"},
	}}}
	got := skyStripEventGlyphs(data)
	if len(got) != 1 || got[0].Label != "Northern lights" {
		t.Errorf("expected only the named event; got %+v", got)
	}
}

func TestSkyStripAllGlyphs_MoonsBeforeEvents(t *testing.T) {
	data := skyStripTestData(true)
	got := skyStripAllGlyphs(data)
	if len(got) != 2 {
		t.Fatalf("expected 2 glyphs; got %d", len(got))
	}
	if got[0].Label != "Waning Gibbous" || got[1].Label != "Duskfall meteors" {
		t.Errorf("expected moons before events; got %+v", got)
	}
}

func TestSkyStripActive_MirrorsWsActive(t *testing.T) {
	if skyStripActive(skyStripTestData(false)) {
		t.Error("expected inactive without a worldstate seed")
	}
	if !skyStripActive(skyStripTestData(true)) {
		t.Error("expected active with a worldstate seed + calendar")
	}
}
