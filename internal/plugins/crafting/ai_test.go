package crafting

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

// templateFixture is shared with test/js/crafting_lib.test.mjs, so the
// server lays a method's steps exactly as the editor's template picker.
const templateFixture = "../../../test/js/fixtures/craft_template_steps.json"

type templateCase struct {
	Name     string              `json:"name"`
	Tags     map[string][]string `json:"tags"`
	Template string              `json:"template"`
	In       RecipeInput         `json:"in"`
	Want     RecipeInput         `json:"want"`
}

func templateCases() []templateCase {
	return []templateCase{
		{Name: "items whose tags fit", Template: "steady", Tags: map[string][]string{"moss": {"herb"}, "water": {"liquid"}},
			In: RecipeInput{Materials: []Material{{ItemID: "moss", Qty: 2}, {ItemID: "water", Qty: 1}}}},
		{Name: "nothing fits, so any lines are added", Template: "volatile", Tags: map[string][]string{"ore": {"metal"}},
			In: RecipeInput{Materials: []Material{{ItemID: "ore", Qty: 1}}}},
		{Name: "an any line and an untagged item", Template: "steady", Tags: map[string][]string{},
			In: RecipeInput{Materials: []Material{{ItemID: "plain", Qty: 1}, {Tag: "bone", Qty: 3}}}},
		{Name: "the first fitting material is used twice", Template: "volatile", Tags: map[string][]string{"moss": {"herb", "liquid"}},
			In: RecipeInput{Materials: []Material{{ItemID: "moss", Qty: 1}}}},
	}
}

