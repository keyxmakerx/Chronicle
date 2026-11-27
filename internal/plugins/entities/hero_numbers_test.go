package entities

import (
	"fmt"
	"strings"
	"testing"
)

// The declaration a Draw Steel–shaped system sends: a class fixes some
// characteristics at 2 and offers arrays for the rest; Stamina is class plus
// kit, winded half of it, Recoveries the class's.
func numbersFixture() *HeroNumbers {
	return &HeroNumbers{
		Characteristics: &HeroCharacteristics{Label: "Characteristics", Step: "class", FixedProperty: "primary", FixedValue: 2, ArraysProperty: "arrays",
			Fields: []HeroField{{"might", "Might"}, {"agility", "Agility"}, {"reason", "Reason"}, {"intuition", "Intuition"}, {"presence", "Presence"}}},
		Sums: []HeroSum{
			{Fields: []string{"stamina_max", "stamina_current"}, Parts: []HeroSumPart{{"class", "stamina"}, {"kit", "stamina_bonus"}}},
			{Fields: []string{"winded"}, Parts: []HeroSumPart{{"class", "stamina"}, {"kit", "stamina_bonus"}}, Half: true},
			{Fields: []string{"recoveries", "recoveries_max"}, Parts: []HeroSumPart{{"class", "recoveries"}}},
		},
	}
}

func picked(class, kit *HeroChoice) map[string]heroPickResult {
	r := map[string]heroPickResult{}
	if class != nil {
		r["class"] = heroPickResult{choice: class}
	}
	if kit != nil {
		r["kit"] = heroPickResult{choice: kit}
	}
	return r
}

var (
	shadow = &HeroChoice{Name: "Shadow", Properties: map[string]any{"primary": []any{"Agility"}, "stamina": float64(18), "recoveries": float64(8),
		"arrays": []any{"2, 2, −1, −1", "2, 1, 1, −1", "2, 1, 0, 0", "1, 1, 1, 0"}}}
	censor = &HeroChoice{Name: "Censor", Properties: map[string]any{"primary": []any{"Might", "Presence"}, "stamina": float64(21), "recoveries": float64(12),
		"arrays": []any{"2, −1, −1", "1, 1, −1", "1, 0, 0"}}}
	warriorPriest = &HeroChoice{Name: "Warrior Priest", Properties: map[string]any{"stamina_bonus": float64(9)}}
)

