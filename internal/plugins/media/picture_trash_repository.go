package media

import (
	"context"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// This file is the SQL behind a campaign's Trash for pictures. A trashed
// picture keeps its row and its file, and FindByID still serves it, so pages
// using it keep showing it until the purge. Only the media browser, the
// picker, the sync list, the export and upload dedup leave it out.

// TrashedPicture is a campaign picture as the campaign Trash lists it.
type TrashedPicture struct {
	ID        string
	Name      string
	DeletedAt time.Time
}

// TrashCampaignPicture marks one of the campaign's own pictures as trashed.
// A note picture or page file is not the campaign's to trash, and an id that
// matches nothing is not found.
func (r *mediaRepository) TrashCampaignPicture(ctx context.Context, campaignID, id string, at time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE media_files SET deleted_at = ?
		 WHERE id = ? AND campaign_id = ? AND deleted_at IS NULL AND usage_type NOT IN (?, ?)`,
		at, id, campaignID, UsageNoteImage, UsagePageFile)
	if err != nil {
		return fmt.Errorf("moving picture to trash: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("media file not found")
	}
	return nil
}

// RestoreCampaignPicture takes a trashed picture out of the campaign Trash.
func (r *mediaRepository) RestoreCampaignPicture(ctx context.Context, campaignID, id string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE media_files SET deleted_at = NULL WHERE id = ? AND campaign_id = ? AND deleted_at IS NOT NULL`,
		id, campaignID)
	if err != nil {
		return fmt.Errorf("restoring picture: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("that picture isn't in the Trash")
	}
	return nil
}

// ListTrashedPictures lists a campaign's trashed pictures, newest first.
func (r *mediaRepository) ListTrashedPictures(ctx context.Context, campaignID string) ([]TrashedPicture, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, original_name, deleted_at FROM media_files
		 WHERE campaign_id = ? AND deleted_at IS NOT NULL
		 ORDER BY deleted_at DESC`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing trashed pictures: %w", err)
	}
	defer rows.Close()
	var out []TrashedPicture
	for rows.Next() {
		var t TrashedPicture
		if err := rows.Scan(&t.ID, &t.Name, &t.DeletedAt); err != nil {
			return nil, fmt.Errorf("scanning trashed picture: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListPicturesTrashedBefore lists the ids of pictures trashed before cutoff,
// in any campaign, for the purge.
func (r *mediaRepository) ListPicturesTrashedBefore(ctx context.Context, cutoff time.Time) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM media_files WHERE deleted_at IS NOT NULL AND deleted_at < ? ORDER BY deleted_at`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("listing pictures to purge: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning picture to purge: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
