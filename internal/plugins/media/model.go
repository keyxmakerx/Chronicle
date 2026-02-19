// Package media manages file uploads, storage, and serving for Chronicle.
// Supports image uploads with automatic thumbnail generation at multiple sizes.
// Files are stored on the local filesystem in a date-based directory structure.
package media

import (
	"path/filepath"
	"strings"
	"time"
)

// MediaFile represents an uploaded file stored on disk.
type MediaFile struct {
	ID             string            `json:"id"`
	CampaignID     *string           `json:"campaign_id,omitempty"`
	UploadedBy     string            `json:"uploaded_by"`
	Filename       string            `json:"filename"`       // UUID-based filename on disk.
	OriginalName   string            `json:"original_name"`  // User's original filename.
	MimeType       string            `json:"mime_type"`
	FileSize       int64             `json:"file_size"`
	UsageType      string            `json:"usage_type"`     // attachment, entity_image, avatar, backdrop.
	ThumbnailPaths map[string]string `json:"thumbnail_paths"` // size -> filename (e.g., "300" -> "uuid_300.jpg").
	CreatedAt      time.Time         `json:"created_at"`
}

// UploadInput holds the validated input for creating a media file.
type UploadInput struct {
	CampaignID   string
	UploadedBy   string
	OriginalName string
	MimeType     string
	FileSize     int64
	UsageType    string
	FileBytes    []byte
}

// UploadResponse is the JSON response returned after a successful upload.
type UploadResponse struct {
	ID           string `json:"id"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
}

// --- MIME Type Validation ---

// AllowedMimeTypes defines which MIME types are accepted for upload.
var AllowedMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// MimeToExtension maps MIME types to file extensions.
var MimeToExtension = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// IsImage returns true if the file is an image based on MIME type.
func (f *MediaFile) IsImage() bool {
	return strings.HasPrefix(f.MimeType, "image/")
}

// Extension returns the file extension for this media file.
func (f *MediaFile) Extension() string {
	if ext, ok := MimeToExtension[f.MimeType]; ok {
		return ext
	}
	return filepath.Ext(f.OriginalName)
}

// Usage type constants.
const (
	UsageAttachment  = "attachment"
	UsageEntityImage = "entity_image"
	UsageAvatar      = "avatar"
	UsageBackdrop    = "backdrop"
)
