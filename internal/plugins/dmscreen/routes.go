package dmscreen

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the DM Screen routes. Both need Scribe or above; the
// service checks the role again so a wiring mistake can't open it to players.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	cg.GET("/dm-screen", h.Show, campaigns.RequireRole(campaigns.RoleScribe))
	cg.POST("/dm-screen/reveal/:eid", h.Reveal, campaigns.RequireRole(campaigns.RoleScribe))
}
