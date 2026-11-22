package app

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

type fakeSheetTypes struct {
	types map[string][]entities.EntityType
	added map[int][]string
}

func (f *fakeSheetTypes) GetEntityTypes(_ context.Context, id string) ([]entities.EntityType, error) {
	return f.types[id], nil
}

func (f *fakeSheetTypes) ReconcileEntityTypeFields(_ context.Context, id int, declared []entities.FieldDefinition) (int, error) {
	for _, d := range declared {
		f.added[id] = append(f.added[id], d.Key)
	}
	return len(declared), nil
}

type fakePCLists map[string][]int

func (f fakePCLists) CharacterTypeIDs(_ context.Context, id string) ([]int, error) { return f[id], nil }
func (f fakePCLists) NPCTypeIDs(context.Context, string) ([]int, error)            { return nil, nil }

func sheetManifest() *systems.SystemManifest {
	no := false
	return &systems.SystemManifest{ID: "drawsteel", EntityPresets: []systems.EntityPresetDef{
		{Slug: "drawsteel-character", Name: "Hero", Category: "character", Fields: []systems.FieldDef{
			{Key: "class", Label: "Class", Type: "string", FoundryCollection: "items"},
			{Key: "stamina_max", Label: "Stamina", Type: "number", FoundryPath: "system.stamina.max", FoundryWritable: &no},
			{Key: "wealth", Label: "Wealth", Type: "number", FoundryPath: "system.hero.wealth"},
		}},
		{Slug: "drawsteel-creature", Name: "Creature", Category: "creature", Fields: []systems.FieldDef{
			{Key: "level", Label: "Level", Type: "number"},
			{Key: "abilities_json", Label: "Abilities", Type: "string", Format: "a JSON list; each ability has name and type"},
			{Key: "book_source", Label: "Book source", Type: "string", GMOnly: true},
		}},
	}}
}

// The heroes' category and the monsters' category get the system's fields,
// whatever they are called; a campaign without a system is left alone.
func TestAddSystemSheetFieldsOnce(t *testing.T) {
	types := []entities.EntityType{
		{ID: 1, Slug: "character", Name: "Character", Enabled: true},
		{ID: 2, Slug: "creature", Name: "Creature", Enabled: true},
		{ID: 3, Slug: "location", Name: "Location", Enabled: true},
	}
	st := &fakeExtrasSettings{vals: map[string]string{}}
	ents := &fakeSheetTypes{types: map[string][]entities.EntityType{"c1": types, "c2": types}, added: map[int][]string{}}
	manifest := func(_ context.Context, id string) *systems.SystemManifest {
		if id == "c1" {
			return sheetManifest()
		}
		return nil
	}
	n, err := addSystemSheetFieldsOnce(context.Background(), st, fakeExtrasCampaigns{ids: []string{"c1", "c2"}}, manifest, ents, fakePCLists{"c1": {1}})
	if err != nil {
		t.Fatal(err)
	}
	if n != 6 || strings.Join(ents.added[1], ",") != "class,stamina_max,wealth" || strings.Join(ents.added[2], ",") != "level,abilities_json,book_source" || ents.added[3] != nil {
		t.Fatalf("added %d: %v", n, ents.added)
	}
	if st.vals[systemSheetFieldsKey] != "1" {
		t.Error("run was not recorded")
	}
	ents.added = map[int][]string{}
	if n, _ := addSystemSheetFieldsOnce(context.Background(), st, fakeExtrasCampaigns{ids: []string{"c1"}}, manifest, ents, fakePCLists{"c1": {1}}); n != 0 || len(ents.added) != 0 {
		t.Fatalf("second run added %d", n)
	}
}

func TestAIFieldNotes(t *testing.T) {
	types := []entities.EntityType{{ID: 1, Slug: "character", Enabled: true}, {ID: 2, Slug: "creature", Enabled: true}}
	n := aiFieldNotes{
		manifest: func(context.Context, string) *systems.SystemManifest { return sheetManifest() },
		lists:    fakePCLists{"c1": {1}},
		types:    &fakeSheetTypes{types: map[string][]entities.EntityType{"c1": types}},
	}
	hero := n.FieldNotes(context.Background(), "c1", types[0])
	if !strings.Contains(hero["class"], "Foundry keeps") || !strings.Contains(hero["stamina_max"], "Foundry keeps") || hero["wealth"] != "" {
		t.Errorf("hero notes = %v; want class and stamina_max kept by Foundry, wealth free", hero)
	}
	monster := n.FieldNotes(context.Background(), "c1", types[1])
	if monster["abilities_json"] != "a JSON list; each ability has name and type" || len(monster) != 1 {
		t.Errorf("monster notes = %v", monster)
	}
}

