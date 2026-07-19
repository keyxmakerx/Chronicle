// Package entities contains the sidebar node model and repository for
// pure organizational folders in the sidebar tree. Sidebar nodes have
// no page content — they exist solely for hierarchical grouping.
package entities

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// SidebarNode is an organizational folder in the sidebar entity tree.
// Unlike entities, nodes have no page content, entry, or fields — they
// are pure grouping containers visible only in the sidebar drill panel.
type SidebarNode struct {
	ID           string    `json:"id"`
	CampaignID   string    `json:"campaign_id"`
	EntityTypeID int       `json:"entity_type_id"`
	Name         string    `json:"name"`
	ParentID     *string   `json:"parent_id,omitempty"` // Parent node or entity ID.
	SortOrder    int       `json:"sort_order"`
	NodeType     string    `json:"node_type"` // "folder"
	CreatedAt    time.Time `json:"created_at"`
}

// CreateSidebarNodeInput is the validated input for creating a sidebar node.
type CreateSidebarNodeInput struct {
	Name         string
	EntityTypeID int
	ParentID     string // Empty = root level.
}

// SidebarNodeRepository provides CRUD for sidebar folder nodes.
type SidebarNodeRepository interface {
	ListByType(ctx context.Context, campaignID string, entityTypeID int) ([]SidebarNode, error)
	FindByID(ctx context.Context, id string) (*SidebarNode, error)
	Create(ctx context.Context, node *SidebarNode) error
	Update(ctx context.Context, node *SidebarNode) error
	Delete(ctx context.Context, id string) error
	UpdateParent(ctx context.Context, id, campaignID string, parentID *string) error
	// ResequenceNodes renumbers sort_order = position (0..N-1) for the given
	// ordered node IDs in one transaction — the dense re-sequence mechanic
	// (mirrors entities' ResequenceSiblings). Replaces the old raw single-row
	// UpdateSortOrder so folder-node reorder shares the tie-safe mechanic.
	ResequenceNodes(ctx context.Context, campaignID string, orderedIDs []string) error
}

// sidebarNodeRepository implements SidebarNodeRepository with MariaDB.
type sidebarNodeRepository struct {
	db *sql.DB
}

// NewSidebarNodeRepository creates a sidebar node repository.
func NewSidebarNodeRepository(db *sql.DB) SidebarNodeRepository {
	return &sidebarNodeRepository{db: db}
}

