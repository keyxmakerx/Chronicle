package campaigns

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// EntityTypeLister lists entity types for the settings page sidebar config.
// Avoids importing the entities package directly.
type EntityTypeLister interface {
	GetEntityTypesForSettings(ctx context.Context, campaignID string) ([]SettingsEntityType, error)
}

// SettingsEntityType is a minimal entity type representation for the settings
// and customization pages. Includes Description for the Category Dashboards tab.
type SettingsEntityType struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	NamePlural  string  `json:"name_plural"`
	Icon        string  `json:"icon"`
	Color       string  `json:"color"`
	Description *string `json:"description,omitempty"`
}

// EntityTypeLayoutFetcher fetches a single entity type's full layout and field
// data for the page layout editor. Avoids importing the entities package directly.
type EntityTypeLayoutFetcher interface {
	GetEntityTypeForLayoutEditor(ctx context.Context, entityTypeID int) (*LayoutEditorEntityType, error)
}

// LayoutEditorEntityType holds the entity type data needed to mount the
// template-editor widget in the Customization Hub's Page Layouts tab.
// LayoutJSON and FieldsJSON are pre-serialized so templates can emit them
// directly as data attributes.
type LayoutEditorEntityType struct {
	ID         int
	CampaignID string
	Name       string
	NamePlural string
	Icon       string
	Color      string
	LayoutJSON string
	FieldsJSON string
}

// RecentEntityLister returns recently updated entities for the campaign dashboard.
// Avoids importing the entities package directly.
type RecentEntityLister interface {
	ListRecentForDashboard(ctx context.Context, campaignID string, role int, userID string, limit int) ([]RecentEntity, error)
}

// RecentEntity is a minimal entity representation for the dashboard recent pages section.
type RecentEntity struct {
	ID        string
	Name      string
	TypeName  string
	TypeIcon  string
	TypeColor string
	ImagePath *string
	IsPrivate bool
	UpdatedAt time.Time
}

// AuditLogger records audit events for campaign-scoped actions. Defined here
// as an interface to avoid circular imports with the audit plugin.
type AuditLogger interface {
	LogEvent(ctx context.Context, campaignID, userID, action string, details map[string]any) error
}

// PluginHubAddon is a minimal addon representation for the plugin hub page.
type PluginHubAddon struct {
	AddonID   int
	Slug      string
	Name      string
	Icon      string
	Category  string
	Enabled   bool
	Installed bool // Whether backing code exists (for showing Coming Soon vs toggle).
}

// AddonLister lists addons for the plugin hub page. Avoids importing the addons
// package directly.
type AddonLister interface {
	ListForPluginHub(ctx context.Context, campaignID string) ([]PluginHubAddon, error)
}

// MediaUploader uploads media files for campaign backdrops. Avoids importing the
// media package directly.
type MediaUploader interface {
	UploadBackdrop(ctx context.Context, campaignID, userID string, fileBytes []byte, originalName, mimeType string) (filename string, err error)
}

// SMTPChecker reports whether SMTP email delivery is configured.
// Used to show warnings on the settings page when email is unavailable.
type SMTPChecker interface {
	IsConfigured(ctx context.Context) bool
}

// SystemLister lists available game systems for the settings page dropdown.
// Avoids importing the systems package directly (prevents import cycle).
type SystemLister interface {
	ListSystems() []SystemOption
}

// Handler handles HTTP requests for campaign operations. Handlers are thin:
// bind request, call service, render response. No business logic lives here.
type Handler struct {
	service       CampaignService
	groupSvc      GroupService
	entityLister  EntityTypeLister
	layoutFetcher EntityTypeLayoutFetcher
	recentLister  RecentEntityLister
	auditLogger   AuditLogger
	addonLister   AddonLister
	mediaUploader MediaUploader
	smtpChecker   SMTPChecker
	systemLister  SystemLister
	baseURL       string
}

// NewHandler creates a new campaign handler.
func NewHandler(service CampaignService) *Handler {
	return &Handler{service: service}
}

// SetGroupService sets the group service for campaign group management.
func (h *Handler) SetGroupService(svc GroupService) {
	h.groupSvc = svc
}

// SetEntityLister sets the entity type lister for the settings page.
// Called after both plugins are wired to avoid circular dependencies.
func (h *Handler) SetEntityLister(lister EntityTypeLister) {
	h.entityLister = lister
}

// SetLayoutFetcher sets the entity type layout fetcher for the Page Layouts tab.
// Called after both plugins are wired to avoid circular dependencies.
func (h *Handler) SetLayoutFetcher(fetcher EntityTypeLayoutFetcher) {
	h.layoutFetcher = fetcher
}

// SetRecentEntityLister sets the recent entity lister for the dashboard.
// Called after both plugins are wired to avoid circular dependencies.
func (h *Handler) SetRecentEntityLister(lister RecentEntityLister) {
	h.recentLister = lister
}

// SetAuditLogger sets the audit logger for recording campaign mutations.
// Called after all plugins are wired to avoid initialization order issues.
func (h *Handler) SetAuditLogger(logger AuditLogger) {
	h.auditLogger = logger
}

// SetAddonLister sets the addon lister for the plugin hub page.
func (h *Handler) SetAddonLister(lister AddonLister) {
	h.addonLister = lister
}

// SetMediaUploader sets the media uploader for backdrop image uploads.
func (h *Handler) SetMediaUploader(uploader MediaUploader) {
	h.mediaUploader = uploader
}

