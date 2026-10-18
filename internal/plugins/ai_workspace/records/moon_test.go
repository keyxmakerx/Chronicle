package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// fakeMarks records the moon mark writes a block makes.
type fakeMarks struct {
	modes []string
	names []map[string]string
	puts  []calendar.MoonMark
	dels  []calendar.DayDate
}

func (f *fakeMarks) SetMoonMode(_ context.Context, _, _ string, _ int, mode string, names map[string]string, _ permissions.Viewer) (*calendar.Moon, error) {
	f.modes, f.names = append(f.modes, mode), append(f.names, names)
	return &calendar.Moon{}, nil
}
func (f *fakeMarks) SetMoonMark(_ context.Context, _, _ string, _ int, k calendar.MoonMark, _ permissions.Viewer) (*calendar.Moon, error) {
	f.puts = append(f.puts, k)
	return &calendar.Moon{}, nil
}
func (f *fakeMarks) DeleteMoonMark(_ context.Context, _, _ string, _ int, d calendar.DayDate, _ permissions.Viewer) (*calendar.Moon, error) {
	f.dels = append(f.dels, d)
	return nil, apperror.NewNotFound("no mark on that day")
}

func TestMoonKind(t *testing.T) {
	mark := func(fields map[string]any) any { return fields }
	tests := []struct {
		name    string
		actor   Actor
		mode    string // the moon's mode before
		action  string
		fields  map[string]any
		wantErr string
		check   func(t *testing.T, f *fakeMarks)
	}{
		{"set by hand with marks", owner, calendar.MoonCycle, ActionUpdate, map[string]any{
			"moves": "by_hand", "words": map[string]any{"vanished": "Gone below"},
			"marks": []any{
				mark(map[string]any{"year": 1492, "month": "Hammer", "day": 3, "state": "Full"}),
				mark(map[string]any{"year": 1492, "month": 1, "day": 9, "state": "vanished", "players_see": true}),
				mark(map[string]any{"year": 1492, "month": 2, "day": 1, "remove": true}),
			}}, "", func(t *testing.T, f *fakeMarks) {
			if len(f.modes) != 1 || f.modes[0] != calendar.MoonManual || f.names[0]["vanished"] != "Gone below" {
				t.Errorf("mode %v names %v", f.modes, f.names)
			}
			if len(f.puts) != 2 || f.puts[0].State != calendar.MoonFull || f.puts[0].Month != 1 || !f.puts[1].Shown || f.puts[0].Shown {
				t.Errorf("puts %+v", f.puts)
			}
			// Removing a mark that is already gone is not an error.
			if len(f.dels) != 1 {
				t.Errorf("dels %v", f.dels)
			}
		}},
		{"words keep the mode", owner, calendar.MoonManual, ActionUpdate, map[string]any{"words": map[string]any{"full": "Bright"}}, "", func(t *testing.T, f *fakeMarks) {
			if len(f.modes) != 1 || f.modes[0] != calendar.MoonManual {
				t.Errorf("mode %v", f.modes)
			}
		}},
		{"marks on a cycling moon", owner, calendar.MoonCycle, ActionUpdate, map[string]any{"marks": []any{mark(map[string]any{"year": 1492, "month": 1, "day": 3, "state": "full"})}}, "add `moves: by_hand`", nil},
		{"unknown state", owner, calendar.MoonManual, ActionUpdate, map[string]any{"marks": []any{mark(map[string]any{"year": 1492, "month": 1, "day": 3, "state": "gibbous"})}}, "state is full", nil},
		{"unknown mark key", owner, calendar.MoonManual, ActionUpdate, map[string]any{"marks": []any{mark(map[string]any{"year": 1492, "month": 1, "day": 3, "state": "full", "shown": true})}}, "not a mark key", nil},
		{"a day twice", owner, calendar.MoonManual, ActionUpdate, map[string]any{"marks": []any{
			mark(map[string]any{"year": 1492, "month": 1, "day": 3, "state": "full"}), mark(map[string]any{"year": 1492, "month": 1, "day": 3, "state": "dark"})}}, "listed twice", nil},
		{"unknown word state", owner, calendar.MoonManual, ActionUpdate, map[string]any{"words": map[string]any{"gibbous": "x"}}, "not a state", nil},
		{"bad moves", owner, calendar.MoonManual, ActionUpdate, map[string]any{"moves": "sideways"}, "by_hand", nil},
		{"nothing to do", owner, calendar.MoonManual, ActionUpdate, map[string]any{}, "say what to change", nil},
		{"create", owner, calendar.MoonManual, ActionCreate, map[string]any{"moves": "by_hand"}, "only takes action: update", nil},
		{"player", player, calendar.MoonManual, ActionUpdate, map[string]any{"moves": "cycle"}, "owner or a co-DM", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCal()
			c.cal.Moons = []calendar.Moon{{ID: 7, Name: "Umbra", CycleDays: 30, PhaseMode: tt.mode}}
			f := &fakeMarks{}
			k := MoonKind{Cal: c, Marks: f}
			r := rec("moon", tt.action, "umbra", tt.fields, "")
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
			tt.check(t, f)
		})
	}
}

func TestMoonKindRefusesRealWorld(t *testing.T) {
	c := newCal()
	c.cal.Mode = calendar.ModeRealLife
	c.cal.Moons = []calendar.Moon{{ID: 1, Name: "Moon", CycleDays: 29.5}}
	k := MoonKind{Cal: c, Marks: &fakeMarks{}}
	if p := k.Plan(context.Background(), camp, owner, rec("moon", ActionUpdate, "Moon", map[string]any{"moves": "by_hand"}, "")); !strings.Contains(p.Error, "real Moon") {
		t.Fatalf("plan error %q", p.Error)
	}
}
