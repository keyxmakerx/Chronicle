// export_calendar_roundtrip_test.go proves campaign export/import carries the
// calendar section: it fails (zero calendar exported, nothing recreated) if
// either the calendar export/import adapter or its wiring call is removed,
// the same regression shape as export_notes_roundtrip_test.go.
package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeCalendarService is an in-memory calendar.CalendarService. It embeds the
// interface so only the methods the export/import adapters actually call
// need bodies; any other call would panic loudly rather than silently no-op
// — the same pattern export_notes_roundtrip_test.go uses for notes.
type fakeCalendarService struct {
	calendar.CalendarService

	// --- export-side fixture ---
	cal    *calendar.Calendar
	events []calendar.Event

	// --- import-side captured state ---
	created       *calendar.Calendar
	defaultSetFor string
	months        []calendar.MonthInput
	weekdays      []calendar.WeekdayInput
	moons         []calendar.MoonInput
	seasons       []calendar.Season
	eras          []calendar.EraInput
	cycles        []calendar.CycleInput
	festivals     []calendar.FestivalInput
	weather       *calendar.WeatherInput
	kinds         []calendar.EventKindInput
	createdEvents []calendar.CreateEventInput
	updateInput   *calendar.UpdateCalendarInput
}

func (f *fakeCalendarService) GetDefaultCalendarForViewer(_ context.Context, _ string, _ permissions.Viewer) (*calendar.Calendar, error) {
	if f.cal == nil {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return f.cal, nil
}

func (f *fakeCalendarService) ListCalendars(_ context.Context, _ string, _ permissions.Viewer) ([]calendar.Calendar, error) {
	if f.cal == nil {
		return nil, nil
	}
	return []calendar.Calendar{*f.cal}, nil
}

func (f *fakeCalendarService) GetCalendarForViewer(_ context.Context, _ string, _ string, _ permissions.Viewer) (*calendar.Calendar, error) {
	if f.cal == nil {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return f.cal, nil
}

func (f *fakeCalendarService) ListAllEventsForCalendar(_ context.Context, _ string, _ string, v permissions.Viewer) ([]calendar.Event, error) {
	if !v.IsSystem() {
		return nil, apperror.NewForbidden("system access only")
	}
	return f.events, nil
}

func (f *fakeCalendarService) CreateCalendar(_ context.Context, campaignID string, input calendar.CreateCalendarInput) (*calendar.Calendar, error) {
	visibility := input.Visibility
	if visibility == "" {
		visibility = "everyone"
	}
	f.created = &calendar.Calendar{
		ID: "new-cal", CampaignID: campaignID,
		Mode: input.Mode, Name: input.Name, Description: input.Description,
		EpochName: input.EpochName, CurrentYear: input.CurrentYear,
		HoursPerDay: input.HoursPerDay, MinutesPerHour: input.MinutesPerHour,
		SecondsPerMinute: input.SecondsPerMinute,
		LeapYearEvery:    input.LeapYearEvery, LeapYearOffset: input.LeapYearOffset,
		Visibility:      visibility,
		VisibilityRules: input.VisibilityRules,
	}
	return f.created, nil
}

func (f *fakeCalendarService) SetDefaultCalendar(_ context.Context, _ string, calendarID string) error {
	f.defaultSetFor = calendarID
	return nil
}

func (f *fakeCalendarService) SetMonths(_ context.Context, _ string, _ string, months []calendar.MonthInput) error {
	f.months = months
	return nil
}

func (f *fakeCalendarService) SetWeekdays(_ context.Context, _ string, _ string, weekdays []calendar.WeekdayInput) error {
	f.weekdays = weekdays
	return nil
}

func (f *fakeCalendarService) SetMoons(_ context.Context, _ string, _ string, moons []calendar.MoonInput) error {
	f.moons = moons
	return nil
}

func (f *fakeCalendarService) SetSeasons(_ context.Context, _ string, _ string, seasons []calendar.Season) error {
	f.seasons = seasons
	return nil
}

func (f *fakeCalendarService) SetCycles(_ context.Context, _ string, _ string, cycles []calendar.CycleInput) error {
	f.cycles = cycles
	return nil
}

func (f *fakeCalendarService) SetFestivals(_ context.Context, _ string, _ string, festivals []calendar.FestivalInput) error {
	f.festivals = festivals
	return nil
}

func (f *fakeCalendarService) SetWeather(_ context.Context, _ string, _ string, input calendar.WeatherInput) error {
	f.weather = &input
	return nil
}

func (f *fakeCalendarService) CreateEra(_ context.Context, _ string, _ string, input calendar.EraInput) (*calendar.Era, error) {
	f.eras = append(f.eras, input)
	return &calendar.Era{ID: len(f.eras), Name: input.Name}, nil
}

func (f *fakeCalendarService) CreateEventKind(_ context.Context, _ string, input calendar.EventKindInput) (*calendar.EventKind, error) {
	f.kinds = append(f.kinds, input)
	return &calendar.EventKind{ID: len(f.kinds), Slug: input.Slug, Name: input.Name}, nil
}

func (f *fakeCalendarService) UpdateCalendar(_ context.Context, _ string, _ string, input calendar.UpdateCalendarInput) error {
	f.updateInput = &input
	if f.created != nil {
		f.created.CurrentMonth = input.CurrentMonth.Val(f.created.CurrentMonth)
		f.created.CurrentDay = input.CurrentDay.Val(f.created.CurrentDay)
		f.created.CurrentHour = input.CurrentHour.Val(f.created.CurrentHour)
		f.created.CurrentMinute = input.CurrentMinute.Val(f.created.CurrentMinute)
		f.created.Hemisphere = input.Hemisphere.Ptr(f.created.Hemisphere)
		f.created.ForecastsEnabled = input.ForecastsEnabled.Val(f.created.ForecastsEnabled)
		f.created.MonthStartsNewWeek = input.MonthStartsNewWeek.Val(f.created.MonthStartsNewWeek)
		if input.SetRealTime != nil {
			f.created.TracksRealTime = *input.SetRealTime
			f.created.RealTimeZone = input.RealTimeZone
		}
	}
	return nil
}

func (f *fakeCalendarService) CreateEvent(_ context.Context, _ string, _ string, input calendar.CreateEventInput) (*calendar.Event, error) {
	f.createdEvents = append(f.createdEvents, input)
	return &calendar.Event{ID: "evt-" + input.Name}, nil
}

// TestCampaignExportImport_CalendarRoundTrip is the regression: a campaign
// with a calendar (months, weekdays, a hidden moon, a season, a day-granular
// era, event kinds and both an everyone and a dm_only event) must export
// that calendar and rebuild it on import.
func TestCampaignExportImport_CalendarRoundTrip(t *testing.T) {
	desc := "The world's calendar"
	epoch := "Age of Sail"
	entityID := "entity-77"
	startHour, startMinute := 9, 30
	visRules := `{"denied_users":["player-1"]}`
	hemisphere := calendar.HemisphereSouth
	moonTint := "#ffccaa"
	weatherIcon := "cloud-rain"

	src := &fakeCalendarService{
		cal: &calendar.Calendar{
			ID: "cal-1", CampaignID: "c1", Mode: calendar.ModeFantasy,
			Name: "Harvest Calendar", Description: &desc, EpochName: &epoch,
			CurrentYear: 998, CurrentMonth: 3, CurrentDay: 12,
			CurrentHour: 14, CurrentMinute: 5,
			HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
			LeapYearEvery: 4, LeapYearOffset: 0,
			// #805 settings: Hemisphere/ForecastsEnabled/MonthStartsNewWeek
			// used to be dropped entirely.
			Hemisphere: &hemisphere, ForecastsEnabled: true, MonthStartsNewWeek: true,
			Months: []calendar.Month{
				{Name: "Thaw", Days: 30, SortOrder: 0},
				{Name: "Bloom", Days: 30, SortOrder: 1, LeapYearDays: 1},
			},
			Weekdays: []calendar.Weekday{
				{Name: "Sunday", SortOrder: 0}, {Name: "Moonday", SortOrder: 1},
			},
			Moons: []calendar.Moon{
				{
					ID: 1, Name: "Secret Moon", CycleDays: 29.5, Color: "#ffffff", HiddenFromPlayers: true,
					// #805 look fields: used to be dropped, so a restored
					// moon rendered with the library default look.
					BaseDesign: "moon-cratered", Tint: &moonTint, PhaseSource: "canvas-arc",
					Size: 1.4, OrbitSpeed: 0.8,
				},
			},
			Seasons: []calendar.Season{
				{Name: "Spring", StartMonth: 1, StartDay: 1, EndMonth: 2, EndDay: 30, Color: "#22c55e"},
			},
			Eras: []calendar.Era{
				{ID: 1, Name: "First Age", StartYear: 0, StartMonth: 1, StartDay: 1, Color: "#eab308"},
			},
			EventKinds: []calendar.EventKind{
				{ID: 1, Slug: "festival", Name: "Festival", Icon: "fa-star", Color: "#10b981", DefaultAnnounced: calendar.AnnouncedAhead},
			},
			// #805 sub-resources: used to be dropped entirely on export.
			Cycles: []calendar.Cycle{
				{Name: "Zodiac", CycleLength: 12, Type: "yearly", Entries: []calendar.CycleEntry{
					{Name: "Rat", YearOffset: 0},
				}},
			},
			Festivals: []calendar.Festival{
				{Name: "Founding Day", Month: intPtr(1), Day: intPtr(1)},
			},
			Weather: &calendar.Weather{
				Icon: &weatherIcon, TemperatureCelsius: float64Ptr(12.5),
				Wind: &calendar.Wind{SpeedKPH: float64Ptr(20)},
			},
		},
		events: []calendar.Event{
			{
				ID: "evt-1", Name: "Harvest Festival", EntityID: &entityID,
				Year: 998, Month: 2, Day: 20, StartHour: &startHour, StartMinute: &startMinute,
				Visibility: "everyone", KindID: intPtr(1),
			},
			{
				ID: "evt-2", Name: "Secret War Council", Year: 998, Month: 3, Day: 1,
				Visibility: "dm_only", VisibilityRules: &visRules,
			},
		},
	}

	// slugLookup stands in for the entity export adapter's real one — this
	// test exercises the calendar adapter directly rather than through the
	// full ExportImportService (entity export/import isn't wired here).
	slugLookup := func(id string) string {
		if id == entityID {
			return "the-duke"
		}
		return ""
	}

	exportAdapter := &calendarExportAdapter{svc: src}
	calData, err := exportAdapter.ExportCalendar(context.Background(), "c1", slugLookup)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if calData == nil {
		t.Fatal("exported campaign has no calendar section")
	}
	if len(calData.Events) != 2 {
		t.Fatalf("exported %d events, want 2", len(calData.Events))
	}
	if len(calData.Moons) != 1 || calData.Moons[0].HiddenFromPlayers == nil || !*calData.Moons[0].HiddenFromPlayers {
		t.Errorf("exported moon lost its HiddenFromPlayers flag: %+v", calData.Moons)
	}
	if len(calData.Eras) != 1 || calData.Eras[0].StartMonth != 1 || calData.Eras[0].StartDay != 1 {
		t.Errorf("exported era lost its day-granular start: %+v", calData.Eras)
	}

	// #805: settings, moon look fields, cycles, festivals and weather must
	// all reach the export — none of them did before.
	if calData.Hemisphere == nil || *calData.Hemisphere != hemisphere {
		t.Errorf("exported calendar lost its hemisphere: %+v", calData.Hemisphere)
	}
	if !calData.ForecastsEnabled || !calData.MonthStartsNewWeek {
		t.Errorf("exported calendar lost forecasts_enabled/month_starts_new_week: %+v/%v", calData.ForecastsEnabled, calData.MonthStartsNewWeek)
	}
	if len(calData.Moons) != 1 || calData.Moons[0].BaseDesign != "moon-cratered" || calData.Moons[0].Tint == nil || *calData.Moons[0].Tint != moonTint ||
		calData.Moons[0].PhaseSource != "canvas-arc" || calData.Moons[0].Size != 1.4 || calData.Moons[0].OrbitSpeed != 0.8 {
		t.Errorf("exported moon lost its look fields: %+v", calData.Moons)
	}
	if len(calData.Cycles) != 1 || calData.Cycles[0].Name != "Zodiac" || len(calData.Cycles[0].Entries) != 1 {
		t.Errorf("exported calendar lost its cycles: %+v", calData.Cycles)
	}
	if len(calData.Festivals) != 1 || calData.Festivals[0].Name != "Founding Day" {
		t.Errorf("exported calendar lost its festivals: %+v", calData.Festivals)
	}
	if calData.Weather == nil || calData.Weather.Icon == nil || *calData.Weather.Icon != weatherIcon {
		t.Errorf("exported calendar lost its weather: %+v", calData.Weather)
	}
	if len(calData.EventCategories) != 1 || calData.EventCategories[0].DefaultAnnounced != calendar.AnnouncedAhead {
		t.Errorf("exported event category lost its default_announced: %+v", calData.EventCategories)
	}

	// The envelope must survive a JSON round trip: this is what lands on disk.
	env := &campaigns.CampaignExport{Format: campaigns.ExportFormat, Version: campaigns.ExportVersion, Calendar: calData}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var reloaded campaigns.CampaignExport
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if reloaded.Calendar == nil {
		t.Fatal("reloaded envelope lost its calendar section")
	}

	var dmEvent, publicEvent *campaigns.ExportCalendarEvent
	for i := range reloaded.Calendar.Events {
		e := &reloaded.Calendar.Events[i]
		switch e.Name {
		case "Secret War Council":
			dmEvent = e
		case "Harvest Festival":
			publicEvent = e
		}
	}
	if dmEvent == nil {
		t.Fatal("dm_only event missing from export — a backup must not silently drop DM-only content")
	}
	if dmEvent.Visibility != "dm_only" {
		t.Errorf("dm_only event's visibility = %q, want dm_only", dmEvent.Visibility)
	}
	if dmEvent.VisibilityRules == nil || *dmEvent.VisibilityRules != visRules {
		t.Error("dm_only event lost its per-user visibility_rules")
	}
	if publicEvent == nil {
		t.Fatal("public event missing from export")
	}
	if publicEvent.EntitySlug == nil || *publicEvent.EntitySlug != "the-duke" {
		t.Errorf("public event lost its entity link: %+v", publicEvent.EntitySlug)
	}
	if publicEvent.Category == nil || *publicEvent.Category != "festival" {
		t.Errorf("public event lost its category: %+v", publicEvent.Category)
	}

	// --- Import side ---
	dst := &fakeCalendarService{}
	idMap := campaigns.NewIDMap("imported-campaign")
	idMap.EntitySlugToID["the-duke"] = "new-entity-77"
	report := campaigns.NewImportReport()
	if err := (&calendarImportAdapter{svc: dst}).ImportCalendar(context.Background(), "imported-campaign", reloaded.Calendar, idMap, report); err != nil {
		t.Fatalf("import calendar: %v", err)
	}
	if report.HasFailures() {
		t.Errorf("clean calendar import reported %d failures: %s", report.Count(), report.Summary())
	}
	if dst.created == nil {
		t.Fatal("ImportCalendar did not create a calendar")
	}
	if dst.defaultSetFor != dst.created.ID {
		t.Error("imported calendar was not marked default — dashboard/skybox reads would find nothing")
	}
	if len(dst.months) != 2 || len(dst.weekdays) != 2 {
		t.Errorf("imported calendar lost its months/weekdays: months=%d weekdays=%d", len(dst.months), len(dst.weekdays))
	}
	if len(dst.moons) != 1 || !dst.moons[0].HiddenFromPlayers {
		t.Error("imported moon lost its HiddenFromPlayers flag — it would now show to players")
	}
	if len(dst.eras) != 1 || dst.eras[0].StartMonth != 1 || dst.eras[0].StartDay != 1 {
		t.Errorf("imported era lost its day-granular start: %+v", dst.eras)
	}

	// #805: settings, moon look fields, cycles, festivals and weather must
	// all reach the actual service calls on import — none of them did
	// before (ExportCalendarData had nowhere to carry them from, and
	// CalendarService had no SetCycles/SetFestivals/SetWeather at all).
	if dst.created.Hemisphere == nil || *dst.created.Hemisphere != hemisphere {
		t.Errorf("imported calendar lost its hemisphere: %+v", dst.created.Hemisphere)
	}
	if !dst.created.ForecastsEnabled || !dst.created.MonthStartsNewWeek {
		t.Errorf("imported calendar lost forecasts_enabled/month_starts_new_week: %v/%v", dst.created.ForecastsEnabled, dst.created.MonthStartsNewWeek)
	}
	if len(dst.moons) != 1 || dst.moons[0].BaseDesign != "moon-cratered" || dst.moons[0].Tint == nil || *dst.moons[0].Tint != moonTint ||
		dst.moons[0].PhaseSource != "canvas-arc" || dst.moons[0].Size != 1.4 || dst.moons[0].OrbitSpeed != 0.8 {
		t.Errorf("imported moon lost its look fields: %+v", dst.moons)
	}
	if len(dst.cycles) != 1 || dst.cycles[0].Name != "Zodiac" || len(dst.cycles[0].Entries) != 1 || dst.cycles[0].Entries[0].Name != "Rat" {
		t.Errorf("imported calendar lost its cycles: %+v", dst.cycles)
	}
	if len(dst.festivals) != 1 || dst.festivals[0].Name != "Founding Day" {
		t.Errorf("imported calendar lost its festivals: %+v", dst.festivals)
	}
	if dst.weather == nil || dst.weather.Icon == nil || *dst.weather.Icon != weatherIcon {
		t.Errorf("imported calendar lost its weather: %+v", dst.weather)
	}
	if len(dst.kinds) != 1 || dst.kinds[0].DefaultAnnounced != calendar.AnnouncedAhead {
		t.Errorf("imported event kind lost its default_announced: %+v", dst.kinds)
	}

	if len(dst.createdEvents) != 2 {
		t.Fatalf("imported %d events, want 2", len(dst.createdEvents))
	}
	var dmCreated, publicCreated *calendar.CreateEventInput
	for i := range dst.createdEvents {
		e := &dst.createdEvents[i]
		switch e.Name {
		case "Secret War Council":
			dmCreated = e
		case "Harvest Festival":
			publicCreated = e
		}
	}
	if dmCreated == nil {
		t.Fatal("dm_only event was not recreated on import")
	}
	if dmCreated.Visibility != "dm_only" || !dmCreated.CanAuthorDmOnly {
		t.Error("imported dm_only event lost its visibility or its authoring grant")
	}
	if dmCreated.VisibilityRules == nil || *dmCreated.VisibilityRules != visRules {
		t.Error("imported dm_only event lost its per-user visibility_rules")
	}
	if publicCreated == nil {
		t.Fatal("public event was not recreated on import")
	}
	if publicCreated.EntityID == nil || *publicCreated.EntityID != "new-entity-77" {
		t.Errorf("imported event was not re-linked to its entity: %+v", publicCreated.EntityID)
	}
	if publicCreated.KindID == nil {
		t.Error("imported event lost its event-kind link")
	}
}

// TestCalendarImportAdapter_PreV5EraDefaultsToJanuaryFirst pins the pre-V5
// compat path: an ExportCalendarEra with no StartMonth/StartDay (the shape
// every backup taken before calendar-v5 carries — see ExportCalendarEra's
// doc comment) must still import as a valid, ordered era rather than being
// dropped or rejected.
func TestCalendarImportAdapter_PreV5EraDefaultsToJanuaryFirst(t *testing.T) {
	dst := &fakeCalendarService{}
	a := &calendarImportAdapter{svc: dst}
	report := campaigns.NewImportReport()
	data := &campaigns.ExportCalendarData{
		Name: "Legacy Calendar",
		Eras: []campaigns.ExportCalendarEra{
			{Name: "Old Era", StartYear: 100}, // no StartMonth/StartDay: pre-V5 shape
		},
	}
	if err := a.ImportCalendar(context.Background(), "c2", data, campaigns.NewIDMap("c2"), report); err != nil {
		t.Fatalf("import calendar: %v", err)
	}
	if report.HasFailures() {
		t.Fatalf("pre-V5 era import reported failures: %s", report.Summary())
	}
	if len(dst.eras) != 1 {
		t.Fatalf("got %d eras, want 1", len(dst.eras))
	}
	if dst.eras[0].StartMonth != 1 || dst.eras[0].StartDay != 1 {
		t.Errorf("pre-V5 era did not default to month 1 day 1: %+v", dst.eras[0])
	}
}

// TestCalendarExportImport_DmOnlyCalendarVisibilityRoundTrip is the
// regression for the "export/import silently makes a GM-only calendar
// public" finding: a campaign whose calendar is itself dm_only (with a
// visibility_rules allow-list, not just individual dm_only events) must
// come back exactly as dm_only on import — never "everyone", which would
// expose every "everyone" event inside it that was previously unreachable
// only because the calendar itself was hidden.
func TestCalendarExportImport_DmOnlyCalendarVisibilityRoundTrip(t *testing.T) {
	visRules := `{"allowed_users":["gm-2"]}`
	src := &fakeCalendarService{
		cal: &calendar.Calendar{
			ID: "cal-1", CampaignID: "c1", Mode: calendar.ModeFantasy,
			Name:            "Inner Circle Calendar",
			Visibility:      "dm_only",
			VisibilityRules: &visRules,
			HoursPerDay:     24, MinutesPerHour: 60, SecondsPerMinute: 60,
		},
	}

	exportAdapter := &calendarExportAdapter{svc: src}
	calData, err := exportAdapter.ExportCalendar(context.Background(), "c1", func(string) string { return "" })
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if calData.Visibility != "dm_only" {
		t.Errorf("exported calendar visibility = %q, want dm_only", calData.Visibility)
	}
	if calData.VisibilityRules == nil || *calData.VisibilityRules != visRules {
		t.Error("exported calendar lost its visibility_rules allow-list")
	}

	// Round-trip through JSON, exactly like a real backup file.
	env := &campaigns.CampaignExport{Format: campaigns.ExportFormat, Version: campaigns.ExportVersion, Calendar: calData}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var reloaded campaigns.CampaignExport
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	dst := &fakeCalendarService{}
	report := campaigns.NewImportReport()
	if err := (&calendarImportAdapter{svc: dst}).ImportCalendar(context.Background(), "imported-campaign", reloaded.Calendar, campaigns.NewIDMap("imported-campaign"), report); err != nil {
		t.Fatalf("import calendar: %v", err)
	}
	if report.HasFailures() {
		t.Fatalf("clean calendar import reported failures: %s", report.Summary())
	}
	if dst.created == nil {
		t.Fatal("ImportCalendar did not create a calendar")
	}
	if dst.created.Visibility != "dm_only" {
		t.Errorf("imported calendar visibility = %q, want dm_only — a GM-only calendar must never come back public", dst.created.Visibility)
	}
	if dst.created.VisibilityRules == nil || *dst.created.VisibilityRules != visRules {
		t.Error("imported calendar lost its visibility_rules allow-list")
	}
}

// TestCalendarImportAdapter_PreV5CalendarDefaultsToDmOnly pins the
// fail-toward-privacy default: a backup taken before this field existed (or
// any backup whose Visibility is otherwise unrecognized) must import as
// dm_only, never as "everyone" — see importCalendarVisibility's doc comment.
func TestCalendarImportAdapter_PreV5CalendarDefaultsToDmOnly(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty (pre-V5 backup)", ""},
		{"unrecognized value", "public"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := &fakeCalendarService{}
			data := &campaigns.ExportCalendarData{Name: "Legacy Calendar", Visibility: tt.in}
			report := campaigns.NewImportReport()
			if err := (&calendarImportAdapter{svc: dst}).ImportCalendar(context.Background(), "c2", data, campaigns.NewIDMap("c2"), report); err != nil {
				t.Fatalf("import calendar: %v", err)
			}
			if dst.created == nil {
				t.Fatal("ImportCalendar did not create a calendar")
			}
			if dst.created.Visibility != "dm_only" {
				t.Errorf("visibility %q imported as %q, want dm_only (fail toward privacy)", tt.in, dst.created.Visibility)
			}
		})
	}
}