// SetBaseURL sets the public-facing base URL for generating integration URLs
// (e.g. Foundry VTT module install URL).
func (h *Handler) SetBaseURL(url string) {
	h.baseURL = url
}

// SetSMTPChecker sets the SMTP configuration checker for the settings page.
func (h *Handler) SetSMTPChecker(checker SMTPChecker) {
	h.smtpChecker = checker
}

// SetSystemLister sets the system lister for the game system dropdown.
func (h *Handler) SetSystemLister(lister SystemLister) {
	h.systemLister = lister
}

// logAudit fires a fire-and-forget audit entry. Errors are logged but
// never block the primary operation.
func (h *Handler) logAudit(c echo.Context, campaignID, action string, details map[string]any) {
	if h.auditLogger == nil {
		return
	}
	userID := auth.GetUserID(c)
	if err := h.auditLogger.LogEvent(c.Request().Context(), campaignID, userID, action, details); err != nil {
		slog.Warn("audit log failed", slog.String("action", action), slog.Any("error", err))
	}
}

// --- Campaign CRUD ---

// Index renders the campaign list page (GET /campaigns).
func (h *Handler) Index(c echo.Context) error {
	userID := auth.GetUserID(c)

	page, _ := strconv.Atoi(c.QueryParam("page"))
	opts := DefaultListOptions()
	if page > 0 {
		opts.Page = page
	}

	campaigns, total, err := h.service.List(c.Request().Context(), userID, opts)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, CampaignListContent(campaigns, total, opts, csrfToken))
	}
	return middleware.Render(c, http.StatusOK, CampaignIndexPage(campaigns, total, opts, csrfToken))
}

// Picker returns an HTMX fragment listing the user's campaigns for the
// topbar campaign selector dropdown. Loads lazily on dropdown open.
func (h *Handler) Picker(c echo.Context) error {
	userID := auth.GetUserID(c)

	// Fetch all campaigns (up to 50 — most users have far fewer).
	opts := DefaultListOptions()
	opts.PerPage = 50
	campaigns, _, err := h.service.List(c.Request().Context(), userID, opts)
	if err != nil {
		return err
	}

	// Get the current campaign ID (if any) to mark as active.
	var activeCampaignID string
	if cc := GetCampaignContext(c); cc != nil {
		activeCampaignID = cc.Campaign.ID
	}

	return middleware.Render(c, http.StatusOK, CampaignPickerFragment(campaigns, activeCampaignID))
}

// NewForm renders the campaign creation form (GET /campaigns/new).
func (h *Handler) NewForm(c echo.Context) error {
	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, CampaignNewPage(csrfToken, "", ""))
}

// Create processes the campaign creation form (POST /campaigns).
func (h *Handler) Create(c echo.Context) error {
	var req CreateCampaignRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	// Validate field lengths before processing.
	if err := apperror.ValidateRequired("name", req.Name); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("name", req.Name, apperror.MaxNameLength); err != nil {
		return err
	}
	if err := apperror.ValidateStringLength("description", req.Description, apperror.MaxDescriptionLength); err != nil {
		return err
	}

	userID := auth.GetUserID(c)
	input := CreateCampaignInput(req)

	campaign, err := h.service.Create(c.Request().Context(), userID, input)
	if err != nil {
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := apperror.UserMessage(err, "failed to create campaign")
		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, CampaignCreateForm(csrfToken, &req, errMsg))
		}
		return middleware.Render(c, http.StatusOK, CampaignNewPage(csrfToken, req.Name, errMsg))
	}

	return middleware.HTMXRedirect(c, "/campaigns/"+campaign.ID)
}

// Show renders the campaign dashboard (GET /campaigns/:id).
// Uses role-aware dashboard layout: each role can have its own Campaign Page
// layout, falling back to the default layout.
func (h *Handler) Show(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	// Check for pending transfer to show banner.
	transfer, _ := h.service.GetPendingTransfer(c.Request().Context(), cc.Campaign.ID)

	// Fetch recently updated pages for the dashboard.
	var recentEntities []RecentEntity
	if h.recentLister != nil {
		recentEntities, _ = h.recentLister.ListRecentForDashboard(
			c.Request().Context(), cc.Campaign.ID, int(cc.MemberRole), auth.GetUserID(c), 8,
		)
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, CampaignShowPage(cc, transfer, recentEntities, csrfToken))
}

// EditForm redirects to the unified settings page (GET /campaigns/:id/edit).
// Kept for backward compatibility with bookmarks and links.
func (h *Handler) EditForm(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/settings")
}

