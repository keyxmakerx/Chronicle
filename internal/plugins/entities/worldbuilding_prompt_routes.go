package entities

import (
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterWorldbuildingPromptRoutes adds worldbuilding prompt API routes.
// Prompts are readable by all members (shown on entity pages for Scribe+)
// but only manageable by campaign owners.
func RegisterWorldbuildingPromptRoutes(e *echo.Echo, h *WorldbuildingPromptHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// Read (Player+): shown on entity pages for content creators.
	cg.GET("/worldbuilding-prompts", h.ListAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// Write (Owner only): manage prompts.
	cg.POST("/worldbuilding-prompts", h.CreateAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/worldbuilding-prompts/:pid", h.UpdateAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/worldbuilding-prompts/:pid", h.DeleteAPI, campaigns.RequireRole(campaigns.RoleOwner))
}
