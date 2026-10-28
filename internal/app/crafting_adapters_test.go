package app

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

func TestSheetNumber(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int
		ok   bool
	}{
		{"a number", float64(2), 2, true},
		{"a number rounds", 2.6, 3, true},
		{"a negative number", float64(-1), -1, true},
		{"a whole number", 5, 5, true},
		{"a JSON number", json.Number("1.4"), 1, true},
		{"text holding a number", " 3 ", 3, true},
		{"a signed bonus", "+2", 2, true},
		{"a minus", "-1", -1, true},
		{"words", "two", 0, false},
		{"empty text", "", 0, false},
		{"not a number", math.NaN(), 0, false},
		{"no value", nil, 0, false},
		{"a yes or no", true, 0, false},
		{"a list", []any{1}, 0, false},
	}
	for _, c := range cases {
		got, ok, err := sheetNumber(c.in)
		if err != nil || got != c.want || ok != c.ok {
			t.Errorf("%s: got %d %v %v, want %d %v", c.name, got, ok, err, c.want, c.ok)
		}
	}
}

func TestEventOn(t *testing.T) {
	cases := []struct {
		name string
		e    calendar.Event
		want bool
	}{
		{"a one-off on the day", calendar.Event{Year: 3, Month: 2, Day: 12}, true},
		{"a one-off another day", calendar.Event{Year: 3, Month: 2, Day: 11}, false},
		{"a one-off another year", calendar.Event{Year: 2, Month: 2, Day: 12}, false},
		{"a repeat that falls on the day", calendar.Event{Year: 1, Month: 1, Day: 1, Occurrences: []calendar.Occurrence{{Year: 3, Month: 2, Day: 5}, {Year: 3, Month: 2, Day: 12}}}, true},
		{"a repeat skipped that day", calendar.Event{Year: 1, Month: 1, Day: 1, Occurrences: []calendar.Occurrence{{Year: 3, Month: 2, Day: 12, Skipped: true}}}, false},
		{"a repeat moved to the day", calendar.Event{Year: 1, Month: 1, Day: 1, Occurrences: []calendar.Occurrence{{Year: 3, Month: 2, Day: 12, MovedFrom: &calendar.DayDate{Year: 3, Month: 2, Day: 9}}}}, true},
		// A repeat whose start date is today but has no date this month
		// is not on today: its occurrences are what count.
		{"a repeat with no date this month", calendar.Event{Year: 3, Month: 2, Day: 12, Occurrences: []calendar.Occurrence{}}, false},
	}
	for _, c := range cases {
		if got := eventOn(c.e, 3, 2, 12); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

// fakeCraftingCalendar is the calendar a project roll reads.
type fakeCraftingCalendar struct {
	cal     *calendar.Calendar
	err     error
	events  []calendar.Event
	weather []calendar.DayWeather
	viewers []permissions.Viewer
}

func (f *fakeCraftingCalendar) GetDefaultCalendarForViewer(_ context.Context, _ string, v permissions.Viewer) (*calendar.Calendar, error) {
	f.viewers = append(f.viewers, v)
	return f.cal, f.err
}

func (f *fakeCraftingCalendar) ListEventsForMonth(context.Context, string, string, int, int, permissions.Viewer) ([]calendar.Event, error) {
	return f.events, nil
}

func (f *fakeCraftingCalendar) ListDayWeather(context.Context, string, string, int, int, permissions.Viewer) ([]calendar.DayWeather, error) {
	return f.weather, nil
}

func TestCraftingDaysToday(t *testing.T) {
	ctx := context.Background()
	snow, rain := "Snow", "Rain"
	month := []calendar.Month{{ID: 1, Name: "Deepwinter", Days: 30, SortOrder: 1}, {ID: 2, Name: "Frostmoot", Days: 30, SortOrder: 2}}
	cal := &calendar.Calendar{ID: "cal-1", CurrentYear: 3, CurrentMonth: 2, CurrentDay: 12, Months: month,
		Seasons: []calendar.Season{{Name: "Winter", StartMonth: 1, StartDay: 1, EndMonth: 2, EndDay: 30}}}

	t.Run("today's date, events, weather and season", func(t *testing.T) {
		f := &fakeCraftingCalendar{cal: cal,
			events:  []calendar.Event{{ID: "e1", Name: "Market day", Year: 3, Month: 2, Day: 12}, {ID: "e2", Name: "Feast", Year: 3, Month: 2, Day: 20}},
			weather: []calendar.DayWeather{{Year: 3, Month: 2, Day: 11, PresetLabel: &rain}, {Year: 3, Month: 2, Day: 12, PresetLabel: &snow}}}
		day, err := (&craftingDaysAdapter{cal: f}).Today(ctx, "camp")
		if err != nil || day == nil {
			t.Fatalf("today: %+v, %v", day, err)
		}
		if day.Label != "12 Frostmoot" || !reflect.DeepEqual(day.Events, []string{"Market day"}) || day.Weather != "Snow" || day.Season != "Winter" {
			t.Errorf("today = %+v", day)
		}
		// The recipe's rules are the Director's, so the day is read with
		// the Director's sight whoever rolls.
		if len(f.viewers) != 1 || f.viewers[0].Role() < permissions.RoleOwner {
			t.Errorf("read as %+v", f.viewers)
		}
	})

	for name, f := range map[string]*fakeCraftingCalendar{
		"no calendar":        {err: apperror.NewNotFound("calendar not found")},
		"a calendar not set": {cal: &calendar.Calendar{ID: "cal-2", Months: month}},
		"nothing came back":  {},
	} {
		t.Run(name+" means no today", func(t *testing.T) {
			day, err := (&craftingDaysAdapter{cal: f}).Today(ctx, "camp")
			if err != nil || day != nil {
				t.Errorf("today: %+v, %v", day, err)
			}
		})
	}

	t.Run("a calendar failing is an error", func(t *testing.T) {
		if _, err := (&craftingDaysAdapter{cal: &fakeCraftingCalendar{err: errors.New("db down")}}).Today(ctx, "camp"); err == nil {
			t.Error("no error")
		}
	})
	t.Run("no calendar plugin", func(t *testing.T) {
		if day, err := (&craftingDaysAdapter{}).Today(ctx, "camp"); err != nil || day != nil {
			t.Errorf("today: %+v, %v", day, err)
		}
	})
}

// fakeCraftingRespites hears respites pressed in the Homebrew drawer.
type fakeCraftingRespites struct {
	crafting.Service
	heard []crafting.Viewer
}

func (f *fakeCraftingRespites) RespitePressed(_ context.Context, _ string, v crafting.Viewer) error {
	f.heard = append(f.heard, v)
	return nil
}

func TestCraftingRespiteListener(t *testing.T) {
	ctx := context.Background()
	by := homebrew.Viewer{UserID: "dm", Director: true, VisibilityRole: 3}
	for _, c := range []struct {
		name    string
		enabled func(context.Context, string) bool
		want    int
	}{
		{"crafting on", func(context.Context, string) bool { return true }, 1},
		{"crafting off", func(context.Context, string) bool { return false }, 0},
		{"no gate", nil, 1},
	} {
		svc := &fakeCraftingRespites{}
		if err := (&craftingRespiteListener{svc: svc, enabled: c.enabled}).RespitePressed(ctx, "camp", by); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(svc.heard) != c.want {
			t.Errorf("%s: heard %d", c.name, len(svc.heard))
		}
		if c.want == 1 && svc.heard[0] != (crafting.Viewer{UserID: "dm", Director: true, VisibilityRole: 3}) {
			t.Errorf("%s: as %+v", c.name, svc.heard[0])
		}
	}
	// With Homebrew degraded a respite from the Crafting app still logs.
	if err := (&craftingRespiteHook{}).RespiteLogged(ctx, "camp", crafting.Viewer{UserID: "dm", Director: true}); err != nil {
		t.Errorf("no Homebrew: %v", err)
	}
}
