// Package admin provides site-wide administration functionality.
// Admin routes require the site admin flag (users.is_admin) and provide
// user management, campaign oversight, and SMTP configuration access.
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/settings"
	"github.com/keyxmakerx/chronicle/internal/plugins/smtp"
	"github.com/keyxmakerx/chronicle/internal/systems"
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

// AddonUsageCounter provides campaign usage counts for addon slugs.
// Used by the system diagnostics page to show how many campaigns use each system.
type AddonUsageCounter interface {
	CountCampaignsUsingAddon(ctx context.Context, addonSlug string) (int, error)
}

// Handler handles admin dashboard HTTP requests. Depends on other plugins'
// services via interfaces -- no direct repo access.
type Handler struct {
	authRepo        auth.UserRepository
	campaignService campaigns.CampaignService
	smtpService     smtp.SMTPService
	mediaRepo       media.MediaRepository
	mediaService    media.MediaService
	// resolveMaxUploadSize returns the live max-upload-size from the
	// settings service. Used by the storage admin page so the displayed
	// "Max upload" matches what /media/upload's body-limit actually
	// enforces — was previously the env-var value frozen at startup.
	resolveMaxUploadSize func() int64
	settingsService      settings.SettingsService
	addonCounter         AddonCounter
	securityService      SecurityService
	hygieneScanner       DataHygieneScanner
	databaseExplorer     DatabaseExplorer
	healthChecker        HealthChecker
	backupLister         BackupLister
	pendingCounter       PendingCounter
	addonUsageCounter    AddonUsageCounter
	apiAlertCounter      APIAlertCounter
	// activity is the admin change log; nil-safe so a missing log never blocks an admin action.
	activity ActivityService
	baseURL  string
	navPins  AdminNavPinService
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
	// LimitsTab selects the limits view (?tab=limits) instead of the file list.
	LimitsTab bool
	// TrashRetentionDays is how long deleted world pages stay in a Trash.
	TrashRetentionDays int
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
//
// resolveMaxUploadSize is a function so the storage page reflects the
// LIVE limit configured by the admin (matching what the body-limit
// middleware actually enforces) rather than the value frozen at startup.
// Reports surface the resolver's result; if nil, the page falls back to
// the legacy maxUploadSize field.
func (h *Handler) SetMediaDeps(repo media.MediaRepository, svc media.MediaService, resolveMaxUploadSize func() int64) {
	h.mediaRepo = repo
	h.mediaService = svc
	h.resolveMaxUploadSize = resolveMaxUploadSize
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

// SetHealthChecker injects the on-demand health-check runner for the Database >
// Health tab. Optional — when nil the Health tab shows "unavailable".
func (h *Handler) SetHealthChecker(checker HealthChecker) {
	h.healthChecker = checker
}

// SetBackupLister injects the backup/restore data source for the Database >
// Backups tab. Optional — when nil the Backups tab shows "unavailable".
func (h *Handler) SetBackupLister(lister BackupLister) {
	h.backupLister = lister
}

// SetBaseURL sets the public-facing base URL for the Foundry module admin page.
func (h *Handler) SetBaseURL(url string) {
	h.baseURL = url
}

// SetPendingCounter wires the pending submission counter for the dashboard.
func (h *Handler) SetPendingCounter(counter PendingCounter) {
	h.pendingCounter = counter
}

// SetAddonUsageCounter wires the addon usage counter for system diagnostics.
func (h *Handler) SetAddonUsageCounter(counter AddonUsageCounter) {
	h.addonUsageCounter = counter
}

// SetAPIAlertCounter wires the unresolved API alert count for the Home list.
func (h *Handler) SetAPIAlertCounter(counter APIAlertCounter) {
	h.apiAlertCounter = counter
}

// SetActivityService wires the admin change log.
func (h *Handler) SetActivityService(svc ActivityService) {
	h.activity = svc
}

// record logs an admin change. Called after the change succeeded; a missing or
// failing log never affects the response.
func (h *Handler) record(c echo.Context, action, targetType, targetID, label string) {
	if h.activity == nil {
		return
	}
	h.activity.RecordActivity(c.Request().Context(), auth.GetUserID(c), action, targetType, targetID, label)
}

// userLabel looks up a display name for the log; empty when the lookup fails,
// since the id is still recorded.
func (h *Handler) userLabel(ctx context.Context, userID string) string {
	u, err := h.authRepo.FindByID(ctx, userID)
	if err != nil || u == nil {
		return ""
	}
	return u.DisplayName
}

// campaignLabel looks up a campaign name for the log, before a delete removes it.
func (h *Handler) campaignLabel(ctx context.Context, campaignID string) string {
	cm, err := h.campaignService.GetByID(ctx, campaignID)
	if err != nil || cm == nil {
		return ""
	}
	return cm.Name
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
			data.ScanFailed = true
		}
		data.Stats = stats

		orphanedMedia, err := h.hygieneScanner.ScanOrphanedMedia(ctx)
		if err != nil {
			slog.Warn("failed to scan orphaned media", slog.Any("error", err))
			data.ScanFailed = true
		}
		data.OrphanedMedia = orphanedMedia

		orphanedKeys, err := h.hygieneScanner.ScanOrphanedAPIKeys(ctx)
		if err != nil {
			slog.Warn("failed to scan orphaned API keys", slog.Any("error", err))
			data.ScanFailed = true
		}
		data.OrphanedAPIKeys = orphanedKeys

		staleFiles, err := h.hygieneScanner.ScanStaleFiles(ctx)
		if err != nil {
			slog.Warn("failed to scan stale files", slog.Any("error", err))
			data.ScanFailed = true
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
	h.record(c, "hygiene.purged", "hygiene", "", fmt.Sprintf("%d orphaned files", purged))
	return middleware.HTMXRedirect(c, "/admin/data-hygiene")
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
	h.record(c, "hygiene.purged", "hygiene", "", fmt.Sprintf("%d orphaned API keys", purged))
	return middleware.HTMXRedirect(c, "/admin/data-hygiene")
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
	h.record(c, "hygiene.purged", "hygiene", "", fmt.Sprintf("%d stale files", purged))
	return middleware.HTMXRedirect(c, "/admin/data-hygiene")
}

// --- Dashboard ---

// Dashboard renders the admin overview page (GET /admin).
func (h *Handler) Dashboard(c echo.Context) error {
	ctx := c.Request().Context()

	// A failed count is -1 so the tile shows a dash instead of a false 0.
	userCount, err := h.authRepo.CountUsers(ctx)
	if err != nil {
		slog.Warn("admin dashboard: count users failed", slog.Any("error", err))
		userCount = -1
	}
	campaignCount, err := h.campaignService.CountAll(ctx)
	if err != nil {
		slog.Warn("admin dashboard: count campaigns failed", slog.Any("error", err))
		campaignCount = -1
	}

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

	// degradedPlugins feeds the Database tile (unhealthy or behind); the Needs
	// you list splits the two because they are fixed on different pages.
	var degradedPlugins []string
	var unhealthyPlugins, pendingMigrations int
	if h.databaseExplorer != nil {
		statuses, _ := h.databaseExplorer.GetMigrationStatus(ctx)
		for _, s := range statuses {
			if !s.Healthy || s.Pending > 0 {
				degradedPlugins = append(degradedPlugins, s.Slug)
			}
			if !s.Healthy {
				unhealthyPlugins++
			} else if s.Pending > 0 {
				pendingMigrations += s.Pending
			}
		}
	}

	var apiAlerts int
	if h.apiAlertCounter != nil {
		apiAlerts, _ = h.apiAlertCounter.CountUnresolvedAPIAlerts(ctx)
	}

	needs := buildNeedsYou(needsInput{
		UnhealthyPlugins:   unhealthyPlugins,
		PendingMigrations:  pendingMigrations,
		APIAlerts:          apiAlerts,
		PendingSubmissions: pendingSubmissions,
		SMTPConfigured:     smtpConfigured,
		SMTPKnown:          h.smtpService != nil,
	})

	var recent []ActivityEntry
	changesThisWeek := -1
	if h.activity != nil {
		var err error
		if recent, _, err = h.activity.List(ctx, ActivityFilter{}, 1, 10); err != nil {
			slog.Warn("failed to load admin activity", slog.Any("error", err))
		}
		weekly := ResolveActivityFilter(ActivityQuery{When: When7Days}, time.Now())
		if _, n, err := h.activity.List(ctx, weekly, 1, 1); err != nil {
			slog.Warn("admin dashboard: count recent changes failed", slog.Any("error", err))
		} else {
			changesThisWeek = n
		}
	}

	// System counts for the Packages and Health tiles.
	registeredSystems := len(systems.Registry())
	failedSystems := registeredSystems - len(systems.AllSystems())
	if failedSystems < 0 {
		failedSystems = 0
	}

	in := homeInput{
		Users: userCount, Campaigns: campaignCount, Features: addonCount,
		APIAlerts: -1, ChangesThisWeek: changesThisWeek,
		PendingSubmissions: pendingSubmissions,
		RegisteredSystems:  registeredSystems, FailedSystems: failedSystems,
		ActiveSessions: -1,
		MediaFiles:     mediaFileCount, StorageBytes: totalStorageBytes,
		DegradedParts:  len(degradedPlugins),
		SMTPConfigured: smtpConfigured, SMTPKnown: h.smtpService != nil,
		Now: time.Now(),
	}
	if h.addonCounter == nil {
		in.Features = -1
	}
	if h.apiAlertCounter != nil {
		in.APIAlerts = apiAlerts
	}
	if securityStats != nil {
		in.ActiveSessions = securityStats.ActiveSessions
		in.FailedLogins24h = securityStats.FailedLogins24h
		in.DisabledUsers = securityStats.DisabledUsers
	}
	if h.backupLister != nil {
		if bi, err := h.backupLister.BackupInfo(ctx); err != nil {
			slog.Warn("admin dashboard: backup info failed", slog.Any("error", err))
		} else {
			in.BackupsKnown, in.BackupsEnabled = true, bi.Enabled
			in.LastBackup = latestBackupTime(bi)
		}
	}

	return middleware.Render(c, http.StatusOK, AdminDashboardPage(buildHomeGroups(in), needs, recent))
}

// latestBackupTime is the newest backup file's time, ignoring manifests, which
// describe a backup rather than being one. Zero when there is none.
func latestBackupTime(bi BackupInfo) time.Time {
	var newest time.Time
	for _, a := range bi.Artifacts {
		if a.Kind == "manifest" {
			continue
		}
		if a.ModTime.After(newest) {
			newest = a.ModTime
		}
	}
	return newest
}

// Activity renders the filterable admin change log (GET /admin/activity).
func (h *Handler) Activity(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	q := ActivityQuery{Actor: c.QueryParam("actor"), Area: c.QueryParam("area"), When: c.QueryParam("when")}.Normalize()
	data := ActivityPageData{Query: q, Page: page, PerPage: activityPerPage}
	if h.activity != nil {
		ctx := c.Request().Context()
		var err error
		if data.Entries, data.Total, err = h.activity.List(ctx, ResolveActivityFilter(q, time.Now()), page, activityPerPage); err != nil {
			return apperror.NewInternal(fmt.Errorf("listing admin activity: %w", err))
		}
		// A missing "who" menu must not hide the log itself.
		if data.Actors, err = h.activity.Actors(ctx); err != nil {
			slog.Warn("failed to load admin activity actors", slog.Any("error", err))
		}
	}
	return middleware.Render(c, http.StatusOK, AdminActivityPage(data))
}

// --- Users ---

// Users renders the user management page (GET /admin/users).
func (h *Handler) Users(c echo.Context) error {
	ctx := c.Request().Context()
	lq := parseListQuery(c.QueryParam("q"), c.QueryParam("f"), c.QueryParam("page"))
	filter := auth.ParseUserFilter(lq.Filter)

	// Counts first: they give the total for the chosen chip, which lets a
	// page past the end be clamped before the rows are fetched.
	counts, err := h.authRepo.CountUserFilters(ctx, lq.Q)
	if err != nil {
		return err
	}
	page := clampPage(lq.Page, counts.For(filter), adminListPerPage)

	users, err := h.authRepo.SearchUsers(ctx, auth.UserSearchOptions{
		Query:   lq.Q,
		Filter:  filter,
		Offset:  (page - 1) * adminListPerPage,
		PerPage: adminListPerPage,
	})
	if err != nil {
		return err
	}

	data := UserListData{
		Users:     users,
		View:      userListView(lq, filter, counts, page),
		CSRFToken: middleware.GetCSRFToken(c),
	}
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, AdminUsersList(data))
	}
	return middleware.Render(c, http.StatusOK, AdminUsersPage(data))
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

	if newState {
		h.record(c, "user.admin_granted", "user", targetID, user.DisplayName)
	} else {
		h.record(c, "user.admin_revoked", "user", targetID, user.DisplayName)
	}

	return middleware.HTMXRedirect(c, peopleListReturnURL(c.Request().Header.Get("HX-Current-URL"), "/admin/users"))
}