// Update processes the campaign edit form (PUT /campaigns/:id).
func (h *Handler) Update(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req UpdateCampaignRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	input := UpdateCampaignInput(req)

	_, err := h.service.Update(c.Request().Context(), cc.Campaign.ID, input)
	if err != nil {
		errMsg := apperror.UserMessage(err, "failed to update campaign")
		csrfToken := middleware.GetCSRFToken(c)
		transfer, _ := h.service.GetPendingTransfer(c.Request().Context(), cc.Campaign.ID)
		var entityTypes []SettingsEntityType
		if h.entityLister != nil {
			entityTypes, _ = h.entityLister.GetEntityTypesForSettings(c.Request().Context(), cc.Campaign.ID)
		}
		return middleware.Render(c, http.StatusOK, CampaignSettingsPage(cc, transfer, entityTypes, nil, csrfToken, errMsg, h.baseURL, "general", false, nil))
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.updated", nil)

	return middleware.HTMXRedirect(c, "/campaigns/"+cc.Campaign.ID+"/settings")
}

// Delete removes a campaign (DELETE /campaigns/:id). Requires the request to
// include a confirm_name field that matches the campaign name exactly (S4).
func (h *Handler) Delete(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req struct {
		ConfirmName string `json:"confirm_name" form:"confirm_name"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if req.ConfirmName != cc.Campaign.Name {
		return apperror.NewBadRequest("campaign name does not match; deletion cancelled")
	}

	if err := h.service.Delete(c.Request().Context(), cc.Campaign.ID); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.deleted", nil)
	return middleware.HTMXRedirect(c, "/campaigns")
}

// --- Backdrop & Branding ---

// UploadBackdrop handles POST /campaigns/:id/backdrop. Accepts an image file,
// stores it via the media service, and sets the campaign's backdrop_path.
func (h *Handler) UploadBackdrop(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can change the backdrop")
	}

	if h.mediaUploader == nil {
		return apperror.NewInternal(nil)
	}

	file, err := c.FormFile("file")
	if err != nil {
		return apperror.NewBadRequest("no file provided")
	}

	src, err := file.Open()
	if err != nil {
		return apperror.NewInternal(err)
	}
	defer func() { _ = src.Close() }()

	fileBytes, err := io.ReadAll(src)
	if err != nil {
		return apperror.NewBadRequest("failed to read file")
	}

	mimeType := http.DetectContentType(fileBytes)

	filename, err := h.mediaUploader.UploadBackdrop(
		c.Request().Context(), cc.Campaign.ID,
		auth.GetUserID(c), fileBytes, file.Filename, mimeType,
	)
	if err != nil {
		return err
	}

	if err := h.service.UpdateBackdropPath(c.Request().Context(), cc.Campaign.ID, &filename); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.backdrop.uploaded", nil)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Trigger", "backdrop-updated")
		return middleware.Render(c, http.StatusOK, BackdropUploadSection(cc.Campaign.ID, &filename, middleware.GetCSRFToken(c)))
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/settings")
}

// RemoveBackdrop handles DELETE /campaigns/:id/backdrop. Clears the campaign's
// backdrop image.
func (h *Handler) RemoveBackdrop(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can change the backdrop")
	}

	if err := h.service.UpdateBackdropPath(c.Request().Context(), cc.Campaign.ID, nil); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.backdrop.removed", nil)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Trigger", "backdrop-updated")
		return middleware.Render(c, http.StatusOK, BackdropUploadSection(cc.Campaign.ID, nil, middleware.GetCSRFToken(c)))
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/settings")
}

// UpdateAccentColorAPI handles PUT /campaigns/:id/accent-color. Sets the
// campaign's accent color for branding customization.
func (h *Handler) UpdateAccentColorAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can change branding")
	}

	color := c.FormValue("accent_color")
	// Validate hex color format (allow empty to reset).
	if color != "" && (len(color) != 7 || color[0] != '#') {
		return apperror.NewBadRequest("invalid color format, expected #RRGGBB")
	}

	if err := h.service.UpdateAccentColor(c.Request().Context(), cc.Campaign.ID, color); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.accent_color.updated", map[string]any{"color": color})

	// HTMX and API (Accept: application/json) callers get a direct response;
	// plain browser navigation gets a redirect back to settings.
	if middleware.IsHTMX(c) {
		return c.NoContent(http.StatusNoContent)
	}
	if c.Request().Header.Get("Accept") == "application/json" {
		return c.JSON(http.StatusOK, map[string]string{"accent_color": color})
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/settings")
}

// UpdateBrandingAPI handles PUT /campaigns/:id/branding. Sets the campaign's
// custom brand name and optional logo path.
func (h *Handler) UpdateBrandingAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can change branding")
	}

	var req struct {
		BrandName string `json:"brand_name"`
		BrandLogo string `json:"brand_logo"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.service.UpdateBranding(c.Request().Context(), cc.Campaign.ID, req.BrandName, req.BrandLogo); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.branding.updated", map[string]any{
		"brand_name": req.BrandName,
		"brand_logo": req.BrandLogo,
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateTopbarStyleAPI handles PUT /campaigns/:id/topbar-style. Sets the
// campaign's topbar visual customization (solid color, gradient, or image).
func (h *Handler) UpdateTopbarStyleAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can change topbar style")
	}

	var style TopbarStyle
	if err := c.Bind(&style); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// Empty mode means clear/reset.
	var stylePtr *TopbarStyle
	if style.Mode != "" {
		stylePtr = &style
	}

	if err := h.service.UpdateTopbarStyle(c.Request().Context(), cc.Campaign.ID, stylePtr); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.topbar_style.updated", map[string]any{
		"mode": style.Mode,
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateDmGrantsAPI handles PUT /campaigns/:id/dm-grants. Sets which users
// are granted visibility of dm_only content (co-DM privileges).
func (h *Handler) UpdateDmGrantsAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can manage DM grants")
	}

	var req struct {
		UserIDs []string `json:"user_ids"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.UpdateDmGrants(c.Request().Context(), cc.Campaign.ID, req.UserIDs); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.dm_grants.updated", map[string]any{"user_ids": req.UserIDs})

	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// GetDmGrantsAPI handles GET /campaigns/:id/dm-grants. Returns the current
// list of users granted dm_only visibility.
func (h *Handler) GetDmGrantsAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can view DM grants")
	}

	settings := cc.Campaign.ParseSettings()
	ids := settings.DmGrantIDs
	if ids == nil {
		ids = []string{}
	}

	return c.JSON(http.StatusOK, map[string]any{"user_ids": ids})
}

// UpdateFontFamilyAPI handles PUT /campaigns/:id/font-family. Sets the
// campaign's body font family for visual customization.
func (h *Handler) UpdateFontFamilyAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can change font settings")
	}

	var req struct {
		FontFamily string `json:"font_family"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.service.UpdateFontFamily(c.Request().Context(), cc.Campaign.ID, req.FontFamily); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.font_family.updated", map[string]any{"font_family": req.FontFamily})
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateWelcomeMessageAPI handles PUT /campaigns/:id/welcome-message. Sets the
// campaign's MOTD banner displayed on the dashboard.
func (h *Handler) UpdateWelcomeMessageAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can set the welcome message")
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.service.UpdateWelcomeMessage(c.Request().Context(), cc.Campaign.ID, req.Message); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.welcome_message.updated", nil)
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateDefaultVisibilityAPI handles PUT /campaigns/:id/default-visibility. Sets
// the default visibility applied to newly created entities.
func (h *Handler) UpdateDefaultVisibilityAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can change default visibility")
	}

	var req struct {
		Visibility string `json:"visibility"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.service.UpdateDefaultVisibility(c.Request().Context(), cc.Campaign.ID, req.Visibility); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.default_visibility.updated", map[string]any{"visibility": req.Visibility})
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Settings ---

// Settings renders the campaign settings page (GET /campaigns/:id/settings).
// Supports ?tab= query param to open a specific tab (general, people, integrations, activity).
func (h *Handler) Settings(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	ctx := c.Request().Context()
	transfer, _ := h.service.GetPendingTransfer(ctx, cc.Campaign.ID)
	csrfToken := middleware.GetCSRFToken(c)

	// Fetch entity types for sidebar config widget.
	var entityTypes []SettingsEntityType
	if h.entityLister != nil {
		entityTypes, _ = h.entityLister.GetEntityTypesForSettings(ctx, cc.Campaign.ID)
	}

	// Fetch members for the People tab.
	members, _ := h.service.ListMembers(ctx, cc.Campaign.ID)

	// Check SMTP configuration for invite/transfer warnings.
	smtpConfigured := false
	if h.smtpChecker != nil {
		smtpConfigured = h.smtpChecker.IsConfigured(ctx)
	}

	// Read the active tab from query params.
	activeTab := c.QueryParam("tab")
	if activeTab == "" {
		activeTab = "general"
	}

	// Fetch available game systems for the system selector.
	var systemOptions []SystemOption
	if h.systemLister != nil {
		systemOptions = h.systemLister.ListSystems()
	}

	return middleware.Render(c, http.StatusOK, CampaignSettingsPage(cc, transfer, entityTypes, members, csrfToken, "", h.baseURL, activeTab, smtpConfigured, systemOptions))
}

// PluginHub renders the campaign plugin hub page, showing all enabled
// addons with quick links to their main pages.
// GET /campaigns/:id/plugins
func (h *Handler) PluginHub(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var addons []PluginHubAddon
	if h.addonLister != nil {
		var err error
		addons, err = h.addonLister.ListForPluginHub(c.Request().Context(), cc.Campaign.ID)
		if err != nil {
			slog.Warn("plugin hub: list addons failed", slog.Any("error", err))
		}
	}

	isOwner := cc.MemberRole >= RoleOwner
	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, PluginHubPage(cc, addons, isOwner, csrfToken))
}

// PluginHubFragment returns the plugin hub list content as an HTMX fragment.
// Used for in-place refresh after toggling features.
// GET /campaigns/:id/plugins/fragment
func (h *Handler) PluginHubFragment(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var addons []PluginHubAddon
	if h.addonLister != nil {
		var err error
		addons, err = h.addonLister.ListForPluginHub(c.Request().Context(), cc.Campaign.ID)
		if err != nil {
			slog.Warn("plugin hub fragment: list addons failed", slog.Any("error", err))
		}
	}

	isOwner := cc.MemberRole >= RoleOwner
	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, PluginHubListContent(cc, addons, isOwner, csrfToken))
}

// --- Customization Hub ---

// Customize renders the Campaign Customization Hub (GET /campaigns/:id/customize).
// Owners use this page to control navigation, dashboards, and category layouts.
func (h *Handler) Customize(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	csrfToken := middleware.GetCSRFToken(c)

	// Fetch entity types for the categories tabs and navigation editor.
	var entityTypes []SettingsEntityType
	if h.entityLister != nil {
		entityTypes, _ = h.entityLister.GetEntityTypesForSettings(c.Request().Context(), cc.Campaign.ID)
	}

	return middleware.Render(c, http.StatusOK, CustomizePage(cc, entityTypes, csrfToken))
}

// LayoutEditorFragment returns an HTMX fragment containing the template-editor
// widget for a specific entity type. Used by the Page Layouts tab in the
// Customization Hub to lazy-load editors one category at a time.
// GET /campaigns/:id/customize/layout-editor/:etid
func (h *Handler) LayoutEditorFragment(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}

	if h.layoutFetcher == nil {
		return apperror.NewMissingContext()
	}

	et, err := h.layoutFetcher.GetEntityTypeForLayoutEditor(c.Request().Context(), etID)
	if err != nil {
		return err
	}

	// IDOR protection: verify entity type belongs to this campaign.
	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, LayoutEditorFragment(cc, et, csrfToken))
}

// --- Sidebar Config API ---

// GetSidebarConfig returns the sidebar configuration as JSON (GET /campaigns/:id/sidebar-config).
func (h *Handler) GetSidebarConfig(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	cfg, err := h.service.GetSidebarConfig(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, cfg)
}

// UpdateSidebarConfig updates the sidebar configuration (PUT /campaigns/:id/sidebar-config).
func (h *Handler) UpdateSidebarConfig(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req UpdateSidebarConfigRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	config := SidebarConfig{
		Items:           req.Items,
		HiddenEntityIDs: req.HiddenEntityIDs,
		HiddenNodeIDs:   req.HiddenNodeIDs,
		EntityTypeOrder: req.EntityTypeOrder,
		HiddenTypeIDs:   req.HiddenTypeIDs,
		CustomSections:  req.CustomSections,
		CustomLinks:     req.CustomLinks,
	}

	if err := h.service.UpdateSidebarConfig(c.Request().Context(), cc.Campaign.ID, config); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Sidebar Drill-Down ---

// SidebarDrill returns the drill-down panel content for a sidebar category
// (GET /campaigns/:id/sidebar/drill/:slug). The LayoutInjector populates
// entity types and counts into the context, and the Templ component finds
// the matching entity type by slug.
func (h *Handler) SidebarDrill(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	slug := c.Param("slug")
	if slug == "" {
		return apperror.NewBadRequest("category slug is required")
	}

	return middleware.Render(c, http.StatusOK, layouts.SidebarDrillFragment(slug))
}

// --- Dashboard Layout ---

// GetDashboardLayout returns the current dashboard layout as JSON (GET /campaigns/:id/dashboard-layout).
// Supports ?role= query param (default/player/scribe) for role-specific layouts.
// Returns null if no custom layout is set (meaning the default is in use).
func (h *Handler) GetDashboardLayout(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	roleName := c.QueryParam("role")
	if roleName == "" {
		roleName = "default"
	}

	// Fetch campaign to access the raw dashboard_layout JSON.
	campaign, err := h.service.GetByID(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	layout := campaign.GetRoleDashboardJSON(roleName)
	return c.JSON(http.StatusOK, layout)
}

// UpdateDashboardLayout saves a new dashboard layout (PUT /campaigns/:id/dashboard-layout).
// Supports ?role= query param (default/player/scribe) to update a specific role's layout.
func (h *Handler) UpdateDashboardLayout(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	roleName := c.QueryParam("role")
	if roleName == "" {
		roleName = "default"
	}

	var layout DashboardLayout
	if err := json.NewDecoder(c.Request().Body).Decode(&layout); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := validateDashboardLayout(&layout); err != nil {
		return err
	}

	// Fetch campaign, merge this role's layout into the wrapper, and save.
	campaign, err := h.service.GetByID(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	fullJSON, err := campaign.SetRoleDashboardJSON(roleName, &layout)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling role layout: %w", err))
	}

	if err := h.service.UpdateDashboardLayoutRaw(c.Request().Context(), cc.Campaign.ID, fullJSON); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "dashboard_layout_updated", map[string]any{"role": roleName})
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ResetDashboardLayout removes the custom dashboard layout (DELETE /campaigns/:id/dashboard-layout).
// Supports ?role= query param. If "default", resets entire layout. If "player"/"scribe",
// removes only that role's override.
func (h *Handler) ResetDashboardLayout(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	roleName := c.QueryParam("role")
	if roleName == "" {
		roleName = "default"
	}

	// For "default" reset with no role param, clear the entire column.
	if roleName == "default" {
		if err := h.service.ResetDashboardLayout(c.Request().Context(), cc.Campaign.ID); err != nil {
			return err
		}
	} else {
		// Remove just this role's layout from the wrapper.
		campaign, err := h.service.GetByID(c.Request().Context(), cc.Campaign.ID)
		if err != nil {
			return err
		}
		fullJSON, err := campaign.RemoveRoleDashboardJSON(roleName)
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("removing role layout: %w", err))
		}
		if err := h.service.UpdateDashboardLayoutRaw(c.Request().Context(), cc.Campaign.ID, fullJSON); err != nil {
			return err
		}
	}

	h.logAudit(c, cc.Campaign.ID, "dashboard_layout_reset", map[string]any{"role": roleName})
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Owner Dashboard ---

// OwnerDashboard renders the owner-only management dashboard (GET /campaigns/:id/dashboard).
func (h *Handler) OwnerDashboard(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var recentEntities []RecentEntity
	if h.recentLister != nil {
		recentEntities, _ = h.recentLister.ListRecentForDashboard(
			c.Request().Context(), cc.Campaign.ID, int(cc.MemberRole), auth.GetUserID(c), 8,
		)
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, OwnerDashboardPage(cc, recentEntities, csrfToken))
}

// GetOwnerDashboardLayout returns the owner dashboard layout JSON (GET /campaigns/:id/owner-dashboard-layout).
func (h *Handler) GetOwnerDashboardLayout(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	layout, err := h.service.GetOwnerDashboardLayout(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, layout)
}

// UpdateOwnerDashboardLayout saves the owner dashboard layout (PUT /campaigns/:id/owner-dashboard-layout).
func (h *Handler) UpdateOwnerDashboardLayout(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var layout DashboardLayout
	if err := json.NewDecoder(c.Request().Body).Decode(&layout); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.UpdateOwnerDashboardLayout(c.Request().Context(), cc.Campaign.ID, &layout); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "owner_dashboard_layout_updated", nil)
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ResetOwnerDashboardLayout removes the custom owner dashboard layout (DELETE /campaigns/:id/owner-dashboard-layout).
func (h *Handler) ResetOwnerDashboardLayout(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if err := h.service.ResetOwnerDashboardLayout(c.Request().Context(), cc.Campaign.ID); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "owner_dashboard_layout_reset", nil)
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- View As Player Toggle ---

// viewAsPlayerCookie is the cookie name for the "view as player" display toggle.
const viewAsPlayerCookie = "chronicle_view_as_player"

// ToggleViewAsPlayer toggles the "view as player" cookie for campaign owners
// (POST /campaigns/:id/toggle-view-mode). When active, templates render as
// if the owner has the Player role -- hiding owner-only UI and private entities.
// This is a display-only toggle; the owner retains actual ownership for access control.
func (h *Handler) ToggleViewAsPlayer(c echo.Context) error {
	// Read current cookie state and toggle.
	viewing := false
	if cookie, err := c.Cookie(viewAsPlayerCookie); err == nil && cookie.Value == "1" {
		viewing = true
	}

	req := c.Request()
	secure := req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https"

	if viewing {
		// Clear the cookie.
		c.SetCookie(&http.Cookie{
			Name:     viewAsPlayerCookie,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   secure,
			MaxAge:   -1,
		})
	} else {
		// Set the cookie.
		c.SetCookie(&http.Cookie{
			Name:     viewAsPlayerCookie,
			Value:    "1",
			Path:     "/",
			HttpOnly: false, // JS reads this for toggle state indicator.
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   30 * 24 * 60 * 60, // 30 days.
		})
	}

	// HTMX: tell the client to do a full page refresh to re-render everything.
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Refresh", "true")
		return c.NoContent(http.StatusNoContent)
	}

	// Regular request: redirect back to the current page.
	referer := req.Header.Get("Referer")
	if referer == "" {
		cc := GetCampaignContext(c)
		if cc != nil {
			referer = "/campaigns/" + cc.Campaign.ID
		} else {
			referer = "/campaigns"
		}
	}
	return c.Redirect(http.StatusSeeOther, referer)
}

// --- Members ---

// Members returns the member list as JSON for API consumers (Alpine.js pickers)
// or redirects HTML requests to the settings People tab.
func (h *Handler) Members(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	// Return JSON for API consumers (Alpine.js widgets, DM Privileges).
	if c.Request().Header.Get("Accept") == "application/json" {
		members, err := h.service.ListMembers(c.Request().Context(), cc.Campaign.ID)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, members)
	}

	// Redirect HTML requests to the unified settings page.
	return c.Redirect(http.StatusFound, fmt.Sprintf("/campaigns/%s/settings?tab=people", cc.Campaign.ID))
}

// AddMember adds a user to the campaign (POST /campaigns/:id/members).
func (h *Handler) AddMember(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req AddMemberRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	role := RoleFromString(req.Role)
	if err := h.service.AddMember(c.Request().Context(), cc.Campaign.ID, req.Email, role); err != nil {
		// Re-render with error message.
		members, _ := h.service.ListMembers(c.Request().Context(), cc.Campaign.ID)
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := apperror.UserMessage(err, "failed to add member")
		return middleware.Render(c, http.StatusOK, CampaignMembersPage(cc, members, csrfToken, errMsg))
	}

	h.logAudit(c, cc.Campaign.ID, "member.joined", map[string]any{
		"email": req.Email,
		"role":  req.Role,
	})

	// Refresh the member list.
	members, _ := h.service.ListMembers(c.Request().Context(), cc.Campaign.ID)
	csrfToken := middleware.GetCSRFToken(c)

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, MemberListComponent(cc, members, csrfToken, ""))
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/members")
}

// RemoveMember removes a user from the campaign (DELETE /campaigns/:id/members/:uid).
func (h *Handler) RemoveMember(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	targetUserID := c.Param("uid")
	if err := h.service.RemoveMember(c.Request().Context(), cc.Campaign.ID, targetUserID); err != nil {
		members, _ := h.service.ListMembers(c.Request().Context(), cc.Campaign.ID)
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := apperror.UserMessage(err, "failed to remove member")
		return middleware.Render(c, http.StatusOK, MemberListComponent(cc, members, csrfToken, errMsg))
	}

	h.logAudit(c, cc.Campaign.ID, "member.left", map[string]any{
		"target_user_id": targetUserID,
	})

	members, _ := h.service.ListMembers(c.Request().Context(), cc.Campaign.ID)
	csrfToken := middleware.GetCSRFToken(c)

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, MemberListComponent(cc, members, csrfToken, ""))
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/members")
}

// UpdateRole changes a member's role (PUT /campaigns/:id/members/:uid/role).
func (h *Handler) UpdateRole(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	targetUserID := c.Param("uid")
	var req UpdateRoleRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	role := RoleFromString(req.Role)
	if err := h.service.UpdateMemberRole(c.Request().Context(), cc.Campaign.ID, targetUserID, role); err != nil {
		members, _ := h.service.ListMembers(c.Request().Context(), cc.Campaign.ID)
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := apperror.UserMessage(err, "failed to update role")
		return middleware.Render(c, http.StatusOK, MemberListComponent(cc, members, csrfToken, errMsg))
	}

	h.logAudit(c, cc.Campaign.ID, "member.role_changed", map[string]any{
		"target_user_id": targetUserID,
		"new_role":       req.Role,
	})

	members, _ := h.service.ListMembers(c.Request().Context(), cc.Campaign.ID)
	csrfToken := middleware.GetCSRFToken(c)

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, MemberListComponent(cc, members, csrfToken, ""))
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/members")
}

// UpdateMemberCharacterAPI sets a member's character entity assignment.
// PUT /campaigns/:id/members/:uid/character
func (h *Handler) UpdateMemberCharacterAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	targetUserID := c.Param("uid")
	var req struct {
		CharacterEntityID *string `json:"character_entity_id"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if err := h.service.UpdateMemberCharacter(c.Request().Context(), cc.Campaign.ID, targetUserID, req.CharacterEntityID); err != nil {
		return c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.UserMessage(err, "failed to update character")})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Ownership Transfer ---

// TransferForm renders the ownership transfer form (GET /campaigns/:id/transfer).
func (h *Handler) TransferForm(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	transfer, _ := h.service.GetPendingTransfer(c.Request().Context(), cc.Campaign.ID)
	csrfToken := middleware.GetCSRFToken(c)

	var entityTypes []SettingsEntityType
	if h.entityLister != nil {
		entityTypes, _ = h.entityLister.GetEntityTypesForSettings(c.Request().Context(), cc.Campaign.ID)
	}

	return middleware.Render(c, http.StatusOK, CampaignSettingsPage(cc, transfer, entityTypes, nil, csrfToken, "", h.baseURL, "general", false, nil))
}

// Transfer initiates an ownership transfer (POST /campaigns/:id/transfer).
func (h *Handler) Transfer(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req TransferOwnershipRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	userID := auth.GetUserID(c)
	_, err := h.service.InitiateTransfer(c.Request().Context(), cc.Campaign.ID, userID, req.Email)
	if err != nil {
		transfer, _ := h.service.GetPendingTransfer(c.Request().Context(), cc.Campaign.ID)
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := apperror.UserMessage(err, "failed to initiate transfer")
		var entityTypes []SettingsEntityType
		if h.entityLister != nil {
			entityTypes, _ = h.entityLister.GetEntityTypesForSettings(c.Request().Context(), cc.Campaign.ID)
		}
		return middleware.Render(c, http.StatusOK, CampaignSettingsPage(cc, transfer, entityTypes, nil, csrfToken, errMsg, h.baseURL, "general", false, nil))
	}

	return middleware.HTMXRedirect(c, "/campaigns/"+cc.Campaign.ID+"/settings")
}

// AcceptTransfer accepts a pending ownership transfer (GET /campaigns/:id/accept-transfer).
func (h *Handler) AcceptTransfer(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return apperror.NewBadRequest("transfer token is required")
	}

	userID := auth.GetUserID(c)
	if err := h.service.AcceptTransfer(c.Request().Context(), token, userID); err != nil {
		return err
	}

	campaignID := c.Param("id")
	return middleware.HTMXRedirect(c, "/campaigns/"+campaignID)
}

