package relations

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repository ---

type mockRelationRepo struct {
	createFn      func(ctx context.Context, rel *Relation) error
	findByIDFn    func(ctx context.Context, id int) (*Relation, error)
	listByEntityFn func(ctx context.Context, entityID string) ([]Relation, error)
	deleteFn      func(ctx context.Context, id int) error
	findReverseFn func(ctx context.Context, sourceEntityID, targetEntityID, relationType string) (*Relation, error)
}

func (m *mockRelationRepo) Create(ctx context.Context, rel *Relation) error {
	if m.createFn != nil {
		return m.createFn(ctx, rel)
	}
	rel.ID = 1
	return nil
}

func (m *mockRelationRepo) FindByID(ctx context.Context, id int) (*Relation, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return &Relation{
		ID:                  id,
		CampaignID:          "camp-1",
		SourceEntityID:      "entity-a",
		TargetEntityID:      "entity-b",
		RelationType:        "allied with",
		ReverseRelationType: "allied with",
	}, nil
}

func (m *mockRelationRepo) ListByEntity(ctx context.Context, entityID string) ([]Relation, error) {
	if m.listByEntityFn != nil {
		return m.listByEntityFn(ctx, entityID)
	}
	return nil, nil
}

func (m *mockRelationRepo) Delete(ctx context.Context, id int) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockRelationRepo) FindReverse(ctx context.Context, sourceEntityID, targetEntityID, relationType string) (*Relation, error) {
	if m.findReverseFn != nil {
		return m.findReverseFn(ctx, sourceEntityID, targetEntityID, relationType)
	}
	return nil, nil
}

// --- Test Helpers ---

func newTestService(repo *mockRelationRepo) RelationService {
	return NewRelationService(repo)
}

func assertAppError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != expectedCode {
		t.Errorf("expected status %d, got %d (message: %s)", expectedCode, appErr.Code, appErr.Message)
	}
}

// --- Create Tests ---

