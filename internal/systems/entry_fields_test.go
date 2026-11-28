package systems

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// entryFieldsManifest builds a manifest whose one preset holds the field
// under test, and runs the load-time checks.
func entryFieldsManifest(t *testing.T, fieldJSON string) *SystemManifest {
	t.Helper()
	raw := `{
		"id": "entry-test", "name": "Entry Test", "api_version": "1",
		"entity_presets": [{
			"slug": "entry-character", "name": "Hero", "name_plural": "Heroes", "icon": "fa-user",
			"fields": [` + fieldJSON + `]
		}]
	}`
	var m SystemManifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	m.sanitizeEntryFields()
	m.sanitizeEntryEffects()
	return &m
}

// A kit field maps entry properties onto its Foundry item; anything that
// could write outside the item's system data, or that core can't describe to
// the module, is dropped with a warning and the rest of the field survives.
func TestEntryFieldValidation(t *testing.T) {
	const kit = `{"key":"kit","label":"Kit","type":"string","foundry_collection":"items","foundry_item_type":["kit"],"foundry_item_single":true`
	cases := []struct {
		name     string
		field    string
		wantKeys string // the mapping's kept property keys, sorted and comma-joined
		wantWarn string // substring of a warning, "" for none
	}{
		{"number, text and tiers kept", kit + `,"foundry_entry_fields":{
			"stamina_bonus":{"path":"system.bonuses.stamina","format":"number"},
			"melee_damage_bonus":{"path":"system.bonuses.melee.damage","format":"tiers"},
			"armor":{"path":"system.equipment.armor","format":"text"}}}`, "armor,melee_damage_bonus,stamina_bonus", ""},
		{"a path outside system data", kit + `,"foundry_entry_fields":{
			"stamina_bonus":{"path":"system.bonuses.stamina","format":"number"},
			"owner":{"path":"ownership.default","format":"number"}}}`, "stamina_bonus", `"owner" ignored: path`},
		{"flags are not system data", kit + `,"foundry_entry_fields":{"x":{"path":"flags.chronicle-sync.entrySlug","format":"text"}}}`, "", "path"},
		{"system alone is not a path", kit + `,"foundry_entry_fields":{"x":{"path":"system","format":"text"}}}`, "", "path"},
		{"a path with odd characters", kit + `,"foundry_entry_fields":{"x":{"path":"system.a[0]","format":"text"}}}`, "", "path"},
		{"a segment starting with an underscore", kit + `,"foundry_entry_fields":{"x":{"path":"system.__proto__.polluted","format":"text"}}}`, "", "path"},
		{"a segment starting with a digit", kit + `,"foundry_entry_fields":{"x":{"path":"system.1a","format":"text"}}}`, "", "path"},
		{"a reserved segment", kit + `,"foundry_entry_fields":{"x":{"path":"system.constructor.prototype.x","format":"text"}}}`, "", "path"},
		{"more than eight segments under system", kit + `,"foundry_entry_fields":{"x":{"path":"system.a.b.c.d.e.f.g.h.i","format":"text"}}}`, "", "path"},
		{"an unknown format", kit + `,"foundry_entry_fields":{"x":{"path":"system.a","format":"html"}}}`, "", "format"},
		{"a missing format", kit + `,"foundry_entry_fields":{"x":{"path":"system.a"}}}`, "", "format"},
		{"a property that is not an identifier", kit + `,"foundry_entry_fields":{"a b":{"path":"system.a","format":"text"}}}`, "", "plain identifier"},
		{"not a single item", `{"key":"abilities","label":"A","type":"string","foundry_collection":"items","foundry_item_type":["ability"],
			"foundry_entry_fields":{"x":{"path":"system.a","format":"text"}}}`, "", "not a single Foundry item"},
		{"a scalar field", `{"key":"hp","label":"HP","type":"number","foundry_path":"system.hp",
			"foundry_entry_fields":{"x":{"path":"system.a","format":"number"}}}`, "", "not a single Foundry item"},
		{"a GM-only field", kit + `,"gm_only":true,"foundry_entry_fields":{"x":{"path":"system.a","format":"text"}}}`, "", "GM-only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := entryFieldsManifest(t, tc.field)
			got := m.EntityPresets[0].Fields[0].FoundryEntryFields
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
// a GM-only field, which goes out with no Foundry mapping at all.
func TestEntryFieldsExported(t *testing.T) {
	m := entryFieldsManifest(t, `{"key":"kit","label":"Kit","type":"string","foundry_collection":"items","foundry_item_type":["kit"],"foundry_item_single":true,
		"foundry_entry_fields":{"speed_bonus":{"path":"system.bonuses.speed","format":"number"}}}`)
	out := m.CharacterFieldsForAPI()
	if out == nil || len(out.Fields) != 1 {
		t.Fatalf("export: %+v", out)
	}
	if got := out.Fields[0].FoundryEntryFields["speed_bonus"]; got.Path != "system.bonuses.speed" || got.Format != "number" {
		t.Fatalf("exported mapping %+v", got)
	}
	raw, _ := json.Marshal(out.Fields[0])
	if !strings.Contains(string(raw), `"foundry_entry_fields":{"speed_bonus":{"path":"system.bonuses.speed","format":"number"}}`) {
		t.Fatalf("wire shape: %s", raw)
	}
	m.EntityPresets[0].Fields[0].GMOnly = true
	if got := m.CharacterFieldsForAPI().Fields[0].FoundryEntryFields; got != nil {
		t.Fatalf("a GM-only field must go out unmapped: %v", got)
	}
}
