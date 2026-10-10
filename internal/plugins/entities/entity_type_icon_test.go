// entity_type_icon_test.go pins the icon name check on entity type
// create/update: an invalid icon is refused before the repository is
// touched, a valid Font Awesome name is accepted, and an absent icon falls
// back to the documented default.
package entities

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// badIconNames are inputs that must fail sanitize.ValidateIcon.
var badEntityTypeIconNames = []string{
	`fa-x" data-y="z`,
	`<b>`,
	`FA-BOOK`,
	`fa-`,
}

func TestCreateEntityType_InvalidIcon(t *testing.T) {
	for _, icon := range badEntityTypeIconNames {
		t.Run(icon, func(t *testing.T) {
			created := false
			typeRepo := &mockEntityTypeRepo{
				createFn: func(_ context.Context, _ *EntityType) error {
					created = true
					return nil
				},
			}
			svc := newTestService(&mockEntityRepo{}, typeRepo)
			_, err := svc.CreateEntityType(context.Background(), "camp-1", CreateEntityTypeInput{
				Name: "Beastiary",
				Icon: icon,
			})
			assertAppError(t, err, 400)
			if created {
				t.Error("expected repo.Create not to be called for an invalid icon")
			}
		})
	}
}

func TestCreateEntityType_ValidIcon(t *testing.T) {
	var captured *EntityType
	typeRepo := &mockEntityTypeRepo{
		createFn: func(_ context.Context, et *EntityType) error {
			captured = et
			return nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo)
	_, err := svc.CreateEntityType(context.Background(), "camp-1", CreateEntityTypeInput{
		Name: "Dragon",
		Icon: "fa-dragon",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-dragon" {
		t.Errorf("expected icon fa-dragon to be persisted, got %+v", captured)
	}
}

func TestCreateEntityType_EmptyIconDefaults(t *testing.T) {
	var captured *EntityType
	typeRepo := &mockEntityTypeRepo{
		createFn: func(_ context.Context, et *EntityType) error {
			captured = et
			return nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo)
	_, err := svc.CreateEntityType(context.Background(), "camp-1", CreateEntityTypeInput{
		Name: "Location",
		Icon: "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-circle" {
		t.Errorf("expected default icon fa-circle, got %+v", captured)
	}
}

func TestUpdateEntityType_InvalidIcon(t *testing.T) {
	for _, icon := range badEntityTypeIconNames {
		t.Run(icon, func(t *testing.T) {
			updated := false
			typeRepo := &mockEntityTypeRepo{
				findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
					return &EntityType{ID: id, CampaignID: "camp-1", Name: "Location", Color: "#6b7280"}, nil
				},
				updateFn: func(_ context.Context, _ *EntityType) error {
					updated = true
					return nil
				},
			}
			svc := newTestService(&mockEntityRepo{}, typeRepo)
			_, err := svc.UpdateEntityType(context.Background(), 1, UpdateEntityTypeInput{
				Name: patch.Of("Location"),
				Icon: patch.Of(icon),
			})
			assertAppError(t, err, 400)
			if updated {
				t.Error("expected repo.Update not to be called for an invalid icon")
			}
		})
	}
}

func TestUpdateEntityType_ValidIcon(t *testing.T) {
	var captured *EntityType
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
			return &EntityType{ID: id, CampaignID: "camp-1", Name: "Location", Color: "#6b7280"}, nil
		},
		updateFn: func(_ context.Context, et *EntityType) error {
			captured = et
			return nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo)
	_, err := svc.UpdateEntityType(context.Background(), 1, UpdateEntityTypeInput{
		Name: patch.Of("Location"),
		Icon: patch.Of("fa-dragon"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-dragon" {
		t.Errorf("expected icon fa-dragon to be persisted, got %+v", captured)
	}
}

func TestUpdateEntityType_EmptyIconDefaults(t *testing.T) {
	var captured *EntityType
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
			return &EntityType{ID: id, CampaignID: "camp-1", Name: "Location", Color: "#6b7280", Icon: "fa-map"}, nil
		},
		updateFn: func(_ context.Context, et *EntityType) error {
			captured = et
			return nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo)
	_, err := svc.UpdateEntityType(context.Background(), 1, UpdateEntityTypeInput{
		Name: patch.Of("Location"),
		Icon: patch.Of(""),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-circle" {
		t.Errorf("expected empty icon to reset to default fa-circle, got %+v", captured)
	}
}
