package entities

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// EntityService handles business logic for entity operations.
// It owns slug generation, privacy enforcement, and entity type seeding.
// Also implements the campaigns.EntityTypeSeeder interface.
type EntityService interface {
	// Entity CRUD
	Create(ctx context.Context, campaignID, userID string, input CreateEntityInput) (*Entity, error)
	GetByID(ctx context.Context, id string) (*Entity, error)
	GetBySlug(ctx context.Context, campaignID, slug string) (*Entity, error)
	Update(ctx context.Context, entityID string, input UpdateEntityInput) (*Entity, error)
	UpdateEntry(ctx context.Context, entityID, entryJSON, entryHTML string) error
	UpdateFields(ctx context.Context, entityID string, fieldsData map[string]any) error
	UpdateFieldOverrides(ctx context.Context, entityID string, overrides *FieldOverrides) error
	UpdateImage(ctx context.Context, entityID, imagePath string) error
	Delete(ctx context.Context, entityID string) error

	// Hierarchy
	GetChildren(ctx context.Context, entityID string, role int) ([]Entity, error)
	GetAncestors(ctx context.Context, entityID string) ([]Entity, error)

	// Backlinks
	GetBacklinks(ctx context.Context, entityID string, role int) ([]Entity, error)

	// Popup preview config
	UpdatePopupConfig(ctx context.Context, entityID string, config *PopupConfig) error

	// Listing and search
	List(ctx context.Context, campaignID string, typeID int, role int, opts ListOptions) ([]Entity, int, error)
	ListRecent(ctx context.Context, campaignID string, role int, limit int) ([]Entity, error)
	Search(ctx context.Context, campaignID, query string, typeID int, role int, opts ListOptions) ([]Entity, int, error)

	// Entity types
	GetEntityTypes(ctx context.Context, campaignID string) ([]EntityType, error)
	GetEntityTypeBySlug(ctx context.Context, campaignID, slug string) (*EntityType, error)
	GetEntityTypeByID(ctx context.Context, id int) (*EntityType, error)
	CountByType(ctx context.Context, campaignID string, role int) (map[int]int, error)
	CreateEntityType(ctx context.Context, campaignID string, input CreateEntityTypeInput) (*EntityType, error)
	UpdateEntityType(ctx context.Context, id int, input UpdateEntityTypeInput) (*EntityType, error)
	DeleteEntityType(ctx context.Context, id int) error
	UpdateEntityTypeLayout(ctx context.Context, id int, layout EntityTypeLayout) error
	UpdateEntityTypeColor(ctx context.Context, id int, color string) error
	UpdateEntityTypeDashboard(ctx context.Context, id int, description *string, pinnedIDs []string) error

	// Category dashboard layout
	GetCategoryDashboardLayout(ctx context.Context, id int) (*string, error)
	UpdateCategoryDashboardLayout(ctx context.Context, id int, layoutJSON string) error
	ResetCategoryDashboardLayout(ctx context.Context, id int) error

	// Seeder (satisfies campaigns.EntityTypeSeeder interface).
	SeedDefaults(ctx context.Context, campaignID string) error
}

// entityService implements EntityService.
type entityService struct {
	entities EntityRepository
	types    EntityTypeRepository
}

// NewEntityService creates a new entity service with the given dependencies.
func NewEntityService(entities EntityRepository, types EntityTypeRepository) EntityService {
	return &entityService{
		entities: entities,
		types:    types,
	}
}

// --- Entity CRUD ---

