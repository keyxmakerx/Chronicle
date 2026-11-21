package rolls

import (
	"context"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

var drawSteelCrits = []systems.CritRule{{Dice: "2d10", Judge: "total", Succ: []int{19, 20}}}

type fakeCrits struct{ rules []systems.CritRule }

func (f fakeCrits) CritRules(context.Context, string) []systems.CritRule { return f.rules }

// d builds dice as sides:value pairs written like {10, 9}.
func d(pairs ...[2]int) []Die {
	out := make([]Die, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, Die{Sides: p[0], Value: p[1]})
	}
	return out
}

func crits(dice []Die) []string {
	out := make([]string, len(dice))
	for i, x := range dice {
		out[i] = x.Crit
	}
	return out
}

func TestMarkCrits(t *testing.T) {
	tests := []struct {
		name   string
		rules  []systems.CritRule
		dice   []Die
		groups [][2]int
		want   []string
	}{
		{"draw steel 9+10 is a crit on both dice", drawSteelCrits, d([2]int{10, 9}, [2]int{10, 10}), [][2]int{{0, 2}}, []string{"succ", "succ"}},
		{"draw steel 10+8 is not", drawSteelCrits, d([2]int{10, 10}, [2]int{10, 8}), [][2]int{{0, 2}}, []string{"", ""}},
		{"draw steel 1+1 has no fumble", drawSteelCrits, d([2]int{10, 1}, [2]int{10, 1}), [][2]int{{0, 2}}, []string{"", ""}},
		{"draw steel d20 beside the pair is no crit", drawSteelCrits, d([2]int{10, 10}, [2]int{10, 10}, [2]int{20, 20}), [][2]int{{0, 2}, {2, 3}}, []string{"succ", "succ", ""}},
		{"a pair split across two terms is not judged as a pair", drawSteelCrits, d([2]int{10, 10}, [2]int{10, 10}), [][2]int{{0, 1}, {1, 2}}, []string{"", ""}},
		{"three d10s are not a pair", drawSteelCrits, d([2]int{10, 10}, [2]int{10, 10}, [2]int{10, 10}), [][2]int{{0, 3}}, []string{"", "", ""}},
		{"default natural 20", systems.DefaultCritRules(), d([2]int{20, 20}), [][2]int{{0, 1}}, []string{"succ"}},
		{"default natural 1", systems.DefaultCritRules(), d([2]int{20, 1}), [][2]int{{0, 1}}, []string{"fail"}},
		{"default d6 six", systems.DefaultCritRules(), d([2]int{6, 6}), [][2]int{{0, 1}}, []string{""}},
		{"each judges every die by its own face", systems.DefaultCritRules(), d([2]int{20, 20}, [2]int{20, 1}, [2]int{20, 7}), [][2]int{{0, 3}}, []string{"succ", "fail", ""}},
		{"empty rules mean no crits", []systems.CritRule{}, d([2]int{20, 20}, [2]int{20, 1}), [][2]int{{0, 2}}, []string{"", ""}},
		{"an earlier rule that applies wins", []systems.CritRule{{Dice: "d20", Succ: []int{19}}, {Dice: "d20", Succ: []int{20}}}, d([2]int{20, 20}), [][2]int{{0, 1}}, []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markCrits(tt.rules, tt.dice, tt.groups)
			if got := crits(tt.dice); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("crits = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMarkCritsOverwritesClientClaims(t *testing.T) {
	dice := []Die{{Sides: 20, Value: 7, Crit: CritSucc}}
	markCrits(systems.DefaultCritRules(), dice, runGroups(dice))
	if dice[0].Crit != "" {
		t.Fatalf("a client's crit survived: %q", dice[0].Crit)
	}
}

func TestGroupers(t *testing.T) {
	terms, _, _ := parseFormula("2d10+3+1d20")
	if got, want := termGroups(terms), [][2]int{{0, 2}, {2, 3}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("termGroups = %v, want %v", got, want)
	}
	runs := runGroups(d([2]int{10, 1}, [2]int{10, 2}, [2]int{20, 3}, [2]int{10, 4}))
	if want := [][2]int{{0, 2}, {2, 3}, {3, 4}}; !reflect.DeepEqual(runs, want) {
		t.Fatalf("runGroups = %v, want %v", runs, want)
	}
}

// The service's roll func shows every die's highest face, which is a crit
// under the default rules and under Draw Steel's for a pair of d10s.
func TestServiceJudgesEveryPath(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		rules CritSource
		run   func(s *service) []Die
		want  []string
	}{
		{"table, draw steel pair and a d20", fakeCrits{drawSteelCrits}, func(s *service) []Die {
			v, err := s.Table(ctx, "c1", player, TableInput{Formula: "2d10+1d20"})
			if err != nil {
				t.Fatal(err)
			}
			return v.Dice
		}, []string{"succ", "succ", ""}},
		{"table, no source uses the default", nil, func(s *service) []Die {
			v, err := s.Table(ctx, "c1", player, TableInput{Formula: "1d20+1d6"})
			if err != nil {
				t.Fatal(err)
			}
			return v.Dice
		}, []string{"succ", ""}},
		{"table, a system with no crits", fakeCrits{[]systems.CritRule{}}, func(s *service) []Die {
			v, _ := s.Table(ctx, "c1", player, TableInput{Formula: "1d20"})
			return v.Dice
		}, []string{""}},
		{"foundry report ignores a client's crit", nil, func(s *service) []Die {
			in := ReportInput{Origin: OriginFoundry, OriginID: "m1", RollerName: "Jun", Formula: "1d20", Total: 7, Mode: ModePublic,
				Dice: []Die{{Sides: 20, Value: 7, Crit: CritSucc}}}
			if _, _, err := s.Report(ctx, "c1", in); err != nil {
				t.Fatal(err)
			}
			return s.repo.(*fakeRepo).rolls[0].Dice
		}, []string{""}},
		{"foundry report, draw steel pair", fakeCrits{drawSteelCrits}, func(s *service) []Die {
			in := ReportInput{Origin: OriginFoundry, OriginID: "m1", RollerName: "Jun", Formula: "2d10", Total: 19, Mode: ModePublic,
				Dice: d([2]int{10, 9}, [2]int{10, 10})}
			if _, _, err := s.Report(ctx, "c1", in); err != nil {
				t.Fatal(err)
			}
			return s.repo.(*fakeRepo).rolls[0].Dice
		}, []string{"succ", "succ"}},
		{"record from crafting, draw steel pair", fakeCrits{drawSteelCrits}, func(s *service) []Die {
			_, err := s.Record(ctx, "c1", LogInput{Source: SourceCrafting, Formula: "2d10", Total: 20, Dice: []Die{{Sides: 10, Value: 10, Crit: CritFail}, {Sides: 10, Value: 10}}})
			if err != nil {
				t.Fatal(err)
			}
			return s.repo.(*fakeRepo).rolls[0].Dice
		}, []string{"succ", "succ"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _, _ := newTestService()
			s.crits = tt.rules
			if got := crits(tt.run(s)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("crits = %q, want %q", got, tt.want)
			}
		})
	}
}