// TestCalendarImportAdapter_PreV5MoonDefaultsToHidden pins the
// fail-toward-privacy default for ExportCalendarMoon.HiddenFromPlayers: a
// backup taken before this field existed (whose JSON omits the key
// entirely, unmarshaling to a nil pointer) must import that moon as hidden,
// never visible — see importMoonHidden's doc comment. An explicit true or
// false always passes through unchanged.
func TestCalendarImportAdapter_PreV5MoonDefaultsToHidden(t *testing.T) {
	tests := []struct {
		name   string
		hidden *bool
		want   bool
	}{
		{"missing key (pre-V5 backup)", nil, true},
		{"explicit false", boolPtr(false), false},
		{"explicit true", boolPtr(true), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := &fakeCalendarService{}
			data := &campaigns.ExportCalendarData{
				Name: "Legacy Calendar",
				Moons: []campaigns.ExportCalendarMoon{
					{Name: "Wandering Moon", CycleDays: 29.5, Color: "#ffffff", HiddenFromPlayers: tt.hidden},
				},
			}
			report := campaigns.NewImportReport()
			if err := (&calendarImportAdapter{svc: dst}).ImportCalendar(context.Background(), "c2", data, campaigns.NewIDMap("c2"), report); err != nil {
				t.Fatalf("import calendar: %v", err)
			}
			if report.HasFailures() {
				t.Fatalf("moon import reported failures: %s", report.Summary())
			}
			if len(dst.moons) != 1 {
				t.Fatalf("got %d moons, want 1", len(dst.moons))
			}
			if dst.moons[0].HiddenFromPlayers != tt.want {
				t.Errorf("HiddenFromPlayers = %v, want %v", dst.moons[0].HiddenFromPlayers, tt.want)
			}
		})
	}
}