// CancelTransfer cancels a pending ownership transfer (POST /campaigns/:id/cancel-transfer).
func (h *Handler) CancelTransfer(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if err := h.service.CancelTransfer(c.Request().Context(), cc.Campaign.ID); err != nil {
		return err
	}

	return middleware.HTMXRedirect(c, "/campaigns/"+cc.Campaign.ID+"/settings")
}

// --- Campaign Group Management ---

// listGroupsResponse wraps the group list for JSON responses.
type listGroupsResponse struct {
	Groups []CampaignGroup `json:"groups"`
}

// createGroupRequest is the JSON payload for creating a group.
type createGroupRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// updateGroupRequest is the JSON payload for updating a group.
type updateGroupRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// addGroupMemberRequest is the JSON payload for adding a member to a group.
type addGroupMemberRequest struct {
	UserID string `json:"user_id"`
}

// ListGroupsAPI returns all groups for a campaign (GET /campaigns/:id/groups).
func (h *Handler) ListGroupsAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	groups, err := h.groupSvc.ListGroups(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, listGroupsResponse{Groups: groups})
}

// CreateGroupAPI creates a new campaign group (POST /campaigns/:id/groups).
func (h *Handler) CreateGroupAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req createGroupRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	group, err := h.groupSvc.CreateGroup(c.Request().Context(), cc.Campaign.ID, req.Name, req.Description)
	if err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "group.created", map[string]any{
		"group_id":   group.ID,
		"group_name": group.Name,
	})

	return c.JSON(http.StatusCreated, group)
}

