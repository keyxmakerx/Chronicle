package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

type fakeHeaderCalendar struct {
	cal        *calendar.Calendar
	calErr     error
	days       []calendar.DayWeather
	weatherErr error
	calls      int
}

func (f *fakeHeaderCalendar) GetDefaultCalendarForViewer(context.Context, string, permissions.Viewer) (*calendar.Calendar, error) {
	f.calls++
	return f.cal, f.calErr
}

func (f *fakeHeaderCalendar) ListDayWeather(context.Context, string, string, int, int, permissions.Viewer) ([]calendar.DayWeather, error) {
	return f.days, f.weatherErr
}

type fakeHeaderNights struct {
	night *sessions.NextNight
	err   error
	calls int
}

func (f *fakeHeaderNights) NextGameNight(context.Context, string, time.Time) (*sessions.NextNight, error) {
	f.calls++
	return f.night, f.err
}

func sp(s string) *string   { return &s }
func fp(f float64) *float64 { return &f }

func testCalendar() *calendar.Calendar {
	return &calendar.Calendar{
		ID: "cal1", CurrentYear: 1203, CurrentMonth: 1, CurrentDay: 14,
		Months: []calendar.Month{{Name: "Frostfall", Days: 30}},
		Moons:  []calendar.Moon{{Name: "Selûne", CycleDays: 30}},
	}
}

