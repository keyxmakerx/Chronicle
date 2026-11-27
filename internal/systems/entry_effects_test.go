package systems

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// An ancestry field maps entry properties onto changes to the hero; a mapping
// that could write outside the hero's system data, or that the module can't
// apply, is dropped with a warning and the rest of the field survives.
func TestEntryEffectValidation(t *testing.T) {
	const ancestry = `{"key":"ancestry","label":"Ancestry","type":"string","foundry_collection":"items","foundry_item_type":["ancestry"],"foundry_item_single":true`
	cases := []struct {
		name     string
		field    string
		wantKeys string // the mapping's kept property keys, sorted and comma-joined
		wantWarn string // substring of a warning, "" for none
	}{
		{"speed, size and stability kept", ancestry + `,"foundry_entry_effects":{
			"speed":{"path":"system.movement.value","type":"upgrade","format":"number"},
			"size":{"path":"system.combat.size","type":"override","format":"size"},
			"stability":{"path":"system.combat.stability","type":"add","format":"number"}}}`, "size,speed,stability", ""},
		{"a path outside system data", ancestry + `,"foundry_entry_effects":{
			"speed":{"path":"system.movement.value","type":"upgrade","format":"number"},
			"light":{"path":"token.light.bright","type":"upgrade","format":"number"}}}`, "speed", `"light" ignored: path`},
		{"a reserved segment", ancestry + `,"foundry_entry_effects":{"x":{"path":"system.constructor.x","type":"add","format":"number"}}}`, "", "path"},
		{"an underscore segment", ancestry + `,"foundry_entry_effects":{"x":{"path":"system.__proto__.x","type":"add","format":"number"}}}`, "", "path"},
		{"an unknown type", ancestry + `,"foundry_entry_effects":{"x":{"path":"system.a","type":"custom","format":"number"}}}`, "", "type must be"},
		{"a missing type", ancestry + `,"foundry_entry_effects":{"x":{"path":"system.a","format":"number"}}}`, "", "type must be"},
		{"text is not a change", ancestry + `,"foundry_entry_effects":{"x":{"path":"system.a","type":"override","format":"text"}}}`, "", "format must be"},
		{"a size added", ancestry + `,"foundry_entry_effects":{"size":{"path":"system.combat.size","type":"add","format":"size"}}}`, "", "only be an override"},
		{"a property that is not an identifier", ancestry + `,"foundry_entry_effects":{"a b":{"path":"system.a","type":"add","format":"number"}}}`, "", "plain identifier"},
		{"not a single item", `{"key":"abilities","label":"A","type":"string","foundry_collection":"items","foundry_item_type":["ability"],
			"foundry_entry_effects":{"x":{"path":"system.a","type":"add","format":"number"}}}`, "", "not a single Foundry item"},
		{"a GM-only field", ancestry + `,"gm_only":true,"foundry_entry_effects":{"x":{"path":"system.a","type":"add","format":"number"}}}`, "", "GM-only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := entryFieldsManifest(t, tc.field)
			got := m.EntityPresets[0].Fields[0].FoundryEntryEffects
			keys := make([]string, 0, len(got))
			for k := range got {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			if strings.Join(keys, ",") != tc.wantKeys {
				t.Fatalf("kept %v, want %q", keys, tc.wantKeys)
			}
			warns := strings.Join(m.EntryFieldWarnings, "\n")
			if tc.wantWarn == "" && warns != "" {
				t.Fatalf("unexpected warning: %s", warns)
			}
			if tc.wantWarn != "" && !strings.Contains(warns, tc.wantWarn) {
				t.Fatalf("warnings %q, want one saying %q", warns, tc.wantWarn)
			}
		})
	}
}

// The mapping reaches the Foundry module through character-fields, except on
// a GM-only field.
func TestEntryEffectsExported(t *testing.T) {
	m := entryFieldsManifest(t, `{"key":"ancestry","label":"Ancestry","type":"string","foundry_collection":"items","foundry_item_type":["ancestry"],"foundry_item_single":true,
		"foundry_entry_effects":{"speed":{"path":"system.movement.value","type":"upgrade","format":"number"}}}`)
	out := m.CharacterFieldsForAPI()
	if out == nil || len(out.Fields) != 1 {
		t.Fatalf("export: %+v", out)
	}
	raw, _ := json.Marshal(out.Fields[0])
	if !strings.Contains(string(raw), `"foundry_entry_effects":{"speed":{"path":"system.movement.value","type":"upgrade","format":"number"}}`) {
		t.Fatalf("wire shape: %s", raw)
	}
	m.EntityPresets[0].Fields[0].GMOnly = true
	if got := m.CharacterFieldsForAPI().Fields[0].FoundryEntryEffects; got != nil {
		t.Fatalf("a GM-only field must go out unmapped: %v", got)
	}
}
