package sessions

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all session-related routes.
func RegisterRoutes(e *echo.Echo, h *Handler,
	campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {

	// Authenticated routes (create, update, delete, entity linking).
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	cg.POST("/sessions", h.CreateSession, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/sessions/:sid", h.UpdateSessionAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/sessions/:sid", h.DeleteSessionAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/sessions/:sid/rsvp", h.RSVPSession, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/sessions/:sid/entities", h.LinkEntityAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/sessions/:sid/entities/:eid", h.UnlinkEntityAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Public-capable view routes.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/sessions", h.ListSessions, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/sessions/:sid", h.ShowSession, campaigns.RequireRole(campaigns.RolePlayer))
}
