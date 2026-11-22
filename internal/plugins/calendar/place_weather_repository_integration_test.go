// place_weather_repository_integration_test.go exercises
// PlaceWeatherRepository against a real MariaDB: a place's readings keep
// the world's rules (a generated write never replaces a painted or locked
// day), places stay apart, the party's moves replace on their day, and a
// deleted page takes its weather with it while its moves become everywhere
// else. Skipped under `-short`.
package calendar

import (
	"context"
	"testing"
)

func TestPlaceWeatherRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	calRepo, repo := NewCalendarRepository(db), NewPlaceWeatherRepository(db)
	fix := newTestCampaign(t, db, "placewx")
	cal := newTestCalendar(testUUID(t), fix.CampaignID, "Places")
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("Create calendar: %v", err)
	}
	salt := newTestEntity(t, db, fix.CampaignID, fix.UserID, "Saltreach")
	hollow := newTestEntity(t, db, fix.CampaignID, fix.UserID, "Hollow")
	s := func(v string) *string { return &v }
	day := func(d int, lbl, source string) DayWeatherInput {
		return DayWeatherInput{Year: 5, Month: 1, Day: d, Source: source, WeatherInput: WeatherInput{PresetLabel: s(lbl)}}
	}

	if err := repo.SetPlaceDays(ctx, cal.ID, salt, []DayWeatherInput{day(1, "Fog", WeatherSourceManual), day(2, "Rain", WeatherSourceGenerated), day(3, "Sleet", WeatherSourceGenerated)}); err != nil {
		t.Fatalf("SetPlaceDays: %v", err)
	}
	if err := repo.SetPlaceDays(ctx, cal.ID, hollow, []DayWeatherInput{day(1, "Sandstorm", WeatherSourceGenerated)}); err != nil {
		t.Fatalf("SetPlaceDays hollow: %v", err)
	}
	if n, err := repo.LockPlaceDays(ctx, cal.ID, salt, []DayDate{{5, 1, 3}, {5, 1, 9}}, true); err != nil || n != 1 {
		t.Fatalf("LockPlaceDays = %d, %v; want 1 (a day with no reading is skipped)", n, err)
	}
	// A generated roll over every day keeps the painted day 1 and the locked day 3.
	if err := repo.SetPlaceDays(ctx, cal.ID, salt, []DayWeatherInput{day(1, "Hail", WeatherSourceGenerated), day(2, "Clear", WeatherSourceGenerated), day(3, "Clear", WeatherSourceGenerated)}); err != nil {
		t.Fatalf("SetPlaceDays roll: %v", err)
	}
	got, err := repo.ListPlaceDays(ctx, cal.ID, []string{salt, hollow}, 5, 0)
	if err != nil {
		t.Fatalf("ListPlaceDays: %v", err)
	}
	want := map[int]string{1: "Fog", 2: "Clear", 3: "Sleet"}
	if len(got[salt]) != 3 {
		t.Fatalf("salt days = %+v", got[salt])
	}
	for _, d := range got[salt] {
		if *d.PresetLabel != want[d.Day] {
			t.Errorf("day %d = %s, want %s", d.Day, *d.PresetLabel, want[d.Day])
		}
	}
	if len(got[hollow]) != 1 || *got[hollow][0].PresetLabel != "Sandstorm" {
		t.Fatalf("hollow days = %+v; places must stay apart", got[hollow])
	}
	if err := repo.ClearPlaceDays(ctx, cal.ID, salt, []DayDate{{5, 1, 2}}); err != nil {
		t.Fatalf("ClearPlaceDays: %v", err)
	}
	if got, _ = repo.ListPlaceDays(ctx, cal.ID, []string{salt}, 5, 1); len(got[salt]) != 2 {
		t.Fatalf("after clear = %+v", got[salt])
	}

	for _, m := range []PartyMark{{Year: 5, Month: 1, Day: 4, PlaceID: salt}, {Year: 5, Month: 1, Day: 2, PlaceID: hollow}, {Year: 5, Month: 1, Day: 4}} {
		if err := repo.SetPartyMark(ctx, cal.ID, m); err != nil {
			t.Fatalf("SetPartyMark: %v", err)
		}
	}
	marks, err := repo.ListPartyMarks(ctx, cal.ID)
	if err != nil || len(marks) != 2 || marks[0].Day != 2 || marks[0].PlaceID != hollow || marks[1].Day != 4 || marks[1].PlaceID != "" {
		t.Fatalf("marks = %+v, %v; want day 2 at hollow, day 4 everywhere else", marks, err)
	}

	mustExec(t, db, `DELETE FROM entities WHERE id = ?`, hollow)
	if got, _ = repo.ListPlaceDays(ctx, cal.ID, []string{hollow}, 5, 0); len(got[hollow]) != 0 {
		t.Fatalf("a deleted page kept its weather: %+v", got[hollow])
	}
	if marks, _ = repo.ListPartyMarks(ctx, cal.ID); len(marks) != 2 || marks[0].PlaceID != "" {
		t.Fatalf("a move to a deleted page = %+v; want everywhere else", marks)
	}
	if err := repo.ClearPartyMark(ctx, cal.ID, DayDate{5, 1, 2}); err != nil {
		t.Fatalf("ClearPartyMark: %v", err)
	}
	if marks, _ = repo.ListPartyMarks(ctx, cal.ID); len(marks) != 1 {
		t.Fatalf("after clear = %+v", marks)
	}
}