// --- Campaigns ---

// Campaigns renders the campaign management page (GET /admin/campaigns).
func (h *Handler) Campaigns(c echo.Context) error {
	ctx := c.Request().Context()
	lq := parseListQuery(c.QueryParam("q"), c.QueryParam("f"), c.QueryParam("page"))

	// Counts first: they validate the chip and give the total, which lets a
	// page past the end be clamped before the rows are fetched.
	counts, err := h.campaignService.CountBySystem(ctx, lq.Q)
	if err != nil {
		return err
	}
	filter := resolveSystemFilter(lq.Filter, counts)
	view := campaignListView(lq, filter, counts, 1)
	view.Page = clampPage(lq.Page, view.Total, adminListPerPage)

	list, err := h.campaignService.SearchAll(ctx, campaigns.AdminSearchOptions{
		Query:       lq.Q,
		System:      filter,
		ListOptions: campaigns.ListOptions{Page: view.Page, PerPage: adminListPerPage},
	})
	if err != nil {
		return err
	}

	data := CampaignListData{
		Campaigns: list,
		View:      view,
		CSRFToken: middleware.GetCSRFToken(c),
	}
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, AdminCampaignsList(data))
	}
	return middleware.Render(c, http.StatusOK, AdminCampaignsPage(data))
}

