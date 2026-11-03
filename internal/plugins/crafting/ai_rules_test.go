package crafting

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMergeAIRules(t *testing.T) {
	full := Rule{When: []Cond{{Kind: "moon", Moon: "The Lantern", Phase: "full"}}, Then: Effect{Kind: "edge"}, Players: "book",
		Look: &Look{Spots: map[string]LookSpot{"window": {Look: "dawn"}}}}
	storm := Rule{When: []Cond{{Kind: "weather", Weather: "Storm"}}, Then: Effect{Kind: "cannot"}, Players: "secret", Draft: true}
	stored := []Rule{full, storm}

	// The full-moon rule comes back as the lookup printed it: kept, look and
	// approval included. The storm rule is left out, and a new one arrives.
	raw := json.RawMessage(`[` + string(AIRuleData(full)) + `,
		{"when": {"kind": "time", "time": "NIGHT"}, "then": {"kind": "points", "amount": 3}, "chance": "1 in 4", "players": "shown"}]`)
	rc, err := MergeAIRules(stored, raw)
	if err != nil {
		t.Fatal(err)
	}
	if rc.Kept != 1 || rc.New != 1 || rc.Gone != 1 || len(rc.Rules) != 2 {
		t.Fatalf("counts %+v", rc)
	}
	if got := rc.Rules[0]; got.Draft || got.Look == nil || got.Look.Spots["window"].Look != "dawn" {
		t.Fatalf("the kept rule lost its approval or look: %+v", got)
	}
	if got := rc.Rules[1]; !got.Draft || got.When[0].Time != "night" || got.Then.Amount != 3 || got.Chance != "1 in 4" || got.Players != "whenItHappens" {
		t.Fatalf("the new rule: %+v", got)
	}

	// Sent back with a change, a rule is new again and waits for approval;
	// a draft sent back unchanged stays a draft.
	changed := full
	changed.Then = Effect{Kind: "bane"}
	rc, err = MergeAIRules(stored, json.RawMessage(`[`+string(AIRuleData(changed))+`,`+string(AIRuleData(storm))+`]`))
	if err != nil || rc.Kept != 1 || rc.New != 1 || rc.Gone != 1 || !rc.Rules[0].Draft || !rc.Rules[1].Draft {
		t.Fatalf("a changed rule: %+v, %v", rc, err)
	}

	// An empty list clears every rule; null is the same.
	for _, empty := range []string{`[]`, `null`} {
		if rc, err := MergeAIRules(stored, json.RawMessage(empty)); err != nil || len(rc.Rules) != 0 || rc.Gone != 2 {
			t.Fatalf("%s: %+v, %v", empty, rc, err)
		}
	}

	// The edges of every range, and the forms the editor reads, still pass.
	edges := `[{"when": [{"kind": "always"}], "then": {"kind": "points", "amount": -10}},
		{"when": [{"kind": "always"}], "then": {"kind": "points", "amount": "10"}, "chance": "1 in 20"},
		{"when": [{"kind": "always"}], "then": {"kind": "points"}, "chance": "always"},
		{"when": [{"kind": "always"}], "then": {"kind": "extra", "amount": 3}, "players": "secret"},
		{"when": [{"kind": "moon", "moon": "The Lantern", "phase": "Waxing"}, {"kind": "time", "time": "Dusk"}], "then": {"kind": "only"}, "chance": "always"}]`
	if rc, err := MergeAIRules(nil, json.RawMessage(edges)); err != nil || rc.New != 5 {
		t.Fatalf("values in range: %+v, %v", rc, err)
	}

	refusals := []struct{ raw, want string }{
		{`{"when": []}`, "rules are a list"},
		{`[{"when": [{"kind": "moon"}], "then": {"kind": "fly"}}]`, "rule 1: `then` needs a `kind` of only, cannot, edge"},
		{`[{"when": [{"kind": "always"}], "then": {"kind": "edge"}}, {"when": [{"kind": "tide"}], "then": {"kind": "edge"}}]`, "rule 2: `when` needs at least one condition with a `kind` of moon, event"},
		{`["edge on a full moon"]`, "rule 1 is not a rule"},
		// A value the editor would clamp, drop or replace is refused, not changed.
		{`[{"when": [{"kind": "always"}], "then": {"kind": "points", "amount": 50}}]`, "rule 1: a `points` effect's `amount` is a whole number from -10 to 10, not 0; 50 is outside that"},
		{`[{"when": [{"kind": "always"}], "then": {"kind": "points", "amount": 0}}]`, "not 0; 0 is outside"},
		{`[{"when": [{"kind": "always"}], "then": {"kind": "points", "amount": 2.5}}]`, "2.5 is outside"},
		{`[{"when": [{"kind": "always"}], "then": {"kind": "points", "amount": "lots"}}]`, "`amount` \"lots\" isn't a number"},
		{`[{"when": [{"kind": "always"}], "then": {"kind": "extra", "amount": 4}}]`, "from 1 to 3; 4 is outside"},
		{`[{"when": [{"kind": "always"}], "then": {"kind": "edge"}, "chance": "1 in 7"}]`, "rule 1: `chance` \"1 in 7\" isn't one Chronicle rolls"},
		{`[{"when": [{"kind": "always"}], "then": {"kind": "edge"}, "chance": 7}]`, "`chance` 7 isn't one"},
		{`[{"when": [{"kind": "season", "season": "Winter"}], "then": {"kind": "only"}, "chance": "1 in 2"}]`, "an `only` rule has no `chance`"},
		{`[{"when": [{"kind": "always"}], "then": {"kind": "edge"}, "players": "everyone"}]`, "`players` \"everyone\" isn't book"},
		{`[{"when": [{"kind": "moon", "moon": "The Lantern", "phase": "gibbous"}], "then": {"kind": "edge"}}]`, "`phase` \"gibbous\" isn't a moon phase"},
		{`[{"when": [{"kind": "time", "time": "noon"}], "then": {"kind": "edge"}}]`, "`time` \"noon\" isn't dawn, day, dusk or night"},
		{`[{"when": [{"kind": "always"}, {"kind": "tide"}], "then": {"kind": "edge"}}]`, "rule 1: condition 2: `kind` \"tide\" isn't a condition"},
		{`[{"when": [{"kind": "time", "time": "night"}, {"kind": "time", "time": "dawn"}, {"kind": "time", "time": "dusk"}, {"kind": "time", "time": "day"}], "then": {"kind": "edge"}}]`, "`when` has 4 conditions; a rule takes at most 3"},
		{`[{"when": [{"kind": "weather", "weather": "` + strings.Repeat("a", maxRuleText+1) + `"}], "then": {"kind": "edge"}}]`, "`weather` is 121 characters; it holds at most 120"},
		{`[{"when": [{"kind": "always"}], "then": {"kind": "say", "text": "` + strings.Repeat("a", maxRuleLine+1) + `"}}]`, "`text` is 241 characters"},
		{`[` + strings.Repeat(`{"when":[{"kind":"always"}],"then":{"kind":"edge"}},`, maxRules) + `{"when":[{"kind":"always"}],"then":{"kind":"edge"}}]`, "at most 40 rules"},
	}
	for _, r := range refusals {
		if _, err := MergeAIRules(stored, json.RawMessage(r.raw)); err == nil || !strings.Contains(err.Error(), r.want) {
			t.Errorf("%s: %v, want %q", r.raw, err, r.want)
		}
	}
}