// TestCalendarExportImport_MoonHiddenFlagRoundTrip is the regression for
// "restoring an older campaign backup un-hides a moon the GM hid": a fresh
// export must always write an explicit true/false for every moon (never the
// omitted-key shape a pre-V5 backup has), and both a hidden and a visible
// moon must come back exactly as they went in, through a real JSON round
// trip — not just collapse to whichever value the zero-value default gives.
func TestCalendarExportImport_MoonHiddenFlagRoundTrip(t *testing.T) {
	src := &fakeCalendarService{
		cal: &calendar.Calendar{
			ID: "cal-1", CampaignID: "c1", Mode: calendar.ModeFantasy,
			Name: "Two Moons", HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
			Moons: []calendar.Moon{
				{ID: 1, Name: "Secret Moon", CycleDays: 29.5, Color: "#ffffff", HiddenFromPlayers: true},
				{ID: 2, Name: "Common Moon", CycleDays: 14.0, Color: "#cccccc", HiddenFromPlayers: false},
			},
		},
	}

	exportAdapter := &calendarExportAdapter{svc: src}
	calData, err := exportAdapter.ExportCalendar(context.Background(), "c1", func(string) string { return "" })
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(calData.Moons) != 2 {
		t.Fatalf("exported %d moons, want 2", len(calData.Moons))
	}
	for _, m := range calData.Moons {
		if m.HiddenFromPlayers == nil {
			t.Errorf("exported moon %q has a nil HiddenFromPlayers — a fresh export must always write true/false", m.Name)
		}
	}

	// Round-trip through JSON, exactly like a real backup file.
	env := &campaigns.CampaignExport{Format: campaigns.ExportFormat, Version: campaigns.ExportVersion, Calendar: calData}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var reloaded campaigns.CampaignExport
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	dst := &fakeCalendarService{}
	report := campaigns.NewImportReport()
	if err := (&calendarImportAdapter{svc: dst}).ImportCalendar(context.Background(), "imported-campaign", reloaded.Calendar, campaigns.NewIDMap("imported-campaign"), report); err != nil {
		t.Fatalf("import calendar: %v", err)
	}
	if report.HasFailures() {
		t.Fatalf("clean moon import reported failures: %s", report.Summary())
	}
	if len(dst.moons) != 2 {
		t.Fatalf("imported %d moons, want 2", len(dst.moons))
	}
	want := map[string]bool{"Secret Moon": true, "Common Moon": false}
	for _, m := range dst.moons {
		wantHidden, ok := want[m.Name]
		if !ok {
			t.Fatalf("unexpected imported moon %q", m.Name)
		}
		if m.HiddenFromPlayers != wantHidden {
			t.Errorf("moon %q imported HiddenFromPlayers = %v, want %v", m.Name, m.HiddenFromPlayers, wantHidden)
		}
	}
}

func intPtr(i int) *int { return &i }

func boolPtr(b bool) *bool { return &b }

func float64Ptr(f float64) *float64 { return &f }
