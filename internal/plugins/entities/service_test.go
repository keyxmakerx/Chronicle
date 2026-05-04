package entities

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repositories ---

// mockEntityTypeRepo implements EntityTypeRepository for testing.
type mockEntityTypeRepo struct {
	findByIDFn       func(ctx context.Context, id int) (*EntityType, error)
	findBySlugFn     func(ctx context.Context, campaignID, slug string) (*EntityType, error)
	listByCampaignFn func(ctx context.Context, campaignID string) ([]EntityType, error)
	updateLayoutFn   func(ctx context.Context, id int, layoutJSON string) error
	seedDefaultsFn   func(ctx context.Context, campaignID string) error
	createFn         func(ctx context.Context, et *EntityType) error
	updateFn         func(ctx context.Context, et *EntityType) error
	deleteFn         func(ctx context.Context, id int) error
	slugExistsFn     func(ctx context.Context, campaignID, slug string) (bool, error)
	maxSortOrderFn   func(ctx context.Context, campaignID string) (int, error)
}

func (m *mockEntityTypeRepo) Create(ctx context.Context, et *EntityType) error {
	if m.createFn != nil {
		return m.createFn(ctx, et)
	}
	return nil
}

func (m *mockEntityTypeRepo) FindByID(ctx context.Context, id int) (*EntityType, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, apperror.NewNotFound("entity type not found")
}

func (m *mockEntityTypeRepo) FindBySlug(ctx context.Context, campaignID, slug string) (*EntityType, error) {
	if m.findBySlugFn != nil {
		return m.findBySlugFn(ctx, campaignID, slug)
	}
	return nil, apperror.NewNotFound("entity type not found")
}

func (m *mockEntityTypeRepo) ListByCampaign(ctx context.Context, campaignID string) ([]EntityType, error) {
	if m.listByCampaignFn != nil {
		return m.listByCampaignFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockEntityTypeRepo) ListChildTypes(_ context.Context, _ int) ([]EntityType, error) {
	return nil, nil
}

func (m *mockEntityTypeRepo) ListAll(_ context.Context) ([]EntityType, error) {
	return nil, nil
}

func (m *mockEntityTypeRepo) ListByPresetCategory(ctx context.Context, campaignID, category string) ([]EntityType, error) {
	return nil, nil
}

func (m *mockEntityTypeRepo) UpdateLayout(ctx context.Context, id int, layoutJSON string) error {
	if m.updateLayoutFn != nil {
		return m.updateLayoutFn(ctx, id, layoutJSON)
	}
	return nil
}

func (m *mockEntityTypeRepo) UpdateColor(ctx context.Context, id int, color string) error {
	return nil
}

func (m *mockEntityTypeRepo) UpdateDashboard(ctx context.Context, id int, description *string, pinnedIDs []string) error {
	return nil
}

func (m *mockEntityTypeRepo) UpdateDashboardLayout(ctx context.Context, id int, layoutJSON *string) error {
	return nil
}

func (m *mockEntityTypeRepo) SeedDefaults(ctx context.Context, campaignID string) error {
	if m.seedDefaultsFn != nil {
		return m.seedDefaultsFn(ctx, campaignID)
	}
	return nil
}

func (m *mockEntityTypeRepo) SeedFromTypes(ctx context.Context, campaignID string, types []EntityType) error {
	return nil
}

func (m *mockEntityTypeRepo) Update(ctx context.Context, et *EntityType) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, et)
	}
	return nil
}

func (m *mockEntityTypeRepo) Delete(ctx context.Context, id int) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockEntityTypeRepo) SlugExists(ctx context.Context, campaignID, slug string) (bool, error) {
	if m.slugExistsFn != nil {
		return m.slugExistsFn(ctx, campaignID, slug)
	}
	return false, nil
}

func (m *mockEntityTypeRepo) MaxSortOrder(ctx context.Context, campaignID string) (int, error) {
	if m.maxSortOrderFn != nil {
		return m.maxSortOrderFn(ctx, campaignID)
	}
	return 0, nil
}

// mockEntityRepo implements EntityRepository for testing.
type mockEntityRepo struct {
	createFn         func(ctx context.Context, entity *Entity) error
	findByIDFn       func(ctx context.Context, id string) (*Entity, error)
	findBySlugFn     func(ctx context.Context, campaignID, slug string) (*Entity, error)
	updateFn         func(ctx context.Context, entity *Entity) error
	updateEntryFn    func(ctx context.Context, id, entryJSON, entryHTML string) error
	updateImageFn    func(ctx context.Context, id, imagePath string) error
	deleteFn         func(ctx context.Context, id string) error
	slugExistsFn     func(ctx context.Context, campaignID, slug string) (bool, error)
	listByCampaignFn func(ctx context.Context, campaignID string, typeIDs []int, role int, userID string, opts ListOptions) ([]Entity, int, error)
	searchFn         func(ctx context.Context, campaignID, query string, typeIDs []int, role int, userID string, opts ListOptions) ([]Entity, int, error)
	countByTypeFn    func(ctx context.Context, campaignID string, role int, userID string) (map[int]int, error)
	listRecentFn     func(ctx context.Context, campaignID string, role int, userID string, limit int) ([]Entity, error)
	findChildrenFn   func(ctx context.Context, parentID string, role int, userID string) ([]Entity, error)
	findAncestorsFn  func(ctx context.Context, entityID string) ([]Entity, error)
	updateParentFn   func(ctx context.Context, entityID string, parentID *string) error
	findBacklinksFn  func(ctx context.Context, entityID string, role int, userID string) ([]Entity, error)
	setAliasesFn     func(ctx context.Context, entityID string, aliases []string) error
	updatePrivateFn  func(ctx context.Context, entityID string, isPrivate bool) error
	listByOwnerFn    func(ctx context.Context, campaignID, ownerUserID string) ([]Entity, error)
	updateOwnerFn    func(ctx context.Context, entityID string, ownerUserID *string) error
	updateMapIDFn    func(ctx context.Context, entityID string, mapID *string) error
}

func (m *mockEntityRepo) Create(ctx context.Context, entity *Entity) error {
	if m.createFn != nil {
		return m.createFn(ctx, entity)
	}
	return nil
}

func (m *mockEntityRepo) FindByID(ctx context.Context, id string) (*Entity, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, apperror.NewNotFound("entity not found")
}

func (m *mockEntityRepo) FindBySlug(ctx context.Context, campaignID, slug string) (*Entity, error) {
	if m.findBySlugFn != nil {
		return m.findBySlugFn(ctx, campaignID, slug)
	}
	return nil, apperror.NewNotFound("entity not found")
}

func (m *mockEntityRepo) Update(ctx context.Context, entity *Entity) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, entity)
	}
	return nil
}

func (m *mockEntityRepo) UpdateEntry(ctx context.Context, id, entryJSON, entryHTML, searchText string) error {
	if m.updateEntryFn != nil {
		return m.updateEntryFn(ctx, id, entryJSON, entryHTML)
	}
	return nil
}

func (m *mockEntityRepo) UpdatePlayerNotes(ctx context.Context, id, notesJSON, notesHTML string) error {
	return nil
}

func (m *mockEntityRepo) UpdateFields(ctx context.Context, id string, fieldsData map[string]any, searchText string) error {
	return nil
}

func (m *mockEntityRepo) UpdateFieldOverrides(ctx context.Context, id string, overrides *FieldOverrides) error {
	return nil
}

