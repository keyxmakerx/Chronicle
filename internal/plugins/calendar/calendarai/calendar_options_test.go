package calendarai

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeTierStore reads and records the campaign's importance levels.
type fakeTierStore struct {
	defs []campaigns.TierDefinition
	set  [][]campaigns.TierDefinition
}

func (f *fakeTierStore) GetEventTierDefinitions(context.Context, string) ([]campaigns.TierDefinition, error) {
	return f.defs, nil
}
func (f *fakeTierStore) SetEventTierDefinitions(_ context.Context, _ string, d []campaigns.TierDefinition) error {
	f.set = append(f.set, d)
	return nil
}

func tierStore() *fakeTierStore {
	return &fakeTierStore{defs: []campaigns.TierDefinition{
		{Slug: "major", Name: "Major", Color: "#ef4444", Prominence: 100},
		{Slug: "standard", Name: "Standard", Color: "#6366f1", Prominence: 50, IsDefault: true},
	}}
}

func optCal() *calendar.Calendar {
	return &calendar.Calendar{ID: "h", Name: "Harptos", CurrentYear: 1492, CurrentMonth: 1, CurrentDay: 15,
		Months: []calendar.Month{{Name: "Hammer", Days: 30}, {Name: "Ches", Days: 20}}}
}

func tierEntries(extra ...map[string]any) []any {
	out := []any{
		map[string]any{"slug": "major", "name": "Major", "color": "#ef4444", "prominence": 100},
		map[string]any{"slug": "standard", "name": "Standard", "color": "#6366f1", "prominence": 50, "default": true},
	}
	for _, e := range extra {
		out = append(out, e)
	}
	return out
}

func TestCalendarKind_PartialDate(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		fields  map[string]any
		want    []int
		wantErr string
	}{
		{"day only", ActionUpdate, map[string]any{"current_day": 5}, []int{1492, 1, 5}, ""},
		{"month only", ActionUpdate, map[string]any{"current_month": "Ches"}, []int{1492, 2, 15}, ""},
		{"year only", ActionUpdate, map[string]any{"current_year": 1500}, []int{1500, 1, 15}, ""},
		{"impossible result", ActionUpdate, map[string]any{"current_month": 2, "current_day": 25}, nil, "Ches has no day 25"},
		{"impossible from month alone", ActionUpdate, map[string]any{"current_day": 31}, nil, "Hammer has no day 31"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeStructure{cal: optCal()}
			k := CalendarKind{Svc: f}
			r := rec(KindCalendar, tt.action, "Harptos", tt.fields, "")
			p := k.Plan(context.Background(), camp, owner, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("error %q, want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" {
				t.Fatal(p.Error)
			}
			if err := k.Apply(context.Background(), camp, owner, r); err != nil {
				t.Fatal(err)
			}
			if len(f.today) != 3 || f.today[0] != tt.want[0] || f.today[1] != tt.want[1] || f.today[2] != tt.want[2] {
				t.Errorf("today %v, want %v", f.today, tt.want)
			}
		})
	}
	t.Run("create with a partial date", func(t *testing.T) {
		fields := newCalendarFields()
		delete(fields, "current_month")
		p := CalendarKind{Svc: &fakeStructure{}}.Plan(context.Background(), camp, owner, rec(KindCalendar, ActionCreate, "Harptos", fields, ""))
		if !strings.Contains(p.Error, "all needed") {
			t.Errorf("error %q", p.Error)
		}
	})
	t.Run("real-world calendar still refused", func(t *testing.T) {
		cal := optCal()
		cal.Mode, cal.TracksRealTime = calendar.ModeRealLife, true
		p := CalendarKind{Svc: &fakeStructure{cal: cal}}.Plan(context.Background(), camp, owner, rec(KindCalendar, ActionUpdate, "Harptos", map[string]any{"current_day": 3}, ""))
		if !strings.Contains(p.Error, "real-world") {
			t.Errorf("error %q", p.Error)
		}
	})
}

