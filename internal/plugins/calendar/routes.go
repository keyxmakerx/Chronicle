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

	// Public-capable views: calendar list, grid, timeline, upcoming events, and
	// entity-event fragments are viewable by players and public campaigns.
	// These must use AllowPublicCampaignAccess so HTMX lazy-loads from
	// the dashboard and entity pages (which use OptionalAuth) work correctly.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "calendar"),
	)
	pub.GET("/calendars", h.Index, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendars/:calId", h.Show, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendars/:calId/embed", h.EmbedCalendar, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendars/:calId/timeline", h.ShowTimeline, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendars/:calId/week", h.ShowWeek, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendars/:calId/day", h.ShowDay, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendars/:calId/upcoming", h.UpcomingEventsFragment, campaigns.RequireRole(campaigns.RolePlayer))

	// Dashboard block routes: no calId, handlers fall back to default calendar.
	pub.GET("/calendars/embed", h.EmbedCalendar, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendars/upcoming", h.UpcomingEventsFragment, campaigns.RequireRole(campaigns.RolePlayer))

	// Entity events fragment uses the default calendar (no calId needed).
	pub.GET("/calendars/entity-events/:eid", h.EntityEventsFragment, campaigns.RequireRole(campaigns.RolePlayer))

	// Backward-compat routes: redirect old /calendar paths to /calendars.
	pub.GET("/calendar", h.legacyRedirect)
}

// legacyRedirect handles the old /campaigns/:id/calendar route by redirecting
// to the new /campaigns/:id/calendars path. Preserves bookmarks and external links.
func (h *Handler) legacyRedirect(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	return c.Redirect(301, "/campaigns/"+cc.Campaign.ID+"/calendars")
}
