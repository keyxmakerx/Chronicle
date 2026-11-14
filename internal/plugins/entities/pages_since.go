package entities

// What the campaign home's "Since you were last here" block reads of pages:
// how many were created after a time and what the newest are called.
//
// Both reads carry visibilityFilter, the one predicate every list, search and
// count shares, in the same WHERE as the time bound. The count is therefore
// taken over the pages the viewer may open and never over all pages and then
// trimmed, so it cannot reveal how many hidden pages exist.

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PagesSince is the pages a viewer may open that were created after a time.
type PagesSince struct {
	// Count is every such page; Newest names up to the asked-for few, newest
	// first.
	Count  int
	Newest []PageSinceRef
}

// PageSinceRef is one new page.
type PageSinceRef struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// PagesSinceReader reads new pages for a viewer.
type PagesSinceReader struct {
	db *sql.DB
}

// NewPagesSinceReader creates the reader.
func NewPagesSinceReader(db *sql.DB) *PagesSinceReader { return &PagesSinceReader{db: db} }

// Since counts the live, non-template pages of the campaign created after
// since that the viewer (role and user id, as every other entity read takes
// them) may open, and returns the newest nameLimit of them. It uses
// idx_entities_campaign_created for the campaign and time bound.
func (r *PagesSinceReader) Since(ctx context.Context, campaignID string, since time.Time, role int, userID string, nameLimit int) (PagesSince, error) {
	visFilter, visArgs := visibilityFilter(role, userID)
	where := ` FROM entities e WHERE e.campaign_id = ? AND e.created_at > ? AND e.is_template = 0` + visFilter
	args := append([]any{campaignID, since}, visArgs...)

	var out PagesSince
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*)`+where, args...).Scan(&out.Count); err != nil {
		return PagesSince{}, fmt.Errorf("counting pages created since: %w", err)
	}
	if out.Count == 0 || nameLimit <= 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT e.id, e.name, e.created_at`+where+` ORDER BY e.created_at DESC, e.id LIMIT ?`,
		append(args, nameLimit)...)
	if err != nil {
		return PagesSince{}, fmt.Errorf("naming pages created since: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p PageSinceRef
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return PagesSince{}, fmt.Errorf("scanning page created since: %w", err)
		}
		out.Newest = append(out.Newest, p)
	}
	return out, rows.Err()
}
