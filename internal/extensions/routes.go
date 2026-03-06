package extensions

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterAdminRoutes adds extension management routes to the admin group.
// All routes require site admin authentication.
func RegisterAdminRoutes(adminGroup *echo.Group, h *Handler) {
	g := adminGroup.Group("/extensions")

	g.GET("", h.ListExtensions)
	g.GET("/:extID", h.GetExtension)
	g.POST("/install", h.InstallExtension)
	g.POST("/rescan", h.RescanExtensions)
	g.PUT("/:extID", h.UpdateExtension)
	g.DELETE("/:extID", h.UninstallExtension)
}

// RegisterWASMAdminRoutes adds WASM plugin management routes to the admin group.
// All routes require site admin authentication.
func RegisterWASMAdminRoutes(adminGroup *echo.Group, wh *WASMHandler) {
	g := adminGroup.Group("/extensions/wasm")

	g.GET("/plugins", wh.ListWASMPlugins)
	g.GET("/plugins/:extID/:slug", wh.GetWASMPlugin)
	g.POST("/plugins/:extID/:slug/reload", wh.ReloadWASMPlugin)
	g.POST("/plugins/:extID/:slug/stop", wh.StopWASMPlugin)
}

// RegisterWASMCampaignRoutes adds per-campaign WASM plugin routes.
// Requires campaign membership for listing and Scribe+ for calling.
func RegisterWASMCampaignRoutes(e *echo.Echo, wh *WASMHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	g := e.Group("/campaigns/:id/extensions/wasm",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	g.GET("/plugins", wh.ListCampaignWASMPlugins)

	// Scribe+ required to invoke WASM plugin functions.
	scribe := g.Group("", campaigns.RequireRole(campaigns.RoleScribe))
	scribe.POST("/:extID/:slug/call", wh.CallWASMPlugin)
}

// RegisterCampaignRoutes adds per-campaign extension routes.
// Requires campaign Owner role for enable/disable operations.
func RegisterCampaignRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	g := e.Group("/campaigns/:id/extensions",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	g.GET("", h.ListCampaignExtensions)
	g.GET("/marker-icons", h.ListMarkerIcons)
	g.GET("/themes", h.ListThemes)
	g.GET("/widgets", h.ListWidgets)
	g.GET("/:extID/preview", h.PreviewExtension)

	// Owner-only operations.
	owner := g.Group("", campaigns.RequireRole(campaigns.RoleOwner))
	owner.POST("/:extID/enable", h.EnableExtension)
	owner.POST("/:extID/disable", h.DisableExtension)
}

// RegisterAssetRoutes adds the static asset serving route.
// This is public (assets are referenced in CSS/HTML for all users).
func RegisterAssetRoutes(e *echo.Echo, h *Handler) {
	e.GET("/extensions/:extID/assets/*", h.ServeAsset)
}
