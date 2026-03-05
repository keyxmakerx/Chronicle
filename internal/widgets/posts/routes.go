package posts

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all entity post routes on the given Echo instance.
// Posts are scoped to a campaign and require campaign membership.
//
// Permissions:
//   - Player (read): list posts for an entity
//   - Scribe (write): create, update, delete, and reorder posts
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	// Write routes: Scribe or above can manage posts.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	cg.POST("/entities/:eid/posts", h.CreatePost, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/entities/:eid/posts/reorder", h.ReorderPosts, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/entities/:eid/posts/:pid", h.UpdatePost, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/entities/:eid/posts/:pid", h.DeletePost, campaigns.RequireRole(campaigns.RoleScribe))

	// Public-capable read routes: allow public campaign visitors to see posts.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/entities/:eid/posts", h.ListPosts, campaigns.RequireRole(campaigns.RolePlayer))
}
