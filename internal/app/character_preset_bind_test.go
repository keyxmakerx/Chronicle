package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

type fakeBindTypes struct {
	types   map[string][]entities.EntityType
	adopted []string
	added   map[int]int
}

func (f *fakeBindTypes) GetEntityTypes(_ context.Context, id string) ([]entities.EntityType, error) {
	return f.types[id], nil
}

func (f *fakeBindTypes) AdoptDefaultCharacterType(_ context.Context, id, _ string, declared []entities.FieldDefinition) (*entities.EntityType, error) {
	if len(declared) != 0 {
		panic("the heal must not re-add Characters' fields")
	}
	f.adopted = append(f.adopted, id)
	return &entities.EntityType{ID: 1}, nil
}

func (f *fakeBindTypes) ReconcileEntityTypeFields(_ context.Context, id int, declared []entities.FieldDefinition) (int, error) {
	f.added[id] += len(declared)
	return len(declared), nil
}

// Only a campaign the name match left unbound is healed, its empty Player
// Character category gets the sheet's fields, and the sweep runs once.
func TestBindCharacterPresetOnce(t *testing.T) {
	pc := entities.PresetCategoryPlayerCharacter
	bound := presetCategoryCharacter
	parent := 1
	dnd := &systems.SystemManifest{ID: "dnd5e", EntityPresets: []systems.EntityPresetDef{
		{Slug: "dnd5e-character", Name: "Character", Category: "character", Fields: []systems.FieldDef{
			{Key: "race", Label: "Race", Type: "string"}, {Key: "class", Label: "Class", Type: "string"}}},
	}}
	hero := &systems.SystemManifest{ID: "drawsteel", EntityPresets: []systems.EntityPresetDef{
		{Slug: "drawsteel-character", Name: "Hero", Category: "character"},
	}}
	ents := &fakeBindTypes{added: map[int]int{}, types: map[string][]entities.EntityType{
		// The bug: Characters matched by name, an empty Player Character.
		"broken": {{ID: 1, Slug: "character", Name: "Character", Fields: []entities.FieldDefinition{{Key: "race"}}},
			{ID: 2, Slug: "player-character", PresetCategory: &pc, ParentTypeID: &parent}},
		// Already bound: left alone.
		"fine": {{ID: 3, Slug: "character", Name: "Character", PresetCategory: &bound}},
		// The owner renamed Characters: not a name match, left alone.
		"renamed": {{ID: 4, Slug: "character", Name: "Heroes"}},
		// A system whose preset is named otherwise took the adopt path.
		"hero": {{ID: 5, Slug: "character", Name: "Character"}},
	}}
	manifests := map[string]*systems.SystemManifest{"broken": dnd, "fine": dnd, "renamed": dnd, "hero": hero}
	manifest := func(_ context.Context, id string) *systems.SystemManifest { return manifests[id] }
	st := &fakeExtrasSettings{vals: map[string]string{}}
	camps := fakeExtrasCampaigns{ids: []string{"broken", "fine", "renamed", "hero", "none"}}

	n, err := bindCharacterPresetOnce(context.Background(), st, camps, manifest, ents)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(ents.adopted) != 1 || ents.adopted[0] != "broken" {
		t.Fatalf("healed %d, adopted %v; want only the broken campaign", n, ents.adopted)
	}
	if ents.added[2] != 2 || len(ents.added) != 1 {
		t.Fatalf("fields added %v, want the 2 sheet fields on the empty Player Character only", ents.added)
	}
	if st.vals[characterPresetBindKey] != "1" {
		t.Error("run was not recorded")
	}
	ents.adopted = nil
	if n, _ := bindCharacterPresetOnce(context.Background(), st, camps, manifest, ents); n != 0 || ents.adopted != nil {
		t.Fatalf("second run healed %d", n)
	}
}