func TestLayTemplateMatchesTheEditor(t *testing.T) {
	cases := templateCases()
	if os.Getenv("CRAFTING_WRITE_TEMPLATE_FIXTURE") == "1" {
		for i := range cases {
			cases[i].Want, _ = LayTemplate(cases[i].In, cases[i].Template, cases[i].Tags)
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", " ")
		if err := enc.Encode(map[string]any{
			"about":     "A method's steps laid over a recipe, from internal/plugins/crafting/ai_test.go (CRAFTING_WRITE_TEMPLATE_FIXTURE=1). The server (Go LayTemplate) and the editor (crafting_lib.js applyTemplate) must both give want.",
			"templates": Templates, "cases": cases,
		}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(templateFixture, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(templateFixture)
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Cases []templateCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	if len(fx.Cases) != len(cases) {
		t.Fatalf("the fixture has %d cases, the test %d; rewrite it", len(fx.Cases), len(cases))
	}
	for _, c := range fx.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, ok := LayTemplate(c.In, c.Template, c.Tags)
			if !ok || !reflect.DeepEqual(got, c.Want) {
				t.Fatalf("laid\n%+v\nwant\n%+v", got, c.Want)
			}
		})
	}
	if _, ok := LayTemplate(RecipeInput{}, "forge", nil); ok {
		t.Fatal("an unknown method was laid")
	}
}

func TestKeepStepMaterials(t *testing.T) {
	old := []Material{{ItemID: "moss", Qty: 1}, {Tag: "liquid", Qty: 1}, {ItemID: "bone", Qty: 1}}
	steps := []Step{{T: "add", Material: idx(0), To: "mortar"}, {T: "grind", Turns: 3}, {T: "pour", Material: idx(1)}, {T: "add", Material: idx(2), To: "flask"}, {T: "add", Material: idx(9)}}
	// The liquid line moves to the front and the bone goes.
	got := KeepStepMaterials(old, steps, []Material{{Tag: "liquid", Qty: 2}, {ItemID: "moss", Qty: 1}})
	want := []*int{idx(1), nil, idx(0), nil, nil}
	for i, s := range got {
		if (s.Material == nil) != (want[i] == nil) || (s.Material != nil && *s.Material != *want[i]) {
			t.Errorf("step %d points at %v, want %v", i, s.Material, want[i])
		}
	}
	if got[1].Turns != 3 || got[0].To != "mortar" || *steps[0].Material != 0 {
		t.Fatal("a step lost its setting, or the stored steps changed")
	}
}

func TestSaveAIRecipe(t *testing.T) {
	s, repo, _ := newTestService()
	ctx := context.Background()
	repo.tags = map[string][]string{"moss": {"herb"}, "water": {"liquid"}}
	in := AIRecipe{Name: "Healing potion", Method: "steady", Materials: []Material{{ItemID: "moss", Qty: 2}, {ItemID: "water", Qty: 1}},
		Result: Result{ItemID: "potion", Qty: 1}, Goal: 30, Stat: "Reason", Cost: Cost{Amount: 25, Currency: "gp"}}

	if _, err := s.SaveAIRecipe(ctx, "c", player, in, false); code(err) != http.StatusForbidden {
		t.Fatalf("a player: %v", err)
	}
	if _, err := s.SaveAIRecipe(ctx, "c", director, in, true); err != nil || len(repo.recipes) != 0 {
		t.Fatalf("a check saved, or refused: %v", err)
	}
	r, err := s.SaveAIRecipe(ctx, "c", director, in, false)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Draft || r.Template != "steady" || len(r.Steps) != 5 || *r.Steps[0].Material != 0 || *r.Steps[3].Material != 1 || r.Stat != "reason" {
		t.Fatalf("created %+v", r)
	}

	// The name is taken whatever its case, at review and at save.
	dup := in
	dup.Name = "HEALING POTION"
	if _, err := s.SaveAIRecipe(ctx, "c", director, dup, true); code(err) != http.StatusConflict {
		t.Fatalf("a check let a taken name through: %v", err)
	}

	// The Director approves it and writes rules and colours of their own.
	if _, err := s.Approve(ctx, "c", r.ID, director); err != nil {
		t.Fatal(err)
	}
	stored := repo.recipes[r.ID]
	stored.Rules, stored.Colors = []Rule{{When: []Cond{{Kind: "always"}}, Then: Effect{Kind: "edge"}}}, Colors{Final: "#ff0000"}
	repo.recipes[r.ID] = stored

	// An update with no method keeps the steps, each on the same material,
	// and keeps the recipe approved with its rules and colours.
	up := in
	up.ID, up.Method, up.Goal = r.ID, "", 40
	up.Materials = []Material{{ItemID: "water", Qty: 1}, {ItemID: "moss", Qty: 3}}
	got, err := s.SaveAIRecipe(ctx, "c", director, up, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Draft || got.Goal != 40 || *got.Steps[0].Material != 1 || *got.Steps[3].Material != 0 || len(got.Rules) != 1 || got.Colors.Final != "#ff0000" || got.Template != "steady" {
		t.Fatalf("updated %+v", got)
	}

	// A method on update lays its steps over the new materials.
	up.Method = "volatile"
	if got, err = s.SaveAIRecipe(ctx, "c", director, up, false); err != nil || got.Template != "volatile" || got.Steps[len(got.Steps)-1].T != "strike" {
		t.Fatalf("relaid %+v, %v", got, err)
	}

	bad := in
	bad.Name, bad.Method = "Other", "forge"
	if _, err := s.SaveAIRecipe(ctx, "c", director, bad, true); err == nil || !strings.Contains(err.Error(), `no method "forge"`) {
		t.Fatalf("an unknown method: %v", err)
	}
	bad.Method, bad.Result.ItemID = "", "nobody"
	if _, err := s.SaveAIRecipe(ctx, "c", director, bad, true); err == nil || !strings.Contains(err.Error(), "not an Armory item") {
		t.Fatalf("a missing item: %v", err)
	}
	if _, err := s.SaveAIRecipe(ctx, "c", director, AIRecipe{ID: "gone", Name: "x"}, true); code(err) != http.StatusNotFound {
		t.Fatalf("a missing recipe: %v", err)
	}
}

func TestCheckDrops(t *testing.T) {
	fx := newSpoilFx()
	ctx := context.Background()
	good := DropInput{Lines: []DropLine{{ItemID: "moss", Mode: DropTake, Qty: 1}}}
	if err := fx.s.CheckDrops(ctx, "c", "goblin", director, good); err != nil {
		t.Fatal(err)
	}
	if _, ok := fx.sp.drops["goblin"]; ok {
		t.Fatal("a check saved")
	}
	if err := fx.s.CheckDrops(ctx, "c", "goblin", player, good); code(err) != http.StatusForbidden {
		t.Fatalf("a player: %v", err)
	}
	if err := fx.s.CheckDrops(ctx, "c", "nowhere", director, good); code(err) != http.StatusNotFound {
		t.Fatalf("no page: %v", err)
	}
	bad := DropInput{Lines: []DropLine{{ItemID: "moss", Mode: DropHarvest, Stat: "reason", Yields: []int{0, 1, 2}}}}
	if err := fx.s.CheckDrops(ctx, "c", "goblin", director, bad); err == nil || !strings.Contains(err.Error(), "lowest at least 1") {
		t.Fatalf("a harvest paying 0 at the bottom: %v", err)
	}
}
