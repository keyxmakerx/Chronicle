package tags

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TagPermissionRepository owns all SQL for the tag_permissions table — the
// additive visibility grants carried by tags (C-PERM-W1-TAG-GRANTS). Kept as a
// distinct repository from TagRepository so the grant surface can be tested and
// reasoned about in isolation; both back the same widget.
type TagPermissionRepository interface {
	// Create inserts a grant. The grant's ID is set on the struct after insert.
	// Returns a Conflict error if the (tag, subject) pair already exists.
	Create(ctx context.Context, p *TagPermission) error

	// GetByID retrieves a single grant by its primary key.
	GetByID(ctx context.Context, id int) (*TagPermission, error)

	// ListByTag returns all grants on a tag, newest first.
	ListByTag(ctx context.Context, tagID int) ([]TagPermission, error)

	// Delete removes a grant by ID.
	Delete(ctx context.Context, id int) error

	// ListGrantsForEntity returns every tag-derived grant on a single entity:
	// one row per (tag-the-entity-bears × grant-on-that-tag). Joins through
	// entity_tags so it reflects exactly what the visibility filter sees.
	// Used by the effective-visibility glance.
	ListGrantsForEntity(ctx context.Context, entityID string) ([]EntityTagGrant, error)
}

// tagPermissionRepository implements TagPermissionRepository with hand-written
// SQL against MariaDB.
type tagPermissionRepository struct {
	db *sql.DB
}

// NewTagPermissionRepository creates a TagPermissionRepository backed by the
// given database connection.
func NewTagPermissionRepository(db *sql.DB) TagPermissionRepository {
	return &tagPermissionRepository{db: db}
}

// Create inserts a grant and sets the auto-generated ID on the struct.
func (r *tagPermissionRepository) Create(ctx context.Context, p *TagPermission) error {
	query := `INSERT INTO tag_permissions (tag_id, subject_type, subject_id, created_by)
	           VALUES (?, ?, ?, ?)`
	result, err := r.db.ExecContext(ctx, query, p.TagID, p.SubjectType, p.SubjectID, p.CreatedBy)
	if err != nil {
		if isDuplicateEntry(err) {
			return apperror.NewConflict("this tag is already granted to that subject")
		}
		return fmt.Errorf("inserting tag permission: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting last insert id: %w", err)
	}
	p.ID = int(id)
	return nil
}

// GetByID retrieves a single grant by primary key.
func (r *tagPermissionRepository) GetByID(ctx context.Context, id int) (*TagPermission, error) {
	query := `SELECT id, tag_id, subject_type, subject_id, created_by, created_at
	           FROM tag_permissions WHERE id = ?`
	var p TagPermission
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID, &p.TagID, &p.SubjectType, &p.SubjectID, &p.CreatedBy, &p.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("tag grant not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying tag permission by id: %w", err)
	}
	return &p, nil
}

// ListByTag returns all grants on a tag, newest first.
func (r *tagPermissionRepository) ListByTag(ctx context.Context, tagID int) ([]TagPermission, error) {
	query := `SELECT id, tag_id, subject_type, subject_id, created_by, created_at
	           FROM tag_permissions WHERE tag_id = ? ORDER BY created_at DESC, id DESC`
	rows, err := r.db.QueryContext(ctx, query, tagID)
	if err != nil {
		return nil, fmt.Errorf("listing tag permissions: %w", err)
	}
	defer rows.Close()

	var out []TagPermission
	for rows.Next() {
		var p TagPermission
		if err := rows.Scan(&p.ID, &p.TagID, &p.SubjectType, &p.SubjectID, &p.CreatedBy, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning tag permission row: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Delete removes a grant by ID.
func (r *tagPermissionRepository) Delete(ctx context.Context, id int) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM tag_permissions WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting tag permission: %w", err)
	}
	return nil
}

// ListGrantsForEntity returns every tag-derived grant on a single entity.
func (r *tagPermissionRepository) ListGrantsForEntity(ctx context.Context, entityID string) ([]EntityTagGrant, error) {
	query := `SELECT t.id, t.name, t.slug, t.color, tp.subject_type, tp.subject_id
	           FROM entity_tags et
	           JOIN tag_permissions tp ON tp.tag_id = et.tag_id
	           JOIN tags t ON t.id = et.tag_id
	           WHERE et.entity_id = ?
	           ORDER BY t.name ASC, tp.subject_type ASC, tp.subject_id ASC`
	rows, err := r.db.QueryContext(ctx, query, entityID)
	if err != nil {
		return nil, fmt.Errorf("listing grants for entity: %w", err)
	}
	defer rows.Close()

	var out []EntityTagGrant
	for rows.Next() {
		var g EntityTagGrant
		if err := rows.Scan(&g.TagID, &g.TagName, &g.TagSlug, &g.TagColor, &g.SubjectType, &g.SubjectID); err != nil {
			return nil, fmt.Errorf("scanning entity tag grant row: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
