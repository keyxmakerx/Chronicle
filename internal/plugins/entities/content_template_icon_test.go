// content_template_icon_test.go pins the icon name check on content
// template create/update: an invalid icon is refused before the repository
// is touched, a valid Font Awesome name is accepted, and the documented
// default/keep-existing behavior applies when the icon is absent.
package entities

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// mockContentTemplateRepo implements ContentTemplateRepository for testing.
type mockContentTemplateRepo struct {
	createFn   func(ctx context.Context, t *ContentTemplate) error
	findByIDFn func(ctx context.Context, id int) (*ContentTemplate, error)
	updateFn   func(ctx context.Context, t *ContentTemplate) error
}

func (m *mockContentTemplateRepo) Create(ctx context.Context, t *ContentTemplate) error {
	if m.createFn != nil {
		return m.createFn(ctx, t)
	}
	return nil
}

func (m *mockContentTemplateRepo) FindByID(ctx context.Context, id int) (*ContentTemplate, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockContentTemplateRepo) ListForCampaign(ctx context.Context, campaignID string) ([]ContentTemplate, error) {
	return nil, nil
}

func (m *mockContentTemplateRepo) ListForCampaignAndType(ctx context.Context, campaignID string, entityTypeID int) ([]ContentTemplate, error) {
	return nil, nil
}

func (m *mockContentTemplateRepo) Update(ctx context.Context, t *ContentTemplate) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, t)
	}
	return nil
}

func (m *mockContentTemplateRepo) Delete(ctx context.Context, id int) error {
	return nil
}

// badContentTemplateIconNames are inputs that must fail sanitize.ValidateIcon.
var badContentTemplateIconNames = []string{
	`fa-x" data-y="z`,
	`<b>`,
	`FA-BOOK`,
	`fa-`,
}

func TestCreateContentTemplate_InvalidIcon(t *testing.T) {
	for _, icon := range badContentTemplateIconNames {
		t.Run(icon, func(t *testing.T) {
			created := false
			repo := &mockContentTemplateRepo{
				createFn: func(_ context.Context, _ *ContentTemplate) error {
					created = true
					return nil
				},
			}
			svc := NewContentTemplateService(repo, &mockEntityTypeRepo{})
			_, err := svc.Create(context.Background(), "camp-1", CreateContentTemplateInput{
				Name:        "Recap",
				ContentJSON: `{"type":"doc"}`,
				Icon:        icon,
			})
			assertAppError(t, err, 400)
			if created {
				t.Error("expected repo.Create not to be called for an invalid icon")
			}
		})
	}
}

func TestCreateContentTemplate_ValidIcon(t *testing.T) {
	var captured *ContentTemplate
	repo := &mockContentTemplateRepo{
		createFn: func(_ context.Context, tpl *ContentTemplate) error {
			captured = tpl
			return nil
		},
	}
	svc := NewContentTemplateService(repo, &mockEntityTypeRepo{})
	_, err := svc.Create(context.Background(), "camp-1", CreateContentTemplateInput{
		Name:        "Recap",
		ContentJSON: `{"type":"doc"}`,
		Icon:        "fa-dragon",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-dragon" {
		t.Errorf("expected icon fa-dragon to be persisted, got %+v", captured)
	}
}

func TestCreateContentTemplate_EmptyIconDefaults(t *testing.T) {
	var captured *ContentTemplate
	repo := &mockContentTemplateRepo{
		createFn: func(_ context.Context, tpl *ContentTemplate) error {
			captured = tpl
			return nil
		},
	}
	svc := NewContentTemplateService(repo, &mockEntityTypeRepo{})
	_, err := svc.Create(context.Background(), "camp-1", CreateContentTemplateInput{
		Name:        "Recap",
		ContentJSON: `{"type":"doc"}`,
		Icon:        "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-file-lines" {
		t.Errorf("expected default icon fa-file-lines, got %+v", captured)
	}
}

func TestUpdateContentTemplate_InvalidIcon(t *testing.T) {
	for _, icon := range badContentTemplateIconNames {
		t.Run(icon, func(t *testing.T) {
			updated := false
			repo := &mockContentTemplateRepo{
				findByIDFn: func(_ context.Context, id int) (*ContentTemplate, error) {
					return &ContentTemplate{ID: id, Name: "Recap", ContentJSON: `{"type":"doc"}`, Icon: "fa-scroll"}, nil
				},
				updateFn: func(_ context.Context, _ *ContentTemplate) error {
					updated = true
					return nil
				},
			}
			svc := NewContentTemplateService(repo, &mockEntityTypeRepo{})
			_, err := svc.Update(context.Background(), 1, UpdateContentTemplateInput{
				Name:        patch.Of("Recap"),
				ContentJSON: patch.Of(`{"type":"doc"}`),
				Icon:        patch.Of(icon),
			})
			assertAppError(t, err, 400)
			if updated {
				t.Error("expected repo.Update not to be called for an invalid icon")
			}
		})
	}
}

func TestUpdateContentTemplate_ValidIcon(t *testing.T) {
	var captured *ContentTemplate
	repo := &mockContentTemplateRepo{
		findByIDFn: func(_ context.Context, id int) (*ContentTemplate, error) {
			return &ContentTemplate{ID: id, Name: "Recap", ContentJSON: `{"type":"doc"}`, Icon: "fa-scroll"}, nil
		},
		updateFn: func(_ context.Context, tpl *ContentTemplate) error {
			captured = tpl
			return nil
		},
	}
	svc := NewContentTemplateService(repo, &mockEntityTypeRepo{})
	_, err := svc.Update(context.Background(), 1, UpdateContentTemplateInput{
		Name:        patch.Of("Recap"),
		ContentJSON: patch.Of(`{"type":"doc"}`),
		Icon:        patch.Of("fa-dragon"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-dragon" {
		t.Errorf("expected icon fa-dragon to be persisted, got %+v", captured)
	}
}

// TestUpdateContentTemplate_EmptyIconKeepsExisting: an absent icon on update
// preserves the stored value rather than replacing it with a default — this
// is the create-only default case, unlike CreateContentTemplate.
func TestUpdateContentTemplate_EmptyIconKeepsExisting(t *testing.T) {
	var captured *ContentTemplate
	repo := &mockContentTemplateRepo{
		findByIDFn: func(_ context.Context, id int) (*ContentTemplate, error) {
			return &ContentTemplate{ID: id, Name: "Recap", ContentJSON: `{"type":"doc"}`, Icon: "fa-scroll"}, nil
		},
		updateFn: func(_ context.Context, tpl *ContentTemplate) error {
			captured = tpl
			return nil
		},
	}
	svc := NewContentTemplateService(repo, &mockEntityTypeRepo{})
	_, err := svc.Update(context.Background(), 1, UpdateContentTemplateInput{
		Name:        patch.Of("Recap"),
		ContentJSON: patch.Of(`{"type":"doc"}`),
		Icon:        patch.Of(""),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-scroll" {
		t.Errorf("expected existing icon fa-scroll to be kept, got %+v", captured)
	}
}
