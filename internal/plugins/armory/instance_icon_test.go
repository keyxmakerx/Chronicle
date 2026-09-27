// instance_icon_test.go pins the icon name check (and the sibling hex color
// check) on inventory instance create/update: an invalid value is refused
// before the repository is touched, a valid value is accepted, and an
// absent icon falls back to the documented default.
package armory

import (
	"context"
	"testing"
)

// badInstanceIconNames are inputs that must fail sanitize.ValidateIcon.
var badInstanceIconNames = []string{
	`fa-x" data-y="z`,
	`<b>`,
	`FA-BOOK`,
	`fa-`,
}

// badInstanceColors are inputs that must fail the #rrggbb color check.
var badInstanceColors = []string{
	`#fff"x`,
	`red`,
}

func TestCreateInstance_InvalidIcon(t *testing.T) {
	for _, icon := range badInstanceIconNames {
		t.Run(icon, func(t *testing.T) {
			created := false
			repo := &mockInstanceRepo{
				createFn: func(_ context.Context, _, _, _, _, _, _ string) (*InventoryInstance, error) {
					created = true
					return &InventoryInstance{ID: 1}, nil
				},
			}
			svc := newTestInstanceService(repo)
			_, err := svc.CreateInstance(context.Background(), "camp-1", CreateInstanceInput{
				Name: "Party Loot",
				Icon: icon,
			})
			if !isAppError(err) {
				t.Fatalf("expected an AppError, got %v", err)
			}
			if created {
				t.Error("expected repo.Create not to be called for an invalid icon")
			}
		})
	}
}

func TestCreateInstance_ValidIconAndDefault(t *testing.T) {
	var capturedIcon string
	repo := &mockInstanceRepo{
		createFn: func(_ context.Context, _, _, _, _, icon, _ string) (*InventoryInstance, error) {
			capturedIcon = icon
			return &InventoryInstance{ID: 1, Icon: icon}, nil
		},
	}
	svc := newTestInstanceService(repo)

	if _, err := svc.CreateInstance(context.Background(), "camp-1", CreateInstanceInput{
		Name: "Vault", Icon: "fa-dragon",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedIcon != "fa-dragon" {
		t.Errorf("expected icon fa-dragon, got %q", capturedIcon)
	}

	if _, err := svc.CreateInstance(context.Background(), "camp-1", CreateInstanceInput{
		Name: "Vault", Icon: "",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedIcon != "fa-box" {
		t.Errorf("expected default icon fa-box, got %q", capturedIcon)
	}
}

func TestCreateInstance_InvalidColor(t *testing.T) {
	for _, color := range badInstanceColors {
		t.Run(color, func(t *testing.T) {
			created := false
			repo := &mockInstanceRepo{
				createFn: func(_ context.Context, _, _, _, _, _, _ string) (*InventoryInstance, error) {
					created = true
					return &InventoryInstance{ID: 1}, nil
				},
			}
			svc := newTestInstanceService(repo)
			_, err := svc.CreateInstance(context.Background(), "camp-1", CreateInstanceInput{
				Name:  "Party Loot",
				Color: color,
			})
			if !isAppError(err) {
				t.Fatalf("expected an AppError, got %v", err)
			}
			if created {
				t.Error("expected repo.Create not to be called for an invalid color")
			}
		})
	}
}

func TestUpdateInstance_InvalidIcon(t *testing.T) {
	for _, icon := range badInstanceIconNames {
		t.Run(icon, func(t *testing.T) {
			updated := false
			repo := &mockInstanceRepo{
				findByIDFn: func(_ context.Context, id int) (*InventoryInstance, error) {
					return &InventoryInstance{ID: id, CampaignID: "camp-1"}, nil
				},
				updateFn: func(_ context.Context, _ int, _, _, _, _, _ string) error {
					updated = true
					return nil
				},
			}
			svc := newTestInstanceService(repo)
			err := svc.UpdateInstance(context.Background(), "camp-1", 1, CreateInstanceInput{
				Name: "Updated",
				Icon: icon,
			})
			if !isAppError(err) {
				t.Fatalf("expected an AppError, got %v", err)
			}
			if updated {
				t.Error("expected repo.Update not to be called for an invalid icon")
			}
		})
	}
}

func TestUpdateInstance_ValidIconAndDefault(t *testing.T) {
	var capturedIcon string
	repo := &mockInstanceRepo{
		findByIDFn: func(_ context.Context, id int) (*InventoryInstance, error) {
			return &InventoryInstance{ID: id, CampaignID: "camp-1"}, nil
		},
		updateFn: func(_ context.Context, _ int, _, _, _, icon, _ string) error {
			capturedIcon = icon
			return nil
		},
	}
	svc := newTestInstanceService(repo)

	if err := svc.UpdateInstance(context.Background(), "camp-1", 1, CreateInstanceInput{
		Name: "Updated", Icon: "fa-dragon",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedIcon != "fa-dragon" {
		t.Errorf("expected icon fa-dragon, got %q", capturedIcon)
	}

	if err := svc.UpdateInstance(context.Background(), "camp-1", 1, CreateInstanceInput{
		Name: "Updated", Icon: "",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedIcon != "fa-box" {
		t.Errorf("expected default icon fa-box, got %q", capturedIcon)
	}
}

func TestUpdateInstance_InvalidColor(t *testing.T) {
	for _, color := range badInstanceColors {
		t.Run(color, func(t *testing.T) {
			updated := false
			repo := &mockInstanceRepo{
				findByIDFn: func(_ context.Context, id int) (*InventoryInstance, error) {
					return &InventoryInstance{ID: id, CampaignID: "camp-1"}, nil
				},
				updateFn: func(_ context.Context, _ int, _, _, _, _, _ string) error {
					updated = true
					return nil
				},
			}
			svc := newTestInstanceService(repo)
			err := svc.UpdateInstance(context.Background(), "camp-1", 1, CreateInstanceInput{
				Name:  "Updated",
				Color: color,
			})
			if !isAppError(err) {
				t.Fatalf("expected an AppError, got %v", err)
			}
			if updated {
				t.Error("expected repo.Update not to be called for an invalid color")
			}
		})
	}
}
