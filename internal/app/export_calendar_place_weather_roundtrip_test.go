package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// placeBackupCalendarService adds the place weather backup half to the
// fake: the export side reads from backup, the import side captures what it
// is asked to restore.
type placeBackupCalendarService struct {
	*fakeCalendarService
	backup *calendar.PlaceBackup

	restoredPlaces map[string][]calendar.DayWeather
	restoredMoves  []calendar.PartyMark
}

func (p *placeBackupCalendarService) PlaceWeatherForBackup(context.Context, string, string, permissions.Viewer) (*calendar.PlaceBackup, error) {
	return p.backup, nil
}

func (p *placeBackupCalendarService) RestorePlaceWeather(_ context.Context, _, _, placeID string, days []calendar.DayWeather, _ permissions.Viewer) (int, error) {
	p.restoredPlaces[placeID] = days
	return 0, nil
}

func (p *placeBackupCalendarService) RestorePartyMoves(_ context.Context, _, _ string, marks []calendar.PartyMark, _ permissions.Viewer) (int, error) {
	p.restoredMoves = marks
	return 0, nil
}

func reckoning() *fakeCalendarService {
	return &fakeCalendarService{cal: &calendar.Calendar{
		ID: "cal-1", Name: "Reckoning", Mode: calendar.ModeFantasy, CurrentYear: 5,
		Months: []calendar.Month{{Name: "One", Days: 30}},
	}}
}

// A backup carries each place's readings and the party's moves by page
// slug, and the import puts them back on the pages it re-created.
func TestCalendarExportImport_PlaceWeatherRoundTrip(t *testing.T) {
	sleet := "Sleet"
	src := &placeBackupCalendarService{
		fakeCalendarService: reckoning(),
		backup: &calendar.PlaceBackup{
			Days: map[string][]calendar.DayWeather{
				"e-salt": {
					{Year: 5, Month: 1, Day: 3, PresetLabel: &sleet, Source: calendar.WeatherSourceManual, Locked: boolPtr(true)},
					{Year: 5, Month: 1, Day: 4, Source: calendar.WeatherSourceGenerated, Locked: boolPtr(false)},
				},
				// A page the backup does not carry has no slug to name it by.
				"e-gone": {{Year: 5, Month: 1, Day: 3, Source: calendar.WeatherSourceManual}},
			},
			Marks: []calendar.PartyMark{
				{Year: 5, Month: 1, Day: 2, PlaceID: "e-gull"},
				{Year: 5, Month: 1, Day: 5},
				{Year: 5, Month: 1, Day: 8, PlaceID: "e-gone"},
			},
		},
	}
	slugs := map[string]string{"e-salt": "saltreach", "e-gull": "gull-rock"}
	calData, err := (&calendarExportAdapter{svc: src}).ExportCalendar(context.Background(), "c1", func(id string) string { return slugs[id] })
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	raw, err := json.Marshal(&campaigns.CampaignExport{Format: campaigns.ExportFormat, Version: campaigns.ExportVersion, Calendar: calData})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var reloaded campaigns.CampaignExport
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := reloaded.Calendar
	if len(got.PlaceWeather) != 1 || got.PlaceWeather[0].PlaceSlug != "saltreach" || len(got.PlaceWeather[0].Days) != 2 ||
		!got.PlaceWeather[0].Days[0].Locked || got.PlaceWeather[0].Days[1].Source != calendar.WeatherSourceGenerated {
		t.Fatalf("exported place weather = %+v", got.PlaceWeather)
	}
	if len(got.PartyMoves) != 2 || got.PartyMoves[0].PlaceSlug != "gull-rock" || got.PartyMoves[1].PlaceSlug != "" {
		t.Fatalf("exported party moves = %+v", got.PartyMoves)
	}

	// A place whose page did not come back is reported, not guessed at.
	got.PlaceWeather = append(got.PlaceWeather, campaigns.ExportCalendarPlaceWeather{
		PlaceSlug: "lost-isle", Days: got.PlaceWeather[0].Days,
	})
	dst := &placeBackupCalendarService{
		fakeCalendarService: &fakeCalendarService{},
		restoredPlaces:      map[string][]calendar.DayWeather{},
	}
	idMap := campaigns.NewIDMap("c2")
	idMap.EntitySlugToID["saltreach"] = "n-salt"
	idMap.EntitySlugToID["gull-rock"] = "n-gull"
	report := campaigns.NewImportReport()
	if err := (&calendarImportAdapter{svc: dst}).ImportCalendar(context.Background(), "c2", got, idMap, report); err != nil {
		t.Fatalf("import: %v", err)
	}

	days := dst.restoredPlaces["n-salt"]
	if len(dst.restoredPlaces) != 1 || len(days) != 2 || !*days[0].Locked || *days[1].Locked ||
		days[0].PresetLabel == nil || *days[0].PresetLabel != sleet || days[1].Source != calendar.WeatherSourceGenerated {
		t.Errorf("restored place weather = %+v", dst.restoredPlaces)
	}
	want := []calendar.PartyMark{{Year: 5, Month: 1, Day: 2, PlaceID: "n-gull"}, {Year: 5, Month: 1, Day: 5}}
	if len(dst.restoredMoves) != 2 || dst.restoredMoves[0] != want[0] || dst.restoredMoves[1] != want[1] {
		t.Errorf("restored party moves = %+v, want %+v", dst.restoredMoves, want)
	}
	if f := report.Failures(); len(f) != 1 || f[0].Name != "lost-isle" {
		t.Errorf("only the place with no page should be reported: %+v", f)
	}
}

// A calendar with no place weather writes no place keys, so the file reads
// in an older importer; a backup from before place weather restores none.
func TestCalendarExportImport_NoPlaceWeatherLeavesNoKeys(t *testing.T) {
	src := &placeBackupCalendarService{fakeCalendarService: reckoning()}
	calData, err := (&calendarExportAdapter{svc: src}).ExportCalendar(context.Background(), "c1", func(string) string { return "" })
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	raw, err := json.Marshal(calData)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if s := string(raw); strings.Contains(s, "place_weather") || strings.Contains(s, "party_moves") {
		t.Fatalf("an empty calendar wrote place keys: %s", s)
	}

	var older campaigns.ExportCalendarData
	if err := json.Unmarshal([]byte(`{"name":"Reckoning","mode":"fantasy","current_year":5,"months":[{"name":"One","days":30}]}`), &older); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	dst := &placeBackupCalendarService{fakeCalendarService: &fakeCalendarService{}, restoredPlaces: map[string][]calendar.DayWeather{}}
	report := campaigns.NewImportReport()
	if err := (&calendarImportAdapter{svc: dst}).ImportCalendar(context.Background(), "c2", &older, campaigns.NewIDMap("c2"), report); err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(dst.restoredPlaces) != 0 || dst.restoredMoves != nil || len(report.Failures()) != 0 {
		t.Fatalf("an older backup restored place weather: %+v %+v %+v", dst.restoredPlaces, dst.restoredMoves, report.Failures())
	}
}
