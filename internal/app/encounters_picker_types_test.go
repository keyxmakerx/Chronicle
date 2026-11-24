package app

import (
	"context"
	"reflect"
	"strconv"
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

// heroListEntities serves pages by type in fixed-size pages, like the list
// API, so the walk past the first page is exercised.
type heroListEntities struct {
	entities.EntityService
	types []entities.EntityType
	pages map[int][]entities.Entity
}

func (h *heroListEntities) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return h.types, nil
}

func (h *heroListEntities) List(_ context.Context, _ string, typeID int, _ int, _ string, opts entities.ListOptions) ([]entities.Entity, int, error) {
	all := h.pages[typeID]
	from := (opts.Page - 1) * opts.PerPage
	if from >= len(all) {
		return nil, len(all), nil
	}
	return all[from:min(from+opts.PerPage, len(all))], len(all), nil
}

// The hero picker lists heroes only: claimed, playable, of the player
// character sub-type, or with a class or hero choice picked. NPCs on the
// same list stay out, and heroes past the first page of results are found.
func TestEncountersHeroes(t *testing.T) {
	pcCat := entities.PresetCategoryPlayerCharacter
	types := []entities.EntityType{
		{ID: 1, Slug: "character", Enabled: true},
		{ID: 13, Slug: "player-character", PresetCategory: &pcCat, Enabled: true},
	}
	owner := "u1"
	var chars []entities.Entity
	for i := 0; i < 150; i++ {
		chars = append(chars, entities.Entity{ID: "npc" + strconv.Itoa(i), Name: "NPC " + strconv.Itoa(i), EntityTypeID: 1})
	}
	chars = append(chars,
		entities.Entity{ID: "tyne", Name: "Tyne", EntityTypeID: 1, OwnerUserID: &owner},
		entities.Entity{ID: "orrin", Name: "Orrin", EntityTypeID: 1, Playable: true},
		entities.Entity{ID: "ash", Name: "ash", EntityTypeID: 1, FieldsData: map[string]any{"ancestry": "Human"}},
	)
	pcs := []entities.Entity{{ID: "pc", Name: "Saatraaol", EntityTypeID: 13}}
	sys := fakeEnabled{sys: fakeSystem{manifest: &systems.SystemManifest{ID: "drawsteel", EntityPresets: []systems.EntityPresetDef{{
		Slug: "drawsteel-character",
		Fields: []systems.FieldDef{
			{Key: "class", FoundryCollection: "items", FoundryItemSingle: true, FoundryItemType: []string{"class"}},
			{Key: "ancestry", FoundryCollection: "items", FoundryItemSingle: true, FoundryItemType: []string{"ancestry"}},
		},
	}}}}}
	a := &encountersPageAdapter{svc: &heroListEntities{types: types, pages: map[int][]entities.Entity{1: append(chars, pcs...), 13: pcs}}, sys: sys}
	got, err := a.Heroes(context.Background(), "c1", []int{1, 13}, encounters.Viewer{VisibilityRole: 3})
	if err != nil {
		t.Fatal(err)
	}
	want := []encounters.HeroPage{{ID: "ash", Name: "ash"}, {ID: "orrin", Name: "Orrin"}, {ID: "pc", Name: "Saatraaol"}, {ID: "tyne", Name: "Tyne"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
