// Route registration for page blocks.
package pageblocks

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the routes under the campaign group. Reads follow the
// page: anyone who can view it (a public-campaign guest included) may read,
// and the service hides what the viewer must not see. Writes need a signed-in
// member; the service then applies the page's own edit rule. CSRF is enforced
// by the global middleware.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/page-blocks/:eid/:bid", h.Get, campaigns.RequireViewAccess())

	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	cg.PUT("/page-blocks/:eid/:bid", h.Put, campaigns.RequireRole(campaigns.RolePlayer))
	// The page map's text form: the service refuses anyone who may not edit
	// the page, as it does for PUT.
	cg.GET("/page-blocks/:eid/:bid/sketch", h.SketchText, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/page-blocks/:eid/:bid/sketch", h.Sketch, campaigns.RequireRole(campaigns.RolePlayer))
}
