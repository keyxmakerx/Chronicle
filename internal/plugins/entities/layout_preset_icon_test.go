// layout_preset_icon_test.go pins the icon name check on layout preset
// create/update: an invalid icon is refused before the repository is
// touched, a valid Font Awesome name is accepted, and an absent icon falls
// back to the documented default.
package entities

import (
	"context"
	"testing"
)

// mockLayoutPresetRepo implements LayoutPresetRepository for testing.
type mockLayoutPresetRepo struct {
	createFn   func(ctx context.Context, p *LayoutPreset) error
	findByIDFn func(ctx context.Context, id int) (*LayoutPreset, error)
	updateFn   func(ctx context.Context, p *LayoutPreset) error
}

func (m *mockLayoutPresetRepo) Create(ctx context.Context, p *LayoutPreset) error {
	if m.createFn != nil {
		return m.createFn(ctx, p)
	}
	return nil
}

func (m *mockLayoutPresetRepo) FindByID(ctx context.Context, id int) (*LayoutPreset, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockLayoutPresetRepo) ListForCampaign(ctx context.Context, campaignID string) ([]LayoutPreset, error) {
	return nil, nil
}

func (m *mockLayoutPresetRepo) Update(ctx context.Context, p *LayoutPreset) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, p)
	}
	return nil
}

func (m *mockLayoutPresetRepo) Delete(ctx context.Context, id int) error {
	return nil
}

// badLayoutPresetIconNames are inputs that must fail sanitize.ValidateIcon.
var badLayoutPresetIconNames = []string{
	`fa-x" onmouseover="y`,
	`<b>`,
	`FA-BOOK`,
	`fa-`,
}

// validLayoutJSON is a structurally valid layout preset body, reused across
// these tests so only the icon input varies.
var validLayoutJSON = mustMarshalLayout(DefaultLayout())

func TestCreateLayoutPreset_InvalidIcon(t *testing.T) {
	for _, icon := range badLayoutPresetIconNames {
		t.Run(icon, func(t *testing.T) {
			created := false
			repo := &mockLayoutPresetRepo{
				createFn: func(_ context.Context, _ *LayoutPreset) error {
					created = true
					return nil
				},
			}
			svc := NewLayoutPresetService(repo)
			_, err := svc.Create(context.Background(), "camp-1", CreateLayoutPresetInput{
				Name:       "Standard",
				LayoutJSON: validLayoutJSON,
				Icon:       icon,
			})
			assertAppError(t, err, 400)
			if created {
				t.Error("expected repo.Create not to be called for an invalid icon")
			}
		})
	}
}

func TestCreateLayoutPreset_ValidIcon(t *testing.T) {
	var captured *LayoutPreset
	repo := &mockLayoutPresetRepo{
		createFn: func(_ context.Context, p *LayoutPreset) error {
			captured = p
			return nil
		},
	}
	svc := NewLayoutPresetService(repo)
	_, err := svc.Create(context.Background(), "camp-1", CreateLayoutPresetInput{
		Name:       "Standard",
		LayoutJSON: validLayoutJSON,
		Icon:       "fa-dragon",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-dragon" {
		t.Errorf("expected icon fa-dragon to be persisted, got %+v", captured)
	}
}

func TestCreateLayoutPreset_EmptyIconDefaults(t *testing.T) {
	var captured *LayoutPreset
	repo := &mockLayoutPresetRepo{
		createFn: func(_ context.Context, p *LayoutPreset) error {
			captured = p
			return nil
		},
	}
	svc := NewLayoutPresetService(repo)
	_, err := svc.Create(context.Background(), "camp-1", CreateLayoutPresetInput{
		Name:       "Standard",
		LayoutJSON: validLayoutJSON,
		Icon:       "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-table-columns" {
		t.Errorf("expected default icon fa-table-columns, got %+v", captured)
	}
}

func TestUpdateLayoutPreset_InvalidIcon(t *testing.T) {
	for _, icon := range badLayoutPresetIconNames {
		t.Run(icon, func(t *testing.T) {
			updated := false
			repo := &mockLayoutPresetRepo{
				findByIDFn: func(_ context.Context, id int) (*LayoutPreset, error) {
					return &LayoutPreset{ID: id, Name: "Standard", LayoutJSON: validLayoutJSON, Icon: "fa-table-columns"}, nil
				},
				updateFn: func(_ context.Context, _ *LayoutPreset) error {
					updated = true
					return nil
				},
			}
			svc := NewLayoutPresetService(repo)
			_, err := svc.Update(context.Background(), 1, UpdateLayoutPresetInput{
				Name:       "Standard",
				LayoutJSON: validLayoutJSON,
				Icon:       icon,
			})
			assertAppError(t, err, 400)
			if updated {
				t.Error("expected repo.Update not to be called for an invalid icon")
			}
		})
	}
}

func TestUpdateLayoutPreset_ValidIcon(t *testing.T) {
	var captured *LayoutPreset
	repo := &mockLayoutPresetRepo{
		findByIDFn: func(_ context.Context, id int) (*LayoutPreset, error) {
			return &LayoutPreset{ID: id, Name: "Standard", LayoutJSON: validLayoutJSON, Icon: "fa-table-columns"}, nil
		},
		updateFn: func(_ context.Context, p *LayoutPreset) error {
			captured = p
			return nil
		},
	}
	svc := NewLayoutPresetService(repo)
	_, err := svc.Update(context.Background(), 1, UpdateLayoutPresetInput{
		Name:       "Standard",
		LayoutJSON: validLayoutJSON,
		Icon:       "fa-dragon",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-dragon" {
		t.Errorf("expected icon fa-dragon to be persisted, got %+v", captured)
	}
}

// TestUpdateLayoutPreset_EmptyIconDefaults: layout presets apply the same
// default on update as on create (validateInput is shared), unlike
// content templates / worldbuilding prompts which keep the existing icon.
func TestUpdateLayoutPreset_EmptyIconDefaults(t *testing.T) {
	var captured *LayoutPreset
	repo := &mockLayoutPresetRepo{
		findByIDFn: func(_ context.Context, id int) (*LayoutPreset, error) {
			return &LayoutPreset{ID: id, Name: "Standard", LayoutJSON: validLayoutJSON, Icon: "fa-map"}, nil
		},
		updateFn: func(_ context.Context, p *LayoutPreset) error {
			captured = p
			return nil
		},
	}
	svc := NewLayoutPresetService(repo)
	_, err := svc.Update(context.Background(), 1, UpdateLayoutPresetInput{
		Name:       "Standard",
		LayoutJSON: validLayoutJSON,
		Icon:       "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-table-columns" {
		t.Errorf("expected default icon fa-table-columns, got %+v", captured)
	}
}
