// Package admin provides site-wide administration functionality.
// Admin routes require the site admin flag (users.is_admin) and provide
// user management, campaign oversight, and SMTP configuration access.
package admin

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/settings"
	"github.com/keyxmakerx/chronicle/internal/plugins/smtp"
)

// AddonCounter provides a count of registered addons for the admin dashboard.
// CountFeatures excludes module-category addons (game systems) which are
// shown on the Content Packs page instead.
type AddonCounter interface {
	CountAddons(ctx context.Context) (int, error)
	CountFeatures(ctx context.Context) (int, error)
}

// PendingCounter provides a count of pending package submissions for the dashboard.
type PendingCounter interface {
	CountPendingSubmissions(ctx context.Context) (int, error)
}

// Handler handles admin dashboard HTTP requests. Depends on other plugins'
// services via interfaces -- no direct repo access.
type Handler struct {
	authRepo         auth.UserRepository
	campaignService  campaigns.CampaignService
	smtpService      smtp.SMTPService
	mediaRepo        media.MediaRepository
	mediaService     media.MediaService
	maxUploadSize    int64
	settingsService  settings.SettingsService
	addonCounter     AddonCounter
	securityService  SecurityService
	hygieneScanner   DataHygieneScanner
	databaseExplorer DatabaseExplorer
	pendingCounter   PendingCounter
	foundryDir       func() string // Resolves the active Foundry module directory.
	baseURL          string
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

// SetHygieneScanner wires the data hygiene scanner for the hygiene dashboard.
func (h *Handler) SetHygieneScanner(scanner DataHygieneScanner) {
	h.hygieneScanner = scanner
}

// SetDatabaseExplorer wires the database explorer for the schema visualization page.
func (h *Handler) SetDatabaseExplorer(explorer DatabaseExplorer) {
	h.databaseExplorer = explorer
}

// SetBaseURL sets the public-facing base URL for the Foundry module admin page.
func (h *Handler) SetBaseURL(url string) {
	h.baseURL = url
}

// SetPendingCounter wires the pending submission counter for the dashboard.
func (h *Handler) SetPendingCounter(counter PendingCounter) {
	h.pendingCounter = counter
}

// SetFoundryDir sets the resolver for the active Foundry module directory.
// This allows the admin page to read/write the correct module.json whether
// served from the package manager install or the bundled fallback.
// The resolver should be app.foundryModuleDir — the same function the
// serving layer uses — so both paths are always in sync.
func (h *Handler) SetFoundryDir(resolver func() string) {
	h.foundryDir = resolver
}

// resolveFoundryDir returns the active Foundry module directory, preferring
// the package-manager install path and falling back to the bundled directory.
func (h *Handler) resolveFoundryDir() string {
	if h.foundryDir != nil {
		if dir := h.foundryDir(); dir != "" {
			slog.Debug("resolveFoundryDir: using resolved path",
				slog.String("path", dir),
			)
			return dir
		}
		slog.Warn("resolveFoundryDir: resolver returned empty, falling back to bundled")
	} else {
		slog.Warn("resolveFoundryDir: no foundry dir resolver set, falling back to bundled")
	}
	return "foundry-module"
}

// --- Data Hygiene ---

// DataHygiene renders the data hygiene dashboard (GET /admin/data-hygiene).
func (h *Handler) DataHygiene(c echo.Context) error {
	ctx := c.Request().Context()

	data := DataHygieneData{
		CSRFToken: middleware.GetCSRFToken(c),
	}

	if h.hygieneScanner != nil {
		stats, err := h.hygieneScanner.GetDiskUsageStats(ctx)
		if err != nil {
			slog.Warn("failed to get hygiene stats", slog.Any("error", err))
		}
		data.Stats = stats

		orphanedMedia, err := h.hygieneScanner.ScanOrphanedMedia(ctx)
		if err != nil {
			slog.Warn("failed to scan orphaned media", slog.Any("error", err))
		}
		data.OrphanedMedia = orphanedMedia

		orphanedKeys, err := h.hygieneScanner.ScanOrphanedAPIKeys(ctx)
		if err != nil {
			slog.Warn("failed to scan orphaned API keys", slog.Any("error", err))
		}
		data.OrphanedAPIKeys = orphanedKeys

		staleFiles, err := h.hygieneScanner.ScanStaleFiles(ctx)
		if err != nil {
			slog.Warn("failed to scan stale files", slog.Any("error", err))
		}
		data.StaleFiles = staleFiles
	}

	return middleware.Render(c, http.StatusOK, DataHygienePage(data))
}

// PurgeOrphanedMediaAPI handles DELETE /admin/data-hygiene/orphaned-media.
func (h *Handler) PurgeOrphanedMediaAPI(c echo.Context) error {
	if h.hygieneScanner == nil {
		return apperror.NewInternal(fmt.Errorf("hygiene scanner not configured"))
	}
	purged, err := h.hygieneScanner.PurgeOrphanedMedia(c.Request().Context())
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("purging orphaned media: %w", err))
	}
	slog.Info("admin purged orphaned media", slog.Int("purged", purged))
	return c.Redirect(http.StatusSeeOther, "/admin/data-hygiene")
}

