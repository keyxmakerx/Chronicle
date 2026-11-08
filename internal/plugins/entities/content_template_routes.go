package entities

import (
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterContentTemplateRoutes adds content template API routes.
// Templates are readable by all members (for the create form and editor insert
// menu) but only manageable by campaign owners.
func RegisterContentTemplateRoutes(e *echo.Echo, h *ContentTemplateHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// Read (Player+): needed by entity create form template picker and editor.
	cg.GET("/content-templates", h.ListAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/content-templates/:tid", h.GetAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// Write (Owner only): manage templates via Customization Hub.
	cg.POST("/content-templates", h.CreateAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/content-templates/:tid", h.UpdateAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/content-templates/:tid", h.DeleteAPI, campaigns.RequireRole(campaigns.RoleOwner))
}
