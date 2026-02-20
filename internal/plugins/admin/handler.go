// Package admin provides site-wide administration functionality.
// Admin routes require the site admin flag (users.is_admin) and provide
// user management, campaign oversight, and SMTP configuration access.
package admin

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/smtp"
)

// Handler handles admin dashboard HTTP requests. Depends on other plugins'
// services via interfaces -- no direct repo access.
type Handler struct {
	authRepo        auth.UserRepository
	campaignService campaigns.CampaignService
	smtpService     smtp.SMTPService
	mediaRepo       media.MediaRepository
	mediaService    media.MediaService
	maxUploadSize   int64
}

// NewHandler creates a new admin handler.
func NewHandler(authRepo auth.UserRepository, campaignService campaigns.CampaignService, smtpService smtp.SMTPService) *Handler {
	return &Handler{
		authRepo:        authRepo,
		campaignService: campaignService,
		smtpService:     smtpService,
	}
}

// SetMediaDeps sets the media dependencies for the storage admin page.
// Called after media plugin is wired to avoid constructor bloat.
func (h *Handler) SetMediaDeps(repo media.MediaRepository, svc media.MediaService, maxUploadSize int64) {
	h.mediaRepo = repo
	h.mediaService = svc
	h.maxUploadSize = maxUploadSize
}

// --- Dashboard ---

// Dashboard renders the admin overview page (GET /admin).
func (h *Handler) Dashboard(c echo.Context) error {
	ctx := c.Request().Context()

	userCount, _ := h.authRepo.CountUsers(ctx)
	campaignCount, _ := h.campaignService.CountAll(ctx)

	var smtpConfigured bool
	if h.smtpService != nil {
		smtpConfigured = h.smtpService.IsConfigured(ctx)
	}

	var mediaFileCount int
	var totalStorageBytes int64
	if h.mediaRepo != nil {
		if stats, err := h.mediaRepo.GetStorageStats(ctx); err == nil {
			mediaFileCount = stats.TotalFiles
			totalStorageBytes = stats.TotalBytes
		}
	}

	return middleware.Render(c, http.StatusOK, AdminDashboardPage(userCount, campaignCount, mediaFileCount, totalStorageBytes, smtpConfigured))
}

// --- Users ---

// Users renders the user management page (GET /admin/users).
func (h *Handler) Users(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage := 25
	offset := (page - 1) * perPage

	users, total, err := h.authRepo.ListUsers(c.Request().Context(), offset, perPage)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, AdminUsersPage(users, total, page, perPage, csrfToken))
}

// ToggleAdmin toggles a user's is_admin flag (PUT /admin/users/:id/admin).
func (h *Handler) ToggleAdmin(c echo.Context) error {
	targetID := c.Param("id")

	// Prevent admins from removing their own admin status.
	currentUserID := auth.GetUserID(c)
	if targetID == currentUserID {
		return apperror.NewBadRequest("cannot change your own admin status")
	}

	// Get current state to toggle.
	user, err := h.authRepo.FindByID(c.Request().Context(), targetID)
	if err != nil {
		return err
	}

	newState := !user.IsAdmin

	// Prevent removing the last admin, which would lock out all admin access.
	if !newState {
		adminCount, err := h.authRepo.CountAdmins(c.Request().Context())
		if err != nil {
			return err
		}
		if adminCount <= 1 {
			return apperror.NewBadRequest("cannot remove the last admin")
		}
	}

	if err := h.authRepo.UpdateIsAdmin(c.Request().Context(), targetID, newState); err != nil {
		return err
	}

	slog.Info("admin toggled",
		slog.String("target_user", targetID),
		slog.Bool("new_state", newState),
		slog.String("by", currentUserID),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/users")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/users")
}

// --- Campaigns ---

// Campaigns renders the campaign management page (GET /admin/campaigns).
func (h *Handler) Campaigns(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	opts := campaigns.ListOptions{Page: page, PerPage: 25}
	allCampaigns, total, err := h.campaignService.ListAll(c.Request().Context(), opts)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, AdminCampaignsPage(allCampaigns, total, page, opts.PerPage, csrfToken))
}

// DeleteCampaign force-deletes a campaign (DELETE /admin/campaigns/:id).
func (h *Handler) DeleteCampaign(c echo.Context) error {
	campaignID := c.Param("id")

	if err := h.campaignService.Delete(c.Request().Context(), campaignID); err != nil {
		return err
	}

	slog.Info("admin deleted campaign",
		slog.String("campaign_id", campaignID),
		slog.String("by", auth.GetUserID(c)),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/campaigns")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/campaigns")
}

// JoinCampaign adds the admin to a campaign with the selected role
// (POST /admin/campaigns/:id/join).
func (h *Handler) JoinCampaign(c echo.Context) error {
	campaignID := c.Param("id")
	userID := auth.GetUserID(c)

	roleStr := c.FormValue("role")
	role := campaigns.RoleFromString(roleStr)
	if !role.IsValid() {
		return apperror.NewBadRequest("invalid role")
	}

	// Use AdminAddMember which handles Owner conflict (force-transfer).
	if err := h.campaignService.AdminAddMember(c.Request().Context(), campaignID, userID, role); err != nil {
		return err
	}

	slog.Info("admin joined campaign",
		slog.String("campaign_id", campaignID),
		slog.String("user_id", userID),
		slog.String("role", roleStr),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/campaigns")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/campaigns")
}

// LeaveCampaign removes the admin from a campaign (DELETE /admin/campaigns/:id/leave).
func (h *Handler) LeaveCampaign(c echo.Context) error {
	campaignID := c.Param("id")
	userID := auth.GetUserID(c)

	if err := h.campaignService.RemoveMember(c.Request().Context(), campaignID, userID); err != nil {
		return err
	}

	slog.Info("admin left campaign",
		slog.String("campaign_id", campaignID),
		slog.String("user_id", userID),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/campaigns")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/campaigns")
}

// --- Storage ---

// Storage renders the storage management page (GET /admin/storage).
func (h *Handler) Storage(c echo.Context) error {
	if h.mediaRepo == nil {
		return apperror.NewInternal(nil)
	}

	ctx := c.Request().Context()

	stats, err := h.mediaRepo.GetStorageStats(ctx)
	if err != nil {
		return err
	}

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage := 25
	offset := (page - 1) * perPage

	files, total, err := h.mediaRepo.ListAll(ctx, perPage, offset)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, AdminStoragePage(stats, files, total, page, perPage, h.maxUploadSize, csrfToken))
}

// DeleteMedia deletes a media file (DELETE /admin/media/:fileID).
func (h *Handler) DeleteMedia(c echo.Context) error {
	if h.mediaService == nil {
		return apperror.NewInternal(nil)
	}

	fileID := c.Param("fileID")

	if err := h.mediaService.Delete(c.Request().Context(), fileID); err != nil {
		return err
	}

	slog.Info("admin deleted media file",
		slog.String("file_id", fileID),
		slog.String("by", auth.GetUserID(c)),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/storage")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/storage")
}
