package syncapi

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterAdminRoutes adds API monitoring routes to the admin group.
// These routes require site admin privileges.
func RegisterAdminRoutes(adminGroup *echo.Group, h *Handler) {
	// Dashboard.
	adminGroup.GET("/api", h.AdminDashboard)

	// Data endpoints (JSON for dashboard widgets).
	adminGroup.GET("/api/logs", h.AdminRequestLogs)
	adminGroup.GET("/api/security", h.AdminSecurityEvents)

	// Security event management.
	adminGroup.PUT("/api/security/:eventID/resolve", h.ResolveEvent)

	// IP blocklist management.
	adminGroup.POST("/api/ip-blocks", h.BlockIP)
	adminGroup.DELETE("/api/ip-blocks/:blockID", h.UnblockIP)

	// Admin key management (can act on any key).
	adminGroup.PUT("/api/keys/:keyID/toggle", h.AdminToggleKey)
	adminGroup.DELETE("/api/keys/:keyID", h.AdminRevokeKey)
}

// RegisterCampaignRoutes adds API key management routes for campaign owners.
func RegisterCampaignRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// API key management (campaign owner only).
	cg.GET("/api-keys", h.KeysPage, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/api-keys", h.CreateKey, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/api-keys/:keyID/toggle", h.ToggleKey, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/api-keys/:keyID", h.RevokeKey, campaigns.RequireRole(campaigns.RoleOwner))

	// Sync status embed (owner only — used by dashboard sync status block).
	cg.GET("/sync-status", h.SyncStatusEmbed, campaigns.RequireRole(campaigns.RoleOwner))
}