// PurgeOrphanedAPIKeysAPI handles DELETE /admin/data-hygiene/orphaned-api-keys.
func (h *Handler) PurgeOrphanedAPIKeysAPI(c echo.Context) error {
	if h.hygieneScanner == nil {
		return apperror.NewInternal(fmt.Errorf("hygiene scanner not configured"))
	}
	purged, err := h.hygieneScanner.PurgeOrphanedAPIKeys(c.Request().Context())
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("purging orphaned API keys: %w", err))
	}
	slog.Info("admin purged orphaned API keys", slog.Int("purged", purged))
	return c.Redirect(http.StatusSeeOther, "/admin/data-hygiene")
}

// PurgeStaleFilesAPI handles DELETE /admin/data-hygiene/stale-files.
func (h *Handler) PurgeStaleFilesAPI(c echo.Context) error {
	if h.hygieneScanner == nil {
		return apperror.NewInternal(fmt.Errorf("hygiene scanner not configured"))
	}
	purged, err := h.hygieneScanner.PurgeStaleFiles(c.Request().Context())
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("purging stale files: %w", err))
	}
	slog.Info("admin purged stale files", slog.Int("purged", purged))
	return c.Redirect(http.StatusSeeOther, "/admin/data-hygiene")
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
		addonCount, _ = h.addonCounter.CountFeatures(ctx)
	}

	var securityStats *SecurityStats
	if h.securityService != nil {
		securityStats, _ = h.securityService.GetStats(ctx)
	}

	var pendingSubmissions int
	if h.pendingCounter != nil {
		pendingSubmissions, _ = h.pendingCounter.CountPendingSubmissions(ctx)
	}

	// Check for degraded plugins to show alert banner.
	var degradedPlugins []string
	if h.databaseExplorer != nil {
		statuses, _ := h.databaseExplorer.GetMigrationStatus(ctx)
		for _, s := range statuses {
			if !s.Healthy || s.Pending > 0 {
				degradedPlugins = append(degradedPlugins, s.Slug)
			}
		}
	}

	return middleware.Render(c, http.StatusOK, AdminDashboardPage(userCount, campaignCount, mediaFileCount, totalStorageBytes, smtpConfigured, addonCount, securityStats, degradedPlugins, pendingSubmissions))
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