// GetGroupAPI returns a single group with its members (GET /campaigns/:id/groups/:gid).
func (h *Handler) GetGroupAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	gid, err := strconv.Atoi(c.Param("gid"))
	if err != nil {
		return apperror.NewBadRequest("invalid group ID")
	}

	group, err := h.groupSvc.GetGroup(c.Request().Context(), gid)
	if err != nil {
		return err
	}

	// Verify group belongs to this campaign.
	if group.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("group not found")
	}

	members, err := h.groupSvc.ListGroupMembers(c.Request().Context(), gid)
	if err != nil {
		return err
	}
	group.Members = members

	return c.JSON(http.StatusOK, group)
}

// UpdateGroupAPI updates a campaign group (PUT /campaigns/:id/groups/:gid).
func (h *Handler) UpdateGroupAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	gid, err := strconv.Atoi(c.Param("gid"))
	if err != nil {
		return apperror.NewBadRequest("invalid group ID")
	}

	// Verify group belongs to this campaign.
	group, err := h.groupSvc.GetGroup(c.Request().Context(), gid)
	if err != nil {
		return err
	}
	if group.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("group not found")
	}

	var req updateGroupRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.groupSvc.UpdateGroup(c.Request().Context(), gid, req.Name, req.Description); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "group.updated", map[string]any{
		"group_id":   gid,
		"group_name": req.Name,
	})

	return c.NoContent(http.StatusNoContent)
}

