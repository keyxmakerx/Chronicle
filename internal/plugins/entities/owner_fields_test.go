package entities

import (
	"reflect"
	"testing"
)

// The Attributes box offers a claimed owner exactly what the fields door
// accepts from them: identity texts, their story, and builder steps that have
// a list. GM-only fields, numbers and lists of names are left out.
func TestOwnerEditableKeys(t *testing.T) {
	fields := []FieldDefinition{
		{Key: "race"}, {Key: "class"}, {Key: "level"}, {Key: "background"},
		{Key: "backstory"}, {Key: "skills_json"}, {Key: "heritage", GMOnly: true},
	}
	plan := &HeroPlan{Steps: []HeroStep{{FieldKey: "class", Choices: []HeroChoice{{Name: "Wizard"}}}}}
	tests := []struct {
		name string
		plan *HeroPlan
		want []string
	}{
		{"with the hero plan", plan, []string{"race", "class", "background", "backstory"}},
		{"without one", nil, []string{"race", "background", "backstory"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ownerEditableKeys(fields, tt.plan); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
