package calendarai

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"gopkg.in/yaml.v3"
)

func skyCal(moons ...calendar.Moon) *fakeCal {
	c := newCal()
	c.cal.Moons = moons
	c.stored = []calendar.Event{{ID: "ev-feast", Name: "Feast", Year: 1492, Month: 1, Day: 21}}
	return c
}

var (
	selune = calendar.Moon{ID: 7, Name: "Selûne"}
	shard  = calendar.Moon{ID: 8, Name: "The Shard"}
)

// Pins `sky:` on a new event: moons by name, the one-moon default, the
// calendar's own checks, and the payload the event is saved with.
func TestEventKind_SkyOnCreate(t *testing.T) {
	tests := []struct {
		name        string
		moons       []calendar.Moon
		sky         any
		wantPayload string
		wantSummary string
		wantErr     string
	}{
		{name: "comet", moons: []calendar.Moon{selune},
			sky:         map[string]any{"type": "comet", "direction": map[string]any{"compass": "W", "altitude": "low"}, "color": "#ffd9a0", "tail_degrees": 30},
			wantPayload: `{"type":"comet","direction":{"compass":"W","altitude":"low"},"color":"#ffd9a0","tail_degrees":30}`,
			wantSummary: "in the sky: a comet, low in the west"},
		{name: "moon by name, any case", moons: []calendar.Moon{selune, shard},
			sky:         map[string]any{"type": "blood", "moons": []any{"selûne"}},
			wantPayload: `{"type":"blood","moons":[7]}`,
			wantSummary: "in the sky: a blood moon (Selûne)"},
		{name: "one name, not a list", moons: []calendar.Moon{selune, shard},
			sky:         map[string]any{"type": "conjunction", "moons": "The Shard"},
			wantPayload: `{"type":"conjunction","moons":[8]}`},
		{name: "one-moon calendar fills the moon in", moons: []calendar.Moon{selune},
			sky:         "Hunter's Moon",
			wantPayload: `{"type":"hunters-moon","moons":[7]}`,
			wantSummary: "in the sky: a hunter's moon (Selûne)"},
		{name: "words for types and directions", moons: nil,
			sky:         map[string]any{"Type": "Meteor Shower", "direction": "north east", "rate_per_hour": 60},
			wantPayload: `{"type":"meteor-shower","direction":{"compass":"NE"},"rate_per_hour":60}`,
			wantSummary: "in the sky: a meteor shower, in the north-east"},
		{name: "solar eclipse needs no moon on a busy calendar", moons: []calendar.Moon{selune, shard},
			sky:         map[string]any{"type": "solar eclipse", "subtype": "Total"},
			wantPayload: `{"type":"eclipse-solar","subtype":"total"}`,
			wantSummary: "in the sky: a total solar eclipse"},

		{name: "bad colour", sky: map[string]any{"type": "comet", "color": "orange"},
			wantErr: "sky event: color must be a colour written like #ffd9a0"},
		{name: "bad type", sky: map[string]any{"type": "dragon"},
			wantErr: `sky event: "dragon" is not a kind of sky event`},
		{name: "unknown field", sky: map[string]any{"type": "comet", "speed": 3},
			wantErr: `sky event: "speed" is not a sky event field`},
		{name: "out of range", sky: map[string]any{"type": "aurora", "intensity": 2},
			wantErr: "sky event: intensity must be a number from 0 to 1"},
		{name: "not a moon of the calendar", moons: []calendar.Moon{selune},
			sky:     map[string]any{"type": "blood", "moons": []any{"Luna"}},
			wantErr: `sky event: "Luna" is not a moon of Harptos (moons: Selûne)`},
		{name: "which moon, on a two-moon calendar", moons: []calendar.Moon{selune, shard},
			sky:     "blood",
			wantErr: "sky event: say which moon a blood moon is with moons (moons: Selûne, The Shard)"},
		{name: "a moon night with no moons at all", sky: "harvest",
			wantErr: "sky event: a harvest moon needs a moon, and Harptos has none"},
		{name: "no type", sky: map[string]any{"color": "#ffffff"},
			wantErr: "sky needs a type"},
		{name: "none on create", sky: "none",
			wantErr: "sky: none only applies to action: update"},
		{name: "a list", sky: []any{"comet"},
			wantErr: "sky is a sky event"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := skyCal(tt.moons...)
			k := EventKind{Svc: c}
			r := rec("event", ActionCreate, "Sky night", map[string]any{"year": 1492, "month": "Hammer", "day": 3, "sky": tt.sky}, "")
			p := k.Plan(context.Background(), camp, owner, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("plan error %q, want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("plan: %+v", p)
			}
			if tt.wantSummary != "" && !strings.Contains(p.Summary, tt.wantSummary) {
				t.Errorf("summary %q, want it to say %q", p.Summary, tt.wantSummary)
			}
			for _, w := range p.Warnings {
				if strings.Contains(w, "sky") {
					t.Errorf("unexpected warning %q", w)
				}
			}
			if err := k.Apply(context.Background(), camp, owner, r); err != nil {
				t.Fatalf("apply: %v", err)
			}
			in := c.events[len(c.events)-1]
			if in.Payload == nil || *in.Payload != tt.wantPayload {
				got := "<nil>"
				if in.Payload != nil {
					got = *in.Payload
				}
				t.Errorf("payload %s, want %s", got, tt.wantPayload)
			}
		})
	}
}

