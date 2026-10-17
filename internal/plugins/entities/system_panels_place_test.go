package entities

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

// withPanels is DefaultLayout with the Game System Panels block placed.
func withPanels() EntityTypeLayout {
	return insertAfterTitle(DefaultLayout(), TemplateBlock{ID: "blk-system-panels", Type: BlockSystemPanels})
}

// TestPlaceSystemPanels: only the named types gain the block, a layout that
// already places it is not written, and no other page piece is added.
func TestPlaceSystemPanels(t *testing.T) {
	written := map[int][]string{}
	repo := &mockEntityTypeRepo{
		listByCampaignFn: func(context.Context, string) ([]EntityType, error) {
			return []EntityType{
				{ID: 1, Layout: DefaultLayout()},
				{ID: 2, Layout: withPanels()},
				{ID: 3, Layout: DefaultLayout()},
			}, nil
		},
		updateLayoutFn: func(_ context.Context, id int, raw string) error {
			var l EntityTypeLayout
			if err := json.Unmarshal([]byte(raw), &l); err != nil {
				t.Fatalf("written layout: %v", err)
			}
			written[id] = blockTypeOrder(l)
			return nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, repo)

	n, err := svc.PlaceSystemPanels(context.Background(), "camp-1", []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int][]string{1: {"title", "system_panels", "entry", "image", "attributes", "details"}}
	if n != 1 || !reflect.DeepEqual(written, want) {
		t.Errorf("changed %d, wrote %v; want 1, %v", n, written, want)
	}
}

// TestCreateEntityType_SubTypeInheritsSystemPanels: a sub-type of a type whose
// pages show the game system's panels shows them too; under a type without
// the block it gets the plain default layout.
func TestCreateEntityType_SubTypeInheritsSystemPanels(t *testing.T) {
	tests := []struct {
		name         string
		parentLayout EntityTypeLayout
		wantPanels   bool
	}{
		{"parent places the panels block", withPanels(), true},
		{"parent does not", DefaultLayout(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var created *EntityType
			repo := &mockEntityTypeRepo{
				findByIDFn: func(context.Context, int) (*EntityType, error) {
					return &EntityType{ID: 1, CampaignID: "camp-1", Name: "Creature", Layout: tt.parentLayout}, nil
				},
				createFn: func(_ context.Context, et *EntityType) error { created = et; et.ID = 9; return nil },
			}
			svc := newTestService(&mockEntityRepo{}, repo)
			parent := 1
			if _, err := svc.CreateEntityType(context.Background(), "camp-1", CreateEntityTypeInput{Name: "Dragon", ParentTypeID: &parent}); err != nil {
				t.Fatal(err)
			}
			if got := layoutPlacesBlock(created.Layout, BlockSystemPanels); got != tt.wantPanels {
				t.Errorf("sub-type places system panels = %v, want %v", got, tt.wantPanels)
			}
		})
	}
}

// A creature type starts with the panels block so the game system's creature
// panels (a stat block) show without the owner listing it as NPCs.
func TestCreateEntityType_CreatureStartsWithSystemPanels(t *testing.T) {
	tests := []struct {
		name       string
		input      CreateEntityTypeInput
		wantPanels bool
	}{
		{"creature preset", CreateEntityTypeInput{Name: "Creature", PresetCategory: "creature"}, true},
		{"named creature", CreateEntityTypeInput{Name: "Creature"}, true},
		{"other preset", CreateEntityTypeInput{Name: "Spell", PresetCategory: "spell"}, false},
		{"plain type", CreateEntityTypeInput{Name: "Location"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var created *EntityType
			repo := &mockEntityTypeRepo{
				createFn: func(_ context.Context, et *EntityType) error { created = et; et.ID = 9; return nil },
			}
			svc := newTestService(&mockEntityRepo{}, repo)
			if _, err := svc.CreateEntityType(context.Background(), "camp-1", tt.input); err != nil {
				t.Fatal(err)
			}
			if got := layoutPlacesBlock(created.Layout, BlockSystemPanels); got != tt.wantPanels {
				t.Errorf("places system panels = %v, want %v", got, tt.wantPanels)
			}
		})
	}
}
