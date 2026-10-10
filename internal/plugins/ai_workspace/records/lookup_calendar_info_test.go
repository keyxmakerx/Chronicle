package records

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

type fakeWeatherSettings struct{ ws calendar.WeatherSettings }

func (f fakeWeatherSettings) GetWeatherSettings(context.Context, string, string, permissions.Viewer) (*calendar.WeatherSettings, error) {
	return &f.ws, nil
}

func calendarWorld() *Lookups {
	c := newCal()
	label := "DR"
	c.cal.EpochName = &label
	c.cal.HoursPerDay = 24
	c.cal.CurrentYear, c.cal.CurrentMonth, c.cal.CurrentDay = 1492, 1, 5
	c.cal.Weekdays = []calendar.Weekday{{Name: "First"}, {Name: "Rest", IsRestDay: true}}
	c.cal.Seasons = []calendar.Season{{Name: "Winter", StartMonth: 1, StartDay: 1, EndMonth: 1, EndDay: 30}}
	c.cal.Moons = []calendar.Moon{{Name: "Selune", CycleDays: 10, Color: "#eeeeee"}}
	end := 1400
	c.cal.Eras = []calendar.Era{{Name: "Old Age", StartYear: 1, StartMonth: 1, StartDay: 1, EndYear: &end}}
	return &Lookups{Cal: c, Weather: fakeWeatherSettings{calendar.WeatherSettings{
		Climate: "tundra", Continuity: 0.7, ForecastDays: 3, ForecastsEnabled: true,
		Kinds: []calendar.WeatherKind{{ID: "ember-rain", Name: "Ember rain", Like: "rain", Look: &calendar.WeatherKindLook{Effect: "embers"}}},
	}}}
}

func TestLookups_Calendar(t *testing.T) {
	ans := calendarWorld().Answer(context.Background(), camp, owner, []Record{lookup(lookupWhatCalendar, nil)})[0]
	if ans.Error != "" {
		t.Fatal(ans.Error)
	}
	for _, want := range []string{
		"Year label: DR", "Today: Hammer 5 1492 DR", "Winter",
		"1. Hammer: 30 days", "2. Alturiak: 30 days", "First, Rest (rest day)",
		"Winter: Hammer 1 to Hammer 30", "Selune: a 10-day cycle", "next full moon",
		"Old Age: from Hammer 1 1 to the end of 1400",
		"Climate: Tundra (`tundra`)", "forecast 3 days ahead", "Ember rain",
	} {
		if !strings.Contains(ans.Text, want) {
			t.Errorf("answer lacks %q:\n%s", want, ans.Text)
		}
	}
}

func TestLookups_WeatherKinds(t *testing.T) {
	ans := calendarWorld().Answer(context.Background(), camp, owner, []Record{lookup("weather-kinds", nil)})[0]
	if ans.Error != "" {
		t.Fatal(ans.Error)
	}
	// Every list comes from the server's own tables, so a new entry appears.
	for _, c := range calendar.WeatherClimates {
		if !strings.Contains(ans.Text, "`"+c.ID+"`") {
			t.Errorf("climate %s missing", c.ID)
		}
	}
	for _, p := range calendar.WeatherPresets() {
		if !strings.Contains(ans.Text, p.Label) {
			t.Errorf("weather %s missing", p.Label)
		}
	}
	for _, e := range calendar.WeatherEffects() {
		if !strings.Contains(ans.Text, "`"+e.ID+"`") {
			t.Errorf("effect %s missing", e.ID)
		}
	}
	if !strings.Contains(ans.Text, "Ember rain: like rain, drawn as embers") {
		t.Errorf("owner's kind missing:\n%s", ans.Text)
	}
}

func TestNextPhases(t *testing.T) {
	c := newCal().cal
	m := calendar.Moon{Name: "Selune", CycleDays: 7.5, PhaseOffset: 2}
	today := calDate{1492, 1, 28}
	nm, fm := nextPhases(&c, m, today)
	pn := m.MoonPhase(c.AbsoluteDay(nm.Y, nm.M, nm.D))
	pf := m.MoonPhase(c.AbsoluteDay(fm.Y, fm.M, fm.D))
	if math.Min(pn, 1-pn) > 1/7.5 || math.Abs(pf-0.5) > 1/7.5 {
		t.Fatalf("new %v (phase %.2f) full %v (phase %.2f)", nm, pn, fm, pf)
	}
	if fm.Before(today) || nm.Before(today) {
		t.Fatal("phases must be today or later")
	}
}