// DeleteCampaign force-deletes a campaign (DELETE /admin/campaigns/:id).
func (h *Handler) DeleteCampaign(c echo.Context) error {
	campaignID := c.Param("id")

	// Read the name first: the row is gone after the delete.
	label := h.campaignLabel(c.Request().Context(), campaignID)
	if err := h.campaignService.Delete(c.Request().Context(), campaignID); err != nil {
		return err
	}
	h.record(c, "campaign.deleted", "campaign", campaignID, label)

	slog.Info("admin deleted campaign",
		slog.String("campaign_id", campaignID),
		slog.String("by", auth.GetUserID(c)),
	)

	return middleware.HTMXRedirect(c, "/admin/campaigns")
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
	h.record(c, "campaign.joined", "campaign", campaignID, h.campaignLabel(c.Request().Context(), campaignID))

	return middleware.HTMXRedirect(c, "/admin/campaigns")
}

// LeaveCampaign removes the admin from a campaign (DELETE /admin/campaigns/:id/leave).
func (h *Handler) LeaveCampaign(c echo.Context) error {
	campaignID := c.Param("id")
	userID := auth.GetUserID(c)

	if err := h.campaignService.RemoveMember(c.Request().Context(), campaignID, userID); err != nil {
		return err
	}

	h.record(c, "campaign.left", "campaign", campaignID, h.campaignLabel(c.Request().Context(), campaignID))
	slog.Info("admin left campaign",
		slog.String("campaign_id", campaignID),
		slog.String("user_id", userID),
	)

	return middleware.HTMXRedirect(c, "/admin/campaigns")
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
	// Resolve the live max-upload-size for display. The resolver reads
	// the same source the body-limit middleware reads, so the page can't
	// disagree with what /media/upload actually enforces.
	var maxUpload int64
	if h.resolveMaxUploadSize != nil {
		maxUpload = h.resolveMaxUploadSize()
	}
	data := StoragePageData{
		Stats:          stats,
		Files:          files,
		TotalFiles:     total,
		Page:           page,
		PerPage:        perPage,
		MaxUploadSize:  maxUpload,
		Global:         global,
		UserLimits:     userLimits,
		CampaignLimits: campaignLimits,
		Users:          allUsers,
		Campaigns:      allCampaigns,
		CSRFToken:      csrfToken,
		LimitsTab:      c.QueryParam("tab") == "limits",
	}
	if h.settingsService != nil {
		data.TrashRetentionDays = h.settingsService.TrashRetentionDays(ctx)
	}
	return middleware.Render(c, http.StatusOK, AdminStoragePage(data))
}