// RegisterAPIRoutes adds the public REST API endpoints under /api/v1/.
// All routes require API key authentication. Permission middleware enforces
// read/write/sync access levels. Campaign match middleware ensures keys can
// only access their scoped campaign.
func RegisterAPIRoutes(e *echo.Echo, api *APIHandler, calAPI *CalendarAPIHandler, mediaAPI *MediaAPIHandler, mapAPI *MapAPIHandler, syncH *SyncHandler, syncSvc SyncAPIService, addonChecker AddonChecker, opts ...func(*APIHandler)) {
	// Inject addon checker into API handler for system-aware endpoints.
	api.SetAddonChecker(addonChecker)

	// Apply optional configuration (e.g., campaign system lister).
	for _, opt := range opts {
		opt(api)
	}

	// API v1 group with key auth and rate limiting.
	v1 := e.Group("/api/v1",
		RequireAPIKey(syncSvc),
		RateLimit(syncSvc),
	)

	// Campaign-scoped routes with campaign match enforcement.
	cg := v1.Group("/campaigns/:id", RequireCampaignMatch())

	// Read endpoints (require "read" permission).
	cg.GET("", api.GetCampaign, RequirePermission(PermRead))
	cg.GET("/systems", api.ListSystems, RequirePermission(PermRead))
	cg.GET("/systems/:systemId/character-fields", api.GetCharacterFields, RequirePermission(PermRead))
	cg.GET("/entity-types", api.ListEntityTypes, RequirePermission(PermRead))
	cg.GET("/entity-types/:typeID", api.GetEntityType, RequirePermission(PermRead))
	cg.GET("/entities", api.ListEntities, RequirePermission(PermRead))
	cg.GET("/entities/:entityID", api.GetEntity, RequirePermission(PermRead))
	cg.GET("/entities/:entityID/relations", api.ListEntityRelations, RequirePermission(PermRead))
	cg.GET("/entities/:entityID/permissions", api.GetEntityPermissions, RequirePermission(PermRead))
	cg.PUT("/entities/:entityID/permissions", api.SetEntityPermissions, RequirePermission(PermWrite))

	// Calendar read endpoints (require "read" permission + calendar addon).
	calGroup := cg.Group("", RequireAddonAPI(addonChecker, "calendar"))
	calGroup.GET("/calendars", calAPI.ListCalendars, RequirePermission(PermRead))
	calGroup.GET("/calendar", calAPI.GetCalendar, RequirePermission(PermRead))
	calGroup.GET("/calendar/date", calAPI.GetCurrentDate, RequirePermission(PermRead))
	calGroup.GET("/calendar/events", calAPI.ListEvents, RequirePermission(PermRead))
	calGroup.GET("/calendar/events/:eventID", calAPI.GetEvent, RequirePermission(PermRead))

	// Write endpoints (require "write" permission).
	cg.POST("/entities", api.CreateEntity, RequirePermission(PermWrite))
	cg.PUT("/entities/:entityID", api.UpdateEntity, RequirePermission(PermWrite))
	cg.PUT("/entities/:entityID/fields", api.UpdateEntityFields, RequirePermission(PermWrite))
	cg.DELETE("/entities/:entityID", api.DeleteEntity, RequirePermission(PermWrite))

	// Calendar write endpoints (require "write" permission + calendar addon).
	calGroup.POST("/calendar/events", calAPI.CreateEvent, RequirePermission(PermWrite))
	calGroup.PUT("/calendar/events/:eventID", calAPI.UpdateEvent, RequirePermission(PermWrite))
	calGroup.DELETE("/calendar/events/:eventID", calAPI.DeleteEvent, RequirePermission(PermWrite))
	calGroup.PUT("/calendar/settings", calAPI.UpdateCalendarSettings, RequirePermission(PermWrite))
	calGroup.PUT("/calendar/months", calAPI.UpdateMonths, RequirePermission(PermWrite))
	calGroup.PUT("/calendar/weekdays", calAPI.UpdateWeekdays, RequirePermission(PermWrite))
	calGroup.PUT("/calendar/moons", calAPI.UpdateMoons, RequirePermission(PermWrite))
	calGroup.PUT("/calendar/eras", calAPI.UpdateEras, RequirePermission(PermWrite))
	calGroup.POST("/calendar/advance", calAPI.AdvanceDate, RequirePermission(PermWrite))
	calGroup.PUT("/calendar/date", calAPI.SetDate, RequirePermission(PermWrite))
	calGroup.POST("/calendar/advance-time", calAPI.AdvanceTime, RequirePermission(PermWrite))
	calGroup.GET("/calendar/export", calAPI.ExportCalendar, RequirePermission(PermRead))
	calGroup.POST("/calendar/import", calAPI.ImportCalendar, RequirePermission(PermWrite))

	// Media read endpoints (require "read" permission).
	cg.GET("/media", mediaAPI.ListMedia, RequirePermission(PermRead))
	cg.GET("/media/stats", mediaAPI.GetMediaStats, RequirePermission(PermRead))
	cg.GET("/media/:mediaID", mediaAPI.GetMedia, RequirePermission(PermRead))

	// Media write endpoints (require "write" permission).
	cg.POST("/media", mediaAPI.UploadMedia, RequirePermission(PermWrite))
	cg.DELETE("/media/:mediaID", mediaAPI.DeleteMedia, RequirePermission(PermWrite))

	// Map read endpoints (require "read" permission + maps addon).
	mapGroup := cg.Group("", RequireAddonAPI(addonChecker, "maps"))
	mapGroup.GET("/maps", mapAPI.ListMaps, RequirePermission(PermRead))
	mapGroup.GET("/maps/:mapID", mapAPI.GetMap, RequirePermission(PermRead))
	mapGroup.GET("/maps/:mapID/drawings", mapAPI.ListDrawings, RequirePermission(PermRead))
	mapGroup.GET("/maps/:mapID/tokens", mapAPI.ListTokens, RequirePermission(PermRead))
	mapGroup.GET("/maps/:mapID/layers", mapAPI.ListLayers, RequirePermission(PermRead))
	mapGroup.GET("/maps/:mapID/fog", mapAPI.ListFog, RequirePermission(PermRead))

	// Map write endpoints (require "write" permission + maps addon).
	mapGroup.POST("/maps/:mapID/drawings", mapAPI.CreateDrawing, RequirePermission(PermWrite))
	mapGroup.PUT("/maps/:mapID/drawings/:drawingID", mapAPI.UpdateDrawing, RequirePermission(PermWrite))
	mapGroup.DELETE("/maps/:mapID/drawings/:drawingID", mapAPI.DeleteDrawing, RequirePermission(PermWrite))
	mapGroup.POST("/maps/:mapID/tokens", mapAPI.CreateToken, RequirePermission(PermWrite))
	mapGroup.PUT("/maps/:mapID/tokens/:tokenID", mapAPI.UpdateToken, RequirePermission(PermWrite))
	mapGroup.PATCH("/maps/:mapID/tokens/:tokenID/position", mapAPI.UpdateTokenPosition, RequirePermission(PermWrite))
	mapGroup.DELETE("/maps/:mapID/tokens/:tokenID", mapAPI.DeleteToken, RequirePermission(PermWrite))
	mapGroup.POST("/maps/:mapID/layers", mapAPI.CreateLayer, RequirePermission(PermWrite))
	mapGroup.PUT("/maps/:mapID/layers/:layerID", mapAPI.UpdateLayer, RequirePermission(PermWrite))
	mapGroup.DELETE("/maps/:mapID/layers/:layerID", mapAPI.DeleteLayer, RequirePermission(PermWrite))
	mapGroup.POST("/maps/:mapID/fog", mapAPI.CreateFog, RequirePermission(PermWrite))
	mapGroup.DELETE("/maps/:mapID/fog/:fogID", mapAPI.DeleteFog, RequirePermission(PermWrite))
	mapGroup.DELETE("/maps/:mapID/fog", mapAPI.ResetFog, RequirePermission(PermWrite))

	// Sync endpoint (require "sync" permission).
	cg.POST("/sync", api.Sync, RequirePermission(PermSync))

	// Sync mapping endpoints (require "sync" permission).
	cg.GET("/sync/mappings", syncH.ListMappings, RequirePermission(PermSync))
	cg.GET("/sync/mappings/:mappingID", syncH.GetMapping, RequirePermission(PermSync))
	cg.POST("/sync/mappings", syncH.CreateMapping, RequirePermission(PermSync))
	cg.DELETE("/sync/mappings/:mappingID", syncH.DeleteMapping, RequirePermission(PermSync))
	cg.GET("/sync/lookup", syncH.LookupMapping, RequirePermission(PermSync))
	cg.GET("/sync/pull", syncH.PullMappings, RequirePermission(PermSync))
}
