package systems

import (
	"context"
	"testing"
)

func TestCreatorPlan(t *testing.T) {
	ds := &SystemManifest{ID: "drawsteel", Name: "Draw Steel", EntityPresets: []EntityPresetDef{
		{Slug: "drawsteel-creature", Category: "creature", Fields: []FieldDef{{Key: "role", Type: "string"}}},
		{Slug: "drawsteel-character", Category: "character", Fields: []FieldDef{
			{Key: "class", Label: "Class", Type: "string"},
			{Key: "ancestry", Label: "Ancestry", Type: "string"},
			{Key: "level", Label: "Level", Type: "number"},
			{Key: "kit", Type: "string"},
		}},
	}}
	files := map[string]string{
		"drawsteel/data/ancestries.json": `[{"slug":"orc","name":"Orc"}]`,
		"drawsteel/data/kits.json":       `[{"slug":"panther","name":"Panther"}]`,
		"drawsteel/data/levels.json":     `[{"slug":"one","name":"One"}]`,
		"drawsteel/data/roles.json":      `[{"slug":"brute","name":"Brute"}]`,
	}
	cases := []struct {
		name      string
		enabled   fakeAddons
		wantSys   string
		wantSteps []string
	}{
		{"fields with entries, preset order, text fields only", fakeAddons{"drawsteel": true}, "Draw Steel", []string{"ancestry", "kit"}},
		{"no system gives an empty plan", fakeAddons{}, "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestChoiceService(t, files, []*SystemManifest{ds}, tc.enabled)
			plan, err := svc.CreatorPlan(context.Background(), "c1")
			if err != nil {
				t.Fatal(err)
			}
			if plan.SystemName != tc.wantSys {
				t.Errorf("system = %q, want %q", plan.SystemName, tc.wantSys)
			}
			if len(plan.Steps) != len(tc.wantSteps) {
				t.Fatalf("steps = %+v, want %v", plan.Steps, tc.wantSteps)
			}
			for i, k := range tc.wantSteps {
				if plan.Steps[i].FieldKey != k {
					t.Errorf("step %d = %q, want %q", i, plan.Steps[i].FieldKey, k)
				}
			}
			if len(plan.Steps) == 2 && (plan.Steps[0].Label != "Ancestry" || plan.Steps[1].Label != "kit") {
				t.Errorf("labels = %q, %q; want the preset's label, else the key", plan.Steps[0].Label, plan.Steps[1].Label)
			}
		})
	}
}

// A campaign entry alone makes a field a step, even when the package lists nothing.
func TestCreatorPlanCampaignOnlyField(t *testing.T) {
	choiceSourcesMu.Lock()
	saved := choiceSources
	choiceSources = nil
	choiceSourcesMu.Unlock()
	t.Cleanup(func() {
		choiceSourcesMu.Lock()
		choiceSources = saved
		choiceSourcesMu.Unlock()
	})
	RegisterChoiceSource(func(_ context.Context, _, key string) ([]Choice, error) {
		if key == "career" {
			return []Choice{{Name: "Smuggler", Source: "campaign"}}, nil
		}
		return nil, nil
	})
	m := &SystemManifest{ID: "drawsteel", Name: "Draw Steel", EntityPresets: []EntityPresetDef{
		{Slug: "drawsteel-character", Fields: []FieldDef{{Key: "career", Label: "Career", Type: "string"}}},
	}}
	svc := newTestChoiceService(t, nil, []*SystemManifest{m}, fakeAddons{"drawsteel": true})
	plan, err := svc.CreatorPlan(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Choices[0].Name != "Smuggler" {
		t.Fatalf("steps = %+v", plan.Steps)
	}
}

// The creator's starting numbers keep only the preset's own number fields,
// so a mistyped or text key in a package's declaration never reaches a hero.
func TestCreatorNumbersKeepsOnlyNumberFields(t *testing.T) {
	p := &EntityPresetDef{Fields: []FieldDef{
		{Key: "might", Label: "Might", Type: "number"}, {Key: "agility", Type: "number"},
		{Key: "class", Type: "string"}, {Key: "stamina_max", Type: "number"},
	}}
	if creatorNumbers(p) != nil {
		t.Fatal("no declaration should give no numbers")
	}
	p.CreatorNumbers = &CreatorNumbersDef{
		Characteristics: &CreatorCharacteristicsDef{Step: "class", Fields: []string{"might", "class", "nope", "agility", "might"}, ArraysProperty: "arrays", FixedValue: 2},
		Sums: []CreatorSumDef{
			{Fields: []string{"stamina_max", "class"}, Parts: []CreatorSumPart{{Step: "class", Property: "stamina"}, {Step: "", Property: "x"}}, Half: true},
			{Fields: []string{"nope"}, Parts: []CreatorSumPart{{Step: "class", Property: "stamina"}}},
		},
	}
	n := creatorNumbers(p)
	if n == nil || n.Characteristics == nil {
		t.Fatal("characteristics dropped")
	}
	c := n.Characteristics
	if c.Label != "Characteristics" || len(c.Fields) != 2 || c.Fields[0] != (CreatorField{Key: "might", Label: "Might"}) || c.Fields[1] != (CreatorField{Key: "agility", Label: "agility"}) {
		t.Fatalf("characteristics = %+v", c)
	}
	if len(n.Sums) != 1 || len(n.Sums[0].Fields) != 1 || n.Sums[0].Fields[0] != "stamina_max" || len(n.Sums[0].Parts) != 1 || !n.Sums[0].Half {
		t.Fatalf("sums = %+v", n.Sums)
	}
	p.CreatorNumbers = &CreatorNumbersDef{Characteristics: &CreatorCharacteristicsDef{Step: "class", Fields: []string{"class"}, ArraysProperty: "a"}}
	if creatorNumbers(p) != nil {
		t.Fatal("a declaration naming no number field should give no numbers")
	}
}
