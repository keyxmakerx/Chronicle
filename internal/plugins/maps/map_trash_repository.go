package maps

import (
	"context"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// This file is the SQL behind a campaign's Trash for maps. A trashed map
// keeps its row and everything hanging off it; only deleted_at marks it, so
// Restore is one update and the purge's DeleteMap lets the foreign keys
// cascade.

// TrashedMap is a map as the campaign Trash lists it.
type TrashedMap struct {
	ID         string
	CampaignID string
	Name       string
	DeletedAt  time.Time
}

// TrashMap marks one of the campaign's maps as trashed.
func (r *mapRepo) TrashMap(ctx context.Context, campaignID, id string, at time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE maps SET deleted_at = ? WHERE id = ? AND campaign_id = ? AND deleted_at IS NULL`,
		at, id, campaignID)
	if err != nil {
		return fmt.Errorf("moving map to trash: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("map not found")
	}
	return nil
}

// RestoreMap takes a trashed map out of the campaign Trash.
func (r *mapRepo) RestoreMap(ctx context.Context, campaignID, id string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE maps SET deleted_at = NULL WHERE id = ? AND campaign_id = ? AND deleted_at IS NOT NULL`,
		id, campaignID)
	if err != nil {
		return fmt.Errorf("restoring map: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("that map isn't in the Trash")
	}
	return nil
}

// ListTrashedMaps lists a campaign's trashed maps, newest first.
func (r *mapRepo) ListTrashedMaps(ctx context.Context, campaignID string) ([]TrashedMap, error) {
	return r.trashedMaps(ctx,
		`SELECT id, campaign_id, name, deleted_at FROM maps
		 WHERE campaign_id = ? AND deleted_at IS NOT NULL
		 ORDER BY deleted_at DESC`, campaignID)
}

// ListMapsTrashedBefore lists the maps trashed before cutoff, in any
// campaign, for the purge.
func (r *mapRepo) ListMapsTrashedBefore(ctx context.Context, cutoff time.Time) ([]TrashedMap, error) {
	return r.trashedMaps(ctx,
		`SELECT id, campaign_id, name, deleted_at FROM maps
		 WHERE deleted_at IS NOT NULL AND deleted_at < ?
		 ORDER BY deleted_at`, cutoff)
}

func (r *mapRepo) trashedMaps(ctx context.Context, query string, args ...any) ([]TrashedMap, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing trashed maps: %w", err)
	}
	defer rows.Close()
	var out []TrashedMap
	for rows.Next() {
		var t TrashedMap
		if err := rows.Scan(&t.ID, &t.CampaignID, &t.Name, &t.DeletedAt); err != nil {
			return nil, fmt.Errorf("scanning trashed map: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