// A field the category carries from another game is named as not the
// system's, so the AI leaves race or challenge_rating alone in a Draw Steel
// campaign; a category the system has no sheet for gets no such note.
func TestAIFieldNotes_MarksOtherSystemsFields(t *testing.T) {
	m := sheetManifest()
	m.Name = "Draw Steel"
	creature := entities.EntityType{ID: 2, Slug: "creature", Enabled: true, Fields: []entities.FieldDefinition{
		{Key: "level", Type: "number"}, {Key: "challenge_rating", Type: "text"},
	}}
	location := entities.EntityType{ID: 3, Slug: "location", Enabled: true, Fields: []entities.FieldDefinition{{Key: "population", Type: "number"}}}
	n := aiFieldNotes{
		manifest: func(context.Context, string) *systems.SystemManifest { return m },
		lists:    fakePCLists{},
		types:    &fakeSheetTypes{types: map[string][]entities.EntityType{"c1": {creature, location}}},
	}
	notes := n.FieldNotes(context.Background(), "c1", creature)
	if !strings.Contains(notes["challenge_rating"], "Not a Draw Steel field") || notes["level"] != "" {
		t.Errorf("creature notes = %v", notes)
	}
	if notes := n.FieldNotes(context.Background(), "c1", location); len(notes) != 0 {
		t.Errorf("location notes = %v", notes)
	}
}

// A creature's live Stamina and conditions are what Foundry sends back, so
// the AI is told to leave them out; a bounded number says its bounds.
func TestAIFieldNotes_CreatureFoundryPullAndBounds(t *testing.T) {
	one, twenty := 1.0, 20.0
	m := &systems.SystemManifest{ID: "drawsteel", EntityPresets: []systems.EntityPresetDef{
		{Slug: "creature", Category: "creature", FoundryPullFields: []string{"stamina_current", "conditions_json"}, Fields: []systems.FieldDef{
			{Key: "level", Label: "Level", Type: "number", Min: &one, Max: &twenty},
			{Key: "stamina", Label: "Stamina", Type: "number", Min: &one},
			{Key: "stamina_current", Label: "Current Stamina", Type: "number", Min: &one},
			{Key: "conditions_json", Label: "Conditions", Type: "string", Format: "a JSON list"},
			{Key: "name_note", Label: "Note", Type: "string"},
		}},
	}}
	creature := entities.EntityType{ID: 2, Slug: "creature", Enabled: true}
	n := aiFieldNotes{
		manifest: func(context.Context, string) *systems.SystemManifest { return m },
		lists:    fakePCLists{},
		types:    &fakeSheetTypes{types: map[string][]entities.EntityType{"c1": {creature}}},
	}
	notes := n.FieldNotes(context.Background(), "c1", creature)
	tests := []struct{ key, want string }{
		{"stamina_current", "Foundry keeps this one: leave it out"},
		{"conditions_json", "a JSON list. Foundry keeps this one: leave it out"},
		{"level", "A number from 1 to 20"},
		{"stamina", "A number of at least 1"},
		{"name_note", ""},
	}
	for _, tc := range tests {
		if notes[tc.key] != tc.want {
			t.Errorf("%s note = %q, want %q", tc.key, notes[tc.key], tc.want)
		}
	}
}

type fakeCreatorPlan struct{ steps []systems.CreatorStep }

func (f fakeCreatorPlan) CreatorPlan(context.Context, string) (*systems.CreatorPlan, error) {
	return &systems.CreatorPlan{Steps: f.steps}, nil
}

// A hero field with a pick list says the value is a name from the campaign's
// list and where to read it; Foundry's "leave it out" shows only when a
// Foundry world is linked, and then wins over the pick-list note.
func TestAIFieldNotes_PickListsAndFoundryLink(t *testing.T) {
	m := sheetManifest()
	m.EntityPresets[0].Fields = append(m.EntityPresets[0].Fields,
		systems.FieldDef{Key: "ancestry", Label: "Ancestry", Type: "string"},
		systems.FieldDef{Key: "subclass", Label: "Subclass", Type: "string"})
	hero := entities.EntityType{ID: 1, Slug: "character", Enabled: true}
	plan := fakeCreatorPlan{steps: []systems.CreatorStep{
		{FieldKey: "class"}, {FieldKey: "ancestry"}, {FieldKey: "subclass", Parent: "class"},
	}}
	yes := func(context.Context, string) bool { return true }
	no := func(context.Context, string) bool { return false }
	tests := []struct {
		name   string
		linked func(context.Context, string) bool
		key    string
		want   []string
		not    []string
	}{
		{"pick list", yes, "ancestry", []string{"A name from the campaign's `ancestry` list", "`system-entries` lookup", "`system_entries`"}, []string{"Foundry keeps"}},
		{"dependent list", yes, "subclass", []string{"must belong to the chosen `class`"}, nil},
		{"kept and linked", yes, "class", []string{"Foundry keeps this one: leave it out"}, []string{"system-entries"}},
		{"kept, not linked", no, "class", []string{"A name from the campaign's `class` list"}, []string{"Foundry keeps"}},
		{"unwired link reads as linked", nil, "class", []string{"Foundry keeps"}, nil},
		{"no list, not linked", no, "stamina_max", nil, []string{"Foundry keeps"}},
		{"no list, linked", yes, "stamina_max", []string{"Foundry keeps"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := aiFieldNotes{
				manifest: func(context.Context, string) *systems.SystemManifest { return m },
				lists:    fakePCLists{"c1": {1}},
				types:    &fakeSheetTypes{types: map[string][]entities.EntityType{"c1": {hero}}},
				choices:  plan,
				linked:   tc.linked,
			}
			got := n.FieldNotes(context.Background(), "c1", hero)[tc.key]
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("note %q lacks %q", got, w)
				}
			}
			for _, w := range tc.not {
				if strings.Contains(got, w) {
					t.Errorf("note %q holds %q", got, w)
				}
			}
		})
	}
}
