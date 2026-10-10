package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// StorageStats holds aggregate storage statistics for the admin dashboard.
type StorageStats struct {
	TotalFiles  int                       // Total number of media files.
	TotalBytes  int64                     // Total storage used in bytes.
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
	// FindByContentHash returns the media file in `campaignID` whose
	// content_hash matches, or (nil, nil) if no row exists. Used for
	// per-campaign upload deduplication: if a hash already exists in
	// the campaign, the upload service short-circuits and returns the
	// existing file instead of writing a duplicate to disk.
	FindByContentHash(ctx context.Context, campaignID, hash string) (*MediaFile, error)
	// ListMissingContentHash returns up to `limit` media files in the
	// campaign whose content_hash is NULL. Used by the startup backfill
	// goroutine to populate hashes for rows that pre-date migration 26.
	// Pass campaignID="" to scan across all campaigns.
	ListMissingContentHash(ctx context.Context, limit int) ([]MediaFile, error)
	// SetContentHash writes a content_hash for an existing media file.
	// Used by the backfill goroutine; the regular Create path sets the
	// hash inline so this is rarely called outside of backfill.
	SetContentHash(ctx context.Context, id, hash string) error
	Delete(ctx context.Context, id string) error
	ListByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]MediaFile, int, error)
	GetStorageStats(ctx context.Context) (*StorageStats, error)
	ListAll(ctx context.Context, limit, offset int) ([]AdminMediaFile, int, error)

	// GetCampaignUsage returns the total bytes and file count for a campaign.
	// Used for storage quota enforcement at upload time.
	GetCampaignUsage(ctx context.Context, campaignID string) (totalBytes int64, fileCount int, err error)

	// GetUserNoteImageUsage returns the bytes and file count of the note
	// pictures a user uploaded into a campaign, bound to a note or not. Bound
	// ones are never swept and the owner cannot list them, so this is the only
	// place their total is bounded.
	GetUserNoteImageUsage(ctx context.Context, campaignID, userID string) (totalBytes int64, fileCount int, err error)

	// GetUserCampaignlessUsage returns the total bytes and file count for a
	// user's uploads that carry no campaign_id (avatars, and any
	// /media/upload posted with a blank campaign_id). Used for storage
	// quota enforcement on that bucket, which has no campaign to check a
	// quota against instead.
	GetUserCampaignlessUsage(ctx context.Context, userID string) (totalBytes int64, fileCount int, err error)

	// FindReferences returns entities that reference the given media file,
	// either via image_path or in their editor HTML content.
	FindReferences(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error)

	// ListAllFilenames returns all filenames (including thumbnail paths) tracked
	// in the database. Used by the orphan cleanup job to find disk files without
	// a corresponding DB record.
	ListAllFilenames(ctx context.Context) (map[string]bool, error)

	// ListFilesByCampaign returns all media files for a campaign without
	// pagination. Used for bulk cleanup during campaign deletion.
	ListFilesByCampaign(ctx context.Context, campaignID string) ([]MediaFile, error)

	// ListUnboundNotePictures returns the ids of note pictures created before
	// olderThan that no note is bound to and no saved note version still names:
	// uploaded and never saved into a note, or left behind when their note was
	// deleted or edited. A version still naming one is kept so a restore after
	// an accidental delete finds its picture.
	ListUnboundNotePictures(ctx context.Context, olderThan time.Time) ([]string, error)
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
	          mime_type, file_size, content_hash, usage_type, thumbnail_paths, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// content_hash may be empty for legacy callers — store NULL in that
	// case so the (campaign_id, content_hash) dedup index doesn't return
	// stale matches against empty strings.
	var contentHash any
	if file.ContentHash != "" {
		contentHash = file.ContentHash
	}

	_, err = r.db.ExecContext(ctx, query,
		file.ID, file.CampaignID, file.UploadedBy,
		file.Filename, file.OriginalName, file.MimeType,
		file.FileSize, contentHash, file.UsageType, string(thumbJSON),
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
	                 m.mime_type, m.file_size, m.content_hash, m.usage_type, m.thumbnail_paths, m.created_at,
	                 c.is_public
	          FROM media_files m
	          LEFT JOIN campaigns c ON m.campaign_id = c.id
	          WHERE m.id = ?`

	file := &MediaFile{}
	var thumbJSON string
	var contentHash sql.NullString
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&file.ID, &file.CampaignID, &file.UploadedBy,
		&file.Filename, &file.OriginalName, &file.MimeType,
		&file.FileSize, &contentHash, &file.UsageType, &thumbJSON,
		&file.CreatedAt, &file.CampaignIsPublic,
	)
	if contentHash.Valid {
		file.ContentHash = contentHash.String
	}
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

// FindByContentHash returns the media file in the campaign whose
// content_hash matches the given hex sha256, or (nil, nil) if none.
// Used by the upload service to short-circuit duplicate uploads.
//
// Scoped to a single campaign on purpose — cross-campaign dedup is
// off, so each campaign's media space stays isolated for clean
// export / cascade-delete behavior. Note pictures never match: a picture one
// player put in a private note must not be handed to another uploader of the
// same bytes, nor adopted by a page whose readers the note rule does not know.
func (r *mediaRepository) FindByContentHash(ctx context.Context, campaignID, hash string) (*MediaFile, error) {
	if campaignID == "" || hash == "" {
		return nil, nil
	}
	query := `SELECT m.id, m.campaign_id, m.uploaded_by, m.filename, m.original_name,
	                 m.mime_type, m.file_size, m.content_hash, m.usage_type, m.thumbnail_paths, m.created_at,
	                 c.is_public
	          FROM media_files m
	          LEFT JOIN campaigns c ON m.campaign_id = c.id
	          WHERE m.campaign_id = ? AND m.content_hash = ? AND m.usage_type <> ?
	          LIMIT 1`

	file := &MediaFile{}
	var thumbJSON string
	var contentHash sql.NullString
	err := r.db.QueryRowContext(ctx, query, campaignID, hash, UsageNoteImage).Scan(
		&file.ID, &file.CampaignID, &file.UploadedBy,
		&file.Filename, &file.OriginalName, &file.MimeType,
		&file.FileSize, &contentHash, &file.UsageType, &thumbJSON,
		&file.CreatedAt, &file.CampaignIsPublic,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying media file by content hash: %w", err)
	}
	if contentHash.Valid {
		file.ContentHash = contentHash.String
	}
	file.ThumbnailPaths = make(map[string]string)
	if thumbJSON != "" && thumbJSON != "{}" {
		if err := json.Unmarshal([]byte(thumbJSON), &file.ThumbnailPaths); err != nil {
			// Non-fatal — the file is still usable, just without thumbnail metadata.
			file.ThumbnailPaths = make(map[string]string)
		}
	}
	return file, nil
}

// ListMissingContentHash returns up to `limit` rows that have no
// content_hash yet. The startup backfill goroutine iterates these in
// batches and computes hashes from disk. Order is unspecified — we
// just need to cover them all eventually.
func (r *mediaRepository) ListMissingContentHash(ctx context.Context, limit int) ([]MediaFile, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `SELECT id, campaign_id, uploaded_by, filename, original_name,
	                 mime_type, file_size, usage_type, thumbnail_paths, created_at
	          FROM media_files
	          WHERE content_hash IS NULL
	          LIMIT ?`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("listing media missing content_hash: %w", err)
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
			return nil, fmt.Errorf("scanning media file row: %w", err)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// SetContentHash backfills the content_hash for an existing row.
// No-op if hash is empty (caller error).
func (r *mediaRepository) SetContentHash(ctx context.Context, id, hash string) error {
	if hash == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE media_files SET content_hash = ? WHERE id = ?`,
		hash, id,
	)
	if err != nil {
		return fmt.Errorf("updating media content_hash: %w", err)
	}
	return nil
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

