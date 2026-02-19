package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// MediaRepository defines the data access contract for media file operations.
type MediaRepository interface {
	Create(ctx context.Context, file *MediaFile) error
	FindByID(ctx context.Context, id string) (*MediaFile, error)
	Delete(ctx context.Context, id string) error
	ListByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]MediaFile, int, error)
}

// mediaRepository implements MediaRepository with MariaDB queries.
type mediaRepository struct {
	db *sql.DB
}

// NewMediaRepository creates a new media repository.
func NewMediaRepository(db *sql.DB) MediaRepository {
	return &mediaRepository{db: db}
}

// Create inserts a new media file record.
func (r *mediaRepository) Create(ctx context.Context, file *MediaFile) error {
	thumbJSON, err := json.Marshal(file.ThumbnailPaths)
	if err != nil {
		return fmt.Errorf("marshaling thumbnail paths: %w", err)
	}

	query := `INSERT INTO media_files (id, campaign_id, uploaded_by, filename, original_name,
	          mime_type, file_size, usage_type, thumbnail_paths, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = r.db.ExecContext(ctx, query,
		file.ID, file.CampaignID, file.UploadedBy,
		file.Filename, file.OriginalName, file.MimeType,
		file.FileSize, file.UsageType, string(thumbJSON),
		file.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting media file: %w", err)
	}
	return nil
}

// FindByID retrieves a media file by its UUID.
func (r *mediaRepository) FindByID(ctx context.Context, id string) (*MediaFile, error) {
	query := `SELECT id, campaign_id, uploaded_by, filename, original_name,
	                 mime_type, file_size, usage_type, thumbnail_paths, created_at
	          FROM media_files WHERE id = ?`

	file := &MediaFile{}
	var thumbJSON string
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&file.ID, &file.CampaignID, &file.UploadedBy,
		&file.Filename, &file.OriginalName, &file.MimeType,
		&file.FileSize, &file.UsageType, &thumbJSON,
		&file.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("media file not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying media file by id: %w", err)
	}

	file.ThumbnailPaths = make(map[string]string)
	if thumbJSON != "" && thumbJSON != "{}" {
		if err := json.Unmarshal([]byte(thumbJSON), &file.ThumbnailPaths); err != nil {
			return nil, fmt.Errorf("unmarshaling thumbnail paths: %w", err)
		}
	}
	return file, nil
}

// Delete removes a media file record.
func (r *mediaRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM media_files WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting media file: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return apperror.NewNotFound("media file not found")
	}
	return nil
}

// ListByCampaign returns media files for a campaign with pagination.
func (r *mediaRepository) ListByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]MediaFile, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM media_files WHERE campaign_id = ?`, campaignID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting media files: %w", err)
	}

	query := `SELECT id, campaign_id, uploaded_by, filename, original_name,
	                 mime_type, file_size, usage_type, thumbnail_paths, created_at
	          FROM media_files WHERE campaign_id = ?
	          ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, campaignID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing media files: %w", err)
	}
	defer rows.Close()

	var files []MediaFile
	for rows.Next() {
		var f MediaFile
		var thumbJSON string
		if err := rows.Scan(
			&f.ID, &f.CampaignID, &f.UploadedBy,
			&f.Filename, &f.OriginalName, &f.MimeType,
			&f.FileSize, &f.UsageType, &thumbJSON,
			&f.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning media file row: %w", err)
		}
		f.ThumbnailPaths = make(map[string]string)
		if thumbJSON != "" && thumbJSON != "{}" {
			if err := json.Unmarshal([]byte(thumbJSON), &f.ThumbnailPaths); err != nil {
				return nil, 0, fmt.Errorf("unmarshaling thumbnail paths: %w", err)
			}
		}
		files = append(files, f)
	}
	return files, total, rows.Err()
}
