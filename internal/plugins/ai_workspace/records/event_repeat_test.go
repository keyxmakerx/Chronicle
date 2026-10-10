package records

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// repeatCal is a calendar with a week, a moon and seasons for rule rows.
func repeatCal() *fakeCal {
	c := newCal()
	c.cal.Weekdays = []calendar.Weekday{{Name: "Sunday"}, {Name: "Moonday"}, {Name: "Kingsday"}}
	c.cal.Moons = []calendar.Moon{{ID: 7, Name: "Selune", CycleDays: 30}}
	c.cal.Seasons = []calendar.Season{{ID: 4, Name: "Winter", StartMonth: 1, StartDay: 1, EndMonth: 1, EndDay: 30}}
	c.stored = []calendar.Event{{ID: "ev-masks", Name: "Festival of Masks", Year: 1492, Month: 1, Day: 5}}
	return c
}

func eventFields(extra map[string]any) map[string]any {
	f := map[string]any{"year": 1492, "month": "Hammer", "day": 21}
	for k, v := range extra {
		f[k] = v
	}
	return f
}

func TestEventKind_RepeatCreate(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		check func(t *testing.T, in calendar.CreateEventInput)
	}{
		{"yearly", map[string]any{"repeat": "yearly"}, func(t *testing.T, in calendar.CreateEventInput) {
			if !in.IsRecurring || in.RecurrenceType == nil || *in.RecurrenceType != calendar.RecurrenceYearly || in.RecurrenceRule != nil {
				t.Fatalf("%+v", in)
			}
		}},
		{"every 4 years until, by name of month", map[string]any{"repeat": "Yearly", "repeat_every": 4, "repeat_until_year": 1520, "repeat_until_month": "Alturiak", "repeat_until_day": 2}, func(t *testing.T, in calendar.CreateEventInput) {
			if *in.RecurrenceInterval != 4 || *in.RecurrenceEndYear != 1520 || *in.RecurrenceEndMonth != 2 || *in.RecurrenceEndDay != 2 {
				t.Fatalf("%+v", in)
			}
		}},
		{"monthly, 6 times", map[string]any{"repeat": "monthly", "repeat_times": 6}, func(t *testing.T, in calendar.CreateEventInput) {
			if *in.RecurrenceMaxOccurrences != 6 || in.RecurrenceEndYear != nil {
				t.Fatalf("%+v", in)
			}
		}},
		{"rule by names", map[string]any{"repeat": "rule", "repeat_every": 2, "repeat_offset_days": -1, "repeat_on": []any{
			map[string]any{"Moon": "selune", "phase": "full"},
			map[string]any{"weekday": "Kingsday", "nth": "last"},
			map[string]any{"weekdays": []any{"Sunday", "Moonday"}},
			map[string]any{"season": "Winter"},
			map[string]any{"season_start": "any"},
			map[string]any{"event": "Festival of Masks", "days": 2},
		}}, func(t *testing.T, in calendar.CreateEventInput) {
			if *in.RecurrenceType != calendar.RecurrenceByRule || in.RecurrenceInterval != nil {
				t.Fatalf("%+v", in)
			}
			r, err := calendar.ParseRecurrenceRule(in.RecurrenceRule)
			if err != nil {
				t.Fatal(err)
			}
			if r.Every != 2 || r.OffsetDays != -1 || len(r.Match) != 6 {
				t.Fatalf("rule %+v", r)
			}
			m := r.Match
			if m[0].Kind != calendar.RuleMoonPhase || m[0].MoonID != 7 || m[0].Phase != "full" {
				t.Fatalf("moon %+v", m[0])
			}
			if m[1].Kind != calendar.RuleNthWeekday || m[1].N != -1 || *m[1].Weekday != 2 {
				t.Fatalf("nth %+v", m[1])
			}
			if m[2].Kind != calendar.RuleWeekday || len(m[2].Weekdays) != 2 || m[2].Weekdays[1] != 1 {
				t.Fatalf("weekdays %+v", m[2])
			}
			if m[3].SeasonID != 4 || m[4].Kind != calendar.RuleSeasonStart || m[4].SeasonID != 0 {
				t.Fatalf("seasons %+v %+v", m[3], m[4])
			}
			if m[5].Kind != calendar.RuleAfterEvent || m[5].EventID != "ev-masks" || *m[5].Days != 2 {
				t.Fatalf("event %+v", m[5])
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := repeatCal()
			k := EventKind{Svc: c}
			r := rec("event", ActionCreate, "Feast", eventFields(tt.extra), "")
			if p := k.Plan(context.Background(), camp, owner, r); p.Error != "" || !strings.Contains(p.Summary, "repeats") {
				t.Fatalf("plan %+v", p)
			}
			if err := k.Apply(context.Background(), camp, owner, r); err != nil {
				t.Fatal(err)
			}
			tt.check(t, c.events[len(c.events)-1])
		})
	}
}