// ListByCampaign returns media files for a campaign with pagination. Note
// pictures are left out: this feeds the media browser, the picker and the sync
// API, none of which may name a file that only its note's readers can open.
func (r *mediaRepository) ListByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]MediaFile, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM media_files WHERE campaign_id = ? AND usage_type <> ?`, campaignID, UsageNoteImage,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting media files: %w", err)
	}

	query := `SELECT id, campaign_id, uploaded_by, filename, original_name,
	                 mime_type, file_size, content_hash, usage_type, thumbnail_paths, created_at
	          FROM media_files WHERE campaign_id = ? AND usage_type <> ?
	          ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, campaignID, UsageNoteImage, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing media files: %w", err)
	}
	defer rows.Close()

	var files []MediaFile
	for rows.Next() {
		var f MediaFile
		var thumbJSON string
		var contentHash sql.NullString
		if err := rows.Scan(
			&f.ID, &f.CampaignID, &f.UploadedBy,
			&f.Filename, &f.OriginalName, &f.MimeType,
			&f.FileSize, &contentHash, &f.UsageType, &thumbJSON,
			&f.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning media file row: %w", err)
		}
		if contentHash.Valid {
			f.ContentHash = contentHash.String
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

	// A note picture keeps its row for disk accounting, but its file name and
	// thumbnails are the note's business: the page shows neither, so a site
	// admin sees a size and an owner, not a private picture's name or image.
	query := `SELECT m.id, m.campaign_id, m.uploaded_by, m.filename,
	                 CASE WHEN m.usage_type = 'note_image' THEN 'Note picture' ELSE m.original_name END,
	                 m.mime_type, m.file_size, m.usage_type,
	                 CASE WHEN m.usage_type = 'note_image' THEN '{}' ELSE m.thumbnail_paths END,
	                 m.created_at,
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

// GetUserNoteImageUsage counts one uploader's note pictures in one campaign.
// Every row counts, bound or not, so a member cannot dodge the cap by saving
// each picture into a note.
func (r *mediaRepository) GetUserNoteImageUsage(ctx context.Context, campaignID, userID string) (int64, int, error) {
	var totalBytes int64
	var fileCount int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(file_size), 0) FROM media_files
		 WHERE campaign_id = ? AND uploaded_by = ? AND usage_type = ?`,
		campaignID, userID, UsageNoteImage,
	).Scan(&fileCount, &totalBytes)
	if err != nil {
		return 0, 0, fmt.Errorf("querying note picture usage: %w", err)
	}
	return totalBytes, fileCount, nil
}