func (m *mockEntityRepo) UpdateImage(ctx context.Context, id, imagePath string) error {
	if m.updateImageFn != nil {
		return m.updateImageFn(ctx, id, imagePath)
	}
	return nil
}

func (m *mockEntityRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockEntityRepo) SlugExists(ctx context.Context, campaignID, slug string) (bool, error) {
	if m.slugExistsFn != nil {
		return m.slugExistsFn(ctx, campaignID, slug)
	}
	return false, nil
}

func (m *mockEntityRepo) ListByCampaign(ctx context.Context, campaignID string, typeIDs []int, role int, userID string, opts ListOptions) ([]Entity, int, error) {
	if m.listByCampaignFn != nil {
		return m.listByCampaignFn(ctx, campaignID, typeIDs, role, userID, opts)
	}
	return nil, 0, nil
}

func (m *mockEntityRepo) Search(ctx context.Context, campaignID, query string, typeIDs []int, role int, userID string, opts ListOptions) ([]Entity, int, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, campaignID, query, typeIDs, role, userID, opts)
	}
	return nil, 0, nil
}

func (m *mockEntityRepo) CountByType(ctx context.Context, campaignID string, role int, userID string) (map[int]int, error) {
	if m.countByTypeFn != nil {
		return m.countByTypeFn(ctx, campaignID, role, userID)
	}
	return nil, nil
}

func (m *mockEntityRepo) ListRecent(ctx context.Context, campaignID string, role int, userID string, limit int) ([]Entity, error) {
	if m.listRecentFn != nil {
		return m.listRecentFn(ctx, campaignID, role, userID, limit)
	}
	return nil, nil
}

func (m *mockEntityRepo) FindChildren(ctx context.Context, parentID string, role int, userID string) ([]Entity, error) {
	if m.findChildrenFn != nil {
		return m.findChildrenFn(ctx, parentID, role, userID)
	}
	return nil, nil
}

func (m *mockEntityRepo) FindAncestors(ctx context.Context, entityID string) ([]Entity, error) {
	if m.findAncestorsFn != nil {
		return m.findAncestorsFn(ctx, entityID)
	}
	return nil, nil
}

func (m *mockEntityRepo) UpdateParent(ctx context.Context, entityID, campaignID string, parentID *string) error {
	if m.updateParentFn != nil {
		return m.updateParentFn(ctx, entityID, parentID)
	}
	return nil
}

func (m *mockEntityRepo) UpdateParentNode(ctx context.Context, entityID, campaignID string, parentNodeID *string) error {
	return nil
}

func (m *mockEntityRepo) UpdateSortOrder(ctx context.Context, entityID, campaignID string, sortOrder int) error {
	return nil
}

func (m *mockEntityRepo) FindBacklinks(ctx context.Context, entityID string, role int, userID string) ([]Entity, error) {
	if m.findBacklinksFn != nil {
		return m.findBacklinksFn(ctx, entityID, role, userID)
	}
	return nil, nil
}

func (m *mockEntityRepo) UpdatePopupConfig(ctx context.Context, entityID string, config *PopupConfig) error {
	return nil
}

func (m *mockEntityRepo) CopyEntityTags(ctx context.Context, sourceEntityID, targetEntityID string) error {
	return nil
}

func (m *mockEntityRepo) ListNames(_ context.Context, _ string, _ int, _ string) ([]EntityNameEntry, error) {
	return nil, nil
}

func (m *mockEntityRepo) ListAliases(_ context.Context, _ string) ([]EntityAlias, error) {
	return nil, nil
}

func (m *mockEntityRepo) SetAliases(ctx context.Context, entityID string, aliases []string) error {
	if m.setAliasesFn != nil {
		return m.setAliasesFn(ctx, entityID, aliases)
	}
	return nil
}

func (m *mockEntityRepo) FindAllMentionLinks(_ context.Context, _ string, _ int, _ string) ([]MentionLink, error) {
	return nil, nil
}

func (m *mockEntityRepo) UpdateCoverImage(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockEntityRepo) UpdatePrivate(ctx context.Context, entityID string, isPrivate bool) error {
	if m.updatePrivateFn != nil {
		return m.updatePrivateFn(ctx, entityID, isPrivate)
	}
	return nil
}

func (m *mockEntityRepo) UpdateEntityType(_ context.Context, _ string, _ int) error {
	return nil
}

func (m *mockEntityRepo) ListByOwner(ctx context.Context, campaignID, ownerUserID string) ([]Entity, error) {
	if m.listByOwnerFn != nil {
		return m.listByOwnerFn(ctx, campaignID, ownerUserID)
	}
	return nil, nil
}

func (m *mockEntityRepo) UpdateOwner(ctx context.Context, entityID string, ownerUserID *string) error {
	if m.updateOwnerFn != nil {
		return m.updateOwnerFn(ctx, entityID, ownerUserID)
	}
	return nil
}

func (m *mockEntityRepo) UpdateMapID(ctx context.Context, entityID string, mapID *string) error {
	if m.updateMapIDFn != nil {
		return m.updateMapIDFn(ctx, entityID, mapID)
	}
	return nil
}

// --- Test Helpers ---

// mockPermissionRepo implements EntityPermissionRepository for testing.
type mockPermissionRepo struct {
	listByEntityFn         func(ctx context.Context, entityID string) ([]EntityPermission, error)
	setPermissionsFn       func(ctx context.Context, entityID string, grants []PermissionGrant) error
	deleteByEntityFn       func(ctx context.Context, entityID string) error
	getEffectivePermFn     func(ctx context.Context, entityID string, role int, userID string) (*EffectivePermission, error)
	updateVisibilityFn     func(ctx context.Context, entityID string, visibility VisibilityMode) error
}

func (m *mockPermissionRepo) ListByEntity(ctx context.Context, entityID string) ([]EntityPermission, error) {
	if m.listByEntityFn != nil {
		return m.listByEntityFn(ctx, entityID)
	}
	return nil, nil
}

func (m *mockPermissionRepo) SetPermissions(ctx context.Context, entityID string, grants []PermissionGrant) error {
	if m.setPermissionsFn != nil {
		return m.setPermissionsFn(ctx, entityID, grants)
	}
	return nil
}

func (m *mockPermissionRepo) DeleteByEntity(ctx context.Context, entityID string) error {
	if m.deleteByEntityFn != nil {
		return m.deleteByEntityFn(ctx, entityID)
	}
	return nil
}

func (m *mockPermissionRepo) GetEffectivePermission(ctx context.Context, entityID string, role int, userID string) (*EffectivePermission, error) {
	if m.getEffectivePermFn != nil {
		return m.getEffectivePermFn(ctx, entityID, role, userID)
	}
	return &EffectivePermission{CanView: true, CanEdit: true}, nil
}

func (m *mockPermissionRepo) UpdateVisibility(ctx context.Context, entityID string, visibility VisibilityMode) error {
	if m.updateVisibilityFn != nil {
		return m.updateVisibilityFn(ctx, entityID, visibility)
	}
	return nil
}

func newTestService(entityRepo *mockEntityRepo, typeRepo *mockEntityTypeRepo) EntityService {
	return NewEntityService(entityRepo, typeRepo, &mockPermissionRepo{})
}

// assertAppError checks that an error is an AppError with the expected code.
func assertAppError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != expectedCode {
		t.Errorf("expected status code %d, got %d (message: %s)", expectedCode, appErr.Code, appErr.Message)
	}
}

// --- Create Tests ---

