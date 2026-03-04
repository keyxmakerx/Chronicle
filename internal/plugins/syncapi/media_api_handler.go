package syncapi

import (
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// MediaAPIHandler serves media endpoints for the REST API v1.
// External clients (Foundry VTT, custom scripts) use these endpoints to
// list, upload, and delete campaign media files via API key authentication.
type MediaAPIHandler struct {
	syncSvc  SyncAPIService
	mediaSvc media.MediaService
	signer   *media.URLSigner
}

// NewMediaAPIHandler creates a new media API handler.
func NewMediaAPIHandler(syncSvc SyncAPIService, mediaSvc media.MediaService) *MediaAPIHandler {
	return &MediaAPIHandler{
		syncSvc:  syncSvc,
		mediaSvc: mediaSvc,
	}
}

// SetURLSigner sets the HMAC URL signer for generating signed media URLs in
// API responses. Called during wiring in app/routes.go.
func (h *MediaAPIHandler) SetURLSigner(signer *media.URLSigner) {
	h.signer = signer
}

// apiMediaFileResponse is the API-safe representation of a media file.
// Includes signed URLs for direct file access when a signer is configured.
type apiMediaFileResponse struct {
	ID           string            `json:"id"`
	CampaignID   *string           `json:"campaign_id,omitempty"`
	UploadedBy   string            `json:"uploaded_by"`
	OriginalName string            `json:"original_name"`
	MimeType     string            `json:"mime_type"`
	FileSize     int64             `json:"file_size"`
	UsageType    string            `json:"usage_type"`
	URL          string            `json:"url"`
	ThumbnailURL string            `json:"thumbnail_url,omitempty"`
	Thumbnails   map[string]string `json:"thumbnails,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

// toAPIResponse converts a MediaFile to an API-safe response with signed URLs.
func (h *MediaAPIHandler) toAPIResponse(file *media.MediaFile) apiMediaFileResponse {
	resp := apiMediaFileResponse{
		ID:           file.ID,
		CampaignID:   file.CampaignID,
		UploadedBy:   file.UploadedBy,
		OriginalName: file.OriginalName,
		MimeType:     file.MimeType,
		FileSize:     file.FileSize,
		UsageType:    file.UsageType,
		CreatedAt:    file.CreatedAt,
	}

	// Generate signed URLs if signer is available.
	if h.signer != nil {
		resp.URL = h.signer.Sign(file.ID, 1*time.Hour)
		resp.Thumbnails = make(map[string]string)
		for size := range file.ThumbnailPaths {
			resp.Thumbnails[size] = h.signer.SignThumb(file.ID, size, 1*time.Hour)
		}
		if thumbURL, ok := resp.Thumbnails["300"]; ok {
			resp.ThumbnailURL = thumbURL
		}
	} else {
		resp.URL = "/media/" + file.ID
		if _, ok := file.ThumbnailPaths["300"]; ok {
			resp.ThumbnailURL = "/media/" + file.ID + "/thumb/300"
		}
	}

	return resp
}

// ListMedia returns paginated media files for the campaign.
// GET /api/v1/campaigns/:id/media?page=1&per_page=20
func (h *MediaAPIHandler) ListMedia(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	page, _ := strconv.Atoi(c.QueryParam("page"))
	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	files, total, err := h.mediaSvc.ListCampaignMedia(ctx, campaignID, page, perPage)
	if err != nil {
		slog.Error("api: failed to list media", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list media")
	}

	data := make([]apiMediaFileResponse, 0, len(files))
	for i := range files {
		data = append(data, h.toAPIResponse(&files[i]))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data":     data,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// GetMedia returns metadata for a single media file.
// GET /api/v1/campaigns/:id/media/:mediaID
func (h *MediaAPIHandler) GetMedia(c echo.Context) error {
	mediaID := c.Param("mediaID")
	ctx := c.Request().Context()

	file, err := h.mediaSvc.GetByID(ctx, mediaID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "media file not found")
	}

	// IDOR protection: verify file belongs to this campaign.
	if file.CampaignID == nil || *file.CampaignID != c.Param("id") {
		return echo.NewHTTPError(http.StatusNotFound, "media file not found")
	}

	return c.JSON(http.StatusOK, h.toAPIResponse(file))
}

// GetMediaStats returns aggregate storage stats for the campaign.
// GET /api/v1/campaigns/:id/media/stats
func (h *MediaAPIHandler) GetMediaStats(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	stats, err := h.mediaSvc.GetCampaignStats(ctx, campaignID)
	if err != nil {
		slog.Error("api: failed to get media stats", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get media stats")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"total_files": stats.TotalFiles,
		"total_bytes": stats.TotalBytes,
	})
}

// maxAPIUploadBody is the maximum request body size for API media uploads.
// Set to 11 MB (10 MB file limit + 10% margin for multipart encoding overhead).
const maxAPIUploadBody = 11 * 1024 * 1024

// UploadMedia handles file upload via multipart form.
// POST /api/v1/campaigns/:id/media
//
// Form fields:
//   - file: the file to upload (required)
//   - usage_type: "attachment", "entity_image", "avatar", "backdrop" (optional, defaults to "attachment")
func (h *MediaAPIHandler) UploadMedia(c echo.Context) error {
	// Limit request body size to prevent memory exhaustion from oversized payloads.
	// The media service enforces the exact per-file limit; this is a coarse guard.
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxAPIUploadBody)

	key := GetAPIKey(c)
	if key == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "api key required")
	}

	campaignID := c.Param("id")

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "no file provided")
	}

	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}
	defer func() { _ = src.Close() }()

	fileBytes, err := io.ReadAll(src)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}

	usageType := c.FormValue("usage_type")
	if usageType == "" {
		usageType = media.UsageAttachment
	}

	input := media.UploadInput{
		CampaignID:   campaignID,
		UploadedBy:   key.UserID,
		OriginalName: file.Filename,
		MimeType:     file.Header.Get("Content-Type"),
		FileSize:     int64(len(fileBytes)),
		UsageType:    usageType,
		FileBytes:    fileBytes,
	}

	mediaFile, err := h.mediaSvc.Upload(c.Request().Context(), input)
	if err != nil {
		return echo.NewHTTPError(apperror.SafeCode(err), apperror.SafeMessage(err))
	}

	return c.JSON(http.StatusCreated, h.toAPIResponse(mediaFile))
}

// DeleteMedia deletes a media file from the campaign.
// DELETE /api/v1/campaigns/:id/media/:mediaID
func (h *MediaAPIHandler) DeleteMedia(c echo.Context) error {
	mediaID := c.Param("mediaID")
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	// Verify file belongs to this campaign.
	file, err := h.mediaSvc.GetByID(ctx, mediaID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "media file not found")
	}
	if file.CampaignID == nil || *file.CampaignID != campaignID {
		return echo.NewHTTPError(http.StatusNotFound, "media file not found")
	}

	if err := h.mediaSvc.Delete(ctx, mediaID); err != nil {
		slog.Error("api: failed to delete media", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete media")
	}

	return c.NoContent(http.StatusNoContent)
}
