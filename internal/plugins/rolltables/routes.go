// Route registration for roll tables.
package rolltables

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the browser routes under the campaign group. The
// tables are a DM's prep and may spoil outcomes, so players never read them;
// scribes, who can edit pages that hold a roller, may read and roll them,
// and only the DM team may change them. CSRF is enforced by the global
// middleware.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	dmTeam := campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool {
		return cc.CanAuthorDmOnly()
	}, "only the campaign owner or a member with DM access may do this")

	scribeOrDM := campaigns.RequireCapability(canRead, readDenyMsg)

	cg.GET("/roll-tables", h.Get, scribeOrDM)
	cg.PUT("/roll-tables", h.Put, dmTeam)
}
