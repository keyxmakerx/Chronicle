package campaigns

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
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
	ListRecentForDashboard(ctx context.Context, campaignID string, role int, limit int) ([]RecentEntity, error)
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

// Handler handles HTTP requests for campaign operations. Handlers are thin:
// bind request, call service, render response. No business logic lives here.
type Handler struct {
	service       CampaignService
	entityLister  EntityTypeLister
	layoutFetcher EntityTypeLayoutFetcher
	recentLister  RecentEntityLister
	auditLogger   AuditLogger
}

// NewHandler creates a new campaign handler.
func NewHandler(service CampaignService) *Handler {
	return &Handler{service: service}
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

	userID := auth.GetUserID(c)
	input := CreateCampaignInput{
		Name:        req.Name,
		Description: req.Description,
	}

	campaign, err := h.service.Create(c.Request().Context(), userID, input)
	if err != nil {
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := "failed to create campaign"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}
		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, CampaignCreateForm(csrfToken, &req, errMsg))
		}
		return middleware.Render(c, http.StatusOK, CampaignNewPage(csrfToken, req.Name, errMsg))
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/campaigns/"+campaign.ID)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+campaign.ID)
}

// Show renders the campaign dashboard (GET /campaigns/:id).
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
			c.Request().Context(), cc.Campaign.ID, int(cc.MemberRole), 8,
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

	input := UpdateCampaignInput{
		Name:        req.Name,
		Description: req.Description,
		IsPublic:    req.IsPublic,
	}

	_, err := h.service.Update(c.Request().Context(), cc.Campaign.ID, input)
	if err != nil {
		errMsg := "failed to update campaign"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}
		csrfToken := middleware.GetCSRFToken(c)
		transfer, _ := h.service.GetPendingTransfer(c.Request().Context(), cc.Campaign.ID)
		var entityTypes []SettingsEntityType
		if h.entityLister != nil {
			entityTypes, _ = h.entityLister.GetEntityTypesForSettings(c.Request().Context(), cc.Campaign.ID)
		}
		return middleware.Render(c, http.StatusOK, CampaignSettingsPage(cc, transfer, entityTypes, csrfToken, errMsg))
	}

	h.logAudit(c, cc.Campaign.ID, "campaign.updated", nil)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/campaigns/"+cc.Campaign.ID+"/settings")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/settings")
}

// Delete removes a campaign (DELETE /campaigns/:id).
func (h *Handler) Delete(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if err := h.service.Delete(c.Request().Context(), cc.Campaign.ID); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/campaigns")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns")
}

// --- Settings ---

// Settings renders the campaign settings page (GET /campaigns/:id/settings).
func (h *Handler) Settings(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	transfer, _ := h.service.GetPendingTransfer(c.Request().Context(), cc.Campaign.ID)
	csrfToken := middleware.GetCSRFToken(c)

	// Fetch entity types for sidebar config widget.
	var entityTypes []SettingsEntityType
	if h.entityLister != nil {
		entityTypes, _ = h.entityLister.GetEntityTypesForSettings(c.Request().Context(), cc.Campaign.ID)
	}

	return middleware.Render(c, http.StatusOK, CampaignSettingsPage(cc, transfer, entityTypes, csrfToken, ""))
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

// --- Dashboard Layout ---

// GetDashboardLayout returns the current dashboard layout as JSON (GET /campaigns/:id/dashboard-layout).
// Returns null if no custom layout is set (meaning the default is in use).
func (h *Handler) GetDashboardLayout(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	layout, err := h.service.GetDashboardLayout(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, layout)
}

// UpdateDashboardLayout saves a new dashboard layout (PUT /campaigns/:id/dashboard-layout).
func (h *Handler) UpdateDashboardLayout(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var layout DashboardLayout
	if err := json.NewDecoder(c.Request().Body).Decode(&layout); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.UpdateDashboardLayout(c.Request().Context(), cc.Campaign.ID, &layout); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "dashboard_layout_updated", nil)
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ResetDashboardLayout removes the custom dashboard layout (DELETE /campaigns/:id/dashboard-layout).
func (h *Handler) ResetDashboardLayout(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	if err := h.service.ResetDashboardLayout(c.Request().Context(), cc.Campaign.ID); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "dashboard_layout_reset", nil)
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

// Members renders the member list page (GET /campaigns/:id/members).
func (h *Handler) Members(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	members, err := h.service.ListMembers(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, CampaignMembersPage(cc, members, csrfToken, ""))
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
		errMsg := "failed to add member"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}
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
		errMsg := "failed to remove member"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}
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
		errMsg := "failed to update role"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}
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

	return middleware.Render(c, http.StatusOK, CampaignSettingsPage(cc, transfer, entityTypes, csrfToken, ""))
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
		errMsg := "failed to initiate transfer"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}
		var entityTypes []SettingsEntityType
		if h.entityLister != nil {
			entityTypes, _ = h.entityLister.GetEntityTypesForSettings(c.Request().Context(), cc.Campaign.ID)
		}
		return middleware.Render(c, http.StatusOK, CampaignSettingsPage(cc, transfer, entityTypes, csrfToken, errMsg))
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/campaigns/"+cc.Campaign.ID+"/settings")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/settings")
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
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/campaigns/"+campaignID)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+campaignID)
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

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/campaigns/"+cc.Campaign.ID+"/settings")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/settings")
}
