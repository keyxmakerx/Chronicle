package entities

import (
	"strings"
	"testing"
)

// stepsPlan is a plan shaped like a system that declares a dependent step
// (subclass under class) and a counted one (kit); no real package is read.
func stepsPlan() *HeroPlan {
	plan := &HeroPlan{Steps: []HeroStep{
		{FieldKey: "class", Label: "Class", Choices: []HeroChoice{
			{Name: "Fury", Properties: map[string]any{"kits": float64(1)}},
			{Name: "Conduit", Properties: map[string]any{"kits": float64(0), "kit_note": "Conduits don't use a kit."}},
			{Name: "Beastheart", Properties: map[string]any{"kits": float64(1)}},
		}},
		{FieldKey: "subclass", Label: "Subclass", Parent: "class", Choices: []HeroChoice{
			{Name: "Berserker", Parent: "Fury", Properties: map[string]any{"kits": float64(0)}},
			{Name: "Reaver", Parent: "Fury"},
			{Name: "Light", Parent: "Conduit"},
		}},
		{FieldKey: "kit", Label: "Kit", CountFrom: []HeroCount{
			{Step: "class", Property: "kits", Note: "kit_note"},
			{Step: "subclass", Property: "kits", Note: "kit_note"},
		}, Choices: []HeroChoice{{Name: "Panther"}}},
	}}
	for i := range plan.Steps {
		for j := range plan.Steps[i].Choices {
			plan.Steps[i].Choices[j].Source = "package"
		}
	}
	return plan
}