func TestEventKind_RepeatRefusals(t *testing.T) {
	tests := []struct {
		name, action, want string
		extra              map[string]any
	}{
		{"unknown type", ActionCreate, "not one of", map[string]any{"repeat": "fortnightly"}},
		{"keys without repeat", ActionCreate, "needs repeat", map[string]any{"repeat_times": 3}},
		{"none on create", ActionCreate, "only applies to action: update", map[string]any{"repeat": "none"}},
		{"zero step", ActionCreate, "1 or more", map[string]any{"repeat": "yearly", "repeat_every": 0}},
		{"rule keys on yearly", ActionCreate, "only goes with repeat: rule", map[string]any{"repeat": "yearly", "repeat_on": []any{map[string]any{"day": 1}}}},
		{"rule without conditions", ActionCreate, "needs repeat_on", map[string]any{"repeat": "rule"}},
		{"unknown moon lists the real ones", ActionCreate, "moons: Selune", map[string]any{"repeat": "rule", "repeat_on": []any{map[string]any{"moon": "Luna", "phase": "full"}}}},
		{"bad phase", ActionCreate, "phase", map[string]any{"repeat": "rule", "repeat_on": []any{map[string]any{"moon": "Selune", "phase": "gibbous"}}}},
		{"two kinds in one item", ActionCreate, "own repeat_on item", map[string]any{"repeat": "rule", "repeat_on": []any{map[string]any{"month": "Hammer", "day": 1}}}},
		{"unknown key", ActionCreate, "not a repeat_on key", map[string]any{"repeat": "rule", "repeat_on": []any{map[string]any{"tide": "high"}}}},
		{"missing event", ActionCreate, "no event called", map[string]any{"repeat": "rule", "repeat_on": []any{map[string]any{"event": "Greengrass"}}}},
		{"plain string item", ActionCreate, "must be keys", map[string]any{"repeat": "rule", "repeat_on": []any{"full moon"}}},
		{"step on weekly", ActionCreate, "use repeat: custom", map[string]any{"repeat": "weekly", "repeat_every": 3}},
		{"custom without step", ActionCreate, "needs repeat_every", map[string]any{"repeat": "custom"}},
		{"nth without weekday", ActionCreate, "nth needs weekday", map[string]any{"repeat": "rule", "repeat_on": []any{map[string]any{"nth": 2}}}},
		{"days without event", ActionCreate, "days needs event", map[string]any{"repeat": "rule", "repeat_on": []any{map[string]any{"days": 2}}}},
		{"on delete", ActionDelete, "action: delete", map[string]any{"repeat": "yearly"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := repeatCal()
			c.stored = append(c.stored, calendar.Event{ID: "ev-feast", Name: "Feast", Year: 1492, Month: 1, Day: 21})
			p := EventKind{Svc: c}.Plan(context.Background(), camp, owner, rec("event", tt.action, "Feast", eventFields(tt.extra), ""))
			if !strings.Contains(p.Error, tt.want) {
				t.Fatalf("error %q, want %q", p.Error, tt.want)
			}
		})
	}
}

