package media

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// MemberChecker verifies campaign membership without importing the full
// campaigns service. Implemented via an adapter in app/routes.go.
type MemberChecker interface {
	IsCampaignMember(campaignID, userID string) bool
}

// SecurityEventLogger records security events for the admin security dashboard.
// Implemented by the admin security service; wired after both are initialized.
type SecurityEventLogger interface {
	LogEvent(ctx context.Context, eventType, userID, actorID, ip, userAgent string, details map[string]any) error
}

// Handler handles HTTP requests for media operations.
type Handler struct {
	service        MediaService
	signer         *URLSigner
	memberChecker  MemberChecker
	securityLogger SecurityEventLogger
}

// NewHandler creates a new media handler.
func NewHandler(service MediaService) *Handler {
	return &Handler{service: service}
}

// SetURLSigner sets the HMAC URL signer for signed URL generation and
// verification. Called during wiring in app/routes.go.
func (h *Handler) SetURLSigner(signer *URLSigner) {
	h.signer = signer
}

// SetMemberChecker sets the campaign membership checker for access control
// on private campaign media. Called during wiring in app/routes.go.
func (h *Handler) SetMemberChecker(checker MemberChecker) {
	h.memberChecker = checker
}

// SetSecurityLogger wires a security event logger for recording media events
// (uploads, deletes, quota failures). Called during wiring in app/routes.go.
func (h *Handler) SetSecurityLogger(logger SecurityEventLogger) {
	h.securityLogger = logger
}

// logSecurityEvent fires a security event if a logger is wired. Fire-and-forget
// so media operations are never blocked by logging failures.
func (h *Handler) logSecurityEvent(ctx context.Context, eventType, userID, actorID, ip, userAgent string, details map[string]any) {
	if h.securityLogger != nil {
		_ = h.securityLogger.LogEvent(ctx, eventType, userID, actorID, ip, userAgent, details)
	}
}