// Create creates a new entity in a campaign.
func (s *entityService) Create(ctx context.Context, campaignID, userID string, input CreateEntityInput) (*Entity, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("entity name is required")
	}
	if len(name) > 200 {
		return nil, apperror.NewBadRequest("entity name must be at most 200 characters")
	}

	// Verify the entity type exists and belongs to this campaign.
	et, err := s.types.FindByID(ctx, input.EntityTypeID)
	if err != nil {
		return nil, apperror.NewBadRequest("invalid entity type")
	}
	if et.CampaignID != campaignID {
		return nil, apperror.NewBadRequest("entity type does not belong to this campaign")
	}

	// Generate a unique slug scoped to the campaign.
	slug, err := s.generateSlug(ctx, campaignID, name)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("generating slug: %w", err))
	}

	now := time.Now().UTC()
	typeLabel := strings.TrimSpace(input.TypeLabel)
	var typeLabelPtr *string
	if typeLabel != "" {
		typeLabelPtr = &typeLabel
	}

	// Validate parent if specified.
	var parentIDPtr *string
	if pid := strings.TrimSpace(input.ParentID); pid != "" {
		parent, err := s.entities.FindByID(ctx, pid)
		if err != nil {
			return nil, apperror.NewBadRequest("parent entity not found")
		}
		if parent.CampaignID != campaignID {
			return nil, apperror.NewBadRequest("parent entity does not belong to this campaign")
		}
		parentIDPtr = &pid
	}

	fieldsData := input.FieldsData
	if fieldsData == nil {
		fieldsData = make(map[string]any)
	}

	entity := &Entity{
		ID:           generateUUID(),
		CampaignID:   campaignID,
		EntityTypeID: input.EntityTypeID,
		Name:         name,
		Slug:         slug,
		ParentID:     parentIDPtr,
		TypeLabel:    typeLabelPtr,
		IsPrivate:    input.IsPrivate,
		IsTemplate:   false,
		FieldsData:   fieldsData,
		CreatedBy:    userID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.entities.Create(ctx, entity); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating entity: %w", err))
	}

	slog.Info("entity created",
		slog.String("entity_id", entity.ID),
		slog.String("campaign_id", campaignID),
		slog.String("type", et.Slug),
		slog.String("name", name),
	)

	return entity, nil
}

// GetByID retrieves an entity by ID.
func (s *entityService) GetByID(ctx context.Context, id string) (*Entity, error) {
	return s.entities.FindByID(ctx, id)
}

// GetBySlug retrieves an entity by campaign ID and slug.
func (s *entityService) GetBySlug(ctx context.Context, campaignID, slug string) (*Entity, error) {
	return s.entities.FindBySlug(ctx, campaignID, slug)
}

// Update modifies an existing entity's name, type_label, privacy, entry, and fields.
func (s *entityService) Update(ctx context.Context, entityID string, input UpdateEntityInput) (*Entity, error) {
	entity, err := s.entities.FindByID(ctx, entityID)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("entity name is required")
	}
	if len(name) > 200 {
		return nil, apperror.NewBadRequest("entity name must be at most 200 characters")
	}

	// Regenerate slug if name changed.
	if name != entity.Name {
		slug, err := s.generateSlug(ctx, entity.CampaignID, name)
		if err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("generating slug: %w", err))
		}
		entity.Slug = slug
	}

	entity.Name = name
	entity.IsPrivate = input.IsPrivate

	typeLabel := strings.TrimSpace(input.TypeLabel)
	if typeLabel != "" {
		entity.TypeLabel = &typeLabel
	} else {
		entity.TypeLabel = nil
	}

	// Validate and update parent_id.
	pid := strings.TrimSpace(input.ParentID)
	if pid != "" {
		if pid == entityID {
			return nil, apperror.NewBadRequest("an entity cannot be its own parent")
		}
		parent, err := s.entities.FindByID(ctx, pid)
		if err != nil {
			return nil, apperror.NewBadRequest("parent entity not found")
		}
		if parent.CampaignID != entity.CampaignID {
			return nil, apperror.NewBadRequest("parent entity does not belong to this campaign")
		}
		// Check for circular reference: the proposed parent must not be
		// a descendant of this entity.
		ancestors, err := s.entities.FindAncestors(ctx, pid)
		if err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("checking ancestors: %w", err))
		}
		for _, a := range ancestors {
			if a.ID == entityID {
				return nil, apperror.NewBadRequest("circular reference: the selected parent is a descendant of this entity")
			}
		}
		entity.ParentID = &pid
	} else {
		entity.ParentID = nil
	}

	// Update entry content if provided. Sanitize HTML to prevent stored XSS.
	entry := strings.TrimSpace(input.Entry)
	if entry != "" {
		entity.Entry = &entry
		sanitized := sanitize.HTML(entry)
		entity.EntryHTML = &sanitized
	}

	if input.FieldsData != nil {
		entity.FieldsData = input.FieldsData
	}

	entity.UpdatedAt = time.Now().UTC()

	if err := s.entities.Update(ctx, entity); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("updating entity: %w", err))
	}

	return entity, nil
}