// DeleteMedia deletes a media file (DELETE /admin/media/:fileID).
func (h *Handler) DeleteMedia(c echo.Context) error {
	if h.mediaService == nil {
		return apperror.NewMissingContext()
	}

	fileID := c.Param("fileID")

	// Read the file name first: the row is gone after the delete.
	var fileLabel string
	if f, err := h.mediaService.GetByID(c.Request().Context(), fileID); err == nil && f != nil {
		fileLabel = f.OriginalName
	}
	if err := h.mediaService.Delete(c.Request().Context(), fileID); err != nil {
		return err
	}
	h.record(c, "media.deleted", "media", fileID, fileLabel)

	slog.Info("admin deleted media file",
		slog.String("file_id", fileID),
		slog.String("by", auth.GetUserID(c)),
	)

	return middleware.HTMXRedirect(c, "/admin/storage")
}

// --- Security ---

// Security renders the security dashboard page (GET /admin/security).
func (h *Handler) Security(c echo.Context) error {
	if h.securityService == nil {
		return apperror.NewMissingContext()
	}

	ctx := c.Request().Context()

	tab := normalizeSecurityTab(c.QueryParam("tab"))

	// A failed read shows an inline notice, not an empty-looking section.
	stats, statsErr := h.securityService.GetStats(ctx)
	if statsErr != nil {
		slog.Warn("security page: load stats failed", slog.Any("error", statsErr))
	}

	// Only the open tab's list is read; the others are one click away.
	eventType := c.QueryParam("type")
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	var (
		events      []SecurityEvent
		totalEvents int
		eventsErr   error
		watch       []WatchItem
	)
	switch tab {
	case SecurityTabLog:
		events, totalEvents, eventsErr = h.securityService.ListEvents(ctx, eventType, page)
		if eventsErr != nil {
			slog.Warn("security page: load events failed", slog.Any("error", eventsErr))
		}
	case SecurityTabOverview:
		var recent []SecurityEvent
		recent, _, eventsErr = h.securityService.ListEvents(ctx, EventLoginFailed, 1)
		if eventsErr != nil {
			slog.Warn("security page: load recent failures failed", slog.Any("error", eventsErr))
		} else {
			watch = worthALook(recent, time.Now())
		}
	}

	var (
		sessions    []auth.SessionInfo
		sessionsErr error
	)
	if tab == SecurityTabSessions {
		sessions, sessionsErr = h.securityService.GetActiveSessions(ctx)
		if sessionsErr != nil {
			slog.Warn("security page: load sessions failed", slog.Any("error", sessionsErr))
		}
	}

	csrfToken := middleware.GetCSRFToken(c)

	// Current site registration mode (defaults to "open" when the settings
	// service is absent or unset).
	registrationMode := settings.RegistrationOpen
	if h.settingsService != nil {
		if m, err := h.settingsService.GetRegistrationMode(ctx); err == nil {
			registrationMode = m
		}
	}

	data := SecurityPageData{
		Tab:              tab,
		WatchItems:       watch,
		Stats:            stats,
		Events:           events,
		TotalEvents:      totalEvents,
		EventFilter:      eventType,
		Page:             page,
		PerPage:          securityPerPage,
		Sessions:         sessions,
		CSRFToken:        csrfToken,
		RegistrationMode: registrationMode,
		StatsFailed:      statsErr != nil,
		EventsFailed:     eventsErr != nil,
		SessionsFailed:   sessionsErr != nil,
	}

	return middleware.Render(c, http.StatusOK, AdminSecurityPage(data))
}

