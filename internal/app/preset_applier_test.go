package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

func TestMapPresetFieldType(t *testing.T) {
	cases := []struct{ in, want string }{
		{"number", "number"},
		{"boolean", "checkbox"},
		{"enum", "select"},
		{"markdown", "textarea"},
		{"list", "textarea"},
		{"url", "url"},
		{"string", "text"},
		{"", "text"},
		{"something-unknown", "text"},
	}
	for _, c := range cases {
		if got := mapPresetFieldType(c.in); got != c.want {
			t.Errorf("mapPresetFieldType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMapPresetFields(t *testing.T) {
	// nil / empty → nil (service normalizes to []).
	if got := mapPresetFields(nil); got != nil {
		t.Errorf("expected nil for no fields, got %v", got)
	}

	// Maps key/label/type; drops the Foundry-sync annotations (those go to the
	// Foundry module via the character-fields API, not the entity-type schema).
	in := []systems.FieldDef{
		{Key: "might", Label: "Might", Type: "number", FoundryPath: "system.characteristics.might"},
		{Key: "backstory", Label: "Backstory", Type: "markdown"},
		{Key: "abilities_json", Label: "Abilities", Type: "string", FoundryCollection: "items"},
	}
	got := mapPresetFields(in)
	if len(got) != 3 {
		t.Fatalf("expected 3 mapped fields, got %d", len(got))
	}
	if got[0].Key != "might" || got[0].Label != "Might" || got[0].Type != "number" {
		t.Errorf("might mapped wrong: %+v", got[0])
	}
	if got[1].Type != "textarea" { // markdown → textarea
		t.Errorf("backstory type = %q, want textarea", got[1].Type)
	}
	if got[2].Type != "text" { // string → text
		t.Errorf("abilities type = %q, want text", got[2].Type)
	}
}

// TestMapPresetFields_CarriesPlay pins that a manifest play block survives
// preset application onto the stored field definition, deep-copied, and that a
// field without one stays nil.
func TestMapPresetFields_CarriesPlay(t *testing.T) {
	zero, tru := 0.0, true
	play := &systems.PlayDef{
		Edit: "owner", Kind: "conditions", Min: &zero, MaxField: "hp_max", Step: 1,
		Options: []string{"dazed", "bleeding"}, MaxLength: 12, ToFoundry: &tru, CombatAuthority: "foundry",
	}
	got := mapPresetFields([]systems.FieldDef{
		{Key: "conditions", Label: "Conditions", Type: "string", Play: play},
		{Key: "notes", Label: "Notes", Type: "markdown"},
	})
	if got[1].Play != nil {
		t.Errorf("field without play got %+v", got[1].Play)
	}
	p := got[0].Play
	if p == nil {
		t.Fatal("play block lost in preset application")
	}
	if p.Edit != "owner" || p.Kind != "conditions" || p.MaxField != "hp_max" || p.Step != 1 ||
		p.MaxLength != 12 || p.CombatAuthority != "foundry" || p.Min == nil || *p.Min != 0 ||
		p.ToFoundry == nil || !*p.ToFoundry || len(p.Options) != 2 {
		t.Errorf("play not carried faithfully: %+v", p)
	}
	play.Options[0] = "mutated"
	if p.Options[0] != "dazed" {
		t.Error("stored options alias the manifest's slice")
	}
}
