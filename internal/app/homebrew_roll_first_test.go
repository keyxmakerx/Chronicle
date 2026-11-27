package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolls"
)

// fakeTable records the Table rolls it is asked for and answers a total.
type fakeTable struct {
	total int
	asked []rolls.TableInput
	by    []rolls.Viewer
}

func (f *fakeTable) Table(_ context.Context, _ string, v rolls.Viewer, in rolls.TableInput) (*rolls.View, error) {
	f.asked, f.by = append(f.asked, in), append(f.by, v)
	t := f.total
	return &rolls.View{ID: "roll-9", Total: &t}, nil
}

// fakePower declares Might and Agility; mira's hero has Might 2 and no
// Agility number.
type fakePower struct{ declared bool }

func (f fakePower) PowerRoll(_ context.Context, _, userID string, _ bool) (rolls.PowerRollLine, error) {
	if !f.declared {
		return rolls.PowerRollLine{}, nil
	}
	line := rolls.PowerRollLine{Declared: true, Characteristics: []rolls.PowerStat{{Key: "might", Label: "Might"}, {Key: "agility", Label: "Agility"}}}
	if userID == "mira" {
		two := 2
		line.HeroID, line.Characteristics[0].Value = "hero-mira", &two
	}
	return line, nil
}

func TestRollFirstCheck(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		declared bool
		want     string // stored args, "" for refused
	}{
		{"plain dice", `{"dice":"2D10 + 1","target":12}`, false, `{"dice":"2d10+1","target":12}`},
		{"a characteristic by key, stored as the system spells it", `{"dice":"2d10","characteristic":"might","target":12}`, true, `{"dice":"2d10","characteristic":"Might","target":12}`},
		{"a bad dice expression", `{"dice":"2d7","target":12}`, false, ""},
		{"no dice", `{"dice":"","target":12}`, false, ""},
		{"words for dice", `{"dice":"roll well","target":12}`, false, ""},
		{"a characteristic with no game system", `{"dice":"2d10","characteristic":"Might","target":12}`, false, ""},
		{"a characteristic the system lacks", `{"dice":"2d10","characteristic":"Luck","target":12}`, true, ""},
		{"no target", `{"dice":"2d10","target":0}`, false, ""},
		{"a target too high", `{"dice":"2d10","target":5000}`, false, ""},
		{"an unknown arg", `{"dice":"2d10","target":3,"total":20}`, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := rollFirstAction(&fakeTable{}, fakePower{declared: tt.declared})
			got, err := def.Check(context.Background(), "camp", json.RawMessage(tt.args))
			if tt.want == "" {
				if err == nil {
					t.Fatalf("took %s", got)
				}
				return
			}
			if err != nil || string(got) != tt.want {
				t.Fatalf("got %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func TestRollFirstRollsThroughTheTable(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		user    string
		formula string
		label   string
		hero    string
		refused bool
	}{
		{"the hero's Might is added", `{"dice":"2d10","characteristic":"Might","target":12}`, "mira", "2d10+2", "Might", "hero-mira", false},
		{"plain dice name the rule", `{"dice":"d20","target":12}`, "mira", "1d20", "Rule from Lucky draw", "", false},
		{"a hero with no number can't roll", `{"dice":"2d10","characteristic":"Agility","target":12}`, "mira", "", "", "", true},
		{"a player with no hero can't add Might", `{"dice":"2d10","characteristic":"Might","target":12}`, "tam", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tbl := &fakeTable{total: 15}
			def := rollFirstAction(tbl, fakePower{declared: true})
			act := homebrew.Act{Args: json.RawMessage(tt.args), By: homebrew.Viewer{UserID: tt.user, Name: "Mira"}, Note: "Rule from Lucky draw"}
			got, err := def.Rolls.Roll(context.Background(), "camp", act)
			if tt.refused {
				if err == nil || len(tbl.asked) != 0 {
					t.Fatalf("rolled %+v", got)
				}
				if _, err := def.Rolls.Offer(context.Background(), "camp", act); err == nil {
					t.Error("offered a roll that can't be made")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			in := tbl.asked[0]
			if in.Formula != tt.formula || in.Label != tt.label || in.CharacterID != tt.hero || in.Mode != rolls.ModePublic {
				t.Errorf("table input = %+v", in)
			}
			if tbl.by[0].UserID != tt.user || tbl.by[0].Name != "Mira" || tbl.by[0].Director {
				t.Errorf("rolled as %+v", tbl.by[0])
			}
			if got.Total != 15 || got.Target != 12 || got.Formula != tt.formula || got.RollID != "roll-9" {
				t.Errorf("rolled = %+v", got)
			}
		})
	}
}
