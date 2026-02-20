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