// --- Hierarchy ---

// GetChildren returns the direct children of an entity, respecting privacy.
func (s *entityService) GetChildren(ctx context.Context, entityID string, role int) ([]Entity, error) {
	children, err := s.entities.FindChildren(ctx, entityID, role)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("finding children: %w", err))
	}
	return children, nil
}

// GetAncestors returns the ancestor chain from immediate parent to root.
func (s *entityService) GetAncestors(ctx context.Context, entityID string) ([]Entity, error) {
	ancestors, err := s.entities.FindAncestors(ctx, entityID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("finding ancestors: %w", err))
	}
	return ancestors, nil
}

// GetBacklinks returns entities that reference the given entity via @mention
// links in their entry content. Respects privacy by role.
func (s *entityService) GetBacklinks(ctx context.Context, entityID string, role int) ([]Entity, error) {
	backlinks, err := s.entities.FindBacklinks(ctx, entityID, role)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("finding backlinks: %w", err))
	}
	return backlinks, nil
}

// UpdatePopupConfig updates the entity's hover preview tooltip configuration.
func (s *entityService) UpdatePopupConfig(ctx context.Context, entityID string, config *PopupConfig) error {
	return s.entities.UpdatePopupConfig(ctx, entityID, config)
}

// UpdateEntry updates only the entry content for an entity. Used by the
// editor widget's autosave to persist content without a full entity update.
// The entryHTML is sanitized with bluemonday before storage to prevent stored XSS.
func (s *entityService) UpdateEntry(ctx context.Context, entityID, entryJSON, entryHTML string) error {
	if strings.TrimSpace(entryJSON) == "" {
		return apperror.NewBadRequest("entry content is required")
	}
	// Sanitize HTML to strip dangerous content (script tags, event handlers, etc.).
	entryHTML = sanitize.HTML(entryHTML)
	if err := s.entities.UpdateEntry(ctx, entityID, entryJSON, entryHTML); err != nil {
		return err
	}
	slog.Info("entity entry updated", slog.String("entity_id", entityID))
	return nil
}

// UpdateFields updates only the entity's custom field values. Used by the
// attributes widget to persist inline field edits.
func (s *entityService) UpdateFields(ctx context.Context, entityID string, fieldsData map[string]any) error {
	if fieldsData == nil {
		fieldsData = make(map[string]any)
	}
	if err := s.entities.UpdateFields(ctx, entityID, fieldsData); err != nil {
		return err
	}
	slog.Info("entity fields updated", slog.String("entity_id", entityID))
	return nil
}

// UpdateFieldOverrides persists per-entity field customizations (added, hidden,
// modified fields). Validates that added fields have required properties.
func (s *entityService) UpdateFieldOverrides(ctx context.Context, entityID string, overrides *FieldOverrides) error {
	if overrides != nil {
		// Validate added fields have keys and labels.
		for i, f := range overrides.Added {
			if f.Key == "" {
				return apperror.NewBadRequest(fmt.Sprintf("added field %d missing key", i))
			}
			if f.Label == "" {
				return apperror.NewBadRequest(fmt.Sprintf("added field %d missing label", i))
			}
			if f.Type == "" {
				overrides.Added[i].Type = "text"
			}
		}
	}
	if err := s.entities.UpdateFieldOverrides(ctx, entityID, overrides); err != nil {
		return err
	}
	slog.Info("entity field overrides updated", slog.String("entity_id", entityID))
	return nil
}

