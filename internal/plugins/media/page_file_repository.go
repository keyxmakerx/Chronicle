package media

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// PageFile is a file attached to a page: the binding row joined to the stored
// file's name, size and type.
type PageFile struct {
	ID         string    `json:"id"`
	EntityID   string    `json:"entityId"`
	CampaignID string    `json:"campaignId"`
	Name       string    `json:"name"`
	MimeType   string    `json:"mimeType"`
	Size       int64     `json:"size"`
	GMOnly     bool      `json:"gmOnly"`
	UploadedBy string    `json:"uploadedBy"`
	CreatedAt  time.Time `json:"createdAt"`
}

// PageFileRepository owns the page_files binding table: which page a stored
// file belongs to and whether it is for the GM only. The file itself lives in
// media_files and is stored and removed by the media service.
type PageFileRepository interface {
	// Bind records that the stored file belongs to the page.
	Bind(ctx context.Context, f PageFile) error

	// Find returns the binding of one file, or a not-found error when the file
	// is unbound or is not a page file at all.
	Find(ctx context.Context, mediaID string) (*PageFile, error)

	// ListByEntity returns a page's files, oldest first. GM-only files are
	// included only when includeGMOnly is set.
	ListByEntity(ctx context.Context, campaignID, entityID string, includeGMOnly bool) ([]PageFile, error)

	// CountByEntity returns how many files a page holds, GM-only ones included.
	CountByEntity(ctx context.Context, entityID string) (int, error)

	// SetGMOnly flips a file's GM-only mark.
	SetGMOnly(ctx context.Context, mediaID string, gmOnly bool) error
}

type pageFileRepository struct {
	db *sql.DB
}

// NewPageFileRepository creates the page file binding repository.
func NewPageFileRepository(db *sql.DB) PageFileRepository {
	return &pageFileRepository{db: db}
}

// pageFileSelect joins the binding to its stored file. The usage_type test is
// part of the join so a binding row can never present an ordinary media file
// as a page file.
const pageFileSelect = `SELECT p.media_id, p.entity_id, p.campaign_id, m.original_name, m.mime_type,
	       m.file_size, p.gm_only, p.created_by, p.created_at
	FROM page_files p
	JOIN media_files m ON m.id = p.media_id AND m.usage_type = '` + UsagePageFile + `'`

func scanPageFile(row interface{ Scan(...any) error }) (*PageFile, error) {
	var f PageFile
	if err := row.Scan(&f.ID, &f.EntityID, &f.CampaignID, &f.Name, &f.MimeType,
		&f.Size, &f.GMOnly, &f.UploadedBy, &f.CreatedAt); err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *pageFileRepository) Bind(ctx context.Context, f PageFile) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO page_files (media_id, entity_id, campaign_id, gm_only, created_by) VALUES (?, ?, ?, ?, ?)`,
		f.ID, f.EntityID, f.CampaignID, f.GMOnly, f.UploadedBy)
	if err != nil {
		return fmt.Errorf("binding page file: %w", err)
	}
	return nil
}

func (r *pageFileRepository) Find(ctx context.Context, mediaID string) (*PageFile, error) {
	f, err := scanPageFile(r.db.QueryRowContext(ctx, pageFileSelect+` WHERE p.media_id = ?`, mediaID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("file not found")
	}
	if err != nil {
		return nil, fmt.Errorf("finding page file: %w", err)
	}
	return f, nil
}

func (r *pageFileRepository) ListByEntity(ctx context.Context, campaignID, entityID string, includeGMOnly bool) ([]PageFile, error) {
	rows, err := r.db.QueryContext(ctx,
		pageFileSelect+` WHERE p.campaign_id = ? AND p.entity_id = ? AND (p.gm_only = 0 OR ?)
		 ORDER BY p.created_at, p.media_id`, campaignID, entityID, includeGMOnly)
	if err != nil {
		return nil, fmt.Errorf("listing page files: %w", err)
	}
	defer rows.Close()
	var out []PageFile
	for rows.Next() {
		f, err := scanPageFile(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning page file: %w", err)
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

func (r *pageFileRepository) CountByEntity(ctx context.Context, entityID string) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM page_files WHERE entity_id = ?`, entityID).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting page files: %w", err)
	}
	return n, nil
}

func (r *pageFileRepository) SetGMOnly(ctx context.Context, mediaID string, gmOnly bool) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE page_files SET gm_only = ? WHERE media_id = ?`, gmOnly, mediaID); err != nil {
		return fmt.Errorf("setting page file gm_only: %w", err)
	}
	return nil
}