// DeleteGroupAPI deletes a campaign group (DELETE /campaigns/:id/groups/:gid).
func (h *Handler) DeleteGroupAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	gid, err := strconv.Atoi(c.Param("gid"))
	if err != nil {
		return apperror.NewBadRequest("invalid group ID")
	}

	// Verify group belongs to this campaign.
	group, err := h.groupSvc.GetGroup(c.Request().Context(), gid)
	if err != nil {
		return err
	}
	if group.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("group not found")
	}

	if err := h.groupSvc.DeleteGroup(c.Request().Context(), gid); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "group.deleted", map[string]any{
		"group_id":   gid,
		"group_name": group.Name,
	})

	return c.NoContent(http.StatusNoContent)
}

// AddGroupMemberAPI adds a user to a campaign group (POST /campaigns/:id/groups/:gid/members).
func (h *Handler) AddGroupMemberAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	gid, err := strconv.Atoi(c.Param("gid"))
	if err != nil {
		return apperror.NewBadRequest("invalid group ID")
	}

	// Verify group belongs to this campaign.
	group, err := h.groupSvc.GetGroup(c.Request().Context(), gid)
	if err != nil {
		return err
	}
	if group.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("group not found")
	}

	var req addGroupMemberRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.groupSvc.AddGroupMember(c.Request().Context(), gid, req.UserID); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "group.member_added", map[string]any{
		"group_id": gid,
		"user_id":  req.UserID,
	})

	// Return updated member list.
	members, err := h.groupSvc.ListGroupMembers(c.Request().Context(), gid)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{"members": members})
}

