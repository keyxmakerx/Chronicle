package calendar

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// scheduleAskActorBurst is the third rate-limit layer ([PB-4]): the cheap outer
// guard against a stuck client or a repeated submit, at 10 per hour per ACTOR.
//
// Per-USER, not per-IP, and that is the whole reason it is not
// middleware.RateLimit: the actor here is authenticated, and a table shares an
// IP (one house, one hotspot, one convention hall) far more often than it
// shares an account.
const scheduleAskActorBurst = 10

// calendarUserRateLimit is a per-authenticated-user sliding-window limiter,
// answering 429 with Retry-After.
//
// LIFTED, NOT IMPORTED. bestiary.UserRateLimit is the same algorithm, but
// importing it would be a plugin-to-plugin edge that
// internal/wire/plugin_import_guard_test.go forbids outright. The other
// candidate home — internal/middleware, beside RateLimit — is ALSO unavailable,
// and for a harder reason than taste: a per-user limiter has to read the
// session, auth.GetUserID lives in the auth plugin, and auth/handler.go already
// imports internal/middleware. Putting it there would be an import cycle. So it
// lives here, in the plugin that needs it, which is the choice this slice made
// and is reporting.
//
// Unauthenticated requests fall through: this middleware runs behind
// RequireAuth, so an empty user id means the auth layer is already answering.
func calendarUserRateLimit(maxRequests int, window time.Duration) echo.MiddlewareFunc {
	type entry struct {
		count       int
		windowStart time.Time
	}
	var mu sync.Mutex
	entries := make(map[string]*entry)

	// Bounded memory: a user whose window is two windows stale is forgotten.
	go func() {
		for {
			time.Sleep(time.Minute)
			mu.Lock()
			now := time.Now()
			for key, e := range entries {
				if now.Sub(e.windowStart) > window*2 {
					delete(entries, key)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID := auth.GetUserID(c)
			if userID == "" {
				return next(c)
			}
			now := time.Now()

			mu.Lock()
			e, ok := entries[userID]
			if !ok || now.Sub(e.windowStart) > window {
				entries[userID] = &entry{count: 1, windowStart: now}
				mu.Unlock()
				return next(c)
			}
			e.count++
			if e.count > maxRequests {
				retryAfter := int(window.Seconds() - now.Sub(e.windowStart).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				mu.Unlock()
				c.Response().Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				return c.JSON(http.StatusTooManyRequests, map[string]string{
					"error":   "rate_limited",
					"message": fmt.Sprintf("Too many requests. Try again in %d seconds.", retryAfter),
				})
			}
			mu.Unlock()
			return next(c)
		}
	}
}

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

	// ONE READ, AND THE WHOLE ROUTE BUDGET OF C-CALV4-DAYCARD SPENT ON IT
	// ([DC-8] SIGNED, R2-2a). The v4 event editor needs the event's full record
	// in EDIT mode — description, times, end date, category, visibility — and
	// the day card's page payload deliberately does not carry any of it
	// ([DC-1]: the Ledger row's field set and nothing more, because a payload
	// that carried every event's prose would ship it to every viewer's DOM).
	// No GET for a single event existed: the calendar group's GETs cover
	// event-categories, entities, rsvps and the upcoming fragment, and none
	// returns an event record.
	//
	// LITERAL PATH, on the SAME cg stack its siblings ride (RequireAuth →
	// RequireCampaignAccess → RequireAddon("calendar")). A route registered
	// through a variable is silently skipped by the wire snapshot
	// (wire_contract_test.go:180-188), so the path is written out.
	//
	// THE ROLE FLOOR IS NOT THE SECURITY BOUNDARY and RolePlayer is deliberate:
	// a player may legitimately read an event they can already see — the card
	// lists it and the Ledger prints it. What gates the body is the SAME viewer
	// filter the grid uses, applied inside the handler, plus a second gate on
	// the two audience fields. See GetEventAPI.
	cg.GET("/calendars/:calId/events/:eid", h.GetEventAPI, campaigns.RequireRole(campaigns.RolePlayer))

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
	pub.GET("/calendars/:calId/embed", h.EmbedCalendar, campaigns.RequireViewAccess())   // PRESERVE: no V2 embed
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

	// The calendar-v4 Block's per-viewer LAYER preference (C-CALV4-LAYERS-P9
	// [LYR-4 SIGNED]) — the one new route the whole calendar-v4 wave takes.
	//
	// NOT under /calendar/v2/…, deliberately. Its two nearest siblings live in
	// the FROZEN V2 shell's namespace (calendar_v2.templ and friends are frozen
	// and C-CALV4-SHELF-P7 confirmed the freeze held). The Block outlives that
	// shell, so its preference endpoint is not named after it.
	//
	// LITERAL PATH, on purpose: wire_contract_test.go skips and logs a route
	// registered through a variable, so a non-literal path would be silently
	// absent from routes_snapshot.txt — an auth surface outside the oracle.
	//
	// Player floor: a layer is a per-viewer DISPLAY PREFERENCE and every
	// campaign member has one. It rides the identical `cg` stack as the
	// sidebar-pin sibling (RequireAuth → RequireCampaignAccess →
	// RequireAddon("calendar")), which is what puts user_id in the session and
	// campaign_id behind an authorisation check. See handler_v2.go's
	// BlockPrefsAPI for why the response has no body.
	cg.POST("/calendar/prefs", h.BlockPrefsAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// V2 sub-resource card grids (Wave 1 PR 2 / C-CAL-V2-SUBRESOURCE-CARDS-A).
	// Read-only render is Player+ (the cards display data anyone with
	// campaign access can see). Edit affordances (drawer + add + dnd)
	// are gated client-side by IsOwner; mutations go through the
	// existing V1 PUT endpoints which retain Owner-only auth.
	cg.GET("/calendar/v2/:calId/settings/:resource", h.ShowV2SubresourceSettings, campaigns.RequireRole(campaigns.RolePlayer))
}

// RegisterRSVPRoutes mounts the event-RSVP surface (C-CAL-RSVP-P1).
//
// Kept as its OWN registration function rather than extra parameters on
// RegisterRoutes: the RSVP handler is a separate type with its own cross-plugin
// seams, and widening RegisterRoutes' signature would churn every existing
// caller and route test for no behavioural gain. The authenticated group is
// constructed with the IDENTICAL middleware stack as `cg` above — auth,
// campaign access, and the calendar addon guard — so these routes are gated
// exactly like every other calendar endpoint.
//
// Wire-contract note: adding routes updates internal/wire/routes_snapshot.txt
// (TestWireContractConformance), regenerated in the same commit.
func RegisterRSVPRoutes(e *echo.Echo, h *RSVPHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "calendar"),
	)

	// Answering is Player+ — every member who can SEE the event may respond, and
	// the handler writes only the caller's own row (user id from the session,
	// never the body). Reading is Player+ too, but the per-person breakdown
	// inside the payload is Owner/co-DM-gated in the handler: members see the
	// counts, the Director sees who.
	cg.POST("/calendars/:calId/events/:eid/rsvp", h.SetEventRSVPAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/calendars/:calId/events/:eid/rsvps", h.GetEventRSVPsAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// The "Collect RSVPs" opt-in is Scribe+ — the same gate as editing the event
	// it lives on. Enabling is the invite moment (email + bell fan-out), so it
	// must not be reachable by the people being invited.
	cg.PUT("/calendars/:calId/events/:eid/rsvp-collection", h.SetRSVPCollectionAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// The schedule ask (C-CALV4-RSVP-P8B, [PB-3]). Scribe+, identical to the
	// collection toggle it is the re-send of, and for the same stated reason:
	// this is an invite moment, so it must not be reachable by the people being
	// invited. Campaign-scoped rather than per-event, because the ask is about
	// the roster's schedule and does not require an event to exist; it sits in
	// the /campaigns/:id/calendar/<noun> namespace P9 established with
	// /calendar/prefs, deliberately not under /calendar/v2/… (the frozen V2
	// shell) and deliberately not under /schedule… (Part B's namespace).
	//
	// LITERAL PATH: wire_contract_test.go skips and logs a route registered
	// through a variable, so a non-literal path would be an auth surface outside
	// the oracle.
	//
	// The per-ACTOR limiter is the third rate-limit layer and the only
	// in-memory one; the campaign cooldown and the per-recipient floor are
	// persisted and enforced in the handler.
	cg.POST("/calendar/ask", h.AskAvailabilityAPI,
		campaigns.RequireRole(campaigns.RoleScribe),
		calendarUserRateLimit(scheduleAskActorBurst, time.Hour))

	// Public single-use token flow, mounted at the ROOT and deliberately
	// distinct from the sessions plugin's /rsvp/:token. Registered directly on
	// the Echo instance (not `cg`, not `pub`) because an emailed link carries no
	// campaign in its path and its recipient may be logged out — exactly how
	// sessions mounts its own token routes (sessions/routes.go:75-76). The token
	// is the sole credential; the handler re-checks membership AND event
	// visibility at redemption, fail-closed. GET is a pure read (confirm
	// interstitial / suggestion form); POST applies. Both ride the global CSRF
	// middleware, so the GET mints the cookie the POST double-submits.
	e.GET("/calendar-rsvp/:token", h.RedeemEventRSVPToken)
	e.POST("/calendar-rsvp/:token", h.ApplyEventRSVPToken)
}

// legacyRedirect 301s the bare /campaigns/:id/calendar to V2 (C-CAL-V1-V2-
// CUTOVER — was → V1 /calendars). V2 resolves the active calendar (or shows the
// empty-state setup link). Preserves bookmarks + external links.
func (h *Handler) legacyRedirect(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	return c.Redirect(301, "/campaigns/"+cc.Campaign.ID+"/calendar/v2")
}
