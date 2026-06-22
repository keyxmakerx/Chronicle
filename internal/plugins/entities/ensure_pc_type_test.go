package entities

import (
	"context"
	"testing"
)

// TestEnsurePlayerCharacterType verifies the addon-enable premake: it creates a
// claimable Player Character type with the character-surface layout when none
// exists, and is a no-op (idempotent) when one already does.
func TestEnsurePlayerCharacterType(t *testing.T) {
	t.Run("creates the PC type when none exists", func(t *testing.T) {
		var captured *EntityType
		typeRepo := &mockEntityTypeRepo{
			listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) {
				return []EntityType{{ID: 1, Name: "Location", Slug: "location"}}, nil // no PC type
			},
			createFn: func(_ context.Context, et *EntityType) error { captured = et; et.ID = 9; return nil },
		}
		svc := newTestService(&mockEntityRepo{}, typeRepo)
		svc.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: true}})

		if err := svc.EnsurePlayerCharacterType(context.Background(), "camp-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured == nil {
			t.Fatal("expected the PC type to be created")
		}
		if captured.Slug != SlugPlayerCharacter {
			t.Errorf("created slug = %q, want %q", captured.Slug, SlugPlayerCharacter)
		}
		if captured.Claimable == nil || !*captured.Claimable {
			t.Errorf("created PC type should default claimable=true")
		}
		if len(captured.Layout.Rows) == 0 ||
			len(captured.Layout.Rows[0].Columns) == 0 ||
			len(captured.Layout.Rows[0].Columns[0].Blocks) == 0 ||
			captured.Layout.Rows[0].Columns[0].Blocks[0].Type != "character_surface" {
			t.Errorf("created PC type should use CharacterLayout (character_surface block first)")
		}
	})

	t.Run("idempotent — no-op when a PC type already exists", func(t *testing.T) {
		createCalled := false
		preset := PresetCategoryPlayerCharacter
		typeRepo := &mockEntityTypeRepo{
			listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) {
				return []EntityType{
					{ID: 1, Name: "Hero", Slug: "hero", PresetCategory: &preset}, // a PC type (by preset)
				}, nil
			},
			createFn: func(_ context.Context, _ *EntityType) error { createCalled = true; return nil },
		}
		svc := newTestService(&mockEntityRepo{}, typeRepo)
		svc.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: true}})

		if err := svc.EnsurePlayerCharacterType(context.Background(), "camp-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if createCalled {
			t.Error("should be a no-op when a player-character type already exists")
		}
	})
}
