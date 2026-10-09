package sessions

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// scribeOrCoDirector gates a route to a Scribe+ member OR a co-Director (the
// IsDmGranted capability — see campaigns.CampaignContext.CanAuthorDmOnly,
// which a plain Player can hold; a co-Director is not necessarily Scribe
// role). Reused by every game-night route the operator's "Co-Directors run
// game nights with the Director's controls" server change widens.
func scribeOrCoDirector() echo.MiddlewareFunc {
	return campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool {
		return cc.MemberRole >= campaigns.RoleScribe || cc.CanAuthorDmOnly()
	}, "only a Scribe, the campaign owner, or a granted co-Director may do this")
}

// ownerOrCoDirector gates a route to the campaign Owner or a co-Director —
// campaigns.CanAuthorDmOnly's exact condition, matching the pattern
// internal/plugins/calendar/routes.go already uses for its own Owner-or-
// co-Director route.
func ownerOrCoDirector() echo.MiddlewareFunc {
	return campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool {
		return cc.CanAuthorDmOnly()
	}, "only the campaign owner or a granted co-Director may do this")
}

// RegisterRoutes sets up all session-related routes.
// Game nights have their own addon switch (slug "sessions") and also need the
// calendar addon, since nights and hours are shown in the calendar.
func RegisterRoutes(e *echo.Echo, h *Handler,
	campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {

	// Authenticated routes (create, update, delete, entity linking).
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "calendar"),
		addons.RequireAddon(addonSvc, SessionsAddonSlug),
	)
	cg.POST("/sessions", h.CreateSession, campaigns.RequireRole(campaigns.RoleScribe))
	// Move (schedule-changing edits) and delete run a game night, so a
	// co-Director (the IsDmGranted capability — not necessarily Scribe role,
	// see campaigns.CampaignContext.CanAuthorDmOnly) gets them too, per the
	// operator's server-change #4 ("Co-Directors run game nights with the
	// Director's controls: move, cancel, restore, propose and confirm").
	// scribeOrCoDirector/ownerOrCoDirector below are declared once and reused
	// by every route this build widens the same way.
	cg.PUT("/sessions/:sid", h.UpdateSessionAPI, scribeOrCoDirector())
	cg.DELETE("/sessions/:sid", h.DeleteSessionAPI, ownerOrCoDirector())
	cg.PUT("/sessions/:sid/recap", h.UpdateRecapAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.POST("/sessions/:sid/rsvp", h.RSVPSession, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/sessions/:sid/entities", h.LinkEntityAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/sessions/:sid/entities/:eid", h.UnlinkEntityAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Game-night routes: restoring a soft-deleted session (co-Directors run
	// game nights per scribeOrCoDirector/ownerOrCoDirector above), a member's
	// own tally-exclusion switch, and the calendar-feed settings/token
	// endpoints (the feed's own public redemption route sits with the other
	// public token routes below).
	cg.POST("/sessions/:sid/restore", h.RestoreSessionAPI, ownerOrCoDirector())
	cg.PUT("/sessions/:sid/rsvp-exclude", h.SetRSVPExcludedAPI, campaigns.RequireRole(campaigns.RolePlayer))
	// Game nights with every member's answer and note, for the calendar's
	// day card. Members only: the roster and notes stay inside the table.
	cg.GET("/sessions/nights", h.ListGameNightsAPI, campaigns.RequireRole(campaigns.RolePlayer))
	// A night's recap and visible linked pages, for its page in the calendar.
	cg.GET("/sessions/:sid/page", h.NightPageAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/sessions/feed-settings", h.GetFeedSettingsAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/sessions/feed-settings", h.SetFeedSettingsAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/sessions/feed/token", h.GetFeedTokenAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/sessions/feed/token/replace", h.ReplaceFeedTokenAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// Availability scheduler. Member-only data — every route rides the AUTHED
	// cg group above (auth + campaign access + the calendar-addon guard),
	// NEVER the public pub group below. Any member (Player+) may record their
	// own availability and read the anonymous aggregate heatmap; per-member
	// detail on the overlay is gated to the owner / DM-granted inside the
	// handler by role, not by route.
	h.SetUserDirectory(authSvc)
	// The overlay roster's role column reads the campaign's co-DM grant list:
	// campaigns.Role alone labels a co-DM "Player" while handing them
	// owner-tier detail. One read per overlay, not one per member.
	h.SetCampaignReader(campaignSvc)
	cg.GET("/availability", h.ShowAvailability, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/availability/mine", h.GetMyAvailabilityAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/availability/mine", h.SaveMyAvailabilityAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/availability/overlay", h.GetOverlayAPI, campaigns.RequireRole(campaigns.RolePlayer))
	// Answers is Player+ (it says who has spoken, never what they said); the
	// nudge is Player+ ON THE ROUTE and Owner/co-DM IN THE HANDLER, following
	// this group's stated rule that entitlement is decided by role in a handler
	// rather than encoded in a route (see the comment above).
	cg.GET("/availability/answers", h.AvailabilityAnswersAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/availability/nudge", h.NudgeAvailabilityAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/availability/confirm", h.ConfirmMyAvailabilityAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/availability/away", h.MarkAwayAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/availability/away/clear", h.ClearAwayAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/availability/exceptions", h.ListMyExceptionsAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/availability/exceptions", h.AddExceptionAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/availability/exceptions", h.ReplaceDayExceptionsAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.DELETE("/availability/exceptions/:eid", h.DeleteExceptionAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// Slot proposals. Same member-only gating: any member (Player+) may view a
	// proposal and respond to its options; only Scribe+ may create one. All
	// ride the authed cg group — NEVER the public pub group; the only public
	// proposal route is the emailed token below.
	cg.GET("/proposals", h.ListProposals, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/proposals", h.CreateProposalAPI, scribeOrCoDirector())
	cg.GET("/proposals/:pid", h.ShowProposal, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/proposals/:pid/options/:oid/respond", h.RespondOptionAPI, campaigns.RequireRole(campaigns.RolePlayer))
	// Confirm-winner: Scribe+ or a co-Director picks the winning option,
	// which closes the proposal and mints a planned session from that slot.
	cg.POST("/proposals/:pid/confirm", h.ConfirmProposalAPI, scribeOrCoDirector())

	// Public-capable view routes.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "calendar"),
		addons.RequireAddon(addonSvc, SessionsAddonSlug),
	)
	pub.GET("/sessions", h.ListSessions, campaigns.RequireViewAccess())
	pub.GET("/sessions/:sid", h.ShowSession, campaigns.RequireViewAccess())
	pub.GET("/sessions/embed", h.EmbedSessions, campaigns.RequireViewAccess())
	pub.GET("/game-nights", h.GameNightsLink, campaigns.RequireViewAccess())

	// RSVP token redemption — public endpoint, no auth required (token is the
	// credential, emailed to the user). GET renders a confirm interstitial; POST
	// applies. Splitting them stops mail scanners / link prefetchers from
	// auto-RSVPing via a background GET.
	e.GET("/rsvp/:token", h.RedeemRSVPToken)
	e.POST("/rsvp/:token", h.ApplyRSVPToken)

	// Proposal one-click response redemption — public, mirrors the RSVP token
	// route's placement + hygiene. Same GET-confirm / POST-apply split, and the
	// redeem additionally rechecks the proposal is still open + the user is
	// still a member.
	e.GET("/proposals/respond/:token", h.RedeemProposalToken)
	e.POST("/proposals/respond/:token", h.ApplyProposalToken)

	// "Suggest another time" — same public GET-confirm/POST-apply split as
	// every other token route above (a mail scanner's background GET must
	// never write); see ValidateAndRecordSuggestion for the validate-before-
	// consume ordering this exists to fix.
	e.GET("/rsvp/:token/suggest", h.RedeemSuggestToken)
	e.POST("/rsvp/:token/suggest", h.ApplySuggestToken)

	// Personal game-night calendar feed (operator answer #1) — public, the
	// token IS the credential, exactly like /rsvp/:token.
	e.GET("/sessions/feed/:token", h.GameNightFeedICS)

	// Scheduler notifications. User-scoped, not campaign-scoped — the topbar
	// bell is global — so these ride a plain authenticated group, not the
	// calendar campaign group. Every read/write is scoped to the caller.
	ng := e.Group("", auth.RequireAuth(authSvc))
	ng.GET("/notifications", h.ListNotificationsAPI)
	ng.GET("/notifications/badge", h.NotificationBadgeAPI)
	// The player call-to-action banner. Same group, same caller-scoping and
	// same empty-body-means-empty contract as the badge beside it — it is the
	// badge's louder sibling for the one thing a player is expected to ACT on
	// rather than merely read.
	ng.GET("/notifications/call-to-action", h.CalloutAPI)
	ng.POST("/notifications/:nid/read", h.MarkNotificationReadAPI)
	ng.POST("/notifications/read-all", h.MarkAllNotificationsReadAPI)
}
