package crafting

import (
	"context"
	"encoding/json"
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
			work:  WorkSteady, total: 9, mishap: "The flask cracks", note: "A mishap mid-brew: The flask cracks"},
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
			work:   WorkBotched, total: 7, lost: true, note: "ruined the ingredients"},
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

func TestCleanExtras(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"nothing", ``, `{"botch":{"costs":"bane"},"mishaps":null,"quality":null,"substitutes":[],"breakthrough":false,"mastery":null,"discover":null,"keeps":null,"makersMark":false,"salvage":[],"othersCanHelp":false}`},
		{"not an object", `[1]`, `{"botch":{"costs":"bane"},"mishaps":null,"quality":null,"substitutes":[],"breakthrough":false,"mastery":null,"discover":null,"keeps":null,"makersMark":false,"salvage":[],"othersCanHelp":false}`},
		{"a quirk botch needs its table", `{"botch":{"costs":"baneAndQuirk"}}`, `{"botch":{"costs":"bane"}`},
		{"a quirk botch", `{"botch":{"costs":"baneAndQuirk","quirkTable":"Quirks"}}`, `{"botch":{"costs":"baneAndQuirk","quirkTable":"Quirks"}`},
		{"a lost-ingredients botch", `{"botch":{"costs":"baneAndIngredients","quirkTable":"x"}}`, `{"botch":{"costs":"baneAndIngredients"}`},
		{"mishaps never always", `{"mishaps":{"chance":"always","table":"M"}}`, `"mishaps":{"chance":"1 in 6","table":"M"}`},
		{"mishaps need a table", `{"mishaps":{"chance":"1 in 4"}}`, `"mishaps":null`},
		{"quality falls back", `{"quality":{"fine":"wild","poor":"points"}}`, `"quality":{"fine":"edge","poor":"points"}`},
		{"substitutes are bounded and unique", `{"substitutes":[{"for":0,"use":"herb","costs":"points"},{"for":0,"use":"herb"},{"for":5,"use":"herb"},{"for":1,"use":"nonsense"}]}`, `"substitutes":[{"for":0,"use":"herb","costs":"points"}]`},
		{"mastery snaps to a choice", `{"mastery":{"afterCleanBrews":4,"gives":"edge"}}`, `"mastery":{"afterCleanBrews":3,"gives":"edge"}`},
		{"discovery's closeness", `{"discover":{"closeWhen":"3 of 2","learnedBy":"party"}}`, `"discover":{"closeWhen":"1 of 2","learnedBy":"party","failedTry":"used"}`},
		{"keeps is clamped", `{"keeps":{"days":9999,"thenBecomes":"Sludge"}}`, `"keeps":{"days":360,"thenBecomes":"Sludge"}`},
		{"salvage", `{"salvage":[{"item":"Glass","qty":7},{"item":""}]}`, `"salvage":[{"item":"Glass","qty":2}]`},
		{"switches", `{"breakthrough":true,"makersMark":true,"othersCanHelp":"yes"}`, `"breakthrough":true`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(CleanExtras(json.RawMessage(tc.in), 2))
			if !strings.Contains(string(b), tc.want) {
				t.Fatalf("CleanExtras = %s, want it to hold %s", b, tc.want)
			}
			if tc.name == "switches" && (!strings.Contains(string(b), `"makersMark":true`) || !strings.Contains(string(b), `"othersCanHelp":false`)) {
				t.Fatalf("switches = %s", b)
			}
		})
	}
}
