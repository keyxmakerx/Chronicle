// subresource_v2_b_test.go covers V2 Wave 1 PR 3 (C-CAL-V2-
// SUBRESOURCE-CARDS-B) extensions: 6 new SubresourceKind cases +
// per-resource projections + drawer field branches + the weather-
// singular render path. Mirrors the test patterns from
// subresource_v2_test.go (PR #364).

package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- Per-resource projection tests ---

func TestErasToCards_OngoingAndBounded(t *testing.T) {
	endYear := 1500
	eras := []Era{
		{Name: "Age of Magic", StartYear: 1000, EndYear: &endYear, Color: "#abc"},
		{Name: "Ongoing Era", StartYear: 1500, EndYear: nil, Color: "#def"},
	}
	cards := erasToCards(eras)
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards; got %d", len(cards))
	}
	if !strings.Contains(cards[0].Subtitle, "1000") || !strings.Contains(cards[0].Subtitle, "1500") {
		t.Errorf("bounded era subtitle = %q; want year range", cards[0].Subtitle)
	}
	if !strings.Contains(cards[1].Subtitle, "ongoing") {
		t.Errorf("ongoing era subtitle = %q; want 'ongoing'", cards[1].Subtitle)
	}
	if cards[0].Color != "#abc" {
		t.Errorf("expected color propagation")
	}
}

func TestCategoriesToCards_SubtitleHasSlugAndIcon(t *testing.T) {
	cats := []EventCategory{
		{Name: "Festival", Slug: "festival", Icon: "fa-party-horn", Color: "#ff8"},
		{Name: "Combat", Slug: "combat"},
	}
	cards := categoriesToCards(cats)
	if !strings.Contains(cards[0].Subtitle, "festival") || !strings.Contains(cards[0].Subtitle, "fa-party-horn") {
		t.Errorf("category subtitle = %q; want slug + icon", cards[0].Subtitle)
	}
	if cards[1].Subtitle != "combat" {
		t.Errorf("icon-less category subtitle = %q; want just slug", cards[1].Subtitle)
	}
}

func TestFestivalsToCards_DateAndIntercalary(t *testing.T) {
	month3 := 3
	day15 := 15
	afterMonth5 := 5
	fests := []Festival{
		{Name: "Spring Eve", Month: &month3, Day: &day15},
		{Name: "Intercalary", AfterMonth: &afterMonth5},
		{Name: "No Date Set"},
	}
	cards := festivalsToCards(fests)
	if !strings.Contains(cards[0].Subtitle, "month 3") || !strings.Contains(cards[0].Subtitle, "day 15") {
		t.Errorf("month+day festival subtitle = %q", cards[0].Subtitle)
	}
	if !strings.Contains(cards[1].Subtitle, "intercalary") || !strings.Contains(cards[1].Subtitle, "5") {
		t.Errorf("intercalary festival subtitle = %q", cards[1].Subtitle)
	}
	if cards[2].Subtitle != "no date set" {
		t.Errorf("dateless festival subtitle = %q; want 'no date set'", cards[2].Subtitle)
	}
}

func TestCyclesToCards_EntryCountAndLength(t *testing.T) {
	cycles := []Cycle{
		{Name: "Endlean", Type: "yearly", CycleLength: 11, Entries: []CycleEntry{{Name: "A"}, {Name: "B"}, {Name: "C"}}},
		{Name: "Empty", Type: "monthly", CycleLength: 0},
	}
	cards := cyclesToCards(cycles)
	if !strings.Contains(cards[0].Subtitle, "yearly") || !strings.Contains(cards[0].Subtitle, "length 11") || !strings.Contains(cards[0].Subtitle, "3 entries") {
		t.Errorf("cycle subtitle = %q; want type + length + entries", cards[0].Subtitle)
	}
	if !strings.Contains(cards[1].Subtitle, "monthly") {
		t.Errorf("empty-cycle subtitle = %q; want type", cards[1].Subtitle)
	}
}

