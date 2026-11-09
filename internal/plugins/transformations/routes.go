// Route registration for layers.
package transformations

import (
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the routes under the campaign group. Every route
// needs a signed-in member and the Transformations add-on on; the service
// then decides who may read and refuses every write but the Director's, so
// the gate here is not the only one. CSRF is enforced by the global
// middleware.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, AddonSlug),
	)
	cg.GET("/pages/:eid/layers", h.List)
	cg.POST("/pages/:eid/layers", h.Add)
	cg.DELETE("/layers/:lid", h.Remove)
}
