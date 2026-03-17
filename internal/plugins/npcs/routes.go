// routes.go registers NPC gallery endpoints on the Echo router.
// The gallery page is readable by Players; the reveal toggle requires Scribe+.
package npcs

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up NPC gallery routes on the Echo instance.
// Public-capable routes use AllowPublicCampaignAccess so public campaigns
// show NPCs to unauthenticated visitors. All routes are gated behind the
// "npcs" addon — campaign owners can enable/disable via the Plugin Hub.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	// Authenticated routes: reveal toggle (Scribe+).
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "npcs"),
	)
	cg.POST("/npcs/:eid/reveal", h.ToggleReveal, campaigns.RequireRole(campaigns.RoleScribe))

	// Public-capable routes: gallery view (Player+).
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "npcs"),
	)
	pub.GET("/npcs", h.Index, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/npcs/count", h.CountAPI, campaigns.RequireRole(campaigns.RolePlayer))
}