func TestCalendarKind_UnknownKeys(t *testing.T) {
	k := CalendarKind{Svc: &fakeStructure{cal: optCal()}}
	p := k.Plan(context.Background(), camp, owner, rec(KindCalendar, ActionUpdate, "Harptos", map[string]any{"year_label": "DR", "zodiac": "x", "weather": 1}, ""))
	if p.Error != "" {
		t.Fatal(p.Error)
	}
	want := []string{"`weather` is not a key of the calendar block, so it was ignored", "`zodiac` is not a key of the calendar block, so it was ignored"}
	if strings.Join(p.Warnings, "|") != strings.Join(want, "|") {
		t.Errorf("warnings %q, want %q", p.Warnings, want)
	}
	// Known keys, including the new ones, draw no warning.
	p = k.Plan(context.Background(), camp, owner, rec(KindCalendar, ActionUpdate, "Harptos", map[string]any{"reason": "why", "hemisphere": "south"}, ""))
	if len(p.Warnings) != 0 {
		t.Errorf("warnings %q", p.Warnings)
	}
}

func TestEventKind_UnknownKeys(t *testing.T) {
	k := EventKind{Svc: extrasCal()}
	p := k.Plan(context.Background(), camp, owner, rec("event", ActionCreate, "Fair", map[string]any{"year": 1492, "month": 1, "day": 3, "weather": "rain"}, ""))
	if p.Error != "" {
		t.Fatal(p.Error)
	}
	if len(p.Warnings) != 1 || p.Warnings[0] != "`weather` is not a key of the event block, so it was ignored" {
		t.Errorf("warnings %q", p.Warnings)
	}
}

// Every key a kind's Doc names is one the kind's key set knows, so a new Doc
// line can't ship a key that then draws an "ignored" warning.
func TestKnownKeysCoverDocs(t *testing.T) {
	for _, c := range []struct {
		doc   string
		known map[string]bool
	}{{CalendarKind{}.Doc(), calendarKeys}, {EventKind{}.Doc(), eventKeys}} {
		for _, line := range strings.Split(c.doc, "\n") {
			if !strings.HasPrefix(line, "- `") {
				continue
			}
			key := strings.SplitN(strings.SplitN(strings.TrimPrefix(line, "- `"), "`", 2)[0], ":", 2)[0]
			if !c.known[key] {
				t.Errorf("Doc key %q is missing from the known keys", key)
			}
		}
	}
	for _, key := range []string{"end_hour", "end_minute", "announced"} {
		if !strings.Contains(EventKind{}.Doc(), "`"+key+"`") || !eventKeys[key] {
			t.Errorf("event key %q not documented or known", key)
		}
	}
}

