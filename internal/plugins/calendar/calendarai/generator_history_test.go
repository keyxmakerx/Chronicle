package calendarai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// fakeEras records the eras a history run writes.
type fakeEras struct{ made []calendar.EraInput }

func (f *fakeEras) CreateEra(_ context.Context, _, _ string, in calendar.EraInput) (*calendar.Era, error) {
	f.made = append(f.made, in)
	return &calendar.Era{Name: in.Name}, nil
}

var scribe = Actor{UserID: "sc", Role: 2}

func yr(y int) *int { return &y }

// histCal is a two-month calendar at year 1492 with the given eras.
func histCal(eras ...calendar.Era) *fakeCal {
	c := newCal()
	c.cal.CurrentYear, c.cal.CurrentMonth, c.cal.CurrentDay = 1492, 1, 1
	c.cal.Eras = eras
	return c
}

func histRec(fields map[string]any) Record {
	f := map[string]any{"generator": "history"}
	for k, v := range fields {
		f[k] = v
	}
	return rec("generator", ActionCreate, "Past", f, "")
}

func TestGeneratorKind_HistoryPlan(t *testing.T) {
	one := calendar.Era{Name: "The Long Now", StartYear: 1}
	two := []calendar.Era{{Name: "First", StartYear: 1, EndYear: yr(999)}, {Name: "Second", StartYear: 1000}}
	tests := []struct {
		name      string
		actor     Actor
		eras      []calendar.Era
		noEraAPI  bool
		year      int
		fields    map[string]any
		wantErr   string
		makesEras bool
		warn      string
	}{
		{name: "owner, no eras", actor: owner, makesEras: true},
		{name: "one era: may add eras, which give way", actor: owner, eras: []calendar.Era{one}, makesEras: true, warn: "give way"},
		{name: "two eras: events only", actor: owner, eras: two, makesEras: false},
		{name: "no era writer: events only", actor: owner, noEraAPI: true, makesEras: false},
		{name: "every setting", actor: owner, fields: map[string]any{"recipe": "history.dark", "years": 400, "eras": 4, "density": 8, "tone": "hopeful", "theme": "dwarven", "realm": "Ostmark", "secrets": false, "wars": "often", "plagues": "off"}, makesEras: true},
		{name: "player", actor: player, wantErr: "owner or a co-DM"},
		{name: "plain scribe", actor: scribe, wantErr: "owner or a co-DM"},
		{name: "unknown recipe", actor: owner, fields: map[string]any{"recipe": "empire"}, wantErr: "recipe must be"},
		{name: "years too far", actor: owner, fields: map[string]any{"years": 5001}, wantErr: "years must be"},
		{name: "too many eras", actor: owner, fields: map[string]any{"eras": 9}, wantErr: "eras must be"},
		{name: "density too high", actor: owner, fields: map[string]any{"density": 25}, wantErr: "density"},
		{name: "bad tone", actor: owner, fields: map[string]any{"tone": "sad"}, wantErr: "tone must be"},
		{name: "bad theme", actor: owner, fields: map[string]any{"theme": "lunar"}, wantErr: "theme must be"},
		{name: "bad frequency", actor: owner, fields: map[string]any{"wars": "always"}, wantErr: "wars must be"},
		{name: "long realm", actor: owner, fields: map[string]any{"realm": strings.Repeat("x", 61)}, wantErr: "realm"},
		{name: "over the story budget", actor: owner, year: 6000, fields: map[string]any{"years": 5000, "density": 20}, wantErr: "stories"},
		{name: "budget counts the generator's own span", actor: owner, year: 6000, fields: map[string]any{"density": 20}, makesEras: true},
		{name: "no past", actor: owner, year: 1, wantErr: "no past"},
		{name: "update refused", actor: owner, fields: map[string]any{}, wantErr: "action: create"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := histCal(tt.eras...)
			if tt.year != 0 {
				c.cal.CurrentYear = tt.year
			}
			k := GeneratorKind{Cal: c, Eras: &fakeEras{}}
			if tt.noEraAPI {
				k.Eras = nil
			}
			r := histRec(tt.fields)
			if tt.name == "update refused" {
				r.Action = ActionUpdate
			}
			p := k.Plan(context.Background(), camp, tt.actor, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("error %q, want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" || p.Client == "" {
				t.Fatalf("plan: %+v", p)
			}
			var hp historyPlan
			if err := json.Unmarshal([]byte(p.Client), &hp); err != nil {
				t.Fatal(err)
			}
			if hp.Generator != "history" || hp.Seed == "" || hp.BasedOn == "" || len(hp.Calendar) == 0 {
				t.Fatalf("plan to run: %+v", hp)
			}
			if hp.MakesEras != tt.makesEras {
				t.Fatalf("makesEras %v, want %v", hp.MakesEras, tt.makesEras)
			}
			if len(hp.KeepEras) != len(tt.eras) {
				t.Fatalf("keeps %d eras, want %d", len(hp.KeepEras), len(tt.eras))
			}
			if (tt.warn == "") != (len(p.Warnings) == 0) || (tt.warn != "" && !strings.Contains(p.Warnings[0], tt.warn)) {
				t.Fatalf("warnings %v, want %q", p.Warnings, tt.warn)
			}
		})
	}
}

func TestGeneratorKind_HistoryPlanCarriesSettings(t *testing.T) {
	k := GeneratorKind{Cal: histCal(), Eras: &fakeEras{}}
	p := k.Plan(context.Background(), camp, owner, histRec(map[string]any{"recipe": "frontier", "tone": "hopeful", "secrets": false, "wars": "Rare", "seed": "abc"}))
	var hp historyPlan
	if err := json.Unmarshal([]byte(p.Client), &hp); err != nil {
		t.Fatal(err)
	}
	d := hp.Recipe["details"].(map[string]any)
	m := hp.Recipe["makes"].(map[string]any)
	if hp.BasedOn != "history.frontier" || hp.Seed != "abc" || d["tone"] != "bright" || d["secrets"] != false || m["wars"] != "rare" {
		t.Fatalf("plan %+v", hp)
	}
	if _, set := d["years"]; set {
		t.Fatal("an unset key overrode the built-in")
	}
	if !strings.Contains(p.Summary, "no hidden history") {
		t.Fatalf("summary %q", p.Summary)
	}
}

func TestGeneratorKind_HistoryApply(t *testing.T) {
	era := func(name string, from int, to *int, color string) string {
		b, _ := json.Marshal(histEra{Name: name, StartYear: from, EndYear: to, Color: color})
		return string(b)
	}
	evs := `[{"name":"Founding","year":600,"month":1,"day":3,"visibility":"everyone","description":"Walls go up."},` +
		`{"name":"Secret treaty","year":900,"month":2,"day":4,"visibility":"dm_only"},` +
		`{"name":"Odd visibility","year":901,"month":2,"day":4,"visibility":"players"}]`
	tests := []struct {
		name       string
		actor      Actor
		eras       []calendar.Era
		fields     map[string]any
		out        string
		wantErr    string
		wantEras   int
		wantEvents int
	}{
		{name: "eras and events", actor: owner,
			out:      `{"eras":[` + era("Founding Age", 571, yr(899), "#a1b2c3") + `,` + era("Present", 900, nil, "oklch(0.6 0.1 40)") + `],"events":` + evs + `}`,
			wantEras: 2, wantEvents: 3},
		{name: "two eras already: eras left out, events kept", actor: owner,
			eras:       []calendar.Era{{Name: "A", StartYear: 1, EndYear: yr(500)}, {Name: "B", StartYear: 501}},
			out:        `{"eras":[` + era("Sneaky", 1, yr(10), "") + `],"events":` + evs + `}`,
			wantEvents: 3},
		{name: "overlaps the calendar's era: nothing written", actor: owner,
			eras:    []calendar.Era{{Name: "Mine", StartYear: 1200, EndYear: yr(1300)}},
			out:     `{"eras":[` + era("Over", 1250, yr(1400), "") + `],"events":` + evs + `}`,
			wantErr: "overlaps the era"},
		{name: "open era runs on over a later era", actor: owner,
			eras:    []calendar.Era{{Name: "Later", StartYear: 1600}},
			out:     `{"eras":[` + era("Open", 900, nil, "") + `],"events":[]}`,
			wantErr: "overlaps the era"},
		{name: "generated eras overlap each other", actor: owner,
			out:     `{"eras":[` + era("One", 100, yr(300), "") + `,` + era("Two", 300, yr(400), "") + `],"events":[]}`,
			wantErr: "overlaps the era"},
		{name: "era in the future", actor: owner,
			out:     `{"eras":[` + era("Tomorrow", 1400, yr(1600), "") + `],"events":[]}`,
			wantErr: "outside the calendar's past"},
		{name: "era already there by name is skipped", actor: owner,
			eras:     []calendar.Era{{Name: "Founding Age", StartYear: 571, EndYear: yr(899)}},
			out:      `{"eras":[` + era("Founding Age", 571, yr(899), "") + `,` + era("Present", 900, nil, "") + `],"events":[]}`,
			wantEras: 1},
		{name: "future and malformed events fail, the rest go in", actor: owner,
			out:     `{"eras":[],"events":[{"name":"Ok","year":10,"month":1,"day":1},{"name":"Future","year":1493,"month":1,"day":1},{"name":"No such day","year":10,"month":2,"day":31},{"name":"  ","year":10,"month":1,"day":2}]}`,
			wantErr: "3 of 4", wantEvents: 1},
		{name: "event older than the years asked for", actor: owner, fields: map[string]any{"years": 100},
			out:     `{"eras":[],"events":[{"name":"Too old","year":1000,"month":1,"day":1},{"name":"Recent","year":1450,"month":1,"day":1}]}`,
			wantErr: "1 of 2", wantEvents: 1},
		{name: "too many events", actor: owner,
			out:     `{"eras":[],"events":[` + strings.TrimSuffix(strings.Repeat(`{"name":"x","year":1,"month":1,"day":1},`, maxGenEvents+1), ",") + `]}`,
			wantErr: "returned 301 events"},
		{name: "too many eras", actor: owner,
			out:     `{"eras":[` + strings.TrimSuffix(strings.Repeat(era("e", 1, yr(1), "")+",", maxGenHistoryEras+1), ",") + `],"events":[]}`,
			wantErr: "returned 9 eras"},
		{name: "not run", actor: owner, out: ``, wantErr: "did not run"},
		{name: "garbage", actor: owner, out: `[1,2]`, wantErr: "could not be read"},
		{name: "player", actor: player, out: `{"eras":[],"events":[]}`, wantErr: "owner or a co-DM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := histCal(tt.eras...)
			fe := &fakeEras{}
			k := GeneratorKind{Cal: c, Eras: fe}
			r := histRec(tt.fields)
			r.Generated = tt.out
			err := k.Apply(context.Background(), camp, tt.actor, r)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("apply: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("err %v, want %q", err, tt.wantErr)
			}
			if len(fe.made) != tt.wantEras || len(c.events) != tt.wantEvents {
				t.Fatalf("wrote %d eras and %d events, want %d and %d", len(fe.made), len(c.events), tt.wantEras, tt.wantEvents)
			}
			for _, e := range c.events {
				if !e.AllDay || e.IsRecurring || e.Announced == nil || *e.Announced != calendar.AnnouncedOnDay || !e.CanAuthorDmOnly {
					t.Fatalf("history event saved as %+v", e)
				}
			}
		})
	}
}

