// repository.go provides data access for the NPC gallery. Queries the existing
// entities + entity_types tables — no separate NPC table is needed because
// NPCs are just character entities filtered by visibility.
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
	// ListRevealed returns non-private character entities visible to the
	// given role/user, with pagination and optional filters.
	ListRevealed(ctx context.Context, campaignID string, characterTypeID int, role int, userID string, opts NPCListOptions) ([]NPCCard, int, error)

	// CountRevealed returns the number of visible NPCs for badge display.
	CountRevealed(ctx context.Context, campaignID string, characterTypeID int, role int, userID string) (int, error)
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

// ListRevealed fetches revealed character entities for the NPC gallery.
// Filters by character entity type and applies visibility rules based on role.
func (r *npcRepository) ListRevealed(ctx context.Context, campaignID string, characterTypeID int, role int, userID string, opts NPCListOptions) ([]NPCCard, int, error) {
	where := "WHERE e.campaign_id = ? AND e.entity_type_id = ? AND e.is_template = false"
	args := []any{campaignID, characterTypeID}

	// For players, only show non-private NPCs. Scribes/Owners see all.
	if role < 2 {
		where += " AND e.is_private = false"
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

	// Count total for pagination.
	countQuery := fmt.Sprintf("SELECT COUNT(DISTINCT e.id) FROM entities e%s %s", tagJoin, where)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting NPCs: %w", err)
	}

	// Fetch page.
	query := fmt.Sprintf(`SELECT %s
		FROM entities e
		INNER JOIN entity_types et ON et.id = e.entity_type_id
		%s
		%s
		GROUP BY e.id
		%s
		LIMIT ? OFFSET ?`, npcSelectColumns, tagJoin, where, opts.OrderByClause())

	pageArgs := append(args, opts.PerPage, opts.Offset())
	rows, err := r.db.QueryContext(ctx, query, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing NPCs: %w", err)
	}
	defer rows.Close()

	var cards []NPCCard
	for rows.Next() {
		card, err := scanNPCCard(rows)
		if err != nil {
			return nil, 0, err
		}
		cards = append(cards, *card)
	}
	return cards, total, rows.Err()
}

// CountRevealed returns the count of visible character entities for badge display.
func (r *npcRepository) CountRevealed(ctx context.Context, campaignID string, characterTypeID int, role int, userID string) (int, error) {
	where := "WHERE e.campaign_id = ? AND e.entity_type_id = ? AND e.is_template = false"
	args := []any{campaignID, characterTypeID}

	if role < 2 {
		where += " AND e.is_private = false"
	}

	query := fmt.Sprintf("SELECT COUNT(*) FROM entities e %s", where)
	var count int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting revealed NPCs: %w", err)
	}
	return count, nil
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