// Upload handles multipart file uploads (POST /media/upload).
func (h *Handler) Upload(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}

	file, err := c.FormFile("file")
	if err != nil {
		slog.Warn("media upload: FormFile error",
			slog.String("user_id", userID),
			slog.Any("error", err),
		)
		return apperror.NewBadRequest("no file provided")
	}

	src, err := file.Open()
	if err != nil {
		slog.Error("media upload: file open error",
			slog.String("user_id", userID),
			slog.Any("error", err),
		)
		return apperror.NewInternal(err)
	}
	defer func() { _ = src.Close() }()

	fileBytes, err := io.ReadAll(src)
	if err != nil {
		// MaxBytesReader returns a specific error when body exceeds limit.
		if err.Error() == "http: request body too large" {
			return apperror.NewBadRequest("file too large")
		}
		slog.Error("media upload: read error",
			slog.String("user_id", userID),
			slog.Any("error", err),
		)
		return apperror.NewInternal(err)
	}

	// Detect MIME type from file content if browser didn't provide one.
	declaredMime := file.Header.Get("Content-Type")
	if declaredMime == "" || declaredMime == "application/octet-stream" {
		declaredMime = http.DetectContentType(fileBytes)
	}

	input := UploadInput{
		CampaignID:   c.FormValue("campaign_id"),
		UploadedBy:   userID,
		OriginalName: file.Filename,
		MimeType:     declaredMime,
		FileSize:     int64(len(fileBytes)),
		UsageType:    c.FormValue("usage_type"),
		FileBytes:    fileBytes,
	}

	if input.UsageType == "" {
		input.UsageType = UsageAttachment
	}

	mediaFile, err := h.service.Upload(c.Request().Context(), input)
	if err != nil {
		// Log quota-exceeded errors as security events for admin visibility.
		if apperror.SafeCode(err) == http.StatusBadRequest &&
			(strings.Contains(err.Error(), "quota") || strings.Contains(err.Error(), "limit")) {
			h.logSecurityEvent(c.Request().Context(), "media.quota_exceeded",
				userID, "", c.RealIP(), c.Request().UserAgent(),
				map[string]any{
					"campaign_id": input.CampaignID,
					"mime_type":   input.MimeType,
					"size":        input.FileSize,
					"reason":      apperror.SafeMessage(err),
				})
		}
		return err
	}

	h.logSecurityEvent(c.Request().Context(), "media.uploaded",
		userID, "", c.RealIP(), c.Request().UserAgent(),
		map[string]any{
			"file_id":     mediaFile.ID,
			"campaign_id": input.CampaignID,
			"mime_type":   mediaFile.MimeType,
			"size":        mediaFile.FileSize,
		})

	// Return signed URLs if signer is available.
	var url, thumbURL string
	if h.signer != nil {
		url = h.signer.Sign(mediaFile.ID, 1*time.Hour)
		if _, ok := mediaFile.ThumbnailPaths["300"]; ok {
			thumbURL = h.signer.SignThumb(mediaFile.ID, "300", 1*time.Hour)
		}
	} else {
		url = "/media/" + mediaFile.ID
		if _, ok := mediaFile.ThumbnailPaths["300"]; ok {
			thumbURL = "/media/" + mediaFile.ID + "/thumb/300"
		}
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
// Enforces HMAC-signed URL verification for campaign media and access
// control for private campaigns. Files without a campaign (avatars,
// backdrops) are served without signing.
func (h *Handler) Serve(c echo.Context) error {
	fileID := c.Param("id")
	fileID = strings.TrimSuffix(fileID, "/")

	file, err := h.service.GetByID(c.Request().Context(), fileID)
	if err != nil {
		return err
	}

	// Enforce access control.
	if err := h.checkMediaAccess(c, file, false, ""); err != nil {
		return err
	}

	filePath := h.service.FilePath(file)
	h.setSecurityHeaders(c, file)
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

	// Enforce access control (includes size in signature check).
	if err := h.checkMediaAccess(c, file, true, size); err != nil {
		return err
	}

	thumbPath := h.service.ThumbnailPath(file, size)
	h.setSecurityHeaders(c, file)

	return c.File(thumbPath)
}

// checkMediaAccess enforces signed URL verification and private campaign
// access control. Returns nil if access is allowed, or an error to return
// to the client.
func (h *Handler) checkMediaAccess(c echo.Context, file *MediaFile, isThumb bool, thumbSize string) error {
	// Files without a campaign (avatars, backdrops) are public.
	if file.CampaignID == nil {
		return nil
	}

	fileID := file.ID
	expiresStr := c.QueryParam("expires")
	sig := c.QueryParam("sig")

	// Check signed URL if signer is configured.
	if h.signer != nil {
		signatureValid := false
		if expiresStr != "" && sig != "" {
			if isThumb {
				signatureValid = h.signer.VerifyThumb(fileID, thumbSize, expiresStr, sig)
			} else {
				signatureValid = h.signer.Verify(fileID, expiresStr, sig)
			}
		}

		if !signatureValid {
			// No valid signature. Fall back: allow if user is authenticated
			// and is a member of the file's campaign (graceful migration).
			if !h.allowUnsignedAccess(c, file) {
				return apperror.NewForbidden("signed URL required")
			}
		}
	}

	// Defense-in-depth: for private campaigns, also require authenticated
	// campaign membership even if the signed URL is valid.
	if file.CampaignIsPublic != nil && !*file.CampaignIsPublic {
		userID := auth.GetUserID(c)
		if userID == "" {
			return apperror.NewNotFound("media file not found")
		}
		if h.memberChecker != nil && !h.memberChecker.IsCampaignMember(*file.CampaignID, userID) {
			// Also allow site admins.
			session := auth.GetSession(c)
			if session == nil || !session.IsAdmin {
				return apperror.NewNotFound("media file not found")
			}
		}
	}

	return nil
}

// allowUnsignedAccess is the fallback when no valid signed URL is present.
// Allows access for authenticated campaign members so old unsigned URLs
// still work during the migration period.
func (h *Handler) allowUnsignedAccess(c echo.Context, file *MediaFile) bool {
	// Public campaigns: allow unsigned access (backward compatible).
	if file.CampaignIsPublic != nil && *file.CampaignIsPublic {
		return true
	}

	// Authenticated user who is a campaign member.
	userID := auth.GetUserID(c)
	if userID != "" && file.CampaignID != nil {
		if h.memberChecker != nil && h.memberChecker.IsCampaignMember(*file.CampaignID, userID) {
			return true
		}
		// Site admins always have access.
		if session := auth.GetSession(c); session != nil && session.IsAdmin {
			return true
		}
	}

	return false
}

// setSecurityHeaders applies defense-in-depth headers to media responses.
func (h *Handler) setSecurityHeaders(c echo.Context, file *MediaFile) {
	resp := c.Response()

	// Force browser to respect declared Content-Type (prevents MIME sniffing).
	resp.Header().Set("X-Content-Type-Options", "nosniff")

	// Safe filename for Content-Disposition. Serve images inline with a
	// sanitized filename to prevent header injection.
	resp.Header().Set("Content-Disposition",
		fmt.Sprintf(`inline; filename="%s"`, sanitizeFilename(file.OriginalName)))

	// Prevent media URLs from being embedded as iframes.
	resp.Header().Set("X-Frame-Options", "DENY")

	// Restrictive CSP on media responses: no scripts, no styles.
	resp.Header().Set("Content-Security-Policy",
		"default-src 'none'; img-src 'self'; style-src 'none'; script-src 'none'")

	// Prevent referrer leakage of signed URLs or UUIDs.
	resp.Header().Set("Referrer-Policy", "no-referrer")

	// Cache control based on campaign privacy.
	if file.CampaignIsPublic != nil && !*file.CampaignIsPublic {
		// Private campaign media must not be cached by shared proxies.
		resp.Header().Set("Cache-Control", "private, no-store, max-age=0")
	} else {
		// Public/orphan media: cache aggressively (UUID filenames are immutable).
		resp.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
}

// sanitizeFilename strips characters that could be used for header injection,
// allowing only safe characters for Content-Disposition filenames.
func sanitizeFilename(name string) string {
	safe := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) ||
			r == '-' || r == '_' || r == '.' || r == ' ' {
			return r
		}
		return '_'
	}, name)
	if safe == "" {
		safe = "file"
	}
	return safe
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

	var campaignID string
	if file.CampaignID != nil {
		campaignID = *file.CampaignID
	}
	h.logSecurityEvent(c.Request().Context(), "media.deleted",
		userID, "", c.RealIP(), c.Request().UserAgent(),
		map[string]any{
			"file_id":     fileID,
			"campaign_id": campaignID,
		})

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Campaign-scoped media browser ---

