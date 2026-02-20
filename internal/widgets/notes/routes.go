package notes

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all note-related routes on the given Echo instance.
// Note routes are scoped to a campaign and require campaign membership.
// All members can manage their own personal notes (Player or above).
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// All campaign members can CRUD their own notes.
	cg.GET("/notes", h.List, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/notes", h.Create, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/notes/:noteId", h.Update, campaigns.RequireRole(campaigns.RolePlayer))
	cg.DELETE("/notes/:noteId", h.Delete, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/notes/:noteId/toggle", h.ToggleCheck, campaigns.RequireRole(campaigns.RolePlayer))
}
