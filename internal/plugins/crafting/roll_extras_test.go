package crafting

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRollProject_WhatBefalls(t *testing.T) {
	always := []Cond{{Kind: "always"}}
	tables := fakeTables{"Quirks": {"Smells of roses"}, "Mishaps": {"The flask cracks"}}
	cases := []struct {
		name     string
		rules    []Rule
		extras   Extras
		day      *Day
		sheet    fakeSheets
		points   int
		dice     []int
		work     string
		director bool
		code     int
		total    int
		made     []string // the project's notes for the item, in order
		note     string   // a substring of the roll's notes
		noNote   string   // a substring the notes must not hold
		mishap   string
		lost     bool
		result   string // the item the project makes
		brk      bool
	}{
		{name: "a quirk when it is made", points: 20, dice: []int{10, 10},
			rules: []Rule{{When: always, Then: Effect{Kind: "quirk", Table: "Quirks"}, Players: "book"}},
			work:  WorkSteady, total: 22, made: []string{"A quirk: Smells of roses"}, result: "potion"},
		{name: "no quirk before it is made", dice: []int{3, 4},
			rules: []Rule{{When: always, Then: Effect{Kind: "quirk", Table: "Quirks"}, Players: "book"}},
			work:  WorkSteady, total: 9},
		{name: "fine and marked", points: 20, dice: []int{10, 10},
			rules: []Rule{
				{When: always, Then: Effect{Kind: "fine"}, Players: "book"},
				{When: always, Then: Effect{Kind: "mark", Mark: "moonlit"}, Players: "book"},
			},
			work: WorkSteady, total: 22, made: []string{"Fine", "Marked: moonlit"}, result: "potion"},
		{name: "made as something else", points: 20, dice: []int{10, 10},
			rules: []Rule{{When: always, Then: Effect{Kind: "becomes", Item: "dragon's TEAR"}, Players: "book"}},
			work:  WorkSteady, total: 22, made: []string{"Made as dragon's TEAR"}, result: "secret"},
		{name: "made as an item the Armory lacks stays as written, and the Director hears", points: 20, dice: []int{10, 10}, director: true,
			rules: []Rule{{When: always, Then: Effect{Kind: "becomes", Item: "Phoenix ash"}, Players: "book"}},
			work:  WorkSteady, total: 22, result: "potion", note: "no item by that name"},
		{name: "a mishap rule", dice: []int{3, 4},
			rules: []Rule{{When: always, Then: Effect{Kind: "mishap", Table: "Mishaps"}, Players: "book"}},
			work:  WorkSteady, total: 9, mishap: "The flask cracks", noNote: "The flask cracks"},
		{name: "a missing table is the Director's to hear", dice: []int{3, 4}, director: true,
			rules: []Rule{{When: always, Then: Effect{Kind: "mishap", Table: "Gone"}, Players: "book"}},
			work:  WorkSteady, total: 9, note: "no Rolling Table called “Gone”"},
		{name: "a missing table is not the player's", dice: []int{3, 4},
			rules: []Rule{{When: always, Then: Effect{Kind: "mishap", Table: "Gone"}, Players: "book"}},
			work:  WorkSteady, total: 9, noNote: "Rolling Table"},
		{name: "the recipe's mishap chance comes up", dice: []int{3, 4, 1},
			extras: Extras{Mishaps: &Mishaps{Chance: "1 in 6", Table: "Mishaps"}},
			work:   WorkSteady, total: 9, mishap: "The flask cracks"},
		{name: "the recipe's mishap chance doesn't", dice: []int{3, 4, 2},
			extras: Extras{Mishaps: &Mishaps{Chance: "1 in 6", Table: "Mishaps"}},
			work:   WorkSteady, total: 9},
		{name: "botched work leaves a quirk", dice: []int{3, 4},
			extras: Extras{Botch: Botch{Costs: BotchQuirk, QuirkTable: "Quirks"}},
			work:   WorkBotched, total: 7, made: []string{"A quirk: Smells of roses"}, note: "The botch left a quirk"},
		{name: "botched work ruins the ingredients", dice: []int{3, 4},
			extras: Extras{Botch: Botch{Costs: BotchIngredients}},
			work:   WorkBotched, total: 7, lost: true},
		{name: "botched work that finishes keeps them", points: 25, dice: []int{3, 4},
			extras: Extras{Botch: Botch{Costs: BotchIngredients}},
			work:   WorkBotched, total: 7, result: "potion"},
		{name: "a breakthrough rolls again", dice: []int{10, 9, 3, 4},
			extras: Extras{Breakthrough: true},
			work:   WorkSteady, total: 28, brk: true},
		{name: "no breakthrough below the top", dice: []int{9, 9},
			extras: Extras{Breakthrough: true},
			work:   WorkSteady, total: 20},
		{name: "a night-only recipe at night", dice: []int{3, 4}, day: &Day{Time: "night"},
			rules: []Rule{{When: []Cond{{Kind: "time", Time: "night"}}, Then: Effect{Kind: "only"}, Players: "book"}},
			work:  WorkSteady, total: 9},
		{name: "a night-only recipe at noon", day: &Day{Time: "day"},
			rules: []Rule{{When: []Cond{{Kind: "time", Time: "night"}}, Then: Effect{Kind: "only"}, Players: "book"}},
			work:  WorkSteady, code: 409},
		{name: "a maker who knows alchemy", dice: []int{3, 4}, sheet: fakeSheets{"reason": 2, "Alchemy": -1},
			rules: []Rule{{When: []Cond{{Kind: "maker", Skill: "alchemy"}}, Then: Effect{Kind: "points", Amount: 3}, Players: "book"}},
			work:  WorkSteady, total: 12},
		{name: "a maker who doesn't", dice: []int{3, 4}, sheet: fakeSheets{"reason": 2, "smithing": -1},
			rules: []Rule{{When: []Cond{{Kind: "maker", Skill: "alchemy"}}, Then: Effect{Kind: "only"}, Players: "book"}},
			work:  WorkSteady, code: 409},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newProjectFx()
			fx.days.day = tc.day
			fx.s.projects.Tables = tables
			if tc.sheet != nil {
				fx.s.projects.Sheets = tc.sheet
			}
			pv := fx.start(t)
			r := fx.repo.recipes["heal"]
			r.Rules = tc.rules
			r.Extras = tc.extras
			if r.Extras.Botch.Costs == "" {
				r.Extras.Botch.Costs = BotchBane
			}
			fx.repo.recipes["heal"] = r
			fx.ready(pv.ID, 1)
			p := fx.store.projects[pv.ID]
			p.Points = tc.points
			fx.store.projects[pv.ID] = p
			fx.dice = tc.dice
			v := player
			if tc.director {
				v = director
			}
			res, err := fx.s.RollProject(context.Background(), "c", pv.ID, v, RollInput{Work: tc.work, Version: p.Version})
			if tc.code != 0 {
				if code(err) != tc.code {
					t.Fatalf("code = %d (%v), want %d", code(err), err, tc.code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			notes := strings.Join(res.Notes, " ")
			if res.Total != tc.total {
				t.Fatalf("total = %d, want %d; terms %q notes %q", res.Total, tc.total, termTexts(res), notes)
			}
			saved := fx.store.projects[pv.ID]
			if strings.Join(saved.Details.Made, " | ") != strings.Join(tc.made, " | ") {
				t.Fatalf("made = %q, want %q", saved.Details.Made, tc.made)
			}
			if tc.note != "" && !strings.Contains(notes, tc.note) {
				t.Fatalf("notes = %q, want %q", notes, tc.note)
			}
			if tc.noNote != "" && strings.Contains(notes, tc.noNote) {
				t.Fatalf("notes = %q, must not hold %q", notes, tc.noNote)
			}
			got := ""
			if res.Mishap != nil {
				got = res.Mishap.Text
			}
			if got != tc.mishap {
				t.Fatalf("mishap = %q, want %q", got, tc.mishap)
			}
			if res.Lost != tc.lost || (tc.lost && saved.Status != ProjectCancelled) {
				t.Fatalf("lost = %v, status %q", res.Lost, saved.Status)
			}
			if res.Breakthrough != tc.brk {
				t.Fatalf("breakthrough = %v, terms %q", res.Breakthrough, termTexts(res))
			}
			if tc.result != "" {
				if !res.Finished || fx.arm.given[tc.result] != 1 || saved.ResultItemID != tc.result {
					t.Fatalf("finished %v, given %v, want one %s", res.Finished, fx.arm.given, tc.result)
				}
			} else if res.Finished {
				t.Fatalf("finished, want not")
			}
		})
	}
}

func TestRollProject_Discovers(t *testing.T) {
	always := []Cond{{Kind: "always"}}
	cases := []struct {
		name     string
		elixir   *Recipe
		director bool
		note     string
		learned  bool
	}{
		{"a learned recipe", &Recipe{ID: "elixir", Name: "Elixir", Who: "learned"}, false, "Mira discovered how to make Elixir", true},
		{"a recipe everyone knows", &Recipe{ID: "elixir", Name: "Elixir", Who: "all"}, false, "which everyone already knows", false},
		{"a draft, to the Director", &Recipe{ID: "elixir", Name: "Elixir", Who: "learned", Draft: true}, true, "still a draft", false},
		{"no such recipe, to the Director", nil, true, "no recipe by that name", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newProjectFx()
			pv := fx.start(t)
			r := fx.repo.recipes["heal"]
			r.Rules = []Rule{{When: always, Then: Effect{Kind: "discover", Recipe: "elixir"}, Players: "book"}}
			fx.repo.recipes["heal"] = r
			if tc.elixir != nil {
				fx.repo.recipes["elixir"] = *tc.elixir
				fx.repo.order = append(fx.repo.order, "elixir")
			}
			fx.ready(pv.ID, 1)
			v := player
			if tc.director {
				v = director
			}
			res, err := fx.s.RollProject(context.Background(), "c", pv.ID, v, RollInput{Work: WorkSteady, Version: fx.store.projects[pv.ID].Version})
			if err != nil {
				t.Fatal(err)
			}
			if notes := strings.Join(res.Notes, " "); !strings.Contains(notes, tc.note) {
				t.Fatalf("notes = %q, want %q", notes, tc.note)
			}
			knows := false
			for _, k := range fx.repo.recipes["elixir"].Known {
				knows = knows || k == "mira"
			}
			if knows != tc.learned {
				t.Fatalf("learned = %v, want %v", knows, tc.learned)
			}
		})
	}
}

