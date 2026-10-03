package systems

import (
	"encoding/json"
	"testing"
)

func TestValidateDMScreen(t *testing.T) {
	tests := []struct {
		name    string
		d       *DMScreenDef
		wantErr bool
	}{
		{"absent", nil, false},
		{"draw steel shape", &DMScreenDef{
			Party: []DMScreenMeter{
				{Label: "Stamina", Current: "stamina_current", Max: "stamina_max", WarnBelow: 0.5},
				{Label: "Recoveries", Current: "recoveries", Max: "recoveries_max"},
				{LabelField: "heroic_resource_name", Current: "heroic_resource_current"},
			},
			HeroSubtitle:   "class",
			HeroConditions: "conditions_json",
			Conditions:     &DMScreenConditions{Category: "rules-glossary", Property: "category", Value: "condition"},
		}, false},
		{"bad hero subtitle", &DMScreenDef{HeroSubtitle: "Class Name"}, true},
		{"bad hero conditions", &DMScreenDef{HeroConditions: "conds;drop"}, true},
		{"missing current", &DMScreenDef{Party: []DMScreenMeter{{Label: "HP"}}}, true},
		{"bad field key", &DMScreenDef{Party: []DMScreenMeter{{Label: "HP", Current: "hp current"}}}, true},
		{"no label", &DMScreenDef{Party: []DMScreenMeter{{Current: "hp"}}}, true},
		{"warn out of range", &DMScreenDef{Party: []DMScreenMeter{{Label: "HP", Current: "hp", WarnBelow: 2}}}, true},
		{"too many meters", &DMScreenDef{Party: make([]DMScreenMeter, maxDMScreenMeters+1)}, true},
		{"property without value", &DMScreenDef{Conditions: &DMScreenConditions{Category: "conditions", Property: "category"}}, true},
		{"bad category", &DMScreenDef{Conditions: &DMScreenConditions{Category: "../x"}}, true},
		{"category only", &DMScreenDef{Conditions: &DMScreenConditions{Category: "conditions"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDMScreen(tt.d)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateDMScreen() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// A bad dm_screen block fails the whole manifest, like other blocks.
func TestValidateManifest_DMScreen(t *testing.T) {
	var m SystemManifest
	raw := `{"id":"t","name":"T","api_version":"1","dm_screen":{"party":[{"label":"HP"}]}}`
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if err := ValidateManifest(&m); err == nil {
		t.Fatal("expected dm_screen error")
	}
	m.DMScreen.Party[0].Current = "hp_current"
	if err := ValidateManifest(&m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