func TestZonesToCards_ActiveZoneAccentAndPresets(t *testing.T) {
	zones := []WeatherZone{
		{ZoneID: "temperate", Name: "Temperate", Payload: map[string]any{
			"presets": []any{map[string]any{"label": "Clear"}, map[string]any{"label": "Rain"}},
		}},
		{ZoneID: "arctic", Name: "Arctic"},
	}
	cards := zonesToCards(zones, "arctic")
	if !strings.Contains(cards[0].Subtitle, "temperate") || !strings.Contains(cards[0].Subtitle, "2 presets") {
		t.Errorf("zone subtitle = %q; want id + preset count", cards[0].Subtitle)
	}
	if cards[0].IsAccent {
		t.Error("non-active zone should not be accent")
	}
	if !cards[1].IsAccent {
		t.Error("active zone should be marked accent")
	}
	// IDs use ZoneID (slug), not positional index — important for
	// dnd state preservation across zone renames.
	if cards[0].ID != "temperate" || cards[1].ID != "arctic" {
		t.Errorf("zone card IDs should equal zone_id; got %q / %q", cards[0].ID, cards[1].ID)
	}
}

// --- Helper tests ---

func TestSubresourcePUTPath_CategoriesAndZonesOverridden(t *testing.T) {
	cases := map[SubresourceKind]string{
		SubresourceMonths:     "/campaigns/c/calendars/k/months",
		SubresourceCategories: "/campaigns/c/calendars/k/event-categories",
		SubresourceZones:      "/campaigns/c/calendars/k/weather/zones",
		SubresourceWeather:    "/campaigns/c/calendars/k/weather",
	}
	for k, want := range cases {
		if got := subresourcePUTPath("c", "k", k); got != want {
			t.Errorf("PUT path for %q = %q; want %q", k, got, want)
		}
	}
}

func TestSubresourceTitle_BatchB(t *testing.T) {
	cases := map[SubresourceKind]string{
		SubresourceEras:       "Eras",
		SubresourceCategories: "Event Categories",
		SubresourceFestivals:  "Festivals",
		SubresourceCycles:     "Cycles",
		SubresourceZones:      "Weather Zones",
		SubresourceWeather:    "Weather",
	}
	for k, want := range cases {
		if got := subresourceTitle(k); got != want {
			t.Errorf("title(%q) = %q; want %q", k, got, want)
		}
	}
}

func TestKindIsSingular_OnlyWeather(t *testing.T) {
	if !SubresourceWeather.isSingular() {
		t.Error("Weather should be singular")
	}
	for _, k := range []SubresourceKind{
		SubresourceMonths, SubresourceWeekdays, SubresourceMoons, SubresourceSeasons,
		SubresourceEras, SubresourceCategories, SubresourceFestivals, SubresourceCycles,
		SubresourceZones,
	} {
		if k.isSingular() {
			t.Errorf("%q should NOT be singular", k)
		}
	}
}

func TestSubresourcePayloadJSON_WeatherSingular(t *testing.T) {
	preset := "rain"
	temp := 15.5
	data := SubresourceViewData{
		Kind:    SubresourceWeather,
		Weather: &Weather{PresetID: &preset, TemperatureCelsius: &temp},
	}
	got := subresourcePayloadJSON(data)
	// Singular kinds serialize to object, not array.
	if strings.HasPrefix(got, "[") {
		t.Errorf("weather singular should serialize as object; got %q", got)
	}
	var roundTrip Weather
	if err := json.Unmarshal([]byte(got), &roundTrip); err != nil {
		t.Fatalf("weather JSON not parseable: %v (got %q)", err, got)
	}
	if roundTrip.PresetID == nil || *roundTrip.PresetID != "rain" {
		t.Errorf("preset_id round-trip lost; got %+v", roundTrip)
	}
}

func TestSubresourcePayloadJSON_WeatherNilReturnsEmptyObject(t *testing.T) {
	data := SubresourceViewData{Kind: SubresourceWeather, Weather: nil}
	got := subresourcePayloadJSON(data)
	if got != "{}" {
		t.Errorf("nil weather should serialize as '{}'; got %q", got)
	}
}

