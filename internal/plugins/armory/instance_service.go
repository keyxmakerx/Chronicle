// instance_service.go contains business logic for inventory instances.
// Handles creation, validation, and IDOR protection for instance operations.
package armory

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// InstanceService handles business logic for inventory instances.
type InstanceService interface {
	// ListInstances returns all instances for a campaign, with each ItemCount
	// narrowed to the items the viewer (role + userID) may see.
	ListInstances(ctx context.Context, campaignID string, role int, userID string) ([]InventoryInstance, error)

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

// EntityCampaignChecker reports whether an entity belongs to a campaign.
// Implemented in routes.go over the entities service so this plugin never
// reaches into another plugin's repository.
type EntityCampaignChecker interface {
	EntityBelongsToCampaign(ctx context.Context, entityID, campaignID string) (bool, error)
}

// instanceService implements InstanceService.
type instanceService struct {
	repo           InstanceRepository
	visibility     EntityVisibilityFilter
	entityCampaign EntityCampaignChecker
}

// NewInstanceService creates a new instance service. visibility is the same
// canonical gate the gallery uses, so a collection's count can never exceed
// what its gallery view lists; entityCampaign guards AddItem against linking
// another campaign's entity.
func NewInstanceService(repo InstanceRepository, visibility EntityVisibilityFilter, entityCampaign EntityCampaignChecker) InstanceService {
	return &instanceService{repo: repo, visibility: visibility, entityCampaign: entityCampaign}
}

// ListInstances returns the campaign's instances with viewer-visible counts.
// The raw SQL count ignores entity visibility, so a Player would be told
// "Loot (7)" and then see two items; counting through the visibility filter
// avoids both the mismatch and the leak of how many hidden items exist.
func (s *instanceService) ListInstances(ctx context.Context, campaignID string, role int, userID string) ([]InventoryInstance, error) {
	instances, err := s.repo.ListByCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return instances, nil
	}
	byInstance, err := s.repo.ListItemEntityIDsByInstance(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	// nil means "unrestricted" (Owner); a non-nil set is the viewable ids.
	var viewable map[string]bool
	if role < permissions.RoleOwner {
		viewable = map[string]bool{}
		seen := make(map[string]bool)
		all := make([]string, 0)
		for _, ids := range byInstance {
			for _, id := range ids {
				if !seen[id] {
					seen[id] = true
					all = append(all, id)
				}
			}
		}
		// Fail closed without a filter, like the gallery.
		if s.visibility != nil && len(all) > 0 {
			viewable, err = s.visibility.FilterViewableEntityIDs(ctx, campaignID, all, role, userID)
			if err != nil {
				return nil, fmt.Errorf("filtering instance item visibility: %w", err)
			}
		}
	}

	for i := range instances {
		ids := byInstance[instances[i].ID]
		if viewable == nil {
			instances[i].ItemCount = len(ids)
			continue
		}
		n := 0
		for _, id := range ids {
			if viewable[id] {
				n++
			}
		}
		instances[i].ItemCount = n
	}
	return instances, nil
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

	icon, err := sanitize.ValidateIcon(input.Icon)
	if err != nil {
		return nil, err
	}
	if icon == "" {
		icon = "fa-box"
	}

	color := strings.TrimSpace(input.Color)
	if color == "" {
		color = "#6b7280"
	}
	if !instanceColorRe.MatchString(color) {
		return nil, apperror.NewBadRequest("color must be a hex value like #6b7280")
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

	icon, err := sanitize.ValidateIcon(input.Icon)
	if err != nil {
		return err
	}
	if icon == "" {
		icon = "fa-box"
	}

	color := strings.TrimSpace(input.Color)
	if color == "" {
		color = "#6b7280"
	}
	if !instanceColorRe.MatchString(color) {
		return apperror.NewBadRequest("color must be a hex value like #6b7280")
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
	// Without this, any entity id (including another campaign's) could be
	// linked and then counted or listed here.
	if s.entityCampaign == nil {
		return apperror.NewInternal(errors.New("entity campaign checker not configured"))
	}
	ok, err := s.entityCampaign.EntityBelongsToCampaign(ctx, entityID, campaignID)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == http.StatusNotFound {
			return apperror.NewNotFound("entity")
		}
		return err
	}
	if !ok {
		return apperror.NewNotFound("entity")
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

// instanceColorRe matches the #rrggbb values the column (VARCHAR(7)) stores.
var instanceColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

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
