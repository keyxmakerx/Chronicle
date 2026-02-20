package audit

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all audit-related routes on the given Echo instance.
// All routes are campaign-scoped and require authentication plus campaign
// membership. The activity page is restricted to campaign owners (role >= 3).
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	// Campaign-scoped routes requiring authentication and membership.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// Activity page -- campaign owner only.
	cg.GET("/activity", h.Activity, campaigns.RequireRole(campaigns.RoleOwner))

	// Entity history -- any campaign member can view change history.
	cg.GET("/entities/:eid/history", h.EntityHistory, campaigns.RequireRole(campaigns.RolePlayer))
}
