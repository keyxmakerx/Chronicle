package timeline

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all timeline-related routes.
// Timeline routes are scoped to a campaign and require membership.
// CRUD and event management require Owner/Scribe roles; viewing requires Player.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	// Authenticated routes (create, update, delete, link/unlink events).
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "timeline"),
	)

	// Timeline CRUD (Owner only for create/update/delete).
	cg.POST("/timelines", h.CreateForm, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/timelines/:tid", h.UpdateAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/timelines/:tid", h.DeleteAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Calendar list for create form (Owner only — needed for calendar selector).
	cg.GET("/timelines/calendars", h.ListCalendarsAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Event linking (Scribe+).
	cg.POST("/timelines/:tid/events", h.LinkEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.POST("/timelines/:tid/events/all", h.LinkAllEventsAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/timelines/:tid/events/:eid", h.UnlinkEventAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Available events list for event picker (Scribe+).
	cg.GET("/timelines/:tid/available-events", h.ListAvailableEventsAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Standalone event CRUD (Scribe+).
	cg.POST("/timelines/:tid/standalone-events", h.CreateStandaloneEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/timelines/:tid/standalone-events/:eid", h.UpdateStandaloneEventAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/timelines/:tid/standalone-events/:eid", h.DeleteStandaloneEventAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Entity group CRUD (Owner only — swim-lane management).
	cg.GET("/timelines/:tid/groups", h.ListEntityGroupsAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/timelines/:tid/groups", h.CreateEntityGroupAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/timelines/:tid/groups/:gid", h.UpdateEntityGroupAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/timelines/:tid/groups/:gid", h.DeleteEntityGroupAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Entity group member management (Owner only).
	cg.POST("/timelines/:tid/groups/:gid/members", h.AddGroupMemberAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/timelines/:tid/groups/:gid/members/:eid", h.RemoveGroupMemberAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Event connections (Scribe+).
	cg.GET("/timelines/:tid/connections", h.ListConnectionsAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.POST("/timelines/:tid/connections", h.CreateConnectionAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/timelines/:tid/connections/:cid", h.DeleteConnectionAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Visibility management (Owner only).
	cg.PUT("/timelines/:tid/visibility", h.UpdateTimelineVisibilityAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/timelines/:tid/events/:eid/visibility", h.UpdateEventVisibilityAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/timelines/:tid/standalone-events/:eid/visibility", h.UpdateStandaloneEventVisibilityAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/timelines/members", h.ListCampaignMembersAPI, campaigns.RequireRole(campaigns.RoleOwner))

	// Public-capable views: timeline list, show, data endpoint.
	// Use AllowPublicCampaignAccess so HTMX lazy-loads work correctly.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "timeline"),
	)
	pub.GET("/timelines", h.Index, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/timelines/:tid", h.Show, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/timelines/:tid/data", h.TimelineDataAPI, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/timelines/preview", h.PreviewAPI, campaigns.RequireRole(campaigns.RolePlayer))
}
