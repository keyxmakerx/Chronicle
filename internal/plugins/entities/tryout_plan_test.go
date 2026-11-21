package entities

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// The visitor's plan is rebuilt from an allow-list. Whatever the planner hands
// over, no rule detail, held entry, ask, "make your own" offer, ask-first seal
// or entry that belongs to a player reaches it.
func TestTryoutPlan(t *testing.T) {
	plain := HeroChoice{Key: "conduit", Slug: "conduit", Name: "Conduit", Summary: "Channels.", Source: "package",
		Description: "Text.", Properties: map[string]any{"stamina": float64(21)}}
	tests := []struct {
		name string
		in   *HeroPlan
		want map[string][]string // names by step key
	}{
		{"nil plan is an empty try-out plan", nil, map[string][]string{}},
		{"a plain step is kept", &HeroPlan{Steps: []HeroStep{{FieldKey: "class", Label: "Class", Choices: []HeroChoice{plain}}}},
			map[string][]string{"class": {"Conduit"}}},
		{"ask-first entries become plain choices",
			&HeroPlan{Steps: []HeroStep{{FieldKey: "class", Choices: []HeroChoice{plain, {Name: "Tactician", Source: "package", State: HeroChoiceStateAskFirst}}}}},
			map[string][]string{"class": {"Conduit", "Tactician"}}},
		{"a player's own entry and a draft are dropped",
			&HeroPlan{Steps: []HeroStep{{FieldKey: "class", Choices: []HeroChoice{plain,
				{Name: "Mine", Source: "campaign", Mine: true, EntryID: 4}, {Name: "Draft", Source: "campaign", Status: "draft"}}}}},
			map[string][]string{"class": {"Conduit"}}},
		{"an entry from an unknown source is dropped",
			&HeroPlan{Steps: []HeroStep{{FieldKey: "class", Choices: []HeroChoice{plain, {Name: "Odd", Source: "plugin"}}}}},
			map[string][]string{"class": {"Conduit"}}},
		{"a held entry is never carried",
			&HeroPlan{Steps: []HeroStep{{FieldKey: "class", Choices: []HeroChoice{plain}, Held: []HeroChoice{{Name: "Fury", Source: "package"}}}}},
			map[string][]string{"class": {"Conduit"}}},
		{"a step left with nothing to pick is dropped",
			&HeroPlan{Steps: []HeroStep{{FieldKey: "kit", Label: "Kit", Rules: &HeroStepRules{Limited: true}}, {FieldKey: "class", Choices: []HeroChoice{plain}}}},
			map[string][]string{"class": {"Conduit"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tryoutPlan(tc.in)
			if !got.Tryout || !got.Practice {
				t.Errorf("plan is not marked try-out and practice: %+v", got)
			}
			names := map[string][]string{}
			for _, st := range got.Steps {
				for _, c := range st.Choices {
					names[st.FieldKey] = append(names[st.FieldKey], c.Name)
					if c.State != "" || c.Mine || c.Status != "" || c.EntryID != 0 || c.Held {
						t.Errorf("entry %s keeps a player-specific field: %+v", c.Name, c)
					}
				}
			}
			if !reflect.DeepEqual(names, tc.want) {
				t.Errorf("entries = %v, want %v", names, tc.want)
			}
		})
	}
}

// Even when everything a plan can carry is set, the JSON a visitor gets holds
// none of it: the rules, the waiting and suggested asks, the not-here names
// and reasons, the make-your-own offer.
func TestTryoutPlan_JSONCarriesNoRuleDetail(t *testing.T) {
	in := &HeroPlan{SystemID: "sys", SystemName: "Test", Steps: []HeroStep{{
		FieldKey: "class", Label: "Class", CanMake: true,
		Choices: []HeroChoice{{Name: "Conduit", Source: "package", State: HeroChoiceStateAskFirst, Mine: true}},
		Held:    []HeroChoice{{Name: "Fury", Source: "package"}},
		Rules: &HeroStepRules{Limited: true, Required: true, Prefilled: "Conduit", NotHereCount: 1,
			NotHere: []HeroNotHere{{Name: "Fury", Reason: "Too many already"}},
			Waiting: &HeroRequestView{EntryName: "Tactician", Message: "please"}, Suggested: &HeroRequestView{EntryName: "Fury"}},
	}}}
	raw, err := json.Marshal(tryoutPlan(in))
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"Fury", "Too many already", "rules", "notHere", "waiting", "suggested", "canMake", "ask_first", "prefilled", "held", "mine", "Tactician"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("visitor plan carries %q: %s", leak, raw)
		}
	}
	if !strings.Contains(string(raw), `"tryout":true`) || !strings.Contains(string(raw), `"practice":true`) {
		t.Errorf("plan is not flagged: %s", raw)
	}
}

// The plan the handler builds must not alias or change the planner's.
func TestTryoutPlan_DoesNotChangeInput(t *testing.T) {
	in := &HeroPlan{Steps: []HeroStep{{FieldKey: "class", Choices: []HeroChoice{{Name: "A", Source: "package", State: HeroChoiceStateAskFirst}}}}}
	_ = tryoutPlan(in)
	if in.Steps[0].Choices[0].State != HeroChoiceStateAskFirst {
		t.Errorf("input changed: %+v", in.Steps[0])
	}
}