// ListByType returns all sidebar nodes for a campaign and entity type,
// ordered by sort_order then name.
func (r *sidebarNodeRepository) ListByType(ctx context.Context, campaignID string, entityTypeID int) ([]SidebarNode, error) {
	query := `SELECT id, campaign_id, entity_type_id, name, parent_id, sort_order, node_type, created_at
	          FROM sidebar_nodes
	          WHERE campaign_id = ? AND entity_type_id = ?
	          ORDER BY sort_order ASC, name ASC`

	rows, err := r.db.QueryContext(ctx, query, campaignID, entityTypeID)
	if err != nil {
		return nil, fmt.Errorf("listing sidebar nodes: %w", err)
	}
	defer rows.Close()

	var nodes []SidebarNode
	for rows.Next() {
		var n SidebarNode
		if err := rows.Scan(&n.ID, &n.CampaignID, &n.EntityTypeID, &n.Name,
			&n.ParentID, &n.SortOrder, &n.NodeType, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning sidebar node: %w", err)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// FindByID retrieves a sidebar node by ID.
func (r *sidebarNodeRepository) FindByID(ctx context.Context, id string) (*SidebarNode, error) {
	query := `SELECT id, campaign_id, entity_type_id, name, parent_id, sort_order, node_type, created_at
	          FROM sidebar_nodes WHERE id = ?`

	var n SidebarNode
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&n.ID, &n.CampaignID, &n.EntityTypeID, &n.Name,
		&n.ParentID, &n.SortOrder, &n.NodeType, &n.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, apperror.NewNotFound("sidebar node not found")
		}
		return nil, fmt.Errorf("finding sidebar node: %w", err)
	}
	return &n, nil
}

// Create inserts a new sidebar node.
func (r *sidebarNodeRepository) Create(ctx context.Context, node *SidebarNode) error {
	query := `INSERT INTO sidebar_nodes (id, campaign_id, entity_type_id, name, parent_id, sort_order, node_type, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		node.ID, node.CampaignID, node.EntityTypeID, node.Name,
		node.ParentID, node.SortOrder, node.NodeType, node.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating sidebar node: %w", err)
	}
	return nil
}

// Update modifies a sidebar node's name.
func (r *sidebarNodeRepository) Update(ctx context.Context, node *SidebarNode) error {
	query := `UPDATE sidebar_nodes SET name = ? WHERE id = ? AND campaign_id = ?`
	_, err := r.db.ExecContext(ctx, query, node.Name, node.ID, node.CampaignID)
	if err != nil {
		return fmt.Errorf("updating sidebar node: %w", err)
	}
	return nil
}

// Delete removes a sidebar node. Children entities have their
// parent_node_id cleared (ON DELETE SET NULL handles this via FK).
func (r *sidebarNodeRepository) Delete(ctx context.Context, id string) error {
	// Reparent child sidebar_nodes to the deleted node's parent.
	var parentID *string
	err := r.db.QueryRowContext(ctx, "SELECT parent_id FROM sidebar_nodes WHERE id = ?", id).Scan(&parentID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("finding node parent: %w", err)
	}

	_, err = r.db.ExecContext(ctx, "UPDATE sidebar_nodes SET parent_id = ? WHERE parent_id = ?", parentID, id)
	if err != nil {
		return fmt.Errorf("reparenting child nodes: %w", err)
	}

	// Reparent child entities to root (clear parent_node_id).
	_, err = r.db.ExecContext(ctx, "UPDATE entities SET parent_node_id = NULL WHERE parent_node_id = ?", id)
	if err != nil {
		return fmt.Errorf("reparenting child entities: %w", err)
	}

	_, err = r.db.ExecContext(ctx, "DELETE FROM sidebar_nodes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting sidebar node: %w", err)
	}
	return nil
}

// UpdateParent sets or clears a sidebar node's parent.
func (r *sidebarNodeRepository) UpdateParent(ctx context.Context, id, campaignID string, parentID *string) error {
	query := `UPDATE sidebar_nodes SET parent_id = ? WHERE id = ? AND campaign_id = ?`
	_, err := r.db.ExecContext(ctx, query, parentID, id, campaignID)
	if err != nil {
		return fmt.Errorf("updating node parent: %w", err)
	}
	return nil
}

// ResequenceNodes writes sort_order = position for each id (0..N-1) in a single
// transaction, mirroring the entities' ResequenceSiblings. Either every node in
// the ordered set is renumbered or none is, so a partial failure can't leave the
// sibling set with colliding orders that the (sort_order, name) render tiebreak
// would silently revert (the #477 bug class, now closed for folder nodes too).
func (r *sidebarNodeRepository) ResequenceNodes(ctx context.Context, campaignID string, orderedIDs []string) error {
	if len(orderedIDs) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin node resequence tx: %w", err)
	}
	// Rollback is a no-op once Commit succeeds; safe to always defer.
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `UPDATE sidebar_nodes SET sort_order = ? WHERE id = ? AND campaign_id = ?`)
	if err != nil {
		return fmt.Errorf("preparing node resequence update: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for position, id := range orderedIDs {
		if _, err := stmt.ExecContext(ctx, position, id, campaignID); err != nil {
			return fmt.Errorf("resequencing node %s: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing node resequence: %w", err)
	}
	return nil
}

// generateNodeID creates a new UUID for a sidebar node.
func generateNodeID() string {
	return uuid.New().String()
}

// sidebarTreeType resolves the entity type the sidebar tree advertises as
// data-entity-type-id (read by sidebar_tree.js when it creates an empty folder,
// i.e. a sidebar_nodes row). It prefers the drilled category's own type — the
// sidebar URL's ?type= param — so the new node is scoped to the category and
// reloads via ListByType(typeID) on the next refresh. A category listing rolls
// up its sub-type entities (expandTypeIDsForListing), so results[0].EntityTypeID
// may be a SUB-type; scoping a folder node to that sub-type made it invisible on
// refresh and silently orphaned the dropped entities (the "creating a folder does
// nothing" bug). The row/node fallbacks only apply when no category type is
// supplied, so the attribute is never absent. This is a plain Go helper rather
// than a templ conditional-attribute chain because templ (v0.3.x) does not fold
// else-if branches for attributes — it emits each as an independent block plus a
// stray literal "else".
func sidebarTreeType(typeID int, results []Entity, nodes []SidebarNode) int {
	if typeID > 0 {
		return typeID
	}
	if len(results) > 0 {
		return results[0].EntityTypeID
	}
	if len(nodes) > 0 {
		return nodes[0].EntityTypeID
	}
	return 0
}
