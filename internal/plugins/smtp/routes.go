package smtp

import "github.com/labstack/echo/v4"

// RegisterRoutes sets up SMTP admin routes on the given admin route group.
// All routes require site admin middleware (applied by the caller). reauth
// guards the writes that repoint outgoing mail or send it: whoever controls
// the mail server receives every password-reset link.
func RegisterRoutes(adminGroup *echo.Group, h *Handler, reauth echo.MiddlewareFunc) {
	adminGroup.GET("/smtp", h.Settings)
	adminGroup.PUT("/smtp", h.UpdateSettings, reauth)
	adminGroup.POST("/smtp/test", h.TestConnection)
	adminGroup.POST("/smtp/send-test", h.SendTestEmail, reauth)
}
