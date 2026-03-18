// instance_service.go contains business logic for inventory instances.
// Handles creation, validation, and IDOR protection for instance operations.
package armory

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// InstanceService handles business logic for inventory instances.
type InstanceService interface {
	// ListInstances returns all instances for a campaign.
	ListInstances(ctx context.Context, campaignID string) ([]InventoryInstance, error)

	// GetInstance retrieves an instance by ID with campaign IDOR check.
	GetInstance(ctx context.Context, campaignID string, instanceID int) (*InventoryInstance, error)

	// CreateInstance creates a new inventory instance.
	CreateInstance(ctx context.Context, campaignID string, input CreateInstanceInput) (*InventoryInstance, error)

	// UpdateInstance modifies an instance after IDOR validation.
	UpdateInstance(ctx context.Context, campaignID string, instanceID int, input CreateInstanceInput) error

	// DeleteInstance removes an instance after IDOR validation.
	DeleteInstance(ctx context.Context, campaignID string, instanceID int) error

	// AddItem adds an entity to an instance after IDOR validation.
	AddItem(ctx context.Context, campaignID string, instanceID int, entityID string) error

	// RemoveItem removes an entity from an instance after IDOR validation.
	RemoveItem(ctx context.Context, campaignID string, instanceID int, entityID string) error
}

// instanceService implements InstanceService.
type instanceService struct {
	repo InstanceRepository
}

// NewInstanceService creates a new instance service.
func NewInstanceService(repo InstanceRepository) InstanceService {
	return &instanceService{repo: repo}
}

// ListInstances returns all instances for a campaign.
func (s *instanceService) ListInstances(ctx context.Context, campaignID string) ([]InventoryInstance, error) {
	return s.repo.ListByCampaign(ctx, campaignID)
}

// GetInstance retrieves and validates an instance belongs to the campaign.
func (s *instanceService) GetInstance(ctx context.Context, campaignID string, instanceID int) (*InventoryInstance, error) {
	inst, err := s.repo.FindByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, apperror.NewNotFound("inventory instance")
	}
	if inst.CampaignID != campaignID {
		return nil, apperror.NewNotFound("inventory instance")
	}
	return inst, nil
}

// CreateInstance validates input and creates a new instance.
func (s *instanceService) CreateInstance(ctx context.Context, campaignID string, input CreateInstanceInput) (*InventoryInstance, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("name is required")
	}
	if len(name) > 100 {
		return nil, apperror.NewBadRequest("name must be under 100 characters")
	}

	slug := slugify(name)
	if slug == "" {
		slug = "inventory"
	}

	icon := strings.TrimSpace(input.Icon)
	if icon == "" {
		icon = "fa-box"
	}

	color := strings.TrimSpace(input.Color)
	if color == "" {
		color = "#6b7280"
	}

	desc := strings.TrimSpace(input.Description)

	return s.repo.Create(ctx, campaignID, name, slug, desc, icon, color)
}

// UpdateInstance validates and updates an instance.
func (s *instanceService) UpdateInstance(ctx context.Context, campaignID string, instanceID int, input CreateInstanceInput) error {
	// IDOR check.
	if _, err := s.GetInstance(ctx, campaignID, instanceID); err != nil {
		return err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return apperror.NewBadRequest("name is required")
	}
	if len(name) > 100 {
		return apperror.NewBadRequest("name must be under 100 characters")
	}

	slug := slugify(name)
	if slug == "" {
		slug = "inventory"
	}

	icon := strings.TrimSpace(input.Icon)
	if icon == "" {
		icon = "fa-box"
	}

	color := strings.TrimSpace(input.Color)
	if color == "" {
		color = "#6b7280"
	}

	desc := strings.TrimSpace(input.Description)

	return s.repo.Update(ctx, instanceID, name, slug, desc, icon, color)
}

// DeleteInstance validates and removes an instance.
func (s *instanceService) DeleteInstance(ctx context.Context, campaignID string, instanceID int) error {
	if _, err := s.GetInstance(ctx, campaignID, instanceID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, instanceID)
}

// AddItem validates and adds an entity to an instance.
func (s *instanceService) AddItem(ctx context.Context, campaignID string, instanceID int, entityID string) error {
	if _, err := s.GetInstance(ctx, campaignID, instanceID); err != nil {
		return err
	}
	if entityID == "" {
		return apperror.NewBadRequest("entity_id is required")
	}
	return s.repo.AddItem(ctx, instanceID, entityID, 1)
}

// RemoveItem validates and removes an entity from an instance.
func (s *instanceService) RemoveItem(ctx context.Context, campaignID string, instanceID int, entityID string) error {
	if _, err := s.GetInstance(ctx, campaignID, instanceID); err != nil {
		return err
	}
	return s.repo.RemoveItem(ctx, instanceID, entityID)
}

// slugRe matches non-alphanumeric characters for slug generation.
var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a name to a URL-safe slug.
func slugify(name string) string {
	s := strings.ToLower(name)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, s)
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 100 {
		s = s[:100]
	}
	return s
}

// formatError wraps an error with context for logging.
func formatError(msg string, err error) error {
	return fmt.Errorf("%s: %w", msg, err)
}
