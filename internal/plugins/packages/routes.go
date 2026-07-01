package packages

import "github.com/labstack/echo/v4"

// RegisterRoutes mounts all package manager routes under the given admin group.
// All routes require site admin authentication (enforced by the parent group).
func RegisterRoutes(admin *echo.Group, h *Handler) {
	g := admin.Group("/packages")

	// Package CRUD.
	g.GET("", h.ListPackages)
	g.POST("", h.AddPackage)
	// Stale-version cleanup wizard (static segment — registered before the
	// param routes conceptually; echo matches static over :id regardless).
	g.GET("/prune", h.PrunePreview)
	g.DELETE("/prune", h.PruneExecute)
	g.DELETE("/:id", h.RemovePackage)

	// Version management.
	g.GET("/:id/versions", h.ListVersions)
	g.PUT("/:id/version", h.InstallVersion)
	g.PUT("/:id/pin", h.SetPinnedVersion)
	g.DELETE("/:id/pin", h.ClearPinnedVersion)
	g.PUT("/:id/auto-update", h.SetAutoUpdate)
	g.POST("/:id/check", h.CheckForUpdates)

	// Usage tracking.
	g.GET("/:id/usage", h.GetUsage)

	// Submission review.
	g.GET("/pending", h.ListPendingSubmissions)
	g.POST("/:id/review", h.ReviewPackage)

	// Repo URL management.
	g.PUT("/:id/repo", h.UpdateRepoURL)

	// Lifecycle management.
	g.POST("/:id/deprecate", h.DeprecatePackage)
	g.DELETE("/:id/deprecate", h.UndeprecatePackage)
	g.POST("/:id/archive", h.ArchivePackage)
	g.DELETE("/:id/archive", h.UnarchivePackage)

	// Security settings.
	g.GET("/settings", h.GetSecuritySettings)
	g.POST("/settings", h.SaveSecuritySettings)
}

// RegisterPublicRoutes mounts unauthenticated routes for serving files from
// installed packages. External clients (e.g., Foundry VTT) fetch manifests
// and scripts from these endpoints. Rate limiting is applied at the group level.
func RegisterPublicRoutes(e *echo.Echo, sh *ServeHandler, rl echo.MiddlewareFunc) {
	// Generic route: /packages/serve/:type/:slug/filepath
	g := e.Group("/packages/serve")
	g.Use(rl)
	g.GET("/:type/:slug/*", sh.ServePackageFile)

	// C-FMC-5c removed the /foundry-module/* alias routes. Foundry now
	// installs from per-campaign signed URLs at
	// /api/v1/campaigns/:cid/foundry-vtt/module.json (foundry_vtt
	// plugin's RegisterPublicRoutes). The old shared URL had no per-
	// campaign isolation and was incompatible with the per-campaign
	// pin model from PR #300.
}

// RegisterOwnerRoutes mounts the owner-facing system submission routes.
// These routes require authentication but NOT admin privileges.
func RegisterOwnerRoutes(authenticated *echo.Group, oh *OwnerHandler) {
	g := authenticated.Group("/systems")

	g.GET("/browse", oh.BrowseSystems)
	g.GET("/submit", oh.SubmitSystemForm)
	g.POST("/submit", oh.HandleSubmitSystem)
	g.GET("/my-submissions", oh.MySubmissions)
}
