package entities

import (
	"strings"
	"testing"
)

// multiPlan is a plan whose subclass step takes as many picks as the class's
// subclass_count gives: two for Conduit, one for Fury.
func multiPlan() *HeroPlan {
	return &HeroPlan{Steps: []HeroStep{
		{FieldKey: "class", Label: "Class", Choices: []HeroChoice{
			{Name: "Conduit", Properties: map[string]any{"subclass_count": float64(2)}},
			{Name: "Fury"},
		}},
		{FieldKey: "subclass", Label: "Subclass", Parent: "class", Multi: true,
			CountFrom: []HeroCount{{Step: "class", Property: "subclass_count"}},
			Choices: []HeroChoice{
				{Name: "Life", Parent: "Conduit"}, {Name: "War", Parent: "Conduit"},
				{Name: "Fate", Parent: "Conduit", State: HeroChoiceStateAskFirst},
				{Name: "Reaver", Parent: "Fury"}, {Name: "Berserker", Parent: "Fury"},
			}},
	}}
}

func TestResolveHeroPicksMulti(t *testing.T) {
	cases := []struct {
		name, class, sub string
		want, wantErr    string
	}{
		{"two domains", "Conduit", "Life, War", "Life, War", ""},
		{"one of two is allowed", "Conduit", "life", "Life", ""},
		{"spelling follows the plan", "conduit", " war ,life ", "War, Life", ""},
		{"three is too many", "Conduit", "Life, War, Fate", "", "choose up to 2"},
		{"the same one twice", "Conduit", "Life, life", "", "chosen twice"},
		{"one under another class", "Conduit", "Life, Reaver", "", `not a subclass of Conduit`},
		{"a class with no count takes one", "Fury", "Reaver, Berserker", "", "choose up to 1"},
		{"an ask-first beside another", "Conduit", "Life, Fate", "", "choose it on its own"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields, _, err := buildHeroFields(multiPlan(), map[string]HeroPick{"class": {Name: tc.class}, "subclass": {Name: tc.sub}})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if fields["subclass"] != tc.want {
				t.Fatalf("subclass = %q, want %q", fields["subclass"], tc.want)
			}
		})
	}
}

func TestBuildHeroFieldsMultiAskFirstAlone(t *testing.T) {
	fields, asks, err := buildHeroFields(multiPlan(), map[string]HeroPick{"class": {Name: "Conduit"}, "subclass": {Name: "Fate"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, set := fields["subclass"]; set || len(asks) != 1 || asks[0].EntryName != "Fate" {
		t.Fatalf("fields %v asks %v: an ask-first pick alone becomes a request", fields, asks)
	}
}

func TestOwnerMultiValue(t *testing.T) {
	picks := builderPicksFrom(multiPlan())
	owner := "u1"
	hero := func(fields map[string]any) *Entity {
		return &Entity{ID: "e1", CampaignID: "c1", OwnerUserID: &owner, FieldsData: fields}
	}
	cases := []struct {
		name    string
		entity  *Entity
		patch   map[string]any
		wantErr string
	}{
		{"two under the hero's class", hero(map[string]any{"class": "Conduit"}), map[string]any{"subclass": "Life, War"}, ""},
		{"two with the class in the same patch", hero(nil), map[string]any{"class": "Conduit", "subclass": "War, Life"}, ""},
		{"three", hero(map[string]any{"class": "Conduit"}), map[string]any{"subclass": "Life, War, Fate"}, "takes up to 2"},
		{"twice", hero(map[string]any{"class": "Conduit"}), map[string]any{"subclass": "Life, Life"}, "chosen twice"},
		{"off the list", hero(map[string]any{"class": "Conduit"}), map[string]any{"subclass": "Life, Moon"}, "not on the subclass list"},
		{"under another class", hero(map[string]any{"class": "Fury"}), map[string]any{"subclass": "Reaver, Life"}, "takes up to 1"},
		{"another class's entry", hero(map[string]any{"class": "Conduit"}), map[string]any{"subclass": "Life, Reaver"}, "belongs to Fury"},
		{"ask-first beside another", hero(map[string]any{"class": "Conduit"}), map[string]any{"subclass": "Life, Fate"}, "choose it on its own"},
		{"ask-first alone passes to become a request", hero(map[string]any{"class": "Conduit"}), map[string]any{"subclass": "Fate"}, ""},
		{"a held name stays", hero(map[string]any{"class": "Conduit", "subclass": "Life, Moon"}), map[string]any{"subclass": "Moon, War"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			held := ownerFieldVerdicts(tc.entity, tc.patch, nil, picks, false)
			err := held["subclass"]
			if tc.wantErr == "" {
				if len(held) > 0 {
					t.Fatalf("refused: %v", held)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestHeroFieldWarningsMulti(t *testing.T) {
	fields := map[string]any{"class": "Conduit", "subclass": "Life, War"}
	if w := HeroFieldWarnings(multiPlan(), fields, []string{"subclass"}); len(w) != 0 {
		t.Fatalf("two domains warned: %v", w)
	}
	fields["subclass"] = "Life, Moon"
	if w := HeroFieldWarnings(multiPlan(), fields, []string{"subclass"}); len(w) != 1 {
		t.Fatalf("an unknown domain should warn once: %v", w)
	}
}
