// calendar_weather_seam_test.go — the weather unification seam
// (cordinator#53 / C-CAL-PARITY). Pins the day-row-first / legacy-fallback
// read, the current-day canonical write, the zone-pointer split, and the
// worldstate.changed nudge — the behaviors that make GM-panel weather and
// Foundry-synced weather finally share one store.
package calendar

import (
	"context"
	"testing"
)

// seamCal returns the minimal calendar the seam paths need (current date +
// campaign for the WS publish).
func seamCal() *Calendar {
	return &Calendar{ID: "cal-1", CampaignID: "camp-1", CurrentYear: 1492, CurrentMonth: 6, CurrentDay: 15}
}

// TestGetWeather_DayRowWins: when the current day carries an authored row,
// GetWeather serves it (projected into the Weather wire shape) — the legacy
// calendar_weather row only contributes the active-zone pointer.
func TestGetWeather_DayRowWins(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return seamCal(), nil },
		getDayWeatherFn: func(_ context.Context, _ string, year, month, day int) (*DayWeather, error) {
			if year != 1492 || month != 6 || day != 15 {
				t.Errorf("day-weather read targeted %d-%d-%d; want current 1492-6-15", year, month, day)
			}
			temp := 21.5
			return &DayWeather{
				ID: 7, CalendarID: "cal-1", Year: year, Month: month, Day: day,
				WeatherType: "thunderstorm", TemperatureCelsius: &temp,
			}, nil
		},
		getWeatherFn: func(_ context.Context, _ string) (*Weather, error) {
			// Stale legacy row — must NOT win, except for the zone pointer.
			return &Weather{PresetID: strPtr("clear"), ZoneID: strPtr("temperate"), ZoneName: strPtr("Temperate")}, nil
		},
	}
	w, err := newTestCalendarService(repo).GetWeather(context.Background(), "cal-1")
	if err != nil {
		t.Fatalf("GetWeather: %v", err)
	}
	if w == nil || w.PresetID == nil || *w.PresetID != "thunderstorm" {
		t.Fatalf("PresetID = %v, want thunderstorm (day row must win over legacy)", w)
	}
	if w.TemperatureCelsius == nil || *w.TemperatureCelsius != 21.5 {
		t.Errorf("TemperatureCelsius = %v, want 21.5", w.TemperatureCelsius)
	}
	if w.ZoneID == nil || *w.ZoneID != "temperate" {
		t.Errorf("ZoneID = %v, want temperate (zone pointer grafts from calendar_weather)", w.ZoneID)
	}
}

// TestGetWeather_LegacyFallback: no day row → the pre-seam calendar_weather
// state keeps serving unchanged (read-through fallback; no data migration).
func TestGetWeather_LegacyFallback(t *testing.T) {
	legacy := &Weather{ID: 3, CalendarID: "cal-1", PresetID: strPtr("rain"), Description: strPtr("Steady rain")}
	repo := &mockCalendarRepo{
		getByIDFn:    func(_ context.Context, _ string) (*Calendar, error) { return seamCal(), nil },
		getWeatherFn: func(_ context.Context, _ string) (*Weather, error) { return legacy, nil },
	}
	w, err := newTestCalendarService(repo).GetWeather(context.Background(), "cal-1")
	if err != nil {
		t.Fatalf("GetWeather: %v", err)
	}
	if w != legacy {
		t.Fatalf("GetWeather = %+v, want the legacy row verbatim when no day row exists", w)
	}
}

// TestGetWeather_EmptyTypeMapsToNilPreset: a day row whose weather_type is
// "" (a rich-only write with no preset ever set) serves a nil preset_id —
// same as the legacy row's "unset" representation.
func TestGetWeather_EmptyTypeMapsToNilPreset(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return seamCal(), nil },
		getDayWeatherFn: func(_ context.Context, _ string, y, m, d int) (*DayWeather, error) {
			temp := 12.0
			return &DayWeather{CalendarID: "cal-1", Year: y, Month: m, Day: d, WeatherType: "", TemperatureCelsius: &temp}, nil
		},
	}
	w, err := newTestCalendarService(repo).GetWeather(context.Background(), "cal-1")
	if err != nil {
		t.Fatalf("GetWeather: %v", err)
	}
	if w.PresetID != nil {
		t.Errorf("PresetID = %q, want nil for an empty weather_type", *w.PresetID)
	}
}

