package calendar

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeToday answers like the calendar service for a viewer: a Director-only
// event and a hidden moon reach only a viewer at the owner's role, which is
// the filtering the real service does and this block must leave to it.
type fakeToday struct {
	cal      *Calendar
	calErr   error
	weather  []DayWeather
	events   []Event
	weatherQ [][2]int
}

func (f *fakeToday) GetDefaultCalendarForViewer(_ context.Context, _ string, v permissions.Viewer) (*Calendar, error) {
	if f.calErr != nil || f.cal == nil {
		return nil, f.calErr
	}
	cal := *f.cal
	cal.Moons = nil
	for _, m := range f.cal.Moons {
		if m.HiddenFromPlayers && v.Role() < 3 {
			continue
		}
		cal.Moons = append(cal.Moons, m)
	}
	return &cal, nil
}

func (f *fakeToday) ListDayWeather(_ context.Context, _, _ string, year, month int, _ permissions.Viewer) ([]DayWeather, error) {
	f.weatherQ = append(f.weatherQ, [2]int{year, month})
	return f.weather, nil
}

func (f *fakeToday) ListUpcomingEvents(_ context.Context, _, _ string, _ int, v permissions.Viewer) ([]Event, error) {
	var out []Event
	for _, e := range f.events {
		if e.Visibility == "dm_only" && v.Role() < 3 {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func todayCalendar() *Calendar {
	months := []Month{{Name: "Emberfall", Days: 30}, {Name: "Frostwane", Days: 30}}
	return &Calendar{
		ID: "cal-1", Months: months, CurrentYear: 1492, CurrentMonth: 1, CurrentDay: 14,
		Moons: []Moon{
			{Name: "Selune", CycleDays: 28},
			{Name: "Dread", CycleDays: 40, HiddenFromPlayers: true},
		},
	}
}

func reading(day int, label string, temp float64) DayWeather {
	return DayWeather{Day: day, Month: 1, Year: 1492, PresetLabel: &label, TemperatureCelsius: &temp}
}

func TestBuildToday(t *testing.T) {
	player := permissions.RequestViewer(1, "u1")
	owner := permissions.RequestViewer(3, "dm")
	festival := Event{Name: "Festival of Lanterns", Year: 1492, Month: 1, Day: 17}
	secret := Event{Name: "The Pretender Lands", Year: 1492, Month: 1, Day: 15, Visibility: "dm_only"}

	tests := []struct {
		name        string
		src         *fakeToday
		viewer      permissions.Viewer
		owner       bool
		wantDate    string
		wantWeather string
		wantMoons   int
		wantNext    string
		wantSetUp   bool
		wantEmpty   bool
	}{
		{"everything", &fakeToday{cal: todayCalendar(), weather: []DayWeather{reading(13, "Clear", 9), reading(14, "Cold rain", 4), reading(15, "Storm", 2)}, events: []Event{festival}},
			player, false, "Emberfall 14, 1492", "Cold rain, 4°", 1, "Festival of Lanterns in 3 days", false, false},
		{"a player never learns of the Director-only event or moon", &fakeToday{cal: todayCalendar(), events: []Event{secret, festival}},
			player, false, "Emberfall 14, 1492", "", 1, "Festival of Lanterns in 3 days", false, false},
		{"the owner's own view keeps both", &fakeToday{cal: todayCalendar(), events: []Event{secret, festival}},
			owner, true, "Emberfall 14, 1492", "", 2, "The Pretender Lands tomorrow", false, false},
		{"no weather leaves the line out", &fakeToday{cal: todayCalendar(), weather: []DayWeather{reading(15, "Storm", 2)}},
			player, false, "Emberfall 14, 1492", "", 1, "", false, false},
		{"an event today", &fakeToday{cal: todayCalendar(), events: []Event{{Name: "Market day", Year: 1492, Month: 1, Day: 14}}},
			player, false, "Emberfall 14, 1492", "", 1, "Market day today", false, false},
		{"owner with no calendar is told", &fakeToday{calErr: apperror.NewNotFound("calendar")},
			owner, true, "", "", 0, "", true, false},
		{"a player with no calendar gets nothing", &fakeToday{calErr: apperror.NewNotFound("calendar")},
			player, false, "", "", 0, "", false, true},
		{"a failing calendar gets nothing", &fakeToday{calErr: errors.New("db down")},
			owner, true, "", "", 0, "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := buildToday(context.Background(), tt.src, "c1", tt.viewer, tt.owner)
			if v.Date != tt.wantDate || v.Weather != tt.wantWeather || len(v.Moons) != tt.wantMoons || v.Next != tt.wantNext || v.SetUp != tt.wantSetUp {
				t.Errorf("got %+v", v)
			}
			if v.Empty() != tt.wantEmpty {
				t.Errorf("Empty() = %v, want %v for %+v", v.Empty(), tt.wantEmpty, v)
			}
		})
	}
}

// Only today's month is read for weather, so a forecast day cannot be named.
func TestBuildTodayReadsOnlyTodaysWeather(t *testing.T) {
	src := &fakeToday{cal: todayCalendar(), weather: []DayWeather{reading(14, "Clear", 5), reading(20, "Blizzard", -9)}}
	v := buildToday(context.Background(), src, "c1", permissions.RequestViewer(0, ""), false)
	if len(src.weatherQ) != 1 || src.weatherQ[0] != [2]int{1492, 1} {
		t.Errorf("weather read for %v, want only the current month", src.weatherQ)
	}
	if strings.Contains(v.Weather, "Blizzard") || v.Weather != "Clear, 5°" {
		t.Errorf("weather = %q", v.Weather)
	}
}

// A guest's copy is built from the same view type: nothing but worded lines.
func TestTodayInWorldGuestBlockLeaksNothing(t *testing.T) {
	festival := Event{Name: "Festival of Lanterns", Year: 1492, Month: 1, Day: 17}
	secret := Event{Name: "The Pretender Lands", Year: 1492, Month: 1, Day: 15, Visibility: "dm_only"}
	src := &fakeToday{cal: todayCalendar(), weather: []DayWeather{reading(14, "Cold rain", 4), reading(15, "Hidden storm", 1)}, events: []Event{secret, festival}}
	view := buildToday(context.Background(), src, "c1", permissions.RequestViewer(0, ""), false)
	var b bytes.Buffer
	if err := todayInWorldBlock(view).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	for _, not := range []string{"Pretender", "Dread", "Hidden storm", "dm_only", "No calendar is set up"} {
		if strings.Contains(html, not) {
			t.Errorf("guest block holds %q:\n%s", not, html)
		}
	}
	for _, want := range []string{"Today in the world", "Emberfall 14, 1492", "Cold rain", "Selune", "Festival of Lanterns in 3 days", "Open calendar", "/campaigns/c1/calendars/cal-1/view"} {
		if !strings.Contains(html, want) {
			t.Errorf("guest block missing %q:\n%s", want, html)
		}
	}
}

func TestTodayInDays(t *testing.T) {
	for days, want := range map[int]string{-2: "today", 0: "today", 1: "tomorrow", 3: "in 3 days"} {
		if got := todayInDays(days); got != want {
			t.Errorf("todayInDays(%d) = %q, want %q", days, got, want)
		}
	}
}

// todaySvc lets the handler run over the fake: the three reads the block makes
// are the fake's, and any other call would panic on the nil interface.
type todaySvc struct {
	CalendarService
	*fakeToday
}

func (s todaySvc) GetDefaultCalendarForViewer(ctx context.Context, id string, v permissions.Viewer) (*Calendar, error) {
	return s.fakeToday.GetDefaultCalendarForViewer(ctx, id, v)
}

func (s todaySvc) ListDayWeather(ctx context.Context, a, b string, y, m int, v permissions.Viewer) ([]DayWeather, error) {
	return s.fakeToday.ListDayWeather(ctx, a, b, y, m, v)
}

func (s todaySvc) ListUpcomingEvents(ctx context.Context, a, b string, n int, v permissions.Viewer) ([]Event, error) {
	return s.fakeToday.ListUpcomingEvents(ctx, a, b, n, v)
}

// guestCampaign is a public campaign whose Guests home places the block, or
// not.
func guestCampaign(t *testing.T, placed bool) *campaigns.Campaign {
	t.Helper()
	camp := &campaigns.Campaign{ID: "c1", IsPublic: true}
	if placed {
		layout := &campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{{ID: "r", Columns: []campaigns.DashboardColumn{{
			Blocks: []campaigns.DashboardBlock{{ID: "b", Type: campaigns.BlockTodayInWorld}}}}}}}
		full, err := camp.SetRoleDashboardJSON(campaigns.HomeGuest, layout)
		if err != nil {
			t.Fatal(err)
		}
		camp.DashboardLayout = full
	}
	return camp
}

func TestTodayInWorldBlockGuestPlacement(t *testing.T) {
	tests := []struct {
		name     string
		placed   bool
		wantBody bool
	}{
		{"not placed on the guests home", false, false},
		{"placed on the guests home", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{svc: todaySvc{fakeToday: &fakeToday{cal: todayCalendar()}}}
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/today-in-world", nil), rec)
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: guestCampaign(t, tt.placed)})
			if err := h.TodayInWorldBlock(c); err != nil {
				t.Fatalf("handler: %v", err)
			}
			if got := strings.Contains(rec.Body.String(), "Emberfall 14, 1492"); got != tt.wantBody {
				t.Errorf("body has the date = %v, want %v:\n%s", got, tt.wantBody, rec.Body.String())
			}
		})
	}
}
