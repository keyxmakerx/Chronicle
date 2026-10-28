package crafting

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// bookFixture is shared with test/js/crafting_lib.test.mjs: the player's
// book (bookPage, here) and the Director's "What players see" preview
// (CraftingLib.book) must turn the same recipe into the same page. Run with
// CRAFTING_WRITE_BOOK_FIXTURE=1 to rewrite want from this side after a
// deliberate wording change, then check the JS test still passes.
var bookFixture = filepath.Join("..", "..", "..", "test", "js", "fixtures", "craft_book_pages.json")

type bookCase struct {
	Name   string            `json:"name"`
	Recipe Recipe            `json:"recipe"`
	Items  map[string]string `json:"items"`
	Want   json.RawMessage   `json:"want"`
}

func bookCases() []bookCase {
	rules := []Rule{
		{When: []Cond{{Kind: "moon", Moon: "The Lantern", Phase: "full"}}, Then: Effect{Kind: "edge"}, Players: "book", BookLine: "Brew it under a full Lantern."},
		{When: []Cond{{Kind: "weather", Weather: "Rain"}, {Kind: "time", Time: "night"}}, Then: Effect{Kind: "points", Amount: -3}, Chance: "1 in 4", Players: "book"},
		{When: []Cond{{Kind: "always"}}, Then: Effect{Kind: "mishap", Table: "Spills"}, Players: "secret"},
		{When: []Cond{{Kind: "event", Event: "Harvest fair"}}, Then: Effect{Kind: "extra", Amount: 2}, Players: "whenItHappens"},
		{When: []Cond{{Kind: "season", Season: "Winter"}}, Then: Effect{Kind: "cannot"}, Players: "book", Draft: true},
	}
	every := []Rule{}
	for _, c := range []Cond{{Kind: "rolled", Event: "Tide", Result: "High"}, {Kind: "bench", Bench: "The forge"}, {Kind: "ingredient", Mark: "Moonlit"}, {Kind: "maker", Skill: "Alchemy"}} {
		every = append(every, Rule{When: []Cond{c}, Then: Effect{Kind: "fine"}, Players: "book"})
	}
	for _, e := range []Effect{{Kind: "only"}, {Kind: "cannot"}, {Kind: "bane"}, {Kind: "points", Amount: 4}, {Kind: "extra", Amount: 1}, {Kind: "becomes", Item: "Ashen draught"},
		{Kind: "mark", Mark: "Blessed"}, {Kind: "quirk", Table: "Quirks"}, {Kind: "discover", Recipe: "Frost tonic"}, {Kind: "say", Text: "It hums."}} {
		every = append(every, Rule{When: []Cond{{Kind: "always"}}, Then: e, Players: "book"})
	}
	return []bookCase{
		{Name: "a full recipe", Items: map[string]string{"moss": "Bloodmoss", "potion": "Healing potion"}, Recipe: Recipe{
			ID: "r1", Name: "Healing potion", Template: "steady", Bench: "apothecary",
			Materials: []Material{{ItemID: "moss", Qty: 1}, {Tag: "liquid", Qty: 2}},
			Steps:     []Step{{T: "add", Material: idx(0), To: "mortar"}, {T: "grind", Turns: 3}, {T: "tip"}, {T: "pour", Material: idx(1)}, {T: "simmer", Secs: 5}, {T: "strike", N: 2}},
			Result:    Result{ItemID: "potion", Qty: 1}, Goal: 30, Stat: "reason", Prerequisite: "Fresh bloodmoss", Source: "The herbalist's book",
			Cost: Cost{Amount: 25, Currency: "gp"}, Who: "all", Rules: rules}},
		{Name: "items the viewer cannot see", Items: map[string]string{}, Recipe: Recipe{
			ID: "r2", Name: "Caustic flask", Template: "volatile", Bench: "apothecary",
			Materials: []Material{{ItemID: "hidden", Qty: 3}, {Tag: "magic_essence", Qty: 1}},
			Steps:     []Step{{T: "add", Material: idx(0), To: "flask"}, {T: "pour", Material: idx(0)}, {T: "add"}, {T: "simmer", Secs: 10}, {T: "strike", N: 1}, {T: "grind", Turns: 12}},
			Result:    Result{ItemID: "hidden", Qty: 4}, Goal: 45, Stat: "Wisdom", Who: "all", Rules: every}},
		{Name: "nothing yet", Items: map[string]string{}, Recipe: Recipe{
			ID: "r3", Name: "Untitled", Bench: "apothecary", Materials: []Material{}, Steps: []Step{}, Result: Result{Qty: 1}, Goal: 10, Who: "all",
			Cost: Cost{Amount: 0, Currency: "gp"}}},
	}
}

func bookRules() systems.CraftingRules {
	r := systems.DefaultCraftingRules()
	r.Characteristics = []systems.CraftingStat{{Key: "reason", Label: "Reason"}}
	return r
}

func TestBookPagesMatchThePreview(t *testing.T) {
	cases := bookCases()
	if os.Getenv("CRAFTING_WRITE_BOOK_FIXTURE") == "1" {
		for i := range cases {
			p := bookPage(&cases[i].Recipe, cases[i].Items, bookRules())
			cases[i].Want, _ = json.Marshal(p)
		}
		lib := Library{Tags: Tags, Tools: Tools, Steps: StepKinds, BenchKinds: BenchKinds, Templates: Templates, Rules: bookRules()}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", " ")
		if err := enc.Encode(map[string]any{
			"about":   "Recipes and the book page each makes, from internal/plugins/crafting/book_test.go (CRAFTING_WRITE_BOOK_FIXTURE=1). The player's book (Go bookPage) and the Director's preview (crafting_lib.js book) must both give want.",
			"library": lib, "cases": cases,
		}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bookFixture, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(bookFixture)
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Cases []bookCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	if len(fx.Cases) != len(cases) {
		t.Fatalf("the fixture has %d cases, the test %d; rewrite it", len(fx.Cases), len(cases))
	}
	for _, c := range fx.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, _ := json.Marshal(bookPage(&c.Recipe, c.Items, bookRules()))
			var g, w any
			_ = json.Unmarshal(got, &g)
			_ = json.Unmarshal(c.Want, &w)
			if !reflect.DeepEqual(g, w) {
				t.Errorf("page\n%s\nwant\n%s", got, c.Want)
			}
		})
	}
}

func TestBookWords(t *testing.T) {
	tests := []struct{ got, want string }{
		{numberWord(3), "three"},
		{numberWord(12), "12"},
		{rollWords("", bookRules()), "2d10"},
		{rollWords("reason", bookRules()), "2d10 + Reason"},
		{rollWords("Wisdom", bookRules()), "2d10 + Wisdom"},
		{ruleSentence(Rule{When: []Cond{{Kind: "always"}}, Then: Effect{Kind: "points", Amount: 2}}), "Always: +2 project points."},
		{ruleSentence(Rule{When: []Cond{{Kind: "moon", Moon: "Pale", Phase: "full"}}, Then: Effect{Kind: "edge"}, Chance: "1 in 2"}), "When Pale is full: the project roll has an edge (1 in 2)."},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}
