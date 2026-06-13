// reorder_resequence_test.go — ReorderEntity must densely re-sequence the moved
// entity's sibling set so the (sort_order, name) tiebreak in the tree render
// can't snap a dragged entity back (cordinator#47, Bug 1 silent-revert).
package entities

import (
	"context"
	"testing"
)

// newResequenceRepo builds a mock whose sibling list is backed by an in-memory
// order that ResequenceSiblings rewrites — so a second reorder sees the result
// of the first (lets us assert idempotency).
func newResequenceRepo(typeID int, order []string, captured *[][]string, scope *[]any) *mockEntityRepo {
	current := append([]string(nil), order...)
	return &mockEntityRepo{
		findByIDFn: func(_ context.Context, id string) (*Entity, error) {
			return &Entity{ID: id, CampaignID: "camp-1", EntityTypeID: typeID}, nil
		},
		listSiblingIDsFn: func(_ context.Context, campaignID string, entityTypeID int, parentID, parentNodeID *string) ([]string, error) {
			if scope != nil {
				*scope = []any{campaignID, entityTypeID, parentID, parentNodeID}
			}
			return append([]string(nil), current...), nil
		},
		resequenceFn: func(_ context.Context, _ string, orderedIDs []string) error {
			current = append([]string(nil), orderedIDs...)
			if captured != nil {
				*captured = append(*captured, append([]string(nil), orderedIDs...))
			}
			return nil
		},
	}
}

// TestReorderEntity_ResequencesDegenerateTies is the headline case: three
// siblings all at sort_order 0 (the all-equal tie that made dragging a no-op).
// Moving the last to index 0 must produce 0,1,2 with the moved id first.
func TestReorderEntity_ResequencesDegenerateTies(t *testing.T) {
	// DB order under (sort_order, name) with every sort_order == 0 is alphabetical.
	var captured [][]string
	repo := newResequenceRepo(5, []string{"a-id", "b-id", "c-id"}, &captured, nil)
	svc := newTestService(repo, &mockEntityTypeRepo{})

	// Move the last sibling ("c-id") to index 0 (root scope: both parents nil).
	if err := svc.ReorderEntity(context.Background(), "camp-1", "c-id", nil, nil, 0); err != nil {
		t.Fatalf("ReorderEntity: %v", err)
	}

	if len(captured) != 1 {
		t.Fatalf("expected exactly one resequence, got %d", len(captured))
	}
	got := captured[0]
	want := []string{"c-id", "a-id", "b-id"} // index 0,1,2 → moved id first
	assertOrder(t, got, want)
}

// TestReorderEntity_Idempotent: repeating the same reorder leaves the order
// unchanged (dense unique sort_orders, no drift).
func TestReorderEntity_Idempotent(t *testing.T) {
	var captured [][]string
	repo := newResequenceRepo(5, []string{"a-id", "b-id", "c-id"}, &captured, nil)
	svc := newTestService(repo, &mockEntityTypeRepo{})

	for i := 0; i < 2; i++ {
		if err := svc.ReorderEntity(context.Background(), "camp-1", "c-id", nil, nil, 0); err != nil {
			t.Fatalf("ReorderEntity pass %d: %v", i, err)
		}
	}

	if len(captured) != 2 {
		t.Fatalf("expected two resequences, got %d", len(captured))
	}
	assertOrder(t, captured[0], []string{"c-id", "a-id", "b-id"})
	assertOrder(t, captured[1], []string{"c-id", "a-id", "b-id"}) // stable on repeat
}

// TestReorderEntity_ClampsIndexToAppend: an out-of-range index appends rather
// than erroring (the client's append path sends a count that may equal len).
func TestReorderEntity_ClampsIndexToAppend(t *testing.T) {
	var captured [][]string
	repo := newResequenceRepo(5, []string{"a-id", "b-id", "c-id"}, &captured, nil)
	svc := newTestService(repo, &mockEntityTypeRepo{})

	// Move "a-id" with a wildly oversized index → clamped to the end.
	if err := svc.ReorderEntity(context.Background(), "camp-1", "a-id", nil, nil, 999); err != nil {
		t.Fatalf("ReorderEntity: %v", err)
	}
	assertOrder(t, captured[0], []string{"b-id", "c-id", "a-id"})
}

// TestReorderEntity_ScopedToSiblingSet: the re-sequence is scoped to the moved
// entity's own (campaign, type, parent) set — it must load and renumber only
// those siblings, never reach across scopes.
func TestReorderEntity_ScopedToSiblingSet(t *testing.T) {
	var scope []any
	var captured [][]string
	repo := newResequenceRepo(7, []string{"x-id", "y-id"}, &captured, &scope)
	svc := newTestService(repo, &mockEntityTypeRepo{})

	parentNode := "node-42"
	if err := svc.ReorderEntity(context.Background(), "camp-1", "y-id", nil, &parentNode, 0); err != nil {
		t.Fatalf("ReorderEntity: %v", err)
	}

	// The sibling load used the moved entity's campaign + type + folder-node scope.
	if scope == nil {
		t.Fatal("ListSiblingIDsOrdered was not called")
	}
	if scope[0] != "camp-1" {
		t.Errorf("sibling scope campaign = %v, want camp-1", scope[0])
	}
	if scope[1] != 7 {
		t.Errorf("sibling scope entityTypeID = %v, want 7", scope[1])
	}
	gotParentNode, _ := scope[3].(*string)
	if gotParentNode == nil || *gotParentNode != "node-42" {
		t.Errorf("sibling scope parentNodeID = %v, want node-42", scope[3])
	}
	// Only the two siblings in that scope were renumbered.
	assertOrder(t, captured[0], []string{"y-id", "x-id"})
}

func assertOrder(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("resequence length = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d (sort_order %d) = %q, want %q (full: %v)", i, i, got[i], want[i], got)
		}
	}
}
