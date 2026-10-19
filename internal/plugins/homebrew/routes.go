// Route registration for the Homebrew Workshop.
package homebrew

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the routes under the campaign group. Everything here
// is the Director's, which the service checks (a role middleware cannot
// express the co-Director grant). CSRF is enforced by the global middleware.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id/homebrew",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		campaigns.RequireRole(campaigns.RolePlayer),
	)
	cg.GET("", h.Page)
	cg.GET("/catalog", h.Catalog)
	cg.GET("/recipes", h.List)
	cg.POST("/recipes", h.Create)
	cg.GET("/recipes/:rid", h.Get)
	cg.PUT("/recipes/:rid", h.Update)
	cg.DELETE("/recipes/:rid", h.Delete)
	cg.GET("/recipes/:rid/placements", h.Placements)
}