// RemoveGroupMemberAPI removes a user from a campaign group (DELETE /campaigns/:id/groups/:gid/members/:uid).
func (h *Handler) RemoveGroupMemberAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	gid, err := strconv.Atoi(c.Param("gid"))
	if err != nil {
		return apperror.NewBadRequest("invalid group ID")
	}

	// Verify group belongs to this campaign.
	group, err := h.groupSvc.GetGroup(c.Request().Context(), gid)
	if err != nil {
		return err
	}
	if group.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("group not found")
	}

	targetUID := c.Param("uid")
	if err := h.groupSvc.RemoveGroupMember(c.Request().Context(), gid, targetUID); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "group.member_removed", map[string]any{
		"group_id": gid,
		"user_id":  targetUID,
	})

	return c.NoContent(http.StatusNoContent)
}

// --- Archive & Join Code ---

// ArchiveCampaign soft-archives a campaign (POST /campaigns/:id/archive).
func (h *Handler) ArchiveCampaign(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if err := h.service.ArchiveCampaign(c.Request().Context(), cc.Campaign.ID); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.archived", nil)
	return c.JSON(http.StatusOK, map[string]string{"status": "archived"})
}

// UnarchiveCampaign restores a soft-archived campaign (POST /campaigns/:id/unarchive).
func (h *Handler) UnarchiveCampaign(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if err := h.service.UnarchiveCampaign(c.Request().Context(), cc.Campaign.ID); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.unarchived", nil)
	return c.JSON(http.StatusOK, map[string]string{"status": "active"})
}

