// Package admin provides site-wide administration functionality.
// Admin routes require the site admin flag (users.is_admin) and provide
// user management, campaign oversight, and SMTP configuration access.
package admin

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/modules"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/settings"
	"github.com/keyxmakerx/chronicle/internal/plugins/smtp"
)

// AddonCounter provides a count of registered addons for the admin dashboard.
type AddonCounter interface {
	CountAddons(ctx context.Context) (int, error)
}

// Handler handles admin dashboard HTTP requests. Depends on other plugins'
// services via interfaces -- no direct repo access.
type Handler struct {
	authRepo        auth.UserRepository
	campaignService campaigns.CampaignService
	smtpService     smtp.SMTPService
	mediaRepo       media.MediaRepository
	mediaService    media.MediaService
	maxUploadSize   int64
	settingsService settings.SettingsService
	addonCounter    AddonCounter
	securityService SecurityService
}

// StoragePageData holds all data needed for the combined storage management page.
type StoragePageData struct {
	Stats          *media.StorageStats
	Files          []media.AdminMediaFile
	TotalFiles     int
	Page           int
	PerPage        int
	MaxUploadSize  int64
	Global         *settings.GlobalStorageLimits
	UserLimits     []settings.UserStorageLimitWithName
	CampaignLimits []settings.CampaignStorageLimitWithName
	Users          []auth.User
	Campaigns      []campaigns.Campaign
	CSRFToken      string
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

// SetSettingsDeps sets the settings service for the combined storage page.
func (h *Handler) SetSettingsDeps(svc settings.SettingsService) {
	h.settingsService = svc
}

// SetAddonCounter sets the addon counter for the dashboard extension count.
func (h *Handler) SetAddonCounter(counter AddonCounter) {
	h.addonCounter = counter
}

// SetSecurityService wires the security service for the security dashboard.
func (h *Handler) SetSecurityService(svc SecurityService) {
	h.securityService = svc
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

	var addonCount int
	if h.addonCounter != nil {
		addonCount, _ = h.addonCounter.CountAddons(ctx)
	}

	var securityStats *SecurityStats
	if h.securityService != nil {
		securityStats, _ = h.securityService.GetStats(ctx)
	}

	return middleware.Render(c, http.StatusOK, AdminDashboardPage(userCount, campaignCount, mediaFileCount, totalStorageBytes, smtpConfigured, addonCount, securityStats))
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

	// Invalidate all sessions for the target user so the privilege change
	// takes effect immediately. Without this, a revoked admin retains stale
	// IsAdmin=true in their Redis session until it expires.
	if h.securityService != nil {
		if count, err := h.securityService.ForceLogoutUser(c.Request().Context(), targetID); err == nil && count > 0 {
			slog.Info("invalidated sessions after admin toggle",
				slog.String("target_user", targetID),
				slog.Int("session_count", count),
			)
		}
	}

	slog.Info("admin toggled",
		slog.String("target_user", targetID),
		slog.Bool("new_state", newState),
		slog.String("by", currentUserID),
	)

	// Log the privilege change as a security event.
	if h.securityService != nil {
		action := "granted"
		if !newState {
			action = "revoked"
		}
		_ = h.securityService.LogEvent(c.Request().Context(), EventAdminPrivilegeChanged,
			targetID, currentUserID, c.RealIP(), c.Request().UserAgent(),
			map[string]any{"action": action, "target_name": user.DisplayName})
	}

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

// Storage renders the combined storage management page (GET /admin/storage).
// Loads storage stats, files, global settings, overrides, and user/campaign
// lists for the dropdown selectors.
func (h *Handler) Storage(c echo.Context) error {
	if h.mediaRepo == nil {
		return apperror.NewMissingContext()
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

	// Load settings data for the combined page.
	var global *settings.GlobalStorageLimits
	var userLimits []settings.UserStorageLimitWithName
	var campaignLimits []settings.CampaignStorageLimitWithName
	if h.settingsService != nil {
		global, _ = h.settingsService.GetStorageLimits(ctx)
		userLimits, _ = h.settingsService.ListUserLimits(ctx)
		campaignLimits, _ = h.settingsService.ListCampaignLimits(ctx)
	}

	// Load users and campaigns for override dropdowns.
	allUsers, _, _ := h.authRepo.ListUsers(ctx, 0, 1000)
	allCampaigns, _, _ := h.campaignService.ListAll(ctx, campaigns.ListOptions{Page: 1, PerPage: 1000})

	csrfToken := middleware.GetCSRFToken(c)
	data := StoragePageData{
		Stats:          stats,
		Files:          files,
		TotalFiles:     total,
		Page:           page,
		PerPage:        perPage,
		MaxUploadSize:  h.maxUploadSize,
		Global:         global,
		UserLimits:     userLimits,
		CampaignLimits: campaignLimits,
		Users:          allUsers,
		Campaigns:      allCampaigns,
		CSRFToken:      csrfToken,
	}
	return middleware.Render(c, http.StatusOK, AdminStoragePage(data))
}

// DeleteMedia deletes a media file (DELETE /admin/media/:fileID).
func (h *Handler) DeleteMedia(c echo.Context) error {
	if h.mediaService == nil {
		return apperror.NewMissingContext()
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

// --- Modules ---

// Modules renders the module management page (GET /admin/modules).
// Lists all registered game-system modules with their status.
func (h *Handler) Modules(c echo.Context) error {
	mods := modules.Registry()
	return middleware.Render(c, http.StatusOK, AdminModulesPage(mods))
}

// --- Plugins ---

// Plugins renders the plugin management page (GET /admin/plugins).
// Lists all registered plugins with their status and category.
func (h *Handler) Plugins(c echo.Context) error {
	return middleware.Render(c, http.StatusOK, AdminPluginsPage(PluginRegistry()))
}

// --- Security ---

// Security renders the security dashboard page (GET /admin/security).
func (h *Handler) Security(c echo.Context) error {
	if h.securityService == nil {
		return apperror.NewMissingContext()
	}

	ctx := c.Request().Context()

	stats, _ := h.securityService.GetStats(ctx)

	// Load recent security events (first page).
	eventType := c.QueryParam("type")
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	events, totalEvents, _ := h.securityService.ListEvents(ctx, eventType, page)

	// Load active sessions.
	sessions, _ := h.securityService.GetActiveSessions(ctx)

	csrfToken := middleware.GetCSRFToken(c)

	data := SecurityPageData{
		Stats:       stats,
		Events:      events,
		TotalEvents: totalEvents,
		EventFilter: eventType,
		Page:        page,
		PerPage:     securityPerPage,
		Sessions:    sessions,
		CSRFToken:   csrfToken,
	}

	return middleware.Render(c, http.StatusOK, AdminSecurityPage(data))
}

// TerminateSession destroys a specific session (DELETE /admin/security/sessions/:token).
func (h *Handler) TerminateSession(c echo.Context) error {
	if h.securityService == nil {
		return apperror.NewMissingContext()
	}

	token := c.Param("token")
	currentUserID := auth.GetUserID(c)

	if err := h.securityService.TerminateSession(c.Request().Context(), token); err != nil {
		return err
	}

	_ = h.securityService.LogEvent(c.Request().Context(), EventSessionTerminated,
		"", currentUserID, c.RealIP(), c.Request().UserAgent(),
		map[string]any{"token_hint": token[:8]})

	slog.Info("admin terminated session",
		slog.String("by", currentUserID),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/security")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/security")
}

// ForceLogoutUser destroys all sessions for a user (POST /admin/security/users/:id/force-logout).
func (h *Handler) ForceLogoutUser(c echo.Context) error {
	if h.securityService == nil {
		return apperror.NewMissingContext()
	}

	targetID := c.Param("id")
	currentUserID := auth.GetUserID(c)

	count, err := h.securityService.ForceLogoutUser(c.Request().Context(), targetID)
	if err != nil {
		return err
	}

	_ = h.securityService.LogEvent(c.Request().Context(), EventForceLogout,
		targetID, currentUserID, c.RealIP(), c.Request().UserAgent(),
		map[string]any{"sessions_destroyed": count})

	slog.Info("admin force-logged out user",
		slog.String("target_user", targetID),
		slog.Int("sessions_destroyed", count),
		slog.String("by", currentUserID),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/security")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/security")
}

// DisableUser disables a user account (PUT /admin/security/users/:id/disable).
func (h *Handler) DisableUser(c echo.Context) error {
	if h.securityService == nil {
		return apperror.NewMissingContext()
	}

	targetID := c.Param("id")
	currentUserID := auth.GetUserID(c)

	// Prevent admins from disabling themselves.
	if targetID == currentUserID {
		return apperror.NewBadRequest("cannot disable your own account")
	}

	if err := h.securityService.DisableUser(c.Request().Context(), targetID); err != nil {
		return err
	}

	_ = h.securityService.LogEvent(c.Request().Context(), EventUserDisabled,
		targetID, currentUserID, c.RealIP(), c.Request().UserAgent(), nil)

	slog.Info("admin disabled user",
		slog.String("target_user", targetID),
		slog.String("by", currentUserID),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/security")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/security")
}

// EnableUser re-enables a disabled user account (PUT /admin/security/users/:id/enable).
func (h *Handler) EnableUser(c echo.Context) error {
	if h.securityService == nil {
		return apperror.NewMissingContext()
	}

	targetID := c.Param("id")
	currentUserID := auth.GetUserID(c)

	if err := h.securityService.EnableUser(c.Request().Context(), targetID); err != nil {
		return err
	}

	_ = h.securityService.LogEvent(c.Request().Context(), EventUserEnabled,
		targetID, currentUserID, c.RealIP(), c.Request().UserAgent(), nil)

	slog.Info("admin enabled user",
		slog.String("target_user", targetID),
		slog.String("by", currentUserID),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/security")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/security")
}

// SecurityPageData holds all data needed for the security dashboard page.
type SecurityPageData struct {
	Stats       *SecurityStats
	Events      []SecurityEvent
	TotalEvents int
	EventFilter string
	Page        int
	PerPage     int
	Sessions    []auth.SessionInfo
	CSRFToken   string
}