// UpdateRegistrationMode persists the site registration gate (POST
// /admin/security/registration). Reauth-gated as a security-sensitive change.
func (h *Handler) UpdateRegistrationMode(c echo.Context) error {
	if h.settingsService == nil {
		return apperror.NewMissingContext()
	}
	mode := c.FormValue("registration_mode")
	if err := h.settingsService.UpdateRegistrationMode(c.Request().Context(), mode); err != nil {
		return err
	}
	h.record(c, "registration.mode_changed", "setting", "registration_mode", registrationModeLabel(mode))
	slog.Info("registration mode updated", slog.String("mode", mode))
	c.Response().Header().Set("HX-Redirect", securityTabHref(SecurityTabSignup))
	return c.NoContent(http.StatusOK)
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
		map[string]any{"token_hint": tokenHint(tokenHash)})

	h.record(c, "session.terminated", "session", "", "")
	slog.Info("admin terminated session",
		slog.String("by", currentUserID),
	)

	return middleware.HTMXRedirect(c, securityTabHref(SecurityTabSessions))
}

// registrationModeLabel is the plain-language mode name the activity sentence
// ("changed who can sign up to open") reads with.
func registrationModeLabel(mode string) string {
	switch mode {
	case settings.RegistrationOpen:
		return "open"
	case settings.RegistrationInvite:
		return "invite only"
	case settings.RegistrationClosed:
		return "closed"
	}
	return mode
}

