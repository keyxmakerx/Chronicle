package admin

import (
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"

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
	// The status strip's live figures, loaded by every admin page.
	admin.GET("/status", h.Status)

	// Reauth middleware for sensitive operations: requires a password
	// re-confirmation within the last 5 minutes, so a hijacked admin session
	// alone cannot grant admin, lock people out or destroy data.
	reauth := auth.RequireReauth(authService)

	// The admin's own sidebar pins (the whole ordered list).
	admin.PUT("/nav/pins", h.UpdateNavPins)

	// User management.
	admin.GET("/users", h.Users)
	admin.PUT("/users/:id/admin", h.ToggleAdmin, reauth)

	// Campaign management.
	admin.GET("/campaigns", h.Campaigns)
	admin.DELETE("/campaigns/:id", h.DeleteCampaign, reauth)
	admin.POST("/campaigns/:id/join", h.JoinCampaign, reauth)
	admin.DELETE("/campaigns/:id/leave", h.LeaveCampaign)

	// Trash: deleted campaigns and file clean-ups wait here, with Undo.
	// Emptying it early is permanent, so it asks for the password again.
	admin.GET("/trash", h.Trash)
	admin.POST("/trash/campaigns/:id/undo", h.UndoTrashedCampaign)
	admin.POST("/trash/batches/:id/undo", h.UndoTrashedBatch)
	admin.POST("/trash/retention", h.SaveTrashRetention)
	admin.DELETE("/trash", h.EmptyTrash, reauth)

	// Site look: name, logo, look and sign-in background for pages outside a
	// campaign. The POST carries picture uploads, so it has its own body cap
	// (the global 2M limit skips this path in app.go).
	admin.GET("/site-look", h.SiteLook)
	admin.POST("/site-look", h.SaveSiteLook, echomw.BodyLimit("5M"))

	// Site-wide log of admin changes.
	admin.GET("/activity", h.Activity)

	// Storage management.
	admin.GET("/storage", h.Storage)
	admin.DELETE("/media/:fileID", h.DeleteMedia, reauth)

	// Security dashboard.
	admin.GET("/security", h.Security)
	admin.POST("/security/registration", h.UpdateRegistrationMode, reauth)
	admin.DELETE("/security/sessions/:hash", h.TerminateSession, reauth)
	admin.POST("/security/users/:id/force-logout", h.ForceLogoutUser, reauth)
	admin.PUT("/security/users/:id/disable", h.DisableUser, reauth)
	admin.PUT("/security/users/:id/enable", h.EnableUser, reauth)

	// Data hygiene dashboard.
	admin.GET("/data-hygiene", h.DataHygiene)
	admin.DELETE("/data-hygiene/orphaned-media", h.PurgeOrphanedMediaAPI, reauth)
	admin.DELETE("/data-hygiene/orphaned-api-keys", h.PurgeOrphanedAPIKeysAPI, reauth)
	admin.DELETE("/data-hygiene/stale-files", h.PurgeStaleFilesAPI, reauth)

	// System diagnostics.
	admin.GET("/systems", h.Systems)

	// Operator AI workspace for diagnostics: copyable functions list +
	// paste-an-AI-request → human-approve → run read-only. The bare
	// /admin/diagnostics markdown/named-run endpoint is wired separately in
	// app/routes.go on the systems handler; these are the in-chrome UI routes.
	admin.GET("/diagnostics/workspace", h.DiagnosticsWorkspace)
	admin.POST("/diagnostics/workspace/parse", h.DiagnosticsWorkspaceParse)
	admin.POST("/diagnostics/workspace/run", h.DiagnosticsWorkspaceRun)

	// Database explorer.
	admin.GET("/database", h.Database)
	admin.GET("/database/schema", h.DatabaseSchemaAPI)
	admin.GET("/database/status", h.DatabaseStatusAPI)
	admin.POST("/database/migrations/apply", h.ApplyMigrationsAPI, reauth)

	// SMTP settings (delegates to SMTP plugin handler).
	if smtpHandler != nil {
		smtp.RegisterRoutes(admin, smtpHandler, reauth)
	}

	return admin
}
