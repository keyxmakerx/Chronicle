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
	// Sidebar drill-down for public visitors (clicking categories in sidebar).
	pub.GET("/sidebar/drill/:slug", h.SidebarDrill, RequireRole(RolePlayer))

	// Authenticated campaign-scoped routes require membership.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		RequireCampaignAccess(svc),
	)

	// All members.
	cg.GET("/members", h.Members, RequireRole(RolePlayer))
	cg.GET("/plugins", h.PluginHub, RequireRole(RolePlayer))
	cg.GET("/plugins/fragment", h.PluginHubFragment, RequireRole(RolePlayer))

	// Owner-only routes.
	cg.GET("/edit", h.EditForm, RequireRole(RoleOwner))
	cg.PUT("", h.Update, RequireRole(RoleOwner))
	cg.DELETE("", h.Delete, RequireRole(RoleOwner))
	cg.GET("/settings", h.Settings, RequireRole(RoleOwner))
	cg.GET("/customize", h.Customize, RequireRole(RoleOwner))
	cg.GET("/customize/layout-editor/:etid", h.LayoutEditorFragment, RequireRole(RoleOwner))

	// Sidebar config API (Owner only).
	cg.GET("/sidebar-config", h.GetSidebarConfig, RequireRole(RoleOwner))
	cg.PUT("/sidebar-config", h.UpdateSidebarConfig, RequireRole(RoleOwner))

	// Dashboard layout API (Owner only).
	cg.GET("/dashboard-layout", h.GetDashboardLayout, RequireRole(RoleOwner))
	cg.PUT("/dashboard-layout", h.UpdateDashboardLayout, RequireRole(RoleOwner))
	cg.DELETE("/dashboard-layout", h.ResetDashboardLayout, RequireRole(RoleOwner))

	// Owner dashboard (Owner + Co-DM).
	cg.GET("/dashboard", h.OwnerDashboard, RequireRole(RoleOwner))
	cg.GET("/owner-dashboard-layout", h.GetOwnerDashboardLayout, RequireRole(RoleOwner))
	cg.PUT("/owner-dashboard-layout", h.UpdateOwnerDashboardLayout, RequireRole(RoleOwner))
	cg.DELETE("/owner-dashboard-layout", h.ResetOwnerDashboardLayout, RequireRole(RoleOwner))

	// Backdrop and branding (Owner only).
	cg.POST("/backdrop", h.UploadBackdrop, RequireRole(RoleOwner))
	cg.DELETE("/backdrop", h.RemoveBackdrop, RequireRole(RoleOwner))
	cg.PUT("/accent-color", h.UpdateAccentColorAPI, RequireRole(RoleOwner))
	cg.PUT("/branding", h.UpdateBrandingAPI, RequireRole(RoleOwner))
	cg.PUT("/topbar-style", h.UpdateTopbarStyleAPI, RequireRole(RoleOwner))

	// DM grants (Owner only).
	cg.GET("/dm-grants", h.GetDmGrantsAPI, RequireRole(RoleOwner))
	cg.PUT("/dm-grants", h.UpdateDmGrantsAPI, RequireRole(RoleOwner))

	// "View as player" display toggle (Owner only).
	cg.POST("/toggle-view-mode", h.ToggleViewAsPlayer, RequireRole(RoleOwner))

	// Member management (Owner only).
	cg.POST("/members", h.AddMember, RequireRole(RoleOwner))
	cg.DELETE("/members/:uid", h.RemoveMember, RequireRole(RoleOwner))
	cg.PUT("/members/:uid/role", h.UpdateRole, RequireRole(RoleOwner))
	cg.PUT("/members/:uid/character", h.UpdateMemberCharacterAPI, RequireRole(RoleOwner))

	// Ownership transfer (Owner only).
	cg.GET("/transfer", h.TransferForm, RequireRole(RoleOwner))
	cg.POST("/transfer", h.Transfer, RequireRole(RoleOwner))
	cg.POST("/cancel-transfer", h.CancelTransfer, RequireRole(RoleOwner))

	// Campaign groups (Owner only).
	cg.GET("/groups/manage", h.GroupsPage, RequireRole(RoleOwner))
	cg.GET("/groups", h.ListGroupsAPI, RequireRole(RoleOwner))
	cg.POST("/groups", h.CreateGroupAPI, RequireRole(RoleOwner))
	cg.GET("/groups/:gid", h.GetGroupAPI, RequireRole(RoleOwner))
	cg.PUT("/groups/:gid", h.UpdateGroupAPI, RequireRole(RoleOwner))
	cg.DELETE("/groups/:gid", h.DeleteGroupAPI, RequireRole(RoleOwner))
	cg.POST("/groups/:gid/members", h.AddGroupMemberAPI, RequireRole(RoleOwner))
	cg.DELETE("/groups/:gid/members/:uid", h.RemoveGroupMemberAPI, RequireRole(RoleOwner))
}

// RegisterExportRoutes sets up campaign export/import routes.
// Export is campaign-scoped (owner only). Import is auth-only (creates new campaign).
func RegisterExportRoutes(e *echo.Echo, eh *ExportHandler, svc CampaignService, authSvc auth.AuthService) {
	// Import creates a new campaign (auth only, no campaign scope needed).
	authed := e.Group("", auth.RequireAuth(authSvc))
	authed.GET("/campaigns/import", eh.ImportCampaignForm)
	authed.POST("/campaigns/import", eh.ImportCampaign)

	// Export requires campaign owner access.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		RequireCampaignAccess(svc),
	)
	cg.GET("/export", eh.ExportCampaign, RequireRole(RoleOwner))
}
