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
