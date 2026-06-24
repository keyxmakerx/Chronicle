package entities

import (
	"context"
	"net/http"
	"testing"
)

func mergeStrPtr(s string) *string { return &s }
func mergeIntPtr(i int) *int       { return &i }

func TestMergeDuplicatePlayerCharacterType(t *testing.T) {
	t.Run("merges: moves the generic's entities onto the system type, removes the generic", func(t *testing.T) {
		var fromID, toID int
		moveCalled := false
		typeRepo := &mockEntityTypeRepo{
			listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) {
				return []EntityType{
					{ID: 1, Name: "Characters", Slug: "character"},
					{ID: 2, Name: "Player Characters", Slug: "player-character", PresetCategory: mergeStrPtr("player_character"), ParentTypeID: mergeIntPtr(1)},
					{ID: 3, Name: "Heroes", Slug: "drawsteel-character", PresetCategory: mergeStrPtr("character"), ParentTypeID: mergeIntPtr(1)},
				}, nil
			},
			moveEntitiesAndDeleteTypeFn: func(_ context.Context, _ string, from, to int) (int64, error) {
				moveCalled, fromID, toID = true, from, to
				return 3, nil
			},
		}
		entityRepo := &mockEntityRepo{
			countByTypeFn: func(_ context.Context, _ string, _ int, _ string) (map[int]int, error) {
				return map[int]int{2: 3}, nil
			},
		}
		svc := newTestService(entityRepo, typeRepo)

		res, err := svc.MergeDuplicatePlayerCharacterType(context.Background(), "camp-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !moveCalled {
			t.Fatal("expected MoveEntitiesAndDeleteType to be called")
		}
		if fromID != 2 || toID != 3 {
			t.Errorf("move(from,to) = (%d,%d), want (2,3) — generic into system", fromID, toID)
		}
		if res.Moved != 3 || res.RemovedTypeID != 2 || res.TargetTypeID != 3 || res.TargetName != "Heroes" {
			t.Errorf("result = %+v, want Moved=3 RemovedTypeID=2 TargetTypeID=3 TargetName=Heroes", res)
		}
	})

	t.Run("system-less: one generic, no system type → no-op (no move)", func(t *testing.T) {
		moveCalled := false
		typeRepo := &mockEntityTypeRepo{
			listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) {
				return []EntityType{
					{ID: 1, Name: "Characters", Slug: "character"},
					{ID: 2, Name: "Player Characters", Slug: "player-character", PresetCategory: mergeStrPtr("player_character"), ParentTypeID: mergeIntPtr(1)},
				}, nil
			},
			moveEntitiesAndDeleteTypeFn: func(_ context.Context, _ string, _, _ int) (int64, error) {
				moveCalled = true
				return 0, nil
			},
		}
		svc := newTestService(&mockEntityRepo{}, typeRepo)

		res, err := svc.MergeDuplicatePlayerCharacterType(context.Background(), "camp-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.NoOp {
			t.Errorf("expected NoOp result, got %+v", res)
		}
		if moveCalled {
			t.Error("move should not be called when there is no system type to merge into")
		}
	})

	t.Run("ambiguous: two system types → human-readable 409 conflict (no move)", func(t *testing.T) {
		moveCalled := false
		typeRepo := &mockEntityTypeRepo{
			listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) {
				return []EntityType{
					{ID: 1, Name: "Characters", Slug: "character"},
					{ID: 2, Name: "Player Characters", Slug: "player-character", PresetCategory: mergeStrPtr("player_character"), ParentTypeID: mergeIntPtr(1)},
					{ID: 3, Name: "Heroes", Slug: "drawsteel-character", PresetCategory: mergeStrPtr("character"), ParentTypeID: mergeIntPtr(1)},
					{ID: 4, Name: "PCs", Slug: "pf2e-character", PresetCategory: mergeStrPtr("character"), ParentTypeID: mergeIntPtr(1)},
				}, nil
			},
			moveEntitiesAndDeleteTypeFn: func(_ context.Context, _ string, _, _ int) (int64, error) {
				moveCalled = true
				return 0, nil
			},
		}
		svc := newTestService(&mockEntityRepo{}, typeRepo)

		_, err := svc.MergeDuplicatePlayerCharacterType(context.Background(), "camp-1")
		assertAppError(t, err, http.StatusConflict)
		if moveCalled {
			t.Error("move should not be called when the (from,to) pair is ambiguous")
		}
	})

	t.Run("no generic: already reconciled → no-op", func(t *testing.T) {
		typeRepo := &mockEntityTypeRepo{
			listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) {
				return []EntityType{
					{ID: 1, Name: "Characters", Slug: "character"},
					{ID: 3, Name: "Heroes", Slug: "drawsteel-character", PresetCategory: mergeStrPtr("character"), ParentTypeID: mergeIntPtr(1)},
				}, nil
			},
		}
		svc := newTestService(&mockEntityRepo{}, typeRepo)

		res, err := svc.MergeDuplicatePlayerCharacterType(context.Background(), "camp-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.NoOp {
			t.Errorf("expected NoOp, got %+v", res)
		}
	})
}
