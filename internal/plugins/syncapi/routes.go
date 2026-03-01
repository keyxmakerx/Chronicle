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
}

// RegisterAPIRoutes adds the public REST API endpoints under /api/v1/.
// All routes require API key authentication. Permission middleware enforces
// read/write/sync access levels. Campaign match middleware ensures keys can
// only access their scoped campaign.
func RegisterAPIRoutes(e *echo.Echo, api *APIHandler, calAPI *CalendarAPIHandler, syncSvc SyncAPIService) {
	// API v1 group with key auth and rate limiting.
	v1 := e.Group("/api/v1",
		RequireAPIKey(syncSvc),
		RateLimit(syncSvc),
	)

	// Campaign-scoped routes with campaign match enforcement.
	cg := v1.Group("/campaigns/:id", RequireCampaignMatch())

	// Read endpoints (require "read" permission).
	cg.GET("", api.GetCampaign, RequirePermission(PermRead))
	cg.GET("/entity-types", api.ListEntityTypes, RequirePermission(PermRead))
	cg.GET("/entity-types/:typeID", api.GetEntityType, RequirePermission(PermRead))
	cg.GET("/entities", api.ListEntities, RequirePermission(PermRead))
	cg.GET("/entities/:entityID", api.GetEntity, RequirePermission(PermRead))

	// Calendar read endpoints (require "read" permission).
	cg.GET("/calendar", calAPI.GetCalendar, RequirePermission(PermRead))
	cg.GET("/calendar/date", calAPI.GetCurrentDate, RequirePermission(PermRead))
	cg.GET("/calendar/events", calAPI.ListEvents, RequirePermission(PermRead))
	cg.GET("/calendar/events/:eventID", calAPI.GetEvent, RequirePermission(PermRead))

	// Write endpoints (require "write" permission).
	cg.POST("/entities", api.CreateEntity, RequirePermission(PermWrite))
	cg.PUT("/entities/:entityID", api.UpdateEntity, RequirePermission(PermWrite))
	cg.PUT("/entities/:entityID/fields", api.UpdateEntityFields, RequirePermission(PermWrite))
	cg.DELETE("/entities/:entityID", api.DeleteEntity, RequirePermission(PermWrite))

	// Calendar write endpoints (require "write" permission).
	cg.POST("/calendar/events", calAPI.CreateEvent, RequirePermission(PermWrite))
	cg.PUT("/calendar/events/:eventID", calAPI.UpdateEvent, RequirePermission(PermWrite))
	cg.DELETE("/calendar/events/:eventID", calAPI.DeleteEvent, RequirePermission(PermWrite))
	cg.PUT("/calendar/settings", calAPI.UpdateCalendarSettings, RequirePermission(PermWrite))
	cg.PUT("/calendar/months", calAPI.UpdateMonths, RequirePermission(PermWrite))
	cg.PUT("/calendar/weekdays", calAPI.UpdateWeekdays, RequirePermission(PermWrite))
	cg.PUT("/calendar/moons", calAPI.UpdateMoons, RequirePermission(PermWrite))
	cg.PUT("/calendar/eras", calAPI.UpdateEras, RequirePermission(PermWrite))
	cg.POST("/calendar/advance", calAPI.AdvanceDate, RequirePermission(PermWrite))
	cg.POST("/calendar/advance-time", calAPI.AdvanceTime, RequirePermission(PermWrite))

	// Sync endpoint (require "sync" permission).
	cg.POST("/sync", api.Sync, RequirePermission(PermSync))
}