// UpdateImage sets or clears the entity's header image path.
// Validates the path to prevent directory traversal attacks.
func (s *entityService) UpdateImage(ctx context.Context, entityID, imagePath string) error {
	if imagePath != "" {
		// Reject absolute paths and directory traversal attempts.
		if strings.HasPrefix(imagePath, "/") || strings.Contains(imagePath, "..") {
			return apperror.NewBadRequest("invalid image path")
		}
	}
	if err := s.entities.UpdateImage(ctx, entityID, imagePath); err != nil {
		return err
	}
	slog.Info("entity image updated",
		slog.String("entity_id", entityID),
		slog.String("image_path", imagePath),
	)
	return nil
}

// Delete removes an entity.
func (s *entityService) Delete(ctx context.Context, entityID string) error {
	if err := s.entities.Delete(ctx, entityID); err != nil {
		return err
	}
	slog.Info("entity deleted", slog.String("entity_id", entityID))
	return nil
}

// --- Listing and Search ---

// List returns entities with pagination, optional type filter, and privacy enforcement.
func (s *entityService) List(ctx context.Context, campaignID string, typeID int, role int, opts ListOptions) ([]Entity, int, error) {
	if opts.PerPage < 1 || opts.PerPage > 100 {
		opts.PerPage = 24
	}
	if opts.Page < 1 {
		opts.Page = 1
	}
	return s.entities.ListByCampaign(ctx, campaignID, typeID, role, opts)
}

// ListRecent returns the most recently updated entities for a campaign dashboard.
func (s *entityService) ListRecent(ctx context.Context, campaignID string, role int, limit int) ([]Entity, error) {
	if limit < 1 || limit > 20 {
		limit = 8
	}
	return s.entities.ListRecent(ctx, campaignID, role, limit)
}

// Search performs a text search on entity names with a minimum query length.
func (s *entityService) Search(ctx context.Context, campaignID, query string, typeID int, role int, opts ListOptions) ([]Entity, int, error) {
	q := strings.TrimSpace(query)
	if len(q) < 2 {
		return nil, 0, apperror.NewBadRequest("search query must be at least 2 characters")
	}
	if opts.PerPage < 1 || opts.PerPage > 100 {
		opts.PerPage = 24
	}
	if opts.Page < 1 {
		opts.Page = 1
	}
	return s.entities.Search(ctx, campaignID, q, typeID, role, opts)
}

// --- Entity Types ---

// GetEntityTypes returns all entity types for a campaign.
func (s *entityService) GetEntityTypes(ctx context.Context, campaignID string) ([]EntityType, error) {
	return s.types.ListByCampaign(ctx, campaignID)
}

// GetEntityTypeBySlug returns an entity type by campaign ID and slug.
func (s *entityService) GetEntityTypeBySlug(ctx context.Context, campaignID, slug string) (*EntityType, error) {
	return s.types.FindBySlug(ctx, campaignID, slug)
}

// GetEntityTypeByID returns an entity type by its auto-increment ID.
func (s *entityService) GetEntityTypeByID(ctx context.Context, id int) (*EntityType, error) {
	return s.types.FindByID(ctx, id)
}

// CountByType returns entity counts per entity type for sidebar badges.
func (s *entityService) CountByType(ctx context.Context, campaignID string, role int) (map[int]int, error) {
	return s.entities.CountByType(ctx, campaignID, role)
}

// --- Entity Type CRUD ---

// maxEntityTypeSlugAttempts caps slug deduplication iterations for entity types.
const maxEntityTypeSlugAttempts = 100

