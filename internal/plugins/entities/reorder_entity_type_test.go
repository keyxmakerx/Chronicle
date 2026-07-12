// reorder_entity_type_test.go — C-NAV-V3 r3 acceptance criterion: sub-category
// types must be reorderable (persisted, surviving reload) via a dense
// per-parent re-sequence of entity_types.sort_order. Before this the order was
// frozen at creation (written once, campaign-wide, never updated by any path).
package entities

import (
	"context"
	"testing"
)

// newChildTypeReorderRepo builds a stateful mockEntityTypeRepo: ListChildTypes
// returns the current child order, and ResequenceChildTypes rewrites it — so a
// re-read after a reorder reflects the persisted result ("survives reload").
func newChildTypeReorderRepo(parentID int, initial []int, captured *[][]int) *mockEntityTypeRepo {
	current := append([]int(nil), initial...)
	pid := parentID
	return &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
			// Every id in this test is a sub-type of parentID in campaign c1.
			return &EntityType{ID: id, CampaignID: "c1", ParentTypeID: &pid}, nil
		},
		listChildTypesFn: func(_ context.Context, gotParent int) ([]EntityType, error) {
			if gotParent != parentID {
				return nil, nil
			}
			out := make([]EntityType, 0, len(current))
			for _, id := range current {
				out = append(out, EntityType{ID: id, CampaignID: "c1", ParentTypeID: &pid})
			}
			return out, nil
		},
		resequenceChildTypesFn: func(_ context.Context, _ string, orderedIDs []int) error {
			current = append([]int(nil), orderedIDs...)
			if captured != nil {
				*captured = append(*captured, append([]int(nil), orderedIDs...))
			}
			return nil
		},
	}
}

func assertIntOrder(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("order length = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d (sort_order %d) = %d, want %d (full: %v)", i, i, got[i], want[i], got)
		}
	}
}

// TestReorderEntityType_DenseResequencePersistedSurvivesReload is the headline
// r3 pin: moving a sub-category type re-sequences its parent's children densely,
// the write persists, and a fresh read returns the new order.
func TestReorderEntityType_DenseResequencePersistedSurvivesReload(t *testing.T) {
	var captured [][]int
	// Parent 100 with children [10, 20, 30] (creation order, degenerate ties).
	typeRepo := newChildTypeReorderRepo(100, []int{10, 20, 30}, &captured)
	svc := newTestService(&mockEntityRepo{}, typeRepo)

	// Move type 30 to index 0.
	if err := svc.ReorderEntityType(context.Background(), "c1", 30, 0); err != nil {
		t.Fatalf("ReorderEntityType: %v", err)
	}

	if len(captured) != 1 {
		t.Fatalf("expected exactly one resequence, got %d", len(captured))
	}
	assertIntOrder(t, captured[0], []int{30, 10, 20}) // dense 0..N-1, moved id first

	// Survives reload: a fresh read reflects the persisted order.
	children, err := typeRepo.ListChildTypes(context.Background(), 100)
	if err != nil {
		t.Fatalf("ListChildTypes: %v", err)
	}
	reread := make([]int, len(children))
	for i, c := range children {
		reread[i] = c.ID
	}
	assertIntOrder(t, reread, []int{30, 10, 20})

	// Repeating the same move is idempotent (dense unique orders, no drift).
	if err := svc.ReorderEntityType(context.Background(), "c1", 30, 0); err != nil {
		t.Fatalf("ReorderEntityType (repeat): %v", err)
	}
	assertIntOrder(t, captured[1], []int{30, 10, 20})
}

// TestReorderEntityType_RejectsTopLevel: a top-level type (no parent) has no
// sort_order-driven sidebar order — its order lives in sidebar_config Items — so
// reordering it here is a bad request, not a silent no-op.
func TestReorderEntityType_RejectsTopLevel(t *testing.T) {
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
			return &EntityType{ID: id, CampaignID: "c1", ParentTypeID: nil}, nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo)
	err := svc.ReorderEntityType(context.Background(), "c1", 7, 0)
	assertAppError(t, err, 400)
}

// TestReorderEntityType_RejectsForeignCampaign: a type from another campaign is
// not found (authz), never reordered.
func TestReorderEntityType_RejectsForeignCampaign(t *testing.T) {
	pid := 100
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
			return &EntityType{ID: id, CampaignID: "other", ParentTypeID: &pid}, nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo)
	err := svc.ReorderEntityType(context.Background(), "c1", 7, 0)
	assertAppError(t, err, 404)
}
