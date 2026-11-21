package calendarai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// generatedCalendar is a real run of the browser engine's calendar
// generator (the lunar built-in, elven, two moons, three seasons, named
// Starfall Count), as the review screen sends it back.
func generatedCalendar(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/generated_calendar.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// tamper edits the generated calendar the way a hand-edited payload might.
func tamper(t *testing.T, raw string, edit func(m map[string]any)) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	edit(m)
	b, _ := json.Marshal(m)
	return string(b)
}

func genKind(f *fakeStructure) GeneratorKind {
	return GeneratorKind{Cal: fakeStructureCal{f, newCal()}, Structure: f}
}

func harptosMain() *calendar.Calendar {
	return &calendar.Calendar{ID: "cal", Name: "Harptos", Months: []calendar.Month{{Name: "Hammer", Days: 30}}, Weekdays: []calendar.Weekday{{Name: "One"}}}
}

func TestGeneratorKind_CalendarPlan(t *testing.T) {
	cases := []struct {
		name    string
		actor   Actor
		action  string
		main    *calendar.Calendar
		others  []calendar.Calendar
		noSvc   bool
		fields  map[string]any
		wantErr string
		want    string
	}{
		{name: "no calendar: it becomes the main one", actor: owner, fields: map[string]any{"style": "moon-reckoned"}, want: "becomes the main calendar"},
		{name: "beside the main calendar, never replacing it", actor: owner, main: harptosMain(), fields: map[string]any{}, want: `beside "Harptos", which stays the main calendar`},
		{name: "named", actor: owner, fields: map[string]any{"calendar_name": "Starfall Count"}, want: `called "Starfall Count"`},
		{name: "players can't", actor: player, fields: map[string]any{}, wantErr: "only the campaign owner"},
		{name: "a scribe can't either", actor: Actor{UserID: "s", Role: 2}, fields: map[string]any{}, wantErr: "only the campaign owner"},
		{name: "only creates", actor: owner, action: ActionUpdate, fields: map[string]any{}, wantErr: "kind: calendar block"},
		{name: "unknown style", actor: owner, fields: map[string]any{"style": "metric"}, wantErr: "style"},
		{name: "unknown template", actor: owner, fields: map[string]any{"template": "decimal"}, wantErr: "template"},
		{name: "unknown theme", actor: owner, fields: map[string]any{"theme": "robot"}, wantErr: "theme"},
		{name: "randomness out of range", actor: owner, fields: map[string]any{"randomness": 2}, wantErr: "randomness"},
		{name: "too many months", actor: owner, fields: map[string]any{"months": 30}, wantErr: "months must be 1 to 24"},
		{name: "too many moons", actor: owner, fields: map[string]any{"moons": 6}, wantErr: "moons must be 0 to 5"},
		{name: "bad frequency", actor: owner, fields: map[string]any{"festival_days": "always"}, wantErr: "festival_days"},
		{name: "long name", actor: owner, fields: map[string]any{"calendar_name": strings.Repeat("x", 81)}, wantErr: "at most 80"},
		{name: "a name another calendar has", actor: owner, main: harptosMain(), others: []calendar.Calendar{{ID: "o", Name: "Starfall Count"}}, fields: map[string]any{"calendar_name": "starfall count"}, wantErr: "already exists"},
		{name: "the main calendar's name", actor: owner, main: harptosMain(), fields: map[string]any{"calendar_name": "Harptos"}, wantErr: "already exists"},
		{name: "server without the calendar service", actor: owner, noSvc: true, fields: map[string]any{}, wantErr: "aren't available"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeStructure{cal: c.main, others: c.others}
			k := genKind(f)
			if c.noSvc {
				k.Structure = nil
			}
			action := c.action
			if action == "" {
				action = ActionCreate
			}
			c.fields["generator"] = "calendar"
			p := k.Plan(context.Background(), camp, c.actor, rec("generator", action, "A calendar", c.fields, ""))
			if c.wantErr != "" {
				if !strings.Contains(p.Error, c.wantErr) {
					t.Fatalf("error = %q, want %q", p.Error, c.wantErr)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, c.want) {
				t.Fatalf("plan = %+v, want summary with %q", p, c.want)
			}
			if p.Client == "" {
				t.Fatal("no plan for the browser")
			}
		})
	}
}

