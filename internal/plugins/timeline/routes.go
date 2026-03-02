package timeline

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all timeline-related routes.
// Timeline routes are scoped to a campaign and require membership.
// CRUD and event management require Owner/Scribe roles; viewing requires Player.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	// Authenticated routes (create, update, delete, link/unlink events).
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// Timeline CRUD (Owner only for create/update/delete).
	cg.POST("/timelines", h.CreateForm, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/timelines/:tid", h.UpdateAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/timelines/:tid", h.DeleteAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Calendar list for create form (Owner only — needed for calendar selector).
	cg.GET("/timelines/calendars", h.ListCalendarsAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Event linking (Scribe+).
	cg.POST("/timelines/:tid/events", h.LinkEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/timelines/:tid/events/:eid", h.UnlinkEventAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Public-capable views: timeline list, show, data endpoint.
	// Use AllowPublicCampaignAccess so HTMX lazy-loads work correctly.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/timelines", h.Index, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/timelines/:tid", h.Show, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/timelines/:tid/data", h.TimelineDataAPI, campaigns.RequireRole(campaigns.RolePlayer))
}
