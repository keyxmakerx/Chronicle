// page_pins.go — the pages on a campaign home's Pinned Pages widget: the ones
// the Director pins for everyone (page_pins) and each member's own, which are
// the pages they starred (entity_favorites).
package entities

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MaxSharedPins caps how many pages the Director can pin for everyone: the
// widget is a short list to start from, not a second page index.
const MaxSharedPins = 12

// maxOwnPinsShown is how many of a member's own pins the widget draws; the
// rest are counted, and all of them stay in the sidebar's Favorites.
const maxOwnPinsShown = 12

// PinnedPage is one page on the Pinned Pages widget, with what its card shows.
type PinnedPage struct {
	ID        string
	Name      string
	ImagePath *string
	TypeName  string
	TypeIcon  string
	TypeColor string
	PinnedAt  time.Time
}

// PagePinRepository reads and writes the Director's pins, and reads the
// member's own pins from their stars. Every read joins the live page, so a
// page in the Trash drops off and comes back if it is restored.
type PagePinRepository interface {
	// Shared returns the pages pinned for everyone, in the order pinned.
	Shared(ctx context.Context, campaignID string) ([]PinnedPage, error)
	// IsShared reports whether a page is pinned for everyone.
	IsShared(ctx context.Context, campaignID, entityID string) (bool, error)
	// CountShared counts the live pages pinned for everyone.
	CountShared(ctx context.Context, campaignID string) (int, error)
	// Share pins a page for everyone; pinning it again changes nothing.
	Share(ctx context.Context, campaignID, entityID, userID string) error
	// Unshare takes a page off everyone's pins.
	Unshare(ctx context.Context, campaignID, entityID string) error
	// Own returns the pages a member starred in a campaign, newest first.
	Own(ctx context.Context, campaignID, userID string) ([]PinnedPage, error)
	// RemoveOwn takes a page off a member's own pins (their star).
	RemoveOwn(ctx context.Context, campaignID, entityID, userID string) error
}

type pagePinRepository struct {
	db *sql.DB
}

// NewPagePinRepository creates the pins repository.
func NewPagePinRepository(db *sql.DB) PagePinRepository {
	return &pagePinRepository{db: db}
}

// pinnedPageCols are the page and category columns a pinned card shows, after
// the pin's own time.
const pinnedPageCols = `e.id, e.name, e.image_path, et.name, et.icon, et.color`

func (r *pagePinRepository) Shared(ctx context.Context, campaignID string) ([]PinnedPage, error) {
	return r.list(ctx, `SELECT `+pinnedPageCols+`, p.created_at
		FROM page_pins p
		JOIN entities e ON e.id = p.entity_id AND e.campaign_id = p.campaign_id AND e.deleted_at IS NULL
		JOIN entity_types et ON et.id = e.entity_type_id
		WHERE p.campaign_id = ?
		ORDER BY p.created_at, e.name`, campaignID)
}

func (r *pagePinRepository) Own(ctx context.Context, campaignID, userID string) ([]PinnedPage, error) {
	return r.list(ctx, `SELECT `+pinnedPageCols+`, f.created_at
		FROM entity_favorites f
		JOIN entities e ON e.id = f.entity_id AND e.campaign_id = f.campaign_id AND e.deleted_at IS NULL
		JOIN entity_types et ON et.id = e.entity_type_id
		WHERE f.campaign_id = ? AND f.user_id = ?
		ORDER BY f.created_at DESC, e.name`, campaignID, userID)
}

func (r *pagePinRepository) list(ctx context.Context, query string, args ...any) ([]PinnedPage, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing pinned pages: %w", err)
	}
	defer rows.Close()
	var out []PinnedPage
	for rows.Next() {
		var p PinnedPage
		var icon, color sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.ImagePath, &p.TypeName, &icon, &color, &p.PinnedAt); err != nil {
			return nil, fmt.Errorf("scanning pinned page: %w", err)
		}
		p.TypeIcon, p.TypeColor = icon.String, color.String
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *pagePinRepository) IsShared(ctx context.Context, campaignID, entityID string) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM page_pins WHERE campaign_id = ? AND entity_id = ?)`,
		campaignID, entityID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("reading page pin: %w", err)
	}
	return ok, nil
}

func (r *pagePinRepository) CountShared(ctx context.Context, campaignID string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM page_pins p
		JOIN entities e ON e.id = p.entity_id AND e.deleted_at IS NULL
		WHERE p.campaign_id = ?`, campaignID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting page pins: %w", err)
	}
	return n, nil
}

func (r *pagePinRepository) Share(ctx context.Context, campaignID, entityID, userID string) error {
	var by any
	if userID != "" {
		by = userID
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT IGNORE INTO page_pins (campaign_id, entity_id, pinned_by) VALUES (?, ?, ?)`,
		campaignID, entityID, by)
	if err != nil {
		return fmt.Errorf("pinning page: %w", err)
	}
	return nil
}

func (r *pagePinRepository) Unshare(ctx context.Context, campaignID, entityID string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM page_pins WHERE campaign_id = ? AND entity_id = ?`, campaignID, entityID); err != nil {
		return fmt.Errorf("unpinning page: %w", err)
	}
	return nil
}

func (r *pagePinRepository) RemoveOwn(ctx context.Context, campaignID, entityID, userID string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM entity_favorites WHERE campaign_id = ? AND entity_id = ? AND user_id = ?`,
		campaignID, entityID, userID); err != nil {
		return fmt.Errorf("removing own pin: %w", err)
	}
	return nil
}