// Pins `sky:` on update: a new sky replaces the payload, `none` clears it,
// leaving it out keeps it, and it does not go with a delete.
func TestEventKind_SkyOnUpdate(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		fields  map[string]any
		check   func(t *testing.T, in []string)
		wantErr string
	}{
		{name: "replace", action: ActionUpdate, fields: map[string]any{"sky": map[string]any{"type": "dark-night", "intensity": 0.7}}},
		{name: "none clears", action: ActionUpdate, fields: map[string]any{"sky": "None"}},
		{name: "absent keeps", action: ActionUpdate, fields: map[string]any{"rename_to": "Great Feast"}},
		{name: "not with delete", action: ActionDelete, fields: map[string]any{"sky": "comet"}, wantErr: "sky doesn't go with action: delete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := skyCal(selune)
			k := EventKind{Svc: c}
			r := rec("event", tt.action, "Feast", tt.fields, "")
			p := k.Plan(context.Background(), camp, owner, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("plan error %q, want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("plan: %+v", p)
			}
			if err := k.Apply(context.Background(), camp, owner, r); err != nil {
				t.Fatalf("apply: %v", err)
			}
			in := c.updates[0].Payload
			switch tt.name {
			case "replace":
				if v, ok := in.Get(); !ok || v != `{"type":"dark-night","intensity":0.7}` {
					t.Errorf("payload %+v", in)
				}
				if !strings.Contains(p.Summary, "in the sky: a dark night") {
					t.Errorf("summary %q", p.Summary)
				}
			case "none clears":
				if !in.IsNull() {
					t.Errorf("payload %+v, want null", in)
				}
				if !strings.Contains(p.Summary, "no sky event") {
					t.Errorf("summary %q", p.Summary)
				}
			case "absent keeps":
				if in.Present() {
					t.Errorf("payload %+v, want absent", in)
				}
			}
		})
	}
}

// Pins that the Doc's own example, read as YAML the way the importer reads
// a block, plans and saves the sky event it shows.
func TestEventKind_SkyDocExample(t *testing.T) {
	doc := EventKind{}.Doc()
	start := strings.Index(doc, "name: The Ember Comet")
	if start < 0 {
		t.Fatal("the Doc lost its sky example")
	}
	block := doc[start:]
	block = block[:strings.Index(block, "\n---")]
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(block), &fields); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	c := skyCal(selune)
	c.cal.Months = append(c.cal.Months, calendar.Month{Name: "Mirtul", Days: 30})
	k := EventKind{Svc: c}
	r := rec("event", ActionCreate, "The Ember Comet", fields, "")
	if p := k.Plan(context.Background(), camp, owner, r); p.Error != "" || len(p.Warnings) > 0 {
		t.Fatalf("plan: %+v", p)
	}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatalf("apply: %v", err)
	}
	want := `{"type":"comet","direction":{"compass":"W","altitude":"low"},"color":"#ffd9a0","tail_degrees":30}`
	if got := c.events[0].Payload; got == nil || *got != want {
		t.Errorf("payload %v, want %s", got, want)
	}
}
