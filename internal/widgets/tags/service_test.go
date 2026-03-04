package tags

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repository ---

type mockTagRepo struct {
	createFn             func(ctx context.Context, tag *Tag) error
	findByIDFn           func(ctx context.Context, id int) (*Tag, error)
	listByCampaignFn     func(ctx context.Context, campaignID string, includeDmOnly bool) ([]Tag, error)
	updateFn             func(ctx context.Context, tag *Tag) error
	deleteFn             func(ctx context.Context, id int) error
	addTagToEntityFn     func(ctx context.Context, entityID string, tagID int) error
	removeTagFromEntityFn func(ctx context.Context, entityID string, tagID int) error
	getEntityTagsFn      func(ctx context.Context, entityID string, includeDmOnly bool) ([]Tag, error)
	getEntityTagsBatchFn func(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]Tag, error)
}

func (m *mockTagRepo) Create(ctx context.Context, tag *Tag) error {
	if m.createFn != nil {
		return m.createFn(ctx, tag)
	}
	tag.ID = 1
	return nil
}

func (m *mockTagRepo) FindByID(ctx context.Context, id int) (*Tag, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return &Tag{ID: id, CampaignID: "camp-1", Name: "Test", Slug: "test", Color: "#6b7280"}, nil
}

func (m *mockTagRepo) ListByCampaign(ctx context.Context, campaignID string, includeDmOnly bool) ([]Tag, error) {
	if m.listByCampaignFn != nil {
		return m.listByCampaignFn(ctx, campaignID, includeDmOnly)
	}
	return nil, nil
}

func (m *mockTagRepo) Update(ctx context.Context, tag *Tag) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, tag)
	}
	return nil
}

func (m *mockTagRepo) Delete(ctx context.Context, id int) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockTagRepo) AddTagToEntity(ctx context.Context, entityID string, tagID int) error {
	if m.addTagToEntityFn != nil {
		return m.addTagToEntityFn(ctx, entityID, tagID)
	}
	return nil
}

func (m *mockTagRepo) RemoveTagFromEntity(ctx context.Context, entityID string, tagID int) error {
	if m.removeTagFromEntityFn != nil {
		return m.removeTagFromEntityFn(ctx, entityID, tagID)
	}
	return nil
}

func (m *mockTagRepo) GetEntityTags(ctx context.Context, entityID string, includeDmOnly bool) ([]Tag, error) {
	if m.getEntityTagsFn != nil {
		return m.getEntityTagsFn(ctx, entityID, includeDmOnly)
	}
	return nil, nil
}

func (m *mockTagRepo) GetEntityTagsBatch(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]Tag, error) {
	if m.getEntityTagsBatchFn != nil {
		return m.getEntityTagsBatchFn(ctx, entityIDs, includeDmOnly)
	}
	return nil, nil
}

// --- Test Helpers ---

func newTestService(repo *mockTagRepo) TagService {
	return NewTagService(repo)
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
	var captured *Tag
	repo := &mockTagRepo{
		createFn: func(_ context.Context, tag *Tag) error {
			captured = tag
			tag.ID = 42
			return nil
		},
	}
	svc := newTestService(repo)

	result, err := svc.Create(context.Background(), "camp-1", "Important NPCs", "#ff5733", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != 42 {
		t.Errorf("expected ID 42, got %d", result.ID)
	}
	if result.Name != "Important NPCs" {
		t.Errorf("expected name 'Important NPCs', got %q", result.Name)
	}
	if result.Color != "#ff5733" {
		t.Errorf("expected color '#ff5733', got %q", result.Color)
	}
	if captured.Slug != "important-npcs" {
		t.Errorf("expected slug 'important-npcs', got %q", captured.Slug)
	}
	if captured.CampaignID != "camp-1" {
		t.Errorf("expected campaignID 'camp-1', got %q", captured.CampaignID)
	}
}

func TestCreate_DefaultColor(t *testing.T) {
	repo := &mockTagRepo{
		createFn: func(_ context.Context, tag *Tag) error {
			tag.ID = 1
			return nil
		},
	}
	svc := newTestService(repo)

	result, err := svc.Create(context.Background(), "camp-1", "Test Tag", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Color != "#6b7280" {
		t.Errorf("expected default color '#6b7280', got %q", result.Color)
	}
}

func TestCreate_DmOnly(t *testing.T) {
	var captured *Tag
	repo := &mockTagRepo{
		createFn: func(_ context.Context, tag *Tag) error {
			captured = tag
			tag.ID = 1
			return nil
		},
	}
	svc := newTestService(repo)

	_, err := svc.Create(context.Background(), "camp-1", "Secret Tag", "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !captured.DmOnly {
		t.Error("expected dmOnly to be true")
	}
}

func TestCreate_EmptyName(t *testing.T) {
	svc := newTestService(&mockTagRepo{})
	_, err := svc.Create(context.Background(), "camp-1", "", "", false)
	assertAppError(t, err, 400)
}

func TestCreate_WhitespaceOnlyName(t *testing.T) {
	svc := newTestService(&mockTagRepo{})
	_, err := svc.Create(context.Background(), "camp-1", "   ", "", false)
	assertAppError(t, err, 400)
}

func TestCreate_TrimsName(t *testing.T) {
	repo := &mockTagRepo{
		createFn: func(_ context.Context, tag *Tag) error {
			tag.ID = 1
			return nil
		},
	}
	svc := newTestService(repo)

	result, err := svc.Create(context.Background(), "camp-1", "  Quest Items  ", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "Quest Items" {
		t.Errorf("expected trimmed 'Quest Items', got %q", result.Name)
	}
}

func TestCreate_InvalidColor(t *testing.T) {
	svc := newTestService(&mockTagRepo{})

	tests := []struct {
		name  string
		color string
	}{
		{"no hash", "ff5733"},
		{"short hex", "#fff"},
		{"too long", "#ff57331"},
		{"non-hex chars", "#gggggg"},
		{"random string", "red"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), "camp-1", "Tag", tt.color, false)
			assertAppError(t, err, 400)
		})
	}
}

