package admin

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// OrphanedMediaItem represents a media file with no campaign association
// that is not an avatar or backdrop.
type OrphanedMediaItem struct {
	ID        string
	Filename  string
	FileSize  int64
	CreatedAt time.Time
	// Referenced indicates whether any entity still references this file
	// via image_path or entry_html. Referenced files cannot be purged.
	Referenced bool
}

// OrphanedAPIKey represents an API key whose campaign no longer exists.
type OrphanedAPIKey struct {
	ID         int
	KeyPrefix  string
	Name       string
	CampaignID string
	CreatedAt  time.Time
}

// StaleFile represents a file on disk that has no corresponding DB record.
type StaleFile struct {
	Path    string
	Size    int64
	ModTime time.Time
}

// DiskUsageStats holds aggregate data hygiene statistics.
type DiskUsageStats struct {
	TotalDiskBytes   int64
	TrackedDBBytes   int64
	OrphanMediaCount int
	OrphanMediaBytes int64
	OrphanAPIKeys    int
	StaleFileCount   int
	StaleFileBytes   int64
}

// DataHygieneData bundles all data for the hygiene dashboard page.
type DataHygieneData struct {
	Stats           *DiskUsageStats
	OrphanedMedia   []OrphanedMediaItem
	OrphanedAPIKeys []OrphanedAPIKey
	StaleFiles      []StaleFile
	CSRFToken       string
}

// DataHygieneScanner detects and cleans up orphaned data across the system.
type DataHygieneScanner interface {
	ScanOrphanedMedia(ctx context.Context) ([]OrphanedMediaItem, error)
	ScanOrphanedAPIKeys(ctx context.Context) ([]OrphanedAPIKey, error)
	ScanStaleFiles(ctx context.Context) ([]StaleFile, error)
	GetDiskUsageStats(ctx context.Context) (*DiskUsageStats, error)
	PurgeOrphanedMedia(ctx context.Context) (int, error)
	PurgeOrphanedAPIKeys(ctx context.Context) (int, error)
	PurgeStaleFiles(ctx context.Context) (int, error)
}

// hygieneService implements DataHygieneScanner with direct DB queries
// and media service integration.
type hygieneService struct {
	db           *sql.DB
	mediaRepo    media.MediaRepository
	mediaService media.MediaService
	mediaPath    string
	secRepo      SecurityEventRepository
}

// NewHygieneService creates a data hygiene service.
func NewHygieneService(db *sql.DB, mediaRepo media.MediaRepository, mediaService media.MediaService, mediaPath string, secRepo SecurityEventRepository) DataHygieneScanner {
	return &hygieneService{
		db:           db,
		mediaRepo:    mediaRepo,
		mediaService: mediaService,
		mediaPath:    mediaPath,
		secRepo:      secRepo,
	}
}

// ScanOrphanedMedia finds media files with NULL campaign_id that are not
// avatars or backdrops. Cross-checks each against entity references to
// determine if it's safe to delete.
func (s *hygieneService) ScanOrphanedMedia(ctx context.Context) ([]OrphanedMediaItem, error) {
	query := `SELECT id, filename, file_size, created_at
	          FROM media_files
	          WHERE campaign_id IS NULL
	            AND usage_type NOT IN ('avatar', 'backdrop')
	          ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("scanning orphaned media: %w", err)
	}
	defer rows.Close()

	var items []OrphanedMediaItem
	for rows.Next() {
		var item OrphanedMediaItem
		if err := rows.Scan(&item.ID, &item.Filename, &item.FileSize, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning orphaned media row: %w", err)
		}

		// Check if any entity references this file (cannot safely delete).
		refCount, err := s.countMediaReferences(ctx, item.Filename)
		if err != nil {
			slog.Warn("failed to check media references",
				slog.String("file_id", item.ID),
				slog.Any("error", err),
			)
		}
		item.Referenced = refCount > 0

		items = append(items, item)
	}
	return items, rows.Err()
}

// countMediaReferences checks if any entity references a media filename
// via image_path or embedded in entry_html content.
func (s *hygieneService) countMediaReferences(ctx context.Context, filename string) (int, error) {
	var count int
	escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(filename)
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM entities
		 WHERE image_path = ? OR entry_html LIKE CONCAT('%', ?, '%')`,
		filename, escaped,
	).Scan(&count)
	return count, err
}

