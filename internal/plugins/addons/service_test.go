package addons

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repository ---

// mockAddonRepo implements AddonRepository for testing.
type mockAddonRepo struct {
	countFn               func(ctx context.Context) (int, error)
	listFn                func(ctx context.Context) ([]Addon, error)
	findByIDFn            func(ctx context.Context, id int) (*Addon, error)
	findBySlugFn          func(ctx context.Context, slug string) (*Addon, error)
	createFn              func(ctx context.Context, addon *Addon) error
	updateFn              func(ctx context.Context, addon *Addon) error
	deleteFn              func(ctx context.Context, id int) error
	updateStatusFn        func(ctx context.Context, id int, status AddonStatus) error
	listForCampaignFn     func(ctx context.Context, campaignID string) ([]CampaignAddon, error)
	enableForCampaignFn   func(ctx context.Context, campaignID string, addonID int, userID string) error
	disableForCampaignFn  func(ctx context.Context, campaignID string, addonID int) error
	isEnabledFn           func(ctx context.Context, campaignID string, addonSlug string) (bool, error)
	updateCampaignCfgFn   func(ctx context.Context, campaignID string, addonID int, config map[string]any) error
}

func (m *mockAddonRepo) Count(ctx context.Context) (int, error) {
	if m.countFn != nil {
		return m.countFn(ctx)
	}
	return 0, nil
}

func (m *mockAddonRepo) List(ctx context.Context) ([]Addon, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *mockAddonRepo) FindByID(ctx context.Context, id int) (*Addon, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, apperror.NewNotFound("addon not found")
}

func (m *mockAddonRepo) FindBySlug(ctx context.Context, slug string) (*Addon, error) {
	if m.findBySlugFn != nil {
		return m.findBySlugFn(ctx, slug)
	}
	return nil, apperror.NewNotFound("addon not found")
}

func (m *mockAddonRepo) Create(ctx context.Context, addon *Addon) error {
	if m.createFn != nil {
		return m.createFn(ctx, addon)
	}
	addon.ID = 1
	return nil
}

func (m *mockAddonRepo) Update(ctx context.Context, addon *Addon) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, addon)
	}
	return nil
}

func (m *mockAddonRepo) Delete(ctx context.Context, id int) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockAddonRepo) UpdateStatus(ctx context.Context, id int, status AddonStatus) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, status)
	}
	return nil
}

func (m *mockAddonRepo) ListForCampaign(ctx context.Context, campaignID string) ([]CampaignAddon, error) {
	if m.listForCampaignFn != nil {
		return m.listForCampaignFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockAddonRepo) EnableForCampaign(ctx context.Context, campaignID string, addonID int, userID string) error {
	if m.enableForCampaignFn != nil {
		return m.enableForCampaignFn(ctx, campaignID, addonID, userID)
	}
	return nil
}

func (m *mockAddonRepo) DisableForCampaign(ctx context.Context, campaignID string, addonID int) error {
	if m.disableForCampaignFn != nil {
		return m.disableForCampaignFn(ctx, campaignID, addonID)
	}
	return nil
}

func (m *mockAddonRepo) IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error) {
	if m.isEnabledFn != nil {
		return m.isEnabledFn(ctx, campaignID, addonSlug)
	}
	return false, nil
}

func (m *mockAddonRepo) UpdateCampaignConfig(ctx context.Context, campaignID string, addonID int, config map[string]any) error {
	if m.updateCampaignCfgFn != nil {
		return m.updateCampaignCfgFn(ctx, campaignID, addonID, config)
	}
	return nil
}

// --- Test Helpers ---

// assertAppError checks that err is an *apperror.AppError with the expected code.
func assertAppError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %d, got nil", expectedCode)
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperror.AppError, got %T: %v", err, err)
	}
	if appErr.Code != expectedCode {
		t.Errorf("expected status %d, got %d (message: %s)", expectedCode, appErr.Code, appErr.Message)
	}
}

// --- Create Tests ---

