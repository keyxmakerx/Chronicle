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

	// Availability scheduler (C-SCHED-P1). Member-only data — every route rides
	// the AUTHED cg group (RequireAuth + RequireCampaignAccess +
	// RequireAddon("calendar")), NEVER the public pub group below. Any member
	// (Player+) may record their own availability and read the anonymous
	// aggregate heatmap; per-member detail on the overlay is gated to the
	// owner / DM-granted inside the handler by role, not by route (design §5).
	//
	// Step-0 (coordinator anchor correction): the relay said "there is NO
	// RequireAddon() middleware". That is not accurate — addons.RequireAddon
	// (internal/plugins/addons/middleware.go:21) exists and is already used by
	// both sessions and calendar (routes.go); availability mirrors that battle-
	// tested pattern via the shared cg group, and gates on the "calendar" addon
	// like the rest of the sessions plugin.
	h.SetUserDirectory(authSvc)
	cg.GET("/availability", h.ShowAvailability, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/availability/mine", h.GetMyAvailabilityAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/availability/mine", h.SaveMyAvailabilityAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/availability/overlay", h.GetOverlayAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/availability/exceptions", h.ListMyExceptionsAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/availability/exceptions", h.AddExceptionAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.DELETE("/availability/exceptions/:eid", h.DeleteExceptionAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// Public-capable view routes.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "calendar"),
	)
	pub.GET("/sessions", h.ListSessions, campaigns.RequireViewAccess())
	pub.GET("/sessions/:sid", h.ShowSession, campaigns.RequireViewAccess())
	pub.GET("/sidebar/sessions-rsvp", h.SidebarRSVP, campaigns.RequireViewAccess())
	pub.GET("/sessions/embed", h.EmbedSessions, campaigns.RequireViewAccess())

	// RSVP token redemption — public endpoint, no auth required.
	// Token itself is the credential (emailed to the user).
	e.GET("/rsvp/:token", h.RedeemRSVPToken)
}
