package addons

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterAdminRoutes adds addon management routes to the admin group.
// These routes require site admin privileges.
func RegisterAdminRoutes(adminGroup *echo.Group, h *Handler) {
	adminGroup.GET("/addons", h.AdminAddonsPage)
	adminGroup.POST("/addons", h.CreateAddon)
	adminGroup.PUT("/addons/:addonID/status", h.UpdateAddonStatus)
	adminGroup.DELETE("/addons/:addonID", h.DeleteAddon)
}

// RegisterCampaignRoutes adds per-campaign addon management routes.
// Campaign owners can toggle addons and view/configure their settings.
func RegisterCampaignRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// Addon settings page (campaign owner only).
	cg.GET("/addons/settings", h.CampaignAddonsPage, campaigns.RequireRole(campaigns.RoleOwner))

	// Addon list API (JSON, for widgets).
	cg.GET("/addons", h.CampaignAddonsAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Toggle addon on/off for campaign.
	cg.PUT("/addons/:addonID/toggle", h.ToggleCampaignAddon, campaigns.RequireRole(campaigns.RoleOwner))
}
