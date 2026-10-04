package dmscreen

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// CanOpen reports who may open the DM Screen: scribes and the owner, and any
// member the owner has given DM access, since that access is for running the
// game. The top-bar button uses the same rule.
func CanOpen(cc *campaigns.CampaignContext) bool {
	return cc.MemberRole >= campaigns.RoleScribe || cc.IsDmGranted
}

// RegisterRoutes mounts the DM Screen routes behind CanOpen; the service
// checks the role again so a wiring mistake can't open it to players.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	cg.GET("/dm-screen", h.Show, campaigns.RequireCapability(CanOpen, "the DM Screen is for the people running this campaign"))
	cg.POST("/dm-screen/reveal/:eid", h.Reveal, campaigns.RequireCapability(CanOpen, "the DM Screen is for the people running this campaign"))
}
