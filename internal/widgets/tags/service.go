package tags

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// hexColorPattern validates 7-character hex color strings (#RRGGBB).
var hexColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// slugPattern matches one or more non-alphanumeric characters for slug generation.
var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

// TagService defines the business logic contract for tag operations.
// Handlers call these methods -- they never touch the repository directly.
type TagService interface {
	// Create validates input and creates a new tag in the campaign.
	Create(ctx context.Context, campaignID string, name, color string) (*Tag, error)

	// GetByID retrieves a single tag by ID.
	GetByID(ctx context.Context, id int) (*Tag, error)

	// ListByCampaign returns all tags for a campaign.
	ListByCampaign(ctx context.Context, campaignID string) ([]Tag, error)

	// Update validates input and updates an existing tag.
	Update(ctx context.Context, id int, name, color string) (*Tag, error)

	// Delete removes a tag and all its entity associations.
	Delete(ctx context.Context, id int) error

	// SetEntityTags replaces all tags on an entity with the given set of tag IDs.
	// Performs a diff: removes tags not in the new set, adds tags not currently present.
	SetEntityTags(ctx context.Context, entityID string, campaignID string, tagIDs []int) error

	// GetEntityTags returns all tags associated with an entity.
	GetEntityTags(ctx context.Context, entityID string) ([]Tag, error)

	// GetEntityTagsBatch returns tags for multiple entities in a single query.
	GetEntityTagsBatch(ctx context.Context, entityIDs []string) (map[string][]Tag, error)
}

// tagService implements TagService with validation and slug generation.
type tagService struct {
	repo TagRepository
}

// NewTagService creates a new TagService backed by the given repository.
func NewTagService(repo TagRepository) TagService {
	return &tagService{repo: repo}
}

// Create validates the tag name and color, generates a URL-safe slug, and
// persists the new tag. The slug is derived from the name (lowercase, hyphens
// replace non-alphanumeric characters).
func (s *tagService) Create(ctx context.Context, campaignID string, name, color string) (*Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperror.NewBadRequest("tag name is required")
	}

	color = strings.TrimSpace(color)
	if color == "" {
		color = "#6b7280" // Default gray matching the migration default.
	}
	if !hexColorPattern.MatchString(color) {
		return nil, apperror.NewBadRequest("color must be a valid hex color (e.g. #ff5733)")
	}

	tag := &Tag{
		CampaignID: campaignID,
		Name:       name,
		Slug:       generateSlug(name),
		Color:      color,
	}

	if err := s.repo.Create(ctx, tag); err != nil {
		return nil, err
	}

	return tag, nil
}

// GetByID retrieves a single tag by its primary key.
func (s *tagService) GetByID(ctx context.Context, id int) (*Tag, error) {
	return s.repo.FindByID(ctx, id)
}

// ListByCampaign returns all tags for the given campaign.
func (s *tagService) ListByCampaign(ctx context.Context, campaignID string) ([]Tag, error) {
	return s.repo.ListByCampaign(ctx, campaignID)
}

// Update validates the new name and color, regenerates the slug, and persists
// the changes to the tag.
func (s *tagService) Update(ctx context.Context, id int, name, color string) (*Tag, error) {
	// Verify the tag exists before updating.
	tag, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperror.NewBadRequest("tag name is required")
	}

	color = strings.TrimSpace(color)
	if color == "" {
		color = "#6b7280"
	}
	if !hexColorPattern.MatchString(color) {
		return nil, apperror.NewBadRequest("color must be a valid hex color (e.g. #ff5733)")
	}

	tag.Name = name
	tag.Slug = generateSlug(name)
	tag.Color = color

	if err := s.repo.Update(ctx, tag); err != nil {
		return nil, err
	}

	return tag, nil
}

// Delete removes a tag by ID. The database cascade deletes entity_tags rows.
func (s *tagService) Delete(ctx context.Context, id int) error {
	return s.repo.Delete(ctx, id)
}

// SetEntityTags replaces all tags on an entity with the provided tag IDs.
// It performs a diff against the current tags to minimize database operations:
// only tags that need to be added or removed result in queries.
//
// All provided tag IDs are validated to belong to the same campaign as the
// entity to prevent cross-campaign tag assignment.
func (s *tagService) SetEntityTags(ctx context.Context, entityID string, campaignID string, tagIDs []int) error {
	// Validate that all provided tag IDs belong to the correct campaign.
	// This prevents users from assigning tags from other campaigns.
	if len(tagIDs) > 0 {
		campaignTags, err := s.repo.ListByCampaign(ctx, campaignID)
		if err != nil {
			return fmt.Errorf("listing campaign tags for validation: %w", err)
		}

		validIDs := make(map[int]bool, len(campaignTags))
		for _, t := range campaignTags {
			validIDs[t.ID] = true
		}

		for _, id := range tagIDs {
			if !validIDs[id] {
				return apperror.NewBadRequest(fmt.Sprintf("tag ID %d does not belong to this campaign", id))
			}
		}
	}

	// Get current tags to compute the diff.
	currentTags, err := s.repo.GetEntityTags(ctx, entityID)
	if err != nil {
		return fmt.Errorf("getting current entity tags: %w", err)
	}

	// Build sets for efficient diff computation.
	currentSet := make(map[int]bool, len(currentTags))
	for _, t := range currentTags {
		currentSet[t.ID] = true
	}

	desiredSet := make(map[int]bool, len(tagIDs))
	for _, id := range tagIDs {
		desiredSet[id] = true
	}

	// Remove tags that are in current but not in desired.
	for _, t := range currentTags {
		if !desiredSet[t.ID] {
			if err := s.repo.RemoveTagFromEntity(ctx, entityID, t.ID); err != nil {
				return fmt.Errorf("removing tag %d from entity: %w", t.ID, err)
			}
		}
	}

	// Add tags that are in desired but not in current.
	for _, id := range tagIDs {
		if !currentSet[id] {
			if err := s.repo.AddTagToEntity(ctx, entityID, id); err != nil {
				return fmt.Errorf("adding tag %d to entity: %w", id, err)
			}
		}
	}

	return nil
}

// GetEntityTags returns all tags associated with the given entity.
func (s *tagService) GetEntityTags(ctx context.Context, entityID string) ([]Tag, error) {
	return s.repo.GetEntityTags(ctx, entityID)
}

// GetEntityTagsBatch returns tags for multiple entities in one query.
func (s *tagService) GetEntityTagsBatch(ctx context.Context, entityIDs []string) (map[string][]Tag, error) {
	return s.repo.GetEntityTagsBatch(ctx, entityIDs)
}

// generateSlug creates a URL-safe slug from a tag name. Converts to lowercase,
// replaces sequences of non-alphanumeric characters with a single hyphen, and
// trims leading/trailing hyphens.
func generateSlug(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = slugPattern.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "tag"
	}
	return slug
}