// ScanOrphanedAPIKeys finds API keys whose campaign_id references a campaign
// that no longer exists. After migration 000058 adds the FK constraint, new
// orphans cannot be created, but pre-existing orphans may remain.
func (s *hygieneService) ScanOrphanedAPIKeys(ctx context.Context) ([]OrphanedAPIKey, error) {
	query := `SELECT ak.id, ak.key_prefix, ak.name, ak.campaign_id, ak.created_at
	          FROM api_keys ak
	          LEFT JOIN campaigns c ON ak.campaign_id = c.id
	          WHERE c.id IS NULL
	          ORDER BY ak.created_at DESC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("scanning orphaned API keys: %w", err)
	}
	defer rows.Close()

	var items []OrphanedAPIKey
	for rows.Next() {
		var item OrphanedAPIKey
		if err := rows.Scan(&item.ID, &item.KeyPrefix, &item.Name, &item.CampaignID, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning orphaned API key row: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ScanStaleFiles walks the media directory and finds files on disk that have
// no corresponding record in the database.
func (s *hygieneService) ScanStaleFiles(ctx context.Context) ([]StaleFile, error) {
	knownFiles, err := s.mediaRepo.ListAllFilenames(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing known files: %w", err)
	}

	var stale []StaleFile
	err = filepath.Walk(s.mediaPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(s.mediaPath, path)
		if relErr != nil {
			return nil
		}

		// Skip the packages/ subdirectory — package files are managed by
		// the package manager and tracked in the packages table, not media_files.
		if strings.HasPrefix(rel, "packages"+string(filepath.Separator)) || strings.HasPrefix(rel, "packages/") {
			return nil
		}

		if !knownFiles[rel] {
			// Skip recent files (may be in-flight uploads).
			if time.Since(info.ModTime()) < 15*time.Minute {
				return nil
			}
			stale = append(stale, StaleFile{
				Path:    rel,
				Size:    info.Size(),
				ModTime: info.ModTime(),
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking media directory: %w", err)
	}

	return stale, nil
}

// GetDiskUsageStats computes aggregate statistics for the data hygiene dashboard.
func (s *hygieneService) GetDiskUsageStats(ctx context.Context) (*DiskUsageStats, error) {
	stats := &DiskUsageStats{}

	// DB-tracked bytes.
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(file_size), 0) FROM media_files`,
	).Scan(&stats.TrackedDBBytes)
	if err != nil {
		return nil, fmt.Errorf("querying tracked bytes: %w", err)
	}

	// Walk disk for total usage.
	err = filepath.Walk(s.mediaPath, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		stats.TotalDiskBytes += info.Size()
		return nil
	})
	if err != nil {
		slog.Warn("disk usage walk failed", slog.Any("error", err))
	}

	// Orphaned media count and size.
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(file_size), 0) FROM media_files
		 WHERE campaign_id IS NULL AND usage_type NOT IN ('avatar', 'backdrop')`,
	).Scan(&stats.OrphanMediaCount, &stats.OrphanMediaBytes)
	if err != nil {
		return nil, fmt.Errorf("querying orphan media stats: %w", err)
	}

	// Orphaned API keys.
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM api_keys ak
		 LEFT JOIN campaigns c ON ak.campaign_id = c.id
		 WHERE c.id IS NULL`,
	).Scan(&stats.OrphanAPIKeys)
	if err != nil {
		return nil, fmt.Errorf("querying orphan API key count: %w", err)
	}

	// Stale files (computed from scan).
	staleFiles, err := s.ScanStaleFiles(ctx)
	if err != nil {
		slog.Warn("stale file scan failed during stats", slog.Any("error", err))
	} else {
		stats.StaleFileCount = len(staleFiles)
		for _, f := range staleFiles {
			stats.StaleFileBytes += f.Size
		}
	}

	return stats, nil
}

// PurgeOrphanedMedia deletes orphaned media files that are not referenced by
// any entity. Logs each deletion to security_events.
func (s *hygieneService) PurgeOrphanedMedia(ctx context.Context) (int, error) {
	orphans, err := s.ScanOrphanedMedia(ctx)
	if err != nil {
		return 0, err
	}

	purged := 0
	for _, item := range orphans {
		if item.Referenced {
			continue // Safety: don't delete files still referenced by entities.
		}

		if err := s.mediaService.Delete(ctx, item.ID); err != nil {
			slog.Warn("failed to purge orphaned media",
				slog.String("file_id", item.ID),
				slog.Any("error", err),
			)
			continue
		}
		purged++

		s.logSecurityEvent(ctx, "orphan_media_purged",
			fmt.Sprintf("Purged orphaned media file: %s (%s)", item.Filename, item.ID))
	}

	slog.Info("orphaned media purge completed", slog.Int("purged", purged))
	return purged, nil
}

// PurgeOrphanedAPIKeys deletes API keys for campaigns that no longer exist.
func (s *hygieneService) PurgeOrphanedAPIKeys(ctx context.Context) (int, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE ak FROM api_keys ak
		 LEFT JOIN campaigns c ON ak.campaign_id = c.id
		 WHERE c.id IS NULL`,
	)
	if err != nil {
		return 0, fmt.Errorf("purging orphaned API keys: %w", err)
	}

	rows, _ := result.RowsAffected()
	purged := int(rows)

	if purged > 0 {
		s.logSecurityEvent(ctx, "orphan_api_keys_purged",
			fmt.Sprintf("Purged %d orphaned API keys", purged))
	}

	slog.Info("orphaned API key purge completed", slog.Int("purged", purged))
	return purged, nil
}

// PurgeStaleFiles removes files from disk that have no DB record.
func (s *hygieneService) PurgeStaleFiles(ctx context.Context) (int, error) {
	staleFiles, err := s.ScanStaleFiles(ctx)
	if err != nil {
		return 0, err
	}

	purged := 0
	for _, f := range staleFiles {
		fullPath := filepath.Join(s.mediaPath, f.Path)
		if err := os.Remove(fullPath); err != nil {
			slog.Warn("failed to remove stale file",
				slog.String("path", f.Path),
				slog.Any("error", err),
			)
			continue
		}
		purged++
	}

	if purged > 0 {
		s.logSecurityEvent(ctx, "stale_files_purged",
			fmt.Sprintf("Purged %d stale files from disk", purged))
	}

	slog.Info("stale file purge completed", slog.Int("purged", purged))
	return purged, nil
}

// logSecurityEvent writes an audit entry for data hygiene actions.
func (s *hygieneService) logSecurityEvent(ctx context.Context, eventType, detail string) {
	if s.secRepo == nil {
		return
	}
	if err := s.secRepo.Log(ctx, &SecurityEvent{
		EventType: eventType,
		IPAddress: "system",
		Details:   map[string]any{"message": detail},
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		slog.Warn("failed to log security event",
			slog.String("event_type", eventType),
			slog.Any("error", err),
		)
	}
}
