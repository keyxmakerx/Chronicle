package campaigns

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

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

// SettingsEntityType is a minimal entity type representation for the settings page.
type SettingsEntityType struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	NamePlural string `json:"name_plural"`
	Icon       string `json:"icon"`
	Color      string `json:"color"`
}

// Handler handles HTTP requests for campaign operations. Handlers are thin:
// bind request, call service, render response. No business logic lives here.
type Handler struct {
	service      CampaignService
	entityLister EntityTypeLister
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
			return middleware.Render(c, http.StatusOK, CampaignFormComponent(csrfToken, nil, &req, errMsg))
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
		return apperror.NewInternal(nil)
	}

	// Check for pending transfer to show banner.
	transfer, _ := h.service.GetPendingTransfer(c.Request().Context(), cc.Campaign.ID)

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, CampaignShowPage(cc, transfer, csrfToken))
}

// EditForm renders the campaign edit form (GET /campaigns/:id/edit).
func (h *Handler) EditForm(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, CampaignEditPage(cc.Campaign, csrfToken, ""))
}

// Update processes the campaign edit form (PUT /campaigns/:id).
func (h *Handler) Update(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
	}

	var req UpdateCampaignRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	input := UpdateCampaignInput{
		Name:        req.Name,
		Description: req.Description,
	}

	_, err := h.service.Update(c.Request().Context(), cc.Campaign.ID, input)
	if err != nil {
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := "failed to update campaign"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}
		return middleware.Render(c, http.StatusOK, CampaignEditPage(cc.Campaign, csrfToken, errMsg))
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/campaigns/"+cc.Campaign.ID)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID)
}

// Delete removes a campaign (DELETE /campaigns/:id).
func (h *Handler) Delete(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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

// --- Sidebar Config API ---

// GetSidebarConfig returns the sidebar configuration as JSON (GET /campaigns/:id/sidebar-config).
func (h *Handler) GetSidebarConfig(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
	}

	var req UpdateSidebarConfigRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	config := SidebarConfig{
		EntityTypeOrder: req.EntityTypeOrder,
		HiddenTypeIDs:   req.HiddenTypeIDs,
	}

	if err := h.service.UpdateSidebarConfig(c.Request().Context(), cc.Campaign.ID, config); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Members ---

// Members renders the member list page (GET /campaigns/:id/members).
func (h *Handler) Members(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
		return apperror.NewInternal(nil)
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
