package importer

import (
	"context"
	"reflect"
	"strings"
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
		stored  map[string]any
	}{
		{"create", "creature", map[string]any{"level": 1, "organization": "Platoon", "stamina": 20}, []string{"level", "organization", "stamina"}, nil},
		{"update keeps the page's own fields under the AI's", "creature", map[string]any{"level": 3, "organization": "Horde", "stamina": 40}, []string{"stamina"}, map[string]any{"level": 3, "organization": "Horde", "stamina": 20}},
		{"preset category", "character", map[string]any{"level": 2}, []string{"level"}, nil},
		{"a category no system made", "", nil, nil, nil},
		{"fields that don't fit", "", nil, nil, nil},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cls[i]
			if got.SheetKind != tt.kind || !reflect.DeepEqual(got.SheetFields, tt.fields) || !reflect.DeepEqual(got.SheetWritten, tt.written) || !reflect.DeepEqual(got.SheetStored, tt.stored) {
				t.Errorf("got kind %q fields %#v written %#v stored %#v; want %q %#v %#v %#v", got.SheetKind, got.SheetFields, got.SheetWritten, got.SheetStored, tt.kind, tt.fields, tt.written, tt.stored)
			}
		})
	}
}

// The review screen's formula numbers replace only what the AI wrote.
func TestUseSheetFormula(t *testing.T) {
	tests := []struct {
		name    string
		fields  map[string]any
		raw     string
		want    map[string]any
		wantErr bool
	}{
		{"replaces under the AI's spelling", map[string]any{"Stamina": 20, "ev": 6}, `{"stamina": 30}`, map[string]any{"Stamina": 30.0, "ev": 6}, false},
		{"never adds a field", map[string]any{"stamina": 20}, `{"stamina": 30, "winded": 15}`, map[string]any{"stamina": 30.0}, false},
		{"nothing pressed", map[string]any{"stamina": 20}, "", map[string]any{"stamina": 20}, false},
		{"unreadable is reported", map[string]any{"stamina": 20}, `{"stamina":`, map[string]any{"stamina": 20}, true},
		{"not an object is reported", map[string]any{"stamina": 20}, `[30]`, map[string]any{"stamina": 20}, true},
		{"over 64 KB is reported", map[string]any{"stamina": 20}, `{"stamina": "` + strings.Repeat("9", maxSheetFormula) + `"}`, map[string]any{"stamina": 20}, true},
		{"no fields", nil, `{"stamina": 30}`, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UseSheetFormula(tt.fields, tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, want error %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

// The review adds an unwritten number only where the category declares it a
// formula figure; everything else the AI left out stays out.
func TestUseDeclaredFormula(t *testing.T) {
	defs := []entities.FieldDefinition{
		{Key: "level", Type: "number"},
		{Key: "ev", Type: "number", Formula: true},
		{Key: "stamina", Type: "number", Formula: true},
	}
	tests := []struct {
		name   string
		fields map[string]any
		raw    string
		want   map[string]any
	}{
		{"adds declared figures the AI left out", map[string]any{"level": 2}, `{"ev": 4, "Stamina": 20}`, map[string]any{"level": 2, "ev": 4.0, "stamina": 20.0}},
		{"refuses a field not declared", map[string]any{"level": 2}, `{"level": 9, "hp": 1}`, map[string]any{"level": 2}},
		{"leaves a key the AI wrote to UseSheetFormula", map[string]any{"ev": 6}, `{"ev": 4}`, map[string]any{"ev": 6}},
		{"nothing pressed", map[string]any{"level": 2}, "", map[string]any{"level": 2}},
		{"unreadable adds nothing", map[string]any{"level": 2}, `{"ev":`, map[string]any{"level": 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := map[string]any{}
			for k, v := range tt.fields {
				before[k] = v
			}
			got := UseDeclaredFormula(defs, tt.fields, tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.fields, before) {
				t.Errorf("input changed to %#v", tt.fields)
			}
		})
	}
}
