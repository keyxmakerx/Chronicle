package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// The plan is served as JSON, so the entry key a rule names must survive the
// conversion from the systems list: the hero builder matches rules by it.
func TestHeroChoicesServeKey(t *testing.T) {
	in := []systems.Choice{
		{Key: "fury", Slug: "fury", Name: "Fury", Source: systems.ChoiceSourcePackage},
		{Key: "12", Slug: "mothfolk", Name: "Mothfolk", Source: systems.ChoiceSourceCampaign},
	}
	raw, err := json.Marshal(heroChoices(in))
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"fury", "12"} {
		if got[i]["key"] != want {
			t.Errorf("choice %d: key = %v, want %q", i, got[i]["key"], want)
		}
	}
}

// planSpy is a choice service that records the context the plan was read
// with and answers a plan full of rule detail.
type planSpy struct {
	systems.CharacterChoiceService
	ctx context.Context
}

func (p *planSpy) CreatorPlan(ctx context.Context, _ string) (*systems.CreatorPlan, error) {
	p.ctx = ctx
	return &systems.CreatorPlan{SystemID: "sys", SystemName: "Test", Steps: []systems.CreatorStep{{
		FieldKey: "class", Label: "Class", CanMake: true,
		Choices:  []systems.Choice{{Key: "conduit", Name: "Conduit", Source: systems.ChoiceSourcePackage}},
		Excluded: []systems.Choice{{Key: "fury", Name: "Fury", Source: systems.ChoiceSourcePackage}},
		Rules:    &systems.FieldRules{Limited: true, NotHere: []systems.NotHere{{Name: "Fury", Reason: "Too many"}}},
	}}}, nil
}

// A visitor's plan is read as a visitor (the context says so, whatever it
// came with) and is copied across as lists of entries only: no rules, no held
// entries, no offer to make your own.
func TestVisitorPlanCopiesEntriesOnly(t *testing.T) {
	spy := &planSpy{}
	director := systems.WithChoiceViewer(context.Background(), true)
	plan, err := heroPlannerAdapter{choices: spy}.VisitorPlan(director, "camp")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || len(plan.Steps[0].Choices) != 1 || plan.Steps[0].Choices[0].Name != "Conduit" {
		t.Fatalf("plan = %+v", plan)
	}
	st := plan.Steps[0]
	if st.Rules != nil || len(st.Held) != 0 || st.CanMake {
		t.Errorf("step carries rule detail: %+v", st)
	}
	if systems.ChoiceViewerIsDirector(spy.ctx) || systems.ChoiceViewerUser(spy.ctx) != "" {
		t.Error("the plan was read as someone other than a visitor")
	}
}

// termsSpy answers the rules words a system reads from its glossary.
type termsSpy struct {
	systems.CharacterChoiceService
	terms *systems.RulesTerms
}

func (s termsSpy) RulesTerms(context.Context, string) (*systems.RulesTerms, error) {
	return s.terms, nil
}

// The builder's words cross from the systems package to the entities plugin
// intact, and the served shape names only name, category and text.
func TestHeroTermsAdapterCopiesWords(t *testing.T) {
	spy := termsSpy{terms: &systems.RulesTerms{Present: true, Terms: map[string]systems.RulesTerm{
		"save-ends": {Name: "Save Ends", Category: "duration", Text: "Roll at end of turn."},
	}}}
	got, err := heroTermsAdapter{choices: spy}.HeroTerms(context.Background(), "camp")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Present || len(got.Terms) != 1 || got.Terms["save-ends"].Name != "Save Ends" || got.Terms["save-ends"].Category != "duration" || got.Terms["save-ends"].Text != "Roll at end of turn." {
		t.Fatalf("terms = %+v", got)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"terms":{"save-ends":{"name":"Save Ends","category":"duration","text":"Roll at end of turn."}}}` {
		t.Errorf("wire = %s", raw)
	}
}

// shapedSpy answers a plan whose steps carry a system's declarations.
type shapedSpy struct{ systems.CharacterChoiceService }

func (shapedSpy) CreatorPlan(context.Context, string) (*systems.CreatorPlan, error) {
	return &systems.CreatorPlan{Steps: []systems.CreatorStep{{
		FieldKey: "subclass", Label: "Subclass", Needed: true, Parent: "class",
		Choices:   []systems.Choice{{Key: "fury--reaver", Name: "Reaver", Parent: "Fury"}},
		CountFrom: []systems.CreatorCount{{Step: "class", Property: "kits", Note: "kit_note"}},
	}}}, nil
}

// The needed flag, the parent step, the counts and each entry's parent reach
// the creator for a player and for a visitor alike; none of it holds a secret.
func TestPlansCarryStepDeclarations(t *testing.T) {
	a := heroPlannerAdapter{choices: shapedSpy{}}
	player, err := a.HeroPlan(context.Background(), "camp", "u1", false)
	if err != nil {
		t.Fatal(err)
	}
	visitor, err := a.VisitorPlan(context.Background(), "camp")
	if err != nil {
		t.Fatal(err)
	}
	for name, plan := range map[string]*entities.HeroPlan{"player": player, "visitor": visitor} {
		st := plan.Steps[0]
		if !st.Needed || st.Parent != "class" || len(st.CountFrom) != 1 || st.CountFrom[0].Note != "kit_note" || st.Choices[0].Parent != "Fury" {
			t.Errorf("%s plan dropped a declaration: %+v", name, st)
		}
	}
}