func TestGeneratorKind_CalendarPlanRecipe(t *testing.T) {
	k := genKind(&fakeStructure{})
	p := k.Plan(context.Background(), camp, owner, rec("generator", ActionCreate, "", map[string]any{
		"generator": "calendar", "style": "tendays", "theme": "Dwarven", "week_length": 9, "current_year": 1200,
		"festival_days": "often", "leap_years": false, "moons": 0, "seed": "fixed",
	}, ""))
	var got buildPlan
	if err := json.Unmarshal([]byte(p.Client), &got); err != nil {
		t.Fatalf("client plan: %v (%+v)", err, p)
	}
	makes, details := got.Recipe["makes"].(map[string]any), got.Recipe["details"].(map[string]any)
	switch {
	case got.Base != "calendar.harptos-like", got.Seed != "fixed":
		t.Fatalf("base/seed = %q/%q", got.Base, got.Seed)
	case details["theme"] != "dwarven", details["weekLength"] != float64(9), details["currentYear"] != float64(1200):
		t.Fatalf("details = %v", details)
	case makes["festival-days"] != "often", makes["leap"] != false, makes["moons"] != float64(0):
		t.Fatalf("makes = %v", makes)
	}
}

func TestGeneratorKind_CalendarApply(t *testing.T) {
	good := generatedCalendar(t)
	cases := []struct {
		name      string
		main      *calendar.Calendar
		others    []calendar.Calendar
		fields    map[string]any
		generated string
		wantErr   string
	}{
		{name: "a generated calendar is created", generated: good},
		{name: "beside the main calendar", main: harptosMain(), generated: good},
		{name: "named as asked", fields: map[string]any{"calendar_name": "Starfall Count", "moons": 2}, generated: good},
		{name: "did not run", generated: "", wantErr: "did not run"},
		{name: "not a Chronicle calendar", generated: `{"format":"fantasy-calendar"}`, wantErr: "could not be read"},
		{name: "too large", generated: strings.Repeat(" ", maxGenCalendarBytes+1) + good, wantErr: "too large"},
		{name: "carries events", generated: tamper(t, good, func(m map[string]any) {
			m["events"] = []any{map[string]any{"name": "Planted", "year": 812, "month": 1, "day": 1}}
		}), wantErr: "carried events"},
		{name: "follows the real date", generated: tamper(t, good, func(m map[string]any) {
			m["calendar"].(map[string]any)["tracks_real_time"] = true
		}), wantErr: "real-world date"},
		{name: "more moons than asked for", fields: map[string]any{"moons": 1}, generated: good, wantErr: "returned 2 moons"},
		{name: "another name than asked for", fields: map[string]any{"calendar_name": "Other"}, generated: good, wantErr: "not the one asked for"},
		{name: "a second Import makes nothing", main: harptosMain(), others: []calendar.Calendar{{ID: "s", Name: "Starfall Count"}}, generated: good, wantErr: "already exists"},
		{name: "past the limits an upload has", generated: tamper(t, good, func(m map[string]any) {
			ms := make([]any, 201)
			for i := range ms {
				ms[i] = map[string]any{"name": "M", "days": 10}
			}
			m["calendar"].(map[string]any)["months"] = ms
		}), wantErr: "could not be used"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeStructure{cal: c.main, others: c.others}
			fields := map[string]any{"generator": "calendar"}
			for key, v := range c.fields {
				fields[key] = v
			}
			r := rec("generator", ActionCreate, "", fields, "")
			r.Generated = c.generated
			err := genKind(f).Apply(context.Background(), camp, owner, r)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %q", err, c.wantErr)
				}
				if f.created != nil {
					t.Fatal("a refused calendar was created")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if f.created == nil || f.created.CalendarName != "Starfall Count" || len(f.created.Moons) != 2 || len(f.created.Months) == 0 {
				t.Fatalf("created = %+v", f.created)
			}
			if f.applied != 0 || len(f.edits) != 0 {
				t.Fatal("an existing calendar's structure was touched")
			}
		})
	}
}

func moonCal(names ...string) *calendar.Calendar {
	cal := harptosMain()
	for i, n := range names {
		cal.Moons = append(cal.Moons, calendar.Moon{ID: i + 1, Name: n, CycleDays: 30, Color: "#eeeeee", HiddenFromPlayers: n == "Hidden"})
	}
	return cal
}

