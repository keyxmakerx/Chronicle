package entities

import (
	"context"
	"reflect"
	"testing"
)

func fieldKeys(fs []FieldDefinition) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Key
	}
	return out
}

func TestMergeNewFields(t *testing.T) {
	existing := []FieldDefinition{{Key: "a"}, {Key: "b"}}
	declared := []FieldDefinition{
		{Key: "b"}, // already present — ignored
		{Key: "c"},
		{Key: ""}, // empty key — skipped
		{Key: "d"},
	}

	merged, added := mergeNewFields(existing, declared)
	if got := fieldKeys(merged); !reflect.DeepEqual(got, []string{"a", "b", "c", "d"}) {
		t.Fatalf("merged keys = %v, want [a b c d]", got)
	}
	if got := fieldKeys(added); !reflect.DeepEqual(got, []string{"c", "d"}) {
		t.Fatalf("added keys = %v, want [c d]", got)
	}

	// Idempotent: merging the same declared set again adds nothing.
	merged2, added2 := mergeNewFields(merged, declared)
	if len(added2) != 0 {
		t.Fatalf("second merge added %d fields, want 0", len(added2))
	}
	if got := fieldKeys(merged2); !reflect.DeepEqual(got, []string{"a", "b", "c", "d"}) {
		t.Fatalf("second merge changed order: %v", got)
	}
}

func TestMergeNewFields_PreservesExistingOnKeyCollision(t *testing.T) {
	// A declared field whose Key collides with an existing one must NOT overwrite
	// the existing (user/db) definition — additive only.
	existing := []FieldDefinition{{Key: "might", Label: "Might (edited)", Type: "number"}}
	declared := []FieldDefinition{{Key: "might", Label: "Might", Type: "text"}}

	merged, added := mergeNewFields(existing, declared)
	if len(added) != 0 {
		t.Fatalf("added %d, want 0", len(added))
	}
	if merged[0].Label != "Might (edited)" || merged[0].Type != "number" {
		t.Fatalf("collision overwrote existing field: %+v", merged[0])
	}
}

func TestReconcileEntityTypeFields_AddsMissing(t *testing.T) {
	stored := EntityType{
		ID: 7, CampaignID: "c1", Name: "Heroes", NamePlural: "Heroes",
		Icon: "fa-user", Color: "#5b8def",
		Fields: []FieldDefinition{{Key: "might", Label: "Might", Type: "number"}},
	}
	var persisted *EntityType
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, _ int) (*EntityType, error) {
			cp := stored
			cp.Fields = append([]FieldDefinition(nil), stored.Fields...)
			return &cp, nil
		},
		updateFn: func(_ context.Context, et *EntityType) error { persisted = et; return nil },
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo)

	declared := []FieldDefinition{
		{Key: "might", Label: "Might", Type: "number"}, // present
		{Key: "backstory", Label: "Backstory", Type: "textarea"},
		{Key: "", Label: "skip", Type: "text"}, // empty key skipped
		{Key: "wealth", Label: "Wealth", Type: "number"},
	}

	added, err := svc.ReconcileEntityTypeFields(context.Background(), 7, declared)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added != 2 {
		t.Fatalf("added = %d, want 2", added)
	}
	if persisted == nil {
		t.Fatal("expected the type to be persisted")
	}
	if got := fieldKeys(persisted.Fields); !reflect.DeepEqual(got, []string{"might", "backstory", "wealth"}) {
		t.Fatalf("persisted keys = %v, want [might backstory wealth]", got)
	}
	// Name unchanged → slug must not be regenerated.
	if persisted.Name != "Heroes" {
		t.Fatalf("name changed to %q", persisted.Name)
	}
}

func TestReconcileEntityTypeFields_NoopWhenComplete(t *testing.T) {
	stored := EntityType{
		ID: 7, CampaignID: "c1", Name: "Heroes", NamePlural: "Heroes",
		Icon: "fa-user", Color: "#5b8def",
		Fields: []FieldDefinition{{Key: "might"}, {Key: "backstory"}},
	}
	updateCalled := false
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, _ int) (*EntityType, error) {
			cp := stored
			cp.Fields = append([]FieldDefinition(nil), stored.Fields...)
			return &cp, nil
		},
		updateFn: func(_ context.Context, _ *EntityType) error { updateCalled = true; return nil },
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo)

	added, err := svc.ReconcileEntityTypeFields(context.Background(), 7,
		[]FieldDefinition{{Key: "might"}, {Key: "backstory"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added != 0 {
		t.Fatalf("added = %d, want 0", added)
	}
	if updateCalled {
		t.Fatal("Update should not be called when nothing changes")
	}
}

func TestReconcileEntityTypeFields_EmptyDeclaredIsNoop(t *testing.T) {
	findCalled := false
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, _ int) (*EntityType, error) {
			findCalled = true
			return &EntityType{ID: 1}, nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo)

	added, err := svc.ReconcileEntityTypeFields(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added != 0 {
		t.Fatalf("added = %d, want 0", added)
	}
	if findCalled {
		t.Fatal("empty declared should short-circuit before any repo call")
	}
}
