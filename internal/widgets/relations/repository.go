package relations

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// RelationRepository defines the data access contract for entity relations.
// One repository per aggregate root; all SQL lives here.
type RelationRepository interface {
	// Create inserts a new relation row. The relation's ID is set on the
	// struct after insert.
	Create(ctx context.Context, rel *Relation) error

	// FindByID retrieves a single relation by its primary key.
	FindByID(ctx context.Context, id int) (*Relation, error)

	// ListByEntity returns all relations where the given entity is the source,
	// joined with target entity details (name, icon, color, slug, type).
	ListByEntity(ctx context.Context, entityID string) ([]Relation, error)

	// Delete removes a relation by ID.
	Delete(ctx context.Context, id int) error

	// FindReverse finds the reverse relation for a given relation.
	// Used when deleting a relation to also remove its reverse.
	FindReverse(ctx context.Context, sourceEntityID, targetEntityID, relationType string) (*Relation, error)
}

// relationRepository implements RelationRepository using MariaDB with
// hand-written SQL. No ORM.
type relationRepository struct {
	db *sql.DB
}

// NewRelationRepository creates a new RelationRepository backed by the
// given database connection.
func NewRelationRepository(db *sql.DB) RelationRepository {
	return &relationRepository{db: db}
}

// Create inserts a new relation into the entity_relations table and sets the
// auto-generated ID on the provided struct.
func (r *relationRepository) Create(ctx context.Context, rel *Relation) error {
	query := `INSERT INTO entity_relations
	           (campaign_id, source_entity_id, target_entity_id, relation_type, reverse_relation_type, created_by)
	           VALUES (?, ?, ?, ?, ?, ?)`

	result, err := r.db.ExecContext(ctx, query,
		rel.CampaignID, rel.SourceEntityID, rel.TargetEntityID,
		rel.RelationType, rel.ReverseRelationType, rel.CreatedBy,
	)
	if err != nil {
		// Check for duplicate relation (same source, target, type).
		if isDuplicateEntry(err) {
			return apperror.NewConflict("this relation already exists between these entities")
		}
		return fmt.Errorf("inserting relation: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting last insert id: %w", err)
	}
	rel.ID = int(id)

	return nil
}

// FindByID retrieves a single relation by its primary key.
func (r *relationRepository) FindByID(ctx context.Context, id int) (*Relation, error) {
	query := `SELECT id, campaign_id, source_entity_id, target_entity_id,
	                  relation_type, reverse_relation_type, created_at, created_by
	           FROM entity_relations WHERE id = ?`

	var rel Relation
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&rel.ID, &rel.CampaignID, &rel.SourceEntityID, &rel.TargetEntityID,
		&rel.RelationType, &rel.ReverseRelationType, &rel.CreatedAt, &rel.CreatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("relation not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying relation by id: %w", err)
	}
	return &rel, nil
}

// ListByEntity returns all relations originating from the given entity, joined
// with target entity details for display. Ordered by relation type, then by
// target entity name for consistent grouping in the UI.
func (r *relationRepository) ListByEntity(ctx context.Context, entityID string) ([]Relation, error) {
	query := `SELECT er.id, er.campaign_id, er.source_entity_id, er.target_entity_id,
	                  er.relation_type, er.reverse_relation_type, er.created_at, er.created_by,
	                  e.name, COALESCE(et.icon, 'fa-file'), COALESCE(et.color, '#6b7280'),
	                  e.slug, COALESCE(et.name, '')
	           FROM entity_relations er
	           INNER JOIN entities e ON e.id = er.target_entity_id
	           LEFT JOIN entity_types et ON et.id = e.entity_type_id
	           WHERE er.source_entity_id = ?
	           ORDER BY er.relation_type ASC, e.name ASC`

	rows, err := r.db.QueryContext(ctx, query, entityID)
	if err != nil {
		return nil, fmt.Errorf("listing relations by entity: %w", err)
	}
	defer rows.Close()

	var relations []Relation
	for rows.Next() {
		var rel Relation
		if err := rows.Scan(
			&rel.ID, &rel.CampaignID, &rel.SourceEntityID, &rel.TargetEntityID,
			&rel.RelationType, &rel.ReverseRelationType, &rel.CreatedAt, &rel.CreatedBy,
			&rel.TargetEntityName, &rel.TargetEntityIcon, &rel.TargetEntityColor,
			&rel.TargetEntitySlug, &rel.TargetEntityType,
		); err != nil {
			return nil, fmt.Errorf("scanning relation row: %w", err)
		}
		relations = append(relations, rel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating relation rows: %w", err)
	}

	return relations, nil
}

// Delete removes a relation by ID. Returns not-found if the row doesn't exist.
func (r *relationRepository) Delete(ctx context.Context, id int) error {
	query := `DELETE FROM entity_relations WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting relation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return apperror.NewNotFound("relation not found")
	}

	return nil
}

// FindReverse finds the reverse direction of a relation. Given a relation
// A→B with type "parent of", this finds B→A with type "child of" (the
// reverse_relation_type from the forward relation becomes the relation_type
// of the reverse).
func (r *relationRepository) FindReverse(ctx context.Context, sourceEntityID, targetEntityID, relationType string) (*Relation, error) {
	query := `SELECT id, campaign_id, source_entity_id, target_entity_id,
	                  relation_type, reverse_relation_type, created_at, created_by
	           FROM entity_relations
	           WHERE source_entity_id = ? AND target_entity_id = ? AND relation_type = ?`

	var rel Relation
	err := r.db.QueryRowContext(ctx, query, sourceEntityID, targetEntityID, relationType).Scan(
		&rel.ID, &rel.CampaignID, &rel.SourceEntityID, &rel.TargetEntityID,
		&rel.RelationType, &rel.ReverseRelationType, &rel.CreatedAt, &rel.CreatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil // No reverse found is not an error.
	}
	if err != nil {
		return nil, fmt.Errorf("finding reverse relation: %w", err)
	}
	return &rel, nil
}

// isDuplicateEntry checks if a MySQL/MariaDB error is a duplicate key violation.
// Error code 1062 is ER_DUP_ENTRY for unique constraint violations.
func isDuplicateEntry(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Duplicate entry")
}
