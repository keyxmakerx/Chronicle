package systems

import (
	"encoding/json"
	"strings"
	"testing"
)

// An ancestry field turns its signature traits into Foundry features; a
// mapping the module couldn't use is dropped with a warning.
func TestEntryItemValidation(t *testing.T) {
	const ancestry = `{"key":"ancestry","label":"Ancestry","type":"string","foundry_collection":"items","foundry_item_type":["ancestry"],"foundry_item_single":true`
	cases := []struct {
		name     string
		field    string
		wantKept int
		wantWarn string // substring of a warning, "" for none
	}{
		{"signature traits as features", ancestry + `,"foundry_entry_items":{"signature_traits":{"type":"feature"}}}`, 1, ""},
		{"a type that is not an identifier", ancestry + `,"foundry_entry_items":{"signature_traits":{"type":"feature.x"}}}`, 0, "type must be"},
		{"a missing type", ancestry + `,"foundry_entry_items":{"signature_traits":{}}}`, 0, "type must be"},
		{"a type too long", ancestry + `,"foundry_entry_items":{"signature_traits":{"type":"` + strings.Repeat("a", 33) + `"}}}`, 0, "type must be"},
		{"a property that is not an identifier", ancestry + `,"foundry_entry_items":{"a b":{"type":"feature"}}}`, 0, "plain identifier"},
		{"the field's own identity type", ancestry + `,"foundry_entry_items":{"signature_traits":{"type":"ancestry"}}}`, 0, "single-item field syncs"},
		{"a GM-only field", ancestry + `,"gm_only":true,"foundry_entry_items":{"signature_traits":{"type":"feature"}}}`, 0, "GM-only"},
		{"not a single item", `{"key":"abilities","label":"A","type":"string","foundry_collection":"items","foundry_item_type":["ability"],
			"foundry_entry_items":{"x":{"type":"feature"}}}`, 0, "not a single Foundry item"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := entryFieldsManifest(t, tc.field)
			if got := len(m.EntityPresets[0].Fields[0].FoundryEntryItems); got != tc.wantKept {
				t.Fatalf("kept %d, want %d", got, tc.wantKept)
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
func TestEntryItemsExported(t *testing.T) {
	m := entryFieldsManifest(t, `{"key":"ancestry","label":"Ancestry","type":"string","foundry_collection":"items","foundry_item_type":["ancestry"],"foundry_item_single":true,
		"foundry_entry_items":{"signature_traits":{"type":"feature"}}}`)
	raw, _ := json.Marshal(m.CharacterFieldsForAPI().Fields[0])
	if !strings.Contains(string(raw), `"foundry_entry_items":{"signature_traits":{"type":"feature"}}`) {
		t.Fatalf("wire shape: %s", raw)
	}
	m.EntityPresets[0].Fields[0].GMOnly = true
	if got := m.CharacterFieldsForAPI().Fields[0].FoundryEntryItems; got != nil {
		t.Fatalf("a GM-only field must go out unmapped: %v", got)
	}
}