// GenerateJoinCode creates a shareable invite code (POST /campaigns/:id/join-code).
func (h *Handler) GenerateJoinCode(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	code, err := h.service.GenerateJoinCode(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	joinURL := fmt.Sprintf("%s/join/%s", h.baseURL, code)
	h.logAudit(c, cc.Campaign.ID, "campaign.join_code.generated", nil)
	return c.JSON(http.StatusOK, map[string]string{"code": code, "url": joinURL})
}

// RevokeJoinCode removes the shareable invite code (DELETE /campaigns/:id/join-code).
func (h *Handler) RevokeJoinCode(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if err := h.service.RevokeJoinCode(c.Request().Context(), cc.Campaign.ID); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.join_code.revoked", nil)
	return c.NoContent(http.StatusNoContent)
}

// JoinByCode handles a user joining a campaign via invite code (GET /join/:code).
func (h *Handler) JoinByCode(c echo.Context) error {
	code := c.Param("code")
	if code == "" {
		return apperror.NewBadRequest("invite code is required")
	}

	userID := auth.GetUserID(c)
	if userID == "" {
		// Redirect to login, preserving the join URL.
		return c.Redirect(http.StatusFound, fmt.Sprintf("/login?next=/join/%s", code))
	}

	if err := h.service.JoinByCode(c.Request().Context(), code, userID); err != nil {
		return err
	}

	// Redirect to the campaigns list; the user is now a member.
	return c.Redirect(http.StatusFound, "/campaigns")
}

// UpdateSystemID sets the game system for a campaign (PUT /campaigns/:id/system).
func (h *Handler) UpdateSystemID(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var req struct {
		SystemID string `json:"system_id"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.service.UpdateSystemID(c.Request().Context(), cc.Campaign.ID, req.SystemID); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.system.updated", map[string]any{"system_id": req.SystemID})
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// GroupsPage renders the groups management page (GET /campaigns/:id/groups/manage).
func (h *Handler) GroupsPage(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	groups, err := h.groupSvc.ListGroups(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	members, err := h.service.ListMembers(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)

	return middleware.Render(c, http.StatusOK, GroupsManagePage(cc, groups, members, csrfToken))
}
