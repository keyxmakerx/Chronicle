package campaigns

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
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

	// Join via shareable invite code (auth required, no campaign membership needed).
	authed.GET("/join/:code", h.JoinByCode)

	// Public-capable view routes: logged-in users see full UI, guests see
	// read-only content for public campaigns.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		AllowPublicCampaignAccess(svc),
	)
	pub.GET("", h.Show, RequireViewAccess())
	// Sidebar drill-down for public visitors (clicking categories in sidebar).
	pub.GET("/sidebar/drill/:slug", h.SidebarDrill, RequireViewAccess())

	// Authenticated campaign-scoped routes require membership. Archived is
	// read-only by default here (RequireCampaignAccess blocks POST/PUT/
	// PATCH/DELETE) — cgArchived below is the short, explicit exception list.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		RequireCampaignAccess(svc),
	)

	// Writes that must keep working on an archived campaign, so an owner is
	// never locked out of undoing the archive, removing the campaign or
	// switching the display mode. Everything else waits for unarchive first.
	// TestArchivedExceptionList pins this list.
	cgArchived := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		RequireCampaignAccessEvenIfArchived(svc),
	)

	// All members.
	cg.GET("/members", h.Members, RequireRole(RolePlayer))
	cg.GET("/plugins", h.PluginHub, RequireRole(RolePlayer))
	cg.GET("/plugins/fragment", h.PluginHubFragment, RequireRole(RolePlayer))
	// /foundry-presence is registered by foundry_vtt.RegisterOwnerRoutes.

	// Owner-only routes.
	cg.GET("/edit", h.EditForm, RequireRole(RoleOwner))
	cg.PUT("", h.Update, RequireRole(RoleOwner))
	// Deleting must still work once archived — an owner cleaning out old
	// campaigns shouldn't have to unarchive one first.
	cgArchived.DELETE("", h.Delete, RequireRole(RoleOwner))
	cg.GET("/settings", h.Settings, RequireRole(RoleOwner))
	// /ai-export/generate is registered by ai_workspace.RegisterOwnerRoutes
	// against this same campaign group.
	cg.GET("/customize", h.Customize, RequireRole(RoleOwner))
	cg.GET("/customize/layout-editor/:etid", h.LayoutEditorFragment, RequireRole(RoleOwner))

	// Top-level Extensions hub; Content Packs renders as a card inside it
	// via ContentPacksCardRenderer. /extensions/fragment is the HTMX swap
	// target after toggles; the addons-store toggle handler emits the
	// `extensions-hub-refresh` HX-Trigger when posted with
	// redirect_to=extensions-hub.
	cg.GET("/extensions", h.ExtensionsHub, RequireRole(RoleOwner))
	cg.GET("/extensions/fragment", h.ExtensionsHubFragmentAPI, RequireRole(RoleOwner))
	// Mints a Foundry connect line (a new API key) and swaps the Foundry
	// page's connection row. Owner only: it issues a credential.
	cg.POST("/extensions/foundry/connect-line", h.NewFoundryConnectLine, RequireRole(RoleOwner))
	// The owner's Foundry page: checks, connection, module version, install.
	cg.GET("/foundry", h.FoundryPage, RequireRole(RoleOwner))
	// Per-extension inline dashboard fragment: hub cards with
	// HasDashboard=true hx-get this on expand. Disabled / unknown-slug
	// paths render safe placeholders, never a 4xx.
	cg.GET("/extensions/:slug/dashboard", h.ExtensionDashboardFragmentAPI, RequireRole(RoleOwner))

	// Sidebar config API (Owner only).
	cg.GET("/sidebar-config", h.GetSidebarConfig, RequireRole(RoleOwner))
	cg.PUT("/sidebar-config", h.UpdateSidebarConfig, RequireRole(RoleOwner))
	// A member's own pinned rows; the service refuses the owner, who pins
	// for everyone through sidebar-config.
	cg.PUT("/nav-pins", h.UpdateNavPins, RequireRole(RolePlayer))

	// Dashboard layout API (Owner only).
	cg.GET("/dashboard-layout", h.GetDashboardLayout, RequireRole(RoleOwner))
	cg.PUT("/dashboard-layout", h.UpdateDashboardLayout, RequireRole(RoleOwner))
	cg.DELETE("/dashboard-layout", h.ResetDashboardLayout, RequireRole(RoleOwner))

	// Owner dashboard (Owner + Co-DM).
	cg.GET("/dashboard", h.OwnerDashboard, RequireRole(RoleOwner))
	cg.GET("/owner-dashboard-layout", h.GetOwnerDashboardLayout, RequireRole(RoleOwner))
	cg.PUT("/owner-dashboard-layout", h.UpdateOwnerDashboardLayout, RequireRole(RoleOwner))
	cg.DELETE("/owner-dashboard-layout", h.ResetOwnerDashboardLayout, RequireRole(RoleOwner))

	// Branding and appearance (Owner only).
	cg.PUT("/branding", h.UpdateBrandingAPI, RequireRole(RoleOwner))
	cg.PUT("/welcome-message", h.UpdateWelcomeMessageAPI, RequireRole(RoleOwner))
	cg.PUT("/appearance", h.SaveAppearanceAPI, RequireRole(RoleOwner))
	cg.POST("/appearance/picture", h.UploadAppearancePictureAPI, RequireRole(RoleOwner))
	cg.DELETE("/appearance/picture", h.DeleteAppearancePictureAPI, RequireRole(RoleOwner))
	cg.PUT("/default-visibility", h.UpdateDefaultVisibilityAPI, RequireRole(RoleOwner))
	// Event tier definitions per campaign: owner-only campaign-config
	// surface, not exposed via syncapi.
	cg.GET("/event-tier-definitions", h.GetEventTierDefinitionsAPI, RequireRole(RoleOwner))
	cg.PUT("/event-tier-definitions", h.UpdateEventTierDefinitionsAPI, RequireRole(RoleOwner))

	// DM grants (Owner only).
	cg.GET("/dm-grants", h.GetDmGrantsAPI, RequireRole(RoleOwner))
	cg.PUT("/dm-grants", h.UpdateDmGrantsAPI, RequireRole(RoleOwner))

	// "View as player" display toggle (Owner only). It only flips a display
	// cookie, so it stays usable on an archived campaign.
	cgArchived.POST("/toggle-view-mode", h.ToggleViewAsPlayer, RequireRole(RoleOwner))

	// Member management (Owner only).
	cg.POST("/members", h.AddMember, RequireRole(RoleOwner))
	cg.DELETE("/members/:uid", h.RemoveMember, RequireRole(RoleOwner))
	cg.PUT("/members/:uid/role", h.UpdateRole, RequireRole(RoleOwner))
	cg.PUT("/members/:uid/character", h.UpdateMemberCharacterAPI, RequireRole(RoleOwner))

	// Ownership transfer (Owner only).
	cg.GET("/transfer", h.TransferForm, RequireRole(RoleOwner))
	cg.POST("/transfer", h.Transfer, RequireRole(RoleOwner))
	cg.POST("/cancel-transfer", h.CancelTransfer, RequireRole(RoleOwner))

	// Archive (Owner only). Archiving only makes sense on an active campaign,
	// so it keeps the default gate; unarchiving is the only way out of
	// read-only, so it must work on an archived campaign.
	cg.POST("/archive", h.ArchiveCampaign, RequireRole(RoleOwner))
	cgArchived.POST("/unarchive", h.UnarchiveCampaign, RequireRole(RoleOwner))

	// Game system (Owner only).
	cg.PUT("/system", h.UpdateSystemID, RequireRole(RoleOwner))

	// Shareable invite link (Owner only).
	cg.POST("/join-code", h.GenerateJoinCode, RequireRole(RoleOwner))
	cg.DELETE("/join-code", h.RevokeJoinCode, RequireRole(RoleOwner))

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

