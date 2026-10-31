package calendar

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Day readings come back whole, and go back in by source with their locks;
// a day the calendar does not have is skipped and counted.
func TestDayWeatherBackupRoundTrip(t *testing.T) {
	icon := "cloud-rain"
	windKPH := 12.0
	stored := []DayWeather{
		{Year: 5, Month: 1, Day: 3, Icon: &icon, Source: WeatherSourceManual, Locked: boolp(true)},
		{Year: 5, Month: 2, Day: 4, Source: WeatherSourceGenerated, Locked: boolp(false),
			Wind: &Wind{SpeedKPH: &windKPH}},
	}
	var wrote []DayWeatherInput
	var lockedDays []DayDate
	weather := &fakeWeatherRepo{
		listAllDaysFn: func(context.Context, string) ([]DayWeather, error) { return stored, nil },
		setDaysFn: func(_ context.Context, _ string, days []DayWeatherInput) error {
			wrote = append(wrote, days...)
			return nil
		},
		lockDaysFn: func(_ context.Context, _ string, dates []DayDate, locked bool) (int, error) {
			if locked {
				lockedDays = append(lockedDays, dates...)
			}
			return len(dates), nil
		},
	}
	svc, _ := markFixture(t, false)
	svc.weatherRepo = weather

	got, err := svc.DayWeatherForBackup(context.Background(), "cal", "camp", owner)
	if err != nil || len(got) != 2 {
		t.Fatalf("backup = %+v, %v", got, err)
	}
	if _, err := svc.DayWeatherForBackup(context.Background(), "cal", "camp", player); apperror.SafeCode(err) != http.StatusForbidden {
		t.Fatalf("a player's backup read = %v, want 403", err)
	}

	got = append(got, DayWeather{Year: 5, Month: 2, Day: 25, Source: WeatherSourceManual}) // month two has 20 days
	skipped, err := svc.RestoreDayWeather(context.Background(), "cal", "camp", got, owner)
	if err != nil || skipped != 1 {
		t.Fatalf("restore = %d skipped, %v; want 1 skipped", skipped, err)
	}
	if len(wrote) != 2 || wrote[0].Source != WeatherSourceManual || wrote[0].Icon == nil || *wrote[0].Icon != icon ||
		wrote[1].Source != WeatherSourceGenerated || wrote[1].WindSpeedKPH == nil || *wrote[1].WindSpeedKPH != 12 {
		t.Fatalf("restored readings = %+v", wrote)
	}
	if len(lockedDays) != 1 || lockedDays[0] != (DayDate{Year: 5, Month: 1, Day: 3}) {
		t.Fatalf("relocked days = %+v, want only 5/1/3", lockedDays)
	}
}

// A hand-set moon comes back set by hand, with its words and marks; a mark
// on a day the calendar lacks, or in an unknown state, is skipped.
func TestRestoreMoonHand(t *testing.T) {
	svc, marks := markFixture(t, false)
	hand := MoonHand{
		StateNames: map[string]string{MoonFull: "Blazing"},
		Marks: []MoonMark{
			{Year: 5, Month: 1, Day: 4, State: MoonFull, Shown: true},
			{Year: 5, Month: 2, Day: 6, State: MoonVanished},
			{Year: 5, Month: 2, Day: 30, State: MoonDark},
			{Year: 5, Month: 1, Day: 9, State: "eclipsed"},
		},
	}
	skipped, err := svc.RestoreMoonHand(context.Background(), "cal", "camp", 7, hand, owner)
	if err != nil || skipped != 2 {
		t.Fatalf("restore = %d skipped, %v; want 2 skipped", skipped, err)
	}
	if marks.mode[7] != MoonManual || marks.names[7][MoonFull] != "Blazing" {
		t.Fatalf("moon mode = %q names = %v", marks.mode[7], marks.names[7])
	}
	if len(marks.marks[7]) != 2 || !marks.marks[7][DayDate{5, 1, 4}].Shown || marks.marks[7][DayDate{5, 2, 6}].State != MoonVanished {
		t.Fatalf("restored marks = %+v", marks.marks[7])
	}
	if _, err := svc.RestoreMoonHand(context.Background(), "cal", "camp", 7, hand, player); apperror.SafeCode(err) != http.StatusForbidden {
		t.Fatalf("a player's restore = %v, want 403", err)
	}
}