func TestCalendarKind_Options(t *testing.T) {
	south := "south"
	tests := []struct {
		name    string
		mut     func(c *calendar.Calendar)
		fields  map[string]any
		summary string
		check   func(t *testing.T, f *fakeStructure)
		wantErr string
	}{
		{"hemisphere", nil, map[string]any{"hemisphere": "South"}, "sets the hemisphere to south", func(t *testing.T, f *fakeStructure) {
			if v, _ := f.updates[0].Hemisphere.Get(); v != "south" {
				t.Errorf("updates %+v", f.updates)
			}
		}, ""},
		{"hemisphere cleared", func(c *calendar.Calendar) { c.Hemisphere = &south }, map[string]any{"hemisphere": "none"}, "clears the hemisphere", func(t *testing.T, f *fakeStructure) {
			if !f.updates[0].Hemisphere.IsNull() {
				t.Errorf("updates %+v", f.updates)
			}
		}, ""},
		{"hemisphere unknown", nil, map[string]any{"hemisphere": "east"}, "", nil, "hemisphere must be"},
		{"forecasts on", nil, map[string]any{"forecasts": true}, "turns weather forecasts on", func(t *testing.T, f *fakeStructure) {
			if v, _ := f.updates[0].ForecastsEnabled.Get(); !v {
				t.Errorf("updates %+v", f.updates)
			}
		}, ""},
		{"forecasts typo", nil, map[string]any{"forecasts": "maybe"}, "", nil, "forecasts must be true or false"},
		{"forecasts already on is no change", func(c *calendar.Calendar) { c.ForecastsEnabled = true }, map[string]any{"forecasts": true}, "", nil, "changes nothing"},
		{"new week", nil, map[string]any{"month_starts_new_week": "yes"}, "makes every month start a new week", func(t *testing.T, f *fakeStructure) {
			if v, _ := f.updates[0].MonthStartsNewWeek.Get(); !v {
				t.Errorf("updates %+v", f.updates)
			}
		}, ""},
		{"new week on a real-world calendar", func(c *calendar.Calendar) { c.Mode, c.TracksRealTime = calendar.ModeRealLife, true }, map[string]any{"month_starts_new_week": true}, "", nil, "real-world date"},
		{"era look feel", nil, map[string]any{"era_look": map[string]any{"feel": "lively"}}, "changes how eras show", func(t *testing.T, f *fakeStructure) {
			if l := f.looks[0]; l.Feel != "lively" || l.Intensity != 1.7 || l.Speed != 1.9 || !l.ColorsOn {
				t.Errorf("look %+v", l)
			}
		}, ""},
		{"era look fine-tuned", nil, map[string]any{"era_look": map[string]any{"colors_on": false, "intensity": 2}}, "changes how eras show", func(t *testing.T, f *fakeStructure) {
			if l := f.looks[0]; l.Feel != "custom" || l.Intensity != 2 || l.Speed != 1 || l.ColorsOn {
				t.Errorf("look %+v", l)
			}
		}, ""},
		{"era look out of range", nil, map[string]any{"era_look": map[string]any{"intensity": 9}}, "", nil, "intensity must be between"},
		{"era look unchanged", nil, map[string]any{"era_look": map[string]any{"feel": "subtle", "intensity": 1, "speed": 1, "colors_on": true}}, "", nil, "changes nothing"},
		{"era look bad key", nil, map[string]any{"era_look": map[string]any{"tint": 1}}, "", nil, `"tint" is not a key`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cal := optCal()
			cal.EraLook = calendar.DefaultEraLook()
			if tt.mut != nil {
				tt.mut(cal)
			}
			f := &fakeStructure{cal: cal}
			k := CalendarKind{Svc: f}
			r := rec(KindCalendar, ActionUpdate, "Harptos", tt.fields, "")
			p := k.Plan(context.Background(), camp, owner, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("error %q, want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tt.summary) {
				t.Fatalf("plan %+v, want summary with %q", p, tt.summary)
			}
			if err := k.Apply(context.Background(), camp, owner, r); err != nil {
				t.Fatal(err)
			}
			tt.check(t, f)
		})
	}
}

func TestCalendarKind_OptionsRefusedOnCreate(t *testing.T) {
	for _, key := range []string{"hemisphere", "forecasts", "month_starts_new_week", "era_look", "tiers"} {
		fields := newCalendarFields()
		switch key {
		case "era_look":
			fields[key] = map[string]any{"feel": "still"}
		case "tiers":
			fields[key] = tierEntries()
		case "hemisphere":
			fields[key] = "north"
		default:
			fields[key] = true
		}
		p := CalendarKind{Svc: &fakeStructure{}, Tiers: tierStore()}.Plan(context.Background(), camp, owner, rec(KindCalendar, ActionCreate, "Harptos", fields, ""))
		if !strings.Contains(p.Error, "only apply to action: update") {
			t.Errorf("%s on create: %q", key, p.Error)
		}
	}
}

