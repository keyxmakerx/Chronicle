package media

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	// Register decoders for image formats.
	_ "golang.org/x/image/webp"

	"golang.org/x/image/draw"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// StorageLimiter resolves effective storage limits for quota enforcement at
// upload time. Implemented by the settings plugin via an adapter in routes.go.
// When nil, only the static maxSize check applies.
type StorageLimiter interface {
	GetEffectiveLimits(ctx context.Context, userID, campaignID string) (maxUploadSize, maxTotalStorage int64, maxFiles int, err error)
}

// MediaService handles business logic for media file operations.
type MediaService interface {
	Upload(ctx context.Context, input UploadInput) (*MediaFile, error)
	GetByID(ctx context.Context, id string) (*MediaFile, error)
	Delete(ctx context.Context, id string) error
	FilePath(file *MediaFile) string
	ThumbnailPath(file *MediaFile, size string) string
	SetStorageLimiter(limiter StorageLimiter)
	// ListCampaignMedia returns paginated media files for a campaign.
	ListCampaignMedia(ctx context.Context, campaignID string, page, perPage int) ([]MediaFile, int, error)

	// GetCampaignStats returns aggregate storage stats for a campaign.
	GetCampaignStats(ctx context.Context, campaignID string) (*CampaignMediaStats, error)

	// FindReferences returns entities that reference a media file.
	FindReferences(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error)

	// DeleteCampaignMedia deletes a media file after verifying it belongs to the campaign.
	DeleteCampaignMedia(ctx context.Context, campaignID, mediaID string) error

	// DeleteCampaignFiles removes all media files belonging to a campaign from
	// both disk and database. Used during campaign deletion to prevent orphaned
	// files. Returns the number of files deleted.
	DeleteCampaignFiles(ctx context.Context, campaignID string) (int, error)

	// CleanupOrphans finds files on disk without a corresponding DB record and
	// deletes them. Returns the number of files removed.
	CleanupOrphans(ctx context.Context) (int, error)

	// ValidateMediaPath ensures the storage directory exists and is writable.
	ValidateMediaPath() error
}

// maxConcurrentUploadsPerUser limits simultaneous uploads per user to prevent
// resource exhaustion from parallel large file processing.
const maxConcurrentUploadsPerUser = 3

// minFreeDiskBytes is the minimum free disk space required after writing a file.
// Uploads are rejected if writing the file would leave less than this available.
const minFreeDiskBytes = 100 * 1024 * 1024 // 100 MB

// uploadSemaphore tracks concurrent uploads per user.
type uploadSemaphore struct {
	mu    sync.Mutex
	slots map[string]int
}

// acquire increments the user's active upload count and returns true, or
// returns false if the user has reached the concurrency limit.
func (s *uploadSemaphore) acquire(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.slots[userID] >= maxConcurrentUploadsPerUser {
		return false
	}
	s.slots[userID]++
	return true
}

// release decrements the user's active upload count.
func (s *uploadSemaphore) release(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.slots[userID] > 0 {
		s.slots[userID]--
	}
	if s.slots[userID] == 0 {
		delete(s.slots, userID)
	}
}

// mediaService implements MediaService.
type mediaService struct {
	repo      MediaRepository
	mediaPath string         // Root directory for file storage.
	maxSize   int64          // Maximum file size in bytes (static fallback).
	limiter StorageLimiter // Dynamic storage limits from settings plugin. May be nil.
	sem     *uploadSemaphore
}

// NewMediaService creates a new media service.
func NewMediaService(repo MediaRepository, mediaPath string, maxSize int64) MediaService {
	return &mediaService{
		repo:      repo,
		mediaPath: mediaPath,
		maxSize:   maxSize,
		sem:       &uploadSemaphore{slots: make(map[string]int)},
	}
}

// SetStorageLimiter sets the dynamic storage limiter for quota enforcement.
// Called after all plugins are wired to avoid initialization order issues.
func (s *mediaService) SetStorageLimiter(limiter StorageLimiter) {
	s.limiter = limiter
}

