// weather_repository_test.go exercises WeatherRepository against a real
// MariaDB: the upsert (Set creates, then replaces), tenant isolation via
// calendar_id scoping, and the cascade from a deleted calendar. Skipped
// under `-short`.
package calendar

import (
	"context"
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
