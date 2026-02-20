package tags

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TagRepository defines the data access contract for tags and entity-tag
// associations. One repository per aggregate root; all SQL lives here.
type TagRepository interface {
	// Create inserts a new tag. The tag's ID is set on the struct after insert.
	Create(ctx context.Context, tag *Tag) error

	// FindByID retrieves a single tag by its primary key.
	FindByID(ctx context.Context, id int) (*Tag, error)

	// ListByCampaign returns all tags belonging to the given campaign,
	// ordered alphabetically by name.
	ListByCampaign(ctx context.Context, campaignID string) ([]Tag, error)

	// Update modifies an existing tag's name, slug, and color.
	Update(ctx context.Context, tag *Tag) error

	// Delete removes a tag by ID. Cascade deletes remove entity_tags rows.
	Delete(ctx context.Context, id int) error

	// AddTagToEntity creates a row in the entity_tags join table.
	AddTagToEntity(ctx context.Context, entityID string, tagID int) error

	// RemoveTagFromEntity deletes a row from the entity_tags join table.
	RemoveTagFromEntity(ctx context.Context, entityID string, tagID int) error

	// GetEntityTags returns all tags associated with a single entity.
	GetEntityTags(ctx context.Context, entityID string) ([]Tag, error)

	// GetEntityTagsBatch returns tags for multiple entities in a single query.
	// The result is keyed by entity ID. Useful for list views to avoid N+1.
	GetEntityTagsBatch(ctx context.Context, entityIDs []string) (map[string][]Tag, error)
}

// tagRepository implements TagRepository using MariaDB with hand-written SQL.
type tagRepository struct {
	db *sql.DB
}

// NewTagRepository creates a new TagRepository backed by the given database connection.
func NewTagRepository(db *sql.DB) TagRepository {
	return &tagRepository{db: db}
}

// Create inserts a new tag into the tags table and sets the auto-generated ID
// on the provided struct.
func (r *tagRepository) Create(ctx context.Context, tag *Tag) error {
	query := `INSERT INTO tags (campaign_id, name, slug, color)
	           VALUES (?, ?, ?, ?)`

	result, err := r.db.ExecContext(ctx, query,
		tag.CampaignID, tag.Name, tag.Slug, tag.Color,
	)
	if err != nil {
		// Check for duplicate slug within the campaign.
		if isDuplicateEntry(err) {
			return apperror.NewConflict("a tag with this name already exists in the campaign")
		}
		return fmt.Errorf("inserting tag: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting last insert id: %w", err)
	}
	tag.ID = int(id)

	return nil
}

// FindByID retrieves a single tag by its primary key.
func (r *tagRepository) FindByID(ctx context.Context, id int) (*Tag, error) {
	query := `SELECT id, campaign_id, name, slug, color, created_at
	           FROM tags WHERE id = ?`

	var t Tag
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&t.ID, &t.CampaignID, &t.Name, &t.Slug, &t.Color, &t.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("tag not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying tag by id: %w", err)
	}
	return &t, nil
}

// ListByCampaign returns all tags for a campaign, ordered by name.
func (r *tagRepository) ListByCampaign(ctx context.Context, campaignID string) ([]Tag, error) {
	query := `SELECT id, campaign_id, name, slug, color, created_at
	           FROM tags WHERE campaign_id = ?
	           ORDER BY name ASC`

	rows, err := r.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing tags by campaign: %w", err)
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.CampaignID, &t.Name, &t.Slug, &t.Color, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning tag row: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tag rows: %w", err)
	}

	return tags, nil
}

// Update modifies an existing tag's name, slug, and color.
func (r *tagRepository) Update(ctx context.Context, tag *Tag) error {
	query := `UPDATE tags SET name = ?, slug = ?, color = ?
	           WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, tag.Name, tag.Slug, tag.Color, tag.ID)
	if err != nil {
		if isDuplicateEntry(err) {
			return apperror.NewConflict("a tag with this name already exists in the campaign")
		}
		return fmt.Errorf("updating tag: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return apperror.NewNotFound("tag not found")
	}

	return nil
}

// Delete removes a tag by ID. The entity_tags rows are cascade-deleted by
// the foreign key constraint in the migration.
func (r *tagRepository) Delete(ctx context.Context, id int) error {
	query := `DELETE FROM tags WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting tag: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return apperror.NewNotFound("tag not found")
	}

	return nil
}

// AddTagToEntity creates a row in the entity_tags join table. Uses INSERT
// IGNORE to silently skip if the association already exists.
func (r *tagRepository) AddTagToEntity(ctx context.Context, entityID string, tagID int) error {
	query := `INSERT IGNORE INTO entity_tags (entity_id, tag_id)
	           VALUES (?, ?)`

	if _, err := r.db.ExecContext(ctx, query, entityID, tagID); err != nil {
		return fmt.Errorf("adding tag to entity: %w", err)
	}
	return nil
}

// RemoveTagFromEntity deletes a row from the entity_tags join table.
func (r *tagRepository) RemoveTagFromEntity(ctx context.Context, entityID string, tagID int) error {
	query := `DELETE FROM entity_tags WHERE entity_id = ? AND tag_id = ?`

	if _, err := r.db.ExecContext(ctx, query, entityID, tagID); err != nil {
		return fmt.Errorf("removing tag from entity: %w", err)
	}
	return nil
}

// GetEntityTags returns all tags associated with a single entity, ordered
// alphabetically by name.
func (r *tagRepository) GetEntityTags(ctx context.Context, entityID string) ([]Tag, error) {
	query := `SELECT t.id, t.campaign_id, t.name, t.slug, t.color, t.created_at
	           FROM tags t
	           INNER JOIN entity_tags et ON et.tag_id = t.id
	           WHERE et.entity_id = ?
	           ORDER BY t.name ASC`

	rows, err := r.db.QueryContext(ctx, query, entityID)
	if err != nil {
		return nil, fmt.Errorf("getting entity tags: %w", err)
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.CampaignID, &t.Name, &t.Slug, &t.Color, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning entity tag row: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating entity tag rows: %w", err)
	}

	return tags, nil
}

// GetEntityTagsBatch returns tags for multiple entities in a single query,
// keyed by entity ID. This avoids N+1 queries on entity list views.
//
// Returns an empty map if no entity IDs are provided.
func (r *tagRepository) GetEntityTagsBatch(ctx context.Context, entityIDs []string) (map[string][]Tag, error) {
	if len(entityIDs) == 0 {
		return make(map[string][]Tag), nil
	}

	// Build parameterized IN clause to avoid SQL injection.
	placeholders := make([]string, len(entityIDs))
	args := make([]interface{}, len(entityIDs))
	for i, id := range entityIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT et.entity_id, t.id, t.campaign_id, t.name, t.slug, t.color, t.created_at
	           FROM tags t
	           INNER JOIN entity_tags et ON et.tag_id = t.id
	           WHERE et.entity_id IN (%s)
	           ORDER BY t.name ASC`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch getting entity tags: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]Tag)
	for rows.Next() {
		var entityID string
		var t Tag
		if err := rows.Scan(&entityID, &t.ID, &t.CampaignID, &t.Name, &t.Slug, &t.Color, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning batch entity tag row: %w", err)
		}
		result[entityID] = append(result[entityID], t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating batch entity tag rows: %w", err)
	}

	return result, nil
}

// isDuplicateEntry checks if a MySQL/MariaDB error is a duplicate key violation.
// Error code 1062 is ER_DUP_ENTRY for unique constraint violations.
func isDuplicateEntry(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Duplicate entry")
}