func TestGeneratorKind_MoonsPlan(t *testing.T) {
	cases := []struct {
		name    string
		actor   Actor
		cal     *calendar.Calendar
		fields  map[string]any
		wantErr string
		want    string
		locked  int
	}{
		{name: "adds beside the moons there", actor: owner, cal: moonCal("Selune"), fields: map[string]any{"count": 2}, want: "adds 2 moons to Harptos, keeping Selune as they are", locked: 1},
		{name: "hidden ones", actor: owner, cal: moonCal(), fields: map[string]any{"count": 2, "hidden": 1}, want: "(1 hidden from players)"},
		{name: "needs a count", actor: owner, cal: moonCal(), fields: map[string]any{}, wantErr: "how many moons"},
		{name: "room is what the sky has left", actor: owner, cal: moonCal("A", "B", "C", "D", "E"), fields: map[string]any{"count": 2}, wantErr: "count must be 1 to 1"},
		{name: "a full sky", actor: owner, cal: moonCal("A", "B", "C", "D", "E", "F"), fields: map[string]any{"count": 1}, wantErr: "already has 6 moons"},
		{name: "more hidden than new", actor: owner, cal: moonCal(), fields: map[string]any{"count": 1, "hidden": 2}, wantErr: "hidden must be 0 to 1"},
		{name: "bad cycles", actor: owner, cal: moonCal(), fields: map[string]any{"count": 1, "cycles": "weekly"}, wantErr: "cycles"},
		{name: "players can't", actor: player, cal: moonCal(), fields: map[string]any{"count": 1}, wantErr: "only the campaign owner"},
		{name: "no calendar", actor: owner, fields: map[string]any{"count": 1}, wantErr: needsCalendarPrefix},
		{name: "a real-world calendar", actor: owner, cal: func() *calendar.Calendar {
			c := moonCal()
			c.Mode, c.TracksRealTime = calendar.ModeRealLife, true
			return c
		}(), fields: map[string]any{"count": 1}, wantErr: "real-world date"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeStructure{cal: c.cal}
			c.fields["generator"] = "moons"
			p := genKind(f).Plan(context.Background(), camp, c.actor, rec("generator", ActionCreate, "", c.fields, ""))
			if c.wantErr != "" {
				if !strings.Contains(p.Error, c.wantErr) {
					t.Fatalf("error = %q, want %q", p.Error, c.wantErr)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, c.want) {
				t.Fatalf("plan = %+v, want %q", p, c.want)
			}
			var bp buildPlan
			if err := json.Unmarshal([]byte(p.Client), &bp); err != nil {
				t.Fatal(err)
			}
			if len(bp.Locked) != c.locked {
				t.Fatalf("locked = %v", bp.Locked)
			}
			if got := bp.Recipe["makes"].(map[string]any)["count"]; got != float64(c.locked+int(c.fields["count"].(int))) {
				t.Fatalf("engine count = %v; kept moons count in its total", got)
			}
		})
	}
}

func TestGeneratorKind_MoonsApply(t *testing.T) {
	cases := []struct {
		name      string
		generated string
		wantErr   string
	}{
		{name: "new moons join the ones there", generated: `[{"name":"Vantre","cycle_days":12.5,"phase_offset":3,"color":"#aabbcc","hidden":false},{"name":"Ashka","cycle_days":61,"phase_offset":0,"color":"#cc3333","hidden":true}]`},
		{name: "never replaces a moon", generated: `[{"name":"selune","cycle_days":12,"phase_offset":0,"color":"#ffffff"}]`, wantErr: "already on Harptos"},
		{name: "nor a hidden one", generated: `[{"name":"Hidden","cycle_days":12,"phase_offset":0,"color":"#ffffff"}]`, wantErr: "already on Harptos"},
		{name: "the same name twice", generated: `[{"name":"Vantre","cycle_days":12,"color":"#fff"},{"name":"VANTRE","cycle_days":40,"color":"#fff"}]`, wantErr: "already on Harptos"},
		{name: "more than asked for", generated: `[{"name":"A","cycle_days":12},{"name":"B","cycle_days":20},{"name":"C","cycle_days":40}]`, wantErr: "returned 3 moons"},
		{name: "a cycle the calendar block refuses", generated: `[{"name":"Vantre","cycle_days":0}]`, wantErr: "needs a cycle"},
		{name: "no name", generated: `[{"name":"","cycle_days":10}]`, wantErr: "needs a name"},
		{name: "unreadable", generated: `{"moons":1}`, wantErr: "could not be read"},
		{name: "did not run", generated: "", wantErr: "did not run"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeStructure{cal: moonCal("Selune", "Hidden")}
			r := rec("generator", ActionCreate, "", map[string]any{"generator": "moons", "count": 2}, "")
			r.Generated = c.generated
			err := genKind(f).Apply(context.Background(), camp, owner, r)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %q", err, c.wantErr)
				}
				if f.applied != 0 {
					t.Fatal("a refused run changed the calendar")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if f.applied != 1 || len(f.edits) != 1 {
				t.Fatalf("applied %d edits %d", f.applied, len(f.edits))
			}
			e := f.edits[0]
			if len(e.Moons) != 4 {
				t.Fatalf("moons = %+v", e.Moons)
			}
			// The kept moons keep their ids, so their look and hidden flag stay.
			for i, want := range []string{"Selune", "Hidden"} {
				if e.Moons[i].Name != want || e.Moons[i].ID == nil || *e.Moons[i].ID != i+1 {
					t.Fatalf("kept moon %d = %+v", i, e.Moons[i])
				}
			}
			if e.Moons[2].ID != nil || e.Moons[2].Name != "Vantre" || e.Moons[2].HiddenFromPlayers {
				t.Fatalf("new moon = %+v", e.Moons[2])
			}
			if !e.Moons[3].HiddenFromPlayers {
				t.Fatal("a moon generated hidden is not hidden")
			}
			if len(e.Months) != 1 || len(e.Weekdays) != 1 {
				t.Fatalf("the rest of the structure changed: %+v", e)
			}
		})
	}
}

