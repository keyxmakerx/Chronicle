// Route registration for system state.
package systemstate

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the browser routes under the campaign group. Reads are
// open to every member who can view the page, or to every member for a
// campaign-wide document (the handler withholds the gm half unless the caller
// may author DM-only content); writes are DM-team only.
// CSRF is enforced by the global middleware.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	dmTeam := campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool {
		return cc.CanAuthorDmOnly()
	}, "only the campaign owner or a member with DM access may do this")

	cg.GET("/system-state/:system/:key", h.List, dmTeam)
	cg.GET("/entities/:eid/system-state/:system/:key", h.Get)
	cg.PUT("/entities/:eid/system-state/:system/:key", h.Put, dmTeam)
	cg.GET("/campaign-system-state/:system/:key", h.GetCampaign)
	cg.PUT("/campaign-system-state/:system/:key", h.PutCampaign, dmTeam)
}
