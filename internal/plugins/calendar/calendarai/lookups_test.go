package calendarai

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

func TestParseDateText(t *testing.T) {
	cal := &newCal().cal
	tests := []struct {
		in      string
		want    calDate
		wantErr bool
	}{
		{"Hammer 21 1492", calDate{1492, 1, 21}, false},
		{"21 Hammer 1492", calDate{1492, 1, 21}, false},
		{"Alturiak 3, 1492", calDate{1492, 2, 3}, false},
		{"1492-2-3", calDate{1492, 2, 3}, false},
		{"Hammer 31 1492", calDate{}, true},
		{"Hammer 1492", calDate{}, true},
		{"next Tuesday", calDate{}, true},
	}
	for _, tt := range tests {
		got, err := parseDateText(cal, tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("%q: got %v, %v", tt.in, got, err)
		}
	}
}

// An event to change is found through the role-filtered list: the
// operator finds what they can see, and a player never finds a hidden one.
func TestEventKind_FindsEventToChange(t *testing.T) {
	c := newCal()
	c.stored = []calendar.Event{
		{ID: "e1", Name: "Midwinter Feast", Year: 1492, Month: 1, Day: 21},
		{ID: "e2", Name: "Secret Council", Year: 1492, Month: 1, Day: 12, Visibility: "dm_only"},
	}
	k := EventKind{Svc: c}
	if p := k.Plan(context.Background(), camp, owner, rec("event", ActionUpdate, "Secret Council", nil, "")); p.Error != "" {
		t.Fatalf("owner: %s", p.Error)
	}
	if p := k.Plan(context.Background(), camp, player, rec("event", ActionDelete, "Secret Council", nil, "")); p.Error == "" {
		t.Fatal("a player found a hidden event to remove")
	}
}

func TestLookups_CalendarCategoriesAndImportance(t *testing.T) {
	tiers := fakeTiers{{Slug: "major", Name: "Major"}, {Slug: "standard", Name: "Standard", IsDefault: true}, {Slug: "side-quest", Name: "Side quest"}}
	l := calendarWorld()
	l.Tiers = tiers
	l.Cal.(*fakeCal).cal.EventKinds = []calendar.EventKind{{Name: "Festival", Slug: "festival"}}

	ans := l.Answer(context.Background(), camp, owner, []Record{lookup(WhatCalendar, nil)})[0]
	if ans.Error != "" {
		t.Fatal(ans.Error)
	}
	for _, want := range []string{
		"### Event categories (the names `category:` takes on events)\n\nFestival",
		"### Event importance levels (the names `importance:` takes on events)",
		"- Major\n", "- Standard, the usual level", "- Side quest (`side-quest`)",
	} {
		if !strings.Contains(ans.Text, want) {
			t.Errorf("answer lacks %q:\n%s", want, ans.Text)
		}
	}
	if strings.Contains(ans.Text, "Event kinds") {
		t.Error("the old heading is still there")
	}

	// Without tiers wired the rest of the answer stands.
	l.Tiers = nil
	ans = l.Answer(context.Background(), camp, owner, []Record{lookup(WhatCalendar, nil)})[0]
	if ans.Error != "" || strings.Contains(ans.Text, "importance") {
		t.Errorf("unwired tiers: %q %q", ans.Error, ans.Text)
	}

	// A player is not shown the levels.
	l.Tiers = tiers
	ans = l.Answer(context.Background(), camp, player, []Record{lookup(WhatCalendar, nil)})[0]
	if strings.Contains(ans.Text, "importance") {
		t.Error("a player was shown the importance levels")
	}
}

func eventsWorld() *Lookups {
	c := newCal()
	c.stored = []calendar.Event{
		{ID: "e1", Name: "Midwinter Feast", Year: 1492, Month: 1, Day: 21, Visibility: "everyone"},
		{ID: "e2", Name: "Secret Council", Year: 1492, Month: 1, Day: 12, Visibility: "dm_only"},
		{ID: "e3", Name: "Spring Fair", Year: 1492, Month: 2, Day: 3, Visibility: "everyone"},
	}
	label := "Blizzard"
	temp := -12.0
	c.days = []calendar.DayWeather{{Year: 1492, Month: 1, Day: 3, PresetLabel: &label, TemperatureCelsius: &temp}}
	return &Lookups{Cal: c}
}

