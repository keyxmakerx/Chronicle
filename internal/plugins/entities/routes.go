package entities

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all entity-related routes on the given Echo instance.
// Entity routes are scoped to a campaign and require campaign membership.
// CRUD permissions: Player can view, Scribe can create/edit, Owner can delete.
// View routes use AllowPublicCampaignAccess so public campaigns are browseable
// without authentication.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	// Authenticated routes: create, edit, delete, API mutations.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	// Entry API (JSON endpoints for editor widget).
	cg.GET("/entities/:eid/entry", h.GetEntry, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/entities/:eid/entry", h.UpdateEntryAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Image API.
	cg.PUT("/entities/:eid/image", h.UpdateImageAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Scribe routes (create/edit).
	cg.GET("/entities/new", h.NewForm, campaigns.RequireRole(campaigns.RoleScribe))
	cg.POST("/entities", h.Create, campaigns.RequireRole(campaigns.RoleScribe))
	cg.GET("/entities/:eid/edit", h.EditForm, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/entities/:eid", h.Update, campaigns.RequireRole(campaigns.RoleScribe))

	// Owner routes.
	cg.DELETE("/entities/:eid", h.Delete, campaigns.RequireRole(campaigns.RoleOwner))

	// Entity type API (Owner only).
	cg.GET("/entity-types/:etid/layout", h.GetEntityTypeLayout, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/entity-types/:etid/layout", h.UpdateEntityTypeLayout, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/entity-types/:etid/color", h.UpdateEntityTypeColor, campaigns.RequireRole(campaigns.RoleOwner))

	// Public-capable view routes: use AllowPublicCampaignAccess so that
	// public campaigns can be browsed without logging in.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/entities", h.Index, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/entities/search", h.SearchAPI, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/entities/:eid", h.Show, campaigns.RequireRole(campaigns.RolePlayer))

	// Shortcut routes: /campaigns/:id/characters -> entities filtered by type.
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
		pub.GET(sc.path, func(c echo.Context) error {
			c.Set("entity_type_slug", slug)
			return h.Index(c)
		}, campaigns.RequireRole(campaigns.RolePlayer))
	}
}