// tokenHint is the short prefix shown in the event log; the full hash would
// let a log reader address that session.
func tokenHint(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
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

	h.record(c, "user.force_logout", "user", targetID, h.userLabel(c.Request().Context(), targetID))
	slog.Info("admin force-logged out user",
		slog.String("target_user", targetID),
		slog.Int("sessions_destroyed", count),
		slog.String("by", currentUserID),
	)

	return middleware.HTMXRedirect(c, peopleListReturnURL(c.Request().Header.Get("HX-Current-URL"), "/admin/security"))
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

	h.record(c, "user.disabled", "user", targetID, h.userLabel(c.Request().Context(), targetID))
	slog.Info("admin disabled user",
		slog.String("target_user", targetID),
		slog.String("by", currentUserID),
	)

	return middleware.HTMXRedirect(c, peopleListReturnURL(c.Request().Header.Get("HX-Current-URL"), "/admin/security"))
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

	h.record(c, "user.enabled", "user", targetID, h.userLabel(c.Request().Context(), targetID))
	slog.Info("admin enabled user",
		slog.String("target_user", targetID),
		slog.String("by", currentUserID),
	)

	return middleware.HTMXRedirect(c, peopleListReturnURL(c.Request().Header.Get("HX-Current-URL"), "/admin/security"))
}

// --- Database Explorer ---

// Database renders the admin database explorer page (GET /admin/database).
func (h *Handler) Database(c echo.Context) error {
	ctx := c.Request().Context()

	var statuses []PluginMigrationStatus
	var core CoreMigrationStatus
	var tableCount int
	if h.databaseExplorer != nil {
		var err error
		statuses, err = h.databaseExplorer.GetMigrationStatus(ctx)
		if err != nil {
			slog.Warn("failed to get migration status", slog.Any("error", err))
		}

		// Core schema_migrations state (version, dirty, pending, DB-ahead).
		if cs, cerr := h.databaseExplorer.GetCoreMigrationStatus(ctx); cerr != nil {
			slog.Warn("failed to get core migration status", slog.Any("error", cerr))
		} else {
			core = cs
		}

		// Quick table count for the page header.
		schema, err := h.databaseExplorer.GetSchema(ctx)
		if err != nil {
			slog.Warn("failed to get schema for table count", slog.Any("error", err))
		} else {
			tableCount = len(schema.Tables)
		}
	}

	var health *HealthResult
	if h.healthChecker != nil {
		health = h.healthChecker.RunChecks()
	}

	var backups BackupInfo
	if h.backupLister != nil {
		if bi, err := h.backupLister.BackupInfo(ctx); err != nil {
			slog.Warn("failed to list backups", slog.Any("error", err))
		} else {
			backups = bi
		}
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, AdminDatabasePage(normalizeDatabaseTab(c.QueryParam("tab")), core, statuses, health, backups, tableCount, csrfToken))
}

