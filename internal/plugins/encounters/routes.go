// Route registration for encounters.
package encounters

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the routes under the campaign group. Every route
// needs a signed-in member; the service then refuses anyone below Owner
// visibility, so the gate here is not the only one. CSRF is enforced by the
// global middleware.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	cg.GET("/pages/:eid/encounters", h.List)
	cg.POST("/pages/:eid/encounters", h.Create)
	cg.GET("/encounters/:encid", h.Get)
	cg.PUT("/encounters/:encid", h.Update)
	cg.DELETE("/encounters/:encid", h.Delete)
}
