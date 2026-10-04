package syncapi

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// MediaAPIHandler serves media endpoints for the REST API v1.
// External clients (Foundry VTT, custom scripts) use these endpoints to
// list, upload, and delete campaign media files via API key authentication.
type MediaAPIHandler struct {
	syncSvc     SyncAPIService
	mediaSvc    media.MediaService
	signer      *media.URLSigner
	campaignSvc campaigns.CampaignService
	// mapImages withholds the links of a shadowed map's picture from callers
	// below owner. Nil means unwired (tests); production wiring is pinned in app.
	mapImages media.MapImageGuard
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

// SetCampaignService wires campaign membership lookups, needed to resolve
// the caller's role for ListMedia's visibility gate. Called during wiring
// in app/routes.go.
func (h *MediaAPIHandler) SetCampaignService(svc campaigns.CampaignService) {
	h.campaignSvc = svc
}

// SetMapImageGuard wires the shadowed-map-picture check. Called during wiring in
// app/routes.go.
func (h *MediaAPIHandler) SetMapImageGuard(g media.MapImageGuard) {
	h.mapImages = g
}

// response is toAPIResponse minus every link, for a caller below owner, when the
// file is the picture of any map. The links are minted for the cookieless
// API-key flow, which the media server accepts without knowing who asked, and a
// link stays valid for its whole lifetime: one minted before a DM adds a shadow
// would still fetch the original afterwards. So a map picture gets no link for
// such a caller, shadowed or not. A failed check withholds too.
func (h *MediaAPIHandler) response(c echo.Context, file *media.MediaFile) apiMediaFileResponse {
	resp := h.toAPIResponse(file)
	if h.mapImages == nil || file.CampaignID == nil || h.resolveRole(c) >= int(campaigns.RoleOwner) {
		return resp
	}
	isPicture, err := h.mapImages.IsMapPicture(c.Request().Context(), *file.CampaignID, file.ID)
	if err != nil {
		slog.Error("api: map picture check failed; withholding links",
			slog.String("file_id", file.ID), slog.Any("error", err))
		isPicture = true
	}
	if isPicture {
		resp.URL, resp.ThumbnailURL, resp.Thumbnails = "", "", nil
	}
	return resp
}

// resolveRole returns the caller's effective campaign role for ListMedia's
// visibility gate. This intentionally mirrors only the ROLE half of
// APIHandler.resolveRole (api_handler.go) — not its key-owner-degraded
// signal, which is orthogonal to what a caller may read and isn't needed
// here:
//   - A session-authed caller (synthetic key, ID == synthKeySessionID) gets
//     their live campaign membership role.
//   - A real stored Bearer key always resolves to Owner (keys are strictly
//     Owner-minted; see APIHandler.resolveRole's doc comment for the full
//     rationale). Every existing integration (Foundry included) authenticates
//     this way, so this gate never affects it.
//
// Returns campaigns.RoleNone if no campaign service is wired or no key is
// present — fails closed into the "must filter" branch, never open.
func (h *MediaAPIHandler) resolveRole(c echo.Context) int {
	key := GetAPIKey(c)
	if key == nil || h.campaignSvc == nil {
		return int(campaigns.RoleNone)
	}
	if key.ID == synthKeySessionID {
		member, err := h.campaignSvc.GetMember(c.Request().Context(), key.CampaignID, key.UserID)
		if err != nil {
			return int(campaigns.RoleNone)
		}
		return int(member.Role)
	}
	return int(campaigns.RoleOwner)
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

// toAPIResponse converts a MediaFile to an API-safe response with signed
// URLs. Every URL is minted for media.ViewerAPIKey (ADR-058): this handler
// serves ONLY Bearer-token-authenticated syncapi callers, never a browser
// session, and those callers fetch these URLs as cross-origin <img>
// requests that cannot carry a Chronicle session cookie. See
// media.URLSigner.Verify's doc comment for why a single sentinel key
// suffices here.
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
		resp.URL = h.signer.Sign(file.ID, media.ViewerAPIKey, media.SignedURLTTL)
		resp.Thumbnails = make(map[string]string)
		for size := range file.ThumbnailPaths {
			resp.Thumbnails[size] = h.signer.SignThumb(file.ID, size, media.ViewerAPIKey, media.SignedURLTTL)
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
//
// Gated at Scribe+ (in addition to the route's RequirePermission(PermRead)):
// this endpoint has no entity-visibility filter to apply — media_files
// carries no reference to the entity it illustrates, so there is no cheap
// "only what this caller can see" query. Below Scribe it returns an empty
// page rather than every row's id, filename and a signed URL. This mirrors
// the threshold the web app already uses for bulk campaign media access
// (media/routes.go: the picker list and campaign media browser are both
// Scribe+/Owner).
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

	if h.resolveRole(c) < int(campaigns.RoleScribe) {
		return c.JSON(http.StatusOK, map[string]any{
			"data":     []apiMediaFileResponse{},
			"total":    0,
			"page":     page,
			"per_page": perPage,
		})
	}

	files, total, err := h.mediaSvc.ListCampaignMedia(ctx, campaignID, page, perPage)
	if err != nil {
		slog.Error("api: failed to list media", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list media"))
	}

	data := make([]apiMediaFileResponse, 0, len(files))
	for i := range files {
		data = append(data, h.response(c, &files[i]))
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
		return apperror.NewNotFound("media file not found")
	}

	// IDOR protection: verify file belongs to this campaign.
	if file.CampaignID == nil || *file.CampaignID != c.Param("id") {
		return apperror.NewNotFound("media file not found")
	}

	return c.JSON(http.StatusOK, h.response(c, file))
}

// GetMediaStats returns aggregate storage stats for the campaign.
// GET /api/v1/campaigns/:id/media/stats
func (h *MediaAPIHandler) GetMediaStats(c echo.Context) error {
	campaignID := c.Param("id")
	ctx := c.Request().Context()

	stats, err := h.mediaSvc.GetCampaignStats(ctx, campaignID)
	if err != nil {
		slog.Error("api: failed to get media stats", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to get media stats"))
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
		return apperror.NewUnauthorized("api key required")
	}

	campaignID := c.Param("id")

	file, err := c.FormFile("file")
	if err != nil {
		return apperror.NewBadRequest("no file provided")
	}

	src, err := file.Open()
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("failed to read file"))
	}
	defer func() { _ = src.Close() }()

	fileBytes, err := io.ReadAll(src)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("failed to read file"))
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
		return err
	}

	return c.JSON(http.StatusCreated, h.response(c, mediaFile))
}

