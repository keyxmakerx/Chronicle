package maps

import (
	"context"
	"database/sql"
	"fmt"
)

// pictureRefRepo is the SQL behind PictureRefStore.
type pictureRefRepo struct {
	db *sql.DB
}

// NewPictureRefStore creates a PictureRefStore backed by MariaDB.
func NewPictureRefStore(db *sql.DB) PictureRefStore {
	return &pictureRefRepo{db: db}
}

func (r *pictureRefRepo) list(ctx context.Context, query string) ([]PictureRef, error) {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PictureRef
	for rows.Next() {
		var p PictureRef
		if err := rows.Scan(&p.ID, &p.CampaignID, &p.Value); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListMapBackgrounds implements PictureRefStore.
func (r *pictureRefRepo) ListMapBackgrounds(ctx context.Context) ([]PictureRef, error) {
	return r.list(ctx, `SELECT id, campaign_id, image_id FROM maps WHERE image_id IS NOT NULL AND image_id <> ''`)
}

// ListTokenPictures implements PictureRefStore.
func (r *pictureRefRepo) ListTokenPictures(ctx context.Context) ([]PictureRef, error) {
	return r.list(ctx, `SELECT t.id, m.campaign_id, t.image_path
		FROM map_tokens t JOIN maps m ON m.id = t.map_id
		WHERE t.image_path IS NOT NULL AND t.image_path <> ''`)
}

func (r *pictureRefRepo) clear(ctx context.Context, query, id, expected string) (bool, error) {
	res, err := r.db.ExecContext(ctx, query, id, expected)
	if err != nil {
		return false, fmt.Errorf("clearing picture: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ClearMapBackground implements PictureRefStore.
func (r *pictureRefRepo) ClearMapBackground(ctx context.Context, mapID, expected string) (bool, error) {
	return r.clear(ctx, `UPDATE maps SET image_id = NULL WHERE id = ? AND image_id = ?`, mapID, expected)
}

// ClearTokenPicture implements PictureRefStore.
func (r *pictureRefRepo) ClearTokenPicture(ctx context.Context, tokenID, expected string) (bool, error) {
	return r.clear(ctx, `UPDATE map_tokens SET image_path = NULL WHERE id = ? AND image_path = ?`, tokenID, expected)
}