// ValidateMediaPath ensures the media storage directory exists and is writable.
// Call at startup to surface configuration issues early instead of failing on
// the first upload with a 500 error.
func (s *mediaService) ValidateMediaPath() error {
	if err := os.MkdirAll(s.mediaPath, 0750); err != nil {
		return fmt.Errorf("media path %q: cannot create directory: %w", s.mediaPath, err)
	}
	// Write and remove a test file to verify write permissions.
	testFile := filepath.Join(s.mediaPath, ".write-test")
	if err := os.WriteFile(testFile, []byte("ok"), 0640); err != nil {
		return fmt.Errorf("media path %q: not writable: %w", s.mediaPath, err)
	}
	_ = os.Remove(testFile)
	slog.Info("media storage path validated", slog.String("path", s.mediaPath))
	return nil
}

// Upload validates, stores, and records a new media file.
func (s *mediaService) Upload(ctx context.Context, input UploadInput) (*MediaFile, error) {
	// Limit concurrent uploads per user to prevent resource exhaustion.
	if !s.sem.acquire(input.UploadedBy) {
		return nil, apperror.NewBadRequest("too many concurrent uploads; please wait and try again")
	}
	defer s.sem.release(input.UploadedBy)

	// Validate MIME type.
	if !AllowedMimeTypes[input.MimeType] {
		return nil, apperror.NewBadRequest("unsupported file type: " + input.MimeType)
	}

	// Validate file size against static limit (fallback).
	maxUpload := s.maxSize
	if input.FileSize > maxUpload {
		return nil, apperror.NewBadRequest(fmt.Sprintf("file too large; maximum size is %d MB", maxUpload/(1024*1024)))
	}

	// Enforce dynamic storage limits from site settings if available.
	if s.limiter != nil {
		if err := s.checkQuotas(ctx, input); err != nil {
			return nil, err
		}
	}

	// Validate magic bytes match declared MIME type.
	if !validateMagicBytes(input.FileBytes, input.MimeType) {
		return nil, apperror.NewBadRequest("file content does not match declared type")
	}

	// Re-encode the image to strip ALL metadata (EXIF, IPTC, XMP) and
	// destroy any polyglot payloads. The decode-then-encode pipeline
	// produces a clean file containing only pixel data (CDR approach).
	sanitizedBytes, effectiveMime, err := sanitizeImage(input.FileBytes, input.MimeType)
	if err != nil {
		return nil, apperror.NewBadRequest("image sanitization failed: " + err.Error())
	}
	input.FileBytes = sanitizedBytes
	input.FileSize = int64(len(sanitizedBytes))
	input.MimeType = effectiveMime

	// Generate UUID filename in date-based directory.
	id := generateUUID()
	now := time.Now().UTC()
	dir := filepath.Join(s.mediaPath, now.Format("2006/01"))
	ext := MimeToExtension[input.MimeType]
	filename := id + ext

	// Create directory with restrictive permissions.
	if err := os.MkdirAll(dir, 0750); err != nil {
		slog.Error("failed to create media directory",
			slog.String("dir", dir),
			slog.String("campaign_id", input.CampaignID),
			slog.Any("error", err),
		)
		return nil, apperror.NewInternal(fmt.Errorf("creating media directory: %w", err))
	}

	// Verify sufficient disk space before writing to prevent filling the filesystem.
	if err := checkDiskSpace(dir, input.FileSize); err != nil {
		return nil, err
	}

	// Write sanitized file to disk with restrictive permissions.
	fullPath := filepath.Join(dir, filename)
	if err := os.WriteFile(fullPath, input.FileBytes, 0640); err != nil {
		slog.Error("failed to write media file to disk",
			slog.String("path", fullPath),
			slog.String("campaign_id", input.CampaignID),
			slog.String("mime_type", input.MimeType),
			slog.Int64("size", input.FileSize),
			slog.Any("error", err),
		)
		return nil, apperror.NewInternal(fmt.Errorf("writing media file: %w", err))
	}

	// Build file record.
	var campaignPtr *string
	if input.CampaignID != "" {
		campaignPtr = &input.CampaignID
	}

	file := &MediaFile{
		ID:             id,
		CampaignID:     campaignPtr,
		UploadedBy:     input.UploadedBy,
		Filename:       filepath.Join(now.Format("2006/01"), filename),
		OriginalName:   input.OriginalName,
		MimeType:       input.MimeType,
		FileSize:       input.FileSize,
		UsageType:      input.UsageType,
		ThumbnailPaths: make(map[string]string),
		CreatedAt:      now,
	}

	// Generate thumbnails for images (using sanitized bytes).
	if file.IsImage() && input.MimeType != "image/gif" {
		thumbSizes := map[string]int{"300": 300, "800": 800}
		for sizeLabel, maxDim := range thumbSizes {
			thumbFilename, err := s.generateThumbnail(input.FileBytes, dir, id, ext, maxDim)
			if err != nil {
				slog.Warn("thumbnail generation failed",
					slog.String("file_id", id),
					slog.String("size", sizeLabel),
					slog.Any("error", err),
				)
				continue
			}
			file.ThumbnailPaths[sizeLabel] = filepath.Join(now.Format("2006/01"), thumbFilename)
		}
	}

	// Save to database.
	if err := s.repo.Create(ctx, file); err != nil {
		// Clean up all disk files (main + thumbnails) on DB failure.
		// Errors are intentionally ignored — cleanup is best-effort.
		_ = os.Remove(fullPath)
		for _, thumbFile := range file.ThumbnailPaths {
			_ = os.Remove(filepath.Join(s.mediaPath, thumbFile))
		}
		slog.Error("failed to save media record to database",
			slog.String("file_id", id),
			slog.String("campaign_id", input.CampaignID),
			slog.Any("error", err),
		)
		return nil, apperror.NewInternal(fmt.Errorf("saving media record: %w", err))
	}

	slog.Info("media file uploaded",
		slog.String("id", id),
		slog.String("mime_type", input.MimeType),
		slog.Int64("size", input.FileSize),
	)
	return file, nil
}

