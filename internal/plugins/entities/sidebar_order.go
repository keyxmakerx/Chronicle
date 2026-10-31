package entities

import (
	"context"
	"database/sql"
	"fmt"
)

// The orders a sidebar container can keep its rows in. OrderAZ is the
// default and is never stored.
const (
	OrderAZ   = "az"
	OrderHand = "hand"
	OrderNew  = "new"
)

// The kinds of container that hold rows in the sidebar tree.
const (
	OrderContainerCategory = "category"
	OrderContainerFolder   = "folder"
	OrderContainerPage     = "page"
)

// validChildOrder reports whether mode is one a container can be set to.
func validChildOrder(mode string) bool {
	return mode == OrderAZ || mode == OrderHand || mode == OrderNew
}

// childOrderKey names a container in the map ChildOrders returns.
func childOrderKey(kind, id string) string { return kind + ":" + id }

// childOrderOf is the order stored for a container, or OrderAZ.
func childOrderOf(orders map[string]string, kind, id string) string {
	if m, ok := orders[childOrderKey(kind, id)]; ok && validChildOrder(m) {
		return m
	}
	return OrderAZ
}

// ChildOrderRepository stores how each sidebar container orders its rows.
type ChildOrderRepository interface {
	// ListByCampaign returns every stored order, keyed by childOrderKey.
	// A container that is missing sorts A to Z.
	ListByCampaign(ctx context.Context, campaignID string) (map[string]string, error)
	// Set stores mode for one container; OrderAZ removes the row.
	Set(ctx context.Context, campaignID, kind, id, mode string) error
}

type childOrderRepository struct {
	db *sql.DB
}

// NewChildOrderRepository creates the sidebar order repository.
func NewChildOrderRepository(db *sql.DB) ChildOrderRepository {
	return &childOrderRepository{db: db}
}

func (r *childOrderRepository) ListByCampaign(ctx context.Context, campaignID string) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT container_kind, container_id, mode FROM sidebar_child_orders WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing sidebar orders: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var kind, id, mode string
		if err := rows.Scan(&kind, &id, &mode); err != nil {
			return nil, fmt.Errorf("scanning sidebar order: %w", err)
		}
		out[childOrderKey(kind, id)] = mode
	}
	return out, rows.Err()
}

func (r *childOrderRepository) Set(ctx context.Context, campaignID, kind, id, mode string) error {
	if mode == OrderAZ {
		_, err := r.db.ExecContext(ctx,
			`DELETE FROM sidebar_child_orders WHERE campaign_id = ? AND container_kind = ? AND container_id = ?`,
			campaignID, kind, id)
		if err != nil {
			return fmt.Errorf("clearing sidebar order: %w", err)
		}
		return nil
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sidebar_child_orders (campaign_id, container_kind, container_id, mode)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE mode = VALUES(mode)`,
		campaignID, kind, id, mode)
	if err != nil {
		return fmt.Errorf("saving sidebar order: %w", err)
	}
	return nil
}
