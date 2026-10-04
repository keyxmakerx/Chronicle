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

	// Campaign-wide map frame, set from the Customize page's Maps tab.
	cg.PUT("/maps/frame-style", h.SetCampaignFrameAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Canonical marker icon vocabulary: Chronicle is authoritative; the
	// Foundry sync module reads this to align its icon translation table.
	// No :mid — it's a campaign-static catalog.
	cg.GET("/maps/marker-icons", h.MarkerIconsAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// Marker CRUD (Player can list, Scribe+ can create/edit; delete is Scribe+ but the service limits a
	// Scribe to markers it created, Owner deletes any).
	cg.GET("/maps/:mid/markers", h.ListMarkersAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/maps/:mid/markers", h.CreateMarkerAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/maps/:mid/markers/:mkid", h.UpdateMarkerAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/maps/:mid/markers/:mkid", h.DeleteMarkerAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Public-capable views: map list and map viewer.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "maps"),
	)
	pub.GET("/maps", h.Index, campaigns.RequireViewAccess())
	pub.GET("/maps/:mid", h.Show, campaigns.RequireViewAccess())
	// The bare framed viewer for the focus view over entity pages: same group and
	// access check as the page itself.
	pub.GET("/maps/:mid/viewer", h.Viewer, campaigns.RequireViewAccess())
	// The picture for viewers who must not get the original of a shadowed map.
	pub.GET("/maps/:mid/player-image", h.PlayerImage, campaigns.RequireViewAccess())
	// Read-only map data for the embeddable map-widget / entity-map blocks on
	// public campaigns. meta = image + dimensions + visibility-filtered
	// markers; markers also exposed standalone. Both reuse the existing
	// role/visibility filtering and are empty-userID safe. Drawings/tokens
	// get their own pub group in RegisterDrawingRoutes; fog and layers stay
	// cg-only (GM tools).
	pub.GET("/maps/:mid/meta", h.GetMapMetaAPI, campaigns.RequireViewAccess())
	pub.GET("/maps/:mid/markers", h.ListMarkersAPI, campaigns.RequireViewAccess())
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

	// Drawings (Scribe+ can create/edit; delete is Scribe+ but the service limits a
	// Scribe to drawings it created, Owner deletes any).
	cg.GET("/maps/:mid/drawings", dh.ListDrawings, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/maps/:mid/drawings", dh.CreateDrawing, campaigns.RequireRole(campaigns.RoleScribe))
	cg.GET("/maps/:mid/drawings/:did", dh.GetDrawing, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/maps/:mid/drawings/:did", dh.UpdateDrawing, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/maps/:mid/drawings/:did", dh.DeleteDrawing, campaigns.RequireRole(campaigns.RoleScribe))

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
	// entity-map blocks render on public campaigns. Both list handlers already
	// filter by role (GM-only items hidden from players) and need no userID,
	// so anonymous public visitors are safe. Writes stay in cg above; FOG and
	// LAYERS are intentionally NOT exposed (GM tools).
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "maps"),
	)
	pub.GET("/maps/:mid/drawings", dh.ListDrawings, campaigns.RequireViewAccess())
	pub.GET("/maps/:mid/tokens", dh.ListTokens, campaigns.RequireViewAccess())
}

// RegisterHexRoutes sets up the hex layer's API. Painting is gated in
// HexService (owner or DM grant, or a scribe where the map's draw policy and
// fog allow), so the write route only requires membership: a DM-granted player
// is not a scribe by role and must still get through to the service.
func RegisterHexRoutes(e *echo.Echo, hh *HexHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "maps"),
	)
	cg.PATCH("/maps/:mid/hexes/cells", hh.PatchHexCells, campaigns.RequireRole(campaigns.RolePlayer))
	// Same membership-only gate: a DM-granted player is not an owner by role,
	// and HexService decides who may change what the hexes cover.
	cg.PUT("/maps/:mid/hexes/layer", hh.PutHexLayer, campaigns.RequireRole(campaigns.RolePlayer))
	// Fog and the party carry the same membership-only gate; HexService refuses
	// anyone who is not an owner or DM (fog) or not allowed by hexes.party_who.
	cg.POST("/maps/:mid/hexes/fog", hh.PostHexFog, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/maps/:mid/hexes/party", hh.PutHexParty, campaigns.RequireRole(campaigns.RolePlayer))

	// Public-capable read so a public campaign's map shows its hexes. The
	// service filters cells by the viewer's role, which is RoleNone for the
	// public.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "maps"),
	)
	pub.GET("/maps/:mid/hexes", hh.GetHexes, campaigns.RequireViewAccess())
}
