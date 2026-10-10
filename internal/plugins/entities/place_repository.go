package entities

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// PlaceRepository owns the entity_places table: the extra spots a page is
// listed in the page tree. It knows nothing about who may see a page; reads
// only guarantee that both ends are live (not in the Trash) and in the named
// campaign, so a trashed page or parent hides its listing and Restore brings
// it back without touching these rows.
type PlaceRepository interface {
	// Insert adds a listing. Adding one that already exists is a no-op, so a
	// double click or a replayed import never fails or duplicates.
	Insert(ctx context.Context, p *Place) error
	// Delete removes one listing; removing one that is not there is a no-op.
	Delete(ctx context.Context, entityID, parentID string) error
	// ListOf returns the live listings of one live page.
	ListOf(ctx context.Context, campaignID, entityID string) ([]PlaceLink, error)
	// ListUnder returns the live listings whose live parent is one of
	// parentIDs, for drawing those parents' branches of the tree.
	ListUnder(ctx context.Context, campaignID string, parentIDs []string) ([]PlaceLink, error)
	// ListAll returns every listing in a campaign, trashed ends included, for
	// export: a page in the Trash is exported with its listings intact.
	ListAll(ctx context.Context, campaignID string) ([]Place, error)
	// AncestorIDs returns id and every page above it, following BOTH the real
	// parent and the extra listings, so a cycle through either kind is seen.
	// Trashed pages are walked too: a cycle through a page in the Trash would
	// come back to life on Restore.
	AncestorIDs(ctx context.Context, campaignID, id string) (map[string]bool, error)
}

type placeRepository struct {
	db *sql.DB
}

// NewPlaceRepository creates the MariaDB repository for extra listings.
func NewPlaceRepository(db *sql.DB) PlaceRepository {
	return &placeRepository{db: db}
}

func (r *placeRepository) Insert(ctx context.Context, p *Place) error {
	var by any
	if p.CreatedBy != "" {
		by = p.CreatedBy
	}
	// INSERT IGNORE makes a repeated listing a no-op. It also turns a foreign
	// key failure into a warning, so the service checks both pages are live
	// first; a page purged in between just leaves no row.
	_, err := r.db.ExecContext(ctx,
		`INSERT IGNORE INTO entity_places (entity_id, parent_entity_id, campaign_id, sort_order, created_by)
		 VALUES (?, ?, ?, ?, ?)`,
		p.EntityID, p.ParentEntityID, p.CampaignID, p.SortOrder, by)
	if err != nil {
		return fmt.Errorf("adding page listing: %w", err)
	}
	return nil
}

func (r *placeRepository) Delete(ctx context.Context, entityID, parentID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM entity_places WHERE entity_id = ? AND parent_entity_id = ?`, entityID, parentID)
	if err != nil {
		return fmt.Errorf("removing page listing: %w", err)
	}
	return nil
}

// placeLinkSelect joins both ends so a trashed page or parent, or one from
// another campaign, never reaches a caller. A listing under the page's own
// real parent is skipped: it would draw the page twice in one branch (the
// service deletes such rows when a page is moved there, this covers a move
// made by any other writer).
const placeLinkSelect = `SELECT e.id, e.name, et.icon, et.name, et.color, e.is_private, e.visibility, p.id, p.name, ep.sort_order
	FROM entity_places ep
	INNER JOIN entities e ON e.id = ep.entity_id AND e.deleted_at IS NULL AND e.campaign_id = ep.campaign_id
		AND (e.parent_id IS NULL OR e.parent_id <> ep.parent_entity_id)
	INNER JOIN entities p ON p.id = ep.parent_entity_id AND p.deleted_at IS NULL AND p.campaign_id = ep.campaign_id
	INNER JOIN entity_types et ON et.id = e.entity_type_id
	WHERE ep.campaign_id = ?`

func (r *placeRepository) ListOf(ctx context.Context, campaignID, entityID string) ([]PlaceLink, error) {
	return r.queryLinks(ctx, placeLinkSelect+` AND ep.entity_id = ? ORDER BY ep.sort_order ASC, p.name ASC`, campaignID, entityID)
}

func (r *placeRepository) ListUnder(ctx context.Context, campaignID string, parentIDs []string) ([]PlaceLink, error) {
	if len(parentIDs) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(parentIDs)+1)
	args = append(args, campaignID)
	for _, id := range parentIDs {
		args = append(args, id)
	}
	q := placeLinkSelect + ` AND ep.parent_entity_id IN (` +
		strings.TrimSuffix(strings.Repeat("?, ", len(parentIDs)), ", ") + `) ORDER BY ep.sort_order ASC, e.name ASC`
	return r.queryLinks(ctx, q, args...)
}

func (r *placeRepository) queryLinks(ctx context.Context, q string, args ...any) ([]PlaceLink, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing page listings: %w", err)
	}
	defer rows.Close()
	var out []PlaceLink
	for rows.Next() {
		var l PlaceLink
		var icon, color sql.NullString
		var vis string
		if err := rows.Scan(&l.EntityID, &l.EntityName, &icon, &l.EntityTypeName, &color, &l.EntityIsPrivate, &vis, &l.ParentID, &l.ParentName, &l.SortOrder); err != nil {
			return nil, fmt.Errorf("scanning page listing: %w", err)
		}
		l.EntityTypeIcon = icon.String
		l.EntityTypeColor = color.String
		l.EntityVisibility = VisibilityMode(vis)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *placeRepository) ListAll(ctx context.Context, campaignID string) ([]Place, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT entity_id, parent_entity_id, campaign_id, sort_order, COALESCE(created_by, ''), created_at
		 FROM entity_places WHERE campaign_id = ? ORDER BY entity_id, sort_order, parent_entity_id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing campaign page listings: %w", err)
	}
	defer rows.Close()
	var out []Place
	for rows.Next() {
		var p Place
		if err := rows.Scan(&p.EntityID, &p.ParentEntityID, &p.CampaignID, &p.SortOrder, &p.CreatedBy, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning campaign page listing: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *placeRepository) AncestorIDs(ctx context.Context, campaignID, id string) (map[string]bool, error) {
	// UNION (not UNION ALL) is what ends the walk if bad data ever holds a
	// cycle: a row already seen adds nothing, so the recursion runs dry.
	rows, err := r.db.QueryContext(ctx, `
		WITH RECURSIVE edges (child, parent) AS (
			SELECT id, parent_id FROM entities WHERE campaign_id = ? AND parent_id IS NOT NULL
			UNION ALL
			SELECT entity_id, parent_entity_id FROM entity_places WHERE campaign_id = ?
		),
		up (id) AS (
			SELECT CAST(? AS CHAR(36)) COLLATE utf8mb4_unicode_ci
			UNION
			SELECT edges.parent FROM up INNER JOIN edges ON edges.child = up.id
		)
		SELECT id FROM up`, campaignID, campaignID, id)
	if err != nil {
		return nil, fmt.Errorf("walking page ancestors: %w", err)
	}
	defer rows.Close()
	seen := make(map[string]bool)
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, fmt.Errorf("scanning page ancestor: %w", err)
		}
		seen[a] = true
	}
	return seen, rows.Err()
}