// DatabaseStatusAPI returns core + plugin migration status as JSON
// (GET /admin/database/status). Usable by external monitoring/alerting as well
// as the admin page.
func (h *Handler) DatabaseStatusAPI(c echo.Context) error {
	if h.databaseExplorer == nil {
		return apperror.NewInternal(fmt.Errorf("database explorer not configured"))
	}
	ctx := c.Request().Context()
	core, err := h.databaseExplorer.GetCoreMigrationStatus(ctx)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("core migration status: %w", err))
	}
	plugins, err := h.databaseExplorer.GetMigrationStatus(ctx)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("plugin migration status: %w", err))
	}
	return c.JSON(http.StatusOK, map[string]any{
		"core":    core,
		"plugins": plugins,
	})
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

	ctx := c.Request().Context()

	// Snapshot what was pending first: the results list every plugin, healthy
	// or not, so only this lets us say which ones actually applied something.
	pendingBefore := map[string]int{}
	if statuses, err := h.databaseExplorer.GetMigrationStatus(ctx); err == nil {
		for _, s := range statuses {
			pendingBefore[s.Slug] = s.Pending
		}
	}

	results, err := h.databaseExplorer.ApplyPendingMigrations(ctx)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("applying migrations: %w", err))
	}

	var applied, failed []string
	for _, r := range results {
		if r.Healthy {
			if pendingBefore[r.Slug] > 0 {
				applied = append(applied, r.Slug)
				slog.Info("migration applied via admin",
					slog.String("plugin", r.Slug),
					slog.Int("version", r.Version),
				)
			}
			continue
		}
		failed = append(failed, r.Slug)
		slog.Error("migration failed via admin",
			slog.String("plugin", r.Slug),
			slog.Any("error", r.Error),
		)
	}

	if len(applied) > 0 {
		h.record(c, "migrations.applied", "database", "", strings.Join(applied, ", "))
	}

	if len(failed) == 0 {
		return middleware.HTMXRedirect(c, "/admin/database")
	}

	// A redirect would drop the toast, so stay on the page and say what
	// failed; the server log has the technical error.
	msg := "These parts could not be updated: " + strings.Join(failed, ", ") + ". Check the server log, then reload this page."
	if len(applied) > 0 {
		msg = "Updated " + strings.Join(applied, ", ") + ". " + msg
	}
	if payload, jerr := json.Marshal(map[string]any{"chronicle:notify": map[string]string{"message": msg, "type": "error"}}); jerr == nil {
		c.Response().Header().Set("HX-Trigger", string(payload))
	}
	if !middleware.IsHTMX(c) {
		return middleware.HTMXRedirect(c, "/admin/database")
	}
	return c.NoContent(http.StatusNoContent)
}

// Systems renders the system diagnostics page (GET /admin/systems).
func (h *Handler) Systems(c echo.Context) error {
	ctx := c.Request().Context()
	manifests := systems.Registry()
	allSystems := systems.AllSystems()

	// Build a set of instantiated system IDs for fast lookup.
	instantiated := make(map[string]bool, len(allSystems))
	for _, sys := range allSystems {
		instantiated[sys.Info().ID] = true
	}

	// Build diagnostic entries from manifests.
	entries := make([]SystemDiagnosticEntry, 0, len(manifests))
	for _, m := range manifests {
		campaignCount := 0
		if h.addonUsageCounter != nil {
			if n, err := h.addonUsageCounter.CountCampaignsUsingAddon(ctx, m.ID); err == nil {
				campaignCount = n
			}
		}

		entries = append(entries, SystemDiagnosticEntry{
			ID:              m.ID,
			Name:            m.Name,
			Version:         m.Version,
			Status:          string(m.Status),
			Instantiated:    instantiated[m.ID],
			Dir:             systems.Dir(m.ID),
			FoundrySystemID: m.FoundrySystemID,
			CampaignCount:   campaignCount,
		})
	}

	events := systems.DiagnosticEvents()

	return middleware.Render(c, http.StatusOK, AdminSystemsPage(entries, events))
}

// diagnosticsCampaigns loads the campaign list for the workspace reference panel
// and the review-step picker. Best-effort: a load error just yields no options.
func (h *Handler) diagnosticsCampaigns(ctx context.Context) []CampaignOption {
	cs, _, err := h.campaignService.ListAll(ctx, campaigns.ListOptions{Page: 1, PerPage: 500})
	if err != nil {
		return nil
	}
	out := make([]CampaignOption, 0, len(cs))
	for _, c := range cs {
		out = append(out, CampaignOption{ID: c.ID, Name: c.Name, Slug: c.Slug})
	}
	return out
}

// DiagnosticsWorkspace renders the in-app operator AI workspace: the copyable
// functions list + a paste box for the AI's batch request. Read-only.
func (h *Handler) DiagnosticsWorkspace(c echo.Context) error {
	return middleware.Render(c, http.StatusOK, DiagnosticsWorkspacePage(DiagnosticsWorkspaceData{
		FunctionsJSON: systems.FunctionsSpecJSON(),
		CSRFToken:     middleware.GetCSRFToken(c),
		Campaigns:     h.diagnosticsCampaigns(c.Request().Context()),
	}))
}

