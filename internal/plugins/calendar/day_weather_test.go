package calendar

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// dayWeatherFixture is a two-month calendar whose today is year 5, month 2,
// day 10, in campaign "camp".
func dayWeatherFixture(weather *fakeWeatherRepo) CalendarService {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp", CurrentYear: 5, CurrentMonth: 2, CurrentDay: 10}, nil
		},
		getMonthsFn: func(context.Context, string) ([]Month, error) {
			return []Month{{Name: "One", Days: 30}, {Name: "Two", Days: 20}}, nil
		},
	}
	return newTestCalendarService(calRepo, nil, nil, weather)
}

func TestListDayWeather_PlayersSeeOnlyUpToToday(t *testing.T) {
	stored := []DayWeather{
		{Year: 5, Month: 1, Day: 30}, {Year: 5, Month: 2, Day: 9}, {Year: 5, Month: 2, Day: 10},
		{Year: 5, Month: 2, Day: 11}, {Year: 5, Month: 2, Day: 20},
	}
	weather := &fakeWeatherRepo{listDaysFn: func(context.Context, string, int, int) ([]DayWeather, error) {
		return stored, nil
	}}
	svc := dayWeatherFixture(weather)
	tests := []struct {
		name   string
		viewer permissions.Viewer
		year   int
		want   int
	}{
		{"player sees past and today", permissions.RequestViewer(int(permissions.RolePlayer), "u1"), 5, 3},
		{"scribe is a player for weather", permissions.RequestViewer(int(permissions.RoleScribe), "u1"), 5, 3},
		{"owner sees every day", permissions.RequestViewer(int(permissions.RoleOwner), "u1"), 5, 5},
		{"anonymous public viewer", permissions.RequestViewer(int(permissions.RolePlayer), ""), 5, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.ListDayWeather(context.Background(), "cal", "camp", tt.year, 0, tt.viewer)
			if err != nil {
				t.Fatalf("ListDayWeather: %v", err)
			}
			if len(got) != tt.want {
				t.Errorf("got %d days, want %d: %+v", len(got), tt.want, got)
			}
		})
	}
}

func TestListDayWeather_FutureYearSkipsQueryForPlayers(t *testing.T) {
	called := false
	weather := &fakeWeatherRepo{listDaysFn: func(context.Context, string, int, int) ([]DayWeather, error) {
		called = true
		return []DayWeather{{Year: 6, Month: 1, Day: 1}}, nil
	}}
	got, err := dayWeatherFixture(weather).ListDayWeather(context.Background(), "cal", "camp", 6, 0,
		permissions.RequestViewer(int(permissions.RolePlayer), "u1"))
	if err != nil || len(got) != 0 || called {
		t.Fatalf("got %+v, %v, queried=%v; want empty, nil, false", got, err, called)
	}
}

func TestListDayWeather_WrongCampaignIsNotFound(t *testing.T) {
	_, err := dayWeatherFixture(&fakeWeatherRepo{}).ListDayWeather(context.Background(), "cal", "other", 5, 0,
		permissions.RequestViewer(int(permissions.RoleOwner), "u1"))
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != 404 {
		t.Fatalf("err = %v, want 404", err)
	}
}

func TestSetDayWeather_Validation(t *testing.T) {
	str := func(s string) *string { return &s }
	num := func(f float64) *float64 { return &f }
	deg := func(i int) *int { return &i }
	day := func(y, m, d int) DayWeatherInput { return DayWeatherInput{Year: y, Month: m, Day: d} }
	with := func(d DayWeatherInput, f func(*DayWeatherInput)) DayWeatherInput { f(&d); return d }
	tests := []struct {
		name    string
		days    []DayWeatherInput
		wantErr bool
	}{
		{"valid future day", []DayWeatherInput{with(day(9, 2, 20), func(d *DayWeatherInput) { d.PresetID = str("rain") })}, false},
		{"year beyond the bound", []DayWeatherInput{day(3_000_000_000, 1, 1)}, true},
		{"month 0", []DayWeatherInput{day(5, 0, 1)}, true},
		{"month past the last", []DayWeatherInput{day(5, 3, 1)}, true},
		{"day past month end", []DayWeatherInput{day(5, 2, 21)}, true},
		{"duplicate day", []DayWeatherInput{day(5, 1, 1), day(5, 1, 1)}, true},
		{"unknown source", []DayWeatherInput{with(day(5, 1, 1), func(d *DayWeatherInput) { d.Source = "forecast" })}, true},
		{"bad colour", []DayWeatherInput{with(day(5, 1, 1), func(d *DayWeatherInput) { d.Color = str("red;x") })}, true},
		{"too hot", []DayWeatherInput{with(day(5, 1, 1), func(d *DayWeatherInput) { d.TemperatureCelsius = num(500) })}, true},
		{"NaN temperature", []DayWeatherInput{with(day(5, 1, 1), func(d *DayWeatherInput) { d.TemperatureCelsius = num(math.NaN()) })}, true},
		{"negative wind", []DayWeatherInput{with(day(5, 1, 1), func(d *DayWeatherInput) { d.WindSpeedKPH = num(-1) })}, true},
		{"degrees 360", []DayWeatherInput{with(day(5, 1, 1), func(d *DayWeatherInput) { d.WindDirectionDeg = deg(360) })}, true},
		{"intensity above 1", []DayWeatherInput{with(day(5, 1, 1), func(d *DayWeatherInput) { d.PrecipitationIntensity = num(1.5) })}, true},
		{"label too long", []DayWeatherInput{with(day(5, 1, 1), func(d *DayWeatherInput) { d.PresetLabel = str(string(make([]byte, 101))) })}, true},
		{"too many days", make([]DayWeatherInput, maxDayWeatherBatch+1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrote := false
			weather := &fakeWeatherRepo{setDaysFn: func(context.Context, string, []DayWeatherInput) error { wrote = true; return nil }}
			err := dayWeatherFixture(weather).SetDayWeather(context.Background(), "cal", "camp", tt.days)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if wrote == tt.wantErr {
				t.Errorf("repository written = %v, want %v", wrote, !tt.wantErr)
			}
		})
	}
}

func TestSetDayWeather_EmptySourceIsManual(t *testing.T) {
	var got []DayWeatherInput
	weather := &fakeWeatherRepo{setDaysFn: func(_ context.Context, _ string, days []DayWeatherInput) error { got = days; return nil }}
	err := dayWeatherFixture(weather).SetDayWeather(context.Background(), "cal", "camp", []DayWeatherInput{
		{Year: 5, Month: 1, Day: 1}, {Year: 5, Month: 1, Day: 2, Source: WeatherSourceGenerated},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Source != WeatherSourceManual || got[1].Source != WeatherSourceGenerated {
		t.Errorf("sources = %q, %q; want manual, generated", got[0].Source, got[1].Source)
	}
}

func TestClearDayWeather_RejectsBadDate(t *testing.T) {
	cleared := false
	weather := &fakeWeatherRepo{clearDaysFn: func(context.Context, string, []DayDate) error { cleared = true; return nil }}
	err := dayWeatherFixture(weather).ClearDayWeather(context.Background(), "cal", "camp", []DayDate{{5, 2, 25}})
	if err == nil || cleared {
		t.Fatalf("err = %v, cleared = %v; want an error and no write", err, cleared)
	}
}
