// link_code_handler.go — the player's "Link to Foundry" line on the page
// they claimed, and the GM's Foundry client redeeming the code typed in
// Foundry. See link_codes.go.

package syncapi

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SetLinkCodes wires the link-code store. Without it the line never shows
// and the redeem endpoint answers 404.
func (h *SyncHistoryHandler) SetLinkCodes(codes *LinkCodes) {
	h.linkCodes = codes
}

// LinkCodeView is what the "Link to Foundry" line renders.
type LinkCodeView struct {
	CampaignID string
	CSRFToken  string
	// LinkedAs is the Foundry user's name once linked; the line then only
	// says so.
	LinkedAs string
	// Code is the formatted code after a press; empty before one.
	Code    string
	Expires time.Time
}

// linkViewer is who may link themselves: a signed-in member who is not the
// owner (the owner is Foundry's GM, whom sync never links) and not the DM
// team previewing as someone else.
func linkViewer(c *echo.Context, cc *campaigns.CampaignContext) string {
	if cc == nil || !cc.IsMember || cc.Preview != "" || cc.MemberRole >= campaigns.RoleOwner {
		return ""
	}
	return auth.GetUserID(c)
}

// linkState reports whether a Foundry world has reported players for this
// campaign, and the Foundry name the member is linked to, if any.
func (h *SyncHistoryHandler) linkState(ctx context.Context, campaignID, userID string) (bool, string, error) {
	players, err := h.players.List(ctx, campaignID)
	if err != nil {
		return false, "", err
	}
	for _, p := range players {
		if p.MemberUserID == userID {
			return true, p.Name, nil
		}
	}
	return len(players) > 0, "", nil
}

// LinkFragment serves the line for the page's claimant. It renders nothing
// until the campaign's Foundry world has reported its players, so a
// campaign without Foundry never shows it.
// GET /campaigns/:id/foundry/link
func (h *SyncHistoryHandler) LinkFragment(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := linkViewer(c, cc)
	if h.linkCodes == nil || h.players == nil || userID == "" {
		return c.NoContent(http.StatusOK)
	}
	world, linkedAs, err := h.linkState(c.Request().Context(), cc.Campaign.ID, userID)
	if err != nil {
		return err
	}
	if !world {
		return c.NoContent(http.StatusOK)
	}
	return middleware.Render(c, http.StatusOK, FoundryLinkLine(LinkCodeView{
		CampaignID: cc.Campaign.ID,
		CSRFToken:  middleware.GetCSRFToken(c),
		LinkedAs:   linkedAs,
	}))
}

// IssueLinkCode gives the signed-in member a fresh code. Only they see it:
// it is made for the session's own user and returned in this response only.
// POST /campaigns/:id/foundry/link
func (h *SyncHistoryHandler) IssueLinkCode(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := linkViewer(c, cc)
	if h.linkCodes == nil || h.players == nil {
		return apperror.NewNotFound("linking to Foundry is not available")
	}
	if userID == "" {
		return apperror.NewForbidden("only a player of this campaign can link to Foundry")
	}
	code, expires, err := h.linkCodes.Issue(cc.Campaign.ID, userID)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, FoundryLinkLine(LinkCodeView{
		CampaignID: cc.Campaign.ID,
		CSRFToken:  middleware.GetCSRFToken(c),
		Code:       FormatLinkCode(code),
		Expires:    expires,
	}))
}

type redeemLinkCodeRequest struct {
	Code string `json:"code"`
}

type redeemLinkCodeResponse struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
}

// RedeemLinkCode ends a code and names the member it was made for. Only an
// owner or DM-access key may redeem: that is the GM's Foundry client, which
// links the Foundry user who typed the code. A player's client holds no key.
// POST /api/v1/campaigns/:id/sync/link-code
func (h *SyncHistoryHandler) RedeemLinkCode(c *echo.Context) error {
	if h.linkCodes == nil {
		return apperror.NewNotFound("link codes are not available")
	}
	if !h.isDMEquivalent(c) {
		return apperror.NewForbidden("redeeming a link code requires owner or DM access")
	}
	var req redeemLinkCodeRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	ctx := c.Request().Context()
	campaignID := c.Param("id")
	userID, err := h.linkCodes.Redeem(campaignID, req.Code)
	if err != nil {
		return err
	}
	// A member who left after asking for the code links no one.
	members, err := h.campaignSvc.ListMembers(ctx, campaignID)
	if err != nil {
		return err
	}
	for _, m := range members {
		if m.UserID != userID || m.Role >= campaigns.RoleOwner {
			continue
		}
		name := m.DisplayName
		if name == "" {
			name = m.Email
		}
		return c.JSON(http.StatusOK, redeemLinkCodeResponse{UserID: userID, DisplayName: name})
	}
	return apperror.NewNotFound("that link code is wrong or has expired")
}