func TestRollProject_Mastery(t *testing.T) {
	for _, gives := range []string{"edge", "points", "skip"} {
		t.Run(gives, func(t *testing.T) {
			fx := newProjectFx()
			r := fx.repo.recipes["heal"]
			r.Extras = DefaultExtras()
			r.Extras.Mastery = &Mastery{AfterCleanBrews: 2, Gives: gives}
			fx.repo.recipes["heal"] = r
			set := DefaultSettings()
			set.Skip = false
			fx.repo.settings = &set
			pv := fx.start(t)
			fx.ready(pv.ID, 4)
			if _, err := fx.roll(t, pv.ID, WorkSkipped); err == nil {
				t.Fatalf("skipping before mastery: %v", err)
			}
			for i := 0; i < 2; i++ {
				fx.dice = []int{1, 1}
				if _, err := fx.roll(t, pv.ID, WorkClean); err != nil {
					t.Fatal(err)
				}
			}
			fx.dice = []int{1, 1}
			res, err := fx.roll(t, pv.ID, WorkSteady)
			if gives == "skip" {
				if err != nil {
					t.Fatal(err)
				}
				fx.ready(pv.ID, 1)
				if _, err := fx.roll(t, pv.ID, WorkSkipped); err != nil {
					t.Fatalf("a master may skip: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.ToLower(termTexts(res)), "mastery") {
				t.Fatalf("terms = %q, want mastery", termTexts(res))
			}
		})
	}
}

func TestStartProject_Substitute(t *testing.T) {
	for _, costs := range []string{"bane", "points", "none"} {
		t.Run(costs, func(t *testing.T) {
			fx := newProjectFx()
			r := fx.repo.recipes["heal"]
			r.Extras = DefaultExtras()
			r.Extras.Substitutes = []Substitute{{For: 0, Use: "herb", Costs: costs}}
			fx.repo.recipes["heal"] = r
			pv, err := fx.s.StartProject(context.Background(), "c", "mira", player, StartInput{RecipeID: "heal", Picks: map[int]string{0: "moss", 1: "moss"}})
			if err != nil {
				t.Fatal(err)
			}
			subs := fx.store.projects[pv.ID].Details.Subs
			if (costs == "none") != (len(subs) == 0) {
				t.Fatalf("subs = %q", subs)
			}
			fx.ready(pv.ID, 1)
			fx.dice = []int{3, 4}
			res, err := fx.roll(t, pv.ID, WorkSteady)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]int{"bane": 7, "points": 7, "none": 9}[costs]
			if res.Total != want {
				t.Fatalf("total = %d, want %d; terms %q", res.Total, want, termTexts(res))
			}
			if costs == "bane" && !strings.Contains(termTexts(res), "a substitute ingredient") {
				t.Fatalf("terms = %q, want the substitute's bane", termTexts(res))
			}
		})
	}
}

// The server keeps exactly the extras the rules editor keeps: every case in
// the shared fixture, whose answers come from craft_rules_logic.js itself.
func TestCleanExtrasMatchesTheEditor(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "js", "fixtures", "craft_extras_clean.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Cases []struct {
			Name      string          `json:"name"`
			Input     json.RawMessage `json:"input"`
			Materials int             `json:"materials"`
			Want      json.RawMessage `json:"want"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	if len(fx.Cases) == 0 {
		t.Fatal("the fixture has no cases")
	}
	for _, c := range fx.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, _ := json.Marshal(CleanExtras(c.Input, c.Materials))
			var g, w any
			_ = json.Unmarshal(got, &g)
			_ = json.Unmarshal(c.Want, &w)
			if g, w = dropEmpty(g), dropEmpty(w); !reflect.DeepEqual(g, w) {
				gj, _ := json.Marshal(g)
				wj, _ := json.Marshal(w)
				t.Errorf("server kept\n%s\neditor keeps\n%s", gj, wj)
			}
		})
	}
}

func TestCatalog_TablesAndSkills(t *testing.T) {
	fx := newProjectFx()
	fx.s.projects.Tables = fakeTables{"Quirks": {"x"}}
	fx.s.projects.Sheets = fakeSheets{"Alchemy": -1, "reason": 2}
	cat, err := fx.s.Catalog(context.Background(), "c", director)
	if err != nil {
		t.Fatal(err)
	}
	// Mira's and Tobin's sheets both list Alchemy; it is offered once.
	if len(cat.Tables) != 1 || cat.Tables[0].Name != "Quirks" || strings.Join(cat.Tables[0].Rows, ",") != "x" || strings.Join(cat.Skills, ",") != "Alchemy" {
		t.Fatalf("tables %q skills %q", cat.Tables, cat.Skills)
	}
}