func TestSaveAIRecipeRules(t *testing.T) {
	s, repo, _ := newTestService()
	ctx := context.Background()
	repo.tags = map[string][]string{"moss": {"herb"}}
	in := AIRecipe{Name: "Moon tonic", Materials: []Material{{ItemID: "moss", Qty: 1}}, Result: Result{ItemID: "potion", Qty: 1}, Goal: 10,
		Rules: json.RawMessage(`[{"when": [{"kind": "moon", "moon": "The Lantern", "phase": "full"}], "then": {"kind": "edge"}}]`)}

	// A check reads the rules too, and refuses one it can't.
	bad := in
	bad.Rules = json.RawMessage(`[{"when": [{"kind": "moon"}]}]`)
	if _, err := s.SaveAIRecipe(ctx, "c", director, bad, true); err == nil || !strings.Contains(err.Error(), "rule 1") || len(repo.recipes) != 0 {
		t.Fatalf("an unreadable rule: %v", err)
	}

	r, err := s.SaveAIRecipe(ctx, "c", director, in, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rules) != 1 || !r.Rules[0].Draft || r.Rules[0].Players != "book" {
		t.Fatalf("a new recipe's rules: %+v", r.Rules)
	}

	// The Director approves the rule and gives it a look; the AI then sends
	// it back unchanged beside a new one, and changes the goal.
	stored := repo.recipes[r.ID]
	stored.Rules[0].Draft = false
	stored.Rules[0].Look = &Look{Spots: map[string]LookSpot{}, Words: "Moonlit"}
	repo.recipes[r.ID] = stored
	up := in
	up.ID, up.Goal = r.ID, 12
	up.Rules = json.RawMessage(`[` + string(AIRuleData(stored.Rules[0])) + `, {"when": [{"kind": "season", "season": "Winter"}], "then": {"kind": "cannot"}}]`)
	got, err := s.SaveAIRecipe(ctx, "c", director, up, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Goal != 12 || len(got.Rules) != 2 || got.Rules[0].Draft || got.Rules[0].Look == nil || got.Rules[0].Look.Words != "Moonlit" || !got.Rules[1].Draft {
		t.Fatalf("updated rules: %+v", got.Rules)
	}

	// Without rules, an update keeps them.
	up.Rules, up.Goal = nil, 14
	if got, err = s.SaveAIRecipe(ctx, "c", director, up, false); err != nil || len(got.Rules) != 2 {
		t.Fatalf("kept rules: %+v, %v", got, err)
	}
}

// An update that changes the materials and sends rules keeps the substitutes
// following their materials: the rules' own write must not put back the
// stored, unfollowed extras.
func TestSaveAIRecipeRulesKeepFollowedSubstitutes(t *testing.T) {
	s, repo, _ := newTestService()
	ctx := context.Background()
	in := AIRecipe{Name: "Spring tonic", Materials: []Material{{ItemID: "moss", Qty: 1}, {ItemID: "water", Qty: 1}}, Result: Result{ItemID: "potion", Qty: 1}, Goal: 10}
	r, err := s.SaveAIRecipe(ctx, "c", director, in, false)
	if err != nil {
		t.Fatal(err)
	}
	stored := repo.recipes[r.ID]
	stored.Extras.Substitutes = []Substitute{{For: 1, Use: "rainwater"}}
	repo.recipes[r.ID] = stored

	up := in
	up.ID = r.ID
	up.Materials = []Material{{ItemID: "water", Qty: 1}}
	up.Rules = json.RawMessage(`[{"when": [{"kind": "season", "season": "Winter"}], "then": {"kind": "cannot"}}]`)
	if _, err := s.SaveAIRecipe(ctx, "c", director, up, false); err != nil {
		t.Fatal(err)
	}
	subs := repo.recipes[r.ID].Extras.Substitutes
	if len(subs) != 1 || subs[0].For != 0 || subs[0].Use != "rainwater" {
		t.Fatalf("substitutes after materials and rules changed together: %+v", subs)
	}
}
