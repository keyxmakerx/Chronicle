// Route registration for trackers.
package trackers

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the routes under the campaign group. Reading a page's
// trackers follows the page: anyone who can view it may read, and the service
// hides what the viewer must not see. Every write, and everything about kinds,
// needs a Director, which the service checks (a role middleware cannot express
// the co-Director grant). CSRF is enforced by the global middleware.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/trackers/page/:eid", h.Page, campaigns.RequireViewAccess())

	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	cg.GET("/trackers/pages", h.Pages, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/trackers/page/:eid", h.Place, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/trackers/page/:eid/log", h.Log, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/trackers/entries/:entry/strike", h.Strike, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PATCH("/trackers/:tid", h.Patch, campaigns.RequireRole(campaigns.RolePlayer))
	cg.DELETE("/trackers/:tid", h.Remove, campaigns.RequireRole(campaigns.RolePlayer))

	cg.GET("/tracker-kinds", h.ListKinds, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/tracker-kinds", h.CreateKind, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/tracker-kinds/:kid", h.UpdateKind, campaigns.RequireRole(campaigns.RolePlayer))
	cg.DELETE("/tracker-kinds/:kid", h.DeleteKind, campaigns.RequireRole(campaigns.RolePlayer))
}
