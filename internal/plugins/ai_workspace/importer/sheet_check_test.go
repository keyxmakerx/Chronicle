package importer

import (
	"context"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// A row a game system can check carries its sheet as it would be saved and
// the keys the AI wrote; anything else carries nothing for the check.
func TestClassify_SheetCheck(t *testing.T) {
	character := "character"
	types := append(creatureTypes(),
		entities.EntityType{ID: 8, Name: "Hero", Slug: "hero", Enabled: true, PresetCategory: &character, Fields: creatureFields()},
		entities.EntityType{ID: 9, Name: "Note", Slug: "note", Enabled: true, Fields: creatureFields()},
	)
	lookup := &stubLookup{types: types, bySlug: map[string]*entities.Entity{
		"orc-brute": {ID: "ent-9", Name: "Orc Brute", Slug: "orc-brute", CampaignID: "camp-1", EntityTypeID: 7,
			FieldsData: map[string]any{"level": 3, "organization": "Horde", "stamina": 20}},
	}}
	update := creaturePage("Orc Brute", map[string]any{"Stamina": 40})
	update.FrontMatter.Action = ActionUpdate
	hero := creaturePage("Ash", map[string]any{"level": 2})
	hero.FrontMatter.Type = "hero"
	note := creaturePage("Ledger", map[string]any{"level": 2})
	note.FrontMatter.Type = "note"
	pages := []ParsedPage{
		creaturePage("Goblin Sniper", map[string]any{"level": 1, "organization": "Platoon", "stamina": 20}),
		update,
		hero,
		note,
		creaturePage("Orc Shaman", map[string]any{"hp": 9}),
	}
	cls, err := NewClassifier(lookup, "camp-1").ClassifyAll(context.Background(), pages)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		kind    string
		fields  map[string]any
		written []string
	}{
		{"create", "creature", map[string]any{"level": 1, "organization": "Platoon", "stamina": 20}, []string{"level", "organization", "stamina"}},
		{"update keeps the page's own fields under the AI's", "creature", map[string]any{"level": 3, "organization": "Horde", "stamina": 40}, []string{"stamina"}},
		{"preset category", "character", map[string]any{"level": 2}, []string{"level"}},
		{"a category no system made", "", nil, nil},
		{"fields that don't fit", "", nil, nil},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cls[i]
			if got.SheetKind != tt.kind || !reflect.DeepEqual(got.SheetFields, tt.fields) || !reflect.DeepEqual(got.SheetWritten, tt.written) {
				t.Errorf("got kind %q fields %#v written %#v; want %q %#v %#v", got.SheetKind, got.SheetFields, got.SheetWritten, tt.kind, tt.fields, tt.written)
			}
		})
	}
}

// The review screen's formula numbers replace only what the AI wrote.
func TestUseSheetFormula(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]any
		raw    string
		want   map[string]any
	}{
		{"replaces under the AI's spelling", map[string]any{"Stamina": 20, "ev": 6}, `{"stamina": 30}`, map[string]any{"Stamina": 30.0, "ev": 6}},
		{"never adds a field", map[string]any{"stamina": 20}, `{"stamina": 30, "winded": 15}`, map[string]any{"stamina": 30.0}},
		{"nothing pressed", map[string]any{"stamina": 20}, "", map[string]any{"stamina": 20}},
		{"unreadable", map[string]any{"stamina": 20}, `{"stamina":`, map[string]any{"stamina": 20}},
		{"not an object", map[string]any{"stamina": 20}, `[30]`, map[string]any{"stamina": 20}},
		{"no fields", nil, `{"stamina": 30}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UseSheetFormula(tt.fields, tt.raw); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}
