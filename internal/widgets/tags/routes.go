package tags

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all tag-related routes on the given Echo instance.
// Tag routes are scoped to a campaign and require campaign membership.
//
// Permissions:
//   - Player (read): list campaign tags, get entity tags
//   - Scribe (write): create/update/delete tags, set entity tags
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	// All tag routes require authentication and campaign membership.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// Read routes -- any campaign member can view tags.
	cg.GET("/tags", h.ListTags, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/entities/:eid/tags", h.GetEntityTags, campaigns.RequireRole(campaigns.RolePlayer))

	// Write routes -- Scribe or above can manage tags and tag assignments.
	cg.POST("/tags", h.CreateTag, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/tags/:tagId", h.UpdateTag, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/tags/:tagId", h.DeleteTag, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/entities/:eid/tags", h.SetEntityTags, campaigns.RequireRole(campaigns.RoleScribe))
}