// TerminateSession destroys a specific session by its token hash
// (DELETE /admin/security/sessions/:hash). Uses hash-based lookup to avoid
// exposing raw session tokens in admin HTML.
func (h *Handler) TerminateSession(c echo.Context) error {
	if h.securityService == nil {
		return apperror.NewMissingContext()
	}

	tokenHash := c.Param("hash")
	currentUserID := auth.GetUserID(c)

	if err := h.securityService.TerminateSessionByHash(c.Request().Context(), tokenHash); err != nil {
		return err
	}

	_ = h.securityService.LogEvent(c.Request().Context(), EventSessionTerminated,
		"", currentUserID, c.RealIP(), c.Request().UserAgent(),
		map[string]any{"token_hash": tokenHash})

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

// --- Database Explorer ---

// Database renders the admin database explorer page (GET /admin/database).
func (h *Handler) Database(c echo.Context) error {
	ctx := c.Request().Context()

	var statuses []PluginMigrationStatus
	var tableCount int
	if h.databaseExplorer != nil {
		var err error
		statuses, err = h.databaseExplorer.GetMigrationStatus(ctx)
		if err != nil {
			slog.Warn("failed to get migration status", slog.Any("error", err))
		}

		// Quick table count for the page header.
		schema, err := h.databaseExplorer.GetSchema(ctx)
		if err != nil {
			slog.Warn("failed to get schema for table count", slog.Any("error", err))
		} else {
			tableCount = len(schema.Tables)
		}
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, AdminDatabasePage(statuses, tableCount, csrfToken))
}

// DatabaseSchemaAPI returns the full schema as JSON (GET /admin/database/schema).
// The D3 widget fetches this to render the interactive diagram.
func (h *Handler) DatabaseSchemaAPI(c echo.Context) error {
	if h.databaseExplorer == nil {
		return apperror.NewInternal(fmt.Errorf("database explorer not configured"))
	}

	schema, err := h.databaseExplorer.GetSchema(c.Request().Context())
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("getting schema: %w", err))
	}

	return c.JSON(http.StatusOK, schema)
}

// ApplyMigrationsAPI runs all pending plugin migrations (POST /admin/database/migrations/apply).
func (h *Handler) ApplyMigrationsAPI(c echo.Context) error {
	if h.databaseExplorer == nil {
		return apperror.NewInternal(fmt.Errorf("database explorer not configured"))
	}

	results, err := h.databaseExplorer.ApplyPendingMigrations(c.Request().Context())
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("applying migrations: %w", err))
	}

	for _, r := range results {
		if r.Healthy {
			slog.Info("migration applied via admin",
				slog.String("plugin", r.Slug),
				slog.Int("version", r.Version),
			)
		} else {
			slog.Error("migration failed via admin",
				slog.String("plugin", r.Slug),
				slog.Any("error", r.Error),
			)
		}
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/database")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/database")
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

// --- Foundry VTT Module Management ---

// FoundryModuleData holds data for the admin Foundry module page.
type FoundryModuleData struct {
	Version    string
	InstallURL string
	CSRFToken  string
	Files      []FoundryModuleFile
	SourceDir  string // Resolved directory the module is served from.
}

// FoundryModuleFile represents a file in the foundry-module directory.
type FoundryModuleFile struct {
	Path string
	Size int64
}

// FoundryModule renders the Foundry VTT module management page (GET /admin/foundry).
func (h *Handler) FoundryModule(c echo.Context) error {
	moduleDir := h.resolveFoundryDir()
	version := h.readFoundryModuleVersion()
	baseURL := strings.TrimRight(h.baseURL, "/")
	data := FoundryModuleData{
		Version:    version,
		InstallURL: baseURL + "/foundry-module/module.json",
		CSRFToken:  middleware.GetCSRFToken(c),
		Files:      h.listFoundryModuleFiles(),
		SourceDir:  moduleDir,
	}
	return middleware.Render(c, http.StatusOK, AdminFoundryModulePage(data))
}

