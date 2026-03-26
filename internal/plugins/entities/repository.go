package entities

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// --- EntityType Repository ---

// EntityTypeRepository defines the data access contract for entity type operations.
type EntityTypeRepository interface {
	Create(ctx context.Context, et *EntityType) error
	FindByID(ctx context.Context, id int) (*EntityType, error)
	FindBySlug(ctx context.Context, campaignID, slug string) (*EntityType, error)
	ListByCampaign(ctx context.Context, campaignID string) ([]EntityType, error)
	ListByPresetCategory(ctx context.Context, campaignID, category string) ([]EntityType, error)
	Update(ctx context.Context, et *EntityType) error
	Delete(ctx context.Context, id int) error
	UpdateLayout(ctx context.Context, id int, layoutJSON string) error
	UpdateColor(ctx context.Context, id int, color string) error
	UpdateDashboard(ctx context.Context, id int, description *string, pinnedIDs []string) error
	UpdateDashboardLayout(ctx context.Context, id int, layoutJSON *string) error
	SlugExists(ctx context.Context, campaignID, slug string) (bool, error)
	MaxSortOrder(ctx context.Context, campaignID string) (int, error)
	SeedDefaults(ctx context.Context, campaignID string) error
}

// entityTypeRepository implements EntityTypeRepository with MariaDB queries.
type entityTypeRepository struct {
	db *sql.DB
}

// NewEntityTypeRepository creates a new entity type repository.
func NewEntityTypeRepository(db *sql.DB) EntityTypeRepository {
	return &entityTypeRepository{db: db}
}

// Create inserts a new entity type row.
func (r *entityTypeRepository) Create(ctx context.Context, et *EntityType) error {
	fieldsJSON, err := json.Marshal(et.Fields)
	if err != nil {
		return fmt.Errorf("marshaling fields: %w", err)
	}
	layoutJSON, err := json.Marshal(et.Layout)
	if err != nil {
		return fmt.Errorf("marshaling layout: %w", err)
	}

	query := `INSERT INTO entity_types (campaign_id, slug, name, name_plural, icon, color, preset_category, fields, layout_json, sort_order, is_default, enabled)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := r.db.ExecContext(ctx, query,
		et.CampaignID, et.Slug, et.Name, et.NamePlural,
		et.Icon, et.Color, et.PresetCategory, fieldsJSON, layoutJSON, et.SortOrder,
		et.IsDefault, et.Enabled,
	)
	if err != nil {
		return fmt.Errorf("inserting entity type: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting entity type id: %w", err)
	}
	et.ID = int(id)
	return nil
}

// FindByID retrieves an entity type by its auto-increment ID.
func (r *entityTypeRepository) FindByID(ctx context.Context, id int) (*EntityType, error) {
	query := `SELECT id, campaign_id, slug, name, name_plural, icon, color,
	                 preset_category, description, pinned_entity_ids, dashboard_layout,
	                 fields, layout_json, sort_order, is_default, enabled
	          FROM entity_types WHERE id = ?`

	et := &EntityType{}
	var fieldsRaw, layoutRaw, pinnedRaw []byte
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&et.ID, &et.CampaignID, &et.Slug, &et.Name, &et.NamePlural,
		&et.Icon, &et.Color, &et.PresetCategory, &et.Description, &pinnedRaw, &et.DashboardLayout,
		&fieldsRaw, &layoutRaw, &et.SortOrder,
		&et.IsDefault, &et.Enabled,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("entity type not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying entity type by id: %w", err)
	}

	if err := json.Unmarshal(fieldsRaw, &et.Fields); err != nil {
		return nil, fmt.Errorf("unmarshaling entity type fields: %w", err)
	}
	et.Layout = ParseLayoutJSON(layoutRaw)
	if len(pinnedRaw) > 0 {
		if err := json.Unmarshal(pinnedRaw, &et.PinnedEntityIDs); err != nil {
			return nil, fmt.Errorf("unmarshaling pinned entity IDs: %w", err)
		}
	}
	return et, nil
}

// FindBySlug retrieves an entity type by campaign ID and slug.
func (r *entityTypeRepository) FindBySlug(ctx context.Context, campaignID, slug string) (*EntityType, error) {
	query := `SELECT id, campaign_id, slug, name, name_plural, icon, color,
	                 preset_category, description, pinned_entity_ids, dashboard_layout,
	                 fields, layout_json, sort_order, is_default, enabled
	          FROM entity_types WHERE campaign_id = ? AND slug = ?`

	et := &EntityType{}
	var fieldsRaw, layoutRaw, pinnedRaw []byte
	err := r.db.QueryRowContext(ctx, query, campaignID, slug).Scan(
		&et.ID, &et.CampaignID, &et.Slug, &et.Name, &et.NamePlural,
		&et.Icon, &et.Color, &et.PresetCategory, &et.Description, &pinnedRaw, &et.DashboardLayout,
		&fieldsRaw, &layoutRaw, &et.SortOrder,
		&et.IsDefault, &et.Enabled,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("entity type not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying entity type by slug: %w", err)
	}

	if err := json.Unmarshal(fieldsRaw, &et.Fields); err != nil {
		return nil, fmt.Errorf("unmarshaling entity type fields: %w", err)
	}
	et.Layout = ParseLayoutJSON(layoutRaw)
	if len(pinnedRaw) > 0 {
		if err := json.Unmarshal(pinnedRaw, &et.PinnedEntityIDs); err != nil {
			return nil, fmt.Errorf("unmarshaling pinned entity IDs: %w", err)
		}
	}
	return et, nil
}

// ListByCampaign returns all entity types for a campaign, ordered by sort_order.
func (r *entityTypeRepository) ListByCampaign(ctx context.Context, campaignID string) ([]EntityType, error) {
	query := `SELECT id, campaign_id, slug, name, name_plural, icon, color,
	                 preset_category, description, pinned_entity_ids, dashboard_layout,
	                 fields, layout_json, sort_order, is_default, enabled
	          FROM entity_types WHERE campaign_id = ? ORDER BY sort_order, name`

	rows, err := r.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing entity types: %w", err)
	}
	defer rows.Close()

	var types []EntityType
	for rows.Next() {
		var et EntityType
		var fieldsRaw, layoutRaw, pinnedRaw []byte
		if err := rows.Scan(
			&et.ID, &et.CampaignID, &et.Slug, &et.Name, &et.NamePlural,
			&et.Icon, &et.Color, &et.PresetCategory, &et.Description, &pinnedRaw, &et.DashboardLayout,
			&fieldsRaw, &layoutRaw, &et.SortOrder,
			&et.IsDefault, &et.Enabled,
		); err != nil {
			return nil, fmt.Errorf("scanning entity type row: %w", err)
		}
		if err := json.Unmarshal(fieldsRaw, &et.Fields); err != nil {
			return nil, fmt.Errorf("unmarshaling entity type fields: %w", err)
		}
		et.Layout = ParseLayoutJSON(layoutRaw)
		if len(pinnedRaw) > 0 {
			if err := json.Unmarshal(pinnedRaw, &et.PinnedEntityIDs); err != nil {
				return nil, fmt.Errorf("unmarshaling pinned entity IDs: %w", err)
			}
		}
		types = append(types, et)
	}
	return types, rows.Err()
}

// ListByPresetCategory returns entity types in a campaign that were created
// from a system preset with the given category (e.g., "item", "character").
func (r *entityTypeRepository) ListByPresetCategory(ctx context.Context, campaignID, category string) ([]EntityType, error) {
	query := `SELECT id, campaign_id, slug, name, name_plural, icon, color,
	                 preset_category, description, pinned_entity_ids, dashboard_layout,
	                 fields, layout_json, sort_order, is_default, enabled
	          FROM entity_types WHERE campaign_id = ? AND preset_category = ? ORDER BY sort_order, name`

	rows, err := r.db.QueryContext(ctx, query, campaignID, category)
	if err != nil {
		return nil, fmt.Errorf("listing entity types by preset category: %w", err)
	}
	defer rows.Close()

	var types []EntityType
	for rows.Next() {
		var et EntityType
		var fieldsRaw, layoutRaw, pinnedRaw []byte
		if err := rows.Scan(
			&et.ID, &et.CampaignID, &et.Slug, &et.Name, &et.NamePlural,
			&et.Icon, &et.Color, &et.PresetCategory, &et.Description, &pinnedRaw, &et.DashboardLayout,
			&fieldsRaw, &layoutRaw, &et.SortOrder,
			&et.IsDefault, &et.Enabled,
		); err != nil {
			return nil, fmt.Errorf("scanning entity type row: %w", err)
		}
		if err := json.Unmarshal(fieldsRaw, &et.Fields); err != nil {
			return nil, fmt.Errorf("unmarshaling entity type fields: %w", err)
		}
		et.Layout = ParseLayoutJSON(layoutRaw)
		if len(pinnedRaw) > 0 {
			if err := json.Unmarshal(pinnedRaw, &et.PinnedEntityIDs); err != nil {
				return nil, fmt.Errorf("unmarshaling pinned entity IDs: %w", err)
			}
		}
		types = append(types, et)
	}
	return types, rows.Err()
}

// UpdateLayout updates only the layout_json for an entity type. Used by the
// layout builder widget to persist layout changes.
func (r *entityTypeRepository) UpdateLayout(ctx context.Context, id int, layoutJSON string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE entity_types SET layout_json = ? WHERE id = ?`,
		layoutJSON, id,
	)
	if err != nil {
		return fmt.Errorf("updating entity type layout: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity type not found")
	}
	return nil
}

