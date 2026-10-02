package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// multiCalendarFixture is a default calendar plus one extra that shares the
// campaign's event kind, as a real campaign with two calendars would.
func multiCalendarFixture() *fakeCalendarService {
	kinds := []calendar.EventKind{{ID: 1, Slug: "festival", Name: "Festival", Icon: "fa-star", Color: "#10b981", DefaultAnnounced: calendar.AnnouncedAhead}}
	tint := "#aabbcc"
	return &fakeCalendarService{
		cal: &calendar.Calendar{
			ID: "cal-main", CampaignID: "c1", Mode: calendar.ModeFantasy, Name: "Main",
			Visibility: "everyone", EventKinds: kinds,
			Months: []calendar.Month{{Name: "Thaw", Days: 30}},
		},
		events: []calendar.Event{{ID: "e1", Name: "Main Feast", Year: 1, Month: 1, Day: 1, Visibility: "everyone", KindID: intPtr(1)}},
		extraCals: []*calendar.Calendar{{
			ID: "cal-elf", CampaignID: "c1", Mode: calendar.ModeFantasy, Name: "Elven Reckoning",
			Visibility: "dm_only", EventKinds: kinds, ForecastsEnabled: true,
			Months: []calendar.Month{{Name: "Leaf", Days: 40}},
			Moons:  []calendar.Moon{{Name: "Silver", CycleDays: 20, Color: "#fff", BaseDesign: "moon-plain", Tint: &tint, PhaseSource: "canvas-arc", Size: 2, OrbitSpeed: 0.5, HiddenFromPlayers: true}},
		}},
		extraEvents: map[string][]calendar.Event{
			"cal-elf": {{ID: "e2", Name: "Elven Rite", Year: 5, Month: 1, Day: 2, Visibility: "dm_only", KindID: intPtr(1)}},
		},
	}
}

// TestCampaignExportImport_MultipleCalendarsRoundTrip proves every calendar
// survives a backup, only the primary becomes default or the timeline
// anchor, and event kinds are created once yet still link extras' events.
func TestCampaignExportImport_MultipleCalendarsRoundTrip(t *testing.T) {
	src := multiCalendarFixture()
	data, err := (&calendarExportAdapter{svc: src}).ExportCalendar(context.Background(), "c1", func(string) string { return "" })
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if data.Name != "Main" || len(data.AdditionalCalendars) != 1 {
		t.Fatalf("export = %q with %d additional, want Main with 1", data.Name, len(data.AdditionalCalendars))
	}
	if got := data.AdditionalCalendars[0]; len(got.EventCategories) != 0 || len(got.Events) != 1 || got.Visibility != "dm_only" {
		t.Errorf("extra calendar export wrong (kinds must stay on the primary): %+v", got)
	}

	raw, err := json.Marshal(&campaigns.CampaignExport{Format: campaigns.ExportFormat, Version: campaigns.ExportVersion, Calendar: data})
	if err != nil {
		t.Fatal(err)
	}
	var reloaded campaigns.CampaignExport
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatal(err)
	}

	dst := &fakeCalendarService{}
	idMap := campaigns.NewIDMap("new")
	report := campaigns.NewImportReport()
	if err := (&calendarImportAdapter{svc: dst}).ImportCalendar(context.Background(), "new", reloaded.Calendar, idMap, report); err != nil {
		t.Fatal(err)
	}
	if report.HasFailures() {
		t.Fatalf("import failures: %s", report.Summary())
	}
	if len(dst.allCreated) != 2 || dst.allCreated[1].Name != "Elven Reckoning" {
		t.Fatalf("created %d calendars, want both", len(dst.allCreated))
	}
	if dst.allCreated[1].Visibility != "dm_only" || !dst.allCreated[1].ForecastsEnabled {
		t.Errorf("extra calendar lost visibility/forecasts: %+v", dst.allCreated[1])
	}
	if dst.defaultSets != 1 || dst.defaultSetFor != dst.allCreated[0].ID || idMap.CalendarID != dst.allCreated[0].ID {
		t.Errorf("default/timeline anchor must be the primary only: sets=%d default=%q idMap=%q", dst.defaultSets, dst.defaultSetFor, idMap.CalendarID)
	}
	if len(dst.kinds) != 1 {
		t.Errorf("event kinds created %d times, want once", len(dst.kinds))
	}
	if len(dst.createdEvents) != 2 {
		t.Fatalf("imported %d events, want 2", len(dst.createdEvents))
	}
	for _, e := range dst.createdEvents {
		if e.KindID == nil {
			t.Errorf("event %q lost its kind link", e.Name)
		}
	}
	// SetMoons ran for the extra calendar with its look intact.
	if len(dst.moons) != 1 || dst.moons[0].BaseDesign != "moon-plain" || dst.moons[0].Size != 2 || !dst.moons[0].HiddenFromPlayers {
		t.Errorf("extra calendar's moon lost fields: %+v", dst.moons)
	}
}

// TestCalendarImport_FileShapes covers files written by older versions:
// the singular calendar with none of the newer keys must still import, and an
// extra calendar named in the file must not disturb the primary.
func TestCalendarImport_FileShapes(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantCals  int
		wantNames []string
	}{
		{"old file, singular calendar only", `{"name":"Old","mode":"fantasy","current_year":3,"months":[{"name":"M","days":30,"sort_order":0}],"weekdays":[]}`, 1, []string{"Old"}},
		{"new file, one extra", `{"name":"A","mode":"fantasy","months":[],"weekdays":[],"additional_calendars":[{"name":"B","mode":"fantasy","months":[],"weekdays":[]}]}`, 2, []string{"A", "B"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data campaigns.ExportCalendarData
			if err := json.Unmarshal([]byte(tt.json), &data); err != nil {
				t.Fatal(err)
			}
			dst := &fakeCalendarService{}
			report := campaigns.NewImportReport()
			if err := (&calendarImportAdapter{svc: dst}).ImportCalendar(context.Background(), "new", &data, campaigns.NewIDMap("new"), report); err != nil {
				t.Fatal(err)
			}
			if report.HasFailures() {
				t.Errorf("failures: %s", report.Summary())
			}
			if len(dst.allCreated) != tt.wantCals {
				t.Fatalf("created %d, want %d", len(dst.allCreated), tt.wantCals)
			}
			for i, n := range tt.wantNames {
				if dst.allCreated[i].Name != n {
					t.Errorf("calendar %d = %q, want %q", i, dst.allCreated[i].Name, n)
				}
			}
			if dst.defaultSets != 1 {
				t.Errorf("default set %d times, want 1", dst.defaultSets)
			}
		})
	}
}