func TestLookups_EventsAndWeather(t *testing.T) {
	tests := []struct {
		name        string
		r           Record
		a           Actor
		want        []string
		notWant     []string
		wantErrText string
	}{
		{"events in a month", lookup("events", map[string]any{"from": "Hammer 1 1492", "to": "Hammer 30 1492"}), owner,
			[]string{"Midwinter Feast: Hammer 21 1492, everyone", "Secret Council", "Directors only"}, []string{"Spring Fair"}, ""},
		{"events hide Director-only in Safe", lookup("events", map[string]any{"year": 1492, "month": "Hammer"}), player,
			[]string{"Midwinter Feast"}, []string{"Secret Council"}, ""},
		{"events across months", lookup("events", map[string]any{"from": "1492-1-20", "to": "Alturiak 5 1492"}), owner,
			[]string{"Midwinter Feast", "Spring Fair"}, []string{"Secret Council"}, ""},
		{"events need a range", lookup("events", nil), owner, nil, nil, "it needs `from` and `to`"},
		{"events refuse a bad month", lookup("events", map[string]any{"from": "Smarch 1 1492"}), owner, nil, nil, "not a month"},
		{"weather", lookup("weather", map[string]any{"year": 1492, "month": 1}), owner, []string{"Hammer 3 1492: Blizzard, -12°C"}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eventsWorld().Answer(context.Background(), camp, tt.a, []Record{tt.r})[0]
			if tt.wantErrText != "" {
				if !strings.Contains(got.Error, tt.wantErrText) {
					t.Fatalf("error = %q, want %q", got.Error, tt.wantErrText)
				}
				return
			}
			if got.Error != "" {
				t.Fatalf("unexpected error %q", got.Error)
			}
			for _, w := range tt.want {
				if !strings.Contains(got.Text, w) {
					t.Errorf("answer lacks %q:\n%s", w, got.Text)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(got.Text, w) {
					t.Errorf("answer leaks %q:\n%s", w, got.Text)
				}
			}
		})
	}
}

// The lookups keep the `what:`, tools and order they had inside the engine,
// and only the calendar lookup gives the prompt the world's date.
func TestLookups_Contract(t *testing.T) {
	l := calendarWorld()
	var whats, tools []string
	for _, p := range l.All() {
		in := p.LookupInfo()
		whats = append(whats, in.What)
		tools = append(tools, in.Tool.Name)
		if in.Brief == "" || in.Keys == "" || in.Tool.Desc == "" || in.Tool.Owner {
			t.Errorf("%s: incomplete or owner-only: %+v", in.What, in)
		}
	}
	if got := strings.Join(whats, " "); got != "calendar weather-kinds events weather" {
		t.Errorf("whats %q", got)
	}
	if got := strings.Join(tools, " "); got != "calendar_overview weather_kinds calendar_events weather" {
		t.Errorf("tools %q", got)
	}
	var dated int
	for _, p := range l.All() {
		if d, ok := p.(aiplug.Dated); ok && d.WorldToday(context.Background(), camp, owner) != "" {
			dated++
		}
	}
	if dated != 1 {
		t.Errorf("%d lookups give the world's date, want 1", dated)
	}
	if got := (&Lookups{}).WorldToday(context.Background(), camp, owner); got != "" {
		t.Errorf("no calendar service, date %q", got)
	}
	if ans := (&Lookups{}).Answer(context.Background(), camp, owner, []Record{lookup("events", map[string]any{"year": 1})})[0]; !strings.Contains(ans.Error, "on this server") {
		t.Errorf("unwired events lookup answered %+v", ans)
	}
}

// The engine names the calendar block by the calendar's slug.
func TestKindCalendarIsTheSlug(t *testing.T) {
	if KindCalendar != calendar.PluginSlug || WhatCalendar != calendar.PluginSlug {
		t.Errorf("kind %q, lookup %q, slug %q", KindCalendar, WhatCalendar, calendar.PluginSlug)
	}
}
