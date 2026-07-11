package calendar

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all calendar-related routes.
// Calendar routes are scoped to a campaign and require membership.
// Routes use the plural /calendars/:calId pattern supporting multiple calendars.
// Setup and settings require Owner role; viewing requires Player role.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	// Authenticated routes (create, settings, events, advance).
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "calendar"),
	)

	// Calendar creation + import from setup (Owner only).
	// C-CAL-V1-V2-CUTOVER: /calendars/new is the stable setup-chooser route.
	// Index now 301s any campaign WITH calendars to V2, so the "New calendar"
	// affordances (+ V2's empty state) need a dedicated GET to add an additional
	// calendar without bouncing through the redirect. Static segment, so it wins
	// over /calendars/:calId in Echo's router.
	cg.GET("/calendars/new", h.ShowSetup, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars", h.CreateCalendar, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/import-setup", h.ImportFromSetupAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Dashboard widget quick-add: creates event on default calendar (no calId).
	cg.POST("/calendars/events", h.CreateEventAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Per-calendar routes requiring Owner role.
	cg.DELETE("/calendars/:calId", h.DeleteCalendarAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/calendars/:calId/settings", h.ShowSettings, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendars/:calId/settings", h.UpdateCalendarAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendars/:calId/months", h.UpdateMonthsAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendars/:calId/weekdays", h.UpdateWeekdaysAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendars/:calId/moons", h.UpdateMoonsAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendars/:calId/seasons", h.UpdateSeasonsAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendars/:calId/eras", h.UpdateErasAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/calendars/:calId/event-categories", h.GetEventCategoriesAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendars/:calId/event-categories", h.UpdateEventCategoriesAPI, campaigns.RequireRole(campaigns.RoleOwner))
	// C-CAL-WCF-UI: internal UI bindings for weather, cycles, festivals.
	// Data layer / service / syncapi were already shipped; these three
	// PUTs are what the settings page calls when the operator clicks
	// save. Errors emit the wire-contract `{error, message, category}`
	// body via respondSettingsError so the inline error region in each
	// form gets a structured payload to render.
	cg.PUT("/calendars/:calId/weather", h.UpdateWeatherAPI, campaigns.RequireRole(campaigns.RoleOwner))
	// Weather zones (C-CAL-WEATHER-ZONES, Wave 0 PR 3): per-calendar
	// climate region catalog. GET is Player+ (Foundry-side weather
	// picker reads zone labels); PUT is Owner-only (catalog edit).
	cg.GET("/calendars/:calId/weather/zones", h.GetWeatherZonesAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/calendars/:calId/weather/zones", h.UpdateWeatherZonesAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendars/:calId/cycles", h.UpdateCyclesAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendars/:calId/festivals", h.UpdateFestivalsAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Advance date/time (Owner only — GMs advance time during play).
	cg.POST("/calendars/:calId/advance", h.AdvanceDateAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/:calId/advance-time", h.AdvanceTimeAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Import/export (Owner only).
	cg.GET("/calendars/:calId/export", h.ExportCalendarAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/:calId/import", h.ImportCalendarAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendars/:calId/import/preview", h.ImportPreviewAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Sessions fragment for the calendar sessions modal (Player+).
	cg.GET("/calendars/:calId/sessions-fragment", h.SessionsFragment, campaigns.RequireRole(campaigns.RolePlayer))

	// Events CRUD (Scribe+ can create/edit, Owner can delete/set visibility).
	cg.POST("/calendars/:calId/events", h.CreateEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/calendars/:calId/events/:eid", h.UpdateEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/calendars/:calId/events/:eid/visibility", h.UpdateEventVisibilityAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/calendars/:calId/events/:eid", h.DeleteEventAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Per-calendar visibility (C-CAL-DASHBOARD-W5b). Owner/co-DM only — same
	// capability gate the world-state PUT uses, so a co-DM can manage who sees
	// a calendar. Players never reach this (nor the editor DOM).
	cg.PUT("/calendars/:calId/visibility", h.UpdateCalendarVisibilityAPI,
		campaigns.RequireCapability((*campaigns.CampaignContext).CanControlWorldState,
			"managing calendar visibility requires Owner or co-DM access"))

	// Event<->entity ties (C-CAL-WORLDSTATE-PRODUCTION-PORT 2b): the
	// production attach-entity picker persists real entity_event_links
	// (#402) with a participation_role. Read = Player+ (the picker chips
	// render for anyone who can see the event); attach/detach = Scribe+
	// (same gate as event edit). IDOR closed via requireEventInCampaign.
	cg.GET("/calendars/:calId/events/:eid/entities", h.ListEventEntitiesAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/calendars/:calId/events/:eid/entities/:entityId", h.LinkEventEntityAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/calendars/:calId/events/:eid/entities/:entityId", h.UnlinkEventEntityAPI, campaigns.RequireRole(campaigns.RoleScribe))
	// "Create entity from event" drawer action (C-CAL-EDITOR-EXPANSION PR1):
	// creates a campaign entity named after the event + links it. Scribe+ (the
	// drawer's gate); IDOR closed via requireEventInCampaign.
	cg.POST("/calendars/:calId/events/:eid/create-entity", h.CreateEntityFromEventAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Public-capable views: calendar list, grid, timeline, upcoming events, and
	// entity-event fragments are viewable by players and public campaigns.
	// These must use AllowPublicCampaignAccess so HTMX lazy-loads from
	// the dashboard and entity pages (which use OptionalAuth) work correctly.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "calendar"),
	)
	// C-CAL-V1-V2-CUTOVER: the V1 calendar VIEW routes 301 to V2 (full parity,
	// W1 fixed week/day). Index keeps its 0-calendar setup branch (no V2 create
	// flow yet) and 301s any campaign WITH calendars to V2. Show/week/day are
	// thin redirects. The shared API/data routes above are unchanged (V2's
	// backend). PRESERVED (no V2 equivalent yet): the TIMELINE (Timeline V2 is a
	// deferred arc) and the standalone EMBED page — both kept until their V2
	// surfaces exist.
	pub.GET("/calendars", h.Index, campaigns.RequireViewAccess())
	pub.GET("/calendars/:calId", h.RedirectShowV2, campaigns.RequireViewAccess())
	pub.GET("/calendars/:calId/embed", h.EmbedCalendar, campaigns.RequireViewAccess()) // PRESERVE: no V2 embed
	pub.GET("/calendars/:calId/timeline", h.ShowTimeline, campaigns.RequireViewAccess()) // PRESERVE: Timeline V2 deferred
	pub.GET("/calendars/:calId/week", h.RedirectWeekV2, campaigns.RequireViewAccess())
	pub.GET("/calendars/:calId/day", h.RedirectDayV2, campaigns.RequireViewAccess())
	pub.GET("/calendars/:calId/upcoming", h.UpcomingEventsFragment, campaigns.RequireViewAccess()) // PRESERVE: fragment loader

	// Dashboard block routes: no calId, handlers fall back to default calendar.
	pub.GET("/calendars/embed", h.EmbedCalendar, campaigns.RequireViewAccess())
	pub.GET("/calendars/upcoming", h.UpcomingEventsFragment, campaigns.RequireViewAccess())

	// (The /calendars/entity-events/:eid fragment + its EntityEventsFragment
	// handler were retired in C-CAL-EMBED-CONVERGE-POLISH — the per-entity
	// calendar is now the registry-driven `entity_calendar` block.)

	// Backward-compat routes: redirect old /calendar paths to /calendars.
	// Carries RequireViewAccess() for uniformity with every other route on this
	// public group (C-PUBLIC-VIEW-FIX-R2). It is redirect-only and leaks nothing,
	// but the route-gate sweep test requires every public-group route to declare
	// its view gate explicitly rather than relying on the group middleware.
	pub.GET("/calendar", h.legacyRedirect, campaigns.RequireViewAccess())

	// C-CAL-V1-V2-CUTOVER FIX (cordinator#30): the V2 calendar shell is now the
	// ONLY live calendar view — every V1 view route above 301s here, so this is
	// where a public-campaign visitor actually lands. It is a READ surface
	// (Player+, dm_only filtered by role in the handler), so it MUST live in the
	// public-capable group like the V1 routes it replaced; otherwise a logged-out
	// visitor to a PUBLIC campaign is bounced through the redirect into
	// RequireAuth → 401 → /login. The mutating switch/pin POSTs + the settings
	// editor stay authenticated (in cg) below.
	//   GET /campaigns/:id/calendar/v2              — active cal, default view = month
	//   GET /campaigns/:id/calendar/v2/:calId       — explicit cal, default view = month
	//   GET /campaigns/:id/calendar/v2/:calId/:view — explicit cal + view (month|week|day)
	pub.GET("/calendar/v2", h.ShowV2, campaigns.RequireViewAccess())
	pub.GET("/calendar/v2/:calId", h.ShowV2, campaigns.RequireViewAccess())
	pub.GET("/calendar/v2/:calId/:view", h.ShowV2, campaigns.RequireViewAccess())

	// World-state seed GET (C-CAL-WORLDSTATE-SERVER-MODEL). Player+ READ — the
	// worldstate band lazy-loads this on the public calendar + entity-embed
	// surfaces, so it is public-capable (GM-only celestial events are filtered
	// for non-DM viewers in the seed builder). No calId: the handler resolves
	// the active calendar (or ?calendarId=). The PUT (set mood / advance time)
	// stays Owner/co-DM in cg below.
	pub.GET("/calendar/world-state", h.GetWorldState, campaigns.RequireViewAccess())

	// World-state seed PUT (set live mood + advance/set time). Co-DM capability
	// (C-CAL-COGM-CAPABILITY / D6): control is Owner OR DM-grantee, not
	// Owner-only — the #401 seam widened so a co-DM can drive the Phase-4 GM
	// panel. The matching Player+ READ (GET) is public-capable and registered in
	// the pub group above (cordinator#30). No calId: the handler resolves the
	// active calendar (or ?calendarId=).
	cg.PUT("/calendar/world-state", h.PutWorldState,
		campaigns.RequireCapability((*campaigns.CampaignContext).CanControlWorldState,
			"world-state control requires Owner or co-DM access"))

	// Calendars management dashboard (C-APPS-CAL-DASH-W1 / E1). A dedicated
	// page reached from the Extensions hub's per-app "Open dashboard" entry:
	// list + detail pane, composing the existing CRUD + a read-only
	// associations panel. Owner-only (management surface, mirrors the
	// Owner-gated Extensions hub). No :calId in the path — the selected
	// calendar rides ?calId= so list selection HTMX-swaps the detail pane.
	// W5c: Player+ (role-aware) — owners manage all calendars, players see the
	// read-only card grid of the calendars visible to them (per-calendar
	// visibility, W5a). The handler branches on role; no data leaks (the
	// player's list is ListVisibleCalendars and a hidden ?calId is not loaded).
	cg.GET("/apps/calendar", h.AppDashboard, campaigns.RequireRole(campaigns.RolePlayer))

	// V2 calendar shell mutating / per-user routes (C-CAL-V2-SHELL-FOUNDATION).
	// The READ views (GET …/calendar/v2[/:calId[/:view]]) are public-capable and
	// registered in the pub group above (cordinator#30); these POSTs persist
	// per-user state, so they stay authenticated.
	//   POST /campaigns/:id/calendar/v2/switch       — persist multi-cal switcher choice
	//   POST /campaigns/:id/calendar/v2/sidebar-pin  — persist sidebar pin preference (Wave 1.7A §G)
	cg.POST("/calendar/v2/switch", h.SwitchActiveCalendarAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/calendar/v2/sidebar-pin", h.SidebarPinAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// V2 sub-resource card grids (Wave 1 PR 2 / C-CAL-V2-SUBRESOURCE-CARDS-A).
	// Read-only render is Player+ (the cards display data anyone with
	// campaign access can see). Edit affordances (drawer + add + dnd)
	// are gated client-side by IsOwner; mutations go through the
	// existing V1 PUT endpoints which retain Owner-only auth.
	cg.GET("/calendar/v2/:calId/settings/:resource", h.ShowV2SubresourceSettings, campaigns.RequireRole(campaigns.RolePlayer))
}

// legacyRedirect 301s the bare /campaigns/:id/calendar to V2 (C-CAL-V1-V2-
// CUTOVER — was → V1 /calendars). V2 resolves the active calendar (or shows the
// empty-state setup link). Preserves bookmarks + external links.
func (h *Handler) legacyRedirect(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	return c.Redirect(301, "/campaigns/"+cc.Campaign.ID+"/calendar/v2")
}