// CreateEntityType validates input and creates a new entity type in a campaign.
func (s *entityService) CreateEntityType(ctx context.Context, campaignID string, input CreateEntityTypeInput) (*EntityType, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("entity type name is required")
	}
	if len(name) > 100 {
		return nil, apperror.NewBadRequest("entity type name must be at most 100 characters")
	}

	namePlural := strings.TrimSpace(input.NamePlural)
	if namePlural == "" {
		// Auto-pluralize by appending "s" if not provided.
		namePlural = name + "s"
	}
	if len(namePlural) > 100 {
		return nil, apperror.NewBadRequest("entity type plural name must be at most 100 characters")
	}

	icon := strings.TrimSpace(input.Icon)
	if icon == "" {
		icon = "fa-circle" // Default icon.
	}

	color := strings.TrimSpace(input.Color)
	if color == "" {
		color = "#6b7280" // Default gray.
	}
	if !hexColorPattern.MatchString(color) {
		return nil, apperror.NewBadRequest("color must be a valid hex value like #ff0000")
	}

	// Generate a unique slug scoped to the campaign.
	slug, err := s.generateEntityTypeSlug(ctx, campaignID, name)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("generating entity type slug: %w", err))
	}

	// Get the next sort order.
	maxOrder, err := s.types.MaxSortOrder(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("querying max sort order: %w", err))
	}

	et := &EntityType{
		CampaignID: campaignID,
		Slug:       slug,
		Name:       name,
		NamePlural: namePlural,
		Icon:       icon,
		Color:      color,
		Fields:     []FieldDefinition{},
		Layout:     DefaultLayout(),
		SortOrder:  maxOrder + 1,
		IsDefault:  false,
		Enabled:    true,
	}

	if err := s.types.Create(ctx, et); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating entity type: %w", err))
	}

	slog.Info("entity type created",
		slog.Int("entity_type_id", et.ID),
		slog.String("campaign_id", campaignID),
		slog.String("slug", et.Slug),
		slog.String("name", name),
	)

	return et, nil
}

// UpdateEntityType validates input and updates an existing entity type.
func (s *entityService) UpdateEntityType(ctx context.Context, id int, input UpdateEntityTypeInput) (*EntityType, error) {
	et, err := s.types.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("entity type name is required")
	}
	if len(name) > 100 {
		return nil, apperror.NewBadRequest("entity type name must be at most 100 characters")
	}

	namePlural := strings.TrimSpace(input.NamePlural)
	if namePlural == "" {
		namePlural = name + "s"
	}
	if len(namePlural) > 100 {
		return nil, apperror.NewBadRequest("entity type plural name must be at most 100 characters")
	}

	icon := strings.TrimSpace(input.Icon)
	if icon == "" {
		icon = "fa-circle"
	}

	color := strings.TrimSpace(input.Color)
	if color == "" {
		color = et.Color // Keep existing color if not provided.
	}
	if !hexColorPattern.MatchString(color) {
		return nil, apperror.NewBadRequest("color must be a valid hex value like #ff0000")
	}

	// Regenerate slug if name changed.
	if name != et.Name {
		slug, err := s.generateEntityTypeSlug(ctx, et.CampaignID, name)
		if err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("generating entity type slug: %w", err))
		}
		et.Slug = slug
	}

	et.Name = name
	et.NamePlural = namePlural
	et.Icon = icon
	et.Color = color

	// Update fields if provided.
	if input.Fields != nil {
		et.Fields = input.Fields
	}

	if err := s.types.Update(ctx, et); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("updating entity type: %w", err))
	}

	slog.Info("entity type updated",
		slog.Int("entity_type_id", et.ID),
		slog.String("name", name),
	)

	return et, nil
}

// DeleteEntityType removes an entity type if no entities reference it.
func (s *entityService) DeleteEntityType(ctx context.Context, id int) error {
	et, err := s.types.FindByID(ctx, id)
	if err != nil {
		return err
	}

	// Check if any entities use this type. Count with max permission (Owner=3)
	// to include private entities in the check.
	counts, err := s.entities.CountByType(ctx, et.CampaignID, 3)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("counting entities by type: %w", err))
	}

	if count, ok := counts[id]; ok && count > 0 {
		return apperror.NewConflict(fmt.Sprintf("cannot delete entity type: %d entities still use it", count))
	}

	if err := s.types.Delete(ctx, id); err != nil {
		return apperror.NewInternal(fmt.Errorf("deleting entity type: %w", err))
	}

	slog.Info("entity type deleted",
		slog.Int("entity_type_id", id),
		slog.String("campaign_id", et.CampaignID),
		slog.String("name", et.Name),
	)

	return nil
}

