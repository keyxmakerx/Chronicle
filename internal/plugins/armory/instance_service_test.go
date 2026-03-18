package armory

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock ---

type mockInstanceRepo struct {
	createFn          func(ctx context.Context, campaignID, name, slug, desc, icon, color string) (*InventoryInstance, error)
	findByIDFn        func(ctx context.Context, id int) (*InventoryInstance, error)
	listByCampaignFn  func(ctx context.Context, campaignID string) ([]InventoryInstance, error)
	updateFn          func(ctx context.Context, id int, name, slug, desc, icon, color string) error
	deleteFn          func(ctx context.Context, id int) error
	addItemFn         func(ctx context.Context, instanceID int, entityID string, quantity int) error
	removeItemFn      func(ctx context.Context, instanceID int, entityID string) error
	countItemsFn      func(ctx context.Context, instanceID int) (int, error)
}

func (m *mockInstanceRepo) Create(ctx context.Context, campaignID, name, slug, desc, icon, color string) (*InventoryInstance, error) {
	if m.createFn != nil {
		return m.createFn(ctx, campaignID, name, slug, desc, icon, color)
	}
	return &InventoryInstance{ID: 1, CampaignID: campaignID, Name: name, Slug: slug}, nil
}

func (m *mockInstanceRepo) FindByID(ctx context.Context, id int) (*InventoryInstance, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockInstanceRepo) ListByCampaign(ctx context.Context, campaignID string) ([]InventoryInstance, error) {
	if m.listByCampaignFn != nil {
		return m.listByCampaignFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockInstanceRepo) Update(ctx context.Context, id int, name, slug, desc, icon, color string) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, name, slug, desc, icon, color)
	}
	return nil
}

func (m *mockInstanceRepo) Delete(ctx context.Context, id int) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockInstanceRepo) AddItem(ctx context.Context, instanceID int, entityID string, quantity int) error {
	if m.addItemFn != nil {
		return m.addItemFn(ctx, instanceID, entityID, quantity)
	}
	return nil
}

func (m *mockInstanceRepo) RemoveItem(ctx context.Context, instanceID int, entityID string) error {
	if m.removeItemFn != nil {
		return m.removeItemFn(ctx, instanceID, entityID)
	}
	return nil
}

func (m *mockInstanceRepo) CountInstanceItems(ctx context.Context, instanceID int) (int, error) {
	if m.countItemsFn != nil {
		return m.countItemsFn(ctx, instanceID)
	}
	return 0, nil
}

func newTestInstanceService(repo *mockInstanceRepo) *instanceService {
	return &instanceService{repo: repo}
}

func isAppError(err error) bool {
	var appErr *apperror.AppError
	return errors.As(err, &appErr)
}

// --- Tests ---

