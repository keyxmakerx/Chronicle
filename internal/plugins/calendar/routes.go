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
//   - Event kinds, eras and the moon hidden flag are calendar STRUCTURE (no
//     Player read route for any of the three; eras reach a player only
//     inside the calendar read, filtered). Listing event kinds stays
//     Owner only; creating/editing/deleting an event kind or an era, and
//     the moon hidden flag, are gated CanAuthorDmOnly like the event
//     visibility toggle above — the Owner or a granted co-Director, never a
//     plain Scribe.
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

	// --- Calendar creation wizard (presets, import) ---
	// Owner only, matching the calendar CRUD block above: creating a
	// calendar's initial structure from a preset or an uploaded file is
	// calendar structure, not content. "presets" and "import" are static
	// first segments at the same path depth as "list" and ":calid" below —
	// see that block's own comment on why a static segment always wins and
	// none of these can collide with a real (UUID) :calid.
	cg.GET("/calendars/presets", h.ListPresetsAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/calendars/presets/:name", h.PreviewPresetAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/presets/:name", h.CreateFromPresetAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/import/preview", h.PreviewImportAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/import", h.CreateFromImportAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// --- Calendar creation wizard and list UI (HTML) ---
	//
	// HTML page/fragment routes, distinct from the JSON API block above —
	// "wizard" is a static segment at the same path depth as "presets",
	// "import" and ":calid" everywhere else in this file, so it can never
	// collide with a real (UUID) :calid the same way "list" and "presets"
	// already don't (see this file's earlier comments). Every wizard route
	// is Owner only, matching the calendar CRUD block above: creating a
	// calendar's initial structure is calendar structure. The list page is
	// Player+ (read access, filtered by the service's own viewer-aware
	// visibility — see list_handler.go).
	cg.GET("/calendars", h.Index, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/calendars/wizard", h.WizardStart, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/calendars/wizard/presets", h.WizardPresets, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/calendars/wizard/presets/:name", h.WizardPresetReview, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/calendars/wizard/reallife", h.WizardRealWorldReview, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/calendars/wizard/build", h.WizardBuildStep, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/wizard/build/preview", h.WizardBuildPreview, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/calendars/wizard/generate", h.WizardGenerateStep, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/wizard/generate/preview", h.WizardGeneratePreview, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/calendars/wizard/import", h.WizardImportStep, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/wizard/import/preview", h.WizardImportPreview, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/wizard/create", h.WizardCreate, campaigns.RequireRole(campaigns.RoleOwner))

	// Edit structure: an existing calendar's months, weekdays, leap rule,
	// moons and seasons, previewed before saving. Owner only, the same gate
	// as the wizard that first set them and PUT /calendars/:calid above.
	cg.GET("/calendars/:calid/structure", h.StructureEditPage, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/:calid/structure/preview", h.StructureEditPreview, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/:calid/structure", h.StructureEditApply, campaigns.RequireRole(campaigns.RoleOwner))

	// Calendar reads (Player) — re-registered under the public-capable group
	// below, whose registration wins for the same path (see maps/routes.go's
	// /maps, /maps/:mid, /maps/:mid/markers for the identical shape): kept
	// here too so the authenticated-member surface is visible in this list
	// on its own.
	//
	// The list route is "/calendars/list", not bare "/calendars": that bare
	// GET already belongs to THIS file's own h.Index above (the
	// calendars list PAGE, and to routes_snapshot.txt) — a real page a
	// browser navigates to, not a JSON endpoint, so this API cannot reuse
	// it. No real calendar id is ever the literal string "list" (ids are
	// UUIDs), and a static segment always wins over a same-position ":calid"
	// param, so the two can't collide going the other way either.
	cg.GET("/calendars/list", h.ListCalendarsAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/calendars/:calid", h.GetCalendarAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// Events: view Player, create/edit Scribe, delete + visibility Owner.
	cg.GET("/calendars/:calid/events", h.ListEventsAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/calendars/:calid/events/:eid", h.GetEventAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/calendars/:calid/eras/:eraID/events", h.ListEraEventsAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/calendars/:calid/events", h.CreateEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/calendars/:calid/events/:eid", h.UpdateEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/calendars/:calid/events/:eid", h.DeleteEventAPI, campaigns.RequireRole(campaigns.RoleOwner))
	// "This one only": skip or move one occurrence of a repeating event, or
	// undo that. Gated like editing the event (Scribe+; the service also
	// requires the event be visible to the caller).
	cg.PUT("/calendars/:calid/events/:eid/occurrences/:y/:m/:d", h.SetOccurrenceAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/calendars/:calid/events/:eid/occurrences/:y/:m/:d", h.DeleteOccurrenceAPI, campaigns.RequireRole(campaigns.RoleScribe))
	// The rule editor's "next few dates": gated like a read, writes nothing;
	// the service refuses a rule naming anything the caller cannot see.
	cg.POST("/calendars/:calid/recurrence/preview", h.PreviewRecurrenceAPI, campaigns.RequireRole(campaigns.RolePlayer))
	// The dm_only toggle is gated on CanAuthorDmOnly, not a bare role
	// minimum: the operator has decided a granted co-DM (Scribe role, plus
	// the dm_only grant) may use this the same as the Owner, so RequireRole
	// alone would wrongly exclude them.
	cg.PUT("/calendars/:calid/events/:eid/visibility", h.SetEventVisibilityAPI,
		campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool { return cc.CanAuthorDmOnly() },
			"only the campaign owner or a granted co-DM may change this event's visibility"))

	// Event kinds: campaign structure (no Player read). Listing stays Owner
	// only; creating, editing and deleting a kind are gated CanAuthorDmOnly,
	// not a bare role minimum — the operator has decided a granted co-DM may
	// author calendar structure the same as the Owner, matching the eras and
	// moon-hidden writes below and the event-visibility toggle further up.
	cg.GET("/calendars/event-kinds", h.ListEventKindsAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/event-kinds", h.CreateEventKindAPI,
		campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool { return cc.CanAuthorDmOnly() },
			"only the campaign owner or a granted co-DM may create an event kind"))
	cg.PUT("/calendars/event-kinds/:kindID", h.UpdateEventKindAPI,
		campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool { return cc.CanAuthorDmOnly() },
			"only the campaign owner or a granted co-DM may edit an event kind"))
	cg.DELETE("/calendars/event-kinds/:kindID", h.DeleteEventKindAPI,
		campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool { return cc.CanAuthorDmOnly() },
			"only the campaign owner or a granted co-DM may delete an event kind"))

	// Eras: no read route of their own; they ship inside the calendar's own
	// read, filtered for the viewer (an era hidden until it begins, and the
	// Director's notes, never reach a player). Writes are gated
	// CanAuthorDmOnly, same reasoning as the event-kind writes above.
	cg.POST("/calendars/:calid/eras", h.CreateEraAPI,
		campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool { return cc.CanAuthorDmOnly() },
			"only the campaign owner or a granted co-DM may create an era"))
	cg.PUT("/calendars/:calid/eras/:eraID", h.UpdateEraAPI,
		campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool { return cc.CanAuthorDmOnly() },
			"only the campaign owner or a granted co-DM may edit an era"))
	cg.DELETE("/calendars/:calid/eras/:eraID", h.DeleteEraAPI,
		campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool { return cc.CanAuthorDmOnly() },
			"only the campaign owner or a granted co-DM may delete an era"))
	// The era look (colours behind the days, feel, each era's colours and
	// style) is part of the calendar settings, so Owner only like PUT
	// /calendars/:calid and the structure editor.
	cg.PUT("/calendars/:calid/era-look", h.SaveEraLookAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Moon hidden flag: calendar structure, gated CanAuthorDmOnly like the
	// event-kind and era writes above.
	cg.PUT("/calendars/:calid/moons/:moonID/hidden", h.SetMoonHiddenAPI,
		campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool { return cc.CanAuthorDmOnly() },
			"only the campaign owner or a granted co-DM may change a moon's visibility"))

	// Day weather: one reading per day. Read Player (the service hides
	// future days from anyone who can't see dm_only content); painting,
	// storing generated weather and clearing are gated CanAuthorDmOnly like
	// the moon hidden flag above, since a write can reveal future weather.
	cg.GET("/calendars/:calid/weather/days", h.ListDayWeatherAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/calendars/:calid/weather/days", h.SetDayWeatherAPI,
		campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool { return cc.CanAuthorDmOnly() },
			"only the campaign owner or a granted co-DM may set a day's weather"))
	cg.GET("/calendars/:calid/weather/settings", h.GetWeatherSettingsAPI,
		campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool { return cc.CanAuthorDmOnly() },
			"only the campaign owner or a granted co-DM may read the weather settings"))
	cg.POST("/calendars/:calid/weather/days/clear", h.ClearDayWeatherAPI,
		campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool { return cc.CanAuthorDmOnly() },
			"only the campaign owner or a granted co-DM may clear a day's weather"))

	// Real-date anchor preview: read-only, Owner only (moving the anchor
	// re-dates every session scheduled by in-world date at once, so the
	// operator gets a preview of the blast radius before committing to it;
	// the real anchor WRITE this previews is a separate, not-yet-built
	// endpoint).
	cg.POST("/calendars/:calid/anchor-preview", h.AnchorPreviewAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Public-capable reads: calendar list/detail and event list/detail, for
	// the embeddable calendar widget / entity-calendar blocks on public
	// campaigns. Role/visibility filtering happens in the service
	// (permissions.Viewer, ADR-049) and is empty-userID safe for anonymous
	// visitors. Never dm_only data, never hidden moons, never event kinds
	// (Owner-only regardless of campaign visibility), never an era hidden
	// until it begins or a Director's era note.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, PluginSlug),
	)
	// Dashboard/category-dashboard "upcoming events" embed fragment — a
	// static "/calendars/upcoming" segment, so it can never collide with the
	// "/calendars/:calid" param route below it (same reasoning as
	// "/calendars/list" above).
	pub.GET("/calendars/upcoming", h.PreviewUpcomingEvents, campaigns.RequireViewAccess())
	pub.GET("/calendars/list", h.ListCalendarsAPI, campaigns.RequireViewAccess())
	pub.GET("/calendars/:calid", h.GetCalendarAPI, campaigns.RequireViewAccess())
	pub.GET("/calendars/:calid/events", h.ListEventsAPI, campaigns.RequireViewAccess())
	pub.GET("/calendars/:calid/events/:eid", h.GetEventAPI, campaigns.RequireViewAccess())
	pub.GET("/calendars/:calid/eras/:eraID/events", h.ListEraEventsAPI, campaigns.RequireViewAccess())
	pub.GET("/calendars/:calid/weather/days", h.ListDayWeatherAPI, campaigns.RequireViewAccess())

	// --- Calendar page (V5 part A, #741) ---
	// The month grid / day-card / era-card / event / moon page. A real page a
	// browser navigates to, so it's registered in both groups the same way
	// GET /calendars/:calid already is above: Player+ for members, and the
	// public-capable group for anonymous/public-campaign viewers.
	cg.GET("/calendars/:calid/view", h.CalendarViewPage, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendars/:calid/view", h.CalendarViewPage, campaigns.RequireViewAccess())
}
