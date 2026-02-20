package admin

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/smtp"
)

// RegisterRoutes sets up all admin routes on the given Echo instance.
// Creates a /admin group with auth + site admin middleware, then registers
// sub-routes for dashboard, users, campaigns, and SMTP settings.
// Returns the admin group so other plugins can register additional admin routes.
func RegisterRoutes(e *echo.Echo, h *Handler, authService auth.AuthService, smtpHandler *smtp.Handler) *echo.Group {
	admin := e.Group("/admin",
		auth.RequireAuth(authService),
		auth.RequireSiteAdmin(),
	)

	// Dashboard.
	admin.GET("", h.Dashboard)

	// User management.
	admin.GET("/users", h.Users)
	admin.PUT("/users/:id/admin", h.ToggleAdmin)

	// Campaign management.
	admin.GET("/campaigns", h.Campaigns)
	admin.DELETE("/campaigns/:id", h.DeleteCampaign)
	admin.POST("/campaigns/:id/join", h.JoinCampaign)
	admin.DELETE("/campaigns/:id/leave", h.LeaveCampaign)

	// Storage management.
	admin.GET("/storage", h.Storage)
	admin.DELETE("/media/:fileID", h.DeleteMedia)

	// SMTP settings (delegates to SMTP plugin handler).
	if smtpHandler != nil {
		smtp.RegisterRoutes(admin, smtpHandler)
	}

	return admin
}
