package foundry_vtt

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterOwnerRoutes mounts per-campaign owner endpoints under a
// group that already enforces campaign membership. Caller passes
// the campaigns Group (`/campaigns/:id`) plus the RoleOwner gate.
//
// Every endpoint is owner-only — the campaign owner controls which version
// Foundry installs and can rotate the URL. There is no endpoint that writes the
// version directly: it moves only through the update actions below, which an
// admin's hold can refuse.
func RegisterOwnerRoutes(cg *echo.Group, h *Handler, requireOwner echo.MiddlewareFunc) {
	cg.POST("/foundry-vtt/token/rotate", h.RotateTokenAPI, requireOwner)
	cg.GET("/foundry-vtt/install-url", h.InstallURLAPI, requireOwner)
	cg.GET("/foundry-vtt/settings-tab", h.OwnerTabFragmentHandler, requireOwner)

	// Dashboard sync block, lazy-loaded when the dashboard layout
	// includes a sync_status block. Campaign-member access: the inner
	// /sync-status fetch is owner-gated by syncapi.
	cg.GET("/foundry-vtt/dashboard-sync-block", h.DashboardSyncBlockHandler)

	// "New module version is ready" line for the campaign home page and the
	// Module version card of the Foundry page, with the owner's answers to
	// them and the choice of how new versions arrive.
	cg.GET("/foundry-vtt/show-banner-fragment", h.CampaignShowBannerHandler, requireOwner)
	cg.GET("/foundry-vtt/apps-row", h.AppsUpdateRowHandler, requireOwner)
	cg.POST("/foundry-vtt/update", h.OwnerUpdateHandler, requireOwner)
	cg.POST("/foundry-vtt/update/later", h.OwnerUpdateLaterHandler, requireOwner)
	cg.POST("/foundry-vtt/update/switch", h.OwnerUpdateSwitchHandler, requireOwner)
	cg.POST("/foundry-vtt/update/mode", h.OwnerUpdateModeHandler, requireOwner)

	// "Connected to Foundry" presence pill for the map title,
	// lazy-loaded by maps/maps.templ. Campaign-member access.
	cg.GET("/foundry-vtt/presence-pill-fragment", h.CampaignShowPresencePillHandler)

	// Kept at the campaigns-prefix URL (not under /foundry-vtt/) since
	// it has shipped at this path since PR #298; renaming would break
	// callers. Member access is enforced by the parent group.
	cg.GET("/foundry-presence", h.GetFoundryPresenceAPI)
}

// RegisterDMTeamRoutes mounts the NPC "Show in Foundry" button and its
// action for the campaign owner and members given DM access. Caller
// passes the same campaign-member group as RegisterOwnerRoutes.
func RegisterDMTeamRoutes(cg *echo.Group, h *Handler) {
	dmTeam := campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool {
		return cc.CanAuthorDmOnly()
	}, "only the campaign owner or a member with DM access may do this")
	cg.GET("/foundry-vtt/npc-spotlight/:eid", h.NPCSpotlightButtonHandler, dmTeam)
	cg.POST("/foundry-vtt/npc-spotlight/:eid", h.NPCSpotlightAPI, dmTeam)
}

// RegisterAdminRoutes mounts the admin endpoints for the "Campaigns
// Using v0.1.5" expandable UI on /admin/packages. Caller passes the
// admin Group (already RequireAuth + RequireSiteAdmin gated). The
// force-pin routes additionally require admin password re-auth,
// applied inline so a hijacked admin session can't silently relocate
// every campaign. reauth=nil is allowed for dev/test; production
// wiring always passes a non-nil middleware.
func RegisterAdminRoutes(admin *echo.Group, h *Handler, reauth echo.MiddlewareFunc) {
	g := admin.Group("/foundry-vtt")

	// Campaigns-using fragment, embedded in the version row on /admin/packages.
	g.GET("/version/:version/campaigns", h.AdminVersionCampaignsHandler)

	// Per-campaign actions.
	g.POST("/version/:version/notify/:cid", h.AdminNotifyCampaignHandler)
	if reauth != nil {
		g.POST("/version/:version/force-pin/:cid", h.AdminForcePinCampaignHandler, reauth)
	} else {
		g.POST("/version/:version/force-pin/:cid", h.AdminForcePinCampaignHandler)
	}

	// Mass actions.
	g.POST("/version/:version/notify-older", h.AdminNotifyOlderHandler)
	if reauth != nil {
		g.POST("/version/:version/force-pin-older", h.AdminForcePinOlderHandler, reauth)
	} else {
		g.POST("/version/:version/force-pin-older", h.AdminForcePinOlderHandler)
	}

	// Auto-pin banner: surfaces the most recent install summary on
	// /admin/packages. Dismiss stamps a timestamp so it doesn't keep
	// firing after the admin has acknowledged it.
	g.GET("/autopin-banner", h.AdminAutoPinBannerHandler)
	g.POST("/autopin-banner/dismiss", h.AdminAutoPinBannerDismissHandler)

	// Per-row foundry-module fragment for /admin/packages, lazy-loaded
	// so packages.templ stays foundry-agnostic.
	g.GET("/packages/:id/actions-fragment", h.AdminPackageActionsFragmentHandler)
}

// RegisterPublicRoutes mounts the unauthenticated manifest and
// download endpoints. Foundry hits these on every update check.
// The per-campaign signed token is the only access control.
//
// Caller must apply rate-limit middleware so an abusive client
// can't DoS the manifest endpoint into the database.
func RegisterPublicRoutes(e *echo.Echo, h *Handler, rateLimit echo.MiddlewareFunc) {
	g := e.Group("/api/v1/campaigns/:cid/foundry-vtt")
	if rateLimit != nil {
		g.Use(rateLimit)
	}
	g.GET("/module.json", h.PublicManifestAPI)
	g.GET("/module.zip", h.PublicDownloadAPI)
}
