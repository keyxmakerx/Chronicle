package foundry_modules

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterAdminRoutes mounts admin-only endpoints under the admin
// group (which is already RequireAuth + RequireSiteAdmin gated at
// registration time). The force-pin route additionally requires the
// admin password re-auth middleware so a hijacked admin session
// can't silently relocate campaigns to arbitrary versions.
func RegisterAdminRoutes(admin *echo.Group, h *Handler, reauth echo.MiddlewareFunc) {
	g := admin.Group("/modules/foundry")
	g.GET("/versions", h.ListVersionsAPI)
	g.POST("/upload", h.UploadAPI)
	g.PUT("/:version/status", h.SetStatusAPI)
	g.POST("/:version/notify", h.NotifyAPI)
	g.GET("/:version/usage", h.UsageAPI)
	if reauth != nil {
		g.POST("/:version/force-pin", h.ForcePinAPI, reauth)
	} else {
		// Reauth not wired — still register so the route exists, but
		// log a startup warning at the call site. Defensive default.
		g.POST("/:version/force-pin", h.ForcePinAPI)
	}
}

// RegisterOwnerRoutes mounts the per-campaign owner endpoints under
// a group that already enforces campaign membership. Caller passes
// the campaigns Group (`/campaigns/:id`) plus the role-gate middleware
// for RoleOwner.
func RegisterOwnerRoutes(cg *echo.Group, h *Handler, requireOwner echo.MiddlewareFunc) {
	// All three are owner-only (the campaign owner controls which
	// version Foundry sees + can rotate the URL).
	cg.PUT("/foundry/pin", h.SetPinAPI, requireOwner)
	cg.POST("/foundry/token/rotate", h.RotateTokenAPI, requireOwner)
	cg.GET("/foundry/install-url", h.InstallURLAPI, requireOwner)
}

// RegisterPublicRoutes mounts the unauthenticated manifest and
// download endpoints. Foundry hits these on every update check; the
// per-campaign signed token is the only access control.
//
// Rate-limit middleware should be applied by the caller (mirroring
// the packages plugin's RegisterPublicRoutes contract) so an abusive
// client can't DoS the manifest endpoint into the database.
func RegisterPublicRoutes(e *echo.Echo, h *Handler, rateLimit echo.MiddlewareFunc) {
	g := e.Group("/api/v1/campaigns/:cid/foundry")
	if rateLimit != nil {
		g.Use(rateLimit)
	}
	g.GET("/module.json", h.PublicManifestAPI)
	g.GET("/module.zip", h.PublicDownloadAPI)
}

// requireRoleOwnerOnCampaignGroup is a convenience wrapper the caller
// can use to pull the right RequireRole middleware without importing
// campaigns at the routes.go call site. Kept here so the wiring code
// in internal/app/routes.go stays readable.
func RequireRoleOwnerMiddleware() echo.MiddlewareFunc {
	return campaigns.RequireRole(campaigns.RoleOwner)
}
