package entities

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterLayoutPresetRoutes adds layout preset API routes.
// Presets are readable by all members (for the template editor preset picker)
// but only manageable by campaign owners.
func RegisterLayoutPresetRoutes(e *echo.Echo, h *LayoutPresetHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// Read (Player+): needed by template editor's Load Preset dropdown.
	cg.GET("/layout-presets", h.ListAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/layout-presets/:pid", h.GetAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// Write (Owner only): manage presets via Customization Hub / template editor.
	cg.POST("/layout-presets", h.CreateAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/layout-presets/:pid", h.UpdateAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/layout-presets/:pid", h.DeleteAPI, campaigns.RequireRole(campaigns.RoleOwner))
}
