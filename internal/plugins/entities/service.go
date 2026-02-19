package entities

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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
	UpdateImage(ctx context.Context, entityID, imagePath string) error
	Delete(ctx context.Context, entityID string) error

	// Listing and search
	List(ctx context.Context, campaignID string, typeID int, role int, opts ListOptions) ([]Entity, int, error)
	Search(ctx context.Context, campaignID, query string, typeID int, role int, opts ListOptions) ([]Entity, int, error)

	// Entity types
	GetEntityTypes(ctx context.Context, campaignID string) ([]EntityType, error)
	GetEntityTypeBySlug(ctx context.Context, campaignID, slug string) (*EntityType, error)
	GetEntityTypeByID(ctx context.Context, id int) (*EntityType, error)
	CountByType(ctx context.Context, campaignID string, role int) (map[int]int, error)

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

	// Update entry content if provided.
	entry := strings.TrimSpace(input.Entry)
	if entry != "" {
		entity.Entry = &entry
		// TODO: Render entry JSON to HTML when editor widget is implemented.
		entity.EntryHTML = &entry
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

// UpdateEntry updates only the entry content for an entity. Used by the
// editor widget's autosave to persist content without a full entity update.
func (s *entityService) UpdateEntry(ctx context.Context, entityID, entryJSON, entryHTML string) error {
	if strings.TrimSpace(entryJSON) == "" {
		return apperror.NewBadRequest("entry content is required")
	}
	if err := s.entities.UpdateEntry(ctx, entityID, entryJSON, entryHTML); err != nil {
		return err
	}
	slog.Info("entity entry updated", slog.String("entity_id", entityID))
	return nil
}

// UpdateImage sets or clears the entity's header image path.
func (s *entityService) UpdateImage(ctx context.Context, entityID, imagePath string) error {
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

// --- Seeder ---

// SeedDefaults seeds the default entity types for a campaign. This method
// satisfies the campaigns.EntityTypeSeeder interface.
func (s *entityService) SeedDefaults(ctx context.Context, campaignID string) error {
	return s.types.SeedDefaults(ctx, campaignID)
}

// --- Helpers ---

// generateSlug creates a unique slug for an entity within a campaign.
// If the base slug is taken, appends -2, -3, etc.
func (s *entityService) generateSlug(ctx context.Context, campaignID, name string) (string, error) {
	base := Slugify(name)
	slug := base

	for i := 2; ; i++ {
		exists, err := s.entities.SlugExists(ctx, campaignID, slug)
		if err != nil {
			return "", fmt.Errorf("checking slug: %w", err)
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

// generateUUID creates a new v4 UUID string using crypto/rand.
func generateUUID() string {
	uuid := make([]byte, 16)
	_, _ = rand.Read(uuid)
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
