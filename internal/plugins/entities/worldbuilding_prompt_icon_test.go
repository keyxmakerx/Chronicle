// worldbuilding_prompt_icon_test.go pins the icon name check on
// worldbuilding prompt create/update: an invalid icon is refused before the
// repository is touched, a valid Font Awesome name is accepted, and the
// documented default/keep-existing behavior applies when the icon is
// absent.
package entities

import (
	"context"
	"testing"
)

// badPromptIconNames are inputs that must fail sanitize.ValidateIcon.
var badPromptIconNames = []string{
	`fa-x" onmouseover="y`,
	`<b>`,
	`FA-BOOK`,
	`fa-`,
}

func TestCreatePrompt_InvalidIcon(t *testing.T) {
	for _, icon := range badPromptIconNames {
		t.Run(icon, func(t *testing.T) {
			repo := newMockWBPromptRepo()
			svc := NewWorldbuildingPromptService(repo, &mockEntityTypeListerForPrompts{})
			_, err := svc.Create(context.Background(), "camp-1", CreatePromptInput{
				Name:       "Motivation",
				PromptText: "What drives them?",
				Icon:       icon,
			})
			if err == nil {
				t.Fatal("expected an error for an invalid icon")
			}
			if len(repo.prompts) != 0 {
				t.Error("expected repo.Create not to be called for an invalid icon")
			}
		})
	}
}

func TestCreatePrompt_ValidIcon(t *testing.T) {
	repo := newMockWBPromptRepo()
	svc := NewWorldbuildingPromptService(repo, &mockEntityTypeListerForPrompts{})
	p, err := svc.Create(context.Background(), "camp-1", CreatePromptInput{
		Name:       "Motivation",
		PromptText: "What drives them?",
		Icon:       "fa-dragon",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Icon != "fa-dragon" {
		t.Errorf("expected icon fa-dragon, got %q", p.Icon)
	}
}

func TestCreatePrompt_EmptyIconDefaults(t *testing.T) {
	repo := newMockWBPromptRepo()
	svc := NewWorldbuildingPromptService(repo, &mockEntityTypeListerForPrompts{})
	p, err := svc.Create(context.Background(), "camp-1", CreatePromptInput{
		Name:       "Motivation",
		PromptText: "What drives them?",
		Icon:       "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Icon != "fa-lightbulb" {
		t.Errorf("expected default icon fa-lightbulb, got %q", p.Icon)
	}
}

func TestUpdatePrompt_InvalidIcon(t *testing.T) {
	for _, icon := range badPromptIconNames {
		t.Run(icon, func(t *testing.T) {
			repo := newMockWBPromptRepo()
			svc := NewWorldbuildingPromptService(repo, &mockEntityTypeListerForPrompts{})
			p, err := svc.Create(context.Background(), "camp-1", CreatePromptInput{
				Name:       "Original",
				PromptText: "original text",
				Icon:       "fa-heart",
			})
			if err != nil {
				t.Fatalf("unexpected error seeding prompt: %v", err)
			}

			err = svc.Update(context.Background(), p.ID, UpdatePromptInput{
				Name:       "Original",
				PromptText: "original text",
				Icon:       icon,
			})
			if err == nil {
				t.Fatal("expected an error for an invalid icon")
			}

			got, _ := svc.GetByID(context.Background(), p.ID)
			if got.Icon != "fa-heart" {
				t.Errorf("expected repo write not to change the icon, got %q", got.Icon)
			}
		})
	}
}

func TestUpdatePrompt_ValidIcon(t *testing.T) {
	repo := newMockWBPromptRepo()
	svc := NewWorldbuildingPromptService(repo, &mockEntityTypeListerForPrompts{})
	p, _ := svc.Create(context.Background(), "camp-1", CreatePromptInput{
		Name: "Original", PromptText: "original text",
	})

	err := svc.Update(context.Background(), p.ID, UpdatePromptInput{
		Name:       "Original",
		PromptText: "original text",
		Icon:       "fa-dragon",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := svc.GetByID(context.Background(), p.ID)
	if got.Icon != "fa-dragon" {
		t.Errorf("expected icon fa-dragon, got %q", got.Icon)
	}
}

// TestUpdatePrompt_EmptyIconKeepsExisting: an absent icon on update
// preserves the stored value rather than replacing it with a default.
func TestUpdatePrompt_EmptyIconKeepsExisting(t *testing.T) {
	repo := newMockWBPromptRepo()
	svc := NewWorldbuildingPromptService(repo, &mockEntityTypeListerForPrompts{})
	p, _ := svc.Create(context.Background(), "camp-1", CreatePromptInput{
		Name: "Original", PromptText: "original text", Icon: "fa-heart",
	})

	err := svc.Update(context.Background(), p.ID, UpdatePromptInput{
		Name:       "Original",
		PromptText: "original text",
		Icon:       "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := svc.GetByID(context.Background(), p.ID)
	if got.Icon != "fa-heart" {
		t.Errorf("expected existing icon fa-heart to be kept, got %q", got.Icon)
	}
}
