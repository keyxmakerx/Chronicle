// weather_repository_test.go exercises WeatherRepository against a real
// MariaDB: the upsert (Set creates, then replaces), tenant isolation via
// calendar_id scoping, and the cascade from a deleted calendar. Skipped
// under `-short`.
package calendar

import (
	"context"
	"reflect"
	"testing"
)

func TestWeatherRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() }) // after fixture cleanups (LIFO), not before

	ctx := context.Background()
	weatherRepo := NewWeatherRepository(db)
	calRepo := NewCalendarRepository(db)

	fix := newTestCampaign(t, db, "weather")
	calA := newTestCalendar(testUUID(t), fix.CampaignID, "Weather A")
	calB := newTestCalendar(testUUID(t), fix.CampaignID, "Weather B")
	if err := calRepo.Create(ctx, calA); err != nil {
		t.Fatalf("Create calA: %v", err)
	}
	if err := calRepo.Create(ctx, calB); err != nil {
		t.Fatalf("Create calB: %v", err)
	}

	t.Run("Get on a calendar with no weather row returns (nil, nil)", func(t *testing.T) {
		w, err := weatherRepo.Get(ctx, calA.ID)
		if err != nil || w != nil {
			t.Fatalf("Get = %+v, %v; want (nil, nil)", w, err)
		}
	})

	preset := "clear-sky"
	label := "Clear Sky"
	temp := 18.5
	windKPH := 12.0
	windDir := "NE"

	t.Run("Set creates, Get round-trips wind + precipitation sub-structs", func(t *testing.T) {
		if err := weatherRepo.Set(ctx, calA.ID, WeatherInput{
			PresetID: &preset, PresetLabel: &label, TemperatureCelsius: &temp,
			WindSpeedKPH: &windKPH, WindDirection: &windDir,
		}); err != nil {
			t.Fatalf("Set: %v", err)
		}
		got, err := weatherRepo.Get(ctx, calA.ID)
		if err != nil || got == nil {
			t.Fatalf("Get = %+v, %v", got, err)
		}
		if got.PresetID == nil || *got.PresetID != preset {
			t.Errorf("PresetID = %v, want %q", got.PresetID, preset)
		}
		if got.Wind == nil || got.Wind.SpeedKPH == nil || *got.Wind.SpeedKPH != windKPH {
			t.Errorf("Wind = %+v", got.Wind)
		}
		if got.Precipitation != nil {
			t.Errorf("Precipitation should be nil when no precipitation fields were set, got %+v", got.Precipitation)
		}
	})

	t.Run("Set again upserts (replaces) rather than duplicating the row", func(t *testing.T) {
		newLabel := "Storm"
		precipType := "rain"
		precipIntensity := 0.8
		if err := weatherRepo.Set(ctx, calA.ID, WeatherInput{
			PresetLabel: &newLabel, PrecipitationType: &precipType, PrecipitationIntensity: &precipIntensity,
		}); err != nil {
			t.Fatalf("second Set: %v", err)
		}
		got, err := weatherRepo.Get(ctx, calA.ID)
		if err != nil || got == nil {
			t.Fatalf("Get: %+v, %v", got, err)
		}
		if got.PresetLabel == nil || *got.PresetLabel != newLabel {
			t.Errorf("PresetLabel = %v, want %q (the upsert should have replaced it)", got.PresetLabel, newLabel)
		}
		if got.Precipitation == nil || got.Precipitation.Type == nil || *got.Precipitation.Type != precipType {
			t.Errorf("Precipitation = %+v", got.Precipitation)
		}

		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM calendar_weather WHERE calendar_id = ?`, calA.ID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 1 {
			t.Errorf("calendar_weather has %d rows for calA, want exactly 1 (upsert, not insert)", n)
		}
	})

	t.Run("calendar_id scoping: calB's weather is independent of calA's (tenant isolation)", func(t *testing.T) {
		if got, _ := weatherRepo.Get(ctx, calB.ID); got != nil {
			t.Fatalf("calB should have no weather row yet, got %+v", got)
		}
		bLabel := "Fog"
		if err := weatherRepo.Set(ctx, calB.ID, WeatherInput{PresetLabel: &bLabel}); err != nil {
			t.Fatalf("Set(calB): %v", err)
		}
		gotA, _ := weatherRepo.Get(ctx, calA.ID)
		gotB, _ := weatherRepo.Get(ctx, calB.ID)
		if gotA.PresetLabel == nil || *gotA.PresetLabel == bLabel {
			t.Error("calA's weather was overwritten by calB's Set")
		}
		if gotB.PresetLabel == nil || *gotB.PresetLabel != bLabel {
			t.Errorf("calB's weather = %+v", gotB)
		}
	})

	t.Run("deleting the calendar cascades its weather row", func(t *testing.T) {
		if err := calRepo.Delete(ctx, calB.ID); err != nil {
			t.Fatalf("Delete calB: %v", err)
		}
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM calendar_weather WHERE calendar_id = ?`, calB.ID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Error("deleting the calendar should cascade calendar_weather, but the row survived")
		}
	})
}