// RegisterInviteRoutes sets up campaign invite routes.
// The accept page is publicly accessible (with optional auth).
// Invite CRUD is owner-only within the campaign scope.
func RegisterInviteRoutes(e *echo.Echo, ih *InviteHandler, svc CampaignService, authSvc auth.AuthService) {
	// Accept invite — accessible without campaign membership.
	// Uses optional auth so logged-in users auto-accept.
	e.GET("/invites/accept", ih.AcceptInvitePage, auth.OptionalAuth(authSvc))

	// Campaign-scoped invite management (Owner only).
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		RequireCampaignAccess(svc),
	)
	cg.GET("/invites", ih.ListInvitesAPI, RequireRole(RoleOwner))
	cg.GET("/invites/page", ih.InvitesPage, RequireRole(RoleOwner))
	cg.POST("/invites", ih.CreateInviteAPI, RequireRole(RoleOwner))
	cg.DELETE("/invites/:inviteId", ih.RevokeInviteAPI, RequireRole(RoleOwner))
}

// RegisterExportRoutes sets up campaign export/import routes.
// Export is campaign-scoped (owner only). Import is auth-only (creates new campaign).
// Both are heavy (media zips, adapters across many tables), so they carry
// tight rate limits to stop a runaway refresh or POST flood from
// monopolizing the server.
func RegisterExportRoutes(e *echo.Echo, eh *ExportHandler, svc CampaignService, authSvc auth.AuthService) {
	// Import creates a new campaign (auth only, no campaign scope needed).
	authed := e.Group("", auth.RequireAuth(authSvc))
	authed.GET("/campaigns/import", eh.ImportCampaignForm)
	authed.POST("/campaigns/import", eh.ImportCampaign, middleware.RateLimit(5, 1*time.Hour))

	// Export requires campaign owner access.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		RequireCampaignAccess(svc),
	)
	cg.GET("/export", eh.ExportCampaign, RequireRole(RoleOwner), middleware.RateLimit(10, 1*time.Hour))
}