// UpdateColor updates only the color for an entity type. Used by the
// entity type settings widget to change the display color.
func (r *entityTypeRepository) UpdateColor(ctx context.Context, id int, color string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE entity_types SET color = ? WHERE id = ?`,
		color, id,
	)
	if err != nil {
		return fmt.Errorf("updating entity type color: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity type not found")
	}
	return nil
}

// UpdateDashboard updates the category dashboard fields (description and pinned
// entity IDs) for an entity type.
func (r *entityTypeRepository) UpdateDashboard(ctx context.Context, id int, description *string, pinnedIDs []string) error {
	pinnedJSON, err := json.Marshal(pinnedIDs)
	if err != nil {
		return fmt.Errorf("marshaling pinned IDs: %w", err)
	}

	result, err := r.db.ExecContext(ctx,
		`UPDATE entity_types SET description = ?, pinned_entity_ids = ? WHERE id = ?`,
		description, pinnedJSON, id,
	)
	if err != nil {
		return fmt.Errorf("updating entity type dashboard: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity type not found")
	}
	return nil
}

// UpdateDashboardLayout updates the dashboard_layout JSON for an entity type.
// Pass nil to reset to the default layout.
func (r *entityTypeRepository) UpdateDashboardLayout(ctx context.Context, id int, layoutJSON *string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE entity_types SET dashboard_layout = ? WHERE id = ?`,
		layoutJSON, id,
	)
	if err != nil {
		return fmt.Errorf("updating entity type dashboard layout: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity type not found")
	}
	return nil
}

// Update modifies an existing entity type's name, slug, icon, color, and fields.
func (r *entityTypeRepository) Update(ctx context.Context, et *EntityType) error {
	fieldsJSON, err := json.Marshal(et.Fields)
	if err != nil {
		return fmt.Errorf("marshaling fields: %w", err)
	}

	query := `UPDATE entity_types SET name = ?, name_plural = ?, slug = ?, icon = ?, color = ?, fields = ?
	          WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query,
		et.Name, et.NamePlural, et.Slug, et.Icon, et.Color, fieldsJSON, et.ID,
	)
	if err != nil {
		return fmt.Errorf("updating entity type: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity type not found")
	}
	return nil
}

// Delete removes an entity type by ID.
func (r *entityTypeRepository) Delete(ctx context.Context, id int) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM entity_types WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting entity type: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity type not found")
	}
	return nil
}

// SlugExists returns true if an entity type with the given slug exists in the campaign.
func (r *entityTypeRepository) SlugExists(ctx context.Context, campaignID, slug string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM entity_types WHERE campaign_id = ? AND slug = ?)`,
		campaignID, slug,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking entity type slug existence: %w", err)
	}
	return exists, nil
}

// MaxSortOrder returns the highest sort_order value for entity types in a campaign.
// Returns 0 if the campaign has no entity types.
func (r *entityTypeRepository) MaxSortOrder(ctx context.Context, campaignID string) (int, error) {
	var maxOrder sql.NullInt64
	err := r.db.QueryRowContext(ctx,
		`SELECT MAX(sort_order) FROM entity_types WHERE campaign_id = ?`,
		campaignID,
	).Scan(&maxOrder)
	if err != nil {
		return 0, fmt.Errorf("querying max sort order: %w", err)
	}
	if !maxOrder.Valid {
		return 0, nil
	}
	return int(maxOrder.Int64), nil
}