func TestResolveHeroPicksDependentAndCounted(t *testing.T) {
	pick := func(kv ...string) map[string]HeroPick {
		m := map[string]HeroPick{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = HeroPick{Name: kv[i+1]}
		}
		return m
	}
	cases := []struct {
		name    string
		picks   map[string]HeroPick
		wantErr string
	}{
		{"subclass under its class", pick("class", "Fury", "subclass", "Reaver"), ""},
		{"parent matches by case", pick("class", "fury", "subclass", "reaver"), ""},
		{"subclass of another class", pick("class", "Conduit", "subclass", "Reaver"), `not a subclass of Conduit`},
		{"subclass with no class", pick("subclass", "Reaver"), "choose the class before the subclass"},
		{"subclass under a class that lists none", pick("class", "Beastheart", "subclass", "Reaver"), "A Beastheart doesn't use a subclass"},
		{"kit where the class takes one", pick("class", "Fury", "subclass", "Reaver", "kit", "Panther"), ""},
		{"kit before any parent is chosen", pick("kit", "Panther"), ""},
		{"kit on a class that uses none", pick("class", "Conduit", "kit", "Panther"), "A Conduit doesn't use a kit"},
		{"subclass overrides the class count", pick("class", "Fury", "subclass", "Berserker", "kit", "Panther"), "A Berserker doesn't use a kit"},
		{"a count of 0 with no pick is fine", pick("class", "Conduit"), ""},
		{"no kit pick under a needed count is fine", pick("class", "Fury"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveHeroPicks(stepsPlan(), tc.picks, false)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// Practice heroes are checked by the same function, with ask-first options on.
func TestResolveHeroPicksPracticeSameChecks(t *testing.T) {
	_, err := resolveHeroPicks(stepsPlan(), map[string]HeroPick{"class": {Name: "Conduit"}, "kit": {Name: "Panther"}}, true)
	if err == nil || !strings.Contains(err.Error(), "doesn't use a kit") {
		t.Fatalf("err = %v", err)
	}
}

// What the step needs never makes the server refuse a hero without it.
func TestBuildHeroFieldsAcceptsAnEmptyNeededStep(t *testing.T) {
	plan := stepsPlan()
	plan.Steps[0].Needed = true
	fields, _, err := buildHeroFields(plan, map[string]HeroPick{})
	if err != nil || len(fields) != 0 {
		t.Fatalf("fields = %v, err = %v", fields, err)
	}
}

func TestHeroAllowance(t *testing.T) {
	step := stepsPlan().Steps[2]
	chosen := func(m map[string]*HeroChoice) func(string) *HeroChoice {
		return func(k string) *HeroChoice { return m[k] }
	}
	fury := &HeroChoice{Name: "Fury", Properties: map[string]any{"kits": float64(1)}}
	conduit := &HeroChoice{Name: "Conduit", Properties: map[string]any{"kits": float64(0), "kit_note": "Prayer and ward."}}
	sub := &HeroChoice{Name: "Berserker", Properties: map[string]any{"kits": float64(2)}}
	cases := []struct {
		name string
		by   map[string]*HeroChoice
		want heroStepAllowance
	}{
		{"nothing chosen", nil, heroStepAllowance{}},
		{"entry without the property", map[string]*HeroChoice{"class": {Name: "X"}}, heroStepAllowance{}},
		{"class says one", map[string]*HeroChoice{"class": fury}, heroStepAllowance{Count: 1, Known: true, By: "Fury"}},
		{"class says none, with its note", map[string]*HeroChoice{"class": conduit}, heroStepAllowance{Count: 0, Known: true, By: "Conduit", Note: "Prayer and ward."}},
		{"subclass overrides", map[string]*HeroChoice{"class": fury, "subclass": sub}, heroStepAllowance{Count: 2, Known: true, By: "Berserker"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := heroAllowance(&step, chosen(tc.by)); got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The builder door: a value on a dependent step must belong to the parent the
// hero holds, or the one the same patch sets.
func TestClaimedOwnerPatchDependentStep(t *testing.T) {
	uid := "u1"
	hero := func(data map[string]any) *Entity {
		return &Entity{CampaignID: "c1", OwnerUserID: &uid, FieldsData: data}
	}
	types := []FieldDefinition{{Key: "class"}, {Key: "subclass"}, {Key: "kit"}}
	cases := []struct {
		name    string
		data    map[string]any
		patch   map[string]any
		wantErr string
	}{
		{"subclass of the held class", map[string]any{"class": "Fury"}, map[string]any{"subclass": "Reaver"}, ""},
		{"subclass of another class", map[string]any{"class": "Conduit"}, map[string]any{"subclass": "Reaver"}, `belongs to Fury`},
		{"subclass with no class held", nil, map[string]any{"subclass": "Reaver"}, `belongs to Fury`},
		{"parent set in the same patch", map[string]any{"class": "Conduit"}, map[string]any{"class": "Fury", "subclass": "Reaver"}, ""},
		{"patch moves the parent away", map[string]any{"class": "Fury", "subclass": "Reaver"}, map[string]any{"class": "Conduit", "subclass": "Reaver"}, `belongs to Fury`},
		{"clearing a subclass", map[string]any{"class": "Fury", "subclass": "Reaver"}, map[string]any{"subclass": nil}, ""},
		{"a held value re-sent while the parent stays", map[string]any{"class": "Fury", "subclass": "Reaver"}, map[string]any{"subclass": "Reaver"}, ""},
		// Not offering a kit on count 0 is the builder's job; the door lets it be written.
		{"kit on a class that uses none is not refused here", map[string]any{"class": "Conduit"}, map[string]any{"kit": "Panther"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckClaimedOwnerPatch(hero(tc.data), uid, "c1", tc.patch, types, stepsPlan())
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestPracticeAndTryoutPlansKeepStepShape(t *testing.T) {
	for name, plan := range map[string]*HeroPlan{"practice": practicePlan(stepsPlan()), "tryout": tryoutPlan(stepsPlan())} {
		var sub, kit *HeroStep
		for i := range plan.Steps {
			switch plan.Steps[i].FieldKey {
			case "subclass":
				sub = &plan.Steps[i]
			case "kit":
				kit = &plan.Steps[i]
			}
		}
		if sub == nil || kit == nil || sub.Parent != "class" || sub.Choices[0].Parent == "" || len(kit.CountFrom) != 2 {
			t.Errorf("%s plan lost the step declarations: %+v", name, plan.Steps)
		}
	}
}