// TestSetWeather_MergesFromDayRow: the null-preserve merge base is the seam
// READ — a partial write on a day that already has an authored row preserves
// that row's fields, not the stale legacy row's.
func TestSetWeather_MergesFromDayRow(t *testing.T) {
	var written WeatherInput
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return seamCal(), nil },
		getDayWeatherFn: func(_ context.Context, _ string, y, m, d int) (*DayWeather, error) {
			temp := 30.0
			return &DayWeather{CalendarID: "cal-1", Year: y, Month: m, Day: d, WeatherType: "heatwave", TemperatureCelsius: &temp}, nil
		},
		getWeatherFn: func(_ context.Context, _ string) (*Weather, error) {
			stale := -5.0
			return &Weather{PresetID: strPtr("blizzard"), TemperatureCelsius: &stale}, nil
		},
		setDayWeatherRichFn: func(_ context.Context, _ string, _, _, _ int, _ string, in WeatherInput) error {
			written = in
			return nil
		},
	}
	wind := 20.0
	if err := newTestCalendarService(repo).SetWeather(context.Background(), "cal-1",
		WeatherInput{WindSpeedKPH: &wind}); err != nil {
		t.Fatalf("SetWeather: %v", err)
	}
	if written.PresetID == nil || *written.PresetID != "heatwave" {
		t.Errorf("merge base PresetID = %v, want heatwave (day row, not legacy blizzard)", written.PresetID)
	}
	if written.TemperatureCelsius == nil || *written.TemperatureCelsius != 30.0 {
		t.Errorf("merge base temperature = %v, want 30 (day row, not legacy -5)", written.TemperatureCelsius)
	}
	if written.WindSpeedKPH == nil || *written.WindSpeedKPH != 20.0 {
		t.Errorf("input wind = %v, want 20", written.WindSpeedKPH)
	}
}

// TestSetWeather_ZoneInputUpdatesPointer: zone fields on the input land on
// the calendar_weather row (the active-zone pointer, carried by the rolling
// mirror upsert — zones stay legacy-rowed until W2), not on the day row.
func TestSetWeather_ZoneInputUpdatesPointer(t *testing.T) {
	var mirrored WeatherInput
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return seamCal(), nil },
		setWeatherFn: func(_ context.Context, _ string, in WeatherInput) error {
			mirrored = in
			return nil
		},
	}
	if err := newTestCalendarService(repo).SetWeather(context.Background(), "cal-1",
		WeatherInput{ZoneID: strPtr("arctic"), ZoneName: strPtr("Arctic")}); err != nil {
		t.Fatalf("SetWeather: %v", err)
	}
	if mirrored.ZoneID == nil || *mirrored.ZoneID != "arctic" || mirrored.ZoneName == nil || *mirrored.ZoneName != "Arctic" {
		t.Errorf("mirror zone = (%v,%v), want (arctic,Arctic)", mirrored.ZoneID, mirrored.ZoneName)
	}
}

// TestSetWeather_DayBoundaryContinuity: the ROLLING mirror is the merge base
// on a fresh day (no day row yet) — the previous day's merged state carries
// forward instead of blanking (the seam review's P1: a frozen/nil fallback
// made the first sparse write of each new day lose the preset/rich fields).
func TestSetWeather_DayBoundaryContinuity(t *testing.T) {
	// Rolling snapshot left by yesterday's write.
	rolling := &Weather{
		PresetID:           strPtr("rain"),
		PresetLabel:        strPtr("Rain"),
		TemperatureCelsius: floatPtr(5),
	}
	var written WeatherInput
	var wroteType string
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			c := seamCal()
			c.CurrentDay = 16 // a NEW day; getDayWeatherFn default returns nil
			return c, nil
		},
		getWeatherFn: func(_ context.Context, _ string) (*Weather, error) { return rolling, nil },
		setDayWeatherRichFn: func(_ context.Context, _ string, _, _, _ int, weatherType string, in WeatherInput) error {
			written, wroteType = in, weatherType
			return nil
		},
	}
	temp := 8.0
	if err := newTestCalendarService(repo).SetWeather(context.Background(), "cal-1",
		WeatherInput{TemperatureCelsius: &temp}); err != nil {
		t.Fatalf("SetWeather: %v", err)
	}
	if written.PresetID == nil || *written.PresetID != "rain" || wroteType != "rain" {
		t.Errorf("fresh-day sparse write lost the preset: got %v/%q, want rain", written.PresetID, wroteType)
	}
	if written.TemperatureCelsius == nil || *written.TemperatureCelsius != 8 {
		t.Errorf("input temperature = %v, want 8", written.TemperatureCelsius)
	}
	if written.PresetLabel == nil || *written.PresetLabel != "Rain" {
		t.Errorf("fresh-day sparse write lost the label: got %v", written.PresetLabel)
	}
}

// TestSetWeather_PublishesWeatherAndWorldstateChanged: a weather write now
// lands on the rendered day store, so it must emit BOTH the legacy
// calendar.weather.changed and the worldstate.changed nudge (band re-render
// + Foundry W5 bridge re-GET).
func TestSetWeather_PublishesWeatherAndWorldstateChanged(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return seamCal(), nil },
	}
	svc := NewCalendarService(repo)
	pub := &recordingPublisher{}
	svc.(interface{ SetEventPublisher(CalendarEventPublisher) }).SetEventPublisher(pub)

	if err := svc.SetWeather(context.Background(), "cal-1", WeatherInput{PresetID: strPtr("rain")}); err != nil {
		t.Fatalf("SetWeather: %v", err)
	}
	got := pub.types()
	if !contains(got, "calendar.weather.changed") {
		t.Errorf("calendar.weather.changed not published; got %v", got)
	}
	if !contains(got, "calendar.worldstate.changed") {
		t.Errorf("calendar.worldstate.changed not published (band/bridge nudge); got %v", got)
	}
}