// TestGeneratorKind_CalendarAndMoons_Integration runs both generators'
// output through the real calendar service: a generated calendar becomes
// the main one only when there is none, and generated moons join the moons
// already there without touching them.
func TestGeneratorKind_CalendarAndMoons_Integration(t *testing.T) {
	db := openCalendarTestDB(t)
	ctx := context.Background()
	user, campaign := "11111111-1111-1111-1111-111111111111", "33333333-3333-3333-3333-333333333333"
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`, user, "aigen@example.test", "AI Gen", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, campaign, "AI Gen", "ai-gen", user); err != nil {
		t.Fatal(err)
	}
	svc := calendar.NewCalendarService(calendar.NewCalendarRepository(db), calendar.NewEventRepository(db),
		calendar.NewEventKindRepository(db), calendar.NewWeatherRepository(db))
	a := Actor{UserID: user, Role: 3}
	k := GeneratorKind{Cal: svc, Structure: svc}
	good := generatedCalendar(t)

	apply := func(fields map[string]any, generated string) error {
		r := rec("generator", ActionCreate, "", fields, "")
		if p := k.Plan(ctx, campaign, a, r); p.Error != "" {
			return fmt.Errorf("plan: %s", p.Error)
		}
		r.Generated = generated
		return k.Apply(ctx, campaign, a, r)
	}

	if err := apply(map[string]any{"generator": "calendar"}, good); err != nil {
		t.Fatal(err)
	}
	cal, err := svc.GetDefaultCalendarForViewer(ctx, campaign, a.Viewer())
	if err != nil || cal.Name != "Starfall Count" || len(cal.Moons) != 2 || len(cal.Seasons) != 3 || cal.CurrentYear != 812 {
		t.Fatalf("the generated calendar is not the main one as made: %v %+v", err, cal)
	}
	if err := apply(map[string]any{"generator": "calendar"}, good); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("a second Import: %v", err)
	}

	// A second calendar goes beside the first, which stays the main one.
	other := tamper(t, good, func(m map[string]any) { m["calendar"].(map[string]any)["name"] = "Second Count" })
	if err := apply(map[string]any{"generator": "calendar"}, other); err != nil {
		t.Fatal(err)
	}
	cals, _ := svc.ListCalendars(ctx, campaign, a.Viewer())
	main, _ := svc.GetDefaultCalendarForViewer(ctx, campaign, a.Viewer())
	if len(cals) != 2 || main.ID != cal.ID {
		t.Fatalf("%d calendars, main %s (was %s)", len(cals), main.Name, cal.Name)
	}

	kept := cal.Moons
	moons := `[{"name":"Vantre","cycle_days":12.5,"phase_offset":3.25,"color":"#aabbcc","hidden":false},{"name":"Ashka","cycle_days":61,"phase_offset":10,"color":"#cc3333","hidden":true}]`
	if err := apply(map[string]any{"generator": "moons", "count": 2, "hidden": 1}, moons); err != nil {
		t.Fatal(err)
	}
	cal, _ = svc.GetDefaultCalendarForViewer(ctx, campaign, a.Viewer())
	if len(cal.Moons) != 4 {
		t.Fatalf("moons: %+v", cal.Moons)
	}
	byName := map[string]calendar.Moon{}
	for _, m := range cal.Moons {
		byName[m.Name] = m
	}
	for _, old := range kept {
		got := byName[old.Name]
		if got.ID != old.ID || got.CycleDays != old.CycleDays || got.PhaseOffset != old.PhaseOffset || got.Color != old.Color {
			t.Fatalf("kept moon changed: %+v, was %+v", got, old)
		}
	}
	v, ash := byName["Vantre"], byName["Ashka"]
	if v.CycleDays != 12.5 || v.PhaseOffset != 3.25 || v.Color != "#aabbcc" || v.HiddenFromPlayers {
		t.Fatalf("Vantre = %+v", v)
	}
	if !ash.HiddenFromPlayers || ash.CycleDays != 61 {
		t.Fatalf("Ashka = %+v", ash)
	}
}
