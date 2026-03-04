package relations

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all entity relation routes on the given Echo instance.
// Relation routes are scoped to a campaign and require campaign membership.
//
// Permissions:
//   - Player (read): list relations for an entity, get common relation types
//   - Scribe (write): create and delete relations
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	// Authenticated routes: mutations require full auth + campaign membership.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// Write routes -- Scribe or above can manage relations.
	cg.POST("/entities/:eid/relations", h.CreateRelation, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/entities/:eid/relations/:rid/metadata", h.UpdateRelationMetadata, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/entities/:eid/relations/:rid", h.DeleteRelation, campaigns.RequireRole(campaigns.RoleScribe))

	// Public-capable read routes: allow public campaign visitors to see relations.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/entities/:eid/relations", h.ListRelations, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/relation-types", h.GetCommonTypes, campaigns.RequireRole(campaigns.RolePlayer))
}
