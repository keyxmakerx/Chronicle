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
//
// The includeDmOnly parameter controls visibility filtering. Pass true for
// Scribes/Owners (who can see all tags) and false for Players (who should
// not see dm_only tags).
type TagService interface {
	// Create validates input and creates a new tag in the campaign.
	Create(ctx context.Context, campaignID string, name, color string, dmOnly bool) (*Tag, error)

	// GetByID retrieves a single tag by ID.
	GetByID(ctx context.Context, id int) (*Tag, error)

	// ListByCampaign returns all tags for a campaign. When includeDmOnly is
	// false, dm_only tags are excluded (player view).
	ListByCampaign(ctx context.Context, campaignID string, includeDmOnly bool) ([]Tag, error)

	// Update validates input and updates an existing tag. PARTIAL update:
	// Color/DmOnly nil preserves the stored value, non-nil replaces it —
	// see UpdateTagInput's doc comment.
	Update(ctx context.Context, id int, input UpdateTagInput) (*Tag, error)

	// Delete removes a tag and all its entity associations.
	Delete(ctx context.Context, id int) error

	// SetEntityTags replaces all tags on an entity with the given set of tag IDs.
	// Performs a diff: removes tags not in the new set, adds tags not currently present.
	SetEntityTags(ctx context.Context, entityID string, campaignID string, tagIDs []int) error

	// SetEntityTagsVisible is SetEntityTags for a caller who may not see
	// dm_only tags: existing dm_only tags are left untouched and a request
	// naming one is rejected as if it did not exist.
	SetEntityTagsVisible(ctx context.Context, entityID string, campaignID string, tagIDs []int, includeDmOnly bool) error

	// GetEntityTags returns all tags associated with an entity. When
	// includeDmOnly is false, dm_only tags are excluded.
	GetEntityTags(ctx context.Context, entityID string, includeDmOnly bool) ([]Tag, error)

	// GetEntityTagsBatch returns tags for multiple entities in a single query.
	// When includeDmOnly is false, dm_only tags are excluded.
	GetEntityTagsBatch(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]Tag, error)

	// SetHookDispatcher wires the extension hook dispatcher. Optional.
	SetHookDispatcher(d TagHookDispatcher)
}

// TagHookDispatcher tells extension (WASM) plugins that a tag was put on or
// taken off a page. The context carries the write's origin so the dispatcher
// can skip a plugin's own writes.
type TagHookDispatcher interface {
	TagAdded(ctx context.Context, campaignID, entityID string, tag Tag)
	TagRemoved(ctx context.Context, campaignID, entityID string, tag Tag)
}

// tagService implements TagService with validation and slug generation.
type tagService struct {
	repo  TagRepository
	hooks TagHookDispatcher // nil: extension plugins are not told
}

// NewTagService creates a new TagService backed by the given repository.
func NewTagService(repo TagRepository) TagService {
	return &tagService{repo: repo}
}

// SetHookDispatcher wires the extension hook dispatcher.
func (s *tagService) SetHookDispatcher(d TagHookDispatcher) {
	s.hooks = d
}

// Create validates the tag name and color, generates a URL-safe slug, and
// persists the new tag. The slug is derived from the name (lowercase, hyphens
// replace non-alphanumeric characters).
func (s *tagService) Create(ctx context.Context, campaignID string, name, color string, dmOnly bool) (*Tag, error) {
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
		DmOnly:     dmOnly,
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

// ListByCampaign returns all tags for the given campaign. When includeDmOnly
// is false, dm_only tags are excluded (player view).
func (s *tagService) ListByCampaign(ctx context.Context, campaignID string, includeDmOnly bool) ([]Tag, error) {
	return s.repo.ListByCampaign(ctx, campaignID, includeDmOnly)
}

// Update validates the new name, regenerates the slug, and persists the
// changes to the tag. Color and DmOnly are PARTIAL: a nil pointer means the
// caller did not send that key, so the stored value is left alone.
func (s *tagService) Update(ctx context.Context, id int, input UpdateTagInput) (*Tag, error) {
	// Verify the tag exists before updating.
	tag, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("tag name is required")
	}

	// Load-merge-write (ADR-056): `tag` is the row as stored, so Color/DmOnly
	// default to the stored value; only a key the caller actually sent changes.
	color := tag.Color
	if input.Color != nil {
		color = strings.TrimSpace(*input.Color)
		if color == "" {
			color = "#6b7280"
		}
		if !hexColorPattern.MatchString(color) {
			return nil, apperror.NewBadRequest("color must be a valid hex color (e.g. #ff5733)")
		}
	}

	dmOnly := tag.DmOnly
	if input.DmOnly != nil {
		dmOnly = *input.DmOnly
	}

	tag.Name = name
	tag.Slug = generateSlug(name)
	tag.Color = color
	tag.DmOnly = dmOnly

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
	return s.SetEntityTagsVisible(ctx, entityID, campaignID, tagIDs, true)
}

// SetEntityTagsVisible is SetEntityTags limited to the tags the caller may
// see. A caller who cannot see dm_only tags works on a filtered view of both
// the campaign's tags and the entity's, so the diff can neither remove a
// dm_only tag it never saw nor add one it should not know about.
func (s *tagService) SetEntityTagsVisible(ctx context.Context, entityID string, campaignID string, tagIDs []int, includeDmOnly bool) error {
	// Validate that all provided tag IDs belong to the correct campaign.
	// This prevents users from assigning tags from other campaigns.
	validIDs := make(map[int]Tag)
	if len(tagIDs) > 0 {
		campaignTags, err := s.repo.ListByCampaign(ctx, campaignID, includeDmOnly)
		if err != nil {
			return fmt.Errorf("listing campaign tags for validation: %w", err)
		}

		for _, t := range campaignTags {
			validIDs[t.ID] = t
		}

		for _, id := range tagIDs {
			if _, ok := validIDs[id]; !ok {
				return apperror.NewBadRequest(fmt.Sprintf("tag ID %d does not belong to this campaign", id))
			}
		}
	}

	// Diff only against what the caller can see; hidden dm_only tags stay.
	currentTags, err := s.repo.GetEntityTags(ctx, entityID, includeDmOnly)
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
			if s.hooks != nil {
				s.hooks.TagRemoved(ctx, campaignID, entityID, t)
			}
		}
	}

	// Add tags that are in desired but not in current.
	for _, id := range tagIDs {
		if !currentSet[id] {
			if err := s.repo.AddTagToEntity(ctx, entityID, id); err != nil {
				return fmt.Errorf("adding tag %d to entity: %w", id, err)
			}
			if s.hooks != nil {
				s.hooks.TagAdded(ctx, campaignID, entityID, validIDs[id])
			}
		}
	}

	return nil
}

// GetEntityTags returns all tags associated with the given entity.
// When includeDmOnly is false, dm_only tags are excluded.
func (s *tagService) GetEntityTags(ctx context.Context, entityID string, includeDmOnly bool) ([]Tag, error) {
	return s.repo.GetEntityTags(ctx, entityID, includeDmOnly)
}

// GetEntityTagsBatch returns tags for multiple entities in one query.
// When includeDmOnly is false, dm_only tags are excluded.
func (s *tagService) GetEntityTagsBatch(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]Tag, error) {
	return s.repo.GetEntityTagsBatch(ctx, entityIDs, includeDmOnly)
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
