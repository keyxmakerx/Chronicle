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
	"time"

	// Register decoders for image formats.
	_ "golang.org/x/image/webp"

	"golang.org/x/image/draw"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// MediaService handles business logic for media file operations.
type MediaService interface {
	Upload(ctx context.Context, input UploadInput) (*MediaFile, error)
	GetByID(ctx context.Context, id string) (*MediaFile, error)
	Delete(ctx context.Context, id string) error
	FilePath(file *MediaFile) string
	ThumbnailPath(file *MediaFile, size string) string
}

// mediaService implements MediaService.
type mediaService struct {
	repo      MediaRepository
	mediaPath string // Root directory for file storage.
	maxSize   int64  // Maximum file size in bytes.
}

// NewMediaService creates a new media service.
func NewMediaService(repo MediaRepository, mediaPath string, maxSize int64) MediaService {
	return &mediaService{
		repo:      repo,
		mediaPath: mediaPath,
		maxSize:   maxSize,
	}
}

// Upload validates, stores, and records a new media file.
func (s *mediaService) Upload(ctx context.Context, input UploadInput) (*MediaFile, error) {
	// Validate MIME type.
	if !AllowedMimeTypes[input.MimeType] {
		return nil, apperror.NewBadRequest("unsupported file type: " + input.MimeType)
	}

	// Validate file size.
	if input.FileSize > s.maxSize {
		return nil, apperror.NewBadRequest(fmt.Sprintf("file too large; maximum size is %d MB", s.maxSize/(1024*1024)))
	}

	// Validate magic bytes match declared MIME type.
	if !validateMagicBytes(input.FileBytes, input.MimeType) {
		return nil, apperror.NewBadRequest("file content does not match declared type")
	}

	// Generate UUID filename in date-based directory.
	id := generateUUID()
	now := time.Now().UTC()
	dir := filepath.Join(s.mediaPath, now.Format("2006/01"))
	ext := MimeToExtension[input.MimeType]
	filename := id + ext

	// Create directory.
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating media directory: %w", err))
	}

	// Write file to disk.
	fullPath := filepath.Join(dir, filename)
	if err := os.WriteFile(fullPath, input.FileBytes, 0644); err != nil {
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

	// Generate thumbnails for images.
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
		// Clean up disk file on DB failure.
		os.Remove(fullPath)
		return nil, apperror.NewInternal(fmt.Errorf("saving media record: %w", err))
	}

	slog.Info("media file uploaded",
		slog.String("id", id),
		slog.String("mime_type", input.MimeType),
		slog.Int64("size", input.FileSize),
	)
	return file, nil
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

	// Delete main file from disk.
	mainPath := filepath.Join(s.mediaPath, file.Filename)
	os.Remove(mainPath)

	// Delete thumbnails.
	for _, thumbFile := range file.ThumbnailPaths {
		os.Remove(filepath.Join(s.mediaPath, thumbFile))
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

// generateThumbnail creates a resized copy of an image.
func (s *mediaService) generateThumbnail(data []byte, dir, id, ext string, maxDim int) (string, error) {
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
		os.Remove(thumbPath)
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

// generateUUID creates a new v4 UUID string using crypto/rand.
func generateUUID() string {
	uuid := make([]byte, 16)
	_, _ = io.ReadFull(rand.Reader, uuid)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
