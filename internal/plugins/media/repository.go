package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// StorageStats holds aggregate storage statistics for the admin dashboard.
type StorageStats struct {
	TotalFiles  int              // Total number of media files.
	TotalBytes  int64            // Total storage used in bytes.
	ByUsageType map[string]UsageTypeStats // Breakdown by usage type.
}

// UsageTypeStats holds per-usage-type counts and sizes.
type UsageTypeStats struct {
	Count int   `json:"count"`
	Bytes int64 `json:"bytes"`
}

// AdminMediaFile extends MediaFile with uploader display name for admin views.
type AdminMediaFile struct {
	MediaFile
	UploaderName string
}

// MediaRepository defines the data access contract for media file operations.
type MediaRepository interface {
	Create(ctx context.Context, file *MediaFile) error
	FindByID(ctx context.Context, id string) (*MediaFile, error)
	Delete(ctx context.Context, id string) error
	ListByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]MediaFile, int, error)
	GetStorageStats(ctx context.Context) (*StorageStats, error)
	ListAll(ctx context.Context, limit, offset int) ([]AdminMediaFile, int, error)

	// GetCampaignUsage returns the total bytes and file count for a campaign.
	// Used for storage quota enforcement at upload time.
	GetCampaignUsage(ctx context.Context, campaignID string) (totalBytes int64, fileCount int, err error)

	// FindReferences returns entities that reference the given media file,
	// either via image_path or in their editor HTML content.
	FindReferences(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error)

	// ListAllFilenames returns all filenames (including thumbnail paths) tracked
	// in the database. Used by the orphan cleanup job to find disk files without
	// a corresponding DB record.
	ListAllFilenames(ctx context.Context) (map[string]bool, error)
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

// FindByID retrieves a media file by its UUID. LEFT JOINs the campaigns
// table to populate CampaignIsPublic in a single query, avoiding N+1
// lookups when the serve handler checks campaign privacy.
func (r *mediaRepository) FindByID(ctx context.Context, id string) (*MediaFile, error) {
	query := `SELECT m.id, m.campaign_id, m.uploaded_by, m.filename, m.original_name,
	                 m.mime_type, m.file_size, m.usage_type, m.thumbnail_paths, m.created_at,
	                 c.is_public
	          FROM media_files m
	          LEFT JOIN campaigns c ON m.campaign_id = c.id
	          WHERE m.id = ?`

	file := &MediaFile{}
	var thumbJSON string
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&file.ID, &file.CampaignID, &file.UploadedBy,
		&file.Filename, &file.OriginalName, &file.MimeType,
		&file.FileSize, &file.UsageType, &thumbJSON,
		&file.CreatedAt, &file.CampaignIsPublic,
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

// GetStorageStats returns aggregate storage statistics across all media files.
func (r *mediaRepository) GetStorageStats(ctx context.Context) (*StorageStats, error) {
	stats := &StorageStats{
		ByUsageType: make(map[string]UsageTypeStats),
	}

	// Overall totals.
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(file_size), 0) FROM media_files`,
	).Scan(&stats.TotalFiles, &stats.TotalBytes)
	if err != nil {
		return nil, fmt.Errorf("querying storage totals: %w", err)
	}

	// Breakdown by usage type.
	rows, err := r.db.QueryContext(ctx,
		`SELECT usage_type, COUNT(*), COALESCE(SUM(file_size), 0)
		 FROM media_files GROUP BY usage_type`)
	if err != nil {
		return nil, fmt.Errorf("querying usage type stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var usageType string
		var ut UsageTypeStats
		if err := rows.Scan(&usageType, &ut.Count, &ut.Bytes); err != nil {
			return nil, fmt.Errorf("scanning usage type row: %w", err)
		}
		stats.ByUsageType[usageType] = ut
	}
	return stats, rows.Err()
}

// ListAll returns all media files with uploader names, ordered by most recent.
func (r *mediaRepository) ListAll(ctx context.Context, limit, offset int) ([]AdminMediaFile, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM media_files`,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting all media files: %w", err)
	}

	query := `SELECT m.id, m.campaign_id, m.uploaded_by, m.filename, m.original_name,
	                 m.mime_type, m.file_size, m.usage_type, m.thumbnail_paths, m.created_at,
	                 COALESCE(u.display_name, 'Unknown')
	          FROM media_files m
	          LEFT JOIN users u ON m.uploaded_by = u.id
	          ORDER BY m.created_at DESC LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing all media files: %w", err)
	}
	defer rows.Close()

	var files []AdminMediaFile
	for rows.Next() {
		var f AdminMediaFile
		var thumbJSON string
		if err := rows.Scan(
			&f.ID, &f.CampaignID, &f.UploadedBy,
			&f.Filename, &f.OriginalName, &f.MimeType,
			&f.FileSize, &f.UsageType, &thumbJSON,
			&f.CreatedAt, &f.UploaderName,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning admin media file row: %w", err)
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

// GetCampaignUsage returns the total bytes stored and file count for a single
// campaign. Returns 0, 0 if the campaign has no media files.
func (r *mediaRepository) GetCampaignUsage(ctx context.Context, campaignID string) (int64, int, error) {
	var totalBytes int64
	var fileCount int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(file_size), 0) FROM media_files WHERE campaign_id = ?`,
		campaignID,
	).Scan(&fileCount, &totalBytes)
	if err != nil {
		return 0, 0, fmt.Errorf("querying campaign storage usage: %w", err)
	}
	return totalBytes, fileCount, nil
}

// ListAllFilenames returns a set of all filenames tracked in the database,
// including thumbnail paths. The returned map uses relative paths (e.g.,
// "2006/01/uuid.jpg") as keys.
func (r *mediaRepository) ListAllFilenames(ctx context.Context) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT filename, thumbnail_paths FROM media_files`)
	if err != nil {
		return nil, fmt.Errorf("listing all filenames: %w", err)
	}
	defer rows.Close()

	known := make(map[string]bool)
	for rows.Next() {
		var filename, thumbJSON string
		if err := rows.Scan(&filename, &thumbJSON); err != nil {
			return nil, fmt.Errorf("scanning filename row: %w", err)
		}
		known[filename] = true
		if thumbJSON != "" && thumbJSON != "{}" {
			var thumbs map[string]string
			if err := json.Unmarshal([]byte(thumbJSON), &thumbs); err == nil {
				for _, tp := range thumbs {
					known[tp] = true
				}
			}
		}
	}
	return known, rows.Err()
}

// FindReferences returns entities that reference the given media file.
// Checks both entity image_path (direct reference) and entry_html (embedded in editor).
func (r *mediaRepository) FindReferences(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
	query := `SELECT id, name, slug, 'image' AS ref_type
	          FROM entities
	          WHERE campaign_id = ? AND image_path = ?
	          UNION
	          SELECT id, name, slug, 'content' AS ref_type
	          FROM entities
	          WHERE campaign_id = ? AND entry_html LIKE CONCAT('%/media/', ?, '%')
	          ORDER BY name`

	rows, err := r.db.QueryContext(ctx, query, campaignID, mediaID, campaignID, mediaID)
	if err != nil {
		return nil, fmt.Errorf("finding media references: %w", err)
	}
	defer rows.Close()

	var refs []MediaRef
	for rows.Next() {
		var ref MediaRef
		if err := rows.Scan(&ref.EntityID, &ref.EntityName, &ref.EntitySlug, &ref.RefType); err != nil {
			return nil, fmt.Errorf("scanning media reference: %w", err)
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}
