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
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// EntityService handles business logic for entity operations.
// It owns slug generation, privacy enforcement, and entity type seeding.
// Also implements the campaigns.EntityTypeSeeder interface.
type EntityService interface {
	// Entity CRUD
	Create(ctx context.Context, campaignID, userID string, input CreateEntityInput) (*Entity, error)
	Clone(ctx context.Context, campaignID, userID, sourceEntityID string) (*Entity, error)
	GetByID(ctx context.Context, id string) (*Entity, error)
	GetBySlug(ctx context.Context, campaignID, slug string) (*Entity, error)
	Update(ctx context.Context, entityID string, input UpdateEntityInput) (*Entity, error)
	UpdateEntry(ctx context.Context, entityID, entryJSON, entryHTML string) error
	UpdatePlayerNotes(ctx context.Context, entityID, notesJSON, notesHTML string) error
	UpdateFields(ctx context.Context, entityID string, fieldsData map[string]any) error
	UpdateFieldOverrides(ctx context.Context, entityID string, overrides *FieldOverrides) error
	UpdateImage(ctx context.Context, entityID, imagePath string) error
	UpdateCoverImage(ctx context.Context, entityID, coverImagePath string) error
	Delete(ctx context.Context, entityID string) error

	// Hierarchy
	GetChildren(ctx context.Context, entityID string, role int, userID string) ([]Entity, error)
	GetAncestors(ctx context.Context, entityID string) ([]Entity, error)
	ReorderEntity(ctx context.Context, campaignID, entityID string, parentID *string, parentNodeID *string, sortOrder int) error

	// Backlinks
	GetBacklinks(ctx context.Context, entityID string, role int, userID string) ([]Entity, error)

	// Popup preview config
	UpdatePopupConfig(ctx context.Context, entityID string, config *PopupConfig) error

	// Listing and search
	List(ctx context.Context, campaignID string, typeID int, role int, userID string, opts ListOptions) ([]Entity, int, error)
	ListRecent(ctx context.Context, campaignID string, role int, userID string, limit int) ([]Entity, error)
	Search(ctx context.Context, campaignID, query string, typeID int, role int, userID string, opts ListOptions) ([]Entity, int, error)

	// Entity types
	GetEntityTypes(ctx context.Context, campaignID string) ([]EntityType, error)
	GetEntityTypeBySlug(ctx context.Context, campaignID, slug string) (*EntityType, error)
	GetEntityTypeByID(ctx context.Context, id int) (*EntityType, error)
	GetEntityTypesByPresetCategory(ctx context.Context, campaignID, category string) ([]EntityType, error)
	CountByType(ctx context.Context, campaignID string, role int, userID string) (map[int]int, error)

	// Per-entity permissions
	GetEntityPermissions(ctx context.Context, entityID string) ([]EntityPermission, error)
	SetEntityPermissions(ctx context.Context, entityID string, input SetPermissionsInput) error
	CheckEntityAccess(ctx context.Context, entityID string, role int, userID string) (*EffectivePermission, error)
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

	// Auto-linking — returns lightweight name entries for all visible entities.
	ListEntityNames(ctx context.Context, campaignID string, role int, userID string) ([]EntityNameEntry, error)

	// Entity aliases — alternative names for auto-linking, search, and mentions.
	GetAliases(ctx context.Context, entityID string) ([]EntityAlias, error)
	SetAliases(ctx context.Context, entityID string, aliases []string) error

	// Backlinks with context snippets.
	GetBacklinksWithSnippets(ctx context.Context, entityID string, role int, userID string) ([]BacklinkEntry, error)

	// GetMentionLinks returns all @mention references across a campaign for the
	// relations graph. Each link is a source→target pair extracted from entry_html.
	GetMentionLinks(ctx context.Context, campaignID string, role int, userID string) ([]MentionLink, error)

	// TogglePrivate flips an entity's is_private flag and returns the new state.
	// Used by the NPC gallery reveal toggle.
	TogglePrivate(ctx context.Context, entityID string) (newPrivate bool, err error)

	// BulkUpdateType changes the entity type for multiple entities at once.
	// Returns the count of successfully updated entities. Validates that the
	// target type belongs to the campaign and each entity is campaign-scoped.
	BulkUpdateType(ctx context.Context, campaignID string, entityIDs []string, typeID int) (int, error)

	// Seeder (satisfies campaigns.EntityTypeSeeder interface).
	SeedDefaults(ctx context.Context, campaignID string) error
	SeedGenre(ctx context.Context, campaignID string, genre string) error

	// Wiring.
	SetEventPublisher(pub EntityEventPublisher)
	SetBlockRegistry(reg *BlockRegistry)
}