func TestCalendarKind_Tiers(t *testing.T) {
	added := map[string]any{"slug": "side-quest", "name": "Side quest", "color": "#00ff00", "prominence": 20}
	tests := []struct {
		name    string
		tiers   any
		actor   Actor
		noSvc   bool
		summary string
		wantErr string
	}{
		{"adds a level", tierEntries(added), owner, false, "replaces the event importance levels (3 levels)", ""},
		{"unchanged plans as no change", tierEntries(), owner, false, "", "changes nothing"},
		{"two defaults", []any{
			map[string]any{"slug": "a", "name": "A", "color": "#000000", "prominence": 1, "default": true},
			map[string]any{"slug": "b", "name": "B", "color": "#000000", "prominence": 1, "default": true}}, owner, false, "", "exactly one tier must be marked is_default"},
		{"bad colour", []any{map[string]any{"slug": "a", "name": "A", "color": "red", "prominence": 1, "default": true}}, owner, false, "", "must be #RRGGBB"},
		{"bad slug", []any{map[string]any{"slug": "Big One", "name": "A", "color": "#000000", "prominence": 1, "default": true}}, owner, false, "", "slug"},
		{"prominence out of range", []any{map[string]any{"slug": "a", "name": "A", "color": "#000000", "prominence": 101, "default": true}}, owner, false, "", "prominence 101 must be 0-100"},
		{"needs prominence", []any{map[string]any{"slug": "a", "name": "A", "color": "#000000", "default": true}}, owner, false, "", "needs prominence"},
		{"empty list", []any{}, owner, false, "", "at least one tier"},
		{"not the owner", tierEntries(added), Actor{UserID: "co", Role: 2}, false, "", "only the campaign owner"},
		{"service not wired", tierEntries(added), owner, true, "", "tiers can't be set here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := tierStore()
			k := CalendarKind{Svc: &fakeStructure{cal: optCal()}, Tiers: store}
			if tt.noSvc {
				k.Tiers = nil
			}
			r := rec(KindCalendar, ActionUpdate, "Harptos", map[string]any{"tiers": tt.tiers}, "")
			p := k.Plan(context.Background(), camp, tt.actor, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("error %q, want %q", p.Error, tt.wantErr)
				}
				if len(store.set) != 0 {
					t.Error("a refused block wrote levels")
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tt.summary) {
				t.Fatalf("plan %+v", p)
			}
			if err := k.Apply(context.Background(), camp, tt.actor, r); err != nil {
				t.Fatal(err)
			}
			if len(store.set) != 1 || len(store.set[0]) != 3 || store.set[0][2].Slug != "side-quest" {
				t.Errorf("written %+v", store.set)
			}
		})
	}
}

// The lookup and the export write the levels in the shape `tiers:` reads, so
// the block sent back unchanged plans as no change.
func TestCalendarKind_TiersRoundTrip(t *testing.T) {
	store := tierStore()
	block := tiersBlock(store.defs)
	for _, want := range []string{`slug: "major"`, `color: "#ef4444"`, "prominence: 100", "default: true"} {
		if !strings.Contains(block, want) {
			t.Errorf("block lacks %q:\n%s", want, block)
		}
	}
	l := &Lookups{Cal: fakeStructureCal{&fakeStructure{cal: optCal()}, newCal()}, Tiers: store}
	ans := l.Answer(context.Background(), camp, owner, []Record{lookup(WhatCalendar, nil)})[0]
	if ans.Error != "" || !strings.Contains(ans.Text, "send the whole list as `tiers:`") || !strings.Contains(ans.Text, block) {
		t.Fatalf("lookup %q:\n%s", ans.Error, ans.Text)
	}
	if !strings.Contains(ans.Text, "forecasts: false") || !strings.Contains(ans.Text, "era_look: {") {
		t.Errorf("settings missing:\n%s", ans.Text)
	}
	// A player sees neither the levels nor the settings.
	pl := l.Answer(context.Background(), camp, Actor{UserID: "p", Role: 1}, []Record{lookup(WhatCalendar, nil)})[0]
	if strings.Contains(pl.Text, "tiers:") || strings.Contains(pl.Text, "era_look") {
		t.Errorf("a player was shown settings:\n%s", pl.Text)
	}
}
