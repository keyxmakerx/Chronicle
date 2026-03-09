package calendar

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all calendar-related routes.
// Calendar routes are scoped to a campaign and require membership.
// Setup and settings require Owner role; viewing requires Player role.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	// Authenticated routes (create, settings, events, advance).
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "calendar"),
	)

	// Calendar setup + creation + deletion (Owner only).
	cg.POST("/calendar", h.CreateCalendar, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/calendar", h.DeleteCalendarAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Calendar settings page (Owner only).
	cg.GET("/calendar/settings", h.ShowSettings, campaigns.RequireRole(campaigns.RoleOwner))

	// Calendar settings API (Owner only).
	cg.PUT("/calendar/settings", h.UpdateCalendarAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendar/months", h.UpdateMonthsAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendar/weekdays", h.UpdateWeekdaysAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendar/moons", h.UpdateMoonsAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendar/seasons", h.UpdateSeasonsAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendar/eras", h.UpdateErasAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/calendar/event-categories", h.GetEventCategoriesAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendar/event-categories", h.UpdateEventCategoriesAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Advance date/time (Owner only — GMs advance time during play).
	cg.POST("/calendar/advance", h.AdvanceDateAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendar/advance-time", h.AdvanceTimeAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Import/export (Owner only).
	cg.GET("/calendar/export", h.ExportCalendarAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendar/import", h.ImportCalendarAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendar/import/preview", h.ImportPreviewAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/calendar/import-setup", h.ImportFromSetupAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Sessions fragment for the calendar sessions modal (Player+).
	cg.GET("/calendar/sessions-fragment", h.SessionsFragment, campaigns.RequireRole(campaigns.RolePlayer))

	// Events CRUD (Scribe+ can create/edit, Owner can delete/set visibility).
	cg.POST("/calendar/events", h.CreateEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/calendar/events/:eid", h.UpdateEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/calendar/events/:eid/visibility", h.UpdateEventVisibilityAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/calendar/events/:eid", h.DeleteEventAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Public-capable views: calendar grid, timeline, upcoming events, and
	// entity-event fragments are viewable by players and public campaigns.
	// These must use AllowPublicCampaignAccess so HTMX lazy-loads from
	// the dashboard and entity pages (which use OptionalAuth) work correctly.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "calendar"),
	)
	pub.GET("/calendar", h.Show, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendar/embed", h.EmbedCalendar, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendar/timeline", h.ShowTimeline, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendar/week", h.ShowWeek, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendar/day", h.ShowDay, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendar/upcoming", h.UpcomingEventsFragment, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendar/entity-events/:eid", h.EntityEventsFragment, campaigns.RequireRole(campaigns.RolePlayer))
}