// UpdateFoundryModuleVersion updates the version in foundry-module/module.json
// (PUT /admin/foundry/version).
func (h *Handler) UpdateFoundryModuleVersion(c echo.Context) error {
	var req struct {
		Version string `json:"version" form:"version"`
	}
	if err := c.Bind(&req); err != nil || req.Version == "" {
		return apperror.NewBadRequest("version is required")
	}

	// Read current module.json from the active Foundry directory.
	moduleDir := h.resolveFoundryDir()
	manifestPath := filepath.Join(moduleDir, "module.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("read module.json: %w", err))
	}

	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return apperror.NewInternal(fmt.Errorf("parse module.json: %w", err))
	}

	manifest["version"] = req.Version

	// Update the download URL to use the new version tag.
	baseURL := strings.TrimRight(h.baseURL, "/")
	manifest["download"] = baseURL + "/foundry-module/chronicle-sync.zip"
	manifest["manifest"] = baseURL + "/foundry-module/module.json"

	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshal module.json: %w", err))
	}
	out = append(out, '\n')

	if err := os.WriteFile(manifestPath, out, 0644); err != nil {
		return apperror.NewInternal(fmt.Errorf("write module.json: %w", err))
	}

	slog.Info("foundry module version updated",
		slog.String("version", req.Version),
		slog.String("by", auth.GetUserID(c)),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/foundry")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/foundry")
}

// readFoundryModuleVersion reads the version from the active Foundry module's module.json.
func (h *Handler) readFoundryModuleVersion() string {
	data, err := os.ReadFile(filepath.Join(h.resolveFoundryDir(), "module.json"))
	if err != nil {
		return "unknown"
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "unknown"
	}
	return manifest.Version
}

// listFoundryModuleFiles returns all distributable files in the active Foundry module directory.
func (h *Handler) listFoundryModuleFiles() []FoundryModuleFile {
	moduleDir := h.resolveFoundryDir()
	var files []FoundryModuleFile
	_ = filepath.WalkDir(moduleDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		if name == ".ai.md" || name == "TESTING.md" {
			return nil
		}
		rel, _ := filepath.Rel(moduleDir, path)
		info, _ := d.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		files = append(files, FoundryModuleFile{Path: rel, Size: size})
		return nil
	})
	return files
}

// UploadFoundryModule handles zip upload of a new Foundry module version
// (POST /admin/foundry/upload). Extracts the zip into foundry-module/,
// replacing existing files. Preserves manifest/download URLs by rewriting
// them to point at this Chronicle instance.
func (h *Handler) UploadFoundryModule(c echo.Context) error {
	file, err := c.FormFile("module_zip")
	if err != nil {
		return apperror.NewBadRequest("module zip file is required")
	}

	// 10 MB limit for the module zip.
	const maxZipSize = 10 << 20
	if file.Size > maxZipSize {
		return apperror.NewBadRequest("module zip must be under 10 MB")
	}

	src, err := file.Open()
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("open uploaded file: %w", err))
	}
	defer func() { _ = src.Close() }()

	// Read entire zip into memory for zip.NewReader (needs ReadSeeker).
	zipBytes, err := io.ReadAll(io.LimitReader(src, maxZipSize+1))
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("read uploaded file: %w", err))
	}
	if int64(len(zipBytes)) > maxZipSize {
		return apperror.NewBadRequest("module zip must be under 10 MB")
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return apperror.NewBadRequest("uploaded file is not a valid zip archive")
	}

	// Validate: zip must contain a module.json somewhere.
	hasManifest := false
	for _, f := range zr.File {
		if filepath.Base(f.Name) == "module.json" && !f.FileInfo().IsDir() {
			hasManifest = true
			break
		}
	}
	if !hasManifest {
		return apperror.NewBadRequest("zip must contain a module.json file")
	}

	// Determine if files are nested under a single root directory (common in
	// Foundry module zips, e.g. chronicle-sync/module.json). If so, strip
	// that prefix when extracting.
	stripPrefix := detectZipRootDir(zr)

	// Extract files into the active Foundry module directory.
	destDir := h.resolveFoundryDir()
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)

		// Strip the common root directory if present.
		if stripPrefix != "" {
			name = strings.TrimPrefix(name, stripPrefix)
			if name == "" {
				continue // Skip the root dir entry itself.
			}
		}

		// Security: reject paths that escape the destination directory.
		if strings.Contains(name, "..") {
			continue
		}

		// Skip non-distributable files and hidden files.
		base := filepath.Base(name)
		if base == ".ai.md" || base == "TESTING.md" || strings.HasPrefix(base, ".") {
			continue
		}

		destPath := filepath.Join(destDir, filepath.FromSlash(name))

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return apperror.NewInternal(fmt.Errorf("create dir %s: %w", name, err))
			}
			continue
		}

		// Ensure parent directory exists.
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return apperror.NewInternal(fmt.Errorf("create parent dir for %s: %w", name, err))
		}

		rc, err := f.Open()
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("open zip entry %s: %w", name, err))
		}

		data, err := io.ReadAll(io.LimitReader(rc, maxZipSize))
		_ = rc.Close()
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("read zip entry %s: %w", name, err))
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return apperror.NewInternal(fmt.Errorf("write %s: %w", name, err))
		}
	}

	// Rewrite manifest/download URLs in the extracted module.json to point
	// at this Chronicle instance.
	h.rewriteFoundryManifestURLs()

	slog.Info("foundry module uploaded",
		slog.String("filename", file.Filename),
		slog.Int64("size", file.Size),
		slog.String("by", auth.GetUserID(c)),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/foundry")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/foundry")
}

