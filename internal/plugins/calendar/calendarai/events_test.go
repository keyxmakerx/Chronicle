package calendarai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

type fakeCal struct {
	cal     calendar.Calendar
	weather []calendar.DayWeatherInput
	events  []calendar.CreateEventInput
	stored  []calendar.Event      // what the list reads return
	days    []calendar.DayWeather // what ListDayWeather returns
	updates []calendar.UpdateEventInput
	// failNames makes CreateEvent refuse those event names.
	failNames map[string]bool
}

func (f *fakeCal) GetDefaultCalendarForViewer(context.Context, string, permissions.Viewer) (*calendar.Calendar, error) {
	return &f.cal, nil
}

// ListEventsForCalendar drops Director-only events below a co-DM, as the
// real service's role filter does.
func (f *fakeCal) ListEventsForCalendar(_ context.Context, _, _ string, role int) ([]calendar.Event, error) {
	var out []calendar.Event
	for _, e := range f.stored {
		if e.Visibility != "dm_only" || role >= 3 {
			out = append(out, e)
		}
	}
	return out, nil
}

// ListEventsForMonth drops Director-only events for a player, as the real
// service's role filter does.
func (f *fakeCal) ListEventsForMonth(_ context.Context, _, _ string, y, m int, v permissions.Viewer) ([]calendar.Event, error) {
	var out []calendar.Event
	for _, e := range f.stored {
		if e.Year == y && e.Month == m && (e.Visibility != "dm_only" || v.SkipsPerUserRules()) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeCal) CreateEvent(_ context.Context, _, _ string, in calendar.CreateEventInput) (*calendar.Event, error) {
	if f.failNames[in.Name] {
		return nil, errors.New("refused")
	}
	f.events = append(f.events, in)
	f.stored = append(f.stored, calendar.Event{Name: in.Name, Year: in.Year, Month: in.Month, Day: in.Day})
	return &calendar.Event{}, nil
}

func (f *fakeCal) UpdateEvent(_ context.Context, _, _, _ string, in calendar.UpdateEventInput, _ permissions.Viewer) error {
	f.updates = append(f.updates, in)
	return nil
}

func (f *fakeCal) DeleteEvent(context.Context, string, string, string, permissions.Viewer) error {
	return nil
}

func (f *fakeCal) ListDayWeather(_ context.Context, _, _ string, y, m int, _ permissions.Viewer) ([]calendar.DayWeather, error) {
	var out []calendar.DayWeather
	for _, d := range f.days {
		if d.Year == y && d.Month == m {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeCal) SetDayWeather(_ context.Context, _, _ string, days []calendar.DayWeatherInput) error {
	f.weather = append(f.weather, days...)
	return nil
}

func (f *fakeCal) ClearDayWeather(context.Context, string, string, []calendar.DayDate) error {
	return nil
}

func newCal() *fakeCal {
	return &fakeCal{cal: calendar.Calendar{ID: "cal", Name: "Harptos", Months: []calendar.Month{{Name: "Hammer", Days: 30}, {Name: "Alturiak", Days: 30}}}}
}

func TestGeneratorKind_Weather(t *testing.T) {
	fields := map[string]any{"generator": "weather", "year": 1492, "month": "Hammer", "day": 1, "end_year": 1492, "end_month": 1, "end_day": 3}
	tests := []struct {
		name, out string
		wantErr   bool
		saved     int
	}{
		{"in range", `[{"year":1492,"month":1,"day":2,"preset_label":"Rain"}]`, false, 1},
		{"day outside the plan", `[{"year":1492,"month":2,"day":2,"preset_label":"Rain"}]`, true, 0},
		{"not run", ``, true, 0},
		{"garbage", `{"x":1}`, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCal()
			k := GeneratorKind{Cal: c}
			r := rec("generator", ActionCreate, "Spring", fields, "")
			if p := k.Plan(context.Background(), camp, owner, r); p.Error != "" || p.Client == "" {
				t.Fatalf("plan: %+v", p)
			}
			r.Generated = tt.out
			err := k.Apply(context.Background(), camp, owner, r)
			if (err != nil) != tt.wantErr || len(c.weather) != tt.saved {
				t.Fatalf("err %v saved %d", err, len(c.weather))
			}
			for _, d := range c.weather {
				if d.Source != calendar.WeatherSourceGenerated {
					t.Fatal("generated weather saved as manual: it would overwrite hand-set days")
				}
			}
		})
	}
}

func TestGeneratorKind_PlayersCannotRunWeather(t *testing.T) {
	k := GeneratorKind{Cal: newCal()}
	p := k.Plan(context.Background(), camp, player, rec("generator", ActionCreate, "", map[string]any{"generator": "weather", "year": 1, "month": 1, "day": 1}, ""))
	if p.Error == "" {
		t.Fatal("player planned weather")
	}
}

func TestGeneratorKind_Names(t *testing.T) {
	ft := &fakeTables{}
	k := GeneratorKind{Cal: newCal(), Tables: ft}
	r := rec("generator", ActionCreate, "", map[string]any{"generator": "names", "count": 3, "table": "Villagers"}, "")
	var plan genPlan
	p := k.Plan(context.Background(), camp, owner, r)
	if err := json.Unmarshal([]byte(p.Client), &plan); err != nil || plan.NamesKind != "people" {
		t.Fatalf("plan %+v %v", p, err)
	}
	r.Generated = `["Ada"," ","Bram","Cole"]`
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ft.added["Villagers"], ","); got != "Ada,Bram,Cole" {
		t.Fatalf("names added %q", got)
	}
	r.Generated = `[` + strings.Repeat(`"x",`, 40) + `"y"]`
	if err := k.Apply(context.Background(), camp, owner, r); err == nil {
		t.Fatal("accepted more names than the plan allows")
	}
}

func TestEventKind_DatesAndHidden(t *testing.T) {
	c := newCal()
	k := EventKind{Svc: c}
	ctx := context.Background()
	if err := k.Apply(ctx, camp, owner, rec("event", ActionCreate, "Feast", map[string]any{"year": 1492, "month": "alturiak", "day": 30}, "**Big** feast")); err != nil {
		t.Fatal(err)
	}
	if e := c.events[0]; e.Month != 2 || e.DescriptionHTML == nil || !strings.Contains(*e.DescriptionHTML, "<strong>") || !e.CanAuthorDmOnly {
		t.Fatalf("event %+v", e)
	}
	if p := k.Plan(ctx, camp, owner, rec("event", ActionCreate, "Feast", map[string]any{"year": 1492, "month": 1, "day": 31}, "")); p.Error == "" {
		t.Fatal("accepted a day the month does not have")
	}
	if p := k.Plan(ctx, camp, player, rec("event", ActionCreate, "Feast", map[string]any{"year": 1, "month": 1, "day": 1, "visibility": "dm_only"}, "")); p.Error == "" {
		t.Fatal("player added a hidden event")
	}
}

func TestEventKind_StripsScript(t *testing.T) {
	c := newCal()
	k := EventKind{Svc: c}
	if err := k.Apply(context.Background(), camp, owner, rec("event", ActionCreate, "X", map[string]any{"year": 1, "month": 1, "day": 1}, "<script>alert(1)</script>hi")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(*c.events[0].DescriptionHTML, "<script") {
		t.Fatal("script reached the calendar")
	}
}

func TestEventKind_UpdateMoveAndTime(t *testing.T) {
	ctx := context.Background()
	stored := []calendar.Event{{ID: "e1", Name: "Feast", Year: 1492, Month: 1, Day: 5}}
	base := map[string]any{"year": 1492, "month": 1, "day": 5}
	with := func(kv ...any) map[string]any {
		m := map[string]any{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}
	tests := []struct {
		name    string
		action  string
		fields  map[string]any
		wantErr bool
		check   func(t *testing.T, in calendar.UpdateEventInput)
	}{
		{"move", ActionUpdate, with("move_to_year", 1492, "move_to_month", "Alturiak", "move_to_day", 9), false, func(t *testing.T, in calendar.UpdateEventInput) {
			if v, _ := in.Month.Get(); !in.Month.Present() || v != 2 {
				t.Errorf("month not moved: %+v", in.Month)
			}
			if v, _ := in.Day.Get(); v != 9 {
				t.Errorf("day = %v", v)
			}
		}},
		{"minute is saved", ActionUpdate, with("hour", 9, "minute", 30), false, func(t *testing.T, in calendar.UpdateEventInput) {
			if v, _ := in.StartMinute.Get(); !in.StartMinute.Present() || v != 30 {
				t.Errorf("minute = %+v", in.StartMinute)
			}
		}},
		{"matching date alone moves nothing", ActionUpdate, with(), false, func(t *testing.T, in calendar.UpdateEventInput) {
			if in.Day.Present() {
				t.Error("day changed without move_to")
			}
		}},
		{"partial move date", ActionUpdate, with("move_to_year", 1492), true, nil},
		{"move on create", ActionCreate, with("move_to_year", 1492, "move_to_month", 1, "move_to_day", 2), true, nil},
		{"word for hour", ActionUpdate, with("hour", "noon"), true, nil},
		{"word for minute", ActionCreate, with("hour", 3, "minute", "half"), true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCal()
			c.stored = append([]calendar.Event(nil), stored...)
			k := EventKind{Svc: c}
			r := rec("event", tt.action, "Feast", tt.fields, "")
			if tt.action == ActionCreate {
				r.Fields = with("hour", tt.fields["hour"])
				for key, v := range tt.fields {
					r.Fields[key] = v
				}
			}
			err := k.Apply(ctx, camp, owner, r)
			if p := k.Plan(ctx, camp, owner, r); (p.Error != "") != tt.wantErr || (err != nil) != tt.wantErr {
				t.Fatalf("plan %+v err %v, wantErr %v", p, err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(c.updates) != 1 {
					t.Fatalf("updates %d", len(c.updates))
				}
				tt.check(t, c.updates[0])
			}
		})
	}
}

func TestWeatherKind_WarnsOnLockedOrHandSetDays(t *testing.T) {
	yes, no := true, false
	days := []calendar.DayWeather{
		{Year: 1492, Month: 1, Day: 1, Source: calendar.WeatherSourceGenerated, Locked: &yes},
		{Year: 1492, Month: 1, Day: 2, Source: calendar.WeatherSourceManual, Locked: &no},
		{Year: 1492, Month: 1, Day: 3, Source: calendar.WeatherSourceGenerated, Locked: &no},
	}
	tests := []struct {
		day    int
		action string
		want   string
	}{
		{1, ActionCreate, "locked"},
		{1, ActionDelete, "locked"},
		{2, ActionCreate, "by hand"},
		{3, ActionCreate, ""},
		{4, ActionCreate, ""},
	}
	for _, tt := range tests {
		c := newCal()
		c.days = days
		k := WeatherKind{Svc: c}
		r := rec("weather", tt.action, "", map[string]any{"year": 1492, "month": 1, "day": tt.day, "label": "Rain"}, "")
		p := k.Plan(context.Background(), camp, owner, r)
		if p.Error != "" {
			t.Fatalf("day %d: %s", tt.day, p.Error)
		}
		if (tt.want == "") != (len(p.Warnings) == 0) || (tt.want != "" && !strings.Contains(p.Warnings[0], tt.want)) {
			t.Errorf("day %d %s: warnings %v, want %q", tt.day, tt.action, p.Warnings, tt.want)
		}
	}
}

func TestGeneratorKind_WeatherRangeMatchesCap(t *testing.T) {
	// Two 30-day months make a 60-day year: 400 days is 6 years + 40, so
	// the cap is exercised with the day count rather than the year gap.
	cal := newCal()
	cases := []struct {
		name       string
		ey, em, ed int
		wantErr    bool
	}{
		{"two years", 1493, 2, 30, false},
		{"exactly the cap", 1498, 2, 10, false},
		{"one past the cap", 1498, 2, 11, true},
		{"before start", 1491, 1, 1, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			k := GeneratorKind{Cal: cal}
			r := rec("generator", ActionCreate, "", map[string]any{"generator": "weather", "year": 1492, "month": 1, "day": 1, "end_year": tt.ey, "end_month": tt.em, "end_day": tt.ed}, "")
			p := k.Plan(context.Background(), camp, owner, r)
			if (p.Error != "") != tt.wantErr {
				t.Fatalf("plan %+v", p)
			}
		})
	}
}

func TestGeneratorKind_EventsRerunDoesNotDuplicate(t *testing.T) {
	ctx := context.Background()
	c := newCal()
	c.failNames = map[string]bool{"Fair": true}
	k := GeneratorKind{Cal: c}
	r := rec("generator", ActionCreate, "", map[string]any{"generator": "events", "year": 1492}, "")
	r.Generated = `[{"name":"Feast","year":1492,"month":1,"day":2},{"name":"Fair","year":1492,"month":1,"day":9}]`
	err := k.Apply(ctx, camp, owner, r)
	if err == nil || !strings.Contains(err.Error(), "1 were added") {
		t.Fatalf("first run: %v", err)
	}
	c.failNames = nil
	if err := k.Apply(ctx, camp, owner, r); err != nil {
		t.Fatal(err)
	}
	if len(c.events) != 2 {
		t.Fatalf("created %d events across both runs, want 2", len(c.events))
	}
}

// An event's text reaches players without the strip page text gets, so a
// GM-only marker in it is refused, and nothing is written.
func TestEventKind_RefusesGMMarkers(t *testing.T) {
	c := newCal()
	k := EventKind{Svc: c}
	r := rec("event", ActionCreate, "Feast", map[string]any{"year": 1492, "month": 1, "day": 1}, "Feast. {gm{the mayor is the cult leader}gm}")
	err := k.Apply(context.Background(), camp, owner, r)
	var ae *apperror.AppError
	if !errors.As(err, &ae) || !strings.Contains(ae.Message, "DM-only passages only work in page text") || len(c.events) != 0 {
		t.Errorf("apply err = %v, events = %d", err, len(c.events))
	}
}
