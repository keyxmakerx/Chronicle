package calendar

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all calendar-related routes.
// Calendar routes are scoped to a campaign and require membership.
// Setup and settings require Owner role; viewing requires Player role.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	// Authenticated routes (create, settings, events, advance).
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
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

	// Advance date (Owner only — GMs advance time during play).
	cg.POST("/calendar/advance", h.AdvanceDateAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Events CRUD (Scribe+ can create/edit, Owner can delete).
	cg.POST("/calendar/events", h.CreateEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/calendar/events/:eid", h.UpdateEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/calendar/events/:eid", h.DeleteEventAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Public-capable views: calendar grid, timeline, upcoming events, and
	// entity-event fragments are viewable by players and public campaigns.
	// These must use AllowPublicCampaignAccess so HTMX lazy-loads from
	// the dashboard and entity pages (which use OptionalAuth) work correctly.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/calendar", h.Show, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendar/timeline", h.ShowTimeline, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendar/upcoming", h.UpcomingEventsFragment, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/calendar/entity-events/:eid", h.EntityEventsFragment, campaigns.RequireRole(campaigns.RolePlayer))
}