func TestBuildTopbarLive(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	night := &sessions.NextNight{Name: "Ashfall", Date: "2026-10-09", Time: "19:00", At: time.Date(2026, 10, 9, 19, 0, 0, 0, time.UTC)}
	today := []calendar.DayWeather{
		{Day: 13, PresetLabel: sp("Rain")},
		{Day: 14, PresetLabel: sp("Clear"), TemperatureCelsius: fp(3.6)},
		{Day: 15, PresetLabel: sp("Snow")},
	}
	all := []string{"date", "weather", "moon", "session"}

	cases := []struct {
		name         string
		cal          *fakeHeaderCalendar
		nights       *fakeHeaderNights
		req          headerLiveRequest
		wantNil      bool
		wantDate     string
		wantWeather  string
		wantMoons    int
		wantNight    string
		wantCalReads int
		wantNightRds int
	}{
		{"everything available",
			&fakeHeaderCalendar{cal: testCalendar(), days: today}, &fakeHeaderNights{night: night},
			headerLiveRequest{Widgets: all, Calendar: true, Nights: true, Now: now},
			false, "14 Frostfall 1203", "Clear, 4°", 1, "Session Fri 7 pm", 1, 1},
		{"weather is today only, never a neighbouring day",
			&fakeHeaderCalendar{cal: testCalendar(), days: []calendar.DayWeather{{Day: 15, PresetLabel: sp("Snow")}}}, &fakeHeaderNights{},
			headerLiveRequest{Widgets: []string{"weather"}, Calendar: true, Now: now},
			true, "", "", 0, "", 1, 0},
		{"calendar off: nothing from it, no read",
			&fakeHeaderCalendar{cal: testCalendar(), days: today}, &fakeHeaderNights{night: night},
			headerLiveRequest{Widgets: all, Calendar: false, Nights: true, Now: now},
			false, "", "", 0, "Session Fri 7 pm", 0, 1},
		{"nights off for a non-member: no read",
			&fakeHeaderCalendar{cal: testCalendar(), days: today}, &fakeHeaderNights{night: night},
			headerLiveRequest{Widgets: all, Calendar: true, Nights: false, Now: now},
			false, "14 Frostfall 1203", "Clear, 4°", 1, "", 1, 0},
		{"no calendar yet (not found) draws nothing",
			&fakeHeaderCalendar{calErr: apperror.NewNotFound("no calendar")}, &fakeHeaderNights{},
			headerLiveRequest{Widgets: []string{"date", "weather", "moon"}, Calendar: true, Now: now},
			true, "", "", 0, "", 1, 0},
		{"calendar error never breaks the page",
			&fakeHeaderCalendar{calErr: errors.New("db down")}, &fakeHeaderNights{night: night},
			headerLiveRequest{Widgets: all, Calendar: true, Nights: true, Now: now},
			false, "", "", 0, "Session Fri 7 pm", 1, 1},
		{"weather error leaves date and moon",
			&fakeHeaderCalendar{cal: testCalendar(), weatherErr: errors.New("boom")}, &fakeHeaderNights{},
			headerLiveRequest{Widgets: []string{"date", "weather", "moon"}, Calendar: true, Now: now},
			false, "14 Frostfall 1203", "", 1, "", 1, 0},
		{"no weather for today",
			&fakeHeaderCalendar{cal: testCalendar()}, &fakeHeaderNights{},
			headerLiveRequest{Widgets: []string{"date", "weather"}, Calendar: true, Now: now},
			false, "14 Frostfall 1203", "", 0, "", 1, 0},
		{"no upcoming game night",
			&fakeHeaderCalendar{}, &fakeHeaderNights{},
			headerLiveRequest{Widgets: []string{"session"}, Nights: true, Now: now},
			true, "", "", 0, "", 0, 1},
		{"game night error never breaks the page",
			&fakeHeaderCalendar{}, &fakeHeaderNights{err: errors.New("db down")},
			headerLiveRequest{Widgets: []string{"session"}, Nights: true, Now: now},
			true, "", "", 0, "", 0, 1},
		{"only the chosen widgets are read",
			&fakeHeaderCalendar{cal: testCalendar(), days: today}, &fakeHeaderNights{night: night},
			headerLiveRequest{Widgets: []string{"links", "text"}, Calendar: true, Nights: true, Now: now},
			true, "", "", 0, "", 0, 0},
		{"a moon with no cycle is skipped",
			&fakeHeaderCalendar{cal: func() *calendar.Calendar { c := testCalendar(); c.Moons[0].CycleDays = 0; return c }()}, &fakeHeaderNights{},
			headerLiveRequest{Widgets: []string{"moon"}, Calendar: true, Now: now},
			true, "", "", 0, "", 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildTopbarLive(context.Background(), tc.cal, tc.nights, tc.req)
			if tc.cal.calls != tc.wantCalReads || tc.nights.calls != tc.wantNightRds {
				t.Errorf("reads: calendar %d (want %d), nights %d (want %d)", tc.cal.calls, tc.wantCalReads, tc.nights.calls, tc.wantNightRds)
			}
			if tc.wantNil {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil, want data")
			}
			if got.Date != tc.wantDate || got.Weather != tc.wantWeather || len(got.Moons) != tc.wantMoons || got.NextNight.Label != tc.wantNight {
				t.Errorf("got date %q weather %q moons %d night %q", got.Date, got.Weather, len(got.Moons), got.NextNight.Label)
			}
		})
	}
}

