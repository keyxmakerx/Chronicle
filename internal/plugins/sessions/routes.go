package sessions

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all session-related routes.
// Sessions require the calendar addon since sessions are integrated into the calendar.
func RegisterRoutes(e *echo.Echo, h *Handler,
	campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {

	// Authenticated routes (create, update, delete, entity linking).
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "calendar"),
	)
	cg.POST("/sessions", h.CreateSession, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/sessions/:sid", h.UpdateSessionAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/sessions/:sid", h.DeleteSessionAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/sessions/:sid/recap", h.UpdateRecapAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.POST("/sessions/:sid/rsvp", h.RSVPSession, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/sessions/:sid/entities", h.LinkEntityAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/sessions/:sid/entities/:eid", h.UnlinkEntityAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Public-capable view routes.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "calendar"),
	)
	pub.GET("/sessions", h.ListSessions, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/sessions/:sid", h.ShowSession, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/sidebar/sessions-rsvp", h.SidebarRSVP, campaigns.RequireRole(campaigns.RolePlayer))

	// RSVP token redemption — public endpoint, no auth required.
	// Token itself is the credential (emailed to the user).
	e.GET("/rsvp/:token", h.RedeemRSVPToken)
}