// CampaignMediaPageData holds all data for rendering the campaign media browser.
type CampaignMediaPageData struct {
	Files     []MediaFile
	Stats     *CampaignMediaStats
	Total     int
	Page      int
	PerPage   int
	CSRFToken string
}

// CampaignMediaList returns paginated campaign media as JSON. Drives
// the media-picker widget (the slideout the operator opens via "Choose
// from campaign" next to file inputs). Distinct from CampaignMedia
// which renders the admin browser HTML page.
//
// Scribe+ at the route level. The handler also folds in URL strings
// (signed media URLs and thumbnails) so the widget doesn't need to
// know the signing scheme — keeps the picker decoupled from media-
// internal details.
//
// GET /campaigns/:id/media/list?page=N&perPage=M
func (h *Handler) CampaignMediaList(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.QueryParam("perPage"))
	if perPage <= 0 || perPage > 100 {
		perPage = 24
	}

	files, total, err := h.service.ListCampaignMedia(ctx, cc.Campaign.ID, page, perPage)
	if err != nil {
		return err
	}

	type mediaListItem struct {
		ID           string    `json:"id"`
		OriginalName string    `json:"original_name"`
		MimeType     string    `json:"mime_type"`
		FileSize     int64     `json:"file_size"`
		URL          string    `json:"url"`
		ThumbnailURL string    `json:"thumbnail_url,omitempty"`
		CreatedAt    time.Time `json:"created_at"`
	}

	out := make([]mediaListItem, 0, len(files))
	for i := range files {
		f := &files[i]
		item := mediaListItem{
			ID:           f.ID,
			OriginalName: f.OriginalName,
			MimeType:     f.MimeType,
			FileSize:     f.FileSize,
			CreatedAt:    f.CreatedAt,
		}
		// Same signed-URL logic the upload handler uses — keep the
		// picker's URLs valid for an hour, fall back to unsigned when
		// no signer is configured (dev / tests).
		if h.signer != nil {
			item.URL = h.signer.Sign(f.ID, 1*time.Hour)
			if thumb := pickThumbnail(f); thumb != "" {
				item.ThumbnailURL = h.signer.SignThumb(f.ID, thumb, 1*time.Hour)
			}
		} else {
			item.URL = "/media/" + f.ID
			if thumb := pickThumbnail(f); thumb != "" {
				item.ThumbnailURL = "/media/" + f.ID + "/thumb/" + thumb
			}
		}
		out = append(out, item)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"items":    out,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// pickThumbnail picks the smallest available thumbnail size key for a
// file. Picker UIs prefer small thumbs to keep the slideout snappy.
func pickThumbnail(f *MediaFile) string {
	if !f.IsImage() || len(f.ThumbnailPaths) == 0 {
		return ""
	}
	// Prefer 300px if present; fall back to anything else.
	if _, ok := f.ThumbnailPaths["300"]; ok {
		return "300"
	}
	for size := range f.ThumbnailPaths {
		return size
	}
	return ""
}

// CampaignMedia renders the campaign media browser page (GET /campaigns/:id/media).
func (h *Handler) CampaignMedia(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewNotFound("campaign not found")
	}

	ctx := c.Request().Context()

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage := 24

	files, total, err := h.service.ListCampaignMedia(ctx, cc.Campaign.ID, page, perPage)
	if err != nil {
		return err
	}

	stats, err := h.service.GetCampaignStats(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}

	data := CampaignMediaPageData{
		Files:     files,
		Stats:     stats,
		Total:     total,
		Page:      page,
		PerPage:   perPage,
		CSRFToken: middleware.GetCSRFToken(c),
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, CampaignMediaFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, CampaignMediaPage(cc, data))
}

