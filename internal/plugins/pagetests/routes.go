// Route registration for tests.
package pagetests

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the routes under the campaign group. Every route
// needs a signed-in member; the service then decides what each viewer is
// told and refuses every write below Owner visibility, so the gate here is
// not the only one. CSRF is enforced by the global middleware.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	cg.GET("/pages/:eid/tests", h.List)
	cg.POST("/pages/:eid/tests", h.Create)
	cg.GET("/tests", h.Choices)
	cg.PUT("/tests/:tid", h.Update)
	cg.POST("/tests/:tid/tally", h.Tally)
	cg.POST("/tests/:tid/record", h.Record)
	cg.DELETE("/tests/:tid", h.Delete)
}