// generateEntityTypeSlug creates a unique slug for an entity type within a campaign.
// If the base slug is taken, appends -2, -3, etc. After maxEntityTypeSlugAttempts,
// falls back to a random suffix.
func (s *entityService) generateEntityTypeSlug(ctx context.Context, campaignID, name string) (string, error) {
	base := Slugify(name)
	slug := base

	for i := 2; i < maxEntityTypeSlugAttempts+2; i++ {
		exists, err := s.types.SlugExists(ctx, campaignID, slug)
		if err != nil {
			return "", fmt.Errorf("checking entity type slug: %w", err)
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}

	// Fallback: append random suffix to guarantee uniqueness.
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random slug suffix: %w", err)
	}
	return fmt.Sprintf("%s-%s", base, hex.EncodeToString(b)), nil
}

// Layout validation limits.
const (
	maxLayoutRows       = 20
	maxLayoutCols       = 4
	maxLayoutBlocks     = 10
	gridWidth           = 12
)

// validBlockTypes are the allowed values for TemplateBlock.Type.
var validBlockTypes = map[string]bool{
	"title": true, "image": true, "entry": true,
	"attributes": true, "details": true, "divider": true,
}

// UpdateEntityTypeLayout validates and persists a new layout for an entity type.
// Accepts the new row/column/block format.
func (s *entityService) UpdateEntityTypeLayout(ctx context.Context, id int, layout EntityTypeLayout) error {
	if len(layout.Rows) > maxLayoutRows {
		return apperror.NewBadRequest("too many layout rows")
	}

	seenBlockIDs := make(map[string]bool)
	for _, row := range layout.Rows {
		if strings.TrimSpace(row.ID) == "" {
			return apperror.NewBadRequest("row ID is required")
		}
		if len(row.Columns) == 0 || len(row.Columns) > maxLayoutCols {
			return apperror.NewBadRequest("each row must have 1-4 columns")
		}

		totalWidth := 0
		for _, col := range row.Columns {
			if strings.TrimSpace(col.ID) == "" {
				return apperror.NewBadRequest("column ID is required")
			}
			if col.Width < 1 || col.Width > gridWidth {
				return apperror.NewBadRequest("column width must be 1-12")
			}
			totalWidth += col.Width

			if len(col.Blocks) > maxLayoutBlocks {
				return apperror.NewBadRequest("too many blocks in column")
			}
			for _, blk := range col.Blocks {
				if strings.TrimSpace(blk.ID) == "" {
					return apperror.NewBadRequest("block ID is required")
				}
				if seenBlockIDs[blk.ID] {
					return apperror.NewBadRequest("duplicate block ID: " + blk.ID)
				}
				seenBlockIDs[blk.ID] = true
				if !validBlockTypes[blk.Type] {
					return apperror.NewBadRequest("invalid block type: " + blk.Type)
				}
			}
		}
		if totalWidth != gridWidth {
			return apperror.NewBadRequest("column widths in a row must sum to 12")
		}
	}

	layoutJSON, err := json.Marshal(layout)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling layout: %w", err))
	}

	if err := s.types.UpdateLayout(ctx, id, string(layoutJSON)); err != nil {
		return err
	}

	slog.Info("entity type layout updated", slog.Int("entity_type_id", id))
	return nil
}

// hexColorPattern validates CSS hex color values (#rgb or #rrggbb).
var hexColorPattern = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// UpdateEntityTypeColor validates and persists a new color for an entity type.
func (s *entityService) UpdateEntityTypeColor(ctx context.Context, id int, color string) error {
	color = strings.TrimSpace(color)
	if !hexColorPattern.MatchString(color) {
		return apperror.NewBadRequest("color must be a valid hex value like #ff0000")
	}
	return s.types.UpdateColor(ctx, id, color)
}

