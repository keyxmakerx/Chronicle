package media

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// Handler handles HTTP requests for media operations.
type Handler struct {
	service MediaService
}

// NewHandler creates a new media handler.
func NewHandler(service MediaService) *Handler {
	return &Handler{service: service}
}

// Upload handles multipart file uploads (POST /media/upload).
func (h *Handler) Upload(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return apperror.NewBadRequest("no file provided")
	}

	src, err := file.Open()
	if err != nil {
		return apperror.NewInternal(err)
	}
	defer src.Close()

	fileBytes, err := io.ReadAll(src)
	if err != nil {
		return apperror.NewInternal(err)
	}

	input := UploadInput{
		CampaignID:   c.FormValue("campaign_id"),
		UploadedBy:   userID,
		OriginalName: file.Filename,
		MimeType:     file.Header.Get("Content-Type"),
		FileSize:     int64(len(fileBytes)),
		UsageType:    c.FormValue("usage_type"),
		FileBytes:    fileBytes,
	}

	if input.UsageType == "" {
		input.UsageType = UsageAttachment
	}

	mediaFile, err := h.service.Upload(c.Request().Context(), input)
	if err != nil {
		return err
	}

	url := "/media/" + mediaFile.ID
	thumbURL := ""
	if _, ok := mediaFile.ThumbnailPaths["300"]; ok {
		thumbURL = "/media/" + mediaFile.ID + "/thumb/300"
	}

	return c.JSON(http.StatusCreated, UploadResponse{
		ID:           mediaFile.ID,
		URL:          url,
		ThumbnailURL: thumbURL,
		MimeType:     mediaFile.MimeType,
		FileSize:     mediaFile.FileSize,
	})
}

// Serve serves a media file (GET /media/:id).
// Supports thumbnail serving via /media/:id/thumb/:size.
func (h *Handler) Serve(c echo.Context) error {
	fileID := c.Param("id")

	// Strip thumbnail suffix if present (handled by separate route).
	fileID = strings.TrimSuffix(fileID, "/")

	file, err := h.service.GetByID(c.Request().Context(), fileID)
	if err != nil {
		return err
	}

	filePath := h.service.FilePath(file)

	// Set cache headers for immutable content (UUID-based filenames never change).
	c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	c.Response().Header().Set("Content-Type", file.MimeType)

	return c.File(filePath)
}

// allowedThumbSizes restricts thumbnail size parameter to known values,
// preventing the size from being used as an arbitrary map key.
var allowedThumbSizes = map[string]bool{"300": true, "800": true}

// ServeThumbnail serves a thumbnail of a media file (GET /media/:id/thumb/:size).
func (h *Handler) ServeThumbnail(c echo.Context) error {
	fileID := c.Param("id")
	size := c.Param("size")

	if !allowedThumbSizes[size] {
		return apperror.NewBadRequest("invalid thumbnail size")
	}

	file, err := h.service.GetByID(c.Request().Context(), fileID)
	if err != nil {
		return err
	}

	thumbPath := h.service.ThumbnailPath(file, size)

	c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	return c.File(thumbPath)
}

// Info returns metadata about a media file (GET /media/:fileID/info).
// Requires authentication. Only the uploader or a site admin can view info.
func (h *Handler) Info(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}

	fileID := c.Param("fileID")
	file, err := h.service.GetByID(c.Request().Context(), fileID)
	if err != nil {
		return err
	}

	// Ownership check: only uploader or admin can see file info.
	session := auth.GetSession(c)
	if file.UploadedBy != userID && (session == nil || !session.IsAdmin) {
		return apperror.NewNotFound("media file not found")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"id":            file.ID,
		"original_name": file.OriginalName,
		"mime_type":     file.MimeType,
		"file_size":     file.FileSize,
		"usage_type":    file.UsageType,
		"created_at":    file.CreatedAt.Format(time.RFC3339),
		"thumbnails":    file.ThumbnailPaths,
	})
}

// Delete removes a media file (DELETE /media/:fileID).
// Only the uploader or a site admin can delete.
func (h *Handler) Delete(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}

	fileID := c.Param("fileID")
	file, err := h.service.GetByID(c.Request().Context(), fileID)
	if err != nil {
		return err
	}

	// Ownership check.
	session := auth.GetSession(c)
	if file.UploadedBy != userID && (session == nil || !session.IsAdmin) {
		return apperror.NewNotFound("media file not found")
	}

	if err := h.service.Delete(c.Request().Context(), fileID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