// DeleteMedia deletes a media file from the campaign.
// DELETE /api/v1/campaigns/:id/media/:mediaID
//
// Owner-gated to match the web route (media/routes.go: DELETE
// /campaigns/:id/media/:mid requires RoleOwner — "The media browser
// (browse/delete) is Owner-only" per that file's doc comment).
// RequirePermission(PermWrite) alone also admits a Scribe, whether via a
// Foundry key or the Scribe's own session cookie (this route group accepts
// both), which the web UI refuses.
func (h *MediaAPIHandler) DeleteMedia(c echo.Context) error {
	mediaID := c.Param("mediaID")
	campaignID := c.Param("id")
	ctx := c.Request().Context()
	role := h.resolveRole(c)

	// Verify file belongs to this campaign.
	file, err := h.mediaSvc.GetByID(ctx, mediaID)
	if err != nil {
		return apperror.NewNotFound("media file not found")
	}
	if file.CampaignID == nil || *file.CampaignID != campaignID {
		return apperror.NewNotFound("media file not found")
	}

	// Only campaign owners can delete media.
	if role < int(campaigns.RoleOwner) {
		return apperror.NewForbidden("only campaign owners can delete media")
	}

	if err := h.mediaSvc.Delete(ctx, mediaID); err != nil {
		slog.Error("api: failed to delete media", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to delete media"))
	}

	return c.NoContent(http.StatusNoContent)
}