// UpdateEntityTypeDashboard updates the category dashboard description and
// pinned page IDs for an entity type.
func (s *entityService) UpdateEntityTypeDashboard(ctx context.Context, id int, description *string, pinnedIDs []string) error {
	if pinnedIDs == nil {
		pinnedIDs = []string{}
	}
	return s.types.UpdateDashboard(ctx, id, description, pinnedIDs)
}

// GetCategoryDashboardLayout returns the raw dashboard_layout JSON for an
// entity type. Returns nil when the type uses the default layout.
func (s *entityService) GetCategoryDashboardLayout(ctx context.Context, id int) (*string, error) {
	et, err := s.types.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return et.DashboardLayout, nil
}

// UpdateCategoryDashboardLayout validates and saves a custom dashboard layout
// for an entity type. Same validation rules as campaign dashboard layouts.
func (s *entityService) UpdateCategoryDashboardLayout(ctx context.Context, id int, layoutJSON string) error {
	var layout campaigns.DashboardLayout
	if err := json.Unmarshal([]byte(layoutJSON), &layout); err != nil {
		return apperror.NewBadRequest("invalid dashboard layout JSON")
	}

	// Validate structure.
	if len(layout.Rows) > 50 {
		return apperror.NewBadRequest("dashboard layout cannot exceed 50 rows")
	}
	for _, row := range layout.Rows {
		blockCount := 0
		for _, col := range row.Columns {
			if col.Width < 1 || col.Width > 12 {
				return apperror.NewBadRequest("column width must be between 1 and 12")
			}
			blockCount += len(col.Blocks)
			for _, block := range col.Blocks {
				if !campaigns.ValidBlockTypes[block.Type] {
					return apperror.NewBadRequest(fmt.Sprintf("invalid block type: %s", block.Type))
				}
			}
		}
		if blockCount > 20 {
			return apperror.NewBadRequest("a row cannot have more than 20 blocks")
		}
	}

	return s.types.UpdateDashboardLayout(ctx, id, &layoutJSON)
}

// ResetCategoryDashboardLayout removes the custom dashboard layout for an
// entity type, reverting it to the hardcoded default.
func (s *entityService) ResetCategoryDashboardLayout(ctx context.Context, id int) error {
	return s.types.UpdateDashboardLayout(ctx, id, nil)
}

// --- Seeder ---

// SeedDefaults seeds the default entity types for a campaign. This method
// satisfies the campaigns.EntityTypeSeeder interface.
func (s *entityService) SeedDefaults(ctx context.Context, campaignID string) error {
	return s.types.SeedDefaults(ctx, campaignID)
}

// --- Helpers ---

// maxSlugAttempts caps slug deduplication iterations to prevent DoS from
// adversarial name collisions (e.g., creating "test", "test-2" ... "test-N").
const maxSlugAttempts = 100

// generateSlug creates a unique slug for an entity within a campaign.
// If the base slug is taken, appends -2, -3, etc. After maxSlugAttempts,
// falls back to a random suffix.
func (s *entityService) generateSlug(ctx context.Context, campaignID, name string) (string, error) {
	base := Slugify(name)
	slug := base

	for i := 2; i < maxSlugAttempts+2; i++ {
		exists, err := s.entities.SlugExists(ctx, campaignID, slug)
		if err != nil {
			return "", fmt.Errorf("checking slug: %w", err)
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}

	// Fallback: append random suffix to guarantee uniqueness.
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random slug suffix: %w", err)
	}
	return fmt.Sprintf("%s-%s", base, hex.EncodeToString(b)), nil
}

// generateUUID creates a new v4 UUID string using crypto/rand.
// Panics if the system entropy source fails, as this indicates a
// catastrophic system problem that would compromise all security.
func generateUUID() string {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
