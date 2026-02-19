package entities

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- EntityType Repository ---

// EntityTypeRepository defines the data access contract for entity type operations.
type EntityTypeRepository interface {
	Create(ctx context.Context, et *EntityType) error
	FindByID(ctx context.Context, id int) (*EntityType, error)
	FindBySlug(ctx context.Context, campaignID, slug string) (*EntityType, error)
	ListByCampaign(ctx context.Context, campaignID string) ([]EntityType, error)
	UpdateLayout(ctx context.Context, id int, layoutJSON string) error
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

	query := `INSERT INTO entity_types (campaign_id, slug, name, name_plural, icon, color, fields, layout_json, sort_order, is_default, enabled)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := r.db.ExecContext(ctx, query,
		et.CampaignID, et.Slug, et.Name, et.NamePlural,
		et.Icon, et.Color, fieldsJSON, layoutJSON, et.SortOrder,
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
	query := `SELECT id, campaign_id, slug, name, name_plural, icon, color, fields, layout_json, sort_order, is_default, enabled
	          FROM entity_types WHERE id = ?`

	et := &EntityType{}
	var fieldsRaw, layoutRaw []byte
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&et.ID, &et.CampaignID, &et.Slug, &et.Name, &et.NamePlural,
		&et.Icon, &et.Color, &fieldsRaw, &layoutRaw, &et.SortOrder,
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
	if len(layoutRaw) > 0 {
		_ = json.Unmarshal(layoutRaw, &et.Layout)
	}
	return et, nil
}

// FindBySlug retrieves an entity type by campaign ID and slug.
func (r *entityTypeRepository) FindBySlug(ctx context.Context, campaignID, slug string) (*EntityType, error) {
	query := `SELECT id, campaign_id, slug, name, name_plural, icon, color, fields, layout_json, sort_order, is_default, enabled
	          FROM entity_types WHERE campaign_id = ? AND slug = ?`

	et := &EntityType{}
	var fieldsRaw, layoutRaw []byte
	err := r.db.QueryRowContext(ctx, query, campaignID, slug).Scan(
		&et.ID, &et.CampaignID, &et.Slug, &et.Name, &et.NamePlural,
		&et.Icon, &et.Color, &fieldsRaw, &layoutRaw, &et.SortOrder,
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
	if len(layoutRaw) > 0 {
		_ = json.Unmarshal(layoutRaw, &et.Layout)
	}
	return et, nil
}

// ListByCampaign returns all entity types for a campaign, ordered by sort_order.
func (r *entityTypeRepository) ListByCampaign(ctx context.Context, campaignID string) ([]EntityType, error) {
	query := `SELECT id, campaign_id, slug, name, name_plural, icon, color, fields, layout_json, sort_order, is_default, enabled
	          FROM entity_types WHERE campaign_id = ? ORDER BY sort_order, name`

	rows, err := r.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing entity types: %w", err)
	}
	defer rows.Close()

	var types []EntityType
	for rows.Next() {
		var et EntityType
		var fieldsRaw, layoutRaw []byte
		if err := rows.Scan(
			&et.ID, &et.CampaignID, &et.Slug, &et.Name, &et.NamePlural,
			&et.Icon, &et.Color, &fieldsRaw, &layoutRaw, &et.SortOrder,
			&et.IsDefault, &et.Enabled,
		); err != nil {
			return nil, fmt.Errorf("scanning entity type row: %w", err)
		}
		if err := json.Unmarshal(fieldsRaw, &et.Fields); err != nil {
			return nil, fmt.Errorf("unmarshaling entity type fields: %w", err)
		}
		if len(layoutRaw) > 0 {
			_ = json.Unmarshal(layoutRaw, &et.Layout)
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
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("entity type not found")
	}
	return nil
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
}

// SeedDefaults inserts the default entity types for a newly created campaign.
// Uses a transaction to ensure all-or-nothing insertion.
func (r *entityTypeRepository) SeedDefaults(ctx context.Context, campaignID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning seed tx: %w", err)
	}
	defer tx.Rollback()

	query := `INSERT INTO entity_types (campaign_id, slug, name, name_plural, icon, color, fields, sort_order, is_default, enabled)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	for _, et := range defaultEntityTypes {
		fieldsJSON, err := json.Marshal(et.Fields)
		if err != nil {
			return fmt.Errorf("marshaling default fields for %s: %w", et.Slug, err)
		}

		_, err = tx.ExecContext(ctx, query,
			campaignID, et.Slug, et.Name, et.NamePlural,
			et.Icon, et.Color, fieldsJSON, et.SortOrder,
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
	UpdateImage(ctx context.Context, id, imagePath string) error
	Delete(ctx context.Context, id string) error
	SlugExists(ctx context.Context, campaignID, slug string) (bool, error)

	// ListByCampaign returns entities filtered by campaign, optional type, and privacy.
	// When role < RoleScribe (2), private entities are excluded.
	ListByCampaign(ctx context.Context, campaignID string, typeID int, role int, opts ListOptions) ([]Entity, int, error)

	// Search performs a FULLTEXT search on entity names. Falls back to LIKE
	// for queries shorter than 4 characters (MariaDB ft_min_word_len default).
	Search(ctx context.Context, campaignID, query string, typeID int, role int, opts ListOptions) ([]Entity, int, error)

	// CountByType returns entity counts per type for the sidebar badges.
	CountByType(ctx context.Context, campaignID string, role int) (map[int]int, error)
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
	          image_path, parent_id, type_label, is_private, is_template, fields_data, created_by, created_at, updated_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = r.db.ExecContext(ctx, query,
		entity.ID, entity.CampaignID, entity.EntityTypeID,
		entity.Name, entity.Slug, entity.Entry, entity.EntryHTML,
		entity.ImagePath, entity.ParentID, entity.TypeLabel,
		entity.IsPrivate, entity.IsTemplate, fieldsJSON,
		entity.CreatedBy, entity.CreatedAt, entity.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting entity: %w", err)
	}
	return nil
}

// FindByID retrieves an entity with joined type info.
func (r *entityRepository) FindByID(ctx context.Context, id string) (*Entity, error) {
	query := `SELECT e.id, e.campaign_id, e.entity_type_id, e.name, e.slug,
	                 e.entry, e.entry_html, e.image_path, e.parent_id, e.type_label,
	                 e.is_private, e.is_template, e.fields_data, e.created_by,
	                 e.created_at, e.updated_at,
	                 et.name, et.icon, et.color, et.slug
	          FROM entities e
	          INNER JOIN entity_types et ON et.id = e.entity_type_id
	          WHERE e.id = ?`

	return r.scanEntity(r.db.QueryRowContext(ctx, query, id))
}

// FindBySlug retrieves an entity by campaign ID and slug with joined type info.
func (r *entityRepository) FindBySlug(ctx context.Context, campaignID, slug string) (*Entity, error) {
	query := `SELECT e.id, e.campaign_id, e.entity_type_id, e.name, e.slug,
	                 e.entry, e.entry_html, e.image_path, e.parent_id, e.type_label,
	                 e.is_private, e.is_template, e.fields_data, e.created_by,
	                 e.created_at, e.updated_at,
	                 et.name, et.icon, et.color, et.slug
	          FROM entities e
	          INNER JOIN entity_types et ON et.id = e.entity_type_id
	          WHERE e.campaign_id = ? AND e.slug = ?`

	return r.scanEntity(r.db.QueryRowContext(ctx, query, campaignID, slug))
}

// scanEntity scans a single entity row with joined type fields.
func (r *entityRepository) scanEntity(row *sql.Row) (*Entity, error) {
	e := &Entity{}
	var fieldsRaw []byte
	err := row.Scan(
		&e.ID, &e.CampaignID, &e.EntityTypeID, &e.Name, &e.Slug,
		&e.Entry, &e.EntryHTML, &e.ImagePath, &e.ParentID, &e.TypeLabel,
		&e.IsPrivate, &e.IsTemplate, &fieldsRaw, &e.CreatedBy,
		&e.CreatedAt, &e.UpdatedAt,
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
	return e, nil
}

// Update modifies an existing entity.
func (r *entityRepository) Update(ctx context.Context, entity *Entity) error {
	fieldsJSON, err := json.Marshal(entity.FieldsData)
	if err != nil {
		return fmt.Errorf("marshaling fields data: %w", err)
	}

	query := `UPDATE entities SET name = ?, slug = ?, entry = ?, entry_html = ?,
	          type_label = ?, is_private = ?, fields_data = ?, updated_at = ?
	          WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query,
		entity.Name, entity.Slug, entity.Entry, entity.EntryHTML,
		entity.TypeLabel, entity.IsPrivate, fieldsJSON, entity.UpdatedAt,
		entity.ID,
	)
	if err != nil {
		return fmt.Errorf("updating entity: %w", err)
	}

	rows, _ := result.RowsAffected()
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

	rows, _ := result.RowsAffected()
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

// Delete removes an entity.
func (r *entityRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM entities WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting entity: %w", err)
	}

	rows, _ := result.RowsAffected()
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

// ListByCampaign returns entities with pagination and optional type filtering.
// Privacy filtering: when role < 2 (Scribe), private entities are excluded.
func (r *entityRepository) ListByCampaign(ctx context.Context, campaignID string, typeID int, role int, opts ListOptions) ([]Entity, int, error) {
	// Build WHERE clause with privacy filtering.
	where := "WHERE e.campaign_id = ?"
	args := []any{campaignID}

	if typeID > 0 {
		where += " AND e.entity_type_id = ?"
		args = append(args, typeID)
	}
	if role < 2 {
		where += " AND e.is_private = false"
	}

	// Count total for pagination.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM entities e %s", where)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting entities: %w", err)
	}

	// Fetch page.
	query := fmt.Sprintf(`SELECT e.id, e.campaign_id, e.entity_type_id, e.name, e.slug,
	                 e.entry, e.entry_html, e.image_path, e.parent_id, e.type_label,
	                 e.is_private, e.is_template, e.fields_data, e.created_by,
	                 e.created_at, e.updated_at,
	                 et.name, et.icon, et.color, et.slug
	          FROM entities e
	          INNER JOIN entity_types et ON et.id = e.entity_type_id
	          %s
	          ORDER BY e.name
	          LIMIT ? OFFSET ?`, where)

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

// Search performs a text search on entity names with privacy filtering.
// Uses FULLTEXT for queries >= 4 chars, LIKE for shorter queries.
func (r *entityRepository) Search(ctx context.Context, campaignID, query string, typeID int, role int, opts ListOptions) ([]Entity, int, error) {
	where := "WHERE e.campaign_id = ?"
	args := []any{campaignID}

	// FULLTEXT for longer queries, LIKE for short ones.
	if len(query) >= 4 {
		where += " AND MATCH(e.name) AGAINST(? IN BOOLEAN MODE)"
		args = append(args, query+"*")
	} else {
		where += " AND e.name LIKE ?"
		args = append(args, "%"+query+"%")
	}

	if typeID > 0 {
		where += " AND e.entity_type_id = ?"
		args = append(args, typeID)
	}
	if role < 2 {
		where += " AND e.is_private = false"
	}

	// Count total.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM entities e %s", where)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting search results: %w", err)
	}

	// Fetch page.
	selectQuery := fmt.Sprintf(`SELECT e.id, e.campaign_id, e.entity_type_id, e.name, e.slug,
	                 e.entry, e.entry_html, e.image_path, e.parent_id, e.type_label,
	                 e.is_private, e.is_template, e.fields_data, e.created_by,
	                 e.created_at, e.updated_at,
	                 et.name, et.icon, et.color, et.slug
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
// Respects privacy: players don't see private entity counts.
func (r *entityRepository) CountByType(ctx context.Context, campaignID string, role int) (map[int]int, error) {
	query := `SELECT entity_type_id, COUNT(*) FROM entities WHERE campaign_id = ?`
	args := []any{campaignID}

	if role < 2 {
		query += " AND is_private = false"
	}
	query += " GROUP BY entity_type_id"

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

// scanEntityRow scans a single entity from a rows iterator.
func (r *entityRepository) scanEntityRow(rows *sql.Rows) (*Entity, error) {
	e := &Entity{}
	var fieldsRaw []byte
	err := rows.Scan(
		&e.ID, &e.CampaignID, &e.EntityTypeID, &e.Name, &e.Slug,
		&e.Entry, &e.EntryHTML, &e.ImagePath, &e.ParentID, &e.TypeLabel,
		&e.IsPrivate, &e.IsTemplate, &fieldsRaw, &e.CreatedBy,
		&e.CreatedAt, &e.UpdatedAt,
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
	return e, nil
}
