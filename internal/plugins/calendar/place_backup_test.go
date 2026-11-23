package calendar

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// A backup reads every place's readings and every party move; only a
// viewer who sees everything may read or restore them.
func TestPlaceWeatherForBackup(t *testing.T) {
	repo := &fakePlaceRepo{
		days:  map[string][]DayWeather{"salt": {wx(5, 1, 3, "Sleet")}, "hollow": {wx(4, 2, 1, "Dust")}},
		marks: []PartyMark{{Year: 5, Month: 1, Day: 2, PlaceID: "hollow"}},
	}
	pb := placeFixture(nil, repo).(PlaceBackupService)
	got, err := pb.PlaceWeatherForBackup(context.Background(), "cal", "camp", pwDirector)
	if err != nil || len(got.Days) != 2 || len(got.Days["hollow"]) != 1 || len(got.Marks) != 1 {
		t.Fatalf("backup = %+v, %v", got, err)
	}
	if _, err := pb.PlaceWeatherForBackup(context.Background(), "cal", "camp", pwPlayer); apperror.SafeCode(err) != http.StatusForbidden {
		t.Fatalf("a player's backup read = %v, want 403", err)
	}
	if _, err := pb.RestorePartyMoves(context.Background(), "cal", "camp", nil, pwPlayer); apperror.SafeCode(err) != http.StatusForbidden {
		t.Fatalf("a player's restore = %v, want 403", err)
	}
}

func TestRestorePlaceWeather(t *testing.T) {
	sleet := wx(5, 1, 3, "Sleet")
	sleet.Source, sleet.Locked = WeatherSourceManual, boolp(true)
	fog := wx(5, 2, 4, "Fog")
	fog.Source = WeatherSourceGenerated
	offCalendar := wx(5, 2, 25, "Hail") // month two has 20 days
	offCalendar.Source = WeatherSourceManual

	tests := []struct {
		name        string
		place       string
		days        []DayWeather
		wantSkipped int
		wantSet     int
		wantLocked  int
	}{
		{"readings go back by source, the locked one locked", "salt", []DayWeather{sleet, fog}, 0, 2, 1},
		{"a day the calendar lacks is skipped", "salt", []DayWeather{sleet, offCalendar}, 1, 1, 1},
		{"a page the campaign lacks takes nothing", "nowhere", []DayWeather{sleet, fog}, 2, 0, 0},
		// A place whose climate was taken off still gets its readings back.
		{"a page without a climate of its own keeps its readings", "plain", []DayWeather{fog}, 0, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakePlaceRepo{}
			pb := placeFixture(nil, repo).(PlaceBackupService)
			skipped, err := pb.RestorePlaceWeather(context.Background(), "cal", "camp", tt.place, tt.days, permissions.SystemViewer(3))
			if err != nil || skipped != tt.wantSkipped {
				t.Fatalf("restore = %d skipped, %v; want %d", skipped, err, tt.wantSkipped)
			}
			if len(repo.set[tt.place]) != tt.wantSet || len(repo.locked[tt.place]) != tt.wantLocked {
				t.Fatalf("set %+v locked %+v", repo.set[tt.place], repo.locked[tt.place])
			}
		})
	}
}

func TestRestorePartyMoves(t *testing.T) {
	repo := &fakePlaceRepo{}
	pb := placeFixture(nil, repo).(PlaceBackupService)
	skipped, err := pb.RestorePartyMoves(context.Background(), "cal", "camp", []PartyMark{
		{Year: 5, Month: 1, Day: 2, PlaceID: "gull"},
		{Year: 5, Month: 1, Day: 6},                       // nowhere with a climate: kept
		{Year: 5, Month: 1, Day: 6, PlaceID: "salt"},      // second move on one day
		{Year: 5, Month: 2, Day: 30, PlaceID: "salt"},     // month two has 20 days
		{Year: 5, Month: 1, Day: 9, PlaceID: "somewhere"}, // not a page here
	}, pwDirector)
	if err != nil || skipped != 3 {
		t.Fatalf("restore = %d skipped, %v; want 3", skipped, err)
	}
	if len(repo.marks) != 2 || repo.marks[0].PlaceID != "gull" || repo.marks[1].PlaceID != "" {
		t.Fatalf("restored moves = %+v", repo.marks)
	}
}
