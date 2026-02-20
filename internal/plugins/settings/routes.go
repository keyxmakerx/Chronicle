package settings

import "github.com/labstack/echo/v4"

// RegisterRoutes sets up storage settings routes on the given admin group.
// All routes require site admin middleware (applied by the caller via the
// admin group's middleware stack).
func RegisterRoutes(adminGroup *echo.Group, h *Handler) {
	// Global storage settings.
	adminGroup.GET("/storage/settings", h.StorageSettings)
	adminGroup.POST("/storage/settings", h.UpdateStorageSettings)

	// Per-user storage overrides.
	adminGroup.PUT("/users/:id/storage", h.SetUserStorageLimit)
	adminGroup.DELETE("/users/:id/storage", h.DeleteUserStorageLimit)

	// Per-campaign storage overrides.
	adminGroup.PUT("/campaigns/:id/storage", h.SetCampaignStorageLimit)
	adminGroup.DELETE("/campaigns/:id/storage", h.DeleteCampaignStorageLimit)
}