// Hidden history must stay Director-only, and an era colour the calendar
// would refuse is dropped rather than failing the run.
func TestGeneratorKind_HistoryKeepsSecretsHidden(t *testing.T) {
	c := histCal()
	fe := &fakeEras{}
	k := GeneratorKind{Cal: c, Eras: fe}
	r := histRec(nil)
	r.Generated = `{"eras":[{"name":"Age","start_year":10,"end_year":20,"color":"oklch(0.6 0.1 40)"},{"name":"Next","start_year":21,"end_year":null,"color":"#abc"}],` +
		`"events":[{"name":"Poisoned","year":15,"month":1,"day":1,"visibility":"dm_only"},{"name":"Crowned","year":16,"month":1,"day":1,"visibility":"everyone"}]}`
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	vis := map[string]string{}
	for _, e := range c.events {
		vis[e.Name] = e.Visibility
	}
	if vis["Poisoned"] != "dm_only" || vis["Crowned"] != "everyone" {
		t.Fatalf("visibility %v", vis)
	}
	if fe.made[0].Color != "" || fe.made[1].Color != "#abc" || fe.made[1].EndYear != nil || fe.made[0].StartMonth != 1 || fe.made[0].StartDay != 1 {
		t.Fatalf("eras %+v", fe.made)
	}
}

