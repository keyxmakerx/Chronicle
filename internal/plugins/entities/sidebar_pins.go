package entities

import (
	"context"
	"database/sql"
	"fmt"
)

// A pinned page leads its own list in the sidebar (the folder, page or
// category top level it sits in), above a thin rule, whatever order that
// list keeps. Pins are the Director team's and every reader sees them.

// MaxSidebarPins bounds how many pages one campaign may pin, so pins stay a
// short lead rather than a second order.
const MaxSidebarPins = 100

// SidebarPinRepository stores the sidebar's pinned pages.
type SidebarPinRepository interface {
	// ListByCampaign returns each pinned page's position, lowest first.
	ListByCampaign(ctx context.Context, campaignID string) (map[string]int, error)
	// Pin puts a page last among the pins; pinning a pinned page keeps its
	// place.
	Pin(ctx context.Context, campaignID, entityID, userID string) error
	// Unpin removes a page's pin, if it has one.
	Unpin(ctx context.Context, campaignID, entityID string) error
}

type sidebarPinRepository struct{ db *sql.DB }

// NewSidebarPinRepository creates the MariaDB-backed repository.
func NewSidebarPinRepository(db *sql.DB) SidebarPinRepository { return &sidebarPinRepository{db: db} }

func (r *sidebarPinRepository) ListByCampaign(ctx context.Context, campaignID string) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT entity_id, position FROM sidebar_pins WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing sidebar pins: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var pos int
		if err := rows.Scan(&id, &pos); err != nil {
			return nil, fmt.Errorf("scanning sidebar pin: %w", err)
		}
		out[id] = pos
	}
	return out, rows.Err()
}

func (r *sidebarPinRepository) Pin(ctx context.Context, campaignID, entityID, userID string) error {
	// The position is read in the same statement, so two pins at once still
	// land in some order rather than on one place.
	_, err := r.db.ExecContext(ctx,
		`INSERT IGNORE INTO sidebar_pins (campaign_id, entity_id, position, updated_by)
		 SELECT ?, ?, COALESCE(MAX(position), -1) + 1, ? FROM sidebar_pins WHERE campaign_id = ?`,
		campaignID, entityID, userID, campaignID)
	if err != nil {
		return fmt.Errorf("pinning sidebar page: %w", err)
	}
	return nil
}

func (r *sidebarPinRepository) Unpin(ctx context.Context, campaignID, entityID string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM sidebar_pins WHERE campaign_id = ? AND entity_id = ?`, campaignID, entityID); err != nil {
		return fmt.Errorf("unpinning sidebar page: %w", err)
	}
	return nil
}

type sidebarPinsKey struct{}

// withSidebarPins carries the pinned pages to the sidebar list template, as
// the tree's listings are carried, so its signature stays the same.
func withSidebarPins(ctx context.Context, pins map[string]int) context.Context {
	return context.WithValue(ctx, sidebarPinsKey{}, pins)
}

// sidebarPinOf is a page's pin position and whether it is pinned.
func sidebarPinOf(ctx context.Context, entityID string) (int, bool) {
	m, _ := ctx.Value(sidebarPinsKey{}).(map[string]int)
	p, ok := m[entityID]
	return p, ok
}