// checkQuotas enforces dynamic storage limits from the settings plugin.
// Checks per-file size, campaign storage total, and campaign file count.
// Returns a user-facing error if any quota would be exceeded. A limit of 0
// means unlimited (no cap enforced).
func (s *mediaService) checkQuotas(ctx context.Context, input UploadInput) error {
	maxUpload, maxStorage, maxFiles, err := s.limiter.GetEffectiveLimits(ctx, input.UploadedBy, input.CampaignID)
	if err != nil {
		// Quota lookup failure should not block uploads -- log and allow.
		slog.Warn("failed to resolve storage limits, allowing upload",
			slog.String("user_id", input.UploadedBy),
			slog.String("campaign_id", input.CampaignID),
			slog.Any("error", err),
		)
		return nil
	}

	// Per-file size limit from settings (overrides static maxSize).
	if maxUpload > 0 && input.FileSize > maxUpload {
		return apperror.NewBadRequest(fmt.Sprintf("file too large; maximum size is %d MB", maxUpload/(1024*1024)))
	}

	// Campaign-scoped limits only apply if the upload is associated with a campaign.
	if input.CampaignID == "" {
		return nil
	}

	usedBytes, fileCount, err := s.repo.GetCampaignUsage(ctx, input.CampaignID)
	if err != nil {
		slog.Warn("failed to query campaign usage, allowing upload",
			slog.String("campaign_id", input.CampaignID),
			slog.Any("error", err),
		)
		return nil
	}

	if maxStorage > 0 && usedBytes+input.FileSize > maxStorage {
		return apperror.NewBadRequest("campaign storage quota exceeded")
	}
	if maxFiles > 0 && fileCount+1 > maxFiles {
		return apperror.NewBadRequest("campaign file count limit reached")
	}

	return nil
}

// GetByID retrieves a media file by ID.
func (s *mediaService) GetByID(ctx context.Context, id string) (*MediaFile, error) {
	return s.repo.FindByID(ctx, id)
}

// Delete removes a media file from disk and database.
func (s *mediaService) Delete(ctx context.Context, id string) error {
	file, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}

	// Delete from database first.
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	// Delete main file from disk. Errors are intentionally ignored —
	// orphaned files are preferable to failing a successful DB delete.
	mainPath := filepath.Join(s.mediaPath, file.Filename)
	_ = os.Remove(mainPath)

	// Delete thumbnails.
	for _, thumbFile := range file.ThumbnailPaths {
		_ = os.Remove(filepath.Join(s.mediaPath, thumbFile))
	}

	slog.Info("media file deleted", slog.String("id", id))
	return nil
}

