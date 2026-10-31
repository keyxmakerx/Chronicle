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

// backupCalendarService adds the calendar's backup half to the fake: the
// export side reads days and rolls from it, the import side captures what
// it is asked to restore.
type backupCalendarService struct {
	*fakeCalendarService
	days  []calendar.DayWeather
	rolls map[string]calendar.EventRollBackup

	restoredDays  []calendar.DayWeather
	moonHands     map[int]calendar.MoonHand
	restoredRolls map[string]calendar.EventRollBackup
}

func (b *backupCalendarService) DayWeatherForBackup(context.Context, string, string, permissions.Viewer) ([]calendar.DayWeather, error) {
	return b.days, nil
}

func (b *backupCalendarService) RestoreDayWeather(_ context.Context, _, _ string, days []calendar.DayWeather, _ permissions.Viewer) (int, error) {
	b.restoredDays = days
	return 0, nil
}

func (b *backupCalendarService) RestoreMoonHand(_ context.Context, _, _ string, moonID int, hand calendar.MoonHand, _ permissions.Viewer) (int, error) {
	b.moonHands[moonID] = hand
	return 0, nil
}

func (b *backupCalendarService) EventRollsForBackup(context.Context, string, string, permissions.Viewer) (map[string]calendar.EventRollBackup, error) {
	return b.rolls, nil
}

// RestoreEventRoll drops a table roll, as the service does when the
// campaign has no table by that name.
func (b *backupCalendarService) RestoreEventRoll(_ context.Context, eventID, _, _ string, roll calendar.EventRollBackup, _ permissions.Viewer) (bool, error) {
	if roll.Config.Source == calendar.RollSourceTable {
		return true, nil
	}
	b.restoredRolls[eventID] = roll
	return false, nil
}

// A backup carries each painted day's weather, a hand-set moon's words and
// marks, and an event's roll with its nights, and the import restores them.
func TestCampaignExportImport_CalendarDaysMoonsRollsRoundTrip(t *testing.T) {
	icon := "cloud-rain"
	total := 17
	src := &backupCalendarService{
		fakeCalendarService: &fakeCalendarService{
			cal: &calendar.Calendar{
				ID: "cal-1", Name: "Reckoning", Mode: calendar.ModeFantasy, CurrentYear: 5,
				Months: []calendar.Month{{Name: "One", Days: 30}},
				Moons: []calendar.Moon{
					{ID: 7, Name: "Umbra", CycleDays: 30, PhaseMode: calendar.MoonManual,
						StateNames: map[string]string{calendar.MoonFull: "Blazing"},
						Marks:      []calendar.MoonMark{{Year: 5, Month: 1, Day: 4, State: calendar.MoonFull, Shown: true}}},
					{ID: 8, Name: "Selune", CycleDays: 28, PhaseMode: calendar.MoonCycle},
				},
			},
			events: []calendar.Event{
				{ID: "evt-1", Name: "Bloodnight", Year: 5, Month: 1, Day: 3, Visibility: "everyone"},
				{ID: "evt-2", Name: "Omen", Year: 5, Month: 1, Day: 5, Visibility: "everyone"},
			},
		},
		days: []calendar.DayWeather{
			{Year: 5, Month: 1, Day: 3, Icon: &icon, Source: calendar.WeatherSourceManual, Locked: boolPtr(true)},
			{Year: 5, Month: 1, Day: 4, Source: calendar.WeatherSourceGenerated, Locked: boolPtr(false)},
		},
		rolls: map[string]calendar.EventRollBackup{
			"evt-1": {
				Config: calendar.RollConfig{Source: calendar.RollSourceDice, Dice: "2d10",
					Results: []calendar.RollBand{{Name: "Calm"}}},
				Nights: []calendar.RollNight{{Year: 5, Month: 1, Day: 3, Total: &total, Faces: []int{8, 9},
					Source: calendar.RollNightAhead, Locked: true, Told: boolPtr(false)}},
			},
			"evt-2": {
				Config:    calendar.RollConfig{Source: calendar.RollSourceTable, TableID: "t-old"},
				TableName: "Omens",
			},
		},
	}

	calData, err := (&calendarExportAdapter{svc: src}).ExportCalendar(context.Background(), "c1", func(string) string { return "" })
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
	if len(got.WeatherDays) != 2 || !got.WeatherDays[0].Locked || got.WeatherDays[0].Icon == nil ||
		got.WeatherDays[1].Source != calendar.WeatherSourceGenerated {
		t.Errorf("exported day weather = %+v", got.WeatherDays)
	}
	if got.Moons[0].PhaseMode != calendar.MoonManual || len(got.Moons[0].Marks) != 1 || got.Moons[1].PhaseMode != "" {
		t.Errorf("exported moons = %+v", got.Moons)
	}
	if got.Events[0].Roll == nil || len(got.Events[0].Roll.Nights) != 1 || got.Events[1].Roll.TableName != "Omens" {
		t.Errorf("exported rolls = %+v / %+v", got.Events[0].Roll, got.Events[1].Roll)
	}

	dst := &backupCalendarService{
		fakeCalendarService: &fakeCalendarService{},
		moonHands:           map[int]calendar.MoonHand{},
		restoredRolls:       map[string]calendar.EventRollBackup{},
	}
	report := campaigns.NewImportReport()
	if err := (&calendarImportAdapter{svc: dst}).ImportCalendar(context.Background(), "c2", got, campaigns.NewIDMap("c2"), report); err != nil {
		t.Fatalf("import: %v", err)
	}

	if len(dst.restoredDays) != 2 || !*dst.restoredDays[0].Locked || *dst.restoredDays[1].Locked ||
		dst.restoredDays[0].Icon == nil || *dst.restoredDays[0].Icon != icon {
		t.Errorf("restored day weather = %+v", dst.restoredDays)
	}
	// The fake re-creates moons as 200, 201... in order; only Umbra is set by hand.
	if len(dst.moonHands) != 1 || dst.moonHands[200].StateNames[calendar.MoonFull] != "Blazing" ||
		len(dst.moonHands[200].Marks) != 1 || !dst.moonHands[200].Marks[0].Shown {
		t.Errorf("restored moon hands = %+v", dst.moonHands)
	}
	roll, ok := dst.restoredRolls["evt-Bloodnight"]
	if !ok || roll.Config.Dice != "2d10" || len(roll.Nights) != 1 || !roll.Nights[0].Locked ||
		roll.Nights[0].Told == nil || *roll.Nights[0].Told || *roll.Nights[0].Total != 17 {
		t.Errorf("restored roll = %+v", roll)
	}
	if f := report.Failures(); len(f) != 1 || f[0].Name != "Omen" || !strings.Contains(f[0].Reason, "Omens") {
		t.Errorf("a dropped table roll should be the only reported item: %+v", f)
	}
}