func TestCreate_Success(t *testing.T) {
	repo := &mockAddonRepo{
		findBySlugFn: func(ctx context.Context, slug string) (*Addon, error) {
			return nil, apperror.NewNotFound("not found") // Slug is available.
		},
		createFn: func(ctx context.Context, addon *Addon) error {
			if addon.Slug != "dice-roller" {
				t.Errorf("expected slug dice-roller, got %s", addon.Slug)
			}
			if addon.Name != "Dice Roller" {
				t.Errorf("expected name Dice Roller, got %s", addon.Name)
			}
			if addon.Category != CategoryWidget {
				t.Errorf("expected category widget, got %s", addon.Category)
			}
			if addon.Status != StatusPlanned {
				t.Errorf("expected status planned (default), got %s", addon.Status)
			}
			addon.ID = 42
			return nil
		},
	}

	svc := NewAddonService(repo)
	addon, err := svc.Create(context.Background(), CreateAddonInput{
		Slug:        "dice-roller",
		Name:        "Dice Roller",
		Description: "Roll dice in campaigns",
		Version:     "1.0.0",
		Category:    CategoryWidget,
		Icon:        "fa-dice",
		Author:      "Chronicle",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addon.ID != 42 {
		t.Errorf("expected ID 42, got %d", addon.ID)
	}
}

func TestCreate_EmptySlug(t *testing.T) {
	svc := NewAddonService(&mockAddonRepo{})
	_, err := svc.Create(context.Background(), CreateAddonInput{
		Slug:     "",
		Name:     "Test",
		Category: CategoryModule,
	})
	assertAppError(t, err, 400)
}

func TestCreate_EmptyName(t *testing.T) {
	svc := NewAddonService(&mockAddonRepo{})
	_, err := svc.Create(context.Background(), CreateAddonInput{
		Slug:     "test",
		Name:     "",
		Category: CategoryModule,
	})
	assertAppError(t, err, 400)
}

func TestCreate_InvalidCategory(t *testing.T) {
	svc := NewAddonService(&mockAddonRepo{})
	_, err := svc.Create(context.Background(), CreateAddonInput{
		Slug:     "test",
		Name:     "Test",
		Category: AddonCategory("invalid"),
	})
	assertAppError(t, err, 400)
}

func TestCreate_DuplicateSlug(t *testing.T) {
	repo := &mockAddonRepo{
		findBySlugFn: func(ctx context.Context, slug string) (*Addon, error) {
			return &Addon{ID: 1, Slug: slug}, nil // Slug already exists.
		},
	}

	svc := NewAddonService(repo)
	_, err := svc.Create(context.Background(), CreateAddonInput{
		Slug:     "existing",
		Name:     "Test",
		Category: CategoryModule,
	})
	assertAppError(t, err, 409)
}

func TestCreate_RepoError(t *testing.T) {
	repo := &mockAddonRepo{
		findBySlugFn: func(ctx context.Context, slug string) (*Addon, error) {
			return nil, apperror.NewNotFound("not found")
		},
		createFn: func(ctx context.Context, addon *Addon) error {
			return errors.New("db error")
		},
	}

	svc := NewAddonService(repo)
	_, err := svc.Create(context.Background(), CreateAddonInput{
		Slug:     "test",
		Name:     "Test",
		Category: CategoryModule,
	})
	assertAppError(t, err, 500)
}

func TestCreate_InputTrimming(t *testing.T) {
	var capturedAddon *Addon
	repo := &mockAddonRepo{
		findBySlugFn: func(ctx context.Context, slug string) (*Addon, error) {
			return nil, apperror.NewNotFound("not found")
		},
		createFn: func(ctx context.Context, addon *Addon) error {
			capturedAddon = addon
			addon.ID = 1
			return nil
		},
	}

	svc := NewAddonService(repo)
	_, err := svc.Create(context.Background(), CreateAddonInput{
		Slug:        "  my-addon  ",
		Name:        "  My Addon  ",
		Description: "  A description  ",
		Version:     "  1.0  ",
		Category:    CategoryWidget,
		Icon:        "  fa-star  ",
		Author:      "  Author  ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedAddon.Slug != "my-addon" {
		t.Errorf("expected trimmed slug, got %q", capturedAddon.Slug)
	}
	if capturedAddon.Name != "My Addon" {
		t.Errorf("expected trimmed name, got %q", capturedAddon.Name)
	}
	if capturedAddon.Version != "1.0" {
		t.Errorf("expected trimmed version, got %q", capturedAddon.Version)
	}
	if capturedAddon.Description == nil || *capturedAddon.Description != "A description" {
		t.Errorf("expected trimmed description, got %v", capturedAddon.Description)
	}
	if capturedAddon.Author == nil || *capturedAddon.Author != "Author" {
		t.Errorf("expected trimmed author, got %v", capturedAddon.Author)
	}
}

func TestCreate_EmptyDescription(t *testing.T) {
	var capturedAddon *Addon
	repo := &mockAddonRepo{
		findBySlugFn: func(ctx context.Context, slug string) (*Addon, error) {
			return nil, apperror.NewNotFound("not found")
		},
		createFn: func(ctx context.Context, addon *Addon) error {
			capturedAddon = addon
			addon.ID = 1
			return nil
		},
	}

	svc := NewAddonService(repo)
	_, err := svc.Create(context.Background(), CreateAddonInput{
		Slug:        "test",
		Name:        "Test",
		Description: "   ", // Whitespace-only should become nil.
		Category:    CategoryModule,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedAddon.Description != nil {
		t.Errorf("expected nil description for whitespace input, got %q", *capturedAddon.Description)
	}
}

// --- Update Tests ---

func TestUpdate_Success(t *testing.T) {
	existing := &Addon{ID: 1, Slug: "test", Name: "Old", Status: StatusActive, Category: CategoryModule}
	repo := &mockAddonRepo{
		findByIDFn: func(ctx context.Context, id int) (*Addon, error) {
			return existing, nil
		},
	}

	svc := NewAddonService(repo)
	addon, err := svc.Update(context.Background(), 1, UpdateAddonInput{
		Name:        "New Name",
		Description: "New desc",
		Version:     "2.0",
		Status:      StatusActive,
		Icon:        "fa-cog",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addon.Name != "New Name" {
		t.Errorf("expected name New Name, got %s", addon.Name)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	repo := &mockAddonRepo{
		findByIDFn: func(ctx context.Context, id int) (*Addon, error) {
			return nil, apperror.NewNotFound("not found")
		},
	}

	svc := NewAddonService(repo)
	_, err := svc.Update(context.Background(), 999, UpdateAddonInput{
		Name:   "Test",
		Status: StatusActive,
	})
	assertAppError(t, err, 404)
}

func TestUpdate_EmptyName(t *testing.T) {
	repo := &mockAddonRepo{
		findByIDFn: func(ctx context.Context, id int) (*Addon, error) {
			return &Addon{ID: 1, Slug: "test"}, nil
		},
	}

	svc := NewAddonService(repo)
	_, err := svc.Update(context.Background(), 1, UpdateAddonInput{
		Name:   "",
		Status: StatusActive,
	})
	assertAppError(t, err, 400)
}

func TestUpdate_InvalidStatus(t *testing.T) {
	repo := &mockAddonRepo{
		findByIDFn: func(ctx context.Context, id int) (*Addon, error) {
			return &Addon{ID: 1, Slug: "test"}, nil
		},
	}

	svc := NewAddonService(repo)
	_, err := svc.Update(context.Background(), 1, UpdateAddonInput{
		Name:   "Test",
		Status: AddonStatus("invalid"),
	})
	assertAppError(t, err, 400)
}

func TestUpdate_ClearsDescription(t *testing.T) {
	desc := "old"
	existing := &Addon{ID: 1, Slug: "test", Description: &desc}
	var updatedAddon *Addon
	repo := &mockAddonRepo{
		findByIDFn: func(ctx context.Context, id int) (*Addon, error) {
			return existing, nil
		},
		updateFn: func(ctx context.Context, addon *Addon) error {
			updatedAddon = addon
			return nil
		},
	}

	svc := NewAddonService(repo)
	_, err := svc.Update(context.Background(), 1, UpdateAddonInput{
		Name:        "Test",
		Description: "  ", // Whitespace-only should nil out.
		Status:      StatusActive,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updatedAddon.Description != nil {
		t.Errorf("expected nil description, got %q", *updatedAddon.Description)
	}
}

// --- UpdateStatus Tests ---

func TestUpdateStatus_Success(t *testing.T) {
	var capturedStatus AddonStatus
	repo := &mockAddonRepo{
		updateStatusFn: func(ctx context.Context, id int, status AddonStatus) error {
			capturedStatus = status
			return nil
		},
	}

	svc := NewAddonService(repo)
	err := svc.UpdateStatus(context.Background(), 1, StatusDeprecated)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedStatus != StatusDeprecated {
		t.Errorf("expected deprecated, got %s", capturedStatus)
	}
}

func TestUpdateStatus_InvalidStatus(t *testing.T) {
	svc := NewAddonService(&mockAddonRepo{})
	err := svc.UpdateStatus(context.Background(), 1, AddonStatus("bogus"))
	assertAppError(t, err, 400)
}

// --- Delete Tests ---

func TestDelete_Success(t *testing.T) {
	var deletedID int
	repo := &mockAddonRepo{
		deleteFn: func(ctx context.Context, id int) error {
			deletedID = id
			return nil
		},
	}

	svc := NewAddonService(repo)
	err := svc.Delete(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletedID != 42 {
		t.Errorf("expected ID 42, got %d", deletedID)
	}
}

func TestDelete_NotFound(t *testing.T) {
	repo := &mockAddonRepo{
		deleteFn: func(ctx context.Context, id int) error {
			return apperror.NewNotFound("not found")
		},
	}

	svc := NewAddonService(repo)
	err := svc.Delete(context.Background(), 999)
	assertAppError(t, err, 404)
}

// --- EnableForCampaign Tests ---

func TestEnableForCampaign_Success(t *testing.T) {
	var capturedCampaignID string
	var capturedAddonID int
	repo := &mockAddonRepo{
		findByIDFn: func(ctx context.Context, id int) (*Addon, error) {
			return &Addon{ID: id, Slug: "notes", Status: StatusActive}, nil
		},
		enableForCampaignFn: func(ctx context.Context, campaignID string, addonID int, userID string) error {
			capturedCampaignID = campaignID
			capturedAddonID = addonID
			return nil
		},
	}

	svc := NewAddonService(repo)
	err := svc.EnableForCampaign(context.Background(), "camp-1", 5, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedCampaignID != "camp-1" {
		t.Errorf("expected camp-1, got %s", capturedCampaignID)
	}
	if capturedAddonID != 5 {
		t.Errorf("expected addon ID 5, got %d", capturedAddonID)
	}
}

func TestEnableForCampaign_NotActive(t *testing.T) {
	repo := &mockAddonRepo{
		findByIDFn: func(ctx context.Context, id int) (*Addon, error) {
			return &Addon{ID: id, Status: StatusPlanned}, nil
		},
	}

	svc := NewAddonService(repo)
	err := svc.EnableForCampaign(context.Background(), "camp-1", 5, "user-1")
	assertAppError(t, err, 400)
}

func TestEnableForCampaign_DeprecatedAddon(t *testing.T) {
	repo := &mockAddonRepo{
		findByIDFn: func(ctx context.Context, id int) (*Addon, error) {
			return &Addon{ID: id, Status: StatusDeprecated}, nil
		},
	}

	svc := NewAddonService(repo)
	err := svc.EnableForCampaign(context.Background(), "camp-1", 5, "user-1")
	assertAppError(t, err, 400)
}

func TestEnableForCampaign_AddonNotFound(t *testing.T) {
	repo := &mockAddonRepo{
		findByIDFn: func(ctx context.Context, id int) (*Addon, error) {
			return nil, apperror.NewNotFound("not found")
		},
	}

	svc := NewAddonService(repo)
	err := svc.EnableForCampaign(context.Background(), "camp-1", 999, "user-1")
	assertAppError(t, err, 404)
}

// --- DisableForCampaign Tests ---

func TestDisableForCampaign_Success(t *testing.T) {
	var called bool
	repo := &mockAddonRepo{
		disableForCampaignFn: func(ctx context.Context, campaignID string, addonID int) error {
			called = true
			return nil
		},
	}

	svc := NewAddonService(repo)
	err := svc.DisableForCampaign(context.Background(), "camp-1", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected repo.DisableForCampaign to be called")
	}
}

// --- List Tests ---

func TestList_Success(t *testing.T) {
	repo := &mockAddonRepo{
		listFn: func(ctx context.Context) ([]Addon, error) {
			return []Addon{
				{ID: 1, Slug: "notes", Name: "Player Notes"},
				{ID: 2, Slug: "dice", Name: "Dice Roller"},
			}, nil
		},
	}

	svc := NewAddonService(repo)
	addons, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(addons) != 2 {
		t.Errorf("expected 2 addons, got %d", len(addons))
	}
}

func TestList_RepoError(t *testing.T) {
	repo := &mockAddonRepo{
		listFn: func(ctx context.Context) ([]Addon, error) {
			return nil, errors.New("db error")
		},
	}

	svc := NewAddonService(repo)
	_, err := svc.List(context.Background())
	assertAppError(t, err, 500)
}

// --- GetByID Tests ---

func TestGetByID_Success(t *testing.T) {
	repo := &mockAddonRepo{
		findByIDFn: func(ctx context.Context, id int) (*Addon, error) {
			return &Addon{ID: id, Slug: "test", Name: "Test"}, nil
		},
	}

	svc := NewAddonService(repo)
	addon, err := svc.GetByID(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addon.ID != 42 {
		t.Errorf("expected ID 42, got %d", addon.ID)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	svc := NewAddonService(&mockAddonRepo{})
	_, err := svc.GetByID(context.Background(), 999)
	assertAppError(t, err, 404)
}

// --- IsEnabledForCampaign Tests ---

func TestIsEnabledForCampaign(t *testing.T) {
	repo := &mockAddonRepo{
		isEnabledFn: func(ctx context.Context, campaignID, slug string) (bool, error) {
			return campaignID == "camp-1" && slug == "player-notes", nil
		},
	}

	svc := NewAddonService(repo)

	enabled, err := svc.IsEnabledForCampaign(context.Background(), "camp-1", "player-notes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Error("expected addon to be enabled")
	}

	enabled, err = svc.IsEnabledForCampaign(context.Background(), "camp-2", "player-notes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enabled {
		t.Error("expected addon to be disabled for different campaign")
	}
}

// --- CountAddons Tests ---

func TestCountAddons(t *testing.T) {
	repo := &mockAddonRepo{
		countFn: func(ctx context.Context) (int, error) {
			return 11, nil
		},
	}

	svc := NewAddonService(repo)
	count, err := svc.CountAddons(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 11 {
		t.Errorf("expected 11, got %d", count)
	}
}