func TestCreate_Success(t *testing.T) {
	var createdRelations []*Relation
	repo := &mockRelationRepo{
		createFn: func(_ context.Context, rel *Relation) error {
			rel.ID = len(createdRelations) + 1
			createdRelations = append(createdRelations, rel)
			return nil
		},
	}
	svc := newTestService(repo)

	result, err := svc.Create(context.Background(), "camp-1", "entity-a", "entity-b", "parent of", "child of", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.RelationType != "parent of" {
		t.Errorf("expected relationType 'parent of', got %q", result.RelationType)
	}
	if result.ReverseRelationType != "child of" {
		t.Errorf("expected reverseRelationType 'child of', got %q", result.ReverseRelationType)
	}

	// Should create both forward and reverse relations.
	if len(createdRelations) != 2 {
		t.Fatalf("expected 2 created relations, got %d", len(createdRelations))
	}

	// Forward: entity-a → entity-b, "parent of" / "child of"
	fwd := createdRelations[0]
	if fwd.SourceEntityID != "entity-a" || fwd.TargetEntityID != "entity-b" {
		t.Errorf("forward: expected source=entity-a target=entity-b, got source=%s target=%s", fwd.SourceEntityID, fwd.TargetEntityID)
	}
	if fwd.RelationType != "parent of" || fwd.ReverseRelationType != "child of" {
		t.Errorf("forward: expected types parent of/child of, got %s/%s", fwd.RelationType, fwd.ReverseRelationType)
	}

	// Reverse: entity-b → entity-a, "child of" / "parent of"
	rev := createdRelations[1]
	if rev.SourceEntityID != "entity-b" || rev.TargetEntityID != "entity-a" {
		t.Errorf("reverse: expected source=entity-b target=entity-a, got source=%s target=%s", rev.SourceEntityID, rev.TargetEntityID)
	}
	if rev.RelationType != "child of" || rev.ReverseRelationType != "parent of" {
		t.Errorf("reverse: expected types child of/parent of, got %s/%s", rev.RelationType, rev.ReverseRelationType)
	}
}

func TestCreate_SymmetricRelation(t *testing.T) {
	var createdRelations []*Relation
	repo := &mockRelationRepo{
		createFn: func(_ context.Context, rel *Relation) error {
			rel.ID = len(createdRelations) + 1
			createdRelations = append(createdRelations, rel)
			return nil
		},
	}
	svc := newTestService(repo)

	// Empty reverse type should default to forward type.
	result, err := svc.Create(context.Background(), "camp-1", "entity-a", "entity-b", "allied with", "", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RelationType != "allied with" {
		t.Errorf("expected 'allied with', got %q", result.RelationType)
	}
	if result.ReverseRelationType != "allied with" {
		t.Errorf("expected symmetric 'allied with', got %q", result.ReverseRelationType)
	}
}

func TestCreate_TrimsWhitespace(t *testing.T) {
	repo := &mockRelationRepo{
		createFn: func(_ context.Context, rel *Relation) error {
			rel.ID = 1
			return nil
		},
	}
	svc := newTestService(repo)

	result, err := svc.Create(context.Background(), "camp-1", "entity-a", "entity-b", "  parent of  ", "  child of  ", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RelationType != "parent of" {
		t.Errorf("expected trimmed 'parent of', got %q", result.RelationType)
	}
	if result.ReverseRelationType != "child of" {
		t.Errorf("expected trimmed 'child of', got %q", result.ReverseRelationType)
	}
}

func TestCreate_SelfRelation(t *testing.T) {
	svc := newTestService(&mockRelationRepo{})

	_, err := svc.Create(context.Background(), "camp-1", "entity-a", "entity-a", "parent of", "child of", "user-1")
	assertAppError(t, err, 400)
}

func TestCreate_EmptyRelationType(t *testing.T) {
	svc := newTestService(&mockRelationRepo{})

	_, err := svc.Create(context.Background(), "camp-1", "entity-a", "entity-b", "", "", "user-1")
	assertAppError(t, err, 400)
}

func TestCreate_WhitespaceOnlyRelationType(t *testing.T) {
	svc := newTestService(&mockRelationRepo{})

	_, err := svc.Create(context.Background(), "camp-1", "entity-a", "entity-b", "   ", "", "user-1")
	assertAppError(t, err, 400)
}

func TestCreate_RelationTypeTooLong(t *testing.T) {
	svc := newTestService(&mockRelationRepo{})

	longType := ""
	for i := 0; i < 101; i++ {
		longType += "x"
	}
	_, err := svc.Create(context.Background(), "camp-1", "entity-a", "entity-b", longType, "short", "user-1")
	assertAppError(t, err, 400)
}

func TestCreate_ReverseTypeTooLong(t *testing.T) {
	svc := newTestService(&mockRelationRepo{})

	longType := ""
	for i := 0; i < 101; i++ {
		longType += "x"
	}
	_, err := svc.Create(context.Background(), "camp-1", "entity-a", "entity-b", "short", longType, "user-1")
	assertAppError(t, err, 400)
}

func TestCreate_RelationTypeExactly100(t *testing.T) {
	repo := &mockRelationRepo{
		createFn: func(_ context.Context, rel *Relation) error {
			rel.ID = 1
			return nil
		},
	}
	svc := newTestService(repo)

	exactType := ""
	for i := 0; i < 100; i++ {
		exactType += "x"
	}
	_, err := svc.Create(context.Background(), "camp-1", "entity-a", "entity-b", exactType, "short", "user-1")
	if err != nil {
		t.Fatalf("100-char type should be allowed, got error: %v", err)
	}
}

func TestCreate_ForwardRepoError(t *testing.T) {
	repo := &mockRelationRepo{
		createFn: func(_ context.Context, _ *Relation) error {
			return errors.New("db connection failed")
		},
	}
	svc := newTestService(repo)

	_, err := svc.Create(context.Background(), "camp-1", "entity-a", "entity-b", "allied with", "", "user-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreate_ReverseConflictIgnored(t *testing.T) {
	callCount := 0
	repo := &mockRelationRepo{
		createFn: func(_ context.Context, rel *Relation) error {
			callCount++
			if callCount == 2 {
				// Reverse creation returns AppError (conflict).
				return apperror.NewConflict("duplicate relation")
			}
			rel.ID = callCount
			return nil
		},
	}
	svc := newTestService(repo)

	result, err := svc.Create(context.Background(), "camp-1", "entity-a", "entity-b", "allied with", "", "user-1")
	if err != nil {
		t.Fatalf("reverse conflict should be silently ignored, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestCreate_ReverseNonAppError(t *testing.T) {
	callCount := 0
	repo := &mockRelationRepo{
		createFn: func(_ context.Context, rel *Relation) error {
			callCount++
			if callCount == 2 {
				// Non-AppError on reverse creation should propagate.
				return errors.New("db exploded")
			}
			rel.ID = callCount
			return nil
		},
	}
	svc := newTestService(repo)

	_, err := svc.Create(context.Background(), "camp-1", "entity-a", "entity-b", "allied with", "", "user-1")
	if err == nil {
		t.Fatal("expected error for non-AppError reverse failure")
	}
	if !errors.Is(err, errors.Unwrap(err)) && err.Error() == "" {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

func TestCreate_PopulatesCampaignAndCreatedBy(t *testing.T) {
	var captured *Relation
	repo := &mockRelationRepo{
		createFn: func(_ context.Context, rel *Relation) error {
			if captured == nil {
				captured = rel
			}
			rel.ID = 1
			return nil
		},
	}
	svc := newTestService(repo)

	_, err := svc.Create(context.Background(), "camp-xyz", "entity-a", "entity-b", "knows", "", "user-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.CampaignID != "camp-xyz" {
		t.Errorf("expected campaignID 'camp-xyz', got %q", captured.CampaignID)
	}
	if captured.CreatedBy != "user-42" {
		t.Errorf("expected createdBy 'user-42', got %q", captured.CreatedBy)
	}
}

// --- ListByEntity Tests ---

func TestListByEntity_Success(t *testing.T) {
	expected := []Relation{
		{ID: 1, RelationType: "parent of"},
		{ID: 2, RelationType: "allied with"},
	}
	repo := &mockRelationRepo{
		listByEntityFn: func(_ context.Context, entityID string) ([]Relation, error) {
			if entityID != "entity-a" {
				t.Errorf("expected entityID 'entity-a', got %q", entityID)
			}
			return expected, nil
		},
	}
	svc := newTestService(repo)

	result, err := svc.ListByEntity(context.Background(), "entity-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 relations, got %d", len(result))
	}
}

func TestListByEntity_Empty(t *testing.T) {
	repo := &mockRelationRepo{
		listByEntityFn: func(_ context.Context, _ string) ([]Relation, error) {
			return nil, nil
		},
	}
	svc := newTestService(repo)

	result, err := svc.ListByEntity(context.Background(), "entity-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty list, got %v", result)
	}
}

func TestListByEntity_RepoError(t *testing.T) {
	repo := &mockRelationRepo{
		listByEntityFn: func(_ context.Context, _ string) ([]Relation, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestService(repo)

	_, err := svc.ListByEntity(context.Background(), "entity-a")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- GetByID Tests ---

func TestGetByID_Success(t *testing.T) {
	repo := &mockRelationRepo{
		findByIDFn: func(_ context.Context, id int) (*Relation, error) {
			return &Relation{ID: id, RelationType: "parent of"}, nil
		},
	}
	svc := newTestService(repo)

	result, err := svc.GetByID(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != 42 {
		t.Errorf("expected ID 42, got %d", result.ID)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	repo := &mockRelationRepo{
		findByIDFn: func(_ context.Context, _ int) (*Relation, error) {
			return nil, apperror.NewNotFound("relation not found")
		},
	}
	svc := newTestService(repo)

	_, err := svc.GetByID(context.Background(), 999)
	assertAppError(t, err, 404)
}

// --- Delete Tests ---

func TestDelete_Success(t *testing.T) {
	deletedIDs := []int{}
	repo := &mockRelationRepo{
		findByIDFn: func(_ context.Context, id int) (*Relation, error) {
			return &Relation{
				ID:                  id,
				SourceEntityID:      "entity-a",
				TargetEntityID:      "entity-b",
				RelationType:        "parent of",
				ReverseRelationType: "child of",
			}, nil
		},
		findReverseFn: func(_ context.Context, sourceEntityID, targetEntityID, relationType string) (*Relation, error) {
			if sourceEntityID != "entity-b" || targetEntityID != "entity-a" || relationType != "child of" {
				t.Errorf("unexpected reverse lookup: src=%s tgt=%s type=%s", sourceEntityID, targetEntityID, relationType)
			}
			return &Relation{ID: 99}, nil
		},
		deleteFn: func(_ context.Context, id int) error {
			deletedIDs = append(deletedIDs, id)
			return nil
		},
	}
	svc := newTestService(repo)

	err := svc.Delete(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should delete both reverse (99) and forward (1).
	if len(deletedIDs) != 2 {
		t.Fatalf("expected 2 deletions, got %d", len(deletedIDs))
	}
	if deletedIDs[0] != 99 {
		t.Errorf("expected first deletion to be reverse (99), got %d", deletedIDs[0])
	}
	if deletedIDs[1] != 1 {
		t.Errorf("expected second deletion to be forward (1), got %d", deletedIDs[1])
	}
}

func TestDelete_NoReverse(t *testing.T) {
	deletedIDs := []int{}
	repo := &mockRelationRepo{
		findByIDFn: func(_ context.Context, id int) (*Relation, error) {
			return &Relation{
				ID:                  id,
				SourceEntityID:      "entity-a",
				TargetEntityID:      "entity-b",
				RelationType:        "allied with",
				ReverseRelationType: "allied with",
			}, nil
		},
		findReverseFn: func(_ context.Context, _, _, _ string) (*Relation, error) {
			return nil, nil // No reverse found.
		},
		deleteFn: func(_ context.Context, id int) error {
			deletedIDs = append(deletedIDs, id)
			return nil
		},
	}
	svc := newTestService(repo)

	err := svc.Delete(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only delete the forward relation.
	if len(deletedIDs) != 1 {
		t.Fatalf("expected 1 deletion, got %d", len(deletedIDs))
	}
	if deletedIDs[0] != 1 {
		t.Errorf("expected deletion of ID 1, got %d", deletedIDs[0])
	}
}

func TestDelete_NotFound(t *testing.T) {
	repo := &mockRelationRepo{
		findByIDFn: func(_ context.Context, _ int) (*Relation, error) {
			return nil, apperror.NewNotFound("relation not found")
		},
	}
	svc := newTestService(repo)

	err := svc.Delete(context.Background(), 999)
	assertAppError(t, err, 404)
}

func TestDelete_FindReverseError(t *testing.T) {
	repo := &mockRelationRepo{
		findByIDFn: func(_ context.Context, id int) (*Relation, error) {
			return &Relation{
				ID:                  id,
				SourceEntityID:      "entity-a",
				TargetEntityID:      "entity-b",
				RelationType:        "parent of",
				ReverseRelationType: "child of",
			}, nil
		},
		findReverseFn: func(_ context.Context, _, _, _ string) (*Relation, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestService(repo)

	err := svc.Delete(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error when FindReverse fails")
	}
}

func TestDelete_ReverseDeleteErrorIgnored(t *testing.T) {
	deleteCallCount := 0
	repo := &mockRelationRepo{
		findByIDFn: func(_ context.Context, id int) (*Relation, error) {
			return &Relation{
				ID:                  id,
				SourceEntityID:      "entity-a",
				TargetEntityID:      "entity-b",
				RelationType:        "parent of",
				ReverseRelationType: "child of",
			}, nil
		},
		findReverseFn: func(_ context.Context, _, _, _ string) (*Relation, error) {
			return &Relation{ID: 99}, nil
		},
		deleteFn: func(_ context.Context, id int) error {
			deleteCallCount++
			if id == 99 {
				// Reverse deletion fails.
				return errors.New("reverse delete error")
			}
			return nil
		},
	}
	svc := newTestService(repo)

	// Should succeed because reverse delete error is silently ignored.
	err := svc.Delete(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected success despite reverse delete error, got: %v", err)
	}
	if deleteCallCount != 2 {
		t.Errorf("expected 2 delete calls, got %d", deleteCallCount)
	}
}

// --- GetCommonTypes Tests ---

func TestGetCommonTypes_ReturnsNonEmpty(t *testing.T) {
	svc := newTestService(&mockRelationRepo{})
	types := svc.GetCommonTypes()

	if len(types) == 0 {
		t.Fatal("expected non-empty common types")
	}

	// Verify at least one known pair exists.
	found := false
	for _, tp := range types {
		if tp.Forward == "parent of" && tp.Reverse == "child of" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'parent of'/'child of' pair in common types")
	}
}

func TestGetCommonTypes_AllPairsHaveBothLabels(t *testing.T) {
	svc := newTestService(&mockRelationRepo{})
	types := svc.GetCommonTypes()

	for _, tp := range types {
		if tp.Forward == "" {
			t.Error("found common type pair with empty forward label")
		}
		if tp.Reverse == "" {
			t.Error("found common type pair with empty reverse label")
		}
	}
}