func TestCreate_ValidColors(t *testing.T) {
	repo := &mockTagRepo{
		createFn: func(_ context.Context, tag *Tag) error {
			tag.ID = 1
			return nil
		},
	}
	svc := newTestService(repo)

	tests := []string{"#ff5733", "#FF5733", "#000000", "#ffffff", "#aAbBcC"}
	for _, color := range tests {
		t.Run(color, func(t *testing.T) {
			_, err := svc.Create(context.Background(), "camp-1", "Tag", color, false)
			if err != nil {
				t.Errorf("color %q should be valid, got error: %v", color, err)
			}
		})
	}
}

func TestCreate_DuplicateSlug(t *testing.T) {
	repo := &mockTagRepo{
		createFn: func(_ context.Context, _ *Tag) error {
			return apperror.NewConflict("a tag with this name already exists in the campaign")
		},
	}
	svc := newTestService(repo)

	_, err := svc.Create(context.Background(), "camp-1", "Duplicate", "", false)
	assertAppError(t, err, 409)
}

func TestCreate_RepoError(t *testing.T) {
	repo := &mockTagRepo{
		createFn: func(_ context.Context, _ *Tag) error {
			return errors.New("db error")
		},
	}
	svc := newTestService(repo)

	_, err := svc.Create(context.Background(), "camp-1", "Tag", "", false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Update Tests ---

func TestUpdate_Success(t *testing.T) {
	repo := &mockTagRepo{
		findByIDFn: func(_ context.Context, id int) (*Tag, error) {
			return &Tag{ID: id, CampaignID: "camp-1", Name: "Old Name", Slug: "old-name", Color: "#000000"}, nil
		},
		updateFn: func(_ context.Context, tag *Tag) error {
			return nil
		},
	}
	svc := newTestService(repo)

	result, err := svc.Update(context.Background(), 1, "New Name", "#ff5733", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "New Name" {
		t.Errorf("expected 'New Name', got %q", result.Name)
	}
	if result.Slug != "new-name" {
		t.Errorf("expected slug 'new-name', got %q", result.Slug)
	}
	if result.Color != "#ff5733" {
		t.Errorf("expected color '#ff5733', got %q", result.Color)
	}
	if !result.DmOnly {
		t.Error("expected dmOnly to be true")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	repo := &mockTagRepo{
		findByIDFn: func(_ context.Context, _ int) (*Tag, error) {
			return nil, apperror.NewNotFound("tag not found")
		},
	}
	svc := newTestService(repo)

	_, err := svc.Update(context.Background(), 999, "Name", "", false)
	assertAppError(t, err, 404)
}

func TestUpdate_EmptyName(t *testing.T) {
	repo := &mockTagRepo{} // Default FindByID returns a tag.
	svc := newTestService(repo)

	_, err := svc.Update(context.Background(), 1, "", "", false)
	assertAppError(t, err, 400)
}

func TestUpdate_InvalidColor(t *testing.T) {
	repo := &mockTagRepo{} // Default FindByID returns a tag.
	svc := newTestService(repo)

	_, err := svc.Update(context.Background(), 1, "Name", "invalid", false)
	assertAppError(t, err, 400)
}

func TestUpdate_DefaultColor(t *testing.T) {
	repo := &mockTagRepo{}
	svc := newTestService(repo)

	result, err := svc.Update(context.Background(), 1, "Name", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Color != "#6b7280" {
		t.Errorf("expected default color, got %q", result.Color)
	}
}

// --- Delete Tests ---

func TestDelete_Success(t *testing.T) {
	deleted := false
	repo := &mockTagRepo{
		deleteFn: func(_ context.Context, id int) error {
			if id != 42 {
				t.Errorf("expected delete ID 42, got %d", id)
			}
			deleted = true
			return nil
		},
	}
	svc := newTestService(repo)

	err := svc.Delete(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("expected delete to be called")
	}
}

func TestDelete_NotFound(t *testing.T) {
	repo := &mockTagRepo{
		deleteFn: func(_ context.Context, _ int) error {
			return apperror.NewNotFound("tag not found")
		},
	}
	svc := newTestService(repo)

	err := svc.Delete(context.Background(), 999)
	assertAppError(t, err, 404)
}

// --- SetEntityTags Tests ---

func TestSetEntityTags_AddOnly(t *testing.T) {
	addedTags := []int{}
	repo := &mockTagRepo{
		listByCampaignFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return []Tag{{ID: 10}, {ID: 20}, {ID: 30}}, nil
		},
		getEntityTagsFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return nil, nil // No current tags.
		},
		addTagToEntityFn: func(_ context.Context, _ string, tagID int) error {
			addedTags = append(addedTags, tagID)
			return nil
		},
	}
	svc := newTestService(repo)

	err := svc.SetEntityTags(context.Background(), "entity-1", "camp-1", []int{10, 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(addedTags) != 2 {
		t.Fatalf("expected 2 adds, got %d", len(addedTags))
	}
}

func TestSetEntityTags_RemoveOnly(t *testing.T) {
	removedTags := []int{}
	repo := &mockTagRepo{
		getEntityTagsFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return []Tag{{ID: 10}, {ID: 20}}, nil
		},
		removeTagFromEntityFn: func(_ context.Context, _ string, tagID int) error {
			removedTags = append(removedTags, tagID)
			return nil
		},
	}
	svc := newTestService(repo)

	// Empty tagIDs removes all current tags.
	err := svc.SetEntityTags(context.Background(), "entity-1", "camp-1", []int{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(removedTags) != 2 {
		t.Fatalf("expected 2 removals, got %d", len(removedTags))
	}
}

func TestSetEntityTags_Diff(t *testing.T) {
	addedTags := []int{}
	removedTags := []int{}
	repo := &mockTagRepo{
		listByCampaignFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return []Tag{{ID: 10}, {ID: 20}, {ID: 30}}, nil
		},
		getEntityTagsFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return []Tag{{ID: 10}, {ID: 20}}, nil // Currently has 10 and 20.
		},
		addTagToEntityFn: func(_ context.Context, _ string, tagID int) error {
			addedTags = append(addedTags, tagID)
			return nil
		},
		removeTagFromEntityFn: func(_ context.Context, _ string, tagID int) error {
			removedTags = append(removedTags, tagID)
			return nil
		},
	}
	svc := newTestService(repo)

	// Desired: 20, 30 → remove 10, add 30, keep 20.
	err := svc.SetEntityTags(context.Background(), "entity-1", "camp-1", []int{20, 30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(removedTags) != 1 || removedTags[0] != 10 {
		t.Errorf("expected remove [10], got %v", removedTags)
	}
	if len(addedTags) != 1 || addedTags[0] != 30 {
		t.Errorf("expected add [30], got %v", addedTags)
	}
}

func TestSetEntityTags_Idempotent(t *testing.T) {
	addCount := 0
	removeCount := 0
	repo := &mockTagRepo{
		listByCampaignFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return []Tag{{ID: 10}, {ID: 20}}, nil
		},
		getEntityTagsFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return []Tag{{ID: 10}, {ID: 20}}, nil // Already has exactly these tags.
		},
		addTagToEntityFn: func(_ context.Context, _ string, _ int) error {
			addCount++
			return nil
		},
		removeTagFromEntityFn: func(_ context.Context, _ string, _ int) error {
			removeCount++
			return nil
		},
	}
	svc := newTestService(repo)

	err := svc.SetEntityTags(context.Background(), "entity-1", "camp-1", []int{10, 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addCount != 0 {
		t.Errorf("expected 0 adds (idempotent), got %d", addCount)
	}
	if removeCount != 0 {
		t.Errorf("expected 0 removes (idempotent), got %d", removeCount)
	}
}

func TestSetEntityTags_CrossCampaignTag(t *testing.T) {
	repo := &mockTagRepo{
		listByCampaignFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return []Tag{{ID: 10}, {ID: 20}}, nil // Campaign only has 10 and 20.
		},
	}
	svc := newTestService(repo)

	// Tag ID 99 doesn't belong to this campaign.
	err := svc.SetEntityTags(context.Background(), "entity-1", "camp-1", []int{10, 99})
	assertAppError(t, err, 400)
}

func TestSetEntityTags_EmptyTagIDs_SkipsValidation(t *testing.T) {
	listCalled := false
	repo := &mockTagRepo{
		listByCampaignFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			listCalled = true
			return nil, nil
		},
		getEntityTagsFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return nil, nil
		},
	}
	svc := newTestService(repo)

	err := svc.SetEntityTags(context.Background(), "entity-1", "camp-1", []int{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if listCalled {
		t.Error("ListByCampaign should not be called when tagIDs is empty")
	}
}

func TestSetEntityTags_GetEntityTagsError(t *testing.T) {
	repo := &mockTagRepo{
		listByCampaignFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return []Tag{{ID: 10}}, nil
		},
		getEntityTagsFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestService(repo)

	err := svc.SetEntityTags(context.Background(), "entity-1", "camp-1", []int{10})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSetEntityTags_AddError(t *testing.T) {
	repo := &mockTagRepo{
		listByCampaignFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return []Tag{{ID: 10}}, nil
		},
		getEntityTagsFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return nil, nil // No current tags.
		},
		addTagToEntityFn: func(_ context.Context, _ string, _ int) error {
			return errors.New("db error")
		},
	}
	svc := newTestService(repo)

	err := svc.SetEntityTags(context.Background(), "entity-1", "camp-1", []int{10})
	if err == nil {
		t.Fatal("expected error on add failure")
	}
}

func TestSetEntityTags_RemoveError(t *testing.T) {
	repo := &mockTagRepo{
		getEntityTagsFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return []Tag{{ID: 10}}, nil
		},
		removeTagFromEntityFn: func(_ context.Context, _ string, _ int) error {
			return errors.New("db error")
		},
	}
	svc := newTestService(repo)

	err := svc.SetEntityTags(context.Background(), "entity-1", "camp-1", []int{})
	if err == nil {
		t.Fatal("expected error on remove failure")
	}
}

// --- GetEntityTags / GetEntityTagsBatch Tests ---

func TestGetEntityTags_PassesIncludeDmOnly(t *testing.T) {
	var capturedInclude bool
	repo := &mockTagRepo{
		getEntityTagsFn: func(_ context.Context, _ string, includeDmOnly bool) ([]Tag, error) {
			capturedInclude = includeDmOnly
			return nil, nil
		},
	}
	svc := newTestService(repo)

	_, _ = svc.GetEntityTags(context.Background(), "entity-1", true)
	if !capturedInclude {
		t.Error("expected includeDmOnly=true to be passed through")
	}

	_, _ = svc.GetEntityTags(context.Background(), "entity-1", false)
	if capturedInclude {
		t.Error("expected includeDmOnly=false to be passed through")
	}
}

func TestGetEntityTagsBatch_PassesThrough(t *testing.T) {
	expected := map[string][]Tag{
		"entity-1": {{ID: 1, Name: "Tag1"}},
		"entity-2": {{ID: 2, Name: "Tag2"}},
	}
	repo := &mockTagRepo{
		getEntityTagsBatchFn: func(_ context.Context, entityIDs []string, _ bool) (map[string][]Tag, error) {
			if len(entityIDs) != 2 {
				t.Errorf("expected 2 entity IDs, got %d", len(entityIDs))
			}
			return expected, nil
		},
	}
	svc := newTestService(repo)

	result, err := svc.GetEntityTagsBatch(context.Background(), []string{"entity-1", "entity-2"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 entries in result, got %d", len(result))
	}
}

// --- ListByCampaign Tests ---

func TestListByCampaign_PassesIncludeDmOnly(t *testing.T) {
	var capturedInclude bool
	repo := &mockTagRepo{
		listByCampaignFn: func(_ context.Context, _ string, includeDmOnly bool) ([]Tag, error) {
			capturedInclude = includeDmOnly
			return nil, nil
		},
	}
	svc := newTestService(repo)

	_, _ = svc.ListByCampaign(context.Background(), "camp-1", false)
	if capturedInclude {
		t.Error("expected includeDmOnly=false to be passed through")
	}
}

// --- Slug Generation Tests ---

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "Quest Items", "quest-items"},
		{"special chars", "D&D 5e", "d-d-5e"},
		{"leading trailing spaces", "  hello world  ", "hello-world"},
		{"multiple special", "hello---world!!!foo", "hello-world-foo"},
		{"all non-alpha", "!!!!", "tag"},
		{"single word", "NPCs", "npcs"},
		{"numbers", "Level 5", "level-5"},
		{"unicode", "café", "caf"},
		{"mixed case", "ThE BiG bAd", "the-big-bad"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSlug(tt.input)
			if result != tt.expected {
				t.Errorf("generateSlug(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
