package packages

import "github.com/labstack/echo/v4"

// RegisterRoutes mounts all package manager routes under the given admin group.
// All routes require site admin authentication (enforced by the parent group).
func RegisterRoutes(admin *echo.Group, h *Handler) {
	g := admin.Group("/packages")

	// Package CRUD.
	g.GET("", h.ListPackages)
	g.POST("", h.AddPackage)
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

	// Backwards-compatible alias for Foundry VTT module discovery.
	// Foundry expects module.json at a stable URL like /foundry-module/module.json.
	// The download route serves the cached ZIP for module installation.
	e.GET("/foundry-module/download", sh.ServeFoundryDownload, rl)
	e.GET("/foundry-module/*", sh.ServeFoundryAlias, rl)
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