func TestCreate_Success(t *testing.T) {
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
			return &EntityType{ID: 1, CampaignID: "camp-1", Slug: "character"}, nil
		},
	}
	entityRepo := &mockEntityRepo{
		slugExistsFn: func(_ context.Context, _, _ string) (bool, error) {
			return false, nil
		},
	}

	svc := newTestService(entityRepo, typeRepo)
	entity, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
		Name:         "Gandalf",
		EntityTypeID: 1,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entity == nil {
		t.Fatal("expected entity, got nil")
	}
	if entity.Name != "Gandalf" {
		t.Errorf("expected name 'Gandalf', got %q", entity.Name)
	}
	if entity.Slug != "gandalf" {
		t.Errorf("expected slug 'gandalf', got %q", entity.Slug)
	}
	if entity.CampaignID != "camp-1" {
		t.Errorf("expected campaign_id 'camp-1', got %q", entity.CampaignID)
	}
	if entity.CreatedBy != "user-1" {
		t.Errorf("expected created_by 'user-1', got %q", entity.CreatedBy)
	}
	if entity.ID == "" {
		t.Error("expected a generated UUID, got empty string")
	}
}

// TestCreate_OwnerUserID guards CH1+CH5 plumbing: when the API
// passes through an owner_user_id, the service must forward it onto
// the persisted Entity row so the player landing query
// ("characters owned by current user") finds the new entity.
//
// Cross-campaign membership validation deliberately lives at the call
// site (sync API handler) rather than the service — the test for
// that lives where the validation lives.
func TestCreate_OwnerUserID(t *testing.T) {
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
			return &EntityType{ID: 1, CampaignID: "camp-1", Slug: "character"}, nil
		},
	}
	var captured *Entity
	entityRepo := &mockEntityRepo{
		slugExistsFn: func(_ context.Context, _, _ string) (bool, error) { return false, nil },
		createFn: func(_ context.Context, e *Entity) error {
			captured = e
			return nil
		},
	}
	svc := newTestService(entityRepo, typeRepo)

	for _, tc := range []struct {
		name        string
		input       *string
		wantOwner   *string
		description string
	}{
		{
			name:        "claimed at create",
			input:       strPtr("user-claim"),
			wantOwner:   strPtr("user-claim"),
			description: "owner_user_id flows through to the persisted row",
		},
		{
			name:        "nil leaves unclaimed",
			input:       nil,
			wantOwner:   nil,
			description: "omitted owner_user_id results in nil OwnerUserID",
		},
		{
			name:        "empty string normalised to unclaimed",
			input:       strPtr(""),
			wantOwner:   nil,
			description: "empty-string owner_user_id is treated as unclaimed (defensive)",
		},
		{
			name:        "whitespace normalised to unclaimed",
			input:       strPtr("   "),
			wantOwner:   nil,
			description: "whitespace-only owner_user_id is treated as unclaimed (defensive)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			captured = nil
			_, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
				Name:         "Gandalf",
				EntityTypeID: 1,
				OwnerUserID:  tc.input,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if captured == nil {
				t.Fatal("expected CreatePackage to be called; nothing captured")
			}
			switch {
			case tc.wantOwner == nil && captured.OwnerUserID != nil:
				t.Errorf("%s: expected nil OwnerUserID, got %q", tc.description, *captured.OwnerUserID)
			case tc.wantOwner != nil && captured.OwnerUserID == nil:
				t.Errorf("%s: expected OwnerUserID %q, got nil", tc.description, *tc.wantOwner)
			case tc.wantOwner != nil && *captured.OwnerUserID != *tc.wantOwner:
				t.Errorf("%s: expected OwnerUserID %q, got %q", tc.description, *tc.wantOwner, *captured.OwnerUserID)
			}
		})
	}
}

func TestCreate_EmptyName(t *testing.T) {
	svc := newTestService(&mockEntityRepo{}, &mockEntityTypeRepo{})
	_, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
		Name:         "",
		EntityTypeID: 1,
	})
	assertAppError(t, err, 400)
}

func TestCreate_WhitespaceOnlyName(t *testing.T) {
	svc := newTestService(&mockEntityRepo{}, &mockEntityTypeRepo{})
	_, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
		Name:         "   ",
		EntityTypeID: 1,
	})
	assertAppError(t, err, 400)
}

func TestCreate_NameTooLong(t *testing.T) {
	svc := newTestService(&mockEntityRepo{}, &mockEntityTypeRepo{})
	longName := make([]byte, 201)
	for i := range longName {
		longName[i] = 'a'
	}
	_, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
		Name:         string(longName),
		EntityTypeID: 1,
	})
	assertAppError(t, err, 400)
}

func TestCreate_InvalidEntityType(t *testing.T) {
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, _ int) (*EntityType, error) {
			return nil, apperror.NewNotFound("not found")
		},
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo)
	_, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
		Name:         "Test",
		EntityTypeID: 999,
	})
	assertAppError(t, err, 400)
}

func TestCreate_EntityTypeWrongCampaign(t *testing.T) {
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, _ int) (*EntityType, error) {
			return &EntityType{ID: 1, CampaignID: "camp-OTHER"}, nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo)
	_, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
		Name:         "Test",
		EntityTypeID: 1,
	})
	assertAppError(t, err, 400)
}

func TestCreate_SlugDedup(t *testing.T) {
	calls := 0
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, _ int) (*EntityType, error) {
			return &EntityType{ID: 1, CampaignID: "camp-1", Slug: "character"}, nil
		},
	}
	entityRepo := &mockEntityRepo{
		slugExistsFn: func(_ context.Context, _, slug string) (bool, error) {
			calls++
			// First two slugs already taken, third available.
			if slug == "gandalf" || slug == "gandalf-2" {
				return true, nil
			}
			return false, nil
		},
	}

	svc := newTestService(entityRepo, typeRepo)
	entity, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
		Name:         "Gandalf",
		EntityTypeID: 1,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entity.Slug != "gandalf-3" {
		t.Errorf("expected slug 'gandalf-3', got %q", entity.Slug)
	}
	if calls != 3 {
		t.Errorf("expected 3 slug checks, got %d", calls)
	}
}

func TestCreate_TrimsName(t *testing.T) {
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, _ int) (*EntityType, error) {
			return &EntityType{ID: 1, CampaignID: "camp-1"}, nil
		},
	}
	entityRepo := &mockEntityRepo{}

	svc := newTestService(entityRepo, typeRepo)
	entity, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
		Name:         "  Gandalf the Grey  ",
		EntityTypeID: 1,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entity.Name != "Gandalf the Grey" {
		t.Errorf("expected trimmed name 'Gandalf the Grey', got %q", entity.Name)
	}
}

func TestCreate_SetsFieldsDataToEmptyMap(t *testing.T) {
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, _ int) (*EntityType, error) {
			return &EntityType{ID: 1, CampaignID: "camp-1"}, nil
		},
	}
	entityRepo := &mockEntityRepo{}

	svc := newTestService(entityRepo, typeRepo)
	entity, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
		Name:         "Test",
		EntityTypeID: 1,
		FieldsData:   nil,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entity.FieldsData == nil {
		t.Error("expected non-nil FieldsData map")
	}
}

// --- Update Tests ---

func TestUpdate_Success(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{
				ID:         "ent-1",
				CampaignID: "camp-1",
				Name:       "Gandalf",
				Slug:       "gandalf",
			}, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	entity, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{
		Name: "Gandalf the White",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entity.Name != "Gandalf the White" {
		t.Errorf("expected name 'Gandalf the White', got %q", entity.Name)
	}
}

func TestUpdate_EmptyName(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{ID: "ent-1", CampaignID: "camp-1", Name: "Test"}, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	_, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{Name: ""})
	assertAppError(t, err, 400)
}