func TestHeroNumberFields(t *testing.T) {
	brannoc := map[string]int{"agility": 2, "might": 2, "reason": 1, "intuition": 0, "presence": 0}
	tests := []struct {
		name    string
		res     map[string]heroPickResult
		placed  map[string]int
		want    map[string]any
		wantErr string
	}{
		{"Brannoc: Shadow with the Warrior Priest kit", picked(shadow, warriorPriest), brannoc,
			map[string]any{"stamina_max": 27, "stamina_current": 27, "winded": 13, "recoveries": 8, "recoveries_max": 8,
				"agility": 2, "might": 2, "reason": 1, "intuition": 0, "presence": 0}, ""},
		{"no kit yet: class Stamina alone", picked(shadow, nil), nil,
			map[string]any{"stamina_max": 18, "stamina_current": 18, "winded": 9, "recoveries": 8, "recoveries_max": 8}, ""},
		{"nothing picked: no numbers", picked(nil, nil), nil, map[string]any{}, ""},
		{"an array in any order", picked(shadow, nil), map[string]int{"agility": 2, "might": -1, "reason": 1, "intuition": 1, "presence": 2},
			nil, ""},
		{"two fixed characteristics, three-number arrays", picked(censor, nil), map[string]int{"might": 2, "presence": 2, "agility": 1, "reason": 0, "intuition": 0},
			nil, ""},
		{"a fixed characteristic moved", picked(shadow, nil), map[string]int{"agility": 1, "might": 2, "reason": 2, "intuition": -1, "presence": -1},
			nil, "Agility is 2 for Shadow"},
		{"numbers that are no array", picked(shadow, nil), map[string]int{"agility": 2, "might": 2, "reason": 2, "intuition": 2, "presence": 2},
			nil, "aren't one of Shadow's arrays"},
		{"one left out", picked(shadow, nil), map[string]int{"agility": 2, "might": 2, "reason": 1, "intuition": 0},
			nil, "every characteristic, or none"},
		{"a field that isn't a characteristic", picked(shadow, nil), map[string]int{"agility": 2, "might": 2, "reason": 1, "intuition": 0, "presence": 0, "stamina_max": 99},
			nil, "isn't a characteristic"},
		{"placed with no class", picked(nil, nil), brannoc, nil, "before placing them"},
		{"an ask-first class adds nothing", map[string]heroPickResult{"class": {choice: &HeroChoice{Name: "Shadow", State: HeroChoiceStateAskFirst, Properties: shadow.Properties}}},
			brannoc, nil, "before placing them"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := heroNumberFields(numbersFixture(), tc.res, tc.placed)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == nil {
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("%s = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestHeroNumbers_NoDeclaration(t *testing.T) {
	if got, err := heroNumberFields(nil, picked(shadow, nil), nil); err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want nothing", got, err)
	}
	if _, err := heroNumberFields(nil, picked(shadow, nil), map[string]int{"might": 2}); err == nil {
		t.Fatal("placed characteristics accepted by a system that declares none")
	}
}

// A pick writes its own field; a number never overwrites it.
func TestAddHeroNumbers_KeepsPickedFields(t *testing.T) {
	fields := map[string]any{"stamina_max": "from a pick"}
	if err := addHeroNumbersFrom(numbersFixture(), picked(shadow, warriorPriest), nil, fields); err != nil {
		t.Fatal(err)
	}
	if fields["stamina_max"] != "from a pick" || fields["stamina_current"] != 27 {
		t.Fatalf("fields = %v", fields)
	}
}

func TestParseArray(t *testing.T) {
	for in, want := range map[string]string{"2, 1, 0, \u22121": "[2 1 0 -1]", "+1,1,\u20131": "[1 1 -1]", "1, x": "fail", "": "fail"} {
		got, ok := parseArray(in)
		s := "fail"
		if ok {
			s = fmt.Sprint(got)
		}
		if s != want {
			t.Errorf("parseArray(%q) = %s, want %s", in, s, want)
		}
	}
}

// Through the creator's own pick check: the picks named in the request find
// their entries in the plan, and the numbers follow from those.
func TestAddHeroNumbers_FromRequest(t *testing.T) {
	plan := &HeroPlan{Numbers: numbersFixture(), Steps: []HeroStep{
		{FieldKey: "class", Label: "Class", Choices: []HeroChoice{*shadow, *censor}},
		{FieldKey: "kit", Label: "Kit", Choices: []HeroChoice{*warriorPriest}},
	}}
	req := HeroRequest{Name: "Brannoc", Picks: map[string]HeroPick{"class": {Name: "shadow"}, "kit": {Name: "Warrior Priest"}},
		Characteristics: map[string]int{"agility": 2, "might": 2, "reason": 1, "intuition": 0, "presence": 0}}
	fields, _, err := buildHeroFields(plan, req.Picks)
	if err != nil {
		t.Fatal(err)
	}
	if err := addHeroNumbers(plan, req, fields); err != nil {
		t.Fatal(err)
	}
	if fields["class"] != "Shadow" || fields["stamina_max"] != 27 || fields["recoveries"] != 8 || fields["reason"] != 1 {
		t.Fatalf("fields = %v", fields)
	}
	req.Characteristics["reason"] = 2
	if err := addHeroNumbers(plan, req, map[string]any{}); err == nil {
		t.Fatal("numbers off every array were accepted")
	}
}
