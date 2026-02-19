package smtp

import "github.com/labstack/echo/v4"

// RegisterRoutes sets up SMTP admin routes on the given admin route group.
// All routes require site admin middleware (applied by the caller).
func RegisterRoutes(adminGroup *echo.Group, h *Handler) {
	adminGroup.GET("/smtp", h.Settings)
	adminGroup.PUT("/smtp", h.UpdateSettings)
	adminGroup.POST("/smtp/test", h.TestConnection)
}