// detectZipRootDir checks if all zip entries share a single root directory.
// Returns the prefix to strip (e.g. "chronicle-sync/") or empty string.
func detectZipRootDir(zr *zip.Reader) string {
	if len(zr.File) == 0 {
		return ""
	}
	// Find the first path component of each entry.
	var root string
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		parts := strings.SplitN(name, "/", 2)
		if len(parts) < 2 {
			// File at zip root level — no common root dir.
			return ""
		}
		if root == "" {
			root = parts[0]
		} else if parts[0] != root {
			return ""
		}
	}
	return root + "/"
}

// RedeployFoundryModule bumps the patch version in module.json to trigger
// Foundry VTT clients to see an available update (POST /admin/foundry/redeploy).
func (h *Handler) RedeployFoundryModule(c echo.Context) error {
	manifestPath := filepath.Join(h.resolveFoundryDir(), "module.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("read module.json: %w", err))
	}

	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return apperror.NewInternal(fmt.Errorf("parse module.json: %w", err))
	}

	// Bump patch version: 0.1.0 -> 0.1.1, 0.1.1 -> 0.1.2, etc.
	version, _ := manifest["version"].(string)
	newVersion := bumpPatchVersion(version)
	manifest["version"] = newVersion

	// Ensure manifest/download URLs point at this instance.
	baseURL := strings.TrimRight(h.baseURL, "/")
	manifest["manifest"] = baseURL + "/foundry-module/module.json"
	manifest["download"] = baseURL + "/foundry-module/chronicle-sync.zip"

	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshal module.json: %w", err))
	}
	out = append(out, '\n')

	if err := os.WriteFile(manifestPath, out, 0644); err != nil {
		return apperror.NewInternal(fmt.Errorf("write module.json: %w", err))
	}

	slog.Info("foundry module redeployed",
		slog.String("old_version", version),
		slog.String("new_version", newVersion),
		slog.String("by", auth.GetUserID(c)),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/foundry")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/foundry")
}

// bumpPatchVersion increments the patch component of a semver string.
// "0.1.0" -> "0.1.1", "1.2.3" -> "1.2.4". Falls back to "0.0.1" on bad input.
func bumpPatchVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "0.0.1"
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return version + ".1"
	}
	return parts[0] + "." + parts[1] + "." + strconv.Itoa(patch+1)
}

// rewriteFoundryManifestURLs updates manifest and download URLs in
// the active Foundry module's module.json to point at this Chronicle instance.
func (h *Handler) rewriteFoundryManifestURLs() {
	manifestPath := filepath.Join(h.resolveFoundryDir(), "module.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return
	}
	baseURL := strings.TrimRight(h.baseURL, "/")
	manifest["manifest"] = baseURL + "/foundry-module/module.json"
	manifest["download"] = baseURL + "/foundry-module/chronicle-sync.zip"
	out, _ := json.MarshalIndent(manifest, "", "  ")
	out = append(out, '\n')
	_ = os.WriteFile(manifestPath, out, 0644)
}
