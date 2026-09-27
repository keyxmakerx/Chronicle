// icon_test.go pins the icon name check on addon create/update, and the
// import-safe fallback RegisterSystemAddon applies for a package-supplied
// icon it cannot use.
package addons

import (
	"context"
	"testing"
)

// badAddonIconNames are inputs that must fail sanitize.ValidateIcon.
var badAddonIconNames = []string{
	`fa-x" data-y="z`,
	`<b>`,
	`FA-BOOK`,
	`fa-`,
}

func TestCreateAddon_InvalidIcon(t *testing.T) {
	for _, icon := range badAddonIconNames {
		t.Run(icon, func(t *testing.T) {
			created := false
			repo := &mockAddonRepo{
				findBySlugFn: func(_ context.Context, _ string) (*Addon, error) {
					return nil, nil // slug available
				},
				createFn: func(_ context.Context, _ *Addon) error {
					created = true
					return nil
				},
			}
			svc := NewAddonService(repo)
			_, err := svc.Create(context.Background(), CreateAddonInput{
				Slug:     "test-addon",
				Name:     "Test Addon",
				Category: CategoryWidget,
				Icon:     icon,
			})
			assertAppError(t, err, 400)
			if created {
				t.Error("expected repo.Create not to be called for an invalid icon")
			}
		})
	}
}

func TestCreateAddon_ValidIcon(t *testing.T) {
	var captured *Addon
	repo := &mockAddonRepo{
		findBySlugFn: func(_ context.Context, _ string) (*Addon, error) {
			return nil, nil
		},
		createFn: func(_ context.Context, addon *Addon) error {
			captured = addon
			addon.ID = 1
			return nil
		},
	}
	svc := NewAddonService(repo)
	_, err := svc.Create(context.Background(), CreateAddonInput{
		Slug:     "test-addon",
		Name:     "Test Addon",
		Category: CategoryWidget,
		Icon:     "fa-dragon",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-dragon" {
		t.Errorf("expected icon fa-dragon to be persisted, got %+v", captured)
	}
}

func TestUpdateAddon_InvalidIcon(t *testing.T) {
	for _, icon := range badAddonIconNames {
		t.Run(icon, func(t *testing.T) {
			updated := false
			repo := &mockAddonRepo{
				findByIDFn: func(_ context.Context, id int) (*Addon, error) {
					return &Addon{ID: id, Slug: "test-addon", Name: "Old"}, nil
				},
				updateFn: func(_ context.Context, _ *Addon) error {
					updated = true
					return nil
				},
			}
			svc := NewAddonService(repo)
			_, err := svc.Update(context.Background(), 1, UpdateAddonInput{
				Name:   "Test Addon",
				Status: StatusActive,
				Icon:   icon,
			})
			assertAppError(t, err, 400)
			if updated {
				t.Error("expected repo.Update not to be called for an invalid icon")
			}
		})
	}
}

func TestUpdateAddon_ValidIcon(t *testing.T) {
	var captured *Addon
	repo := &mockAddonRepo{
		findByIDFn: func(_ context.Context, id int) (*Addon, error) {
			return &Addon{ID: id, Slug: "test-addon", Name: "Old"}, nil
		},
		updateFn: func(_ context.Context, addon *Addon) error {
			captured = addon
			return nil
		},
	}
	svc := NewAddonService(repo)
	_, err := svc.Update(context.Background(), 1, UpdateAddonInput{
		Name:   "Test Addon",
		Status: StatusActive,
		Icon:   "fa-dragon",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-dragon" {
		t.Errorf("expected icon fa-dragon to be persisted, got %+v", captured)
	}
}

// TestRegisterSystemAddon_InvalidIconFallsBack: a system package manifest is
// third-party input. An icon it supplies that fails the check must not
// reject the whole registration — it stores the column default instead. The
// package-level registry (builtinAddons) is a global, so it is restored
// after the test to avoid leaking state into other tests in this package.
func TestRegisterSystemAddon_InvalidIconFallsBack(t *testing.T) {
	original := builtinAddons
	originalInstalled := installedAddons
	t.Cleanup(func() {
		builtinAddons = original
		installedAddons = originalInstalled
	})
	// Work on copies so the cleanup above restores exactly what was there,
	// even though RegisterSystemAddon mutates installedAddons in place.
	builtinAddons = append([]addonDef(nil), original...)
	installedAddons = make(map[string]bool, len(originalInstalled))
	for k, v := range originalInstalled {
		installedAddons[k] = v
	}

	RegisterSystemAddon("test-system", "Test System", "A test system", "1.0.0", `<b>bad</b>`, "Author")

	found := false
	for _, def := range builtinAddons {
		if def.Slug == "test-system" {
			found = true
			if def.Icon != "fa-puzzle-piece" {
				t.Errorf("expected fallback icon fa-puzzle-piece, got %q", def.Icon)
			}
		}
	}
	if !found {
		t.Fatal("expected test-system to be registered")
	}
}
