package app

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// lastCampaignCookie remembers, per device, the campaign a member was last
// in, so Account settings and Site admin can offer the way straight back. It
// holds only an id; the name is read again, and only for a current member.
const lastCampaignCookie = "chronicle_last_campaign"

// lastCampaignTTL keeps the way back for a season of play.
const lastCampaignTTL = 90 * 24 * time.Hour

// rememberCampaign notes the campaign a member is viewing. Only page loads
// set it, and only when it changes, so most requests send no cookie.
func rememberCampaign(c *echo.Context, cc *campaigns.CampaignContext) {
	req := c.Request()
	if req.Method != http.MethodGet || cc == nil || !cc.IsMember || middleware.IsHTMX(c) {
		return
	}
	if ck, err := req.Cookie(lastCampaignCookie); err == nil && ck.Value == cc.Campaign.ID {
		return
	}
	c.SetCookie(&http.Cookie{
		Name:     lastCampaignCookie,
		Value:    cc.Campaign.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   middleware.SchemeIsSecure(req),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(lastCampaignTTL.Seconds()),
	})
}

// campaignMembers is what returnCampaign needs from the campaigns service.
type campaignMembers interface {
	GetMember(ctx context.Context, campaignID, userID string) (*campaigns.CampaignMember, error)
	GetByID(ctx context.Context, id string) (*campaigns.Campaign, error)
}

// returnCampaign is the remembered campaign to offer as the way back, or nil.
// It is offered only while the person is still a member, so a campaign they
// left never shows its name. listed is the campaigns already read for the
// header, which usually saves the lookup.
func returnCampaign(ctx context.Context, svc campaignMembers, id, userID string, listed []layouts.NavCampaign) *layouts.NavCampaign {
	if id == "" || len(id) > 64 || userID == "" {
		return nil
	}
	for _, nc := range listed {
		if nc.ID == id {
			return &layouts.NavCampaign{ID: nc.ID, Name: nc.Name}
		}
	}
	if m, err := svc.GetMember(ctx, id, userID); err != nil || m == nil {
		return nil
	}
	camp, err := svc.GetByID(ctx, id)
	if err != nil || camp == nil {
		return nil
	}
	return &layouts.NavCampaign{ID: camp.ID, Name: camp.Name}
}