// EntityEventPublisher emits domain events when entities or entity types change.
// Implemented by the WebSocket EventBus adapter in routes.go.
type EntityEventPublisher interface {
	PublishEntityEvent(eventType, campaignID, entityID string, entity *Entity)
	PublishEntityTypeEvent(eventType, campaignID string, entityType *EntityType)
}

// NoopEntityEventPublisher is a no-op implementation for tests.
type NoopEntityEventPublisher struct{}

func (NoopEntityEventPublisher) PublishEntityEvent(string, string, string, *Entity)  {}
func (NoopEntityEventPublisher) PublishEntityTypeEvent(string, string, *EntityType)  {}

// entityService implements EntityService.
type entityService struct {
	entities      EntityRepository
	types         EntityTypeRepository
	permissions   EntityPermissionRepository
	events        EntityEventPublisher
	blockRegistry *BlockRegistry
}

// NewEntityService creates a new entity service with the given dependencies.
func NewEntityService(entities EntityRepository, types EntityTypeRepository, permissions EntityPermissionRepository) EntityService {
	return &entityService{
		entities:    entities,
		types:       types,
		permissions: permissions,
		events:      NoopEntityEventPublisher{},
	}
}

// SetEventPublisher sets the event publisher for real-time sync.
func (s *entityService) SetEventPublisher(pub EntityEventPublisher) {
	s.events = pub
}

// SetBlockRegistry sets the block registry for layout validation.
// Called after all plugins have registered their block types.
func (s *entityService) SetBlockRegistry(reg *BlockRegistry) {
	s.blockRegistry = reg
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

	s.events.PublishEntityEvent("created", campaignID, entity.ID, entity)
	return entity, nil
}

// Clone creates a copy of an existing entity. Copies name (with " (Copy)" suffix),
// entry, image, parent, privacy, fields, field overrides, popup config, and tags.
// Does NOT copy relations (they reference other entities and shouldn't be duplicated).
func (s *entityService) Clone(ctx context.Context, campaignID, userID, sourceEntityID string) (*Entity, error) {
	source, err := s.entities.FindByID(ctx, sourceEntityID)
	if err != nil {
		return nil, apperror.NewNotFound("source entity not found")
	}
	if source.CampaignID != campaignID {
		return nil, apperror.NewNotFound("source entity not found")
	}

	// Generate name and unique slug for the clone.
	cloneName := source.Name + " (Copy)"
	if len(cloneName) > 200 {
		cloneName = cloneName[:200]
	}
	slug, err := s.generateSlug(ctx, campaignID, cloneName)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("generating slug for clone: %w", err))
	}

	now := time.Now().UTC()
	fieldsData := source.FieldsData
	if fieldsData == nil {
		fieldsData = make(map[string]any)
	}

	clone := &Entity{
		ID:             generateUUID(),
		CampaignID:     campaignID,
		EntityTypeID:   source.EntityTypeID,
		Name:           cloneName,
		Slug:           slug,
		Entry:          source.Entry,
		EntryHTML:      source.EntryHTML,
		ImagePath:      source.ImagePath,
		ParentID:       source.ParentID,
		TypeLabel:      source.TypeLabel,
		IsPrivate:      source.IsPrivate,
		IsTemplate:     source.IsTemplate,
		FieldsData:     fieldsData,
		FieldOverrides: source.FieldOverrides,
		PopupConfig:    source.PopupConfig,
		CreatedBy:      userID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.entities.Create(ctx, clone); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating cloned entity: %w", err))
	}

	// Copy tags from source to clone.
	if err := s.entities.CopyEntityTags(ctx, sourceEntityID, clone.ID); err != nil {
		slog.Warn("failed to copy tags during clone", slog.String("source", sourceEntityID), slog.String("clone", clone.ID), slog.Any("error", err))
	}

	slog.Info("entity cloned",
		slog.String("source_id", sourceEntityID),
		slog.String("clone_id", clone.ID),
		slog.String("campaign_id", campaignID),
		slog.String("name", cloneName),
	)

	return clone, nil
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
// If ExpectedUpdatedAt is set, the update is rejected with 409 Conflict if the
// entity has been modified since that timestamp (optimistic concurrency control).
func (s *entityService) Update(ctx context.Context, entityID string, input UpdateEntityInput) (*Entity, error) {
	entity, err := s.entities.FindByID(ctx, entityID)
	if err != nil {
		return nil, err
	}

	// Optimistic concurrency check: reject if the entity was modified after
	// the caller's last-known version.
	if input.ExpectedUpdatedAt != nil {
		if entity.UpdatedAt.After(*input.ExpectedUpdatedAt) {
			return nil, apperror.NewConflict("entity was modified by another user; refresh and retry")
		}
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

	// Update player-facing notes if provided.
	if input.PlayerNotes != nil {
		pn := strings.TrimSpace(*input.PlayerNotes)
		if pn == "" {
			entity.PlayerNotes = nil
			entity.PlayerNotesHTML = nil
		} else {
			entity.PlayerNotes = &pn
			sanitized := sanitize.HTML(pn)
			entity.PlayerNotesHTML = &sanitized
		}
	}

	if input.FieldsData != nil {
		entity.FieldsData = input.FieldsData
	}

	entity.UpdatedAt = time.Now().UTC()

	if err := s.entities.Update(ctx, entity); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("updating entity: %w", err))
	}

	s.events.PublishEntityEvent("updated", entity.CampaignID, entity.ID, entity)
	return entity, nil
}

