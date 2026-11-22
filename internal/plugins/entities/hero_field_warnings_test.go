package entities

import (
	"strings"
	"testing"
)

func warningPlan() *HeroPlan {
	return &HeroPlan{Steps: []HeroStep{
		{FieldKey: "ancestry", Label: "Ancestry", Choices: []HeroChoice{{Name: "Dwarf"}, {Name: "Elf"}, {Name: "Human"}, {Name: "Dragon Knight"}}},
		{FieldKey: "class", Label: "Class", Choices: []HeroChoice{{Name: "Fury"}, {Name: "Censor"}}},
		{FieldKey: "subclass", Label: "Subclass", Parent: "class", Choices: []HeroChoice{
			{Name: "Berserker", Parent: "Fury"}, {Name: "Stormwight", Parent: "Fury"}, {Name: "Exorcist", Parent: "Censor"},
		}},
	}}
}

// A value off the campaign's list, or a subclass under another class, gets a
// warning that names the field and the nearest valid names; a value on the
// list, a key the change did not write, and a missing plan give none.
func TestHeroFieldWarnings(t *testing.T) {
	tests := []struct {
		name    string
		plan    *HeroPlan
		fields  map[string]any
		written []string
		want    []string // substrings, one warning each
	}{
		{"on the list", warningPlan(), map[string]any{"ancestry": "dwarf"}, []string{"ancestry"}, nil},
		{"off the list", warningPlan(), map[string]any{"ancestry": "Dwarff"}, []string{"ancestry"},
			[]string{`Ancestry: "Dwarff" is not on the ancestry list. Closest on the campaign's list: Dwarf,`}},
		{"not written is not checked", warningPlan(), map[string]any{"ancestry": "Dwarff"}, []string{"class"}, nil},
		{"subclass of another class", warningPlan(), map[string]any{"class": "Fury", "subclass": "Exorcist"}, []string{"class", "subclass"},
			[]string{`Subclass: "Exorcist" is not a subclass of Fury. Closest on the campaign's list: Stormwight, Berserker.`}},
		{"subclass under the class the page already had", warningPlan(), map[string]any{"class": "Censor", "subclass": "Berserker"}, []string{"subclass"},
			[]string{"Closest on the campaign's list: Exorcist."}},
		{"subclass off the list names only that class's", warningPlan(), map[string]any{"class": "Fury", "subclass": "Berzerker"}, []string{"subclass"},
			[]string{"Closest on the campaign's list: Berserker, Stormwight."}},
		{"bad class is reported once", warningPlan(), map[string]any{"class": "Furry", "subclass": "Berserker"}, []string{"class", "subclass"},
			[]string{`Class: "Furry" is not on the class list. Closest on the campaign's list: Fury`}},
		{"subclass without a class is left alone", warningPlan(), map[string]any{"subclass": "Berserker"}, []string{"subclass"}, nil},
		{"empty and non-text values", warningPlan(), map[string]any{"ancestry": "", "class": 3.0}, []string{"ancestry", "class"}, nil},
		{"field without a list", warningPlan(), map[string]any{"name": "x"}, []string{"name"}, nil},
		{"no plan", nil, map[string]any{"ancestry": "x"}, []string{"ancestry"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := HeroFieldWarnings(tc.plan, tc.fields, tc.written)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d warnings %q, want %d", len(got), got, len(tc.want))
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("warning %q lacks %q", got[i], w)
				}
				if !strings.Contains(got[i], "only a warning") {
					t.Errorf("warning %q does not say it is only a warning", got[i])
				}
			}
		})
	}
}