// CampaignDeleteMedia handles deletion of a campaign media file
// (DELETE /campaigns/:id/media/:mid).
func (h *Handler) CampaignDeleteMedia(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewNotFound("campaign not found")
	}

	mediaID := c.Param("mid")
	if err := h.service.DeleteCampaignMedia(c.Request().Context(), cc.Campaign.ID, mediaID); err != nil {
		return err
	}

	h.logSecurityEvent(c.Request().Context(), "media.deleted",
		auth.GetUserID(c), "", c.RealIP(), c.Request().UserAgent(),
		map[string]any{
			"file_id":     mediaID,
			"campaign_id": cc.Campaign.ID,
		})

	// Redirect back to media page for HTMX and standard requests.
	c.Response().Header().Set("HX-Redirect", fmt.Sprintf("/campaigns/%s/media", cc.Campaign.ID))
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// CampaignMediaRefs returns an HTMX fragment showing which entities reference
// a media file (GET /campaigns/:id/media/:mid/refs).
func (h *Handler) CampaignMediaRefs(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewNotFound("campaign not found")
	}

	mediaID := c.Param("mid")
	refs, err := h.service.FindReferences(c.Request().Context(), cc.Campaign.ID, mediaID)
	if err != nil {
		return err
	}

	return middleware.Render(c, http.StatusOK, MediaRefsFragment(cc, mediaID, refs))
}
