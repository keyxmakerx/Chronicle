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
	// the AUTHED cg group above (auth + campaign access + the calendar-addon
	// guard), NEVER the public pub group below. Any member (Player+) may record
	// their own availability and read the anonymous aggregate heatmap; per-member
	// detail on the overlay is gated to the owner / DM-granted inside the handler
	// by role, not by route (design §5). The addon-guard middleware is the same
	// one the rest of the sessions plugin already uses, so availability inherits
	// that battle-tested gating through the shared group.
	h.SetUserDirectory(authSvc)
	cg.GET("/availability", h.ShowAvailability, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/availability/mine", h.GetMyAvailabilityAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/availability/mine", h.SaveMyAvailabilityAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/availability/overlay", h.GetOverlayAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/availability/exceptions", h.ListMyExceptionsAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/availability/exceptions", h.AddExceptionAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/availability/exceptions", h.ReplaceDayExceptionsAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.DELETE("/availability/exceptions/:eid", h.DeleteExceptionAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// Slot proposals (C-SCHED-P2). Same member-only gating: any member (Player+)
	// may view a proposal and respond to its options; only Scribe+ may create one
	// (design Q2). All ride the authed cg group — NEVER the public pub group; the
	// only public proposal route is the emailed token below.
	cg.GET("/proposals", h.ListProposals, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/proposals", h.CreateProposalAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.GET("/proposals/:pid", h.ShowProposal, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/proposals/:pid/options/:oid/respond", h.RespondOptionAPI, campaigns.RequireRole(campaigns.RolePlayer))
	// Confirm-winner (C-SCHED-P3): Scribe+ picks the winning option, which closes
	// the proposal and mints a planned session from that slot.
	cg.POST("/proposals/:pid/confirm", h.ConfirmProposalAPI, campaigns.RequireRole(campaigns.RoleScribe))

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

	// RSVP token redemption — public endpoint, no auth required (token is the
	// credential, emailed to the user). GET renders a confirm interstitial; POST
	// applies. Splitting them stops mail scanners / link prefetchers from
	// auto-RSVPing via a background GET (C-SCHED-P3 0b).
	e.GET("/rsvp/:token", h.RedeemRSVPToken)
	e.POST("/rsvp/:token", h.ApplyRSVPToken)

	// Proposal one-click response redemption — public, mirrors the RSVP token
	// route's placement + hygiene. Same GET-confirm / POST-apply split (0b), and
	// the redeem additionally rechecks the proposal is still open + the user is
	// still a member (0a).
	e.GET("/proposals/respond/:token", h.RedeemProposalToken)
	e.POST("/proposals/respond/:token", h.ApplyProposalToken)

	// Scheduler notifications (C-SCHED-P2). User-scoped, not campaign-scoped —
	// the topbar bell is global — so these ride a plain authenticated group, not
	// the calendar campaign group. Every read/write is scoped to the caller.
	ng := e.Group("", auth.RequireAuth(authSvc))
	ng.GET("/notifications", h.ListNotificationsAPI)
	ng.GET("/notifications/badge", h.NotificationBadgeAPI)
	ng.POST("/notifications/:nid/read", h.MarkNotificationReadAPI)
	ng.POST("/notifications/read-all", h.MarkAllNotificationsReadAPI)
}
