package foundry_vtt

import (
	"github.com/labstack/echo/v4"
)

// RegisterOwnerRoutes mounts per-campaign owner endpoints under a
// group that already enforces campaign membership. Caller passes
// the campaigns Group (`/campaigns/:id`) plus the RoleOwner gate.
//
// All four endpoints are owner-only — the campaign owner controls
// which version Foundry installs + can rotate the URL.
//
// Routes are namespaced under `/foundry-vtt/` to coexist with
// foundry_modules' `/foundry/` during the C-FMC-5b parallel period.
// C-FMC-5c deletes foundry_modules and its `/foundry/` namespace
// (this plugin's `/foundry-vtt/` is the survivor).
func RegisterOwnerRoutes(cg *echo.Group, h *Handler, requireOwner echo.MiddlewareFunc) {
	cg.PUT("/foundry-vtt/pin", h.SetPinAPI, requireOwner)
	cg.POST("/foundry-vtt/token/rotate", h.RotateTokenAPI, requireOwner)
	cg.GET("/foundry-vtt/install-url", h.InstallURLAPI, requireOwner)
	cg.GET("/foundry-vtt/settings-tab", h.OwnerTabFragmentHandler, requireOwner)
}

// RegisterAdminRoutes mounts the admin endpoints used by the
// "Campaigns Using v0.1.5" expandable UI on /admin/packages. Caller
// passes the existing admin Group (which is already RequireAuth +
// RequireSiteAdmin gated). The force-pin routes additionally take
// the admin password re-auth middleware — applied inline so a
// hijacked admin session can't silently relocate every campaign.
//
// Force-pin reauth follows the same pattern foundry_modules' force-
// pin used (deleted in C-FMC-5c). reauth=nil is supported for
// dev/test contexts; production wiring always passes a non-nil
// middleware.
func RegisterAdminRoutes(admin *echo.Group, h *Handler, reauth echo.MiddlewareFunc) {
	g := admin.Group("/foundry-vtt")

	// Campaigns-using fragment — embedded in /admin/packages per
	// foundry-module typed package's version row.
	g.GET("/version/:version/campaigns", h.AdminVersionCampaignsHandler)

	// Per-campaign actions.
	g.POST("/version/:version/notify/:cid", h.AdminNotifyCampaignHandler)
	if reauth != nil {
		g.POST("/version/:version/force-pin/:cid", h.AdminForcePinCampaignHandler, reauth)
	} else {
		g.POST("/version/:version/force-pin/:cid", h.AdminForcePinCampaignHandler)
	}

	// Mass actions.
	g.POST("/version/:version/notify-older", h.AdminNotifyOlderHandler)
	if reauth != nil {
		g.POST("/version/:version/force-pin-older", h.AdminForcePinOlderHandler, reauth)
	} else {
		g.POST("/version/:version/force-pin-older", h.AdminForcePinOlderHandler)
	}

	// C-FMC-8: auto-pin banner endpoints. The banner surfaces the
	// most recent install summary so the admin can see "N campaigns
	// were auto-pinned to v0.X" inline at the top of /admin/packages
	// (vs. having to dig into the security_events panel). Dismiss
	// stamps a timestamp so a banner doesn't keep firing across page
	// reloads after the admin has acknowledged it.
	g.GET("/autopin-banner", h.AdminAutoPinBannerHandler)
	g.POST("/autopin-banner/dismiss", h.AdminAutoPinBannerDismissHandler)
}

// RegisterPublicRoutes mounts the unauthenticated manifest and
// download endpoints. Foundry hits these on every update check.
// The per-campaign signed token is the only access control.
//
// Rate-limit middleware should be applied by the caller (mirroring
// the packages plugin's contract) so an abusive client can't DoS
// the manifest endpoint into the database.
//
// URL shape locked at /api/v1/campaigns/:cid/foundry-vtt/... per
// the C-FMC-5-R1 cross-validation with Foundry AI. Parallels the
// existing /api/v1/campaigns/:cid/foundry-presence endpoint from
// PR #298.
func RegisterPublicRoutes(e *echo.Echo, h *Handler, rateLimit echo.MiddlewareFunc) {
	g := e.Group("/api/v1/campaigns/:cid/foundry-vtt")
	if rateLimit != nil {
		g.Use(rateLimit)
	}
	g.GET("/module.json", h.PublicManifestAPI)
	g.GET("/module.zip", h.PublicDownloadAPI)
}
