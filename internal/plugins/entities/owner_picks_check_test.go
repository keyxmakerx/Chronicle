package entities

import (
	"strings"
	"testing"
)

func TestPlayerPicksHeld(t *testing.T) {
	owner := "u1"
	hero := func(fields map[string]any) *Entity {
		return &Entity{ID: "e1", CampaignID: "c1", OwnerUserID: &owner, FieldsData: fields}
	}
	plan := &HeroPlan{Steps: []HeroStep{
		{FieldKey: "class", Choices: []HeroChoice{{Name: "Fury"}, {Name: "Censor", State: HeroChoiceStateAskFirst}},
			Held: []HeroChoice{{Name: "Null"}}, Rules: &HeroStepRules{Limited: true}},
		{FieldKey: "ancestry", Choices: []HeroChoice{{Name: "Human"}}, Rules: &HeroStepRules{Limited: true, Required: true}},
		{FieldKey: "title", Choices: []HeroChoice{{Name: "Knight"}}},
		{FieldKey: "kit", Choices: []HeroChoice{{Name: "Shining Armor"}}},
	}}
	tests := []struct {
		name  string
		hero  *Entity
		patch map[string]any
		plan  *HeroPlan
		want  map[string]string // key -> substring of the reason
	}{
		{"on-list class kept", hero(nil), map[string]any{"class": "Fury"}, plan, nil},
		{"off-list class held", hero(nil), map[string]any{"class": "Wizard", "stamina": float64(30)}, plan, map[string]string{"class": "not on the class list"}},
		{"mapped number is not a pick", hero(nil), map[string]any{"stamina": float64(30), "recoveries": float64(8)}, plan, nil},
		{"ask-first pick held, never stored", hero(nil), map[string]any{"class": "Censor"}, plan, map[string]string{"class": "Director"}},
		{"ask-first pick the hero already holds is kept", hero(map[string]any{"class": "Censor"}), map[string]any{"class": "Censor"}, plan, nil},
		{"entry a rule took off is held for a new pick", hero(nil), map[string]any{"class": "Null"}, plan, map[string]string{"class": "not on the class list"}},
		{"grandfathered value resent unchanged", hero(map[string]any{"class": "Null"}), map[string]any{"class": "Null", "stamina": float64(9)}, plan, nil},
		{"typed text on a limited identity step held", hero(nil), map[string]any{"ancestry": "Hydra"}, plan, map[string]string{"ancestry": "not on the ancestry list"}},
		{"typed text on a step with no rule is free", hero(nil), map[string]any{"kit": "My own kit"}, plan, nil},
		{"off-list name on a pick step with no rule held", hero(nil), map[string]any{"title": "Lord"}, plan, map[string]string{"title": "not on the title list"}},
		{"identity text with no step is free", hero(nil), map[string]any{"culture": "Anything"}, plan, nil},
		{"clearing a required step held", hero(map[string]any{"ancestry": "Human"}), map[string]any{"ancestry": nil}, plan, map[string]string{"ancestry": "can't be left empty"}},
		{"clearing an empty step changes nothing", hero(nil), map[string]any{"ancestry": nil}, plan, nil},
		{"no plan leaves only identity texts", hero(nil), map[string]any{"class": "Wizard", "culture": "Urban"}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlayerPicksHeld(tt.hero, tt.patch, tt.plan)
			if len(got) != len(tt.want) {
				t.Fatalf("held %v, want keys %v", got, tt.want)
			}
			for k, sub := range tt.want {
				if !strings.Contains(got[k], sub) {
					t.Errorf("held[%q] = %q, want it to mention %q", k, got[k], sub)
				}
			}
		})
	}
}

// An off-list name reads the same whether or not a hidden entry has that
// name, so the refusal can't be used to find hidden entries.
func TestPlayerPicksHeld_SameRefusalForHiddenName(t *testing.T) {
	plan := &HeroPlan{Steps: []HeroStep{{FieldKey: "class", Choices: []HeroChoice{{Name: "Fury"}}, Rules: &HeroStepRules{Limited: true}}}}
	hero := &Entity{ID: "e1", CampaignID: "c1"}
	a := PlayerPicksHeld(hero, map[string]any{"class": "Secret Class"}, plan)["class"]
	b := PlayerPicksHeld(hero, map[string]any{"class": "Nonsense"}, plan)["class"]
	if a == "" || strings.ReplaceAll(a, "Secret Class", "X") != strings.ReplaceAll(b, "Nonsense", "X") {
		t.Fatalf("refusals differ: %q vs %q", a, b)
	}
}

func TestPatchChangesNothing(t *testing.T) {
	hero := &Entity{ID: "e1", CampaignID: "c1", FieldsData: map[string]any{"class": "Fury", "stamina": float64(21), "kit": ""}}
	cases := []struct {
		name  string
		patch map[string]any
		want  bool
	}{
		{"whole set resent", map[string]any{"class": "fury ", "stamina": float64(21)}, true},
		{"null over an empty value", map[string]any{"kit": nil}, true},
		{"empty patch", map[string]any{}, true},
		{"a number changed", map[string]any{"class": "Fury", "stamina": float64(18)}, false},
		{"a pick changed", map[string]any{"class": "Null"}, false},
		{"a new key", map[string]any{"ancestry": "Devil"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PatchChangesNothing(hero, c.patch); got != c.want {
				t.Errorf("PatchChangesNothing(%v) = %v, want %v", c.patch, got, c.want)
			}
		})
	}
}
