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

	// Calendar setup + creation (Owner only).
	cg.POST("/calendar", h.CreateCalendar, campaigns.RequireRole(campaigns.RoleOwner))

	// Calendar settings API (Owner only).
	cg.PUT("/calendar/settings", h.UpdateCalendarAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendar/months", h.UpdateMonthsAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendar/weekdays", h.UpdateWeekdaysAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/calendar/moons", h.UpdateMoonsAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Advance date (Owner only — GMs advance time during play).
	cg.POST("/calendar/advance", h.AdvanceDateAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Events CRUD (Scribe+ can create/edit, Owner can delete).
	cg.POST("/calendar/events", h.CreateEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/calendar/events/:eid", h.DeleteEventAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Public-capable view: calendar page viewable by players and public campaigns.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/calendar", h.Show, campaigns.RequireRole(campaigns.RolePlayer))
}