func TestUpdate_RegeneratesSlugOnNameChange(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{
				ID:         "ent-1",
				CampaignID: "camp-1",
				Name:       "Gandalf",
				Slug:       "gandalf",
			}, nil
		},
		slugExistsFn: func(_ context.Context, _, _ string) (bool, error) {
			return false, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	entity, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{
		Name: "Saruman",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entity.Slug != "saruman" {
		t.Errorf("expected slug 'saruman', got %q", entity.Slug)
	}
}

func TestUpdate_KeepsSlugWhenNameUnchanged(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{
				ID:         "ent-1",
				CampaignID: "camp-1",
				Name:       "Gandalf",
				Slug:       "gandalf",
			}, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	entity, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{
		Name: "Gandalf",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entity.Slug != "gandalf" {
		t.Errorf("expected slug to remain 'gandalf', got %q", entity.Slug)
	}
}

func TestUpdate_SetsTypeLabel(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{ID: "ent-1", CampaignID: "camp-1", Name: "Rivendell"}, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	entity, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{
		Name:      "Rivendell",
		TypeLabel: "Elven City",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entity.TypeLabel == nil || *entity.TypeLabel != "Elven City" {
		t.Errorf("expected type_label 'Elven City', got %v", entity.TypeLabel)
	}
}

func TestUpdate_ClearsTypeLabelWhenEmpty(t *testing.T) {
	label := "Old Label"
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{
				ID:         "ent-1",
				CampaignID: "camp-1",
				Name:       "Test",
				TypeLabel:  &label,
			}, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	entity, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{
		Name:      "Test",
		TypeLabel: "",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entity.TypeLabel != nil {
		t.Errorf("expected nil type_label, got %q", *entity.TypeLabel)
	}
}

// --- UpdateEntry Tests ---

func TestUpdateEntry_Success(t *testing.T) {
	entityRepo := &mockEntityRepo{}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	err := svc.UpdateEntry(context.Background(), "ent-1", `{"type":"doc"}`, "<p>Hello</p>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateEntry_EmptyContent(t *testing.T) {
	svc := newTestService(&mockEntityRepo{}, &mockEntityTypeRepo{})
	err := svc.UpdateEntry(context.Background(), "ent-1", "", "<p></p>")
	assertAppError(t, err, 400)
}

func TestUpdateEntry_WhitespaceOnlyContent(t *testing.T) {
	svc := newTestService(&mockEntityRepo{}, &mockEntityTypeRepo{})
	err := svc.UpdateEntry(context.Background(), "ent-1", "   ", "<p></p>")
	assertAppError(t, err, 400)
}

func TestUpdateEntry_RepoError(t *testing.T) {
	entityRepo := &mockEntityRepo{
		updateEntryFn: func(_ context.Context, _, _, _ string) error {
			return apperror.NewNotFound("entity not found")
		},
	}
	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	err := svc.UpdateEntry(context.Background(), "ent-1", `{"type":"doc"}`, "<p>Hello</p>")
	assertAppError(t, err, 404)
}

// --- Delete Tests ---

func TestDelete_Success(t *testing.T) {
	entityRepo := &mockEntityRepo{}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	err := svc.Delete(context.Background(), "ent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete_RepoError(t *testing.T) {
	entityRepo := &mockEntityRepo{
		deleteFn: func(_ context.Context, _ string) error {
			return apperror.NewNotFound("entity not found")
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	err := svc.Delete(context.Background(), "ent-1")
	assertAppError(t, err, 404)
}

// --- List Tests ---

func TestList_DefaultPagination(t *testing.T) {
	called := false
	entityRepo := &mockEntityRepo{
		listByCampaignFn: func(_ context.Context, _ string, _ []int, _ int, _ string, opts ListOptions) ([]Entity, int, error) {
			called = true
			if opts.PerPage != 24 {
				t.Errorf("expected default per_page 24, got %d", opts.PerPage)
			}
			if opts.Page != 1 {
				t.Errorf("expected default page 1, got %d", opts.Page)
			}
			return nil, 0, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	_, _, err := svc.List(context.Background(), "camp-1", 0, 1, "", ListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected repository to be called")
	}
}

func TestList_ClampsPerPage(t *testing.T) {
	entityRepo := &mockEntityRepo{
		listByCampaignFn: func(_ context.Context, _ string, _ []int, _ int, _ string, opts ListOptions) ([]Entity, int, error) {
			if opts.PerPage != 24 {
				t.Errorf("expected clamped per_page 24, got %d", opts.PerPage)
			}
			return nil, 0, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	_, _, _ = svc.List(context.Background(), "camp-1", 0, 1, "", ListOptions{PerPage: 500})
}

// --- Search Tests ---

func TestSearch_MinQueryLength(t *testing.T) {
	svc := newTestService(&mockEntityRepo{}, &mockEntityTypeRepo{})
	_, _, err := svc.Search(context.Background(), "camp-1", "a", 0, 1, "", DefaultListOptions())
	assertAppError(t, err, 400)
}

func TestSearch_TrimsQuery(t *testing.T) {
	svc := newTestService(&mockEntityRepo{}, &mockEntityTypeRepo{})
	_, _, err := svc.Search(context.Background(), "camp-1", "  a  ", 0, 1, "", DefaultListOptions())
	assertAppError(t, err, 400) // "a" is only 1 char after trim
}

func TestSearch_ValidQuery(t *testing.T) {
	entityRepo := &mockEntityRepo{
		searchFn: func(_ context.Context, _ string, query string, _ []int, _ int, _ string, _ ListOptions) ([]Entity, int, error) {
			if query != "gandalf" {
				t.Errorf("expected trimmed query 'gandalf', got %q", query)
			}
			return []Entity{{Name: "Gandalf"}}, 1, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	results, total, err := svc.Search(context.Background(), "camp-1", "  gandalf  ", 0, 1, "", DefaultListOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 result, got %d", total)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 entity, got %d", len(results))
	}
}

// --- Entity Type Tests ---

func TestGetEntityTypes_DelegatesToRepo(t *testing.T) {
	typeRepo := &mockEntityTypeRepo{
		listByCampaignFn: func(_ context.Context, campaignID string) ([]EntityType, error) {
			if campaignID != "camp-1" {
				t.Errorf("expected campaign_id 'camp-1', got %q", campaignID)
			}
			return []EntityType{{Name: "Character"}, {Name: "Location"}}, nil
		},
	}

	svc := newTestService(&mockEntityRepo{}, typeRepo)
	types, err := svc.GetEntityTypes(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(types) != 2 {
		t.Errorf("expected 2 entity types, got %d", len(types))
	}
}

func TestSeedDefaults_DelegatesToRepo(t *testing.T) {
	called := false
	typeRepo := &mockEntityTypeRepo{
		seedDefaultsFn: func(_ context.Context, campaignID string) error {
			called = true
			if campaignID != "camp-1" {
				t.Errorf("expected campaign_id 'camp-1', got %q", campaignID)
			}
			return nil
		},
	}

	svc := newTestService(&mockEntityRepo{}, typeRepo)
	err := svc.SeedDefaults(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected SeedDefaults to be called on repo")
	}
}

// --- Slugify Tests ---

func TestSlugify(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple name", "Gandalf", "gandalf"},
		{"spaces to hyphens", "Gandalf the Grey", "gandalf-the-grey"},
		{"special chars", "Elf-Lord (Rivendell)", "elf-lord-rivendell"},
		{"leading trailing spaces", "  Gandalf  ", "gandalf"},
		{"multiple spaces", "Minas  Tirith", "minas-tirith"},
		{"numbers preserved", "District 9", "district-9"},
		{"all special chars", "!@#$%", "entity"},
		{"empty string", "", "entity"},
		{"unicode simplified", "Théoden", "th-oden"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Slugify(tt.input)
			if got != tt.expected {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// --- ListOptions Tests ---

func TestListOptions_Offset(t *testing.T) {
	tests := []struct {
		name     string
		opts     ListOptions
		expected int
	}{
		{"page 1", ListOptions{Page: 1, PerPage: 24}, 0},
		{"page 2", ListOptions{Page: 2, PerPage: 24}, 24},
		{"page 3 with 10 per page", ListOptions{Page: 3, PerPage: 10}, 20},
		{"page 0 treated as 1", ListOptions{Page: 0, PerPage: 24}, 0},
		{"negative page treated as 1", ListOptions{Page: -1, PerPage: 24}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opts.Offset()
			if got != tt.expected {
				t.Errorf("Offset() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestDefaultListOptions(t *testing.T) {
	opts := DefaultListOptions()
	if opts.Page != 1 {
		t.Errorf("expected default page 1, got %d", opts.Page)
	}
	if opts.PerPage != 24 {
		t.Errorf("expected default per_page 24, got %d", opts.PerPage)
	}
}

// --- Hierarchy Tests ---

func TestCreate_WithParent(t *testing.T) {
	parentID := "parent-123"
	var capturedEntity *Entity
	entityRepo := &mockEntityRepo{
		findByIDFn: func(ctx context.Context, id string) (*Entity, error) {
			if id == parentID {
				return &Entity{ID: parentID, CampaignID: "camp-1", Name: "Parent"}, nil
			}
			return nil, apperror.NewNotFound("not found")
		},
		createFn: func(ctx context.Context, entity *Entity) error {
			capturedEntity = entity
			return nil
		},
	}
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(ctx context.Context, id int) (*EntityType, error) {
			return &EntityType{ID: 1, CampaignID: "camp-1", Slug: "character"}, nil
		},
	}

	svc := newTestService(entityRepo, typeRepo)
	_, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
		Name:         "Child Entity",
		EntityTypeID: 1,
		ParentID:     parentID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedEntity.ParentID == nil || *capturedEntity.ParentID != parentID {
		t.Errorf("expected parent_id %s, got %v", parentID, capturedEntity.ParentID)
	}
}

func TestCreate_WithParentNotFound(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(ctx context.Context, id string) (*Entity, error) {
			return nil, apperror.NewNotFound("not found")
		},
	}
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(ctx context.Context, id int) (*EntityType, error) {
			return &EntityType{ID: 1, CampaignID: "camp-1", Slug: "character"}, nil
		},
	}

	svc := newTestService(entityRepo, typeRepo)
	_, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
		Name:         "Child",
		EntityTypeID: 1,
		ParentID:     "nonexistent",
	})
	assertAppError(t, err, 400)
}

func TestCreate_WithParentWrongCampaign(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(ctx context.Context, id string) (*Entity, error) {
			return &Entity{ID: id, CampaignID: "other-campaign", Name: "Parent"}, nil
		},
	}
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(ctx context.Context, id int) (*EntityType, error) {
			return &EntityType{ID: 1, CampaignID: "camp-1", Slug: "character"}, nil
		},
	}

	svc := newTestService(entityRepo, typeRepo)
	_, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
		Name:         "Child",
		EntityTypeID: 1,
		ParentID:     "parent-in-other-campaign",
	})
	assertAppError(t, err, 400)
}

func TestCreate_NoParent(t *testing.T) {
	var capturedEntity *Entity
	entityRepo := &mockEntityRepo{
		createFn: func(ctx context.Context, entity *Entity) error {
			capturedEntity = entity
			return nil
		},
	}
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(ctx context.Context, id int) (*EntityType, error) {
			return &EntityType{ID: 1, CampaignID: "camp-1", Slug: "character"}, nil
		},
	}

	svc := newTestService(entityRepo, typeRepo)
	_, err := svc.Create(context.Background(), "camp-1", "user-1", CreateEntityInput{
		Name:         "Standalone",
		EntityTypeID: 1,
		ParentID:     "", // Empty = no parent.
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedEntity.ParentID != nil {
		t.Errorf("expected nil parent_id, got %v", capturedEntity.ParentID)
	}
}

func TestUpdate_SelfParent(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(ctx context.Context, id string) (*Entity, error) {
			return &Entity{ID: "ent-1", CampaignID: "camp-1", Name: "Old Name"}, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	_, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{
		Name:     "Updated",
		ParentID: "ent-1", // Self-reference.
	})
	assertAppError(t, err, 400)
}

func TestUpdate_CircularParent(t *testing.T) {
	// Entity A -> set parent to B, but B is a child of A (B's ancestor is A).
	entityRepo := &mockEntityRepo{
		findByIDFn: func(ctx context.Context, id string) (*Entity, error) {
			switch id {
			case "ent-A":
				return &Entity{ID: "ent-A", CampaignID: "camp-1", Name: "A"}, nil
			case "ent-B":
				return &Entity{ID: "ent-B", CampaignID: "camp-1", Name: "B", ParentID: strPtr("ent-A")}, nil
			default:
				return nil, apperror.NewNotFound("not found")
			}
		},
		findAncestorsFn: func(ctx context.Context, entityID string) ([]Entity, error) {
			// B's ancestor chain: [A] (B -> A).
			if entityID == "ent-B" {
				return []Entity{{ID: "ent-A", Name: "A"}}, nil
			}
			return nil, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	_, err := svc.Update(context.Background(), "ent-A", UpdateEntityInput{
		Name:     "A",
		ParentID: "ent-B", // Would create A -> B -> A cycle.
	})
	assertAppError(t, err, 400)
}

func TestGetChildren_DelegatesToRepo(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findChildrenFn: func(ctx context.Context, parentID string, role int, userID string) ([]Entity, error) {
			return []Entity{
				{ID: "child-1", Name: "Child 1"},
				{ID: "child-2", Name: "Child 2"},
			}, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	children, err := svc.GetChildren(context.Background(), "parent-1", 3, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}
}

func TestGetAncestors_DelegatesToRepo(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findAncestorsFn: func(ctx context.Context, entityID string) ([]Entity, error) {
			return []Entity{
				{ID: "parent", Name: "Parent"},
				{ID: "grandparent", Name: "Grandparent"},
			}, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	ancestors, err := svc.GetAncestors(context.Background(), "child-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ancestors) != 2 {
		t.Errorf("expected 2 ancestors, got %d", len(ancestors))
	}
}

func TestGetBacklinks_DelegatesToRepo(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findBacklinksFn: func(ctx context.Context, entityID string, role int, userID string) ([]Entity, error) {
			return []Entity{
				{ID: "ref-1", Name: "Referrer One"},
				{ID: "ref-2", Name: "Referrer Two"},
				{ID: "ref-3", Name: "Referrer Three"},
			}, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	backlinks, err := svc.GetBacklinks(context.Background(), "target-entity", 2, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(backlinks) != 3 {
		t.Errorf("expected 3 backlinks, got %d", len(backlinks))
	}
}

// --- Alias Validation Tests ---

func TestSetAliases_ValidationLimits(t *testing.T) {
	tests := []struct {
		name    string
		aliases []string
		wantErr bool
	}{
		{"empty is ok", []string{}, false},
		{"single alias", []string{"Mithrandir"}, false},
		{"max aliases", []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10"}, false},
		{"too many aliases", []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11"}, true},
		{"too short alias", []string{"x"}, true},
		{"dedup case-insensitive", []string{"Gandalf", "gandalf"}, false},
		{"trims whitespace", []string{"  Gandalf  "}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entityRepo := &mockEntityRepo{}
			svc := newTestService(entityRepo, &mockEntityTypeRepo{})
			err := svc.SetAliases(context.Background(), "entity-1", tt.aliases)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSetAliases_Dedup(t *testing.T) {
	var savedAliases []string
	entityRepo := &mockEntityRepo{
		setAliasesFn: func(_ context.Context, _ string, aliases []string) error {
			savedAliases = aliases
			return nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	err := svc.SetAliases(context.Background(), "entity-1", []string{"Gandalf", "gandalf", "GANDALF", "Mithrandir"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(savedAliases) != 2 {
		t.Errorf("expected 2 deduped aliases, got %d: %v", len(savedAliases), savedAliases)
	}
}

func TestExtractMentionSnippet(t *testing.T) {
	html := `<p>The wizard <a data-mention-id="entity-123" href="/entities/entity-123">@Gandalf</a> arrived at the village of Bree.</p>`
	snippet := extractMentionSnippet(&html, "entity-123")
	if snippet == "" {
		t.Fatal("expected non-empty snippet")
	}
	if len(snippet) > 130 {
		t.Errorf("snippet too long: %d chars", len(snippet))
	}
}

func TestExtractMentionSnippet_Nil(t *testing.T) {
	snippet := extractMentionSnippet(nil, "entity-123")
	if snippet != "" {
		t.Error("expected empty snippet for nil HTML")
	}
}

func TestGetBacklinksWithSnippets(t *testing.T) {
	html := `<p>See <a data-mention-id="target-1" href="/e/target-1">@Target</a> for details.</p>`
	entityRepo := &mockEntityRepo{
		findBacklinksFn: func(_ context.Context, _ string, _ int, _ string) ([]Entity, error) {
			return []Entity{
				{ID: "ref-1", Name: "Source", EntryHTML: &html},
			}, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	entries, err := svc.GetBacklinksWithSnippets(context.Background(), "target-1", 2, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 backlink entry, got %d", len(entries))
	}
	if entries[0].Snippet == "" {
		t.Error("expected non-empty snippet")
	}
	if entries[0].Entity.ID != "ref-1" {
		t.Errorf("expected entity ID ref-1, got %s", entries[0].Entity.ID)
	}
}

// --- Permission Model Validation Tests ---

func TestValidSubjectType(t *testing.T) {
	tests := []struct {
		input SubjectType
		valid bool
	}{
		{SubjectRole, true},
		{SubjectUser, true},
		{SubjectGroup, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidSubjectType(tt.input); got != tt.valid {
			t.Errorf("ValidSubjectType(%q) = %v, want %v", tt.input, got, tt.valid)
		}
	}
}

func TestValidPermission(t *testing.T) {
	tests := []struct {
		input Permission
		valid bool
	}{
		{PermView, true},
		{PermEdit, true},
		{"delete", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidPermission(tt.input); got != tt.valid {
			t.Errorf("ValidPermission(%q) = %v, want %v", tt.input, got, tt.valid)
		}
	}
}

// --- CheckEntityAccess Tests ---

func TestCheckEntityAccess_OwnerAlwaysFullAccess(t *testing.T) {
	svc := newTestService(&mockEntityRepo{}, &mockEntityTypeRepo{})
	perm, err := svc.CheckEntityAccess(context.Background(), "ent-1", 3, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !perm.CanView || !perm.CanEdit {
		t.Errorf("owner should have full access, got view=%v edit=%v", perm.CanView, perm.CanEdit)
	}
}

func TestCheckEntityAccess_DefaultVisibility_PublicEntity(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{
				ID:         "ent-1",
				IsPrivate:  false,
				Visibility: VisibilityDefault,
			}, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})

	// Player (role 1) can view but not edit public entities.
	perm, err := svc.CheckEntityAccess(context.Background(), "ent-1", 1, "player-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !perm.CanView {
		t.Error("player should be able to view public entity")
	}
	if perm.CanEdit {
		t.Error("player should NOT be able to edit public entity")
	}

	// Scribe (role 2) can view and edit.
	perm, err = svc.CheckEntityAccess(context.Background(), "ent-1", 2, "scribe-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !perm.CanView || !perm.CanEdit {
		t.Errorf("scribe should have full access to public entity, got view=%v edit=%v", perm.CanView, perm.CanEdit)
	}
}

func TestCheckEntityAccess_DefaultVisibility_PrivateEntity(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{
				ID:         "ent-1",
				IsPrivate:  true,
				Visibility: VisibilityDefault,
			}, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})

	// Player (role 1) cannot see private entities.
	perm, err := svc.CheckEntityAccess(context.Background(), "ent-1", 1, "player-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if perm.CanView || perm.CanEdit {
		t.Error("player should NOT have access to private entity")
	}

	// Scribe (role 2) can see private entities.
	perm, err = svc.CheckEntityAccess(context.Background(), "ent-1", 2, "scribe-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !perm.CanView || !perm.CanEdit {
		t.Errorf("scribe should have full access to private entity, got view=%v edit=%v", perm.CanView, perm.CanEdit)
	}
}

func TestCheckEntityAccess_CustomVisibility_DelegatesToRepo(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{
				ID:         "ent-1",
				Visibility: VisibilityCustom,
			}, nil
		},
	}
	permRepo := &mockPermissionRepo{
		getEffectivePermFn: func(_ context.Context, entityID string, role int, userID string) (*EffectivePermission, error) {
			if entityID != "ent-1" {
				t.Errorf("expected entity_id 'ent-1', got %q", entityID)
			}
			if userID != "user-42" {
				t.Errorf("expected user_id 'user-42', got %q", userID)
			}
			return &EffectivePermission{CanView: true, CanEdit: false}, nil
		},
	}

	svc := NewEntityService(entityRepo, &mockEntityTypeRepo{}, permRepo)
	perm, err := svc.CheckEntityAccess(context.Background(), "ent-1", 1, "user-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !perm.CanView {
		t.Error("expected CanView=true from permission repo")
	}
	if perm.CanEdit {
		t.Error("expected CanEdit=false from permission repo")
	}
}

// --- SetEntityPermissions Tests ---

func TestSetEntityPermissions_DefaultMode_ClearsCustom(t *testing.T) {
	deleteCalled := false
	visibilityCalled := false
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{ID: "ent-1", CampaignID: "camp-1", Name: "Test"}, nil
		},
	}
	permRepo := &mockPermissionRepo{
		deleteByEntityFn: func(_ context.Context, entityID string) error {
			deleteCalled = true
			return nil
		},
		updateVisibilityFn: func(_ context.Context, entityID string, vis VisibilityMode) error {
			visibilityCalled = true
			if vis != VisibilityDefault {
				t.Errorf("expected visibility 'default', got %q", vis)
			}
			return nil
		},
	}

	svc := NewEntityService(entityRepo, &mockEntityTypeRepo{}, permRepo)
	err := svc.SetEntityPermissions(context.Background(), "ent-1", SetPermissionsInput{
		Visibility: VisibilityDefault,
		IsPrivate:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleteCalled {
		t.Error("expected DeleteByEntity to be called")
	}
	if !visibilityCalled {
		t.Error("expected UpdateVisibility to be called")
	}
}

func TestSetEntityPermissions_CustomMode_ValidatesGrants(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{ID: "ent-1", CampaignID: "camp-1", Name: "Test"}, nil
		},
	}

	svc := NewEntityService(entityRepo, &mockEntityTypeRepo{}, &mockPermissionRepo{})

	// Invalid subject type.
	err := svc.SetEntityPermissions(context.Background(), "ent-1", SetPermissionsInput{
		Visibility: VisibilityCustom,
		Permissions: []PermissionGrant{
			{SubjectType: "invalid", SubjectID: "1", Permission: PermView},
		},
	})
	assertAppError(t, err, 400)

	// Empty subject ID.
	err = svc.SetEntityPermissions(context.Background(), "ent-1", SetPermissionsInput{
		Visibility: VisibilityCustom,
		Permissions: []PermissionGrant{
			{SubjectType: SubjectRole, SubjectID: "", Permission: PermView},
		},
	})
	assertAppError(t, err, 400)

	// Invalid permission.
	err = svc.SetEntityPermissions(context.Background(), "ent-1", SetPermissionsInput{
		Visibility: VisibilityCustom,
		Permissions: []PermissionGrant{
			{SubjectType: SubjectUser, SubjectID: "user-1", Permission: "delete"},
		},
	})
	assertAppError(t, err, 400)
}

func TestSetEntityPermissions_CustomMode_Success(t *testing.T) {
	setCalled := false
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{ID: "ent-1", CampaignID: "camp-1", Name: "Test"}, nil
		},
	}
	permRepo := &mockPermissionRepo{
		setPermissionsFn: func(_ context.Context, entityID string, grants []PermissionGrant) error {
			setCalled = true
			if len(grants) != 2 {
				t.Errorf("expected 2 grants, got %d", len(grants))
			}
			return nil
		},
		updateVisibilityFn: func(_ context.Context, _ string, vis VisibilityMode) error {
			if vis != VisibilityCustom {
				t.Errorf("expected visibility 'custom', got %q", vis)
			}
			return nil
		},
	}

	svc := NewEntityService(entityRepo, &mockEntityTypeRepo{}, permRepo)
	err := svc.SetEntityPermissions(context.Background(), "ent-1", SetPermissionsInput{
		Visibility: VisibilityCustom,
		Permissions: []PermissionGrant{
			{SubjectType: SubjectRole, SubjectID: "1", Permission: PermView},
			{SubjectType: SubjectUser, SubjectID: "user-42", Permission: PermEdit},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !setCalled {
		t.Error("expected SetPermissions to be called")
	}
}

func TestSetEntityPermissions_InvalidVisibility(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{ID: "ent-1", CampaignID: "camp-1"}, nil
		},
	}

	svc := NewEntityService(entityRepo, &mockEntityTypeRepo{}, &mockPermissionRepo{})
	err := svc.SetEntityPermissions(context.Background(), "ent-1", SetPermissionsInput{
		Visibility: "invalid",
	})
	assertAppError(t, err, 400)
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}

// --- Player Character Experience tests (CH2 + CH3) ---

// TestListByOwner_Forwards confirms the service is a thin wrapper over
// the repository call. The interesting policy choice (no visibility
// filter, ordered by updated_at DESC) lives in the repo SQL — the
// service just delegates and validates the user ID is non-empty.
func TestListByOwner_Forwards(t *testing.T) {
	want := []Entity{{ID: "e1"}, {ID: "e2"}}
	repo := &mockEntityRepo{
		listByOwnerFn: func(_ context.Context, campaignID, ownerUserID string) ([]Entity, error) {
			if campaignID != "camp-1" || ownerUserID != "user-x" {
				t.Errorf("unexpected args: campaign=%s owner=%s", campaignID, ownerUserID)
			}
			return want, nil
		},
	}
	svc := newTestService(repo, &mockEntityTypeRepo{})
	got, err := svc.ListByOwner(context.Background(), "camp-1", "user-x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Errorf("expected %d entities, got %d", len(want), len(got))
	}
}

// TestListByOwner_RejectsEmptyUser guards against a bug-shape where a
// caller passes an empty owner_user_id and the repo returns "all
// entities with NULL owner_user_id" — which would surface unowned
// entities on someone's My Characters page.
func TestListByOwner_RejectsEmptyUser(t *testing.T) {
	svc := newTestService(&mockEntityRepo{}, &mockEntityTypeRepo{})
	_, err := svc.ListByOwner(context.Background(), "camp-1", "")
	assertAppError(t, err, 400)
}

// TestClaimEntity_Success exercises the happy path: an unclaimed
// character entity gets owner_user_id set and the persisted update
// is captured.
func TestClaimEntity_Success(t *testing.T) {
	entity := &Entity{
		ID:           "e-char",
		CampaignID:   "camp-1",
		EntityTypeID: 7,
		OwnerUserID:  nil,
	}
	character := &EntityType{ID: 7, Slug: "drawsteel-character", PresetCategory: strPtr("character")}
	var capturedOwner *string
	repo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, id string) (*Entity, error) { return entity, nil },
		updateOwnerFn: func(_ context.Context, _ string, owner *string) error {
			capturedOwner = owner
			return nil
		},
	}
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, _ int) (*EntityType, error) { return character, nil },
	}
	svc := newTestService(repo, typeRepo)
	updated, err := svc.ClaimEntity(context.Background(), "e-char", "player-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedOwner == nil || *capturedOwner != "player-1" {
		t.Errorf("expected owner_user_id update to player-1, got %v", capturedOwner)
	}
	if updated.OwnerUserID == nil || *updated.OwnerUserID != "player-1" {
		t.Errorf("expected returned entity to reflect new owner, got %v", updated.OwnerUserID)
	}
}

// TestClaimEntity_Idempotent confirms re-claiming an already-owned
// entity returns success without touching the DB. Players who hit
// the claim button twice in quick succession (or HTMX double-submits)
// should not see a 409.
func TestClaimEntity_Idempotent(t *testing.T) {
	entity := &Entity{ID: "e1", CampaignID: "c1", EntityTypeID: 1, OwnerUserID: strPtr("player-1")}
	called := false
	repo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) { return entity, nil },
		updateOwnerFn: func(_ context.Context, _ string, _ *string) error {
			called = true
			return nil
		},
	}
	svc := newTestService(repo, &mockEntityTypeRepo{})
	_, err := svc.ClaimEntity(context.Background(), "e1", "player-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("expected no UpdateOwner call on idempotent claim; got one")
	}
}

// TestClaimEntity_RejectsAlreadyClaimed: when entity is owned by a
// different player, the service returns 409 Conflict so the caller
// knows to ask the GM rather than silently steal ownership.
func TestClaimEntity_RejectsAlreadyClaimed(t *testing.T) {
	entity := &Entity{ID: "e1", CampaignID: "c1", EntityTypeID: 1, OwnerUserID: strPtr("player-other")}
	repo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) { return entity, nil },
	}
	svc := newTestService(repo, &mockEntityTypeRepo{})
	_, err := svc.ClaimEntity(context.Background(), "e1", "player-mine")
	assertAppError(t, err, 409)
}

// TestClaimEntity_RejectsNonCharacter prevents claiming a Location or
// Faction by entity_type slug. The handler-level route is Player+ for
// flexibility; the type-shape gate is what stops misuse.
func TestClaimEntity_RejectsNonCharacter(t *testing.T) {
	entity := &Entity{ID: "e1", CampaignID: "c1", EntityTypeID: 99, OwnerUserID: nil}
	location := &EntityType{ID: 99, Slug: "location"}
	repo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) { return entity, nil },
	}
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, _ int) (*EntityType, error) { return location, nil },
	}
	svc := newTestService(repo, typeRepo)
	_, err := svc.ClaimEntity(context.Background(), "e1", "player-1")
	assertAppError(t, err, 400)
}

// TestIsClaimableType pins the heuristic. Two paths to "claimable":
// preset_category=="character", or slug ends in "-character" / equals
// "character". Anything else is not claimable.
func TestIsClaimableType(t *testing.T) {
	cases := []struct {
		name string
		et   *EntityType
		want bool
	}{
		{"nil", nil, false},
		{"location is not", &EntityType{Slug: "location"}, false},
		{"preset character", &EntityType{Slug: "anything", PresetCategory: strPtr("character")}, true},
		{"slug character", &EntityType{Slug: "character"}, true},
		{"slug suffix dnd5e-character", &EntityType{Slug: "dnd5e-character"}, true},
		{"slug suffix drawsteel-character", &EntityType{Slug: "drawsteel-character"}, true},
		{"random suffix", &EntityType{Slug: "shopkeeper"}, false},
	}
	for _, tc := range cases {
		if got := isClaimableType(tc.et); got != tc.want {
			t.Errorf("%s: isClaimableType=%v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestAssignOwner_SetAndClear: Owner/Scribe can both set a new owner
// and unassign by passing nil. Idempotent if the value matches current.
func TestAssignOwner_SetAndClear(t *testing.T) {
	for _, tc := range []struct {
		name        string
		current     *string
		newOwner    *string
		wantUpdated bool
	}{
		{"unowned -> owned", nil, strPtr("p1"), true},
		{"owned -> different owner", strPtr("p1"), strPtr("p2"), true},
		{"owned -> unowned", strPtr("p1"), nil, true},
		{"unowned -> unowned (no-op)", nil, nil, false},
		{"same owner (no-op)", strPtr("p1"), strPtr("p1"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ent := &Entity{ID: "e1", CampaignID: "c1", OwnerUserID: tc.current}
			called := false
			repo := &mockEntityRepo{
				findByIDFn: func(_ context.Context, _ string) (*Entity, error) { return ent, nil },
				updateOwnerFn: func(_ context.Context, _ string, _ *string) error {
					called = true
					return nil
				},
			}
			svc := newTestService(repo, &mockEntityTypeRepo{})
			_, err := svc.AssignOwner(context.Background(), "e1", tc.newOwner)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if called != tc.wantUpdated {
				t.Errorf("UpdateOwner called=%v, want %v", called, tc.wantUpdated)
			}
		})
	}
}

// stubMapVerifier is a one-line MapCampaignVerifier for tests. Returning
// (false, nil) is the "default reject" behavior; tests that exercise the
// allow path replace .ok with true.
type stubMapVerifier struct {
	ok      bool
	err     error
	gotMap  string
	gotCamp string
}

func (s *stubMapVerifier) MapExistsInCampaign(_ context.Context, mapID, campaignID string) (bool, error) {
	s.gotMap = mapID
	s.gotCamp = campaignID
	return s.ok, s.err
}

// TestAssignMap_SetClearAndNoOp pins the same shape AssignOwner has —
// transitions write, no-op transitions don't, and the return reflects
// the new state.
func TestAssignMap_SetClearAndNoOp(t *testing.T) {
	for _, tc := range []struct {
		name        string
		current     *string
		newMap      *string
		wantUpdated bool
	}{
		{"unassigned -> assigned", nil, strPtr("m1"), true},
		{"assigned -> different map", strPtr("m1"), strPtr("m2"), true},
		{"assigned -> unassigned", strPtr("m1"), nil, true},
		{"unassigned -> unassigned (no-op)", nil, nil, false},
		{"same map (no-op)", strPtr("m1"), strPtr("m1"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ent := &Entity{ID: "e1", CampaignID: "c1", MapID: tc.current}
			called := false
			repo := &mockEntityRepo{
				findByIDFn: func(_ context.Context, _ string) (*Entity, error) { return ent, nil },
				updateMapIDFn: func(_ context.Context, _ string, _ *string) error {
					called = true
					return nil
				},
			}
			svc := newTestService(repo, &mockEntityTypeRepo{})
			svc.SetMapVerifier(&stubMapVerifier{ok: true})

			updated, err := svc.AssignMap(context.Background(), "e1", tc.newMap)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if called != tc.wantUpdated {
				t.Errorf("UpdateMapID called=%v, want %v", called, tc.wantUpdated)
			}
			// On a real assignment, the returned entity should reflect the new value.
			if tc.wantUpdated && tc.newMap != nil {
				if updated.MapID == nil || *updated.MapID != *tc.newMap {
					t.Errorf("returned MapID = %v, want %v", updated.MapID, *tc.newMap)
				}
			}
		})
	}
}

// TestAssignMap_RejectsCrossCampaign is the IDOR test: a map from
// another campaign must not be assignable, even if the FK alone would
// succeed (FK enforces existence, not campaign scoping).
func TestAssignMap_RejectsCrossCampaign(t *testing.T) {
	ent := &Entity{ID: "e1", CampaignID: "campA"}
	updateCalled := false
	repo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) { return ent, nil },
		updateMapIDFn: func(_ context.Context, _ string, _ *string) error {
			updateCalled = true
			return nil
		},
	}
	svc := newTestService(repo, &mockEntityTypeRepo{})
	verifier := &stubMapVerifier{ok: false} // map doesn't exist in this campaign
	svc.SetMapVerifier(verifier)

	_, err := svc.AssignMap(context.Background(), "e1", strPtr("m-from-campB"))
	if err == nil {
		t.Fatal("expected NotFound error for cross-campaign map")
	}
	if updateCalled {
		t.Error("UpdateMapID was called despite verifier rejecting the assignment")
	}
	if verifier.gotMap != "m-from-campB" || verifier.gotCamp != "campA" {
		t.Errorf("verifier called with wrong args: gotMap=%q gotCamp=%q", verifier.gotMap, verifier.gotCamp)
	}
}

// TestAssignMap_VerifierErrorBubbles pins that real DB errors from the
// verifier propagate as 500-shape errors instead of being swallowed
// into "not found" — operators need to see the underlying failure.
func TestAssignMap_VerifierErrorBubbles(t *testing.T) {
	ent := &Entity{ID: "e1", CampaignID: "c1"}
	repo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) { return ent, nil },
	}
	svc := newTestService(repo, &mockEntityTypeRepo{})
	svc.SetMapVerifier(&stubMapVerifier{err: errors.New("db down")})

	_, err := svc.AssignMap(context.Background(), "e1", strPtr("m1"))
	if err == nil {
		t.Fatal("expected verifier error to bubble up")
	}
}

// TestAssignMap_NoVerifierWiredRejectsAll pins the safe default: the
// noopMapVerifier returns false for everything, so unwired callers
// can't sneak past the IDOR check.
func TestAssignMap_NoVerifierWiredRejectsAll(t *testing.T) {
	ent := &Entity{ID: "e1", CampaignID: "c1"}
	repo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) { return ent, nil },
	}
	svc := newTestService(repo, &mockEntityTypeRepo{})
	// Intentionally do NOT call SetMapVerifier — the constructor seeds noopMapVerifier.

	_, err := svc.AssignMap(context.Background(), "e1", strPtr("m1"))
	if err == nil {
		t.Fatal("expected default verifier to reject all map IDs")
	}
}
