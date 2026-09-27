package entities

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// WorldbuildingPromptService handles business logic for worldbuilding prompts.
type WorldbuildingPromptService interface {
	Create(ctx context.Context, campaignID string, input CreatePromptInput) (*WorldbuildingPrompt, error)
	GetByID(ctx context.Context, id int) (*WorldbuildingPrompt, error)
	ListForCampaign(ctx context.Context, campaignID string) ([]WorldbuildingPrompt, error)
	ListForType(ctx context.Context, campaignID string, entityTypeID int) ([]WorldbuildingPrompt, error)
	Update(ctx context.Context, id int, input UpdatePromptInput) error
	Delete(ctx context.Context, id int) error
	SeedDefaults(ctx context.Context, campaignID string) error
}

// EntityTypeLister lists entity types for a campaign. Subset of
// EntityTypeRepository to avoid importing the full interface.
type EntityTypeLister interface {
	ListByCampaign(ctx context.Context, campaignID string) ([]EntityType, error)
}

type worldbuildingPromptService struct {
	repo      WorldbuildingPromptRepository
	typeRepo  EntityTypeLister
}

// NewWorldbuildingPromptService creates a new worldbuilding prompt service.
func NewWorldbuildingPromptService(repo WorldbuildingPromptRepository, typeRepo EntityTypeLister) WorldbuildingPromptService {
	return &worldbuildingPromptService{repo: repo, typeRepo: typeRepo}
}

// Create validates and inserts a new worldbuilding prompt.
func (s *worldbuildingPromptService) Create(ctx context.Context, campaignID string, input CreatePromptInput) (*WorldbuildingPrompt, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewValidation("name is required")
	}
	if len(name) > 200 {
		return nil, apperror.NewValidation("name must be 200 characters or fewer")
	}

	text := strings.TrimSpace(input.PromptText)
	if text == "" {
		return nil, apperror.NewValidation("prompt text is required")
	}
	if len(text) > 5000 {
		return nil, apperror.NewValidation("prompt text must be 5000 characters or fewer")
	}

	icon, err := sanitize.ValidateIcon(input.Icon)
	if err != nil {
		return nil, err
	}
	if icon == "" {
		icon = "fa-lightbulb"
	}

	p := &WorldbuildingPrompt{
		CampaignID:   &campaignID,
		EntityTypeID: input.EntityTypeID,
		Name:         name,
		PromptText:   text,
		Icon:         icon,
	}

	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// GetByID retrieves a worldbuilding prompt by its ID.
func (s *worldbuildingPromptService) GetByID(ctx context.Context, id int) (*WorldbuildingPrompt, error) {
	return s.repo.FindByID(ctx, id)
}

// ListForCampaign returns all prompts available to a campaign.
func (s *worldbuildingPromptService) ListForCampaign(ctx context.Context, campaignID string) ([]WorldbuildingPrompt, error) {
	return s.repo.ListForCampaign(ctx, campaignID)
}

// ListForType returns prompts matching a specific entity type plus universal prompts.
func (s *worldbuildingPromptService) ListForType(ctx context.Context, campaignID string, entityTypeID int) ([]WorldbuildingPrompt, error) {
	return s.repo.ListForCampaignAndType(ctx, campaignID, entityTypeID)
}

// Update validates and modifies an existing worldbuilding prompt.
func (s *worldbuildingPromptService) Update(ctx context.Context, id int, input UpdatePromptInput) error {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return apperror.NewValidation("name is required")
	}
	if len(name) > 200 {
		return apperror.NewValidation("name must be 200 characters or fewer")
	}

	text := strings.TrimSpace(input.PromptText)
	if text == "" {
		return apperror.NewValidation("prompt text is required")
	}
	if len(text) > 5000 {
		return apperror.NewValidation("prompt text must be 5000 characters or fewer")
	}

	icon, err := sanitize.ValidateIcon(input.Icon)
	if err != nil {
		return err
	}
	if icon == "" {
		icon = existing.Icon
	}

	existing.Name = name
	existing.PromptText = text
	existing.Icon = icon

	return s.repo.Update(ctx, existing)
}

// Delete removes a worldbuilding prompt by ID.
func (s *worldbuildingPromptService) Delete(ctx context.Context, id int) error {
	return s.repo.Delete(ctx, id)
}

// SeedDefaults populates default worldbuilding prompts for a new campaign.
// Called during campaign creation after entity types are seeded.
func (s *worldbuildingPromptService) SeedDefaults(ctx context.Context, campaignID string) error {
	// Fetch entity types for this campaign to map slugs to IDs.
	entityTypes, err := s.typeRepo.ListByCampaign(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("listing entity types for prompt seeding: %w", err)
	}

	slugToID := make(map[string]int)
	for _, et := range entityTypes {
		slugToID[et.Slug] = et.ID
	}

	// Default prompts organized by entity type slug.
	defaults := map[string][]struct {
		name string
		text string
		icon string
	}{
		"character": {
			{"Motivation", "What motivates this character above all else?", "fa-heart"},
			{"Secret", "What secret does this character keep from others?", "fa-mask"},
			{"First Impression", "How would their closest ally describe them in three words?", "fa-comments"},
			{"Defining Moment", "What event from their past shaped who they are today?", "fa-bolt"},
		},
		"location": {
			{"Sensory Details", "What sounds, smells, or textures define this place?", "fa-ear-listen"},
			{"Power Structure", "Who controls this area and how do they maintain power?", "fa-crown"},
			{"Hidden Secrets", "What hidden danger or secret does this place hold?", "fa-eye-slash"},
			{"History", "How has this place changed over the last century?", "fa-hourglass-half"},
		},
		"organization": {
			{"Origin Story", "What is this group's founding myth or origin story?", "fa-scroll"},
			{"Internal Conflict", "What internal conflict threatens to tear it apart?", "fa-handshake-slash"},
			{"Membership", "How does an outsider gain membership or trust?", "fa-door-open"},
		},
		"item": {
			{"Creation", "What is the history of this item's creation?", "fa-hammer"},
			{"Quirk", "What unexpected side effect or quirk does it have?", "fa-wand-sparkles"},
		},
		"quest": {
			{"Stakes", "What happens if the party fails or ignores this quest?", "fa-skull-crossbones"},
			{"Beneficiary", "Who benefits most from the quest's completion?", "fa-trophy"},
			{"Moral Dilemma", "What moral dilemma does this quest present?", "fa-scale-balanced"},
		},
	}

	for slug, prompts := range defaults {
		typeID, ok := slugToID[slug]
		if !ok {
			continue // Entity type not present in this campaign.
		}

		for i, def := range prompts {
			p := &WorldbuildingPrompt{
				CampaignID:   &campaignID,
				EntityTypeID: &typeID,
				Name:         def.name,
				PromptText:   def.text,
				Icon:         def.icon,
				SortOrder:    i,
			}
			if err := s.repo.Create(ctx, p); err != nil {
				return fmt.Errorf("seeding prompt %q: %w", def.name, err)
			}
		}
	}

	slog.Info("worldbuilding prompts seeded", slog.String("campaign_id", campaignID))
	return nil
}
