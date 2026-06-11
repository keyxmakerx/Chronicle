package maps

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all map-related routes.
// Map routes are scoped to a campaign and require membership.
// CRUD operations require Owner/Scribe role; viewing requires Player role.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	// Authenticated routes (CRUD).
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "maps"),
	)

	// Map CRUD (Owner can create/update/delete maps).
	cg.POST("/maps/new", h.CreateMapForm, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/maps", h.CreateMapAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/maps/:mid", h.UpdateMapAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/maps/:mid", h.DeleteMapAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Canonical marker icon vocabulary (C-MAPS-EDITOR-PIN-AND-ICON-PARITY):
	// Chronicle is authoritative; the Foundry sync module reads this to align
	// its icon translation table. No :mid — it's a campaign-static catalog.
	cg.GET("/maps/marker-icons", h.MarkerIconsAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// Marker CRUD (Player can list, Scribe+ can create/edit, Owner can delete).
	cg.GET("/maps/:mid/markers", h.ListMarkersAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/maps/:mid/markers", h.CreateMarkerAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/maps/:mid/markers/:mkid", h.UpdateMarkerAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/maps/:mid/markers/:mkid", h.DeleteMarkerAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Public-capable views: map list and map viewer.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "maps"),
	)
	pub.GET("/maps", h.Index, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/maps/:mid", h.Show, campaigns.RequireRole(campaigns.RolePlayer))
	// Read-only map data for the embeddable map-widget / entity-map blocks on
	// public campaigns (cordinator#39 finding 4). meta = image + dimensions +
	// visibility-filtered markers; markers also exposed standalone. Both reuse
	// the existing role/visibility filtering and are empty-userID safe.
	// Drawings/tokens get their own pub group in RegisterDrawingRoutes; fog and
	// layers stay cg-only (GM tools).
	pub.GET("/maps/:mid/meta", h.GetMapMetaAPI, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/maps/:mid/markers", h.ListMarkersAPI, campaigns.RequireRole(campaigns.RolePlayer))
}

// RegisterDrawingRoutes sets up API routes for drawings, tokens, layers, and fog.
// These are the real-time map collaboration endpoints used by both the Chronicle
// web UI and the Foundry VTT sync module.
func RegisterDrawingRoutes(e *echo.Echo, dh *DrawingHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	// All drawing/token/layer/fog routes require authentication and campaign access.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "maps"),
	)

	// Drawings (Scribe+ can create/edit, Owner can delete).
	cg.GET("/maps/:mid/drawings", dh.ListDrawings, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/maps/:mid/drawings", dh.CreateDrawing, campaigns.RequireRole(campaigns.RoleScribe))
	cg.GET("/maps/:mid/drawings/:did", dh.GetDrawing, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/maps/:mid/drawings/:did", dh.UpdateDrawing, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/maps/:mid/drawings/:did", dh.DeleteDrawing, campaigns.RequireRole(campaigns.RoleOwner))

	// Tokens (Scribe+ can create/edit/move, Owner can delete).
	cg.GET("/maps/:mid/tokens", dh.ListTokens, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/maps/:mid/tokens", dh.CreateToken, campaigns.RequireRole(campaigns.RoleScribe))
	cg.GET("/maps/:mid/tokens/:tid", dh.GetToken, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/maps/:mid/tokens/:tid", dh.UpdateToken, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PATCH("/maps/:mid/tokens/:tid/position", dh.UpdateTokenPosition, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/maps/:mid/tokens/:tid", dh.DeleteToken, campaigns.RequireRole(campaigns.RoleOwner))

	// Layers (Owner only — structural map changes).
	cg.GET("/maps/:mid/layers", dh.ListLayers, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/maps/:mid/layers", dh.CreateLayer, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/maps/:mid/layers/:lid", dh.GetLayer, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/maps/:mid/layers/:lid", dh.UpdateLayer, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/maps/:mid/layers/:lid", dh.DeleteLayer, campaigns.RequireRole(campaigns.RoleOwner))

	// Fog of war (Owner only — GM controls visibility).
	cg.GET("/maps/:mid/fog", dh.ListFog, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/maps/:mid/fog", dh.CreateFog, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/maps/:mid/fog/:fid", dh.DeleteFog, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/maps/:mid/fog/reset", dh.ResetFog, campaigns.RequireRole(campaigns.RoleOwner))

	// Public-capable READ-ONLY drawings + tokens, so the embeddable map-widget /
	// entity-map blocks render on public campaigns (cordinator#39 finding 4).
	// Both list handlers already filter by role (GM-only items hidden from
	// players) and need no userID, so anonymous public visitors are safe. Writes
	// stay in cg above; FOG and LAYERS are intentionally NOT exposed (GM tools).
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "maps"),
	)
	pub.GET("/maps/:mid/drawings", dh.ListDrawings, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/maps/:mid/tokens", dh.ListTokens, campaigns.RequireRole(campaigns.RolePlayer))
}
