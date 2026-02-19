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

func (m *mockEntityTypeRepo) UpdateLayout(ctx context.Context, id int, layoutJSON string) error {
	if m.updateLayoutFn != nil {
		return m.updateLayoutFn(ctx, id, layoutJSON)
	}
	return nil
}

func (m *mockEntityTypeRepo) SeedDefaults(ctx context.Context, campaignID string) error {
	if m.seedDefaultsFn != nil {
		return m.seedDefaultsFn(ctx, campaignID)
	}
	return nil
}

// mockEntityRepo implements EntityRepository for testing.
type mockEntityRepo struct {
	createFn        func(ctx context.Context, entity *Entity) error
	findByIDFn      func(ctx context.Context, id string) (*Entity, error)
	findBySlugFn    func(ctx context.Context, campaignID, slug string) (*Entity, error)
	updateFn        func(ctx context.Context, entity *Entity) error
	updateEntryFn   func(ctx context.Context, id, entryJSON, entryHTML string) error
	updateImageFn   func(ctx context.Context, id, imagePath string) error
	deleteFn        func(ctx context.Context, id string) error
	slugExistsFn    func(ctx context.Context, campaignID, slug string) (bool, error)
	listByCampaignFn func(ctx context.Context, campaignID string, typeID int, role int, opts ListOptions) ([]Entity, int, error)
	searchFn        func(ctx context.Context, campaignID, query string, typeID int, role int, opts ListOptions) ([]Entity, int, error)
	countByTypeFn   func(ctx context.Context, campaignID string, role int) (map[int]int, error)
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

func (m *mockEntityRepo) UpdateEntry(ctx context.Context, id, entryJSON, entryHTML string) error {
	if m.updateEntryFn != nil {
		return m.updateEntryFn(ctx, id, entryJSON, entryHTML)
	}
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

func (m *mockEntityRepo) ListByCampaign(ctx context.Context, campaignID string, typeID int, role int, opts ListOptions) ([]Entity, int, error) {
	if m.listByCampaignFn != nil {
		return m.listByCampaignFn(ctx, campaignID, typeID, role, opts)
	}
	return nil, 0, nil
}

func (m *mockEntityRepo) Search(ctx context.Context, campaignID, query string, typeID int, role int, opts ListOptions) ([]Entity, int, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, campaignID, query, typeID, role, opts)
	}
	return nil, 0, nil
}

func (m *mockEntityRepo) CountByType(ctx context.Context, campaignID string, role int) (map[int]int, error) {
	if m.countByTypeFn != nil {
		return m.countByTypeFn(ctx, campaignID, role)
	}
	return nil, nil
}

// --- Test Helpers ---

func newTestService(entityRepo *mockEntityRepo, typeRepo *mockEntityTypeRepo) EntityService {
	return NewEntityService(entityRepo, typeRepo)
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
		listByCampaignFn: func(_ context.Context, _ string, _ int, _ int, opts ListOptions) ([]Entity, int, error) {
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
	_, _, err := svc.List(context.Background(), "camp-1", 0, 1, ListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected repository to be called")
	}
}

func TestList_ClampsPerPage(t *testing.T) {
	entityRepo := &mockEntityRepo{
		listByCampaignFn: func(_ context.Context, _ string, _ int, _ int, opts ListOptions) ([]Entity, int, error) {
			if opts.PerPage != 24 {
				t.Errorf("expected clamped per_page 24, got %d", opts.PerPage)
			}
			return nil, 0, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	_, _, _ = svc.List(context.Background(), "camp-1", 0, 1, ListOptions{PerPage: 500})
}

// --- Search Tests ---

func TestSearch_MinQueryLength(t *testing.T) {
	svc := newTestService(&mockEntityRepo{}, &mockEntityTypeRepo{})
	_, _, err := svc.Search(context.Background(), "camp-1", "a", 0, 1, DefaultListOptions())
	assertAppError(t, err, 400)
}

func TestSearch_TrimsQuery(t *testing.T) {
	svc := newTestService(&mockEntityRepo{}, &mockEntityTypeRepo{})
	_, _, err := svc.Search(context.Background(), "camp-1", "  a  ", 0, 1, DefaultListOptions())
	assertAppError(t, err, 400) // "a" is only 1 char after trim
}

func TestSearch_ValidQuery(t *testing.T) {
	entityRepo := &mockEntityRepo{
		searchFn: func(_ context.Context, _ string, query string, _ int, _ int, _ ListOptions) ([]Entity, int, error) {
			if query != "gandalf" {
				t.Errorf("expected trimmed query 'gandalf', got %q", query)
			}
			return []Entity{{Name: "Gandalf"}}, 1, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	results, total, err := svc.Search(context.Background(), "camp-1", "  gandalf  ", 0, 1, DefaultListOptions())
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