// DiagnosticsWorkspaceParse validates a pasted batch request against the
// diagnostic catalog and returns the review fragment (the human-approval gate).
// A malformed request is not an error response — it renders an inline message so
// the operator can fix the paste. POST /admin/diagnostics/workspace/parse.
func (h *Handler) DiagnosticsWorkspaceParse(c echo.Context) error {
	raw := c.FormValue("batch")
	data := DiagnosticsReviewData{Raw: raw, CSRFToken: middleware.GetCSRFToken(c)}
	plan, err := systems.ParseBatch(raw)
	if err != nil {
		data.Err = err.Error()
	} else {
		data.Plan = plan
		// If any call left its campaign/entity slot as a placeholder, offer pickers
		// at the review step instead of failing the run with "not found".
		if systems.PlanNeedsCampaign(plan) {
			data.NeedsCampaign = true
			data.Campaigns = h.diagnosticsCampaigns(c.Request().Context())
		}
		data.NeedsEntity = systems.PlanNeedsEntity(plan)
	}
	return middleware.Render(c, http.StatusOK, DiagnosticsBatchReview(data))
}

// DiagnosticsWorkspaceRun re-parses the approved request (never trusting a
// client-built plan), executes the runnable read-only diagnostics, and returns
// the compact, secret-redacted result fragment.
// POST /admin/diagnostics/workspace/run.
func (h *Handler) DiagnosticsWorkspaceRun(c echo.Context) error {
	raw := c.FormValue("batch")
	plan, err := systems.ParseBatch(raw)
	if err != nil {
		// Shouldn't happen (parse succeeded to reach the approve button), but if
		// the re-parse fails, surface it in the review fragment rather than 500.
		return middleware.Render(c, http.StatusOK, DiagnosticsBatchReview(DiagnosticsReviewData{
			Raw: raw, Err: err.Error(), CSRFToken: middleware.GetCSRFToken(c),
		}))
	}
	// If the operator picked a campaign at the review step, substitute it into
	// any call that left its campaign slot as a placeholder.
	if pick := strings.TrimSpace(c.FormValue("campaign_pick")); pick != "" {
		systems.ApplyCampaignPick(plan, pick)
	}
	if pick := strings.TrimSpace(c.FormValue("entity_pick")); pick != "" {
		systems.ApplyEntityPick(plan, pick)
	}
	result := systems.RunBatch(plan)

	// Audit: site-admin diagnostics runs are logged to the security/activity
	// feed with actor identity, IP, and UA. Counts + byte size only — never the
	// diagnostic payload (mirrors ai_workspace's counts-only discipline).
	if h.securityService != nil {
		_ = h.securityService.LogEvent(c.Request().Context(), EventDiagnosticsBatchRun,
			"", auth.GetUserID(c), c.RealIP(), c.Request().UserAgent(),
			map[string]any{
				"runnable":  plan.RunnableN,
				"calls":     len(plan.Calls),
				"full_dump": plan.Request.FullDump,
				"bytes":     len(result),
			})
	}
	slog.Info("admin ran diagnostics batch",
		slog.Int("runnable", plan.RunnableN),
		slog.Int("calls", len(plan.Calls)),
		slog.Bool("full_dump", plan.Request.FullDump))

	return middleware.Render(c, http.StatusOK, DiagnosticsBatchResult(DiagnosticsResultData{Result: result}))
}

// SecurityPageData holds all data needed for the security dashboard page.
type SecurityPageData struct {
	// Tab is the open tab, already whitelisted (see normalizeSecurityTab).
	Tab string
	// WatchItems feeds the overview's "Worth a look" list.
	WatchItems []WatchItem
	Stats       *SecurityStats
	Events      []SecurityEvent
	TotalEvents int
	EventFilter string
	Page        int
	PerPage     int
	Sessions    []auth.SessionInfo
	CSRFToken   string
	// RegistrationMode is the current site registration gate ("open", "invite",
	// "closed"). Rendered as a select on the security page (B-R4).
	RegistrationMode string
	// StatsFailed, EventsFailed and SessionsFailed mark sections whose read
	// errored, so the page says so instead of rendering an empty state.
	StatsFailed    bool
	EventsFailed   bool
	SessionsFailed bool
}