func TestHeaderEraName(t *testing.T) {
	end := 1199
	cases := []struct {
		name string
		eras []calendar.Era
		want string
	}{
		{"no eras", nil, ""},
		{"today inside an ongoing era", []calendar.Era{{Name: " Age of Ash ", StartYear: 1100, StartMonth: 1, StartDay: 1}}, "Age of Ash"},
		{"the era that ended is skipped", []calendar.Era{
			{Name: "Old Kingdom", StartYear: 900, StartMonth: 1, StartDay: 1, EndYear: &end},
			{Name: "Age of Ash", StartYear: 1200, StartMonth: 1, StartDay: 1},
		}, "Age of Ash"},
		{"an era that has not begun is not today's", []calendar.Era{{Name: "Dawn", StartYear: 1203, StartMonth: 1, StartDay: 15}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := testCalendar()
			c.Eras = tc.eras
			if got := headerEraName(c); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildTopbarLive_Era(t *testing.T) {
	c := testCalendar()
	c.Eras = []calendar.Era{{Name: "Age of Ash", StartYear: 1100, StartMonth: 1, StartDay: 1}}
	got := buildTopbarLive(context.Background(), &fakeHeaderCalendar{cal: c}, &fakeHeaderNights{},
		headerLiveRequest{Widgets: []string{"era"}, Calendar: true})
	if got == nil || got.Era != "Age of Ash" || got.Date != "" || len(got.Moons) != 0 {
		t.Fatalf("got %+v, want only the era", got)
	}
	if got := buildTopbarLive(context.Background(), &fakeHeaderCalendar{cal: testCalendar()}, &fakeHeaderNights{},
		headerLiveRequest{Widgets: []string{"era"}, Calendar: true}); got != nil {
		t.Fatalf("today in no era: got %+v, want nil", got)
	}
}

func TestHeaderNightLabel(t *testing.T) {
	// Saturday 2026-10-03 12:00 UTC.
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	at := func(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, time.UTC) }
	cases := []struct {
		name      string
		n         sessions.NextNight
		want      string
		wantTitle string
	}{
		{"tonight", sessions.NextNight{Name: "A", Time: "19:00", At: at(2026, 10, 3, 19, 0)}, "Session today 7 pm", "A: Saturday 3 October, 7:00 pm"},
		{"tomorrow with minutes", sessions.NextNight{Name: "A", Time: "19:30", At: at(2026, 10, 4, 19, 30)}, "Session tomorrow 7:30 pm", ""},
		{"later this week", sessions.NextNight{Name: "A", Time: "20:00", At: at(2026, 10, 9, 20, 0)}, "Session Fri 8 pm", ""},
		{"a week or more away", sessions.NextNight{Name: "A", Time: "20:00", At: at(2026, 10, 12, 20, 0)}, "Session in 9 days", ""},
		{"no time set", sessions.NextNight{Name: "A", At: at(2026, 10, 3, 23, 59)}, "Session today", "A: Saturday 3 October"},
		{"zone shown in the title", sessions.NextNight{Name: "A", Time: "19:00", TZ: "UTC", At: at(2026, 10, 5, 19, 0)}, "Session Mon 7 pm", "A: Monday 5 October, 7:00 pm (UTC)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, title := headerNightLabel(&tc.n, now)
			if got != tc.want {
				t.Errorf("label %q, want %q", got, tc.want)
			}
			if tc.wantTitle != "" && !strings.HasPrefix(title, tc.wantTitle) {
				t.Errorf("title %q, want prefix %q", title, tc.wantTitle)
			}
		})
	}
}

func TestHeaderWeatherLabel(t *testing.T) {
	cases := []struct {
		name  string
		days  []calendar.DayWeather
		today int
		want  string
	}{
		{"label and temperature", []calendar.DayWeather{{Day: 2, PresetLabel: sp("Clear"), TemperatureCelsius: fp(-2.6)}}, 2, "Clear, -3°"},
		{"label only", []calendar.DayWeather{{Day: 2, PresetLabel: sp("Fog")}}, 2, "Fog"},
		{"temperature only", []calendar.DayWeather{{Day: 2, TemperatureCelsius: fp(10)}}, 2, "10°"},
		{"blank label ignored", []calendar.DayWeather{{Day: 2, PresetLabel: sp("  ")}}, 2, ""},
		{"other days never read", []calendar.DayWeather{{Day: 3, PresetLabel: sp("Snow")}}, 2, ""},
		{"empty month", nil, 2, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := headerWeatherLabel(tc.days, tc.today); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHeaderSkyCalendarID(t *testing.T) {
	cases := []struct {
		name string
		cal  *fakeHeaderCalendar
		want string
	}{
		{"the default calendar", &fakeHeaderCalendar{cal: testCalendar()}, "cal1"},
		{"none, or not this viewer's", &fakeHeaderCalendar{calErr: apperror.NewNotFound("calendar not found")}, ""},
		{"a failed read", &fakeHeaderCalendar{calErr: errors.New("db down")}, ""},
		{"no calendar and no error", &fakeHeaderCalendar{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := headerSkyCalendarID(context.Background(), tc.cal, "camp1", permissions.Viewer{}); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
	if got := headerSkyCalendarID(context.Background(), nil, "camp1", permissions.Viewer{}); got != "" {
		t.Errorf("no calendar service: got %q", got)
	}
}
