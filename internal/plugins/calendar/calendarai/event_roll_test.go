package calendarai

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

type fakeRolls struct {
	cfg    map[string]*calendar.RollConfig
	set    []*calendar.RollConfig
	rolled []calendar.RollNightsInput
	locked []bool
	told   []*bool
	chosen []string
	// calls is every write in order, to check a hand-set result is locked
	// after it is stored.
	calls []string
}

func newFakeRolls() *fakeRolls { return &fakeRolls{cfg: map[string]*calendar.RollConfig{}} }

func (f *fakeRolls) GetEventRoll(_ context.Context, id, _, _ string, _ int, _ permissions.Viewer) (*calendar.EventRollView, error) {
	return &calendar.EventRollView{EventID: id, Config: f.cfg[id]}, nil
}
func (f *fakeRolls) ListMonthRolls(context.Context, string, string, int, int, permissions.Viewer) (map[string][]calendar.RollNightView, error) {
	return nil, nil
}
func (f *fakeRolls) SetEventRoll(_ context.Context, id, _, _ string, cfg *calendar.RollConfig, _ int, _ permissions.Viewer) (*calendar.EventRollView, error) {
	f.set = append(f.set, cfg)
	f.cfg[id] = cfg
	return &calendar.EventRollView{EventID: id, Config: cfg}, nil
}
func (f *fakeRolls) RollEventNights(_ context.Context, id, _, _ string, in calendar.RollNightsInput, _ permissions.Viewer) (*calendar.EventRollView, error) {
	f.rolled = append(f.rolled, in)
	return &calendar.EventRollView{EventID: id}, nil
}
func (f *fakeRolls) LockEventRollNights(_ context.Context, _, _, _ string, _ []calendar.DayDate, locked bool, _ permissions.Viewer) (int, error) {
	f.locked = append(f.locked, locked)
	f.calls = append(f.calls, map[bool]string{true: "lock", false: "unlock"}[locked])
	return 1, nil
}
func (f *fakeRolls) SetEventRollNight(_ context.Context, _, _, _ string, _ calendar.DayDate, result string, _ permissions.Viewer) error {
	f.chosen = append(f.chosen, result)
	f.calls = append(f.calls, "set "+result)
	return nil
}
func (f *fakeRolls) TellEventRollNight(_ context.Context, _, _, _ string, _ calendar.DayDate, told *bool, _ permissions.Viewer) error {
	f.told = append(f.told, told)
	return nil
}

func bloodnightRoll() map[string]any {
	return map[string]any{
		"dice":         "2d10",
		"on_the_night": true,
		"results": []any{
			map[string]any{"max": 11, "name": "Thin blood", "players": "Dull.", "director": "Nothing."},
			map[string]any{"max": 16, "name": "The Hunger", "players": "Red.", "director": "Roll need.", "tell_players": true},
			map[string]any{"name": "Red Tide", "players": "Blood sea.", "director": "Run it.", "tell_players": true},
		},
	}
}

func TestEventKindRoll(t *testing.T) {
	base := func(roll any) map[string]any {
		return map[string]any{"year": 1492, "month": 1, "day": 3, "roll": roll}
	}
	tests := []struct {
		name    string
		actor   Actor
		roll    any
		action  string
		wantErr string
	}{
		{"dice roll", owner, bloodnightRoll(), ActionCreate, ""},
		{"table by name", owner, map[string]any{"table": "Omens", "tell_players": true}, ActionCreate, ""},
		{"unknown table", owner, map[string]any{"table": "Nope"}, ActionCreate, "no rolling table"},
		{"player", player, bloodnightRoll(), ActionCreate, "owner or a co-DM"},
		{"bad key", owner, map[string]any{"dice": "2d10", "results": []any{map[string]any{"name": "x"}}, "colour": "red"}, ActionCreate, "not a roll key"},
		{"bands out of order", owner, map[string]any{"dice": "2d10", "results": []any{
			map[string]any{"max": 12, "name": "a"}, map[string]any{"max": 5, "name": "b"}, map[string]any{"name": "c"}}}, ActionCreate, "lowest first"},
		{"none on create", owner, "none", ActionCreate, "only applies to action: update"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCal()
			rolls := newFakeRolls()
			k := EventKind{Svc: c, Rolls: rolls, Tables: &fakeTables{ids: map[string]string{"Omens": "omens"}}}
			r := rec("event", tt.action, "Bloodnight", base(tt.roll), "")
			p := k.Plan(context.Background(), camp, tt.actor, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("plan error %q, want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("plan: %s", p.Error)
			}
			if err := k.Apply(context.Background(), camp, tt.actor, r); err != nil {
				t.Fatal(err)
			}
			if len(rolls.set) != 1 || rolls.set[0] == nil {
				t.Fatalf("roll not stored: %+v", rolls.set)
			}
		})
	}

	// The table is stored by its id, whatever name the AI used.
	rolls := newFakeRolls()
	k := EventKind{Svc: newCal(), Rolls: rolls, Tables: &fakeTables{ids: map[string]string{"Omens": "omens"}}}
	r := rec("event", ActionCreate, "Bloodnight", base(map[string]any{"table": "omens"}), "")
	if err := k.Apply(context.Background(), camp, owner, r); err != nil || rolls.set[0].TableID != "omens" {
		t.Fatalf("table roll %+v %v", rolls.set, err)
	}
}

