package systems

import (
	"reflect"
	"testing"
)

// The hero's picks are the class field and the single-item hero choices, as
// the Foundry module reads them; subclass and the class item's read-outs are
// not picks, and a system with no mapping falls back to a "class" field.
func TestHeroPickKeys(t *testing.T) {
	item := func(key string, types ...string) FieldDef {
		return FieldDef{Key: key, FoundryCollection: "items", FoundryItemSingle: true, FoundryItemType: types}
	}
	drawSteel := &EntityPresetDef{Fields: []FieldDef{
		item("class", "class"), item("subclass", "subclass"), item("ancestry", "ancestry"),
		item("culture", "culture"), item("career", "career"), item("level", "class"),
		item("kit", "kit"), item("heroic_resource_name", "class"),
		{Key: "abilities_json", FoundryCollection: "items", FoundryItemType: []string{"ability"}},
		{Key: "stamina", FoundryPath: "system.stamina.value"},
	}}
	tests := []struct {
		name   string
		preset *EntityPresetDef
		want   []string
	}{
		{"no system", nil, []string{"class"}},
		{"unmapped class (D&D)", &EntityPresetDef{Fields: []FieldDef{{Key: "class"}, {Key: "race"}}}, []string{"class"}},
		{"Draw Steel", drawSteel, []string{"class", "ancestry", "culture", "career", "kit"}},
		{"renamed class field", &EntityPresetDef{Fields: []FieldDef{item("calling", "class")}}, []string{"calling"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.preset.HeroPickKeys(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
