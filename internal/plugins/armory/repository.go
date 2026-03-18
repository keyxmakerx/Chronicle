// repository.go provides data access for the Armory gallery. Queries the
// existing entities + entity_types tables — no separate table is needed
// because items are entities with entity types having preset_category = 'item'.
package armory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// ArmoryRepository defines the data access contract for Armory gallery queries.
type ArmoryRepository interface {
	// ListItems returns item entities visible to the given role/user,
	// with pagination and optional filters.
	ListItems(ctx context.Context, campaignID string, itemTypeIDs []int, role int, userID string, opts ItemListOptions) ([]ItemCard, int, error)

	// CountItems returns the number of visible items for badge display.
	CountItems(ctx context.Context, campaignID string, itemTypeIDs []int, role int, userID string) (int, error)
}

// armoryRepository implements ArmoryRepository with MariaDB queries.
type armoryRepository struct {
	db *sql.DB
}

// NewArmoryRepository creates a new Armory repository backed by the given database.
func NewArmoryRepository(db *sql.DB) ArmoryRepository {
	return &armoryRepository{db: db}
}

// itemSelectColumns is the column list for Armory gallery queries.
const itemSelectColumns = `e.id, e.name, e.slug, e.image_path, e.type_label,
	e.is_private, e.fields_data,
	et.name, et.icon, et.color`

// ListItems fetches item entities for the Armory gallery.
// Filters by item-category entity types and applies visibility rules.
func (r *armoryRepository) ListItems(ctx context.Context, campaignID string, itemTypeIDs []int, role int, userID string, opts ItemListOptions) ([]ItemCard, int, error) {
	where := "WHERE e.campaign_id = ? AND e.is_template = false"
	args := []any{campaignID}

	// Filter by item type IDs (or a specific type if TypeID set).
	if opts.TypeID > 0 {
		where += " AND e.entity_type_id = ?"
		args = append(args, opts.TypeID)
	} else if len(itemTypeIDs) > 0 {
		placeholders := make([]string, len(itemTypeIDs))
		for i, id := range itemTypeIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where += " AND e.entity_type_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	// Instance filter: join inventory_items to scope to a specific collection.
	instanceJoin := ""
	if opts.InstanceID > 0 {
		instanceJoin = " INNER JOIN inventory_items ii ON ii.entity_id = e.id AND ii.instance_id = ?"
		args = append(args, opts.InstanceID)
	}

	// Visibility: players see only non-private items.
	if role < 2 {
		where += " AND e.is_private = false"
	}

	// Optional name search.
	if opts.Search != "" {
		escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(opts.Search)
		where += " AND e.name LIKE ?"
		args = append(args, "%"+escaped+"%")
	}

	// Optional tag filter.
	tagJoin := ""
	if opts.Tag != "" {
		tagJoin = " INNER JOIN entity_tags etg ON etg.entity_id = e.id INNER JOIN tags t ON t.id = etg.tag_id AND t.slug = ?"
		args = append(args, opts.Tag)
	}

	// Count total for pagination.
	countQuery := fmt.Sprintf("SELECT COUNT(DISTINCT e.id) FROM entities e%s%s %s", instanceJoin, tagJoin, where)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting items: %w", err)
	}

	// Fetch page.
	query := fmt.Sprintf(`SELECT %s
		FROM entities e
		INNER JOIN entity_types et ON et.id = e.entity_type_id
		%s
		%s
		%s
		GROUP BY e.id
		%s
		LIMIT ? OFFSET ?`, itemSelectColumns, instanceJoin, tagJoin, where, opts.OrderByClause())

	pageArgs := append(args, opts.PerPage, opts.Offset())
	rows, err := r.db.QueryContext(ctx, query, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing items: %w", err)
	}
	defer rows.Close()

	var cards []ItemCard
	for rows.Next() {
		card, err := scanItemCard(rows)
		if err != nil {
			return nil, 0, err
		}
		cards = append(cards, *card)
	}
	return cards, total, rows.Err()
}

// CountItems returns the count of visible item entities for badge display.
func (r *armoryRepository) CountItems(ctx context.Context, campaignID string, itemTypeIDs []int, role int, userID string) (int, error) {
	where := "WHERE e.campaign_id = ? AND e.is_template = false"
	args := []any{campaignID}

	if len(itemTypeIDs) > 0 {
		placeholders := make([]string, len(itemTypeIDs))
		for i, id := range itemTypeIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where += " AND e.entity_type_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	if role < 2 {
		where += " AND e.is_private = false"
	}

	query := fmt.Sprintf("SELECT COUNT(*) FROM entities e %s", where)
	var count int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting items: %w", err)
	}
	return count, nil
}

// scanItemCard reads a single item card row from the result set.
func scanItemCard(rows *sql.Rows) (*ItemCard, error) {
	c := &ItemCard{}
	var fieldsRaw []byte
	err := rows.Scan(
		&c.ID, &c.Name, &c.Slug, &c.ImagePath, &c.TypeLabel,
		&c.IsPrivate, &fieldsRaw,
		&c.TypeName, &c.TypeIcon, &c.TypeColor,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning item card: %w", err)
	}

	c.Fields = make(map[string]any)
	if len(fieldsRaw) > 0 {
		if err := json.Unmarshal(fieldsRaw, &c.Fields); err != nil {
			return nil, fmt.Errorf("unmarshaling item fields: %w", err)
		}
	}
	return c, nil
}
