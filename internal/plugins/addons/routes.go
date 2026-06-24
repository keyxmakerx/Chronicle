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

	// Addon list fragment for Customization Hub Extensions tab (HTMX).
	cg.GET("/addons/fragment", h.CampaignAddonsFragment, campaigns.RequireRole(campaigns.RoleOwner))

	// Addon list API (JSON, for widgets).
	cg.GET("/addons", h.CampaignAddonsAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Toggle addon on/off for campaign.
	cg.PUT("/addons/:addonID/toggle", h.ToggleCampaignAddon, campaigns.RequireRole(campaigns.RoleOwner))

	// Per-extension settings / onboarding pages (see setup_handler.go). These
	// are NEW leaves under the /extensions/:slug prefix on this (addons) group;
	// the campaigns plugin owns /extensions, /extensions/fragment and
	// /extensions/:slug/dashboard — distinct full paths, so Echo allows both.
	cg.GET("/extensions/:slug/settings", h.ExtensionSettings, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/extensions/:slug/settings/apply", h.ApplyExtensionSettings, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/extensions/:slug/settings/dismiss", h.DismissExtensionSetup, campaigns.RequireRole(campaigns.RoleOwner))
}