// TestWeatherRepository_Days_Integration covers the per-day table: upsert,
// month filter, a generated write never replacing a manual one, clearing,
// tenant isolation and the cascade from a deleted calendar.
func TestWeatherRepository_Days_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	weatherRepo := NewWeatherRepository(db)
	calRepo := NewCalendarRepository(db)
	fix := newTestCampaign(t, db, "weatherdays")
	calA := newTestCalendar(testUUID(t), fix.CampaignID, "Days A")
	calB := newTestCalendar(testUUID(t), fix.CampaignID, "Days B")
	for _, c := range []*Calendar{calA, calB} {
		if err := calRepo.Create(ctx, c); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	s := func(v string) *string { return &v }
	f := func(v float64) *float64 { return &v }
	day := func(m, d int, source, preset string) DayWeatherInput {
		return DayWeatherInput{Year: 3, Month: m, Day: d, Source: source,
			WeatherInput: WeatherInput{PresetID: s(preset), TemperatureCelsius: f(12), PrecipitationType: s("rain"), PrecipitationIntensity: f(0.4)}}
	}

	if err := weatherRepo.SetDays(ctx, calA.ID, []DayWeatherInput{
		day(1, 1, WeatherSourceManual, "clear"),
		day(1, 2, WeatherSourceGenerated, "rain"),
		day(2, 1, WeatherSourceGenerated, "fog"),
	}); err != nil {
		t.Fatalf("SetDays: %v", err)
	}

	t.Run("ListDays filters by month and folds precipitation", func(t *testing.T) {
		got, err := weatherRepo.ListDays(ctx, calA.ID, 3, 1)
		if err != nil || len(got) != 2 {
			t.Fatalf("ListDays = %+v, %v; want 2 days", got, err)
		}
		if got[0].Day != 1 || got[0].Source != WeatherSourceManual || got[0].Precipitation == nil || *got[0].Precipitation.Type != "rain" {
			t.Errorf("first day = %+v", got[0])
		}
		all, _ := weatherRepo.ListDays(ctx, calA.ID, 3, 0)
		if len(all) != 3 {
			t.Errorf("whole year = %d days, want 3", len(all))
		}
	})

	t.Run("generated never replaces manual, but replaces generated", func(t *testing.T) {
		if err := weatherRepo.SetDays(ctx, calA.ID, []DayWeatherInput{
			day(1, 1, WeatherSourceGenerated, "snow"),
			day(1, 2, WeatherSourceGenerated, "hail"),
		}); err != nil {
			t.Fatalf("SetDays: %v", err)
		}
		got, _ := weatherRepo.ListDays(ctx, calA.ID, 3, 1)
		if *got[0].PresetID != "clear" || got[0].Source != WeatherSourceManual {
			t.Errorf("manual day = %s/%s, want clear/manual", *got[0].PresetID, got[0].Source)
		}
		if *got[1].PresetID != "hail" {
			t.Errorf("generated day = %s, want hail", *got[1].PresetID)
		}
	})

	t.Run("manual replaces generated and takes its source", func(t *testing.T) {
		if err := weatherRepo.SetDays(ctx, calA.ID, []DayWeatherInput{day(1, 2, WeatherSourceManual, "windy")}); err != nil {
			t.Fatalf("SetDays: %v", err)
		}
		got, _ := weatherRepo.ListDays(ctx, calA.ID, 3, 1)
		if *got[1].PresetID != "windy" || got[1].Source != WeatherSourceManual {
			t.Errorf("day = %s/%s, want windy/manual", *got[1].PresetID, got[1].Source)
		}
	})

	t.Run("lock protects a generated day, only existing rows change, and paint clears it", func(t *testing.T) {
		month2 := func() DayWeather {
			t.Helper()
			got, err := weatherRepo.ListDays(ctx, calA.ID, 3, 2)
			if err != nil || len(got) != 1 {
				t.Fatalf("ListDays month 2 = %+v, %v; want 1 day", got, err)
			}
			return got[0]
		}
		if d := month2(); d.Locked == nil || *d.Locked {
			t.Fatalf("a new day must start unlocked, got %v", d.Locked)
		}
		n, err := weatherRepo.LockDays(ctx, calA.ID, []DayDate{{3, 2, 1}, {3, 9, 9}}, true)
		if err != nil || n != 1 {
			t.Fatalf("LockDays = %d, %v; want 1 (the day with no reading is skipped)", n, err)
		}
		if gone, _ := weatherRepo.ListDays(ctx, calA.ID, 3, 9); len(gone) != 0 {
			t.Fatalf("locking must not create a row, got %+v", gone)
		}
		if d := month2(); d.Locked == nil || !*d.Locked {
			t.Fatalf("day should be locked, got %v", d.Locked)
		}
		if n, _ := weatherRepo.LockDays(ctx, calA.ID, []DayDate{{3, 2, 1}}, true); n != 0 {
			t.Errorf("locking a locked day changed %d rows, want 0", n)
		}

		if err := weatherRepo.SetDays(ctx, calA.ID, []DayWeatherInput{day(2, 1, WeatherSourceGenerated, "storm")}); err != nil {
			t.Fatalf("generated SetDays: %v", err)
		}
		d := month2()
		if *d.PresetID != "fog" || d.Source != WeatherSourceGenerated || !*d.Locked {
			t.Errorf("generated write over a locked day = %s/%s locked=%v, want fog/generated locked", *d.PresetID, d.Source, *d.Locked)
		}

		if err := weatherRepo.SetDays(ctx, calA.ID, []DayWeatherInput{day(2, 1, WeatherSourceManual, "gale")}); err != nil {
			t.Fatalf("manual SetDays: %v", err)
		}
		d = month2()
		if *d.PresetID != "gale" || d.Source != WeatherSourceManual || *d.Locked {
			t.Errorf("paint over a locked day = %s/%s locked=%v, want gale/manual unlocked", *d.PresetID, d.Source, *d.Locked)
		}

		// Unlock restores generated overwrites.
		if _, err := weatherRepo.LockDays(ctx, calA.ID, []DayDate{{3, 1, 2}}, true); err != nil {
			t.Fatalf("lock 1/2: %v", err)
		}
		if n, err := weatherRepo.LockDays(ctx, calA.ID, []DayDate{{3, 1, 2}}, false); err != nil || n != 1 {
			t.Fatalf("unlock = %d, %v; want 1", n, err)
		}
	})

	t.Run("days are scoped to their calendar", func(t *testing.T) {
		got, err := weatherRepo.ListDays(ctx, calB.ID, 3, 0)
		if err != nil || len(got) != 0 {
			t.Fatalf("calB = %+v, %v; want none", got, err)
		}
	})

	t.Run("ClearDays removes any source", func(t *testing.T) {
		if err := weatherRepo.ClearDays(ctx, calA.ID, []DayDate{{3, 1, 1}, {3, 2, 1}}); err != nil {
			t.Fatalf("ClearDays: %v", err)
		}
		got, _ := weatherRepo.ListDays(ctx, calA.ID, 3, 0)
		if len(got) != 1 || got[0].Day != 2 {
			t.Errorf("left = %+v, want only 1/2", got)
		}
	})

	t.Run("deleting the calendar cascades", func(t *testing.T) {
		if err := calRepo.Delete(ctx, calA.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM calendar_weather_days WHERE calendar_id = ?`, calA.ID).Scan(&n); err != nil || n != 0 {
			t.Errorf("rows after delete = %d, %v; want 0", n, err)
		}
	})
}

func TestWeatherRepository_Settings_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	repo := NewWeatherRepository(db)
	calRepo := NewCalendarRepository(db)

	fix := newTestCampaign(t, db, "wxsettings")
	calA := newTestCalendar(testUUID(t), fix.CampaignID, "Settings A")
	calB := newTestCalendar(testUUID(t), fix.CampaignID, "Settings B")
	for _, c := range []*Calendar{calA, calB} {
		if err := calRepo.Create(ctx, c); err != nil {
			t.Fatalf("Create %s: %v", c.Name, err)
		}
	}

	if s, err := repo.GetSettings(ctx, calA.ID); err != nil || s != nil {
		t.Fatalf("GetSettings with no row = %+v, %v; want (nil, nil)", s, err)
	}
	if err := repo.SetSettings(ctx, calA.ID, WeatherSettings{Climate: "desert", Continuity: 0.35}); err != nil {
		t.Fatalf("SetSettings: %v", err)
	}
	got, err := repo.GetSettings(ctx, calA.ID)
	if err != nil || got == nil || got.Climate != "desert" || got.Continuity != 0.35 {
		t.Fatalf("GetSettings = %+v, %v; want desert 0.35", got, err)
	}
	if err := repo.SetSettings(ctx, calA.ID, WeatherSettings{Climate: "ashlands", Continuity: 1}); err != nil {
		t.Fatalf("SetSettings update: %v", err)
	}
	got, _ = repo.GetSettings(ctx, calA.ID)
	if got == nil || got.Climate != "ashlands" || got.Continuity != 1 {
		t.Fatalf("after update = %+v; want ashlands 1", got)
	}
	if got.Kinds == nil || len(got.Kinds) != 0 {
		t.Fatalf("a row saved without kinds must read as an empty list, got %#v", got.Kinds)
	}
	if s, _ := repo.GetSettings(ctx, calB.ID); s != nil {
		t.Fatalf("calendar B must not see A's settings, got %+v", s)
	}

	// The forecast reach round-trips, and a row written without it (the
	// column default) reads as five days.
	if err := repo.SetSettings(ctx, calA.ID, WeatherSettings{Climate: "ashlands", Continuity: 1, ForecastDays: 7}); err != nil {
		t.Fatalf("SetSettings forecast days: %v", err)
	}
	if got, _ = repo.GetSettings(ctx, calA.ID); got == nil || got.ForecastDays != 7 {
		t.Fatalf("forecast days = %+v; want 7", got)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO calendar_weather_settings (calendar_id, climate) VALUES (?, 'desert')`, calB.ID); err != nil {
		t.Fatalf("insert settings without forecast_days: %v", err)
	}
	if got, _ = repo.GetSettings(ctx, calB.ID); got == nil || got.ForecastDays != 5 {
		t.Fatalf("default forecast days = %+v; want 5", got)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM calendar_weather_settings WHERE calendar_id = ?`, calB.ID); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	// Own kinds round-trip through the JSON column and are replaced whole.
	rich := fireRainClean
	rich.Lasts, rich.Words, rich.Magic = "stable", []string{"Embers fall"}, true
	rich.Look = &WeatherKindLook{Effect: "embers"}
	want := []WeatherKind{fireRainClean, kindWith(func(k *WeatherKind) { k.ID, k.Name = "mana-storm", "Mana storm" }), rich}
	want[2].ID, want[2].Name = "ember-fall", "Ember fall"
	if err := repo.SetSettings(ctx, calA.ID, WeatherSettings{Climate: "desert", Continuity: 0.5, Kinds: want}); err != nil {
		t.Fatalf("SetSettings with kinds: %v", err)
	}
	got, err = repo.GetSettings(ctx, calA.ID)
	if err != nil || got == nil || !reflect.DeepEqual(got.Kinds, want) {
		t.Fatalf("kinds round trip = %+v, %v; want %+v", got, err, want)
	}
	if err := repo.SetSettings(ctx, calA.ID, WeatherSettings{Climate: "desert", Continuity: 0.5}); err != nil {
		t.Fatalf("SetSettings clearing kinds: %v", err)
	}
	got, _ = repo.GetSettings(ctx, calA.ID)
	if got == nil || got.Kinds == nil || len(got.Kinds) != 0 {
		t.Fatalf("after clearing, kinds = %#v; want empty list", got)
	}
	// A NULL column (a row written before kinds existed) also reads as none.
	if _, err := db.ExecContext(ctx, `UPDATE calendar_weather_settings SET kinds = NULL WHERE calendar_id = ?`, calA.ID); err != nil {
		t.Fatalf("null kinds: %v", err)
	}
	got, _ = repo.GetSettings(ctx, calA.ID)
	if got == nil || got.Kinds == nil || len(got.Kinds) != 0 {
		t.Fatalf("NULL kinds = %#v; want empty list", got)
	}
}