// FilePath returns the absolute path to a media file on disk.
func (s *mediaService) FilePath(file *MediaFile) string {
	return filepath.Join(s.mediaPath, file.Filename)
}

// ThumbnailPath returns the absolute path to a thumbnail on disk.
func (s *mediaService) ThumbnailPath(file *MediaFile, size string) string {
	if thumbFile, ok := file.ThumbnailPaths[size]; ok {
		return filepath.Join(s.mediaPath, thumbFile)
	}
	return s.FilePath(file)
}

// ListCampaignMedia returns paginated media files for a campaign.
func (s *mediaService) ListCampaignMedia(ctx context.Context, campaignID string, page, perPage int) ([]MediaFile, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage
	return s.repo.ListByCampaign(ctx, campaignID, perPage, offset)
}

// GetCampaignStats returns aggregate storage stats for a campaign.
func (s *mediaService) GetCampaignStats(ctx context.Context, campaignID string) (*CampaignMediaStats, error) {
	totalBytes, fileCount, err := s.repo.GetCampaignUsage(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return &CampaignMediaStats{
		TotalFiles: fileCount,
		TotalBytes: totalBytes,
	}, nil
}

// FindReferences returns entities that reference a media file.
func (s *mediaService) FindReferences(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
	return s.repo.FindReferences(ctx, campaignID, mediaID)
}

// DeleteCampaignMedia deletes a media file after verifying it belongs to the campaign.
func (s *mediaService) DeleteCampaignMedia(ctx context.Context, campaignID, mediaID string) error {
	file, err := s.repo.FindByID(ctx, mediaID)
	if err != nil {
		return err
	}

	// Verify the file belongs to this campaign.
	if file.CampaignID == nil || *file.CampaignID != campaignID {
		return apperror.NewNotFound("media file not found")
	}

	return s.Delete(ctx, mediaID)
}

// DeleteCampaignFiles removes all media files belonging to a campaign.
// Deletes physical files from disk (main + thumbnails) and then removes
// the database records. Called before campaign SQL DELETE to prevent orphaned
// media. Errors on individual files are logged but do not abort the operation.
func (s *mediaService) DeleteCampaignFiles(ctx context.Context, campaignID string) (int, error) {
	files, err := s.repo.ListFilesByCampaign(ctx, campaignID)
	if err != nil {
		return 0, fmt.Errorf("listing campaign files: %w", err)
	}

	deleted := 0
	for _, f := range files {
		if err := s.Delete(ctx, f.ID); err != nil {
			slog.Warn("failed to delete media file during campaign cleanup",
				slog.String("file_id", f.ID),
				slog.String("campaign_id", campaignID),
				slog.Any("error", err),
			)
			continue
		}
		deleted++
	}

	slog.Info("campaign media cleanup completed",
		slog.String("campaign_id", campaignID),
		slog.Int("total", len(files)),
		slog.Int("deleted", deleted),
	)
	return deleted, nil
}

// CleanupOrphans walks the media directory, checks each file against the
// database, and deletes any files not tracked. This handles the case where
// an upload crashes between writing the file and saving the DB record.
func (s *mediaService) CleanupOrphans(ctx context.Context) (int, error) {
	knownFiles, err := s.repo.ListAllFilenames(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing known files: %w", err)
	}

	removed := 0
	err = filepath.Walk(s.mediaPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip unreadable entries.
		}
		if info.IsDir() {
			return nil
		}

		// Get the relative path from mediaPath for comparison with DB filenames.
		rel, relErr := filepath.Rel(s.mediaPath, path)
		if relErr != nil {
			return nil
		}

		if !knownFiles[rel] {
			// Grace period: skip files younger than 15 minutes to avoid
			// deleting files from in-progress uploads (TOCTOU race).
			if time.Since(info.ModTime()) < 15*time.Minute {
				slog.Debug("skipping recent orphan file",
					slog.String("path", rel),
					slog.Duration("age", time.Since(info.ModTime())),
				)
				return nil
			}

			if removeErr := os.Remove(path); removeErr == nil {
				removed++
				slog.Info("removed orphan media file", slog.String("path", rel))
			} else {
				slog.Warn("failed to remove orphan media file",
					slog.String("path", rel),
					slog.Any("error", removeErr),
				)
			}
		}
		return nil
	})
	if err != nil {
		return removed, fmt.Errorf("walking media directory: %w", err)
	}

	slog.Info("orphan cleanup completed", slog.Int("removed", removed))
	return removed, nil
}

