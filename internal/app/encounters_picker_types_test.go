package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

type pickerTypesEntities struct {
	entities.EntityService // embed: unimplemented methods panic if hit
	types                  []entities.EntityType
}

func (p *pickerTypesEntities) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return p.types, nil
}

type pickerTypesLists struct{ chars, npcs []int }

func (p pickerTypesLists) CharacterTypeIDs(context.Context, string) ([]int, error) {
	return p.chars, nil
}
func (p pickerTypesLists) NPCTypeIDs(context.Context, string) ([]int, error) { return p.npcs, nil }

// The builder's pickers follow the campaign's own lists: a default campaign's
// Character and Creature types carry no preset, so a preset match finds none.
func TestEncountersPickerTypes(t *testing.T) {
	creature := "creature"
	types := []entities.EntityType{
		{ID: 1, Slug: "character", Enabled: true},
		{ID: 5, Slug: "creature", Enabled: true},
		{ID: 10, Slug: "monster", PresetCategory: &creature, Enabled: true},
		{ID: 13, Slug: "player-character", Enabled: true},
	}
	tests := []struct {
		name          string
		lists         pickerTypesLists
		wantHeroes    []int
		wantCreatures []int
	}{
		{"default campaign", pickerTypesLists{chars: []int{1, 13}, npcs: []int{1}}, []int{1, 13}, []int{5, 10, 1}},
		{"NPC list that is a creature type is not repeated", pickerTypesLists{chars: []int{1}, npcs: []int{5}}, []int{1}, []int{5, 10}},
		{"no NPC list", pickerTypesLists{chars: []int{13}}, []int{13}, []int{5, 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &encountersPageAdapter{svc: &pickerTypesEntities{types: types}, lists: tt.lists}
			got, err := a.PickerTypes(context.Background(), "c1")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Heroes, tt.wantHeroes) || !reflect.DeepEqual(got.Creatures, tt.wantCreatures) {
				t.Fatalf("got heroes %v creatures %v, want %v %v", got.Heroes, got.Creatures, tt.wantHeroes, tt.wantCreatures)
			}
		})
	}
}

// A creature row folds out the system's stat block: the panels it aims at
// creature pages, also on the NPC pages that widget is aimed at. Other NPC
// panels are not stat blocks, and a system with none leaves the page's fields.
func TestEncountersPickerTypes_StatBlocks(t *testing.T) {
	creature := "creature"
	types := []entities.EntityType{
		{ID: 1, Slug: "character", Enabled: true},
		{ID: 10, Slug: "monster", PresetCategory: &creature, Enabled: true},
	}
	tests := []struct {
		name string
		sys  enabledSystemResolver
		want []encounters.StatBlock
	}{
		{"no system resolver", nil, nil},
		{"no system enabled", fakeEnabled{}, nil},
		{"system without panels", fakeEnabled{sys: fakeSystem{manifest: &systems.SystemManifest{ID: "dnd"}}}, nil},
		{"stat block for creatures and NPCs, negotiation left out", fakeEnabled{sys: fakeSystem{manifest: &systems.SystemManifest{
			ID: "drawsteel",
			EntityPanels: []systems.EntityPanelDef{
				{Widget: "statblock", AppliesTo: systems.EntityPanelAppliesNPC},
				{Widget: "statblock", AppliesTo: systems.EntityPanelAppliesCreature},
				{Widget: "negotiation", AppliesTo: systems.EntityPanelAppliesNPC},
			},
		}}}, []encounters.StatBlock{{Widget: "statblock", SystemID: "drawsteel", TypeIDs: []int{10, 1}}}},
		{"stat block for creatures only", fakeEnabled{sys: fakeSystem{manifest: &systems.SystemManifest{
			ID:           "x",
			EntityPanels: []systems.EntityPanelDef{{Widget: "sb", AppliesTo: systems.EntityPanelAppliesCreature}},
		}}}, []encounters.StatBlock{{Widget: "sb", SystemID: "x", TypeIDs: []int{10}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &encountersPageAdapter{svc: &pickerTypesEntities{types: types}, lists: pickerTypesLists{chars: []int{1}, npcs: []int{1}}, sys: tt.sys}
			got, err := a.PickerTypes(context.Background(), "c1")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.StatBlocks, tt.want) {
				t.Fatalf("stat blocks %+v, want %+v", got.StatBlocks, tt.want)
			}
		})
	}
}
