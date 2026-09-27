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

	src := &fakeCalendarService{
		cal: &calendar.Calendar{
			ID: "cal-1", CampaignID: "c1", Mode: calendar.ModeFantasy,
			Name: "Harvest Calendar", Description: &desc, EpochName: &epoch,
			CurrentYear: 998, CurrentMonth: 3, CurrentDay: 12,
			CurrentHour: 14, CurrentMinute: 5,
			HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
			LeapYearEvery: 4, LeapYearOffset: 0,
			Months: []calendar.Month{
				{Name: "Thaw", Days: 30, SortOrder: 0},
				{Name: "Bloom", Days: 30, SortOrder: 1, LeapYearDays: 1},
			},
			Weekdays: []calendar.Weekday{
				{Name: "Sunday", SortOrder: 0}, {Name: "Moonday", SortOrder: 1},
			},
			Moons: []calendar.Moon{
				{ID: 1, Name: "Secret Moon", CycleDays: 29.5, Color: "#ffffff", HiddenFromPlayers: true},
			},
			Seasons: []calendar.Season{
				{Name: "Spring", StartMonth: 1, StartDay: 1, EndMonth: 2, EndDay: 30, Color: "#22c55e"},
			},
			Eras: []calendar.Era{
				{ID: 1, Name: "First Age", StartYear: 0, StartMonth: 1, StartDay: 1, Color: "#eab308"},
			},
			EventKinds: []calendar.EventKind{
				{ID: 1, Slug: "festival", Name: "Festival", Icon: "fa-star", Color: "#10b981"},
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
	if len(calData.Moons) != 1 || !calData.Moons[0].HiddenFromPlayers {
		t.Errorf("exported moon lost its HiddenFromPlayers flag: %+v", calData.Moons)
	}
	if len(calData.Eras) != 1 || calData.Eras[0].StartMonth != 1 || calData.Eras[0].StartDay != 1 {
		t.Errorf("exported era lost its day-granular start: %+v", calData.Eras)
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

func intPtr(i int) *int { return &i }
