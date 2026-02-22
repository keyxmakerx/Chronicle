package campaigns

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// RegisterRoutes sets up all campaign-related routes on the given Echo instance.
// Campaign list and creation require auth. Campaign-scoped view routes allow
// public access to public campaigns. Mutating routes require membership.
func RegisterRoutes(e *echo.Echo, h *Handler, svc CampaignService, authSvc auth.AuthService) {
	// Campaign list and creation require authentication only.
	authed := e.Group("", auth.RequireAuth(authSvc))
	authed.GET("/campaigns", h.Index)
	authed.GET("/campaigns/picker", h.Picker) // HTMX fragment for topbar dropdown.
	authed.GET("/campaigns/new", h.NewForm)
	authed.POST("/campaigns", h.Create)

	// Accept transfer requires auth but not campaign membership (uses token).
	authed.GET("/campaigns/:id/accept-transfer", h.AcceptTransfer)

	// Public-capable view routes: logged-in users see full UI, guests see
	// read-only content for public campaigns.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		AllowPublicCampaignAccess(svc),
	)
	pub.GET("", h.Show, RequireRole(RolePlayer))

	// Authenticated campaign-scoped routes require membership.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		RequireCampaignAccess(svc),
	)

	// All members.
	cg.GET("/members", h.Members, RequireRole(RolePlayer))

	// Owner-only routes.
	cg.GET("/edit", h.EditForm, RequireRole(RoleOwner))
	cg.PUT("", h.Update, RequireRole(RoleOwner))
	cg.DELETE("", h.Delete, RequireRole(RoleOwner))
	cg.GET("/settings", h.Settings, RequireRole(RoleOwner))
	cg.GET("/customize", h.Customize, RequireRole(RoleOwner))

	// Sidebar config API (Owner only).
	cg.GET("/sidebar-config", h.GetSidebarConfig, RequireRole(RoleOwner))
	cg.PUT("/sidebar-config", h.UpdateSidebarConfig, RequireRole(RoleOwner))

	// Dashboard layout API (Owner only).
	cg.GET("/dashboard-layout", h.GetDashboardLayout, RequireRole(RoleOwner))
	cg.PUT("/dashboard-layout", h.UpdateDashboardLayout, RequireRole(RoleOwner))
	cg.DELETE("/dashboard-layout", h.ResetDashboardLayout, RequireRole(RoleOwner))

	// Member management (Owner only).
	cg.POST("/members", h.AddMember, RequireRole(RoleOwner))
	cg.DELETE("/members/:uid", h.RemoveMember, RequireRole(RoleOwner))
	cg.PUT("/members/:uid/role", h.UpdateRole, RequireRole(RoleOwner))

	// Ownership transfer (Owner only).
	cg.GET("/transfer", h.TransferForm, RequireRole(RoleOwner))
	cg.POST("/transfer", h.Transfer, RequireRole(RoleOwner))
	cg.POST("/cancel-transfer", h.CancelTransfer, RequireRole(RoleOwner))
}