func TestSingularHeading_PrefersLabelThenIdThenZone(t *testing.T) {
	label := "Steady rain"
	id := "rain"
	zone := "Temperate"
	cases := []struct {
		name string
		w    *Weather
		want string
	}{
		{"nil weather", nil, "No weather state"},
		{"label wins", &Weather{PresetLabel: &label, PresetID: &id, ZoneName: &zone}, "Steady rain"},
		{"id wins over zone", &Weather{PresetID: &id, ZoneName: &zone}, "rain"},
		{"zone fallback", &Weather{ZoneName: &zone}, "Temperate zone"},
		{"no preset no zone", &Weather{}, "Weather state (no preset)"},
	}
	for _, tc := range cases {
		got := singularHeading(SubresourceViewData{Weather: tc.w})
		if got != tc.want {
			t.Errorf("%s: heading = %q; want %q", tc.name, got, tc.want)
		}
	}
}

func TestSingularDetailLines_FormatsFields(t *testing.T) {
	temp := 15.0
	zoneName := "Temperate"
	desc := "Light rain falls."
	w := &Weather{TemperatureCelsius: &temp, ZoneName: &zoneName, Description: &desc}
	lines := singularDetailLines(SubresourceViewData{Weather: w})
	if len(lines) != 3 {
		t.Fatalf("expected 3 detail lines; got %d: %+v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "15") || !strings.Contains(lines[0], "°C") {
		t.Errorf("temperature line = %q", lines[0])
	}
	if !strings.Contains(lines[1], "Temperate") {
		t.Errorf("zone line = %q", lines[1])
	}
}

func TestSingularDetailLines_EmptyWeatherShowsHint(t *testing.T) {
	lines := singularDetailLines(SubresourceViewData{Weather: nil})
	if len(lines) == 0 || !strings.Contains(lines[0], "Click") {
		t.Errorf("nil weather should hint operator to set state; got %+v", lines)
	}
}

func TestFormatFloat_DropsTrailingZero(t *testing.T) {
	cases := map[float64]string{
		15.0:  "15",
		15.5:  "15.5",
		-3.0:  "-3",
		0.0:   "0",
	}
	for f, want := range cases {
		if got := formatFloat(f); got != want {
			t.Errorf("formatFloat(%v) = %q; want %q", f, got, want)
		}
	}
}

// --- Handler tests ---

// stubFestivalCycleSvc embeds CalendarService + overrides GetFestivals
// and GetCycles since the calendar plugin's mock repo doesn't load
// these via the eager-load path (festivals + cycles are loaded
// separately via repo.GetFestivals + repo.GetCycles).
type stubFestivalCycleSvc struct {
	CalendarService
	cal       *Calendar
	festivals []Festival
	cycles    []Cycle
	weather   *Weather
	zones     *WeatherZonesState
}

func (s *stubFestivalCycleSvc) GetCalendarByID(_ context.Context, id string) (*Calendar, error) {
	if s.cal != nil && s.cal.ID == id {
		return s.cal, nil
	}
	return nil, nil
}
func (s *stubFestivalCycleSvc) GetFestivals(_ context.Context, _ string) ([]Festival, error) {
	return s.festivals, nil
}
func (s *stubFestivalCycleSvc) GetCycles(_ context.Context, _ string) ([]Cycle, error) {
	return s.cycles, nil
}
func (s *stubFestivalCycleSvc) GetWeather(_ context.Context, _ string) (*Weather, error) {
	return s.weather, nil
}
func (s *stubFestivalCycleSvc) GetWeatherZones(_ context.Context, _ string) (*WeatherZonesState, error) {
	return s.zones, nil
}

func TestShowV2SubresourceSettings_ErasPath(t *testing.T) {
	cal := &Calendar{
		ID: "cal-1", CampaignID: "camp-1", Name: "Test",
		Eras: []Era{{Name: "Age of Magic", StartYear: 1000}},
	}
	svc := &stubFestivalCycleSvc{cal: cal}
	h := NewHandler(svc)
	c, rec := newSubresourceReq("eras")
	if err := h.ShowV2SubresourceSettings(c); err != nil {
		t.Fatalf("ShowV2SubresourceSettings: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Age of Magic") {
		t.Errorf("response missing era name; got %d bytes", len(body))
	}
}

func TestShowV2SubresourceSettings_FestivalsPath(t *testing.T) {
	month := 3
	day := 15
	cal := &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Test"}
	svc := &stubFestivalCycleSvc{
		cal:       cal,
		festivals: []Festival{{Name: "Spring Eve", Month: &month, Day: &day}},
	}
	h := NewHandler(svc)
	c, rec := newSubresourceReq("festivals")
	if err := h.ShowV2SubresourceSettings(c); err != nil {
		t.Fatalf("ShowV2SubresourceSettings: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Spring Eve") {
		t.Errorf("response missing festival name; got %d bytes", len(body))
	}
}

func TestShowV2SubresourceSettings_CyclesPath(t *testing.T) {
	cal := &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Test"}
	svc := &stubFestivalCycleSvc{
		cal:    cal,
		cycles: []Cycle{{Name: "Endlean", Type: "yearly", CycleLength: 11}},
	}
	h := NewHandler(svc)
	c, rec := newSubresourceReq("cycles")
	if err := h.ShowV2SubresourceSettings(c); err != nil {
		t.Fatalf("ShowV2SubresourceSettings: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Endlean") {
		t.Errorf("response missing cycle name; got %d bytes", len(body))
	}
}

func TestShowV2SubresourceSettings_ZonesPath(t *testing.T) {
	cal := &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Test"}
	svc := &stubFestivalCycleSvc{
		cal: cal,
		zones: &WeatherZonesState{
			ActiveZone: "temperate",
			Zones:      []WeatherZone{{ZoneID: "temperate", Name: "Temperate"}},
		},
	}
	h := NewHandler(svc)
	c, rec := newSubresourceReq("zones")
	if err := h.ShowV2SubresourceSettings(c); err != nil {
		t.Fatalf("ShowV2SubresourceSettings: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Temperate") {
		t.Errorf("response missing zone name; got %d bytes", len(body))
	}
}

func TestShowV2SubresourceSettings_WeatherSingularPath(t *testing.T) {
	cal := &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Test"}
	preset := "Steady rain"
	temp := 12.0
	svc := &stubFestivalCycleSvc{
		cal:     cal,
		weather: &Weather{PresetLabel: &preset, TemperatureCelsius: &temp},
		zones: &WeatherZonesState{
			Zones: []WeatherZone{{ZoneID: "temperate", Name: "Temperate"}},
		},
	}
	h := NewHandler(svc)
	c, rec := newSubresourceReq("weather")
	if err := h.ShowV2SubresourceSettings(c); err != nil {
		t.Fatalf("ShowV2SubresourceSettings: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Steady rain") {
		t.Errorf("response missing weather preset label; got %d bytes", len(body))
	}
	if !strings.Contains(body, "12") {
		t.Errorf("response missing temperature; got %d bytes", len(body))
	}
	if !strings.Contains(body, "data-subresource-singular=\"true\"") {
		t.Error("response should mark singular kind via data-attribute")
	}
}

func TestShowV2SubresourceSettings_WeatherWithoutZonesShowsAffordance(t *testing.T) {
	cal := &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Test"}
	svc := &stubFestivalCycleSvc{
		cal:     cal,
		weather: nil,
		zones:   &WeatherZonesState{Zones: nil},
	}
	h := NewHandler(svc)
	c, rec := newSubresourceReq("weather")
	if err := h.ShowV2SubresourceSettings(c); err != nil {
		t.Fatalf("ShowV2SubresourceSettings: %v", err)
	}
	body := rec.Body.String()
	// "Configure zones first" affordance only shows for Owners + no zones.
	if !strings.Contains(body, "Configure zones first") {
		t.Errorf("expected zones-first affordance when no zones configured; got %d bytes", len(body))
	}
}

// newSubresourceReq builds an echo.Context for a V2 subresource page,
// shared across the batch-B handler tests.
func newSubresourceReq(resource string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "calId", "resource")
	c.SetParamValues("camp-1", "cal-1", resource)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner,
	})
	c.Set("auth_user_id", "user-1")
	return c, rec
}
