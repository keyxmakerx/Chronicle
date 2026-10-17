package campaigns

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// InviteHandler handles HTTP requests for campaign invites.
type InviteHandler struct {
	service   InviteService
	campaigns CampaignService
	baseURL   string
}

// NewInviteHandler creates a new invite handler.
func NewInviteHandler(service InviteService, campaigns CampaignService, baseURL string) *InviteHandler {
	return &InviteHandler{
		service:   service,
		campaigns: campaigns,
		baseURL:   baseURL,
	}
}

// ListInvitesAPI returns all invites for a campaign as JSON.
// GET /campaigns/:id/invites
func (h *InviteHandler) ListInvitesAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	invites, err := h.service.ListInvites(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if invites == nil {
		invites = []Invite{}
	}
	return c.JSON(http.StatusOK, invites)
}

// CreateInviteAPI creates a new campaign invite.
// POST /campaigns/:id/invites
func (h *InviteHandler) CreateInviteAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var input CreateInviteInput
	if err := c.Bind(&input); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	userID := auth.GetUserID(c)
	invite, err := h.service.CreateInvite(c.Request().Context(), cc.Campaign.ID, userID, input)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, invite)
}

// RevokeInviteAPI deletes a pending invite.
// DELETE /campaigns/:id/invites/:inviteId
func (h *InviteHandler) RevokeInviteAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	inviteID := c.Param("inviteId")
	if inviteID == "" {
		return apperror.NewBadRequest("invite ID required")
	}

	if err := h.service.RevokeInvite(c.Request().Context(), cc.Campaign.ID, inviteID); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// AcceptInvitePage handles invite acceptance.
// GET /invites/accept?token=xxx
func (h *InviteHandler) AcceptInvitePage(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return apperror.NewBadRequest("missing invite token")
	}

	// Fetch the invite to show details.
	invite, err := h.service.GetInviteByToken(c.Request().Context(), token)
	if err != nil {
		return err
	}

	// Fetch campaign name for display.
	campaign, _ := h.campaigns.GetByID(c.Request().Context(), invite.CampaignID)

	// Check if user is logged in.
	userID := auth.GetUserID(c)
	if userID == "" {
		// Not logged in — show the invite details and prompt to log in or register.
		campaignName := ""
		if campaign != nil {
			campaignName = campaign.Name
		}
		return middleware.Render(c, http.StatusOK, InviteAcceptPage(invite, campaignName, token, false, ""))
	}

	// User is logged in — accept the invite.
	accepted, err := h.service.AcceptInvite(c.Request().Context(), token, userID)
	if err != nil {
		// If it's a validation error (already member, expired), show it.
		campaignName := ""
		if campaign != nil {
			campaignName = campaign.Name
		}
		return middleware.Render(c, http.StatusOK, InviteAcceptPage(invite, campaignName, token, false, err.Error()))
	}

	// Success — redirect to the campaign.
	campaignName := ""
	if campaign != nil {
		campaignName = campaign.Name
	}
	_ = accepted
	return middleware.Render(c, http.StatusOK, InviteAcceptPage(invite, campaignName, token, true, ""))
}

// InvitesPage renders the invite management tab in campaign settings.
// GET /campaigns/:id/invites/page
func (h *InviteHandler) InvitesPage(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	invites, err := h.service.ListInvites(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return apperror.NewInternal(err)
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, InviteListFragment(cc, invites))
	}
	return middleware.Render(c, http.StatusOK, InviteListFragment(cc, invites))
}