// defaultEntityTypes defines the entity types seeded when a campaign is created.
var defaultEntityTypes = []EntityType{
	{Slug: "character", Name: "Character", NamePlural: "Characters", Icon: "fa-user", Color: "#3b82f6", SortOrder: 1, IsDefault: true, Enabled: true,
		Fields: []FieldDefinition{
			{Key: "title", Label: "Title", Type: "text", Section: "Basics"},
			{Key: "age", Label: "Age", Type: "text", Section: "Basics"},
			{Key: "gender", Label: "Gender", Type: "text", Section: "Basics"},
			{Key: "race", Label: "Race", Type: "text", Section: "Basics"},
			{Key: "class", Label: "Class", Type: "text", Section: "Basics"},
		}},
	{Slug: "location", Name: "Location", NamePlural: "Locations", Icon: "fa-map-pin", Color: "#ef4444", SortOrder: 2, IsDefault: true, Enabled: true,
		Fields: []FieldDefinition{
			{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
			{Key: "population", Label: "Population", Type: "text", Section: "Basics"},
			{Key: "region", Label: "Region", Type: "text", Section: "Basics"},
		}},
	{Slug: "organization", Name: "Organization", NamePlural: "Organizations", Icon: "fa-building", Color: "#f59e0b", SortOrder: 3, IsDefault: true, Enabled: true,
		Fields: []FieldDefinition{
			{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
			{Key: "leader", Label: "Leader", Type: "text", Section: "Basics"},
			{Key: "headquarters", Label: "Headquarters", Type: "text", Section: "Basics"},
		}},
	{Slug: "item", Name: "Item", NamePlural: "Items", Icon: "fa-box", Color: "#8b5cf6", SortOrder: 4, IsDefault: true, Enabled: true,
		Fields: []FieldDefinition{
			{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
			{Key: "rarity", Label: "Rarity", Type: "text", Section: "Basics"},
			{Key: "weight", Label: "Weight", Type: "text", Section: "Basics"},
		}},
	{Slug: "note", Name: "Note", NamePlural: "Notes", Icon: "fa-sticky-note", Color: "#10b981", SortOrder: 5, IsDefault: true, Enabled: true,
		Fields: []FieldDefinition{}},
	{Slug: "event", Name: "Event", NamePlural: "Events", Icon: "fa-calendar", Color: "#ec4899", SortOrder: 6, IsDefault: true, Enabled: true,
		Fields: []FieldDefinition{
			{Key: "date", Label: "Date", Type: "text", Section: "Basics"},
			{Key: "location", Label: "Location", Type: "text", Section: "Basics"},
		}},
	{Slug: "shop", Name: "Shop", NamePlural: "Shops", Icon: "fa-store", Color: "#f97316", SortOrder: 7, IsDefault: true, Enabled: true,
		Fields: []FieldDefinition{
			{Key: "shop_type", Label: "Shop Type", Type: "select", Section: "Basics",
				Options: []string{"General Store", "Blacksmith", "Apothecary", "Magic Shop", "Tavern", "Armorer", "Jeweler", "Tailor", "Stable", "Other"}},
			{Key: "shop_keeper", Label: "Shopkeeper", Type: "text", Section: "Basics"},
			{Key: "currency", Label: "Currency", Type: "text", Section: "Basics"},
			{Key: "price_modifier", Label: "Price Modifier (%)", Type: "number", Section: "Basics"},
		}},
}

// SeedDefaults inserts the default entity types for a newly created campaign.
// Uses a transaction to ensure all-or-nothing insertion.
func (r *entityTypeRepository) SeedDefaults(ctx context.Context, campaignID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning seed tx: %w", err)
	}
	defer tx.Rollback()

	query := `INSERT INTO entity_types (campaign_id, slug, name, name_plural, icon, color, fields, layout_json, sort_order, is_default, enabled)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	for _, et := range defaultEntityTypes {
		fieldsJSON, err := json.Marshal(et.Fields)
		if err != nil {
			return fmt.Errorf("marshaling default fields for %s: %w", et.Slug, err)
		}
		layoutJSON, err := json.Marshal(et.Layout)
		if err != nil {
			return fmt.Errorf("marshaling default layout for %s: %w", et.Slug, err)
		}

		_, err = tx.ExecContext(ctx, query,
			campaignID, et.Slug, et.Name, et.NamePlural,
			et.Icon, et.Color, fieldsJSON, layoutJSON, et.SortOrder,
			et.IsDefault, et.Enabled,
		)
		if err != nil {
			return fmt.Errorf("seeding entity type %s: %w", et.Slug, err)
		}
	}

	return tx.Commit()
}

// --- Entity Repository ---

// EntityRepository defines the data access contract for entity operations.
type EntityRepository interface {
	Create(ctx context.Context, entity *Entity) error
	FindByID(ctx context.Context, id string) (*Entity, error)
	FindBySlug(ctx context.Context, campaignID, slug string) (*Entity, error)
	Update(ctx context.Context, entity *Entity) error
	UpdateEntry(ctx context.Context, id, entryJSON, entryHTML string) error
	UpdatePlayerNotes(ctx context.Context, id, notesJSON, notesHTML string) error
	UpdateFields(ctx context.Context, id string, fieldsData map[string]any) error
	UpdateFieldOverrides(ctx context.Context, id string, overrides *FieldOverrides) error
	UpdateImage(ctx context.Context, id, imagePath string) error
	UpdateCoverImage(ctx context.Context, id, coverImagePath string) error
	Delete(ctx context.Context, id string) error
	SlugExists(ctx context.Context, campaignID, slug string) (bool, error)

	// ListByCampaign returns entities filtered by campaign, optional type, and visibility.
	// Uses visibilityFilter to handle both legacy is_private and custom permissions.
	ListByCampaign(ctx context.Context, campaignID string, typeID int, role int, userID string, opts ListOptions) ([]Entity, int, error)

	// Search performs a FULLTEXT search on entity names with visibility filtering.
	Search(ctx context.Context, campaignID, query string, typeID int, role int, userID string, opts ListOptions) ([]Entity, int, error)

	// CountByType returns entity counts per type for the sidebar badges.
	CountByType(ctx context.Context, campaignID string, role int, userID string) (map[int]int, error)

	// ListRecent returns the N most recently updated entities for a campaign,
	// ordered by updated_at DESC. Used for the campaign dashboard "recent pages" section.
	ListRecent(ctx context.Context, campaignID string, role int, userID string, limit int) ([]Entity, error)

	// FindChildren returns direct children of an entity, respecting visibility.
	FindChildren(ctx context.Context, parentID string, role int, userID string) ([]Entity, error)

	// FindAncestors returns the ancestor chain from an entity up to the root,
	// ordered from immediate parent to furthest ancestor. Uses a recursive CTE.
	FindAncestors(ctx context.Context, entityID string) ([]Entity, error)

	// UpdateParent sets or clears an entity's parent_id. Scoped to campaign for safety.
	UpdateParent(ctx context.Context, entityID, campaignID string, parentID *string) error
	UpdateParentNode(ctx context.Context, entityID, campaignID string, parentNodeID *string) error // Set sidebar folder node parent.

	// UpdateSortOrder sets an entity's manual sort order within its parent/category.
	// Scoped to campaign for safety.
	UpdateSortOrder(ctx context.Context, entityID, campaignID string, sortOrder int) error

	// FindBacklinks returns entities whose entry_html contains a @mention link
	// pointing to the given entity. Respects visibility filtering.
	FindBacklinks(ctx context.Context, entityID string, role int, userID string) ([]Entity, error)

	// UpdatePopupConfig persists the entity's hover preview configuration.
	UpdatePopupConfig(ctx context.Context, entityID string, config *PopupConfig) error

	// CopyEntityTags copies all entity_tags associations from one entity to another.
	CopyEntityTags(ctx context.Context, sourceEntityID, targetEntityID string) error

	// ListNames returns lightweight name entries for all visible entities in a
	// campaign, including aliases. Used for auto-linking in the editor. Sorted
	// by name length DESC so longer names match first (prevents partial matches).
	ListNames(ctx context.Context, campaignID string, role int, userID string) ([]EntityNameEntry, error)

	// ListAliases returns all aliases for a given entity.
	ListAliases(ctx context.Context, entityID string) ([]EntityAlias, error)

	// SetAliases replaces all aliases for an entity with the given list.
	// Deletes existing aliases and inserts new ones in a single transaction.
	SetAliases(ctx context.Context, entityID string, aliases []string) error

	// FindAllMentionLinks returns all @mention references across a campaign by
	// scanning entry_html for data-mention-id attributes. Each result is a
	// source→target pair. Used by the graph visualization to show mention edges.
	FindAllMentionLinks(ctx context.Context, campaignID string, role int, userID string) ([]MentionLink, error)

	// UpdatePrivate sets an entity's is_private flag. Used by the NPC reveal toggle.
	UpdatePrivate(ctx context.Context, entityID string, isPrivate bool) error

	// UpdateEntityType changes an entity's entity_type_id. Used by bulk type reassignment.
	UpdateEntityType(ctx context.Context, entityID string, typeID int) error
}

// entityRepository implements EntityRepository with MariaDB queries.
type entityRepository struct {
	db *sql.DB
}

// NewEntityRepository creates a new entity repository.
func NewEntityRepository(db *sql.DB) EntityRepository {
	return &entityRepository{db: db}
}

// Create inserts a new entity row.
func (r *entityRepository) Create(ctx context.Context, entity *Entity) error {
	fieldsJSON, err := json.Marshal(entity.FieldsData)
	if err != nil {
		return fmt.Errorf("marshaling fields data: %w", err)
	}

	query := `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, entry, entry_html,
	          player_notes, player_notes_html,
	          image_path, parent_id, parent_node_id, sort_order, type_label, is_private, is_template, fields_data, created_by, created_at, updated_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = r.db.ExecContext(ctx, query,
		entity.ID, entity.CampaignID, entity.EntityTypeID,
		entity.Name, entity.Slug, entity.Entry, entity.EntryHTML,
		entity.PlayerNotes, entity.PlayerNotesHTML,
		entity.ImagePath, entity.ParentID, entity.ParentNodeID, entity.SortOrder, entity.TypeLabel,
		entity.IsPrivate, entity.IsTemplate, fieldsJSON,
		entity.CreatedBy, entity.CreatedAt, entity.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting entity: %w", err)
	}
	return nil
}

// entitySelectColumns is the standard column list for entity queries with joined type info.
const entitySelectColumns = `e.id, e.campaign_id, e.entity_type_id, e.name, e.slug,
	                 e.entry, e.entry_html, e.player_notes, e.player_notes_html,
	                 e.image_path, e.cover_image_path, e.parent_id, e.parent_node_id, e.sort_order, e.type_label,
	                 e.is_private, e.visibility, e.is_template, e.fields_data, e.field_overrides, e.popup_config,
	                 e.created_by, e.created_at, e.updated_at,
	                 et.name, et.icon, et.color, et.slug`

// FindByID retrieves an entity with joined type info.
func (r *entityRepository) FindByID(ctx context.Context, id string) (*Entity, error) {
	query := `SELECT ` + entitySelectColumns + `
	          FROM entities e
	          INNER JOIN entity_types et ON et.id = e.entity_type_id
	          WHERE e.id = ?`

	return r.scanEntity(r.db.QueryRowContext(ctx, query, id))
}

// FindBySlug retrieves an entity by campaign ID and slug with joined type info.
func (r *entityRepository) FindBySlug(ctx context.Context, campaignID, slug string) (*Entity, error) {
	query := `SELECT ` + entitySelectColumns + `
	          FROM entities e
	          INNER JOIN entity_types et ON et.id = e.entity_type_id
	          WHERE e.campaign_id = ? AND e.slug = ?`

	return r.scanEntity(r.db.QueryRowContext(ctx, query, campaignID, slug))
}

// scanEntity scans a single entity row with joined type fields.
// The column order must match entitySelectColumns.
func (r *entityRepository) scanEntity(row *sql.Row) (*Entity, error) {
	e := &Entity{}
	var fieldsRaw, overridesRaw, popupRaw []byte
	err := row.Scan(
		&e.ID, &e.CampaignID, &e.EntityTypeID, &e.Name, &e.Slug,
		&e.Entry, &e.EntryHTML, &e.PlayerNotes, &e.PlayerNotesHTML,
		&e.ImagePath, &e.CoverImagePath, &e.ParentID, &e.ParentNodeID, &e.SortOrder, &e.TypeLabel,
		&e.IsPrivate, &e.Visibility, &e.IsTemplate, &fieldsRaw, &overridesRaw, &popupRaw,
		&e.CreatedBy, &e.CreatedAt, &e.UpdatedAt,
		&e.TypeName, &e.TypeIcon, &e.TypeColor, &e.TypeSlug,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("entity not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scanning entity: %w", err)
	}

	e.FieldsData = make(map[string]any)
	if len(fieldsRaw) > 0 {
		if err := json.Unmarshal(fieldsRaw, &e.FieldsData); err != nil {
			return nil, fmt.Errorf("unmarshaling fields data: %w", err)
		}
	}
	if len(overridesRaw) > 0 {
		e.FieldOverrides = &FieldOverrides{}
		if err := json.Unmarshal(overridesRaw, e.FieldOverrides); err != nil {
			return nil, fmt.Errorf("unmarshaling field overrides: %w", err)
		}
	}
	if len(popupRaw) > 0 {
		e.PopupConfig = &PopupConfig{}
		if err := json.Unmarshal(popupRaw, e.PopupConfig); err != nil {
			return nil, fmt.Errorf("unmarshaling popup config: %w", err)
		}
	}
	return e, nil
}

// Update modifies an existing entity including parent_id.
func (r *entityRepository) Update(ctx context.Context, entity *Entity) error {
	fieldsJSON, err := json.Marshal(entity.FieldsData)
	if err != nil {
		return fmt.Errorf("marshaling fields data: %w", err)
	}

	query := `UPDATE entities SET name = ?, slug = ?, entry = ?, entry_html = ?,
	          player_notes = ?, player_notes_html = ?,
	          type_label = ?, parent_id = ?, sort_order = ?, is_private = ?, fields_data = ?, updated_at = ?
	          WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query,
		entity.Name, entity.Slug, entity.Entry, entity.EntryHTML,
		entity.PlayerNotes, entity.PlayerNotesHTML,
		entity.TypeLabel, entity.ParentID, entity.SortOrder, entity.IsPrivate, fieldsJSON, entity.UpdatedAt,
		entity.ID,
	)
	if err != nil {
		return fmt.Errorf("updating entity: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// UpdateEntry updates only the entry content (JSON + rendered HTML) for an entity.
// Used by the editor widget's autosave without touching other fields.
func (r *entityRepository) UpdateEntry(ctx context.Context, id, entryJSON, entryHTML string) error {
	query := `UPDATE entities SET entry = ?, entry_html = ?, updated_at = NOW() WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, entryJSON, entryHTML, id)
	if err != nil {
		return fmt.Errorf("updating entity entry: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// UpdatePlayerNotes updates only the player_notes content for an entity.
// Used by the Foundry VTT sync module to set player-facing content.
func (r *entityRepository) UpdatePlayerNotes(ctx context.Context, id, notesJSON, notesHTML string) error {
	query := `UPDATE entities SET player_notes = ?, player_notes_html = ?, updated_at = NOW() WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, notesJSON, notesHTML, id)
	if err != nil {
		return fmt.Errorf("updating entity player notes: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// UpdateFields updates only the fields_data JSON column for an entity.
// Used by the attributes widget to save individual field changes without
// touching other entity properties.
func (r *entityRepository) UpdateFields(ctx context.Context, id string, fieldsData map[string]any) error {
	fieldsJSON, err := json.Marshal(fieldsData)
	if err != nil {
		return fmt.Errorf("marshaling fields_data: %w", err)
	}

	query := `UPDATE entities SET fields_data = ?, updated_at = NOW() WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, string(fieldsJSON), id)
	if err != nil {
		return fmt.Errorf("updating entity fields: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// UpdateFieldOverrides updates the per-entity field overrides JSON column.
// Used by the attributes widget to save entity-specific field customizations.
func (r *entityRepository) UpdateFieldOverrides(ctx context.Context, id string, overrides *FieldOverrides) error {
	var overridesJSON any
	if overrides != nil {
		raw, err := json.Marshal(overrides)
		if err != nil {
			return fmt.Errorf("marshaling field_overrides: %w", err)
		}
		overridesJSON = string(raw)
	}

	query := `UPDATE entities SET field_overrides = ?, updated_at = NOW() WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, overridesJSON, id)
	if err != nil {
		return fmt.Errorf("updating entity field overrides: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// UpdateImage updates only the image_path for an entity. Used by the image
// upload API to set or clear an entity's header image.
func (r *entityRepository) UpdateImage(ctx context.Context, id, imagePath string) error {
	var imgVal any
	if imagePath != "" {
		imgVal = imagePath
	}

	query := `UPDATE entities SET image_path = ?, updated_at = NOW() WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, imgVal, id)
	if err != nil {
		return fmt.Errorf("updating entity image: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// UpdateCoverImage updates only the cover_image_path for an entity.
// Used by the cover image upload API.
func (r *entityRepository) UpdateCoverImage(ctx context.Context, id, coverImagePath string) error {
	var val any
	if coverImagePath != "" {
		val = coverImagePath
	}

	query := `UPDATE entities SET cover_image_path = ?, updated_at = NOW() WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, val, id)
	if err != nil {
		return fmt.Errorf("updating entity cover image: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// Delete removes an entity.
func (r *entityRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM entities WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting entity: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// SlugExists returns true if an entity with the given slug exists in the campaign.
func (r *entityRepository) SlugExists(ctx context.Context, campaignID, slug string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM entities WHERE campaign_id = ? AND slug = ?)`,
		campaignID, slug,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking slug existence: %w", err)
	}
	return exists, nil
}

// tagFilterClause returns a WHERE clause fragment and args that filter
// entities to only those having ALL the specified tags (AND logic).
// Uses a subquery with HAVING COUNT to ensure all tags match.
func tagFilterClause(tagSlugs []string) (string, []any) {
	if len(tagSlugs) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(tagSlugs))
	args := make([]any, len(tagSlugs))
	for i, slug := range tagSlugs {
		placeholders[i] = "?"
		args[i] = slug
	}
	clause := fmt.Sprintf(` AND e.id IN (
		SELECT et2.entity_id FROM entity_tags et2
		INNER JOIN tags t2 ON t2.id = et2.tag_id
		WHERE t2.slug IN (%s)
		GROUP BY et2.entity_id
		HAVING COUNT(DISTINCT t2.slug) = ?
	)`, strings.Join(placeholders, ","))
	args = append(args, len(tagSlugs))
	return clause, args
}

// visibilityFilter returns the WHERE clause fragment and args that enforce
// entity visibility based on the viewer's role, user ID, and the entity's
// visibility mode. Owners see everything — returns empty string.
//
// For non-owners, the filter handles two visibility modes:
//   - "default": uses the legacy is_private flag (Scribe+ sees all, Player sees public only)
//   - "custom": checks entity_permissions for explicit grants to the user or their role level
func visibilityFilter(role int, userID string) (string, []any) {
	if role >= permissions.RoleOwner {
		return "", nil
	}

	filter := ` AND (
		(e.visibility = 'default' AND (? >= 2 OR e.is_private = false))
		OR (e.visibility = 'custom' AND EXISTS (
			SELECT 1 FROM entity_permissions ep
			WHERE ep.entity_id = e.id
			AND (
				(ep.subject_type = 'role' AND CAST(ep.subject_id AS UNSIGNED) <= ?)
				OR (ep.subject_type = 'user' AND ep.subject_id = ?)
				OR (ep.subject_type = 'group' AND EXISTS (
					SELECT 1 FROM campaign_group_members cgm
					WHERE cgm.group_id = CAST(ep.subject_id AS UNSIGNED)
					AND cgm.user_id = ?
				))
			)
		))
	)`
	return filter, []any{role, role, userID, userID}
}

// ListByCampaign returns entities with pagination and optional type filtering.
// Visibility filtering considers both legacy is_private and custom permissions.
func (r *entityRepository) ListByCampaign(ctx context.Context, campaignID string, typeID int, role int, userID string, opts ListOptions) ([]Entity, int, error) {
	where := "WHERE e.campaign_id = ?"
	args := []any{campaignID}

	if typeID > 0 {
		where += " AND e.entity_type_id = ?"
		args = append(args, typeID)
	}

	// Tag filtering: entity must have ALL specified tags (AND logic).
	if len(opts.TagSlugs) > 0 {
		tagFilter, tagArgs := tagFilterClause(opts.TagSlugs)
		where += tagFilter
		args = append(args, tagArgs...)
	}

	visFilter, visArgs := visibilityFilter(role, userID)
	where += visFilter
	args = append(args, visArgs...)

	// Count total for pagination.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM entities e %s", where)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting entities: %w", err)
	}

	// Fetch page.
	query := fmt.Sprintf(`SELECT `+entitySelectColumns+`
	          FROM entities e
	          INNER JOIN entity_types et ON et.id = e.entity_type_id
	          %s
	          %s
	          LIMIT ? OFFSET ?`, where, opts.OrderByClause())

	pageArgs := append(args, opts.PerPage, opts.Offset())
	rows, err := r.db.QueryContext(ctx, query, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing entities: %w", err)
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		e, err := r.scanEntityRow(rows)
		if err != nil {
			return nil, 0, err
		}
		entities = append(entities, *e)
	}
	return entities, total, rows.Err()
}

// Search performs a text search on entity names with visibility filtering.
// Uses FULLTEXT for queries >= 4 chars, LIKE for shorter queries.
func (r *entityRepository) Search(ctx context.Context, campaignID, query string, typeID int, role int, userID string, opts ListOptions) ([]Entity, int, error) {
	where := "WHERE e.campaign_id = ?"
	args := []any{campaignID}

	// FULLTEXT for longer queries, LIKE for short ones.
	var nameCondition string
	var nameArg string
	if len(query) >= 4 {
		cleaned := stripFTOperators(query)
		nameCondition = "MATCH(e.name) AGAINST(? IN BOOLEAN MODE)"
		nameArg = cleaned + "*"
	} else {
		escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(query)
		nameCondition = "e.name LIKE ?"
		nameArg = "%" + escaped + "%"
	}

	// Match entity name OR any alias.
	aliasEscaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(query)
	where += fmt.Sprintf(` AND (%s OR e.id IN (
		SELECT ea.entity_id FROM entity_aliases ea WHERE ea.alias LIKE ?
	))`, nameCondition)
	args = append(args, nameArg, "%"+aliasEscaped+"%")

	if typeID > 0 {
		where += " AND e.entity_type_id = ?"
		args = append(args, typeID)
	}

	// Tag filtering: entity must have ALL specified tags (AND logic).
	if len(opts.TagSlugs) > 0 {
		tagFilter, tagArgs := tagFilterClause(opts.TagSlugs)
		where += tagFilter
		args = append(args, tagArgs...)
	}

	visFilter, visArgs := visibilityFilter(role, userID)
	where += visFilter
	args = append(args, visArgs...)

	// Count total.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM entities e %s", where)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting search results: %w", err)
	}

	// Fetch page.
	selectQuery := fmt.Sprintf(`SELECT `+entitySelectColumns+`
	          FROM entities e
	          INNER JOIN entity_types et ON et.id = e.entity_type_id
	          %s
	          ORDER BY e.name
	          LIMIT ? OFFSET ?`, where)

	pageArgs := append(args, opts.PerPage, opts.Offset())
	rows, err := r.db.QueryContext(ctx, selectQuery, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("searching entities: %w", err)
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		e, err := r.scanEntityRow(rows)
		if err != nil {
			return nil, 0, err
		}
		entities = append(entities, *e)
	}
	return entities, total, rows.Err()
}

// CountByType returns a map of entity_type_id → count for sidebar badges.
// Respects visibility filtering for non-owner roles.
func (r *entityRepository) CountByType(ctx context.Context, campaignID string, role int, userID string) (map[int]int, error) {
	// Use alias 'e' to match visibilityFilter expectations.
	query := `SELECT e.entity_type_id, COUNT(*) FROM entities e WHERE e.campaign_id = ?`
	args := []any{campaignID}

	visFilter, visArgs := visibilityFilter(role, userID)
	query += visFilter
	args = append(args, visArgs...)
	query += " GROUP BY e.entity_type_id"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("counting entities by type: %w", err)
	}
	defer rows.Close()

	counts := make(map[int]int)
	for rows.Next() {
		var typeID, count int
		if err := rows.Scan(&typeID, &count); err != nil {
			return nil, fmt.Errorf("scanning count row: %w", err)
		}
		counts[typeID] = count
	}
	return counts, rows.Err()
}

// ListRecent returns the most recently updated entities for a campaign,
// ordered by updated_at DESC. Respects visibility filtering based on role.
func (r *entityRepository) ListRecent(ctx context.Context, campaignID string, role int, userID string, limit int) ([]Entity, error) {
	where := "WHERE e.campaign_id = ?"
	args := []any{campaignID}

	visFilter, visArgs := visibilityFilter(role, userID)
	where += visFilter
	args = append(args, visArgs...)

	query := fmt.Sprintf(`SELECT `+entitySelectColumns+`
	          FROM entities e
	          INNER JOIN entity_types et ON et.id = e.entity_type_id
	          %s
	          ORDER BY e.updated_at DESC
	          LIMIT ?`, where)

	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing recent entities: %w", err)
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		e, err := r.scanEntityRow(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, *e)
	}
	return entities, rows.Err()
}

// FindChildren returns direct children of an entity, respecting visibility.
// Results are ordered alphabetically by name.
func (r *entityRepository) FindChildren(ctx context.Context, parentID string, role int, userID string) ([]Entity, error) {
	where := "WHERE e.parent_id = ?"
	args := []any{parentID}

	visFilter, visArgs := visibilityFilter(role, userID)
	where += visFilter
	args = append(args, visArgs...)

	query := fmt.Sprintf(`SELECT `+entitySelectColumns+`
	          FROM entities e
	          INNER JOIN entity_types et ON et.id = e.entity_type_id
	          %s
	          ORDER BY e.sort_order ASC, e.name ASC`, where)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("finding children: %w", err)
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		e, err := r.scanEntityRow(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, *e)
	}
	return entities, rows.Err()
}

// FindAncestors returns the ancestor chain from an entity up to the root,
// ordered from immediate parent to furthest ancestor. Uses a recursive CTE
// with a depth limit of 20 to prevent infinite loops from data corruption.
func (r *entityRepository) FindAncestors(ctx context.Context, entityID string) ([]Entity, error) {
	query := `WITH RECURSIVE ancestors AS (
	    SELECT e.id, e.campaign_id, e.entity_type_id, e.name, e.slug,
	           e.entry, e.entry_html, e.player_notes, e.player_notes_html,
	           e.image_path, e.cover_image_path, e.parent_id, e.parent_node_id, e.sort_order, e.type_label,
	           e.is_private, e.visibility, e.is_template, e.fields_data, e.field_overrides, e.popup_config,
	           e.created_by, e.created_at, e.updated_at,
	           1 AS depth
	    FROM entities e
	    WHERE e.id = (SELECT parent_id FROM entities WHERE id = ?)
	    UNION ALL
	    SELECT e.id, e.campaign_id, e.entity_type_id, e.name, e.slug,
	           e.entry, e.entry_html, e.player_notes, e.player_notes_html,
	           e.image_path, e.cover_image_path, e.parent_id, e.parent_node_id, e.sort_order, e.type_label,
	           e.is_private, e.visibility, e.is_template, e.fields_data, e.field_overrides, e.popup_config,
	           e.created_by, e.created_at, e.updated_at,
	           a.depth + 1
	    FROM entities e
	    INNER JOIN ancestors a ON e.id = a.parent_id
	    WHERE a.depth < 20
	)
	SELECT a.id, a.campaign_id, a.entity_type_id, a.name, a.slug,
	       a.entry, a.entry_html, a.player_notes, a.player_notes_html,
	       a.image_path, a.cover_image_path, a.parent_id, a.parent_node_id, a.sort_order, a.type_label,
	       a.is_private, a.visibility, a.is_template, a.fields_data, a.field_overrides, a.popup_config,
	       a.created_by, a.created_at, a.updated_at,
	       et.name, et.icon, et.color, et.slug
	FROM ancestors a
	INNER JOIN entity_types et ON et.id = a.entity_type_id
	ORDER BY a.depth ASC`

	rows, err := r.db.QueryContext(ctx, query, entityID)
	if err != nil {
		return nil, fmt.Errorf("finding ancestors: %w", err)
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		e, err := r.scanEntityRow(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, *e)
	}
	return entities, rows.Err()
}

// UpdateParent sets or clears an entity's parent_id. Scoped to campaign for safety.
// Note: does not check RowsAffected because MySQL returns 0 when the value is
// unchanged, which is expected during reorder-within-same-parent operations.
func (r *entityRepository) UpdateParent(ctx context.Context, entityID, campaignID string, parentID *string) error {
	query := `UPDATE entities SET parent_id = ?, updated_at = NOW() WHERE id = ? AND campaign_id = ?`
	_, err := r.db.ExecContext(ctx, query, parentID, entityID, campaignID)
	if err != nil {
		return fmt.Errorf("updating entity parent: %w", err)
	}
	return nil
}

// UpdateParentNode sets or clears an entity's sidebar folder node parent.
// An entity can be parented under a folder node (parent_node_id) or another
// entity (parent_id), but not both. The caller clears the other reference.
func (r *entityRepository) UpdateParentNode(ctx context.Context, entityID, campaignID string, parentNodeID *string) error {
	query := `UPDATE entities SET parent_node_id = ?, updated_at = NOW() WHERE id = ? AND campaign_id = ?`
	_, err := r.db.ExecContext(ctx, query, parentNodeID, entityID, campaignID)
	if err != nil {
		return fmt.Errorf("updating entity parent node: %w", err)
	}
	return nil
}

// UpdateSortOrder sets an entity's manual sort order within its parent/category.
// Scoped to campaign for safety.
// Note: does not check RowsAffected because MySQL returns 0 when the new value
// matches the existing value within the same second (updated_at unchanged).
// This commonly happens during reorder when calculateSortOrder returns the same value.
func (r *entityRepository) UpdateSortOrder(ctx context.Context, entityID, campaignID string, sortOrder int) error {
	query := `UPDATE entities SET sort_order = ?, updated_at = NOW() WHERE id = ? AND campaign_id = ?`
	_, err := r.db.ExecContext(ctx, query, sortOrder, entityID, campaignID)
	if err != nil {
		return fmt.Errorf("updating entity sort order: %w", err)
	}
	return nil
}

// UpdatePrivate sets an entity's is_private flag. Used by the NPC reveal toggle.
func (r *entityRepository) UpdatePrivate(ctx context.Context, entityID string, isPrivate bool) error {
	query := `UPDATE entities SET is_private = ?, updated_at = NOW() WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, isPrivate, entityID)
	if err != nil {
		return fmt.Errorf("updating entity privacy: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// UpdateEntityType changes an entity's entity_type_id. Used by bulk type reassignment
// from the sync API. Updates the updated_at timestamp for change tracking.
func (r *entityRepository) UpdateEntityType(ctx context.Context, entityID string, typeID int) error {
	query := `UPDATE entities SET entity_type_id = ?, updated_at = NOW() WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, typeID, entityID)
	if err != nil {
		return fmt.Errorf("updating entity type: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// FindBacklinks returns entities that mention the given entity via @mention links
// in their entry_html. Searches for the data-mention-id="<entityID>" attribute
// pattern. Respects visibility filtering.
func (r *entityRepository) FindBacklinks(ctx context.Context, entityID string, role int, userID string) ([]Entity, error) {
	where := `WHERE e.entry_html LIKE ? AND e.id != ?`
	escaped := strings.NewReplacer("%", `\%`, "_", `\_`).Replace(entityID)
	pattern := `%data-mention-id="` + escaped + `"%`
	args := []any{pattern, entityID}

	visFilter, visArgs := visibilityFilter(role, userID)
	where += visFilter
	args = append(args, visArgs...)

	query := fmt.Sprintf(`SELECT `+entitySelectColumns+`
	          FROM entities e
	          INNER JOIN entity_types et ON et.id = e.entity_type_id
	          %s
	          ORDER BY e.name
	          LIMIT 50`, where)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("finding backlinks: %w", err)
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		e, err := r.scanEntityRow(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, *e)
	}
	return entities, rows.Err()
}

// UpdatePopupConfig persists the entity's hover preview configuration as JSON.
func (r *entityRepository) UpdatePopupConfig(ctx context.Context, entityID string, config *PopupConfig) error {
	var configJSON []byte
	var err error
	if config != nil {
		configJSON, err = json.Marshal(config)
		if err != nil {
			return fmt.Errorf("marshaling popup config: %w", err)
		}
	}

	query := `UPDATE entities SET popup_config = ?, updated_at = NOW() WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, configJSON, entityID)
	if err != nil {
		return fmt.Errorf("updating popup config: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// CopyEntityTags duplicates all entity_tags rows from one entity to another
// using a single INSERT...SELECT statement.
func (r *entityRepository) CopyEntityTags(ctx context.Context, sourceEntityID, targetEntityID string) error {
	query := `INSERT IGNORE INTO entity_tags (entity_id, tag_id, created_at)
	           SELECT ?, tag_id, NOW() FROM entity_tags WHERE entity_id = ?`
	_, err := r.db.ExecContext(ctx, query, targetEntityID, sourceEntityID)
	if err != nil {
		return fmt.Errorf("copying entity tags: %w", err)
	}
	return nil
}

// scanEntityRow scans a single entity from a rows iterator.
// The column order must match entitySelectColumns.
func (r *entityRepository) scanEntityRow(rows *sql.Rows) (*Entity, error) {
	e := &Entity{}
	var fieldsRaw, overridesRaw, popupRaw []byte
	err := rows.Scan(
		&e.ID, &e.CampaignID, &e.EntityTypeID, &e.Name, &e.Slug,
		&e.Entry, &e.EntryHTML, &e.PlayerNotes, &e.PlayerNotesHTML,
		&e.ImagePath, &e.CoverImagePath, &e.ParentID, &e.ParentNodeID, &e.SortOrder, &e.TypeLabel,
		&e.IsPrivate, &e.Visibility, &e.IsTemplate, &fieldsRaw, &overridesRaw, &popupRaw,
		&e.CreatedBy, &e.CreatedAt, &e.UpdatedAt,
		&e.TypeName, &e.TypeIcon, &e.TypeColor, &e.TypeSlug,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning entity row: %w", err)
	}

	e.FieldsData = make(map[string]any)
	if len(fieldsRaw) > 0 {
		if err := json.Unmarshal(fieldsRaw, &e.FieldsData); err != nil {
			return nil, fmt.Errorf("unmarshaling fields data: %w", err)
		}
	}
	if len(overridesRaw) > 0 {
		e.FieldOverrides = &FieldOverrides{}
		if err := json.Unmarshal(overridesRaw, e.FieldOverrides); err != nil {
			return nil, fmt.Errorf("unmarshaling field overrides: %w", err)
		}
	}
	if len(popupRaw) > 0 {
		e.PopupConfig = &PopupConfig{}
		if err := json.Unmarshal(popupRaw, e.PopupConfig); err != nil {
			return nil, fmt.Errorf("unmarshaling popup config: %w", err)
		}
	}
	return e, nil
}

// --- Entity Permission Repository ---

// EntityPermissionRepository defines the data access contract for per-entity permission grants.
type EntityPermissionRepository interface {
	// ListByEntity returns all permission grants for an entity.
	ListByEntity(ctx context.Context, entityID string) ([]EntityPermission, error)

	// SetPermissions replaces all permissions for an entity in a single transaction.
	SetPermissions(ctx context.Context, entityID string, grants []PermissionGrant) error

	// DeleteByEntity removes all permission grants for an entity.
	DeleteByEntity(ctx context.Context, entityID string) error

	// GetEffectivePermission resolves the effective access level for a user on an entity
	// by checking role-based and user-specific grants.
	GetEffectivePermission(ctx context.Context, entityID string, role int, userID string) (*EffectivePermission, error)

	// UpdateVisibility sets the visibility mode for an entity.
	UpdateVisibility(ctx context.Context, entityID string, visibility VisibilityMode) error
}

// entityPermissionRepository implements EntityPermissionRepository with MariaDB queries.
type entityPermissionRepository struct {
	db *sql.DB
}

// NewEntityPermissionRepository creates a new entity permission repository.
func NewEntityPermissionRepository(db *sql.DB) EntityPermissionRepository {
	return &entityPermissionRepository{db: db}
}

// ListByEntity returns all permission grants for a specific entity.
func (r *entityPermissionRepository) ListByEntity(ctx context.Context, entityID string) ([]EntityPermission, error) {
	query := `SELECT id, entity_id, subject_type, subject_id, permission, created_at
	          FROM entity_permissions WHERE entity_id = ? ORDER BY subject_type, subject_id`

	rows, err := r.db.QueryContext(ctx, query, entityID)
	if err != nil {
		return nil, fmt.Errorf("listing entity permissions: %w", err)
	}
	defer rows.Close()

	var perms []EntityPermission
	for rows.Next() {
		var p EntityPermission
		if err := rows.Scan(&p.ID, &p.EntityID, &p.SubjectType, &p.SubjectID, &p.Permission, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning entity permission: %w", err)
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

// SetPermissions replaces all permission grants for an entity atomically.
func (r *entityPermissionRepository) SetPermissions(ctx context.Context, entityID string, grants []PermissionGrant) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Clear existing grants.
	if _, err := tx.ExecContext(ctx, `DELETE FROM entity_permissions WHERE entity_id = ?`, entityID); err != nil {
		return fmt.Errorf("clearing entity permissions: %w", err)
	}

	// Insert new grants.
	if len(grants) > 0 {
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO entity_permissions (entity_id, subject_type, subject_id, permission) VALUES (?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("preparing permission insert: %w", err)
		}
		defer func() { _ = stmt.Close() }()

		for _, g := range grants {
			if _, err := stmt.ExecContext(ctx, entityID, g.SubjectType, g.SubjectID, g.Permission); err != nil {
				return fmt.Errorf("inserting permission grant: %w", err)
			}
		}
	}

	return tx.Commit()
}

// DeleteByEntity removes all permission grants for an entity.
func (r *entityPermissionRepository) DeleteByEntity(ctx context.Context, entityID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM entity_permissions WHERE entity_id = ?`, entityID)
	if err != nil {
		return fmt.Errorf("deleting entity permissions: %w", err)
	}
	return nil
}

// GetEffectivePermission resolves the effective access for a user on an entity
// by checking all applicable grants (role-based and user-specific).
func (r *entityPermissionRepository) GetEffectivePermission(ctx context.Context, entityID string, role int, userID string) (*EffectivePermission, error) {
	query := `SELECT permission FROM entity_permissions
	          WHERE entity_id = ?
	          AND (
	              (subject_type = 'role' AND CAST(subject_id AS UNSIGNED) <= ?)
	              OR (subject_type = 'user' AND subject_id = ?)
	              OR (subject_type = 'group' AND EXISTS (
	                  SELECT 1 FROM campaign_group_members cgm
	                  WHERE cgm.group_id = CAST(subject_id AS UNSIGNED)
	                  AND cgm.user_id = ?
	              ))
	          )`

	rows, err := r.db.QueryContext(ctx, query, entityID, role, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("querying effective permission: %w", err)
	}
	defer rows.Close()

	ep := &EffectivePermission{}
	for rows.Next() {
		var perm string
		if err := rows.Scan(&perm); err != nil {
			return nil, fmt.Errorf("scanning permission: %w", err)
		}
		switch Permission(perm) {
		case PermView:
			ep.CanView = true
		case PermEdit:
			ep.CanView = true // Edit implies view.
			ep.CanEdit = true
		}
	}
	return ep, rows.Err()
}

// UpdateVisibility sets the visibility mode for an entity.
func (r *entityPermissionRepository) UpdateVisibility(ctx context.Context, entityID string, visibility VisibilityMode) error {
	query := `UPDATE entities SET visibility = ?, updated_at = NOW() WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, visibility, entityID)
	if err != nil {
		return fmt.Errorf("updating entity visibility: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// ftOperatorReplacer strips MySQL FULLTEXT boolean mode operators from user input
// to prevent search manipulation. These operators (+, -, >, <, (, ), ~, *, ")
// have special meaning in BOOLEAN MODE and could alter search behavior.
var ftOperatorReplacer = strings.NewReplacer(
	"+", "", "-", "", ">", "", "<", "",
	"(", "", ")", "", "~", "", "*", "", "\"", "",
)

// stripFTOperators removes FULLTEXT boolean operators from a search query.
func stripFTOperators(query string) string {
	return ftOperatorReplacer.Replace(query)
}

// ListNames returns lightweight name entries for auto-linking, including entity
// aliases as separate rows. Only returns names >= 3 chars. Sorted by name
// length DESC so longer names match first (prevents "King" matching before
// "King Arthur"). Alias entries have IsAlias=true.
func (r *entityRepository) ListNames(ctx context.Context, campaignID string, role int, userID string) ([]EntityNameEntry, error) {
	where := "WHERE e.campaign_id = ? AND CHAR_LENGTH(e.name) >= 3"
	args := []any{campaignID}

	visFilter, visArgs := visibilityFilter(role, userID)
	where += visFilter
	args = append(args, visArgs...)

	// UNION entity names with aliases. Aliases get is_alias=1.
	aliasWhere := "WHERE e.campaign_id = ? AND CHAR_LENGTH(ea.alias) >= 3"
	aliasArgs := []any{campaignID}
	aliasArgs = append(aliasArgs, visArgs...)

	query := fmt.Sprintf(`
		SELECT id, name, slug, type_name, type_icon, type_slug, is_alias FROM (
			SELECT e.id, e.name AS name, e.slug, et.name AS type_name, et.icon AS type_icon, et.slug AS type_slug, 0 AS is_alias
			FROM entities e
			INNER JOIN entity_types et ON et.id = e.entity_type_id
			%s
			UNION ALL
			SELECT e.id, ea.alias AS name, e.slug, et.name AS type_name, et.icon AS type_icon, et.slug AS type_slug, 1 AS is_alias
			FROM entity_aliases ea
			INNER JOIN entities e ON e.id = ea.entity_id
			INNER JOIN entity_types et ON et.id = e.entity_type_id
			%s %s
		) combined
		ORDER BY CHAR_LENGTH(name) DESC, name`, where, aliasWhere, visFilter)

	allArgs := append(args, aliasArgs...)
	rows, err := r.db.QueryContext(ctx, query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("listing entity names: %w", err)
	}
	defer rows.Close()

	var entries []EntityNameEntry
	for rows.Next() {
		var entry EntityNameEntry
		var isAlias int
		if err := rows.Scan(&entry.ID, &entry.Name, &entry.Slug, &entry.TypeName, &entry.TypeIcon, &entry.TypeSlug, &isAlias); err != nil {
			return nil, fmt.Errorf("scanning entity name: %w", err)
		}
		entry.IsAlias = isAlias == 1
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// ListAliases returns all aliases for a given entity, ordered alphabetically.
func (r *entityRepository) ListAliases(ctx context.Context, entityID string) ([]EntityAlias, error) {
	query := `SELECT id, entity_id, alias, created_at FROM entity_aliases WHERE entity_id = ? ORDER BY alias`
	rows, err := r.db.QueryContext(ctx, query, entityID)
	if err != nil {
		return nil, fmt.Errorf("listing aliases: %w", err)
	}
	defer rows.Close()

	var aliases []EntityAlias
	for rows.Next() {
		var a EntityAlias
		if err := rows.Scan(&a.ID, &a.EntityID, &a.Alias, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning alias: %w", err)
		}
		aliases = append(aliases, a)
	}
	return aliases, rows.Err()
}

// SetAliases replaces all aliases for an entity. Deletes existing and batch
// inserts new ones. Caller must validate alias count and length limits.
func (r *entityRepository) SetAliases(ctx context.Context, entityID string, aliases []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alias tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM entity_aliases WHERE entity_id = ?`, entityID); err != nil {
		return fmt.Errorf("deleting old aliases: %w", err)
	}

	if len(aliases) > 0 {
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO entity_aliases (entity_id, alias) VALUES (?, ?)`)
		if err != nil {
			return fmt.Errorf("preparing alias insert: %w", err)
		}
		defer func() { _ = stmt.Close() }()

		for _, alias := range aliases {
			if _, err := stmt.ExecContext(ctx, entityID, alias); err != nil {
				return fmt.Errorf("inserting alias %q: %w", alias, err)
			}
		}
	}

	return tx.Commit()
}

// mentionIDPattern matches data-mention-id="<uuid>" attributes in entry_html.
var mentionIDPattern = regexp.MustCompile(`data-mention-id="([a-f0-9-]+)"`)

// FindAllMentionLinks scans all entities in a campaign for @mention references
// in their entry_html and returns source→target pairs. Respects visibility
// filtering so players only see links from/to entities they can access.
func (r *entityRepository) FindAllMentionLinks(ctx context.Context, campaignID string, role int, userID string) ([]MentionLink, error) {
	where := `WHERE e.campaign_id = ? AND e.entry_html LIKE '%data-mention-id=%'`
	args := []any{campaignID}

	visFilter, visArgs := visibilityFilter(role, userID)
	where += visFilter
	args = append(args, visArgs...)

	query := fmt.Sprintf(`SELECT e.id, e.entry_html FROM entities e %s`, where)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("finding mention links: %w", err)
	}
	defer rows.Close()

	var links []MentionLink
	for rows.Next() {
		var sourceID string
		var entryHTML *string
		if err := rows.Scan(&sourceID, &entryHTML); err != nil {
			return nil, fmt.Errorf("scanning mention row: %w", err)
		}
		if entryHTML == nil {
			continue
		}

		// Extract all mention target IDs from the HTML.
		matches := mentionIDPattern.FindAllStringSubmatch(*entryHTML, -1)
		seen := make(map[string]bool)
		for _, match := range matches {
			targetID := match[1]
			// Skip self-mentions and dedup within same entity.
			if targetID == sourceID || seen[targetID] {
				continue
			}
			seen[targetID] = true
			links = append(links, MentionLink{
				SourceEntityID: sourceID,
				TargetEntityID: targetID,
			})
		}
	}
	return links, rows.Err()
}