// A rerun after a partial failure adds only the missing events, and the
// eras the first run made switch the second to events only.
func TestGeneratorKind_HistoryRerun(t *testing.T) {
	ctx := context.Background()
	c := histCal()
	c.failNames = map[string]bool{"Flood": true}
	fe := &fakeEras{}
	k := GeneratorKind{Cal: c, Eras: fe}
	r := histRec(nil)
	r.Generated = `{"eras":[{"name":"A","start_year":1,"end_year":100},{"name":"B","start_year":101,"end_year":null}],` +
		`"events":[{"name":"Founding","year":5,"month":1,"day":1},{"name":"Flood","year":50,"month":1,"day":1}]}`
	if err := k.Apply(ctx, camp, owner, r); err == nil || !strings.Contains(err.Error(), "1 were added") {
		t.Fatalf("first run: %v", err)
	}
	for _, in := range fe.made {
		c.cal.Eras = append(c.cal.Eras, calendar.Era{Name: in.Name, StartYear: in.StartYear, EndYear: in.EndYear})
	}
	c.failNames = nil
	if err := k.Apply(ctx, camp, owner, r); err != nil {
		t.Fatal(err)
	}
	if len(c.events) != 2 || len(fe.made) != 2 {
		t.Fatalf("after rerun: %d events, %d eras; want 2 and 2", len(c.events), len(fe.made))
	}
}