func TestEventKind_RepeatUpdate(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		check func(t *testing.T, in calendar.UpdateEventInput)
	}{
		{"untouched without repeat keys", map[string]any{"color": "#123456"}, func(t *testing.T, in calendar.UpdateEventInput) {
			if in.IsRecurring.Present() || in.RecurrenceType.Present() || in.RecurrenceEndYear.Present() {
				t.Fatalf("repeat changed by an unrelated edit: %+v", in)
			}
		}},
		{"none stops it", map[string]any{"repeat": "none"}, func(t *testing.T, in calendar.UpdateEventInput) {
			if v, _ := in.IsRecurring.Get(); v || !in.RecurrenceType.IsNull() {
				t.Fatalf("%+v", in)
			}
		}},
		{"a new repeat clears the old end", map[string]any{"repeat": "yearly"}, func(t *testing.T, in calendar.UpdateEventInput) {
			if v, _ := in.RecurrenceType.Get(); v != calendar.RecurrenceYearly {
				t.Fatalf("%+v", in)
			}
			if !in.RecurrenceEndYear.IsNull() || !in.RecurrenceMaxOccurrences.IsNull() || !in.RecurrenceInterval.IsNull() {
				t.Fatalf("old end kept: %+v", in)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := repeatCal()
			c.stored = append(c.stored, calendar.Event{ID: "ev-feast", Name: "Feast", Year: 1492, Month: 1, Day: 21})
			k := EventKind{Svc: c}
			r := rec("event", ActionUpdate, "Feast", eventFields(tt.extra), "")
			if p := k.Plan(context.Background(), camp, owner, r); p.Error != "" {
				t.Fatalf("plan %+v", p)
			}
			if err := k.Apply(context.Background(), camp, owner, r); err != nil {
				t.Fatal(err)
			}
			tt.check(t, c.updates[0])
		})
	}
}

// The Doc lists the calendar's own repeat types, so a type the calendar
// adds reaches the prompt without a change here.
func TestEventKind_DocListsRepeatTypes(t *testing.T) {
	doc := EventKind{}.Doc()
	for _, typ := range calendar.RecurrenceTypes {
		if !strings.Contains(doc, typ) {
			t.Fatalf("Doc misses repeat type %q", typ)
		}
	}
}

func TestGeneratorKind_WeatherUsesCalendarSettings(t *testing.T) {
	base := map[string]any{"generator": "weather", "year": 1492, "month": 1, "day": 1}
	kinds := []calendar.WeatherKind{{ID: "ashfall", Name: "Ashfall", Like: "snow"}}
	settings := fakeWeatherSettings{calendar.WeatherSettings{Climate: "desert", Continuity: 0.8, Kinds: kinds}}
	tests := []struct {
		name     string
		weather  WeatherSettingsAPI
		extra    map[string]any
		climate  any
		cont     any
		kinds    int
		summary  string
		errorHas string
	}{
		{"calendar's settings", settings, nil, "desert", 0.8, 1, "Desert climate (the calendar's own)", ""},
		{"row overrides", settings, map[string]any{"climate": "Tundra", "continuity": 0.2, "warmer": -5}, "tundra", 0.2, 1, "Tundra climate,", ""},
		{"no settings reader", nil, nil, nil, nil, 0, "", ""},
		{"unknown climate lists the real ones", settings, map[string]any{"climate": "swamp"}, nil, nil, 0, "", "temperate"},
		{"out of bounds", settings, map[string]any{"wetter": 3}, nil, nil, 0, "", "wetter must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := map[string]any{}
			for k, v := range base {
				f[k] = v
			}
			for k, v := range tt.extra {
				f[k] = v
			}
			k := GeneratorKind{Cal: newCal(), Weather: tt.weather}
			p := k.Plan(context.Background(), camp, owner, rec("generator", ActionCreate, "Wx", f, ""))
			if tt.errorHas != "" {
				if !strings.Contains(p.Error, tt.errorHas) {
					t.Fatalf("error %q, want %q", p.Error, tt.errorHas)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tt.summary) {
				t.Fatalf("plan %+v", p)
			}
			var gp genPlan
			if err := json.Unmarshal([]byte(p.Client), &gp); err != nil {
				t.Fatal(err)
			}
			d, _ := gp.Recipe["details"].(map[string]any)
			if d["climate"] != tt.climate || d["continuity"] != tt.cont || len(gp.Kinds) != tt.kinds {
				t.Fatalf("details %+v kinds %d", d, len(gp.Kinds))
			}
		})
	}
}

// The Doc's climate list is the calendar's, so a new climate reaches the
// prompt without a change here.
func TestGeneratorKind_DocListsClimates(t *testing.T) {
	doc := GeneratorKind{}.Doc()
	for _, c := range calendar.WeatherClimates {
		if !strings.Contains(doc, c.ID) {
			t.Fatalf("Doc misses climate %q", c.ID)
		}
	}
}
