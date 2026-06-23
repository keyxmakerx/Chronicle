package entities

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TestEnsurePlayerCharacterType verifies the addon-enable premake / migration:
// it creates a claimable Player Character type (character-surface layout) nested
// under the default "Characters" category, re-parents a stray top-level PC type
// an earlier build premade (preserving its entities), skips when a system
// character type already serves as the claimable one, and is otherwise a no-op.
func TestEnsurePlayerCharacterType(t *testing.T) {
	t.Run("creates the PC type (top-level) when no Characters category exists", func(t *testing.T) {
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
		if captured.ParentTypeID != nil {
			t.Errorf("with no Characters category present, the PC type should fall back to top-level (nil parent), got %v", captured.ParentTypeID)
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

	t.Run("no-op when a system character type already exists (no duplicate category)", func(t *testing.T) {
		// Regression: a Draw Steel campaign already ships a drawsteel-character
		// type (preset "character"). The premake must recognize it as the
		// claimable character and NOT create a redundant generic "Player
		// Characters" type — which previously surfaced as a duplicate category
		// and a second sheet to customize.
		createCalled := false
		charPreset := "character"
		typeRepo := &mockEntityTypeRepo{
			listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) {
				return []EntityType{
					{ID: 1, Name: "Character", Slug: "drawsteel-character", PresetCategory: &charPreset},
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
			t.Error("should be a no-op when a system character type (drawsteel-character) already exists")
		}
	})

	t.Run("nests a system character type under Characters without renaming it", func(t *testing.T) {
		// Draw Steel campaign: the default "Characters" category (top-level) plus
		// the system's drawsteel-character type (top-level). The addon nests the
		// system type under Characters (so it reads as the PC sub-category)
		// WITHOUT renaming it (modularity — the system's terminology is kept) and
		// creates no generic type.
		charPreset := "character"
		charType := EntityType{ID: 1, CampaignID: "camp-1", Name: "Character", NamePlural: "Characters", Slug: DefaultCharacterTypeSlug, Icon: "fa-user", Color: "#3b82f6"}
		dsType := EntityType{ID: 2, CampaignID: "camp-1", Name: "Hero", NamePlural: "Heroes", Slug: "drawsteel-character", Icon: "fa-shield", Color: "#7c3aed", PresetCategory: &charPreset}

		var updated *EntityType
		createCalled := false
		typeRepo := &mockEntityTypeRepo{
			listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) {
				return []EntityType{charType, dsType}, nil
			},
			findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
				switch id {
				case 1:
					c := charType
					return &c, nil
				case 2:
					d := dsType
					return &d, nil
				}
				return nil, apperror.NewNotFound("entity type not found")
			},
			updateFn: func(_ context.Context, et *EntityType) error { updated = et; return nil },
			createFn: func(_ context.Context, _ *EntityType) error { createCalled = true; return nil },
		}
		svc := newTestService(&mockEntityRepo{}, typeRepo)
		svc.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: true}})

		if err := svc.EnsurePlayerCharacterType(context.Background(), "camp-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if createCalled {
			t.Error("must nest the system type, not create a generic one")
		}
		if updated == nil {
			t.Fatal("expected the system character type to be nested (Update called)")
		}
		if updated.ID != 2 {
			t.Errorf("nested the wrong type: id = %d, want 2 (the system type)", updated.ID)
		}
		if updated.ParentTypeID == nil || *updated.ParentTypeID != 1 {
			t.Errorf("system type should be nested under Characters (parent id 1), got %v", updated.ParentTypeID)
		}
		if updated.Name != "Hero" || updated.NamePlural != "Heroes" {
			t.Errorf("system type must NOT be renamed; got name=%q plural=%q", updated.Name, updated.NamePlural)
		}
	})

	t.Run("nests BOTH a stray PC type and the system char type when both exist", func(t *testing.T) {
		// The operator's live scenario: a stray generic player-character AND a
		// system drawsteel-character both exist. The fix nests BOTH under
		// Characters (an early return previously left the system type unnested).
		pcPreset := PresetCategoryPlayerCharacter
		charPreset := "character"
		charType := EntityType{ID: 1, CampaignID: "camp-1", Name: "Character", NamePlural: "Characters", Slug: DefaultCharacterTypeSlug}
		strayPC := EntityType{ID: 2, CampaignID: "camp-1", Name: "Player Character", NamePlural: "Player Characters", Slug: SlugPlayerCharacter, Color: "#6366f1", PresetCategory: &pcPreset}
		dsType := EntityType{ID: 3, CampaignID: "camp-1", Name: "Hero", NamePlural: "Heroes", Slug: "drawsteel-character", Color: "#7c3aed", PresetCategory: &charPreset}

		var updatedIDs []int
		createCalled := false
		typeRepo := &mockEntityTypeRepo{
			listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) {
				return []EntityType{charType, strayPC, dsType}, nil
			},
			findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
				switch id {
				case 1:
					c := charType
					return &c, nil
				case 2:
					p := strayPC
					return &p, nil
				case 3:
					d := dsType
					return &d, nil
				}
				return nil, apperror.NewNotFound("entity type not found")
			},
			updateFn: func(_ context.Context, et *EntityType) error {
				updatedIDs = append(updatedIDs, et.ID)
				if et.ParentTypeID == nil || *et.ParentTypeID != 1 {
					t.Errorf("type %d should be nested under Characters (parent 1), got %v", et.ID, et.ParentTypeID)
				}
				return nil
			},
			createFn: func(_ context.Context, _ *EntityType) error { createCalled = true; return nil },
		}
		svc := newTestService(&mockEntityRepo{}, typeRepo)
		svc.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: true}})

		if err := svc.EnsurePlayerCharacterType(context.Background(), "camp-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if createCalled {
			t.Error("must not create a generic type when both already exist")
		}
		got := map[int]bool{}
		for _, id := range updatedIDs {
			got[id] = true
		}
		if !got[2] {
			t.Errorf("the stray PC type (2) should have been nested; updated: %v", updatedIDs)
		}
		if !got[3] {
			t.Errorf("the system char type (3) should have been nested; updated: %v", updatedIDs)
		}
	})

	t.Run("migrates a stray top-level PC type under the default Characters category", func(t *testing.T) {
		// Prod shape: the default "Characters" category (top-level) plus a stray
		// top-level "Player Character" type an earlier build premade. The fix must
		// re-parent the stray (not create or delete), preserving its entities.
		pcPreset := PresetCategoryPlayerCharacter
		charType := EntityType{ID: 1, CampaignID: "camp-1", Name: "Character", NamePlural: "Characters", Slug: DefaultCharacterTypeSlug, Icon: "fa-user", Color: "#3b82f6"}
		pcType := EntityType{ID: 2, CampaignID: "camp-1", Name: "Player Character", NamePlural: "Player Characters", Slug: SlugPlayerCharacter, Icon: "fa-user-shield", Color: "#6366f1", PresetCategory: &pcPreset}

		var updated *EntityType
		createCalled := false
		typeRepo := &mockEntityTypeRepo{
			listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) {
				return []EntityType{charType, pcType}, nil
			},
			findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
				switch id {
				case 1:
					c := charType
					return &c, nil
				case 2:
					p := pcType
					return &p, nil
				}
				return nil, apperror.NewNotFound("entity type not found")
			},
			updateFn: func(_ context.Context, et *EntityType) error { updated = et; return nil },
			createFn: func(_ context.Context, _ *EntityType) error { createCalled = true; return nil },
		}
		svc := newTestService(&mockEntityRepo{}, typeRepo)
		svc.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: true}})

		if err := svc.EnsurePlayerCharacterType(context.Background(), "camp-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if createCalled {
			t.Error("migration must re-parent the existing type, not create a new one")
		}
		if updated == nil {
			t.Fatal("expected the stray PC type to be re-parented (Update called)")
		}
		if updated.ID != 2 {
			t.Errorf("re-parented the wrong type: id = %d, want 2", updated.ID)
		}
		if updated.ParentTypeID == nil || *updated.ParentTypeID != 1 {
			t.Errorf("PC type should be nested under the Characters category (parent id 1), got %v", updated.ParentTypeID)
		}
	})

	t.Run("creates the PC type nested under the default Characters category", func(t *testing.T) {
		charType := EntityType{ID: 1, CampaignID: "camp-1", Name: "Character", NamePlural: "Characters", Slug: DefaultCharacterTypeSlug}
		var captured *EntityType
		typeRepo := &mockEntityTypeRepo{
			listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) {
				return []EntityType{charType}, nil // default Characters present, no PC type yet
			},
			findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
				if id == 1 {
					c := charType
					return &c, nil
				}
				return nil, apperror.NewNotFound("entity type not found")
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
		if captured.ParentTypeID == nil || *captured.ParentTypeID != 1 {
			t.Errorf("created PC type should be nested under the Characters category (parent id 1), got %v", captured.ParentTypeID)
		}
		if captured.Slug != SlugPlayerCharacter {
			t.Errorf("created slug = %q, want %q", captured.Slug, SlugPlayerCharacter)
		}
	})
}

// TestCreateEntityType_RejectsDuplicatePlayerCharacter verifies the single-owner
// guard: with the addon on and a Player Character type already present, a manual
// attempt to create a second one is rejected (409 Conflict).
func TestCreateEntityType_RejectsDuplicatePlayerCharacter(t *testing.T) {
	pcPreset := PresetCategoryPlayerCharacter
	typeRepo := &mockEntityTypeRepo{
		listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) {
			return []EntityType{{ID: 1, Slug: SlugPlayerCharacter, PresetCategory: &pcPreset}}, nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo)
	svc.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: true}})

	_, err := svc.CreateEntityType(context.Background(), "camp-1", CreateEntityTypeInput{
		Name:           "Player Character",
		PresetCategory: PresetCategoryPlayerCharacter,
		Color:          "#6366f1",
	})
	assertAppError(t, err, 409)
}
