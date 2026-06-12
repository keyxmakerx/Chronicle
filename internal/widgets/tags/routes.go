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
//   - Owner (grants): manage a tag's visibility grants (a power — see below)
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	// Authenticated routes: mutations require full auth + campaign membership.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// Write routes -- Scribe or above can manage tags and tag assignments.
	cg.POST("/tags", h.CreateTag, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/tags/:tagId", h.UpdateTag, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/tags/:tagId", h.DeleteTag, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/entities/:eid/tags", h.SetEntityTags, campaigns.RequireRole(campaigns.RoleScribe))

	// Tag visibility grants (C-PERM-W1-TAG-GRANTS) -- OWNER ONLY. Granting a
	// tag can reveal dm_only / custom-restricted content to its subjects, so it
	// sits a rung above ordinary tag management (Scribe).
	cg.GET("/tags/:tagId/grants", h.ListGrants, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/tags/:tagId/grants", h.CreateGrant, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/tags/:tagId/grants/:grantId", h.DeleteGrant, campaigns.RequireRole(campaigns.RoleOwner))

	// Public-capable read routes: allow public campaign visitors to see tags.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/tags", h.ListTags, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/entities/:eid/tags", h.GetEntityTags, campaigns.RequireRole(campaigns.RolePlayer))
}
