package syncapi

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterAdminRoutes adds API monitoring routes to the admin group.
// These routes require site admin privileges. reauth guards the writes that
// change who can reach the API: blocking or unblocking an address, and
// switching off or revoking a key.
func RegisterAdminRoutes(adminGroup *echo.Group, h *Handler, reauth echo.MiddlewareFunc) {
	// Dashboard.
	adminGroup.GET("/api", h.AdminDashboard)

	// Data endpoints (JSON for dashboard widgets).
	adminGroup.GET("/api/logs", h.AdminRequestLogs)
	adminGroup.GET("/api/security", h.AdminSecurityEvents)

	// Security event management.
	adminGroup.PUT("/api/security/:eventID/resolve", h.ResolveEvent)

	// IP blocklist management.
	adminGroup.POST("/api/ip-blocks", h.BlockIP, reauth)
	adminGroup.DELETE("/api/ip-blocks/:blockID", h.UnblockIP, reauth)

	// Admin key management (can act on any key).
	adminGroup.PUT("/api/keys/:keyID/toggle", h.AdminToggleKey, reauth)
	adminGroup.DELETE("/api/keys/:keyID", h.AdminRevokeKey, reauth)
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

	// Sync dashboard fragments (owner only — HTMX-loaded on API keys page).
	cg.GET("/api-keys/sync-overview", h.SyncOverviewFragment, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/api-keys/sync-mappings", h.SyncMappingsFragment, campaigns.RequireRole(campaigns.RoleOwner))

	// API keys tab fragments (owner only — HTMX-loaded within Settings page).
	cg.GET("/integrations/keys", h.IntegrationsKeysFragment, campaigns.RequireRole(campaigns.RoleOwner))

	// Calendar sync beacon: any campaign member may read — NO RequireRole,
	// unlike every other route in this group. See the doc comment on
	// Handler.GetCalendarSyncBeacon.
	cg.GET("/calendar-sync-beacon", h.GetCalendarSyncBeacon)
}

// RegisterAPIRoutes adds the public REST API endpoints under /api/v1/.
// Routes accept EITHER a session cookie (for in-app browser widgets — same
// origin as the UI) OR an Authorization: Bearer API key (external clients
// like Foundry VTT). The RequireAuthOrAPIKey middleware is the single
// identity resolver for both auth shapes: it synthesises an APIKey from a
// valid session so downstream middleware (RequireCampaignMatch,
// RequirePermission) works uniformly without duplicated logic. See the
// doc comment on RequireAuthOrAPIKey for the full flow and CSRF analysis.
//
// Permission middleware enforces read/write/sync access levels. Campaign
// match middleware ensures Bearer keys can only access their scoped
// campaign (session users are naturally scoped to campaigns they belong
// to by the membership lookup in RequireAuthOrAPIKey).
func RegisterAPIRoutes(e *echo.Echo, api *APIHandler, calAPI *CalendarAPIHandler, mediaAPI *MediaAPIHandler, mapAPI *MapAPIHandler, noteAPI *NoteAPIHandler, tagAPI *TagAPIHandler, syncH *SyncHandler, changesH *SyncChangesHandler, stashAPI *StashAPIHandler, syncSvc SyncAPIService, addonChecker AddonChecker, authSvc auth.AuthService, campaignSvc campaigns.CampaignService, opts ...func(*APIHandler)) {
	// Inject addon checker into API handler for system-aware endpoints.
	api.SetAddonChecker(addonChecker)

	// Apply optional configuration (e.g., campaign system lister).
	for _, opt := range opts {
		opt(api)
	}

	// Public version endpoint — no auth, no rate limit. External clients
	// (Foundry module dashboard) probe this to display "Connected to
	// Chronicle vX.Y.Z". Lives outside the /api/v1 group on purpose: it
	// pre-dates auth, intentionally requires none, and the path
	// /api/version is the contract the module already targets.
	e.GET("/api/version", VersionHandler)

	// API v1 group with session-or-bearer auth, rate limiting, and
	// JSON Content-Type enforcement. Rate limiting is a no-op for
	// session callers (see RateLimit comment). RequireJSONContentType
	// requires anything POST/PUT/PATCH-ing on this group to be
	// application/json; the lone multipart endpoint (UploadMedia)
	// re-registers below on v1Multipart, which keeps auth + rate-limit
	// but skips the JSON enforcement.
	//
	// RequireSyncAPIAddon sits immediately after the identity resolver and
	// before the rate limiter: a campaign that has switched the Sync API
	// off should not spend rate-limit budget being refused. It gates real
	// Bearer keys ONLY — first-party browser widgets on these same routes
	// carry a synthetic session key and pass through, because "Sync API"
	// revokes outside access, not Chronicle's own UI. See its doc comment.
	v1 := e.Group("/api/v1",
		RequireAuthOrAPIKey(authSvc, campaignSvc, syncSvc),
		RequireKeyOwnerStillOwner(campaignSvc, syncSvc),
		RequireSyncAPIAddon(addonChecker),
		RateLimit(syncSvc),
		RequireJSONContentType(),
		WithChangeSource(),
	)

	// Multipart sub-group at the same /api/v1 prefix. Same auth +
	// rate-limit chain as v1, but no Content-Type lock so multipart/
	// form-data is accepted at routes mounted here. UploadMedia is the
	// only inhabitant today (see audit table in
	// internal/middleware/multipart_auth_audit_test.go); any future
	// multipart endpoint under /api/v1/* mounts here, not on v1.
	v1Multipart := e.Group("/api/v1",
		RequireAuthOrAPIKey(authSvc, campaignSvc, syncSvc),
		RequireKeyOwnerStillOwner(campaignSvc, syncSvc),
		RequireSyncAPIAddon(addonChecker),
		RateLimit(syncSvc),
	)

	// Campaign-scoped routes with campaign match enforcement.
	campaignMW := []echo.MiddlewareFunc{RequireCampaignMatch(campaignSvc)}
	if api.history != nil {
		campaignMW = append(campaignMW, RecordSyncHistory(api.history.repo, api))
	}
	cg := v1.Group("/campaigns/:id", campaignMW...)

	// Read endpoints (require "read" permission).
	cg.GET("", api.GetCampaign, RequirePermission(PermRead))
	cg.GET("/members", api.ListMembers, RequirePermission(PermRead))
	cg.GET("/systems", api.ListSystems, RequirePermission(PermRead))
	cg.GET("/systems/:systemId/character-fields", api.GetCharacterFields, RequirePermission(PermRead))
	cg.GET("/systems/:systemId/item-fields", api.GetItemFields, RequirePermission(PermRead))
	cg.GET("/entity-types", api.ListEntityTypes, RequirePermission(PermRead))
	cg.GET("/entity-types/:typeID", api.GetEntityType, RequirePermission(PermRead))
	cg.GET("/entities", api.ListEntities, RequirePermission(PermRead))
	cg.GET("/entities/:entityID", api.GetEntity, RequirePermission(PermRead))
	cg.GET("/entities/:entityID/relations", api.ListEntityRelations, RequirePermission(PermRead))
	cg.GET("/entities/:entityID/system-state/:system/:key", api.GetSystemState, RequirePermission(PermRead))
	cg.GET("/entities/:entityID/permissions", api.GetEntityPermissions, RequirePermission(PermRead))
	cg.PUT("/entities/:entityID/permissions", api.SetEntityPermissions, RequirePermission(PermWrite))

	// DM Screen for the Foundry module. The provider refuses players and
	// keeps the downtime switch owner-only; see dm_screen_api.go.
	cg.GET("/dm-screen", api.GetDMScreen, RequirePermission(PermRead))
	cg.POST("/dm-screen/reveal/:entityID", api.RevealDMScreenCharacter, RequirePermission(PermWrite))
	cg.POST("/dm-screen/downtime", api.SetDMScreenDowntime, RequirePermission(PermWrite))

	// Addon discovery (read).
	cg.GET("/addons", api.ListAddons, RequirePermission(PermRead))

	// Tag endpoints (always available, not addon-gated).
	cg.GET("/tags", tagAPI.ListTags, RequirePermission(PermRead))
	cg.POST("/tags", tagAPI.CreateTag, RequirePermission(PermWrite))
	cg.PUT("/tags/:tagId", tagAPI.UpdateTag, RequirePermission(PermWrite))
	cg.DELETE("/tags/:tagId", tagAPI.DeleteTag, RequirePermission(PermWrite))
	cg.PUT("/entities/:entityID/tags", tagAPI.SetEntityTags, RequirePermission(PermWrite))
	cg.POST("/entities/bulk-tags", tagAPI.BulkAssignTags, RequirePermission(PermWrite))

	// Relation type listing and CRUD.
	cg.GET("/relations/types", api.ListRelationTypes, RequirePermission(PermRead))
	cg.POST("/entities/:entityID/relations", api.CreateRelation, RequirePermission(PermWrite))
	cg.PUT("/relations/:relationId", api.UpdateRelation, RequirePermission(PermWrite))
	cg.DELETE("/relations/:relationId", api.DeleteRelation, RequirePermission(PermWrite))

	// Entity type write endpoints.
	cg.POST("/entity-types", api.CreateEntityType, RequirePermission(PermWrite))
	cg.PUT("/entity-types/:typeID", api.UpdateEntityType, RequirePermission(PermWrite))

	// Bulk entity operations.
	cg.POST("/entities/bulk-update", api.BulkUpdateEntityType, RequirePermission(PermWrite))

	// Calendar read endpoints (require "read" permission + calendar addon).
	calGroup := cg.Group("", RequireAddonAPI(addonChecker, "calendar"))
	calGroup.GET("/calendars", calAPI.ListCalendars, RequirePermission(PermRead))
	calGroup.GET("/calendar", calAPI.GetCalendar, RequirePermission(PermRead))
	calGroup.GET("/calendar/date", calAPI.GetCurrentDate, RequirePermission(PermRead))
	// Applied-date confirm: same auth + permission as the GET above — real
	// Bearer keys only, see ConfirmDate's doc comment for the
	// synthetic-session-key rejection.
	calGroup.POST("/calendar/date/confirm", calAPI.ConfirmDate, RequirePermission(PermRead))
	calGroup.GET("/calendar/seasons", calAPI.GetSeasons, RequirePermission(PermRead))
	calGroup.GET("/calendar/moons", calAPI.GetMoons, RequirePermission(PermRead))
	calGroup.GET("/calendar/eras", calAPI.GetEras, RequirePermission(PermRead))
	calGroup.GET("/calendar/event-categories", retiredCalendarRoute(retiredEventCategories))
	calGroup.GET("/calendar/structure", calAPI.GetStructure, RequirePermission(PermRead))
	calGroup.GET("/calendar/weather", calAPI.GetWeather, RequirePermission(PermRead))
	calGroup.GET("/calendar/weather/days", calAPI.ListDayWeather, RequirePermission(PermRead))
	calGroup.GET("/calendar/world-state", retiredCalendarRoute("GET /calendar/date carries the current season, moon phases and weather."))
	calGroup.GET("/calendar/cycles", calAPI.GetCycles, RequirePermission(PermRead))
	calGroup.GET("/calendar/festivals", calAPI.GetFestivals, RequirePermission(PermRead))
	calGroup.GET("/calendar/events", calAPI.ListEvents, RequirePermission(PermRead))
	calGroup.GET("/calendar/events/:eventID", calAPI.GetEvent, RequirePermission(PermRead))

	// Write endpoints (require "write" permission).
	cg.POST("/entities", api.CreateEntity, RequirePermission(PermWrite))
	cg.PUT("/entities/:entityID", api.UpdateEntity, RequirePermission(PermWrite))
	cg.PUT("/entities/:entityID/fields", api.UpdateEntityFields, RequirePermission(PermWrite))
	cg.POST("/entities/:entityID/reveal", api.ToggleEntityReveal, RequirePermission(PermWrite))
	cg.DELETE("/entities/:entityID", api.DeleteEntity, RequirePermission(PermWrite))

	// Calendar write endpoints (require "write" permission + calendar addon).
	// Creating a calendar and the structure, advance and import routes are
	// retired: V5 creates and edits calendars only in Chronicle's calendar.
	calGroup.POST("/calendar", retiredCalendarRoute(retiredCreate))
	calGroup.POST("/calendar/events", calAPI.CreateEvent, RequirePermission(PermWrite))
	calGroup.PUT("/calendar/events/:eventID", calAPI.UpdateEvent, RequirePermission(PermWrite))
	calGroup.DELETE("/calendar/events/:eventID", calAPI.DeleteEvent, RequirePermission(PermWrite))
	calGroup.PUT("/calendar/settings", retiredCalendarRoute(retiredStructure))
	calGroup.PUT("/calendar/months", retiredCalendarRoute(retiredStructure))
	calGroup.PUT("/calendar/weekdays", retiredCalendarRoute(retiredStructure))
	calGroup.PUT("/calendar/moons", retiredCalendarRoute(retiredStructure))
	calGroup.PUT("/calendar/eras", retiredCalendarRoute(retiredStructure))
	calGroup.PUT("/calendar/seasons", retiredCalendarRoute(retiredStructure))
	calGroup.PUT("/calendar/event-categories", retiredCalendarRoute(retiredEventCategories))
	calGroup.PUT("/calendar/weather", retiredCalendarRoute("Weather is set per day in Chronicle's calendar; GET /calendar/weather still reads it."))
	calGroup.PUT("/calendar/cycles", retiredCalendarRoute(retiredStructure))
	calGroup.PUT("/calendar/festivals", retiredCalendarRoute(retiredStructure))
	calGroup.POST("/calendar/advance", retiredCalendarRoute(retiredAdvance))
	calGroup.PUT("/calendar/date", calAPI.SetDate, RequirePermission(PermWrite))
	calGroup.POST("/calendar/advance-time", retiredCalendarRoute(retiredAdvance))
	calGroup.GET("/calendar/export", retiredCalendarRoute("Calendars are included in the campaign export."))
	calGroup.POST("/calendar/import", retiredCalendarRoute("Create a calendar from a file with Chronicle's new-calendar wizard, or a campaign's first calendar from Foundry with POST /calendar."))

	// Media read endpoints (require "read" permission).
	cg.GET("/media", mediaAPI.ListMedia, RequirePermission(PermRead))
	cg.GET("/media/stats", mediaAPI.GetMediaStats, RequirePermission(PermRead))
	cg.GET("/media/:mediaID", mediaAPI.GetMedia, RequirePermission(PermRead))

	// Media write endpoints (require "write" permission). UploadMedia
	// is the lone multipart/form-data POST under /api/v1/* and mounts
	// on v1Multipart so the JSON Content-Type lock on v1 doesn't 415
	// the upload at the door — see the v1Multipart comment above.
	// DeleteMedia stays on the JSON group: it accepts a body-less
	// DELETE which the Content-Type middleware passes through anyway.
	v1Multipart.POST("/campaigns/:id/media", mediaAPI.UploadMedia,
		RequireCampaignMatch(campaignSvc),
		RequirePermission(PermWrite),
	)
	cg.DELETE("/media/:mediaID", mediaAPI.DeleteMedia, RequirePermission(PermWrite))

	// Map read endpoints (require "read" permission + maps addon).
	mapGroup := cg.Group("", RequireAddonAPI(addonChecker, "maps"))
	mapGroup.GET("/maps", mapAPI.ListMaps, RequirePermission(PermRead))
	mapGroup.GET("/maps/look", mapAPI.GetMapLook, RequirePermission(PermRead))
	mapGroup.GET("/maps/:mapID", mapAPI.GetMap, RequirePermission(PermRead))
	mapGroup.GET("/maps/:mapID/player-image", mapAPI.PlayerImage, RequirePermission(PermRead))
	mapGroup.GET("/maps/:mapID/drawings", mapAPI.ListDrawings, RequirePermission(PermRead))
	mapGroup.GET("/maps/:mapID/tokens", mapAPI.ListTokens, RequirePermission(PermRead))
	mapGroup.GET("/maps/:mapID/layers", mapAPI.ListLayers, RequirePermission(PermRead))
	mapGroup.GET("/maps/:mapID/fog", mapAPI.ListFog, RequirePermission(PermRead))
	mapGroup.GET("/maps/:mapID/markers", mapAPI.ListMarkers, RequirePermission(PermRead))
	mapGroup.GET("/maps/:mapID/markers/:markerID", mapAPI.GetMarker, RequirePermission(PermRead))

	// Map write endpoints (require "write" permission + maps addon).
	mapGroup.POST("/maps/:mapID/markers", mapAPI.CreateMarker, RequirePermission(PermWrite))
	mapGroup.PUT("/maps/:mapID/markers/:markerID", mapAPI.UpdateMarker, RequirePermission(PermWrite))
	mapGroup.DELETE("/maps/:mapID/markers/:markerID", mapAPI.DeleteMarker, RequirePermission(PermWrite))
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

	// Shop room read endpoint (require "read" permission + the shop room's
	// addon, named by the app wiring): the Foundry module shows the room to
	// its players.
	shopGroup := cg.Group("", RequireAddonAPI(addonChecker, api.shopRoomAddon))
	shopGroup.GET("/armory/shops/:eid/room", api.GetShopRoom, RequirePermission(PermRead))
	// Buying acts as the member the call names (actingUserId); see
	// ShopBuyAPIService for why that can only narrow the key's power.
	shopGroup.GET("/armory/shops/:eid/buyers", api.GetShopBuyers, RequirePermission(PermRead))
	shopGroup.POST("/armory/shops/:eid/buy", api.BuyFromShop, RequirePermission(PermWrite))

	// Note read endpoints (require "read" permission).
	cg.GET("/notes", noteAPI.ListNotes, RequirePermission(PermRead))
	cg.GET("/notes/:noteID", noteAPI.GetNote, RequirePermission(PermRead))

	// Note write endpoints (require "write" permission).
	cg.POST("/notes", noteAPI.CreateNote, RequirePermission(PermWrite))
	cg.PUT("/notes/:noteID", noteAPI.UpdateNote, RequirePermission(PermWrite))
	cg.DELETE("/notes/:noteID", noteAPI.DeleteNote, RequirePermission(PermWrite))

	// Sync endpoint (require "sync" permission).
	cg.POST("/sync", api.Sync, RequirePermission(PermSync))

	// Sync mapping endpoints (require "sync" permission).
	cg.GET("/sync/mappings", syncH.ListMappings, RequirePermission(PermSync))
	cg.GET("/sync/mappings/:mappingID", syncH.GetMapping, RequirePermission(PermSync))
	cg.POST("/sync/mappings", syncH.CreateMapping, RequirePermission(PermSync))
	cg.DELETE("/sync/mappings/:mappingID", syncH.DeleteMapping, RequirePermission(PermSync))
	cg.GET("/sync/lookup", syncH.LookupMapping, RequirePermission(PermSync))
	cg.GET("/sync/pull", syncH.PullMappings, RequirePermission(PermSync))
	// Change feed: ids only, DM-equivalent callers only (checked in the handler).
	cg.GET("/sync/changes", changesH.ListChanges, RequirePermission(PermSync))
	// Sync history: what synced, which way, who and what failed. Owner or
	// DM access only (checked in the handler), like the change feed.
	if api.history != nil {
		cg.GET("/sync/history", api.history.ListHistory, RequirePermission(PermSync))
		cg.POST("/sync/history", api.history.ReportHistory, RequirePermission(PermSync))
		// Who is in the Foundry world and which member each user is linked
		// to, from the GM's client. Owner or DM access only (in the handler).
		cg.POST("/sync/players", api.history.ReportPlayers, RequirePermission(PermSync))
	}

	// Quests: boards, sheets and rewards for the Foundry module. Owner or
	// co-DM only (checked in the handler); the module passes the players view
	// on to its players. Paying and giving need the rewards addon.
	if q := api.quests; q != nil {
		cg.GET("/quests/homes", q.Homes, RequirePermission(PermRead))
		cg.GET("/quests/boards", q.Boards, RequirePermission(PermRead))
		cg.GET("/quests/party", q.Party, RequirePermission(PermRead))
		cg.GET("/quests/:entityID", q.GetQuest, RequirePermission(PermRead))
		cg.PUT("/quests/:entityID", q.PutQuest, RequirePermission(PermWrite))
		rewards := cg.Group("", RequireAddonAPI(addonChecker, q.rewardAddon))
		rewards.POST("/quests/pay", q.Pay, RequirePermission(PermWrite))
		rewards.POST("/quests/give", q.Give, RequirePermission(PermWrite))
	}

	// Stashes: move items and money as a named campaign member. The group is
	// gated by the stash feature's addon (slug supplied by the wiring), and the
	// acting member's own rules apply to each call — see StashAPIService.
	if stashAPI != nil {
		stashGroup := cg.Group("", RequireAddonAPI(addonChecker, stashAPI.addonSlug))
		stashGroup.GET("/stashes/view", stashAPI.View, RequirePermission(PermRead))
		stashGroup.GET("/stashes/history", stashAPI.History, RequirePermission(PermRead))
		stashGroup.GET("/stashes/requests", stashAPI.Requests, RequirePermission(PermRead))
		stashGroup.GET("/stashes/downtime", stashAPI.Downtime, RequirePermission(PermRead))
		stashGroup.POST("/stashes/moves", stashAPI.Move, RequirePermission(PermWrite))
		stashGroup.POST("/stashes/requests/:moveId/approve", stashAPI.Approve, RequirePermission(PermWrite))
		stashGroup.POST("/stashes/requests/:moveId/decline", stashAPI.Decline, RequirePermission(PermWrite))
		stashGroup.PUT("/stashes/downtime", stashAPI.SetDowntime, RequirePermission(PermWrite))
	}
}