// A roll and its nights come back as stored: locks and "tell players"
// choices included, though a night write resets both.
func TestEventRollBackupRoundTrip(t *testing.T) {
	cfg := bloodnight()
	_ = cfg.Normalize()
	src, srcRolls := rollFixture(t, &cfg, nil)
	src.eventRepo.(*fakeEventRepo).listAllFn = func(context.Context, string) ([]Event, error) {
		return []Event{{ID: "ev-blood"}, {ID: "ev-no-roll"}}, nil
	}
	total := 14
	srcRolls.nights["ev-blood"] = map[DayDate]RollNight{
		{5, 1, 3}: {Year: 5, Month: 1, Day: 3, Total: &total, Faces: []int{7, 7}, Source: RollNightAhead, Locked: true, Told: boolp(true)},
		{5, 2, 3}: {Year: 5, Month: 2, Day: 3, Total: &total, Source: RollNightManual},
	}
	got, err := src.EventRollsForBackup(context.Background(), "cal", "camp", owner)
	if err != nil || len(got) != 1 || len(got["ev-blood"].Nights) != 2 {
		t.Fatalf("backup = %+v, %v", got, err)
	}

	dst, dstRolls := rollFixture(t, nil, nil)
	b := got["ev-blood"]
	b.Nights = append(b.Nights, RollNight{Year: 5, Month: 2, Day: 40, Source: RollNightAhead})
	dropped, err := dst.RestoreEventRoll(context.Background(), "ev-blood", "cal", "camp", b, owner)
	if err != nil || dropped {
		t.Fatalf("restore = dropped %v, %v", dropped, err)
	}
	if dstRolls.cfg["ev-blood"] == nil || dstRolls.cfg["ev-blood"].Dice != cfg.Dice {
		t.Fatalf("restored roll = %+v", dstRolls.cfg["ev-blood"])
	}
	first := dstRolls.nights["ev-blood"][DayDate{5, 1, 3}]
	if len(dstRolls.nights["ev-blood"]) != 2 || !first.Locked || first.Told == nil || !*first.Told || *first.Total != 14 {
		t.Fatalf("restored nights = %+v", dstRolls.nights["ev-blood"])
	}
}

// A table roll is matched to this campaign's table by name, and left off
// when there is none.
func TestRestoreTableRollMatchesByName(t *testing.T) {
	b := EventRollBackup{Config: RollConfig{Source: RollSourceTable, TableID: "old-id"}, TableName: "Omens"}

	svc, rolls := rollFixture(t, nil, nil)
	dropped, err := svc.RestoreEventRoll(context.Background(), "ev-blood", "cal", "camp", b, owner)
	if err != nil || !dropped || rolls.cfg["ev-blood"] != nil {
		t.Fatalf("with no table: dropped %v, %v, cfg %+v", dropped, err, rolls.cfg["ev-blood"])
	}

	svc.SetRollTableSource(namedTables{"new-id": {Name: "Omens", Entries: []RollTableEntry{{Name: "Crows", Weight: 1}}}})
	dropped, err = svc.RestoreEventRoll(context.Background(), "ev-blood", "cal", "camp", b, owner)
	if err != nil || dropped || rolls.cfg["ev-blood"] == nil || rolls.cfg["ev-blood"].TableID != "new-id" {
		t.Fatalf("with the table: dropped %v, %v, cfg %+v", dropped, err, rolls.cfg["ev-blood"])
	}
}

type namedTables map[string]*RollTable

func (f namedTables) RollTable(_ context.Context, _, id string) (*RollTable, error) { return f[id], nil }
func (f namedTables) RollTableIDByName(_ context.Context, _, name string) (string, error) {
	for id, t := range f {
		if t.Name == name {
			return id, nil
		}
	}
	return "", nil
}