func TestCreateInstance_Success(t *testing.T) {
	var capturedName, capturedSlug, capturedIcon, capturedColor string
	repo := &mockInstanceRepo{
		createFn: func(_ context.Context, _, name, slug, _, icon, color string) (*InventoryInstance, error) {
			capturedName = name
			capturedSlug = slug
			capturedIcon = icon
			capturedColor = color
			return &InventoryInstance{ID: 1, Name: name, Slug: slug, Icon: icon, Color: color}, nil
		},
	}
	svc := newTestInstanceService(repo)
	inst, err := svc.CreateInstance(context.Background(), "camp-1", CreateInstanceInput{
		Name: "Party Loot",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedName != "Party Loot" {
		t.Errorf("name = %q, want %q", capturedName, "Party Loot")
	}
	if capturedSlug != "party-loot" {
		t.Errorf("slug = %q, want %q", capturedSlug, "party-loot")
	}
	if capturedIcon != "fa-box" {
		t.Errorf("icon = %q, want default %q", capturedIcon, "fa-box")
	}
	if capturedColor != "#6b7280" {
		t.Errorf("color = %q, want default %q", capturedColor, "#6b7280")
	}
	if inst.ID != 1 {
		t.Errorf("ID = %d, want 1", inst.ID)
	}
}

func TestCreateInstance_EmptyName(t *testing.T) {
	svc := newTestInstanceService(&mockInstanceRepo{})
	_, err := svc.CreateInstance(context.Background(), "camp-1", CreateInstanceInput{Name: ""})
	if err == nil || !isAppError(err) {
		t.Error("expected validation error for empty name")
	}
}

func TestCreateInstance_WhitespaceName(t *testing.T) {
	svc := newTestInstanceService(&mockInstanceRepo{})
	_, err := svc.CreateInstance(context.Background(), "camp-1", CreateInstanceInput{Name: "   "})
	if err == nil {
		t.Error("expected error for whitespace-only name")
	}
}

func TestCreateInstance_LongName(t *testing.T) {
	svc := newTestInstanceService(&mockInstanceRepo{})
	longName := ""
	for i := 0; i < 101; i++ {
		longName += "a"
	}
	_, err := svc.CreateInstance(context.Background(), "camp-1", CreateInstanceInput{Name: longName})
	if err == nil || !isAppError(err) {
		t.Error("expected validation error for name > 100 chars")
	}
}

func TestCreateInstance_CustomIconAndColor(t *testing.T) {
	var capturedIcon, capturedColor string
	repo := &mockInstanceRepo{
		createFn: func(_ context.Context, _, _, _, _, icon, color string) (*InventoryInstance, error) {
			capturedIcon = icon
			capturedColor = color
			return &InventoryInstance{ID: 1}, nil
		},
	}
	svc := newTestInstanceService(repo)
	_, err := svc.CreateInstance(context.Background(), "camp-1", CreateInstanceInput{
		Name: "Vault", Icon: "fa-vault", Color: "#ff0000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedIcon != "fa-vault" {
		t.Errorf("icon = %q, want %q", capturedIcon, "fa-vault")
	}
	if capturedColor != "#ff0000" {
		t.Errorf("color = %q, want %q", capturedColor, "#ff0000")
	}
}

func TestGetInstance_Success(t *testing.T) {
	repo := &mockInstanceRepo{
		findByIDFn: func(_ context.Context, id int) (*InventoryInstance, error) {
			return &InventoryInstance{ID: id, CampaignID: "camp-1"}, nil
		},
	}
	svc := newTestInstanceService(repo)
	inst, err := svc.GetInstance(context.Background(), "camp-1", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.ID != 1 {
		t.Errorf("ID = %d, want 1", inst.ID)
	}
}

func TestGetInstance_NotFound(t *testing.T) {
	repo := &mockInstanceRepo{
		findByIDFn: func(_ context.Context, _ int) (*InventoryInstance, error) {
			return nil, nil
		},
	}
	svc := newTestInstanceService(repo)
	_, err := svc.GetInstance(context.Background(), "camp-1", 99)
	if err == nil || !isAppError(err) {
		t.Error("expected not found error")
	}
}

func TestGetInstance_WrongCampaign(t *testing.T) {
	repo := &mockInstanceRepo{
		findByIDFn: func(_ context.Context, _ int) (*InventoryInstance, error) {
			return &InventoryInstance{ID: 1, CampaignID: "camp-other"}, nil
		},
	}
	svc := newTestInstanceService(repo)
	_, err := svc.GetInstance(context.Background(), "camp-1", 1)
	if err == nil || !isAppError(err) {
		t.Error("expected IDOR error for wrong campaign")
	}
}

func TestDeleteInstance_Success(t *testing.T) {
	deleteCalled := false
	repo := &mockInstanceRepo{
		findByIDFn: func(_ context.Context, _ int) (*InventoryInstance, error) {
			return &InventoryInstance{ID: 1, CampaignID: "camp-1"}, nil
		},
		deleteFn: func(_ context.Context, id int) error {
			deleteCalled = true
			if id != 1 {
				t.Errorf("delete called with ID %d, want 1", id)
			}
			return nil
		},
	}
	svc := newTestInstanceService(repo)
	err := svc.DeleteInstance(context.Background(), "camp-1", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleteCalled {
		t.Error("Delete was not called on repo")
	}
}

func TestDeleteInstance_WrongCampaign(t *testing.T) {
	repo := &mockInstanceRepo{
		findByIDFn: func(_ context.Context, _ int) (*InventoryInstance, error) {
			return &InventoryInstance{ID: 1, CampaignID: "camp-other"}, nil
		},
	}
	svc := newTestInstanceService(repo)
	err := svc.DeleteInstance(context.Background(), "camp-1", 1)
	if err == nil {
		t.Error("expected IDOR error")
	}
}

func TestUpdateInstance_Success(t *testing.T) {
	repo := &mockInstanceRepo{
		findByIDFn: func(_ context.Context, _ int) (*InventoryInstance, error) {
			return &InventoryInstance{ID: 1, CampaignID: "camp-1"}, nil
		},
	}
	svc := newTestInstanceService(repo)
	err := svc.UpdateInstance(context.Background(), "camp-1", 1, CreateInstanceInput{Name: "Updated"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateInstance_EmptyName(t *testing.T) {
	repo := &mockInstanceRepo{
		findByIDFn: func(_ context.Context, _ int) (*InventoryInstance, error) {
			return &InventoryInstance{ID: 1, CampaignID: "camp-1"}, nil
		},
	}
	svc := newTestInstanceService(repo)
	err := svc.UpdateInstance(context.Background(), "camp-1", 1, CreateInstanceInput{Name: ""})
	if err == nil {
		t.Error("expected validation error for empty name")
	}
}

func TestAddItem_Success(t *testing.T) {
	addCalled := false
	repo := &mockInstanceRepo{
		findByIDFn: func(_ context.Context, _ int) (*InventoryInstance, error) {
			return &InventoryInstance{ID: 1, CampaignID: "camp-1"}, nil
		},
		addItemFn: func(_ context.Context, _ int, entityID string, _ int) error {
			addCalled = true
			if entityID != "entity-1" {
				t.Errorf("entityID = %q, want %q", entityID, "entity-1")
			}
			return nil
		},
	}
	svc := newTestInstanceService(repo)
	err := svc.AddItem(context.Background(), "camp-1", 1, "entity-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !addCalled {
		t.Error("AddItem was not called on repo")
	}
}

func TestAddItem_EmptyEntityID(t *testing.T) {
	repo := &mockInstanceRepo{
		findByIDFn: func(_ context.Context, _ int) (*InventoryInstance, error) {
			return &InventoryInstance{ID: 1, CampaignID: "camp-1"}, nil
		},
	}
	svc := newTestInstanceService(repo)
	err := svc.AddItem(context.Background(), "camp-1", 1, "")
	if err == nil {
		t.Error("expected error for empty entity_id")
	}
}

func TestRemoveItem_Success(t *testing.T) {
	repo := &mockInstanceRepo{
		findByIDFn: func(_ context.Context, _ int) (*InventoryInstance, error) {
			return &InventoryInstance{ID: 1, CampaignID: "camp-1"}, nil
		},
	}
	svc := newTestInstanceService(repo)
	err := svc.RemoveItem(context.Background(), "camp-1", 1, "entity-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Party Loot", "party-loot"},
		{"Hello World!", "hello-world"},
		{"already-slugged", "already-slugged"},
		{"  spaces  ", "spaces"},
		{"CamelCase", "camelcase"},
		{"special@chars#here", "special-chars-here"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := slugify(tt.input); got != tt.expected {
				t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