// GetUserCampaignlessUsage returns the total bytes stored and file count for
// a single user's campaign-less media (campaign_id IS NULL). Returns 0, 0 if
// the user has no such files.
func (r *mediaRepository) GetUserCampaignlessUsage(ctx context.Context, userID string) (int64, int, error) {
	var totalBytes int64
	var fileCount int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(file_size), 0) FROM media_files WHERE uploaded_by = ? AND campaign_id IS NULL`,
		userID,
	).Scan(&fileCount, &totalBytes)
	if err != nil {
		return 0, 0, fmt.Errorf("querying user campaign-less storage usage: %w", err)
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
// Checks entity image_path AND cover_image_path (direct references) and
// entry_html (embedded in editor).
//
// SECURITY: the UNION must cover cover_image_path, not just image_path —
// checkMediaAccess's entity-visibility rule treats an unreferenced file as
// falling through to plain campaign membership, so missing cover_image_path
// would leak a dm_only page's cover art to any campaign member (ADR-058). A
// cover match reports the same ref_type ('image') as a profile-image match
// so the "where is this used" UI doesn't mislabel it as "(in content)".
//
// image_path/cover_image_path can hold either the bare media id or a
// disk-path form ("2026/09/<id>.png" — written by an import or a restored
// backup; the API never writes it, but nothing normalizes it away either).
// Matching only bare-id equality makes a path-form reference invisible to
// this check, so it falls through to the same role-blind membership grant
// ADR-058 exists to close. The LIKE arm mirrors
// layouts.normalizeMediaID's basename+strip-extension logic: match a
// "/<id>.<anything>" suffix. mediaID is a server-generated UUID (no LIKE
// wildcard characters), so it's safe unescaped.
// Trashed pages deliberately still count as references: a picture on a trashed
// page must survive cleanup and stay access-protected so a restore brings it
// back intact.
func (r *mediaRepository) FindReferences(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
	query := `SELECT id, name, slug, 'image' AS ref_type
	          FROM entities
	          WHERE campaign_id = ? AND (image_path = ? OR image_path LIKE CONCAT('%/', ?, '.%'))
	          UNION
	          SELECT id, name, slug, 'image' AS ref_type
	          FROM entities
	          WHERE campaign_id = ? AND (cover_image_path = ? OR cover_image_path LIKE CONCAT('%/', ?, '.%'))
	          UNION
	          SELECT id, name, slug, 'content' AS ref_type
	          FROM entities
	          WHERE campaign_id = ? AND entry_html LIKE CONCAT('%/media/', ?, '%')
	          ORDER BY name`

	rows, err := r.db.QueryContext(ctx, query,
		campaignID, mediaID, mediaID,
		campaignID, mediaID, mediaID,
		campaignID, mediaID)
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

// ListFilesByCampaign returns all media files for a campaign without pagination.
// Used for bulk cleanup during campaign deletion — returns lightweight records
// with just the fields needed for disk + DB deletion.
func (r *mediaRepository) ListFilesByCampaign(ctx context.Context, campaignID string) ([]MediaFile, error) {
	query := `SELECT id, campaign_id, uploaded_by, filename, original_name,
	                 mime_type, file_size, usage_type, thumbnail_paths, created_at
	          FROM media_files WHERE campaign_id = ?`

	rows, err := r.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing campaign media files: %w", err)
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
			return nil, fmt.Errorf("scanning media file row: %w", err)
		}
		f.ThumbnailPaths = make(map[string]string)
		if thumbJSON != "" && thumbJSON != "{}" {
			if err := json.Unmarshal([]byte(thumbJSON), &f.ThumbnailPaths); err != nil {
				return nil, fmt.Errorf("unmarshaling thumbnail paths: %w", err)
			}
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// ListUnboundNotePictures finds note pictures nothing holds any more; see the
// interface. It reads note_pictures and note_versions, core tables the notes
// widget fills; the version check is a substring match on a fixed-shape id, so
// it needs no wildcard escaping.
func (r *mediaRepository) ListUnboundNotePictures(ctx context.Context, olderThan time.Time) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT m.id FROM media_files m
		 WHERE m.usage_type = ? AND m.created_at < ?
		   AND NOT EXISTS (SELECT 1 FROM note_pictures p WHERE p.media_id = m.id)
		   AND NOT EXISTS (SELECT 1 FROM note_versions v WHERE LOCATE(m.id, v.entry_html) > 0)
		 LIMIT 500`, UsageNoteImage, olderThan)
	if err != nil {
		return nil, fmt.Errorf("listing unbound note pictures: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning unbound note picture: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
