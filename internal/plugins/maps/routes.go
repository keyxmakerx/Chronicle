package maps

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all map-related routes.
// Map routes are scoped to a campaign and require membership.
// CRUD operations require Owner/Scribe role; viewing requires Player role.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	// Authenticated routes (CRUD).
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// Map CRUD (Owner can create/update/delete maps).
	cg.POST("/maps/new", h.CreateMapForm, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/maps", h.CreateMapAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/maps/:mid", h.UpdateMapAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/maps/:mid", h.DeleteMapAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Marker CRUD (Scribe+ can create/edit, Owner can delete).
	cg.POST("/maps/:mid/markers", h.CreateMarkerAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/maps/:mid/markers/:mkid", h.UpdateMarkerAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/maps/:mid/markers/:mkid", h.DeleteMarkerAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Public-capable views: map list and map viewer.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/maps", h.Index, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/maps/:mid", h.Show, campaigns.RequireRole(campaigns.RolePlayer))
}
