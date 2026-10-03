// repository.go provides data access for the Armory gallery. Queries the
// existing entities + entity_types tables — no separate table is needed
// because items are entities with entity types having preset_category = 'item'.
//
// SECURITY: this repository applies NO visibility predicate of its own.
// A hand-rolled `role < 2 AND e.is_private = false` check here never
// consulted entities.visibility or entity_permissions, so a
// visibility='custom' entity (which doesn't clear is_private) stayed
// listed to Players and anonymous visitors. The visibility decision
// belongs to the service layer, which narrows the ID list returned
// here through the entities plugin's canonical FilterViewableEntityIDs
// — see service.go's visibleItemIDs.
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
	// ListItemIDs returns ALL item entity IDs matching the campaign + optional
	// type/search/tag/instance filters (excluding templates), in the gallery's
	// sort order, with NO visibility restriction and NO pagination applied.
	// The service layer narrows this list to what the viewer may see (via the
	// canonical entities visibility policy) BEFORE paginating, so the visible
	// list and its count are always derived from the exact same filtered ID
	// set and can never disagree.
	ListItemIDs(ctx context.Context, campaignID string, itemTypeIDs []int, opts ItemListOptions) ([]string, error)

	// GetItemCardsByIDs returns full item card data for exactly the given IDs
	// (in that order), scoped to campaignID. Used to fetch one page's worth of
	// cards after the service has already narrowed and paginated the ID list.
	GetItemCardsByIDs(ctx context.Context, campaignID string, ids []string) ([]ItemCard, error)
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

// itemIDsWhereAndJoins builds the shared WHERE clause + optional joins used by
// both ListItemIDs and (historically) the paginated card query, so the type/
// search/tag/instance filters can never drift between the id pass and a full
// row fetch. Returns the join fragments, the WHERE clause, and its args.
func itemIDsWhereAndJoins(campaignID string, itemTypeIDs []int, opts ItemListOptions) (instanceJoin, tagJoin, where string, args []any) {
	where = "WHERE e.campaign_id = ? AND e.is_template = false AND e.deleted_at IS NULL"
	args = []any{campaignID}

	// A specific type narrows the item types but must never widen them: an
	// arbitrary entity type id would otherwise list non-item entities here.
	if opts.TypeID > 0 {
		if containsInt(itemTypeIDs, opts.TypeID) {
			where += " AND e.entity_type_id = ?"
			args = append(args, opts.TypeID)
		} else {
			where += " AND 1 = 0"
		}
	} else if len(itemTypeIDs) > 0 {
		placeholders := make([]string, len(itemTypeIDs))
		for i, id := range itemTypeIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where += " AND e.entity_type_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	// Instance filter: join inventory_items to scope to a specific collection.
	if opts.InstanceID > 0 {
		instanceJoin = " INNER JOIN inventory_items ii ON ii.entity_id = e.id AND ii.instance_id = ?"
		args = append(args, opts.InstanceID)
	}

	// Optional name search.
	if opts.Search != "" {
		escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(opts.Search)
		where += " AND e.name LIKE ?"
		args = append(args, "%"+escaped+"%")
	}

	// Optional tag filter.
	if opts.Tag != "" {
		tagJoin = " INNER JOIN entity_tags etg ON etg.entity_id = e.id INNER JOIN tags t ON t.id = etg.tag_id AND t.slug = ?"
		args = append(args, opts.Tag)
	}

	return instanceJoin, tagJoin, where, args
}

// ListItemIDs fetches every item entity ID matching the campaign + type/
// search/tag/instance filters, unfiltered by visibility and unpaginated. See
// the ArmoryRepository doc comment for why visibility is deliberately absent
// here.
func (r *armoryRepository) ListItemIDs(ctx context.Context, campaignID string, itemTypeIDs []int, opts ItemListOptions) ([]string, error) {
	instanceJoin, tagJoin, where, args := itemIDsWhereAndJoins(campaignID, itemTypeIDs, opts)

	query := fmt.Sprintf(`SELECT e.id FROM entities e%s%s %s GROUP BY e.id %s`,
		instanceJoin, tagJoin, where, opts.OrderByClause())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing item ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning item id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetItemCardsByIDs fetches full card data for exactly the given ids, scoped
// to campaignID, preserving the caller's order (the service has already
// applied visibility + pagination to the id list by this point).
func (r *armoryRepository) GetItemCardsByIDs(ctx context.Context, campaignID string, ids []string) ([]ItemCard, error) {
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
		ORDER BY FIELD(e.id, %s)`, itemSelectColumns, inClause, inClause)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetching item cards: %w", err)
	}
	defer rows.Close()

	var cards []ItemCard
	for rows.Next() {
		card, err := scanItemCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, *card)
	}
	return cards, rows.Err()
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

// containsInt reports whether v is in xs.
func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
