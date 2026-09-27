package calendar

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all calendar-related routes. Calendar routes are
// scoped to a campaign, gated on the "calendar" addon, and require
// membership. Shape mirrors internal/plugins/maps/routes.go.
//
// Permissions (.ai/conventions.md):
//   - Calendar: view Player, create/edit/delete Owner.
//   - Calendar events: view Player (visibility-filtered), create/edit
//     Scribe, delete Owner, the dm_only toggle gated on CanAuthorDmOnly
//     (the Owner or a granted co-DM, never a plain Scribe).
//   - Event kinds, eras and the moon hidden flag are calendar STRUCTURE, so
//     Owner only end to end (no Player read route for any of the three).
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	// Authenticated routes (CRUD + structure management).
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, PluginSlug),
	)

	// Calendar CRUD (Owner only).
	cg.POST("/calendars", h.CreateCalendarAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendars/:calid", h.UpdateCalendarAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/calendars/:calid", h.DeleteCalendarAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendars/:calid/default", h.SetDefaultCalendarAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Calendar reads (Player) — re-registered under the public-capable group
	// below, whose registration wins for the same path (see maps/routes.go's
	// /maps, /maps/:mid, /maps/:mid/markers for the identical shape): kept
	// here too so the authenticated-member surface is visible in this list
	// on its own.
	//
	// The list route is "/calendars/list", not bare "/calendars": that bare
	// GET already belongs to app/routes.go's calendar-rebuild notice page
	// (and to routes_snapshot.txt) — a real page a browser navigates to, not
	// a JSON endpoint, so this API cannot reuse it. No real calendar id is
	// ever the literal string "list" (ids are UUIDs), and a static segment
	// always wins over a same-position ":calid" param, so the two can't
	// collide going the other way either.
	cg.GET("/calendars/list", h.ListCalendarsAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/calendars/:calid", h.GetCalendarAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// Events: view Player, create/edit Scribe, delete + visibility Owner.
	cg.GET("/calendars/:calid/events", h.ListEventsAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/calendars/:calid/events/:eid", h.GetEventAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/calendars/:calid/events", h.CreateEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/calendars/:calid/events/:eid", h.UpdateEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/calendars/:calid/events/:eid", h.DeleteEventAPI, campaigns.RequireRole(campaigns.RoleOwner))
	// The dm_only toggle is gated on CanAuthorDmOnly, not a bare role
	// minimum: the operator has decided a granted co-DM (Scribe role, plus
	// the dm_only grant) may use this the same as the Owner, so RequireRole
	// alone would wrongly exclude them.
	cg.PUT("/calendars/:calid/events/:eid/visibility", h.SetEventVisibilityAPI,
		campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool { return cc.CanAuthorDmOnly() },
			"only the campaign owner or a granted co-DM may change this event's visibility"))

	// Event kinds: campaign structure, Owner only end to end (no Player read).
	cg.GET("/calendars/event-kinds", h.ListEventKindsAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/event-kinds", h.CreateEventKindAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendars/event-kinds/:kindID", h.UpdateEventKindAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/calendars/event-kinds/:kindID", h.DeleteEventKindAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Eras: calendar structure, Owner only end to end (no Player read).
	cg.POST("/calendars/:calid/eras", h.CreateEraAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendars/:calid/eras/:eraID", h.UpdateEraAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/calendars/:calid/eras/:eraID", h.DeleteEraAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Moon hidden flag: calendar structure, Owner only.
	cg.PUT("/calendars/:calid/moons/:moonID/hidden", h.SetMoonHiddenAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// --- Part C: real-world calendar additions ---
	// Real-date anchor preview: read-only, Owner only (moving the anchor
	// re-dates every session scheduled by in-world date at once, so the
	// operator gets a preview of the blast radius before committing to it;
	// the real anchor WRITE this previews is a separate, not-yet-built
	// endpoint).
	cg.POST("/calendars/:calid/anchor-preview", h.AnchorPreviewAPI, campaigns.RequireRole(campaigns.RoleOwner))
	// --- end Part C additions ---

	// Public-capable reads: calendar list/detail and event list/detail, for
	// the embeddable calendar widget / entity-calendar blocks on public
	// campaigns. Role/visibility filtering happens in the service
	// (permissions.Viewer, ADR-049) and is empty-userID safe for anonymous
	// visitors. Never dm_only data, never hidden moons, never event kinds or
	// eras (those stay Owner-only regardless of campaign visibility).
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, PluginSlug),
	)
	pub.GET("/calendars/list", h.ListCalendarsAPI, campaigns.RequireViewAccess())
	pub.GET("/calendars/:calid", h.GetCalendarAPI, campaigns.RequireViewAccess())
	pub.GET("/calendars/:calid/events", h.ListEventsAPI, campaigns.RequireViewAccess())
	pub.GET("/calendars/:calid/events/:eid", h.GetEventAPI, campaigns.RequireViewAccess())
}
