package entities

import (
	"context"
	"testing"
)

func snapStrPtr(s string) *string { return &s }
func snapIntPtr(i int) *int       { return &i }

func TestPlayerCharacterSetupSnapshot(t *testing.T) {
	t.Run("classifies generic, system, and default parent; counts entities + sub-categories", func(t *testing.T) {
		types := []EntityType{
			{ID: 1, Name: "Characters", Slug: "character"}, // default parent (top-level)
			{ID: 2, Name: "Player Characters", Slug: "player-character", PresetCategory: snapStrPtr("player_character"), ParentTypeID: snapIntPtr(1)},
			{ID: 3, Name: "Heroes", Slug: "drawsteel-character", PresetCategory: snapStrPtr("character"), ParentTypeID: snapIntPtr(1)},
			{ID: 4, Name: "NPCs", Slug: "npc", PresetCategory: snapStrPtr("npc"), ParentTypeID: snapIntPtr(1)}, // non-PC sub-category
		}
		typeRepo := &mockEntityTypeRepo{
			listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) { return types, nil },
		}
		entityRepo := &mockEntityRepo{
			countByTypeFn: func(_ context.Context, _ string, _ int, _ string) (map[int]int, error) {
				return map[int]int{2: 3, 3: 0}, nil
			},
		}
		svc := newTestService(entityRepo, typeRepo)

		snap, err := svc.PlayerCharacterSetupSnapshot(context.Background(), "camp-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if snap.DefaultCharsParentID == nil || *snap.DefaultCharsParentID != 1 {
			t.Errorf("DefaultCharsParentID = %v, want 1", snap.DefaultCharsParentID)
		}
		if len(snap.GenericPCTypes) != 1 || snap.GenericPCTypes[0].ID != 2 {
			t.Errorf("GenericPCTypes = %+v, want exactly [id 2]", snap.GenericPCTypes)
		}
		if len(snap.SystemCharTypes) != 1 || snap.SystemCharTypes[0].ID != 3 {
			t.Errorf("SystemCharTypes = %+v, want exactly [id 3]", snap.SystemCharTypes)
		}
		if snap.GenericPCCount != 3 {
			t.Errorf("GenericPCCount = %d, want 3", snap.GenericPCCount)
		}
		if snap.SystemCharCount != 0 {
			t.Errorf("SystemCharCount = %d, want 0", snap.SystemCharCount)
		}
		if snap.SubCategoryCount != 3 {
			t.Errorf("SubCategoryCount = %d, want 3 (ids 2,3,4 nested under Characters)", snap.SubCategoryCount)
		}
	})

	t.Run("no default Characters category → nil parent, zero sub-categories", func(t *testing.T) {
		types := []EntityType{
			{ID: 5, Name: "Player Characters", Slug: "player-character", PresetCategory: snapStrPtr("player_character")}, // top-level
		}
		typeRepo := &mockEntityTypeRepo{
			listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) { return types, nil },
		}
		svc := newTestService(&mockEntityRepo{}, typeRepo)

		snap, err := svc.PlayerCharacterSetupSnapshot(context.Background(), "camp-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if snap.DefaultCharsParentID != nil {
			t.Errorf("DefaultCharsParentID = %v, want nil", snap.DefaultCharsParentID)
		}
		if len(snap.GenericPCTypes) != 1 {
			t.Errorf("GenericPCTypes len = %d, want 1", len(snap.GenericPCTypes))
		}
		if snap.SubCategoryCount != 0 {
			t.Errorf("SubCategoryCount = %d, want 0", snap.SubCategoryCount)
		}
	})
}
