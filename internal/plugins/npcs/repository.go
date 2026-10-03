// repository.go provides data access for the NPC gallery. Queries the existing
// entities + entity_types tables — no separate NPC table is needed because
// NPCs are just character entities filtered by visibility.
//
// SECURITY: this repository applies NO visibility predicate of its own — a
// hand-rolled `role < 2 AND is_private = false` check here would miss
// visibility='custom' entities. The service layer narrows the ID list
// through the entities plugin's canonical FilterViewableEntityIDs (via
// EntityVisibilityFilter in service.go); see service.go's visibleNPCIDs for
// the policy this repository deliberately does not implement.
package npcs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// NPCRepository defines the data access contract for NPC gallery queries.
type NPCRepository interface {
	// ListRevealedIDs returns ALL unclaimed character entity IDs matching the
	// campaign + any of the character types + optional search/tag filters (excluding templates), in
	// the gallery's sort order, with NO visibility restriction and NO
	// pagination applied. The service layer narrows this list to what the
	// viewer may see (via the canonical entities visibility policy) BEFORE
	// paginating, so the visible list and its count are always derived from
	// the exact same filtered ID set and can never disagree.
	ListRevealedIDs(ctx context.Context, campaignID string, characterTypeIDs []int, opts NPCListOptions) ([]string, error)

	// GetNPCCardsByIDs returns full NPC card data for exactly the given IDs
	// (in that order), scoped to campaignID. Used to fetch one page's worth of
	// cards after the service has already narrowed and paginated the ID list.
	GetNPCCardsByIDs(ctx context.Context, campaignID string, ids []string) ([]NPCCard, error)
}

// npcRepository implements NPCRepository with MariaDB queries against the
// entities table.
type npcRepository struct {
	db *sql.DB
}

// NewNPCRepository creates a new NPC repository backed by the given database.
func NewNPCRepository(db *sql.DB) NPCRepository {
	return &npcRepository{db: db}
}

// npcSelectColumns is the column list for NPC gallery queries.
const npcSelectColumns = `e.id, e.name, e.slug, e.image_path, e.type_label,
	e.is_private, e.fields_data,
	et.name, et.icon, et.color`

// ListRevealedIDs fetches every character entity ID matching the campaign +
// character types + search/tag filters, unfiltered by visibility and
// unpaginated. See the NPCRepository doc comment for why visibility is
// deliberately absent here. Claimed entities are left out: they are player
// characters and belong to the Characters page's party section.
func (r *npcRepository) ListRevealedIDs(ctx context.Context, campaignID string, characterTypeIDs []int, opts NPCListOptions) ([]string, error) {
	if len(characterTypeIDs) == 0 {
		return nil, nil
	}
	typePlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(characterTypeIDs)), ",")
	where := "WHERE e.campaign_id = ? AND e.entity_type_id IN (" + typePlaceholders + ") AND e.is_template = false AND e.owner_user_id IS NULL AND e.deleted_at IS NULL"
	args := []any{campaignID}
	for _, id := range characterTypeIDs {
		args = append(args, id)
	}

	// Optional name search.
	if opts.Search != "" {
		escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(opts.Search)
		where += " AND e.name LIKE ?"
		args = append(args, "%"+escaped+"%")
	}

	// Optional tag filter — join through entity_tags.
	tagJoin := ""
	if opts.Tag != "" {
		tagJoin = " INNER JOIN entity_tags etg ON etg.entity_id = e.id INNER JOIN tags t ON t.id = etg.tag_id AND t.slug = ?"
		args = append(args, opts.Tag)
	}

	query := fmt.Sprintf(`SELECT e.id FROM entities e%s %s GROUP BY e.id %s`,
		tagJoin, where, opts.OrderByClause())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing NPC ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning NPC id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetNPCCardsByIDs fetches full card data for exactly the given ids, scoped
// to campaignID, preserving the caller's order (the service has already
// applied visibility + pagination to the id list by this point).
func (r *npcRepository) GetNPCCardsByIDs(ctx context.Context, campaignID string, ids []string) ([]NPCCard, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	idArgs := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		idArgs[i] = id
	}
	inClause := strings.Join(placeholders, ",")

	args := make([]any, 0, 1+len(ids)*2)
	args = append(args, campaignID)
	args = append(args, idArgs...)
	args = append(args, idArgs...)

	query := fmt.Sprintf(`SELECT %s
		FROM entities e
		INNER JOIN entity_types et ON et.id = e.entity_type_id
		WHERE e.campaign_id = ? AND e.deleted_at IS NULL AND e.id IN (%s)
		ORDER BY FIELD(e.id, %s)`, npcSelectColumns, inClause, inClause)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetching NPC cards: %w", err)
	}
	defer rows.Close()

	var cards []NPCCard
	for rows.Next() {
		card, err := scanNPCCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, *card)
	}
	return cards, rows.Err()
}

// scanNPCCard reads a single NPC card row from the result set.
func scanNPCCard(rows *sql.Rows) (*NPCCard, error) {
	c := &NPCCard{}
	var fieldsRaw []byte
	err := rows.Scan(
		&c.ID, &c.Name, &c.Slug, &c.ImagePath, &c.TypeLabel,
		&c.IsPrivate, &fieldsRaw,
		&c.TypeName, &c.TypeIcon, &c.TypeColor,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning NPC card: %w", err)
	}

	c.Fields = make(map[string]any)
	if len(fieldsRaw) > 0 {
		if err := json.Unmarshal(fieldsRaw, &c.Fields); err != nil {
			return nil, fmt.Errorf("unmarshaling NPC fields: %w", err)
		}
	}
	return c, nil
}