// maxImageDimension is the maximum width or height in pixels for uploaded images.
// Images larger than this are rejected to prevent decompression bomb attacks
// (e.g., a tiny PNG that decompresses to gigabytes in memory).
const maxImageDimension = 10000

// generateThumbnail creates a resized copy of an image.
func (s *mediaService) generateThumbnail(data []byte, dir, id, ext string, maxDim int) (string, error) {
	// Check image dimensions before full decode to prevent decompression bombs.
	// DecodeConfig reads only the header, using minimal memory.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("reading image config: %w", err)
	}
	if cfg.Width > maxImageDimension || cfg.Height > maxImageDimension {
		return "", fmt.Errorf("image too large: %dx%d exceeds %d limit", cfg.Width, cfg.Height, maxImageDimension)
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("decoding image: %w", err)
	}

	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Skip if already small enough.
	if w <= maxDim && h <= maxDim {
		return "", fmt.Errorf("image already smaller than %d", maxDim)
	}

	// Calculate new dimensions maintaining aspect ratio.
	newW, newH := maxDim, maxDim
	if w > h {
		newH = h * maxDim / w
	} else {
		newW = w * maxDim / h
	}

	// Resize using Catmull-Rom interpolation.
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	// Write thumbnail.
	thumbFilename := fmt.Sprintf("%s_%d%s", id, maxDim, ext)
	thumbPath := filepath.Join(dir, thumbFilename)

	f, err := os.Create(thumbPath)
	if err != nil {
		return "", fmt.Errorf("creating thumbnail file: %w", err)
	}
	defer f.Close()

	switch ext {
	case ".jpg", ".jpeg":
		err = jpeg.Encode(f, dst, &jpeg.Options{Quality: 85})
	case ".png":
		err = png.Encode(f, dst)
	case ".gif":
		err = gif.Encode(f, dst, nil)
	default:
		// For WebP and others, encode as JPEG thumbnail.
		err = jpeg.Encode(f, dst, &jpeg.Options{Quality: 85})
	}

	if err != nil {
		_ = os.Remove(thumbPath)
		return "", fmt.Errorf("encoding thumbnail: %w", err)
	}

	return thumbFilename, nil
}

// validateMagicBytes checks that the file content's magic bytes match the
// declared MIME type. Prevents uploading non-image files with a spoofed
// Content-Type header.
func validateMagicBytes(data []byte, declaredMIME string) bool {
	if len(data) < 4 {
		return false
	}
	switch declaredMIME {
	case "image/jpeg":
		return len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF
	case "image/png":
		return len(data) >= 8 &&
			data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 &&
			data[4] == 0x0D && data[5] == 0x0A && data[6] == 0x1A && data[7] == 0x0A
	case "image/gif":
		return len(data) >= 6 && string(data[:3]) == "GIF"
	case "image/webp":
		return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
	default:
		return false
	}
}

// checkDiskSpace verifies that writing a file of the given size will leave at
// least minFreeDiskBytes of free space. Prevents media uploads from filling
// the filesystem and breaking other services.
func checkDiskSpace(path string, fileSize int64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		slog.Warn("disk space check failed, allowing upload",
			slog.String("path", path),
			slog.Any("error", err),
		)
		return nil // Don't block uploads if statfs fails.
	}
	available := int64(stat.Bavail) * int64(stat.Bsize)
	if available-fileSize < minFreeDiskBytes {
		return apperror.NewInternal(fmt.Errorf("insufficient disk space: %d bytes available, need %d + %d reserve",
			available, fileSize, minFreeDiskBytes))
	}
	return nil
}

// generateUUID creates a new v4 UUID string using crypto/rand.
// Panics if the system entropy source fails, as this indicates a
// catastrophic system problem that would compromise all security.
func generateUUID() string {
	uuid := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, uuid); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
