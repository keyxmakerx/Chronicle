package entities

import (
	"reflect"
	"testing"
)

// The practice plan puts back what the rules held from this player, labelled,
// and drops every rule that would lock or force a step. Held entries are a
// subset of what the player may read (the input is the player's own limited
// plan), so nothing new is revealed.
func TestPracticePlan(t *testing.T) {
	fury := HeroChoice{Name: "Fury", Slug: "fury", Description: "Rage.", Properties: map[string]any{"stamina": float64(21)}}
	tactician := HeroChoice{Name: "Tactician", Slug: "tactician", State: HeroChoiceStateAskFirst}
	conduit := HeroChoice{Name: "Conduit", Slug: "conduit"}
	tests := []struct {
		name string
		in   *HeroPlan
		want map[string][]HeroChoice // by step key
	}{
		{"nil plan is an empty practice plan", nil, map[string][]HeroChoice{}},
		{"nothing held changes nothing", &HeroPlan{Steps: []HeroStep{{FieldKey: "class", Choices: []HeroChoice{conduit}}}},
			map[string][]HeroChoice{"class": {conduit}}},
		{"held entries return as not offered, whole",
			&HeroPlan{Steps: []HeroStep{{FieldKey: "class", Choices: []HeroChoice{conduit, tactician}, Held: []HeroChoice{fury}}}},
			map[string][]HeroChoice{"class": {conduit, tactician, func() HeroChoice { c := fury; c.State = HeroChoiceStateNotOffered; return c }()}}},
		{"a required short list's left-out entries come back the same way",
			&HeroPlan{Steps: []HeroStep{{FieldKey: "kit", Choices: []HeroChoice{conduit},
				Rules: &HeroStepRules{Limited: true, Required: true, Prefilled: "Conduit"}, Held: []HeroChoice{fury}}}},
			map[string][]HeroChoice{"kit": {conduit, func() HeroChoice { c := fury; c.State = HeroChoiceStateNotOffered; return c }()}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := practicePlan(tc.in)
			if !got.Practice {
				t.Error("plan is not marked practice")
			}
			byKey := map[string][]HeroChoice{}
			for _, st := range got.Steps {
				byKey[st.FieldKey] = st.Choices
				if st.Rules != nil || len(st.Held) != 0 {
					t.Errorf("step %s keeps rules or held entries: %+v", st.FieldKey, st)
				}
			}
			if !reflect.DeepEqual(byKey, tc.want) {
				t.Errorf("choices = %+v\nwant     %+v", byKey, tc.want)
			}
		})
	}
}

// The practice plan must not alias the player's plan: building it leaves the
// input untouched.
func TestPracticePlan_DoesNotChangeInput(t *testing.T) {
	in := &HeroPlan{Steps: []HeroStep{{FieldKey: "class", Choices: []HeroChoice{{Name: "A"}}, Held: []HeroChoice{{Name: "B"}}}}}
	_ = practicePlan(in)
	if len(in.Steps[0].Choices) != 1 || in.Steps[0].Held[0].State != "" {
		t.Errorf("input changed: %+v", in.Steps[0])
	}
}

func TestPracticeSummary(t *testing.T) {
	two := 2
	tests := []struct {
		name  string
		picks map[string]HeroPick
		level *int
		want  string
	}{
		{"ancestry and class", map[string]HeroPick{"ancestry": {Name: "Dwarf"}, "class": {Name: "Conduit"}}, nil, "Dwarf Conduit"},
		{"with level", map[string]HeroPick{"ancestry": {Name: "Dwarf"}, "class": {Name: "Conduit"}}, &two, "Dwarf Conduit, level 2"},
		{"species and calling are read as the party block reads them", map[string]HeroPick{"species": {Name: "Elf"}, "calling": {Name: "Bard"}}, nil, "Elf Bard"},
		{"only a class", map[string]HeroPick{"class": {Name: "Fury"}}, nil, "Fury"},
		{"only a level", map[string]HeroPick{}, &two, "level 2"},
		{"nothing known", map[string]HeroPick{"kit": {Name: "Panther"}}, nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := practiceSummary(tc.picks, tc.level); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPracticePickRows(t *testing.T) {
	plan := &HeroPlan{Steps: []HeroStep{
		{FieldKey: "class", Label: "Class", Choices: []HeroChoice{{Name: "Fury", State: HeroChoiceStateNotOffered}, {Name: "Conduit"}}},
		{FieldKey: "kit", Label: "Kit", Choices: []HeroChoice{{Name: "Panther"}, {Name: "Rapier", State: HeroChoiceStateAskFirst}}},
		{FieldKey: "culture", Label: "Culture", Choices: []HeroChoice{{Name: "Nomad"}}},
	}}
	picks := map[string]HeroPick{
		"kit":   {Name: "rapier", Options: []string{"Edge"}},
		"class": {Name: "Fury"},
		"gone":  {Name: "Whatever"},
	}
	got := practicePickRows(plan, picks)
	want := []PracticePickRow{
		{Label: "Class", Value: "Fury", Note: practiceNoteNotOffered},
		{Label: "Kit", Value: "rapier", Options: "Edge", Note: practiceNoteAskFirst},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %+v, want %+v", got, want)
	}
	if !practiceNeedsYes(plan, picks) {
		t.Error("a hero holding a not-offered entry needs a Director's yes")
	}
	if practiceNeedsYes(plan, map[string]HeroPick{"class": {Name: "Conduit"}}) {
		t.Error("a hero holding only open entries needs no yes")
	}
}

func TestPracticeCopyFields(t *testing.T) {
	plan := &HeroPlan{Steps: []HeroStep{
		{FieldKey: "ancestry", Choices: []HeroChoice{{Name: "Human", Properties: map[string]any{
			"signature_points": float64(3),
			"traits":           []any{map[string]any{"name": "Tough", "cost": float64(1)}, map[string]any{"name": "Quick", "cost": float64(1)}},
		}}}},
		{FieldKey: "class", Choices: []HeroChoice{{Name: "Fury"}}},
		{FieldKey: "kit", Choices: []HeroChoice{{Name: "Panther"}}},
	}}
	tests := []struct {
		name      string
		data      map[string]any
		hidden    map[string]bool
		want      map[string]HeroPick
		wantLevel *int
	}{
		{"values naming plan entries are carried, options kept when still offered",
			map[string]any{"ancestry": "human", "ancestry_choices_json": `["Tough","Gone","Tough"]`, "class": "Fury", "level": float64(3)},
			nil, map[string]HeroPick{"ancestry": {Name: "Human", Options: []string{"Tough"}}, "class": {Name: "Fury"}}, intp(3)},
		{"a value not in the plan is left out", map[string]any{"class": "Homebrew", "kit": "Panther"}, nil,
			map[string]HeroPick{"kit": {Name: "Panther"}}, nil},
		{"hidden keys are never read", map[string]any{"class": "Fury", "kit": "Panther", "level": float64(4)}, map[string]bool{"class": true, "level": true},
			map[string]HeroPick{"kit": {Name: "Panther"}}, nil},
		{"a text level is read", map[string]any{"level": " 5 "}, nil, map[string]HeroPick{}, intp(5)},
		{"a silly level is not", map[string]any{"level": float64(99999)}, nil, map[string]HeroPick{}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, level := practiceCopyFields(plan, tc.data, tc.hidden)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("picks = %+v, want %+v", got, tc.want)
			}
			if (level == nil) != (tc.wantLevel == nil) || level != nil && *level != *tc.wantLevel {
				t.Errorf("level = %v, want %v", level, tc.wantLevel)
			}
		})
	}
}