// --- Hierarchy ---

// GetChildren returns the direct children of an entity, respecting visibility.
func (s *entityService) GetChildren(ctx context.Context, entityID string, role int, userID string) ([]Entity, error) {
	children, err := s.entities.FindChildren(ctx, entityID, role, userID)
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

// ReorderEntity updates an entity's parent and sort order for sidebar tree
// reordering and reparenting. An entity can be parented under either another
// entity (parentID) or a sidebar folder node (parentNodeID), but not both.
// When one is set, the other is automatically cleared.
// Validates campaign ownership and prevents self-parenting.
func (s *entityService) ReorderEntity(ctx context.Context, campaignID, entityID string, parentID *string, parentNodeID *string, sortOrder int) error {
	// Prevent self-parenting.
	if parentID != nil && *parentID == entityID {
		return apperror.NewBadRequest("an entity cannot be its own parent")
	}

	// Verify the entity exists and belongs to this campaign.
	entity, err := s.entities.FindByID(ctx, entityID)
	if err != nil {
		return apperror.NewNotFound("entity not found")
	}
	if entity.CampaignID != campaignID {
		return apperror.NewNotFound("entity not found")
	}

	// If a parent entity is specified, verify it exists in the same campaign.
	if parentID != nil {
		parent, err := s.entities.FindByID(ctx, *parentID)
		if err != nil {
			return apperror.NewBadRequest("parent entity not found")
		}
		if parent.CampaignID != campaignID {
			return apperror.NewBadRequest("parent entity not found")
		}
	}

	// An entity can be parented under an entity (parent_id) or a folder
	// node (parent_node_id), but not both. When one is set, clear the other.
	if parentNodeID != nil {
		// Reparenting under a folder node — clear entity parent.
		if err := s.entities.UpdateParent(ctx, entityID, campaignID, nil); err != nil {
			return err
		}
		if err := s.entities.UpdateParentNode(ctx, entityID, campaignID, parentNodeID); err != nil {
			return err
		}
	} else {
		// Reparenting under an entity or root — clear folder node parent.
		if err := s.entities.UpdateParentNode(ctx, entityID, campaignID, nil); err != nil {
			return err
		}
		if err := s.entities.UpdateParent(ctx, entityID, campaignID, parentID); err != nil {
			return err
		}
	}

	if err := s.entities.UpdateSortOrder(ctx, entityID, campaignID, sortOrder); err != nil {
		return err
	}
	return nil
}

// GetBacklinks returns entities that reference the given entity via @mention
// links in their entry content. Respects visibility filtering.
func (s *entityService) GetBacklinks(ctx context.Context, entityID string, role int, userID string) ([]Entity, error) {
	backlinks, err := s.entities.FindBacklinks(ctx, entityID, role, userID)
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

	// Build search_text from sanitized HTML + existing field values.
	var fieldsData map[string]any
	if entity, err := s.entities.FindByID(ctx, entityID); err == nil {
		fieldsData = entity.FieldsData
	}
	searchText := buildSearchText(entryHTML, fieldsData)

	if err := s.entities.UpdateEntry(ctx, entityID, entryJSON, entryHTML, searchText); err != nil {
		return err
	}
	slog.Info("entity entry updated", slog.String("entity_id", entityID))
	// Emit entity updated event (fetch entity for campaign ID).
	if entity, err := s.entities.FindByID(ctx, entityID); err == nil {
		s.events.PublishEntityEvent("updated", entity.CampaignID, entityID, entity)
	}
	return nil
}

// UpdatePlayerNotes updates only the player-facing notes content for an entity.
// Used by the Foundry VTT sync module to set content visible to players.
func (s *entityService) UpdatePlayerNotes(ctx context.Context, entityID, notesJSON, notesHTML string) error {
	// Sanitize HTML to strip dangerous content.
	notesHTML = sanitize.HTML(notesHTML)
	if err := s.entities.UpdatePlayerNotes(ctx, entityID, notesJSON, notesHTML); err != nil {
		return err
	}
	slog.Info("entity player notes updated", slog.String("entity_id", entityID))
	// Emit entity updated event.
	if entity, err := s.entities.FindByID(ctx, entityID); err == nil {
		s.events.PublishEntityEvent("updated", entity.CampaignID, entityID, entity)
	}
	return nil
}

// UpdateFields updates only the entity's custom field values. Used by the
// attributes widget to persist inline field edits.
func (s *entityService) UpdateFields(ctx context.Context, entityID string, fieldsData map[string]any) error {
	if fieldsData == nil {
		fieldsData = make(map[string]any)
	}

	// Build search_text from existing entry HTML + new field values.
	var entryHTML string
	if entity, err := s.entities.FindByID(ctx, entityID); err == nil && entity.EntryHTML != nil {
		entryHTML = *entity.EntryHTML
	}
	searchText := buildSearchText(entryHTML, fieldsData)

	if err := s.entities.UpdateFields(ctx, entityID, fieldsData, searchText); err != nil {
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

// UpdateCoverImage updates the cover/banner image for an entity.
func (s *entityService) UpdateCoverImage(ctx context.Context, entityID, coverImagePath string) error {
	if coverImagePath != "" {
		if strings.HasPrefix(coverImagePath, "/") || strings.Contains(coverImagePath, "..") {
			return apperror.NewBadRequest("invalid cover image path")
		}
	}
	if err := s.entities.UpdateCoverImage(ctx, entityID, coverImagePath); err != nil {
		return err
	}
	slog.Info("entity cover image updated",
		slog.String("entity_id", entityID),
		slog.String("cover_image_path", coverImagePath),
	)
	return nil
}

// Delete removes an entity.
func (s *entityService) Delete(ctx context.Context, entityID string) error {
	// Fetch entity before deletion to get campaign ID for event publishing.
	entity, _ := s.entities.FindByID(ctx, entityID)

	if err := s.entities.Delete(ctx, entityID); err != nil {
		return err
	}
	slog.Info("entity deleted", slog.String("entity_id", entityID))

	if entity != nil {
		s.events.PublishEntityEvent("deleted", entity.CampaignID, entityID, entity)
	}
	return nil
}

// TogglePrivate flips an entity's is_private flag and returns the new state.
func (s *entityService) TogglePrivate(ctx context.Context, entityID string) (bool, error) {
	entity, err := s.entities.FindByID(ctx, entityID)
	if err != nil {
		return false, err
	}

	newPrivate := !entity.IsPrivate
	if err := s.entities.UpdatePrivate(ctx, entityID, newPrivate); err != nil {
		return false, err
	}

	// Publish event so WebSocket clients see the visibility change.
	entity.IsPrivate = newPrivate
	s.events.PublishEntityEvent("updated", entity.CampaignID, entityID, entity)

	return newPrivate, nil
}

// BulkUpdateType changes the entity type for multiple entities. Validates
// the target type belongs to the campaign, verifies each entity is
// campaign-scoped, and publishes update events for WebSocket sync.
func (s *entityService) BulkUpdateType(ctx context.Context, campaignID string, entityIDs []string, typeID int) (int, error) {
	// Validate target entity type belongs to the campaign.
	et, err := s.types.FindByID(ctx, typeID)
	if err != nil {
		return 0, apperror.NewBadRequest("entity type not found")
	}
	if et.CampaignID != campaignID {
		return 0, apperror.NewBadRequest("entity type does not belong to this campaign")
	}

	updated := 0
	for _, entityID := range entityIDs {
		entity, err := s.entities.FindByID(ctx, entityID)
		if err != nil || entity.CampaignID != campaignID {
			continue // Skip entities that don't exist or belong to another campaign.
		}

		if err := s.entities.UpdateEntityType(ctx, entityID, typeID); err != nil {
			slog.Warn("bulk update type: failed to update entity",
				slog.String("entity_id", entityID),
				slog.Any("error", err),
			)
			continue
		}

		entity.EntityTypeID = typeID
		s.events.PublishEntityEvent("updated", campaignID, entityID, entity)
		updated++
	}

	return updated, nil
}

// --- Listing and Search ---

// List returns entities with pagination, optional type filter, and visibility enforcement.
func (s *entityService) List(ctx context.Context, campaignID string, typeID int, role int, userID string, opts ListOptions) ([]Entity, int, error) {
	if opts.PerPage < 1 || opts.PerPage > 100 {
		opts.PerPage = 24
	}
	if opts.Page < 1 {
		opts.Page = 1
	}
	return s.entities.ListByCampaign(ctx, campaignID, typeID, role, userID, opts)
}

// ListRecent returns the most recently updated entities for a campaign dashboard.
func (s *entityService) ListRecent(ctx context.Context, campaignID string, role int, userID string, limit int) ([]Entity, error) {
	if limit < 1 || limit > 20 {
		limit = 8
	}
	return s.entities.ListRecent(ctx, campaignID, role, userID, limit)
}

// Search performs a text search on entity names with a minimum query length.
func (s *entityService) Search(ctx context.Context, campaignID, query string, typeID int, role int, userID string, opts ListOptions) ([]Entity, int, error) {
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
	return s.entities.Search(ctx, campaignID, q, typeID, role, userID, opts)
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

// GetEntityTypesByPresetCategory returns entity types created from a system
// preset with the given category (e.g., "item" for armory items).
func (s *entityService) GetEntityTypesByPresetCategory(ctx context.Context, campaignID, category string) ([]EntityType, error) {
	return s.types.ListByPresetCategory(ctx, campaignID, category)
}

// GetEntityTypeByID returns an entity type by its auto-increment ID.
func (s *entityService) GetEntityTypeByID(ctx context.Context, id int) (*EntityType, error) {
	return s.types.FindByID(ctx, id)
}

// CountByType returns entity counts per entity type for sidebar badges.
func (s *entityService) CountByType(ctx context.Context, campaignID string, role int, userID string) (map[int]int, error) {
	return s.entities.CountByType(ctx, campaignID, role, userID)
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

	// Set preset category if provided (from system manifest presets).
	var presetCategory *string
	if input.PresetCategory != "" {
		presetCategory = &input.PresetCategory
	}

	// Validate parent type if provided (sub-type hierarchy).
	if input.ParentTypeID != nil {
		parent, err := s.types.FindByID(ctx, *input.ParentTypeID)
		if err != nil {
			return nil, apperror.NewBadRequest("parent entity type not found")
		}
		if parent.CampaignID != campaignID {
			return nil, apperror.NewBadRequest("parent entity type belongs to a different campaign")
		}
		// Prevent deep nesting: parent must be a top-level type.
		if parent.ParentTypeID != nil {
			return nil, apperror.NewBadRequest("sub-types cannot be nested more than one level deep")
		}
	}

	et := &EntityType{
		CampaignID:     campaignID,
		Slug:           slug,
		Name:           name,
		NamePlural:     namePlural,
		Icon:           icon,
		Color:          color,
		PresetCategory: presetCategory,
		ParentTypeID:   input.ParentTypeID,
		Fields:         []FieldDefinition{},
		Layout:         DefaultLayout(),
		SortOrder:      maxOrder + 1,
		IsDefault:      false,
		Enabled:        true,
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

	s.events.PublishEntityTypeEvent("created", campaignID, et)
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

	s.events.PublishEntityTypeEvent("updated", et.CampaignID, et)
	return et, nil
}

// DeleteEntityType removes an entity type if no entities reference it.
func (s *entityService) DeleteEntityType(ctx context.Context, id int) error {
	et, err := s.types.FindByID(ctx, id)
	if err != nil {
		return err
	}

	// Check if any entities use this type. Count with Owner role (3) to include
	// all entities regardless of visibility.
	counts, err := s.entities.CountByType(ctx, et.CampaignID, 3, "")
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

	s.events.PublishEntityTypeEvent("deleted", et.CampaignID, et)
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

// isValidBlockType checks the block registry for allowed block types.
// Falls back to a minimal hardcoded set if no registry is configured (tests).
func (s *entityService) isValidBlockType(blockType string) bool {
	if s.blockRegistry != nil {
		return s.blockRegistry.IsValid(blockType)
	}
	// Fallback for tests that don't wire a registry.
	return defaultBlockTypes[blockType]
}

// defaultBlockTypes is used when no block registry is set (e.g., unit tests).
var defaultBlockTypes = map[string]bool{
	"title": true, "image": true, "entry": true,
	"attributes": true, "details": true, "divider": true,
	"posts": true, "tags": true, "relations": true,
	"shop_inventory": true, "inventory": true, "text_block": true,
}

// maxLayoutSize is the maximum allowed size for serialized layout JSON (100KB).
const maxLayoutSize = 102400

// ValidateLayout checks that a layout conforms to structural constraints.
// The optional isValidBlock callback validates block types; if nil, block type
// validation is skipped (useful for layout presets where block availability is
// campaign-dependent).
func ValidateLayout(layout EntityTypeLayout, isValidBlock func(string) bool) error {
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
				if isValidBlock != nil && !isValidBlock(blk.Type) {
					return apperror.NewBadRequest("invalid block type: " + blk.Type)
				}
			}
		}
		if totalWidth != gridWidth {
			return apperror.NewBadRequest("column widths in a row must sum to 12")
		}
	}
	return nil
}

// UpdateEntityTypeLayout validates and persists a new layout for an entity type.
// Accepts the new row/column/block format.
func (s *entityService) UpdateEntityTypeLayout(ctx context.Context, id int, layout EntityTypeLayout) error {
	if err := ValidateLayout(layout, s.isValidBlockType); err != nil {
		return err
	}

	layoutJSON, err := json.Marshal(layout)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling layout: %w", err))
	}

	if len(layoutJSON) > maxLayoutSize {
		return apperror.NewBadRequest("layout exceeds maximum size")
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

// --- Per-Entity Permissions ---

// GetEntityPermissions returns all permission grants for an entity.
func (s *entityService) GetEntityPermissions(ctx context.Context, entityID string) ([]EntityPermission, error) {
	return s.permissions.ListByEntity(ctx, entityID)
}

// SetEntityPermissions replaces all permission grants and updates visibility mode.
// When visibility is "default", existing custom permissions are cleared.
// When visibility is "custom", the provided grants are validated and stored.
func (s *entityService) SetEntityPermissions(ctx context.Context, entityID string, input SetPermissionsInput) error {
	entity, err := s.entities.FindByID(ctx, entityID)
	if err != nil {
		return err
	}

	switch input.Visibility {
	case VisibilityDefault:
		// Switch to legacy mode: update is_private, clear custom permissions.
		entity.IsPrivate = input.IsPrivate
		entity.UpdatedAt = time.Now().UTC()
		if err := s.entities.Update(ctx, entity); err != nil {
			return apperror.NewInternal(fmt.Errorf("updating entity privacy: %w", err))
		}
		if err := s.permissions.DeleteByEntity(ctx, entityID); err != nil {
			return apperror.NewInternal(fmt.Errorf("clearing permissions: %w", err))
		}
		if err := s.permissions.UpdateVisibility(ctx, entityID, VisibilityDefault); err != nil {
			return apperror.NewInternal(fmt.Errorf("updating visibility mode: %w", err))
		}

	case VisibilityCustom:
		// Validate grants.
		for i, g := range input.Permissions {
			if !ValidSubjectType(g.SubjectType) {
				return apperror.NewBadRequest(fmt.Sprintf("permission %d: invalid subject type %q", i, g.SubjectType))
			}
			if !ValidPermission(g.Permission) {
				return apperror.NewBadRequest(fmt.Sprintf("permission %d: invalid permission %q", i, g.Permission))
			}
			if g.SubjectID == "" {
				return apperror.NewBadRequest(fmt.Sprintf("permission %d: subject_id is required", i))
			}
		}

		if err := s.permissions.SetPermissions(ctx, entityID, input.Permissions); err != nil {
			return apperror.NewInternal(fmt.Errorf("setting permissions: %w", err))
		}
		if err := s.permissions.UpdateVisibility(ctx, entityID, VisibilityCustom); err != nil {
			return apperror.NewInternal(fmt.Errorf("updating visibility mode: %w", err))
		}

	default:
		return apperror.NewBadRequest(fmt.Sprintf("invalid visibility mode %q", input.Visibility))
	}

	slog.Info("entity permissions updated",
		slog.String("entity_id", entityID),
		slog.String("visibility", string(input.Visibility)),
		slog.Int("grants", len(input.Permissions)),
	)
	return nil
}

// CheckEntityAccess resolves whether a user can view/edit a specific entity.
// For "default" visibility, uses the legacy is_private + role check.
// For "custom" visibility, queries entity_permissions.
// Owners always have full access.
func (s *entityService) CheckEntityAccess(ctx context.Context, entityID string, role int, userID string) (*EffectivePermission, error) {
	// Owners always have full access.
	if role >= permissions.RoleOwner {
		return &EffectivePermission{CanView: true, CanEdit: true}, nil
	}

	entity, err := s.entities.FindByID(ctx, entityID)
	if err != nil {
		return nil, err
	}

	if entity.Visibility == VisibilityCustom {
		return s.permissions.GetEffectivePermission(ctx, entityID, role, userID)
	}

	// Legacy default mode.
	ep := &EffectivePermission{}
	if entity.IsPrivate {
		// Only Scribe+ can see private entities in default mode.
		if role >= 2 {
			ep.CanView = true
			ep.CanEdit = true
		}
	} else {
		// Public entity: everyone can view, Scribe+ can edit.
		ep.CanView = true
		if role >= 2 {
			ep.CanEdit = true
		}
	}
	return ep, nil
}

// --- Seeder ---

// SeedDefaults seeds the default entity types for a campaign. This method
// satisfies the campaigns.EntityTypeSeeder interface.
func (s *entityService) SeedDefaults(ctx context.Context, campaignID string) error {
	return s.types.SeedDefaults(ctx, campaignID)
}

// SeedGenre seeds genre-specific entity types for a campaign. If the genre
// is empty or "standard", falls back to SeedDefaults. Otherwise uses the
// genre preset definitions from genre_presets.go.
func (s *entityService) SeedGenre(ctx context.Context, campaignID string, genre string) error {
	if genre == "" || genre == "standard" {
		return s.SeedDefaults(ctx, campaignID)
	}
	preset := FindGenrePreset(genre)
	if preset == nil {
		return s.SeedDefaults(ctx, campaignID)
	}
	return s.types.SeedFromTypes(ctx, campaignID, preset.Types)
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

// ListEntityNames returns lightweight name entries for all visible entities
// in a campaign, including aliases. Used by the auto-linking feature in the editor.
func (s *entityService) ListEntityNames(ctx context.Context, campaignID string, role int, userID string) ([]EntityNameEntry, error) {
	return s.entities.ListNames(ctx, campaignID, role, userID)
}

// GetAliases returns all aliases for a given entity.
func (s *entityService) GetAliases(ctx context.Context, entityID string) ([]EntityAlias, error) {
	aliases, err := s.entities.ListAliases(ctx, entityID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing aliases: %w", err))
	}
	return aliases, nil
}

// SetAliases replaces an entity's aliases. Validates count (max 10) and
// length (2-200 chars per alias). Trims whitespace and deduplicates.
func (s *entityService) SetAliases(ctx context.Context, entityID string, aliases []string) error {
	// Trim and deduplicate.
	seen := make(map[string]bool, len(aliases))
	cleaned := make([]string, 0, len(aliases))
	for _, a := range aliases {
		a = strings.TrimSpace(a)
		lower := strings.ToLower(a)
		if a == "" || seen[lower] {
			continue
		}
		seen[lower] = true
		cleaned = append(cleaned, a)
	}

	if len(cleaned) > MaxAliasesPerEntity {
		return apperror.NewBadRequest(fmt.Sprintf("maximum %d aliases allowed", MaxAliasesPerEntity))
	}
	for _, a := range cleaned {
		if len(a) < MinAliasLength || len(a) > MaxAliasLength {
			return apperror.NewBadRequest(fmt.Sprintf("alias must be %d-%d characters", MinAliasLength, MaxAliasLength))
		}
	}

	if err := s.entities.SetAliases(ctx, entityID, cleaned); err != nil {
		return apperror.NewInternal(fmt.Errorf("setting aliases: %w", err))
	}
	slog.Info("entity aliases updated", slog.String("entity_id", entityID), slog.Int("count", len(cleaned)))
	return nil
}

// GetBacklinksWithSnippets returns backlink entities with context snippets
// showing text around the @mention. Builds on the existing GetBacklinks by
// extracting snippets from each referencing entity's entry_html.
func (s *entityService) GetBacklinksWithSnippets(ctx context.Context, entityID string, role int, userID string) ([]BacklinkEntry, error) {
	backlinks, err := s.entities.FindBacklinks(ctx, entityID, role, userID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("finding backlinks: %w", err))
	}

	entries := make([]BacklinkEntry, 0, len(backlinks))
	for _, bl := range backlinks {
		snippet := extractMentionSnippet(bl.EntryHTML, entityID)
		entries = append(entries, BacklinkEntry{Entity: bl, Snippet: snippet})
	}
	return entries, nil
}

// GetMentionLinks returns all @mention references across a campaign. Delegates
// to the repository which scans entry_html for data-mention-id attributes.
func (s *entityService) GetMentionLinks(ctx context.Context, campaignID string, role int, userID string) ([]MentionLink, error) {
	links, err := s.entities.FindAllMentionLinks(ctx, campaignID, role, userID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("finding mention links: %w", err))
	}
	return links, nil
}

// extractMentionSnippet extracts a plain-text snippet around a mention link
// in HTML content. Returns ~120 chars of context surrounding the mention.
func extractMentionSnippet(html *string, entityID string) string {
	if html == nil || *html == "" {
		return ""
	}

	// Find the mention anchor in the HTML.
	needle := `data-mention-id="` + entityID + `"`
	idx := strings.Index(*html, needle)
	if idx < 0 {
		return ""
	}

	// Expand to surrounding context: take a window around the mention.
	const windowSize = 200
	start := idx - windowSize
	if start < 0 {
		start = 0
	}
	end := idx + windowSize
	if end > len(*html) {
		end = len(*html)
	}
	window := (*html)[start:end]

	// Strip HTML tags to get plain text.
	plain := stripHTMLTags(window)

	// Trim to ~120 chars at word boundaries.
	plain = strings.TrimSpace(plain)
	if len(plain) > 120 {
		// Find last space before limit.
		cut := strings.LastIndex(plain[:120], " ")
		if cut < 40 {
			cut = 120
		}
		plain = plain[:cut] + "…"
	}
	if start > 0 {
		plain = "…" + plain
	}
	return plain
}

// stripHTMLTags removes HTML tags from a string, leaving only text content.
var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

func stripHTMLTags(s string) string {
	return htmlTagRegex.ReplaceAllString(s, "")
}

// buildSearchText creates the denormalized search text for an entity by
// combining stripped HTML entry content with flattened custom field values.
// Used to populate the search_text column for FULLTEXT search.
func buildSearchText(entryHTML string, fieldsData map[string]any) string {
	var parts []string
	if entryHTML != "" {
		parts = append(parts, stripHTMLTags(entryHTML))
	}
	for _, v := range fieldsData {
		switch val := v.(type) {
		case string:
			if val != "" {
				parts = append(parts, val)
			}
		case float64:
			parts = append(parts, fmt.Sprintf("%g", val))
		}
	}
	return strings.Join(parts, " ")
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
