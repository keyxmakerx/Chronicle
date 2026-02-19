package entities

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all entity-related routes on the given Echo instance.
// Entity routes are scoped to a campaign and require campaign membership.
// CRUD permissions: Player can view, Scribe can create/edit, Owner can delete.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	// All entity routes are campaign-scoped.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// Player routes (read access).
	cg.GET("/entities", h.Index, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/entities/search", h.SearchAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/entities/:eid", h.Show, campaigns.RequireRole(campaigns.RolePlayer))

	// Scribe routes (create/edit access).
	cg.GET("/entities/new", h.NewForm, campaigns.RequireRole(campaigns.RoleScribe))
	cg.POST("/entities", h.Create, campaigns.RequireRole(campaigns.RoleScribe))
	cg.GET("/entities/:eid/edit", h.EditForm, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/entities/:eid", h.Update, campaigns.RequireRole(campaigns.RoleScribe))

	// Owner routes (delete access).
	cg.DELETE("/entities/:eid", h.Delete, campaigns.RequireRole(campaigns.RoleOwner))

	// Shortcut routes: /campaigns/:id/characters -> entities filtered by type.
	// Each sets a context value that the Index handler reads.
	shortcuts := []struct {
		path string
		slug string
	}{
		{"/characters", "character"},
		{"/locations", "location"},
		{"/organizations", "organization"},
		{"/items", "item"},
		{"/notes", "note"},
		{"/events", "event"},
	}

	for _, sc := range shortcuts {
		slug := sc.slug // Capture for closure.
		cg.GET(sc.path, func(c echo.Context) error {
			c.Set("entity_type_slug", slug)
			return h.Index(c)
		}, campaigns.RequireRole(campaigns.RolePlayer))
	}
}
