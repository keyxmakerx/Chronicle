package systems

import (
	"encoding/json"
	"strings"
	"testing"
)

// playManifest builds a manifest whose single preset holds the two sibling
// fields every case needs (a number to point max_field at, a string to point
// it wrongly at) plus the field under test.
func playManifest(t *testing.T, fieldJSON string) *SystemManifest {
	t.Helper()
	raw := `{
		"id": "play-test", "name": "Play Test", "api_version": "1",
		"entity_presets": [{
			"slug": "play-character", "name": "Hero", "name_plural": "Heroes", "icon": "fa-user",
			"fields": [
				{"key": "hp_max", "label": "Max", "type": "number"},
				{"key": "title", "label": "Title", "type": "string"},
				` + fieldJSON + `
			]
		}]
	}`
	var m SystemManifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return &m
}

func TestPlayBlockValidation(t *testing.T) {
	cases := []struct {
		name     string
		field    string
		wantKept bool
		wantWarn string // substring of the warning when dropped
	}{
		{"counter with max_field", `{"key":"hp","label":"HP","type":"number","play":{"edit":"owner","kind":"counter","min":0,"max_field":"hp_max","step":1}}`, true, ""},
		{"full block kept", `{"key":"hp","label":"HP","type":"number","play":{"edit":"gm","kind":"resource","min":0,"max":10,"step":2,"to_foundry":false,"combat_authority":"chronicle"}}`, true, ""},
		{"conditions with options", `{"key":"c","label":"C","type":"string","play":{"edit":"owner","kind":"conditions","options":["dazed"]}}`, true, ""},
		{"choice with options", `{"key":"c","label":"C","type":"string","play":{"edit":"owner","kind":"choice","options":["a"],"max_length":20}}`, true, ""},
		{"text", `{"key":"c","label":"C","type":"string","play":{"edit":"none","kind":"text","max_length":50}}`, true, ""},
		{"min equals max", `{"key":"hp","label":"HP","type":"number","play":{"edit":"owner","kind":"counter","min":3,"max":3}}`, true, ""},
		{"gm_only with edit gm is fine", `{"key":"n","label":"N","type":"string","gm_only":true,"play":{"edit":"gm","kind":"text"}}`, true, ""},
		{"unknown kind", `{"key":"hp","label":"HP","type":"number","play":{"edit":"owner","kind":"slider"}}`, false, "unknown kind"},
		{"missing kind", `{"key":"hp","label":"HP","type":"number","play":{"edit":"owner"}}`, false, "unknown kind"},
		{"unknown edit", `{"key":"hp","label":"HP","type":"number","play":{"edit":"everyone","kind":"counter"}}`, false, "unknown edit"},
		{"missing edit", `{"key":"hp","label":"HP","type":"number","play":{"kind":"counter"}}`, false, "unknown edit"},
		{"unknown combat_authority", `{"key":"hp","label":"HP","type":"number","play":{"edit":"owner","kind":"counter","combat_authority":"dice"}}`, false, "combat_authority"},
		{"max_field missing", `{"key":"hp","label":"HP","type":"number","play":{"edit":"owner","kind":"counter","max_field":"nope"}}`, false, "not a field of this preset"},
		{"max_field not a number", `{"key":"hp","label":"HP","type":"number","play":{"edit":"owner","kind":"counter","max_field":"title"}}`, false, "not a number"},
		{"max_field is itself", `{"key":"hp","label":"HP","type":"number","play":{"edit":"owner","kind":"counter","max_field":"hp"}}`, false, "itself"},
		{"conditions without options", `{"key":"c","label":"C","type":"string","play":{"edit":"owner","kind":"conditions"}}`, false, "options"},
		{"choice with empty options", `{"key":"c","label":"C","type":"string","play":{"edit":"owner","kind":"choice","options":[]}}`, false, "options"},
		{"min above max", `{"key":"hp","label":"HP","type":"number","play":{"edit":"owner","kind":"counter","min":5,"max":1}}`, false, "greater than max"},
		{"negative step", `{"key":"hp","label":"HP","type":"number","play":{"edit":"owner","kind":"counter","step":-1}}`, false, "step"},
		{"negative max_length", `{"key":"c","label":"C","type":"string","play":{"edit":"owner","kind":"text","max_length":-1}}`, false, "max_length"},
		{"gm_only owner-editable", `{"key":"n","label":"N","type":"string","gm_only":true,"play":{"edit":"owner","kind":"text"}}`, false, "gm_only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := playManifest(t, tc.field)
			if err := ValidateManifest(m); err != nil {
				t.Fatalf("a bad play block must not fail the manifest: %v", err)
			}
			fields := m.EntityPresets[0].Fields
			f := fields[len(fields)-1]
			if tc.wantKept {
				if f.Play == nil {
					t.Fatalf("play block dropped, warnings: %v", m.PlayWarnings)
				}
				if len(m.PlayWarnings) != 0 {
					t.Errorf("unexpected warnings: %v", m.PlayWarnings)
				}
				return
			}
			if f.Play != nil {
				t.Fatalf("invalid play block kept: %+v", f.Play)
			}
			if f.Key == "" || f.Label == "" {
				t.Errorf("the field itself must survive: %+v", f)
			}
			if len(m.PlayWarnings) != 1 || !strings.Contains(m.PlayWarnings[0], tc.wantWarn) {
				t.Errorf("warnings = %v, want one containing %q", m.PlayWarnings, tc.wantWarn)
			}
			// The warning must reach the report the diagnostics page renders.
			if got := m.BuildValidationReport().Warnings; !containsStr(got, m.PlayWarnings[0]) {
				t.Errorf("report warnings %v lack %q", got, m.PlayWarnings[0])
			}
		})
	}
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestPlayBlockParsing(t *testing.T) {
	m := playManifest(t, `{"key":"hp","label":"HP","type":"number","play":{"edit":"owner","kind":"counter","min":0,"max":9.5,"max_field":"hp_max","step":2,"options":["a"],"max_length":7,"to_foundry":false,"combat_authority":"foundry"}}`)
	if err := ValidateManifest(m); err != nil {
		t.Fatal(err)
	}
	p := m.EntityPresets[0].Fields[2].Play
	if p == nil {
		t.Fatal("play not parsed")
	}
	if p.Edit != "owner" || p.Kind != "counter" || p.MaxField != "hp_max" || p.Step != 2 ||
		p.MaxLength != 7 || p.CombatAuthority != "foundry" || len(p.Options) != 1 {
		t.Errorf("fields not parsed: %+v", p)
	}
	if p.Min == nil || *p.Min != 0 {
		t.Errorf("an explicit min of 0 must be present, got %v", p.Min)
	}
	if p.Max == nil || *p.Max != 9.5 {
		t.Errorf("max = %v", p.Max)
	}
	if p.ToFoundry == nil || *p.ToFoundry {
		t.Errorf("to_foundry false must stay explicit, got %v", p.ToFoundry)
	}

	// Absent block and absent to_foundry stay nil.
	m2 := playManifest(t, `{"key":"x","label":"X","type":"number","play":{"edit":"gm","kind":"counter"}}`)
	if err := ValidateManifest(m2); err != nil {
		t.Fatal(err)
	}
	if got := m2.EntityPresets[0].Fields[2].Play.ToFoundry; got != nil {
		t.Errorf("absent to_foundry should be nil, got %v", *got)
	}
	if m2.EntityPresets[0].Fields[0].Play != nil {
		t.Error("a field with no play block must have nil Play")
	}
}

// Validating twice must neither lose nor duplicate the warning: the dropped
// block is gone on the second pass, so the first pass's warning has to stay.
func TestPlayWarningsSurviveRevalidate(t *testing.T) {
	m := playManifest(t, `{"key":"hp","label":"HP","type":"number","play":{"edit":"owner","kind":"nope"}}`)
	for i := 0; i < 2; i++ {
		if err := ValidateManifest(m); err != nil {
			t.Fatal(err)
		}
	}
	if len(m.PlayWarnings) != 1 {
		t.Errorf("warnings = %v, want exactly one", m.PlayWarnings)
	}
}