func TestRollsGenerator(t *testing.T) {
	c := newCal()
	c.stored = []calendar.Event{{ID: "ev1", Name: "Bloodnight", Year: 1492, Month: 1, Day: 3}}
	rolls := newFakeRolls()
	k := GeneratorKind{Cal: c, Rolls: rolls}
	fields := map[string]any{"generator": "rolls", "event": "Bloodnight", "year": 1493, "keep_locked": true}
	r := rec("generator", ActionCreate, "", fields, "")

	p := k.Plan(context.Background(), camp, owner, r)
	if p.Error != "" || p.Client != "" || len(p.Warnings) != 1 {
		t.Fatalf("an event with no roll should plan with a warning and nothing for the browser: %+v", p)
	}
	rolls.cfg["ev1"] = &calendar.RollConfig{Source: calendar.RollSourceDice, Dice: "2d10"}
	if p = k.Plan(context.Background(), camp, owner, r); p.Error != "" || len(p.Warnings) != 0 {
		t.Fatalf("plan: %+v", p)
	}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	if len(rolls.rolled) != 1 || rolls.rolled[0].Year != 1493 {
		t.Fatalf("rolled %+v", rolls.rolled)
	}

	fields["keep_locked"] = false
	if p := k.Plan(context.Background(), camp, owner, rec("generator", ActionCreate, "", fields, "")); !strings.Contains(p.Error, "locked") {
		t.Fatalf("keep_locked: false should be refused, got %+v", p)
	}
	fields["keep_locked"] = true
	if p := k.Plan(context.Background(), camp, player, rec("generator", ActionCreate, "", fields, "")); p.Error == "" {
		t.Fatal("a player planned a roll")
	}
}

func TestRollNightKind(t *testing.T) {
	c := newCal()
	c.stored = []calendar.Event{{ID: "ev1", Name: "Bloodnight", Year: 1492, Month: 1, Day: 3}}
	rolls := newFakeRolls()
	k := RollNightKind{Cal: c, Rolls: rolls}
	r := rec("roll_night", ActionUpdate, "Bloodnight", map[string]any{"year": 1492, "month": 2, "day": 3, "reroll": true, "locked": true, "tell_players": "default"}, "")
	if p := k.Plan(context.Background(), camp, owner, r); p.Error != "" {
		t.Fatalf("plan: %s", p.Error)
	}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	// Rerolled, then locked so the new result stays; told back to default.
	if len(rolls.rolled) != 1 || len(rolls.locked) != 1 || !rolls.locked[0] || len(rolls.told) != 1 || rolls.told[0] != nil {
		t.Fatalf("rolled %v locked %v told %v", rolls.rolled, rolls.locked, rolls.told)
	}
	if p := k.Plan(context.Background(), camp, player, r); p.Error == "" {
		t.Fatal("a player planned a night change")
	}
	empty := rec("roll_night", ActionUpdate, "Bloodnight", map[string]any{"year": 1492, "month": 2, "day": 3}, "")
	if p := k.Plan(context.Background(), camp, owner, empty); p.Error == "" {
		t.Fatal("a row that does nothing should be refused")
	}
}

// A night given a result by hand is stored, then locked so a year's reroll
// keeps it; locked: false leaves it open, and a reroll with a result is
// refused as two answers to one question.
func TestRollNightKind_Result(t *testing.T) {
	c := newCal()
	c.stored = []calendar.Event{{ID: "ev1", Name: "Bloodnight", Year: 1492, Month: 1, Day: 3}}
	night := func(extra map[string]any) Record {
		f := map[string]any{"year": 1492, "month": 2, "day": 3}
		for k, v := range extra {
			f[k] = v
		}
		return rec("roll_night", ActionUpdate, "Bloodnight", f, "")
	}
	tests := []struct {
		name  string
		extra map[string]any
		calls []string
		bad   bool
	}{
		{"locked by default", map[string]any{"result": "Red Tide"}, []string{"set Red Tide", "lock"}, false},
		{"left unlocked", map[string]any{"result": "Red Tide", "locked": false}, []string{"unlock", "set Red Tide"}, false},
		{"a total", map[string]any{"result": 14}, []string{"set 14", "lock"}, false},
		{"reroll and result", map[string]any{"result": "Red Tide", "reroll": true}, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rolls := newFakeRolls()
			k := RollNightKind{Cal: c, Rolls: rolls}
			r := night(tc.extra)
			p := k.Plan(context.Background(), camp, owner, r)
			if tc.bad {
				if p.Error == "" {
					t.Fatal("planned a row that should be refused")
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("plan: %s", p.Error)
			}
			if !strings.Contains(p.Summary, "Bloodnight") || !strings.Contains(p.Summary, "result") {
				t.Fatalf("summary %q", p.Summary)
			}
			if err := k.Apply(context.Background(), camp, owner, r); err != nil {
				t.Fatal(err)
			}
			if strings.Join(rolls.calls, "|") != strings.Join(tc.calls, "|") {
				t.Fatalf("calls %v, want %v", rolls.calls, tc.calls)
			}
		})
	}
}
