package campaigns

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// This file is the SQL behind the site Trash. A campaign in the Trash keeps
// every row it has; only deleted_at marks it, and every read that opens a
// campaign (repository.go) filters on that column. updated_at is set to itself
// in each statement because the column otherwise bumps on any update, and
// Undo must give the campaign back exactly as it was.

// MoveToTrash marks a campaign as deleted. A campaign already in the Trash
// (or gone) is "not found", so two people deleting at once cannot both win.
func (r *campaignRepository) MoveToTrash(ctx context.Context, id, byUserID, byName string, at time.Time) error {
	var by, name any
	if byUserID != "" {
		by = byUserID
	}
	if byName != "" {
		name = byName
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE campaigns
		 SET deleted_at = ?, deleted_by = ?, deleted_by_name = ?, updated_at = updated_at
		 WHERE id = ? AND deleted_at IS NULL`, at, by, name, id)
	if err != nil {
		return fmt.Errorf("moving campaign to trash: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("campaign not found")
	}
	return nil
}

// RestoreFromTrash brings a trashed campaign back. It refuses once the final
// delete has started (purge_started_at), so an Undo can never land on a
// campaign whose files are already being removed.
func (r *campaignRepository) RestoreFromTrash(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE campaigns
		 SET deleted_at = NULL, deleted_by = NULL, deleted_by_name = NULL, updated_at = updated_at
		 WHERE id = ? AND deleted_at IS NOT NULL AND purge_started_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("restoring campaign from trash: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("that campaign is not in the trash any more")
	}
	return nil
}

// ListTrashed returns every campaign in the Trash, newest deletion first,
// with the bytes of media it holds (storage still counts while it waits).
func (r *campaignRepository) ListTrashed(ctx context.Context) ([]TrashedCampaign, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT c.id, c.name, c.slug, c.deleted_at, c.deleted_by, COALESCE(c.deleted_by_name, ''),
		        c.purge_started_at IS NOT NULL,
		        COALESCE((SELECT SUM(m.file_size) FROM media_files m WHERE m.campaign_id = c.id), 0)
		 FROM campaigns c
		 WHERE c.deleted_at IS NOT NULL
		 ORDER BY c.deleted_at DESC, c.id`)
	if err != nil {
		return nil, fmt.Errorf("listing trashed campaigns: %w", err)
	}
	defer rows.Close()

	var out []TrashedCampaign
	for rows.Next() {
		var t TrashedCampaign
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.DeletedAt, &t.DeletedBy, &t.DeletedByName,
			&t.Emptying, &t.StorageBytes); err != nil {
			return nil, fmt.Errorf("scanning trashed campaign: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListPurgeDue returns the ids of trashed campaigns whose time is up, plus any
// whose final delete started and never finished, oldest first. all returns
// every trashed campaign whatever its age (Empty now).
func (r *campaignRepository) ListPurgeDue(ctx context.Context, cutoff time.Time, all bool) ([]string, error) {
	q := `SELECT id FROM campaigns
	      WHERE deleted_at IS NOT NULL AND (purge_started_at IS NOT NULL OR deleted_at < ?)
	      ORDER BY deleted_at, id`
	args := []any{cutoff}
	if all {
		q = `SELECT id FROM campaigns WHERE deleted_at IS NOT NULL ORDER BY deleted_at, id`
		args = nil
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing campaigns due for purge: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning campaign id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ClaimForPurge records that the final delete of a trashed campaign has begun.
// It reports false when the campaign is not in the Trash (never was, was
// undone, or already purged). Claiming twice is fine: the first time stamps
// purge_started_at and later runs keep it, so a purge that stopped halfway is
// resumed rather than refused.
func (r *campaignRepository) ClaimForPurge(ctx context.Context, id string, at time.Time) (bool, error) {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE campaigns SET purge_started_at = COALESCE(purge_started_at, ?), updated_at = updated_at
		 WHERE id = ? AND deleted_at IS NOT NULL`, at, id); err != nil {
		return false, fmt.Errorf("claiming campaign for purge: %w", err)
	}
	var claimed bool
	err := r.db.QueryRowContext(ctx,
		`SELECT purge_started_at IS NOT NULL FROM campaigns WHERE id = ? AND deleted_at IS NOT NULL`, id).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking purge claim: %w", err)
	}
	return claimed, nil
}

// PurgeTrashed hard-deletes a campaign that was trashed and claimed. The
// conditions repeat the claim so a campaign that was undone, or never
// trashed, is never touched by this statement. FK cascades remove its rows.
func (r *campaignRepository) PurgeTrashed(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM campaigns WHERE id = ? AND deleted_at IS NOT NULL AND purge_started_at IS NOT NULL`, id)
	if err != nil {
		return fmt.Errorf("purging campaign: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("campaign not found")
	}
	return nil
}
