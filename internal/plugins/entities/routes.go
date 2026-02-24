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

	// Fields API (JSON endpoints for attributes widget).
	cg.GET("/entities/:eid/fields", h.GetFieldsAPI, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/entities/:eid/fields", h.UpdateFieldsAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/entities/:eid/field-overrides", h.UpdateFieldOverridesAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Image API.
	cg.PUT("/entities/:eid/image", h.UpdateImageAPI, campaigns.RequireRole(campaigns.RoleScribe))

	// Scribe routes (create/edit).
	cg.GET("/entities/new", h.NewForm, campaigns.RequireRole(campaigns.RoleScribe))
	cg.POST("/entities", h.Create, campaigns.RequireRole(campaigns.RoleScribe))
	cg.GET("/entities/:eid/edit", h.EditForm, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/entities/:eid", h.Update, campaigns.RequireRole(campaigns.RoleScribe))

	// Owner routes.
	cg.DELETE("/entities/:eid", h.Delete, campaigns.RequireRole(campaigns.RoleOwner))

	// Entity type management (Owner only).
	cg.GET("/entity-types", h.EntityTypesPage, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/entity-types", h.CreateEntityType, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/entity-types/:etid", h.UpdateEntityTypeAPI, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/entity-types/:etid", h.DeleteEntityType, campaigns.RequireRole(campaigns.RoleOwner))

	// Template editor (Owner only) — standalone, kept for backward compat.
	cg.GET("/entity-types/:etid/template", h.TemplateEditor, campaigns.RequireRole(campaigns.RoleOwner))

	// Unified entity type configuration (Owner only).
	cg.GET("/entity-types/:etid/config", h.EntityTypeConfig, campaigns.RequireRole(campaigns.RoleOwner))
	// Customize Hub fragment: identity + attributes + category dashboard for one entity type.
	cg.GET("/entity-types/:etid/customize", h.EntityTypeCustomizeFragment, campaigns.RequireRole(campaigns.RoleOwner))

	// Entity type layout/color/dashboard API (Owner only).
	cg.GET("/entity-types/:etid/layout", h.GetEntityTypeLayout, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/entity-types/:etid/layout", h.UpdateEntityTypeLayout, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/entity-types/:etid/color", h.UpdateEntityTypeColor, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/entity-types/:etid/dashboard", h.UpdateEntityTypeDashboard, campaigns.RequireRole(campaigns.RoleOwner))

	// Category dashboard layout API (Owner only).
	cg.GET("/entity-types/:etid/dashboard-layout", h.GetCategoryDashboardLayout, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/entity-types/:etid/dashboard-layout", h.UpdateCategoryDashboardLayout, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/entity-types/:etid/dashboard-layout", h.ResetCategoryDashboardLayout, campaigns.RequireRole(campaigns.RoleOwner))

	// Public-capable view routes: use AllowPublicCampaignAccess so that
	// public campaigns can be browsed without logging in.
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/entities", h.Index, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/entities/search", h.SearchAPI, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/entities/:eid", h.Show, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/entities/:eid/preview", h.PreviewAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// Dynamic category route: resolves any entity type slug to a category
	// dashboard. Echo's router gives static segments (entities, settings, etc.)
	// priority over this parameter route, so it only catches actual type slugs.
	pub.GET("/:typeSlug", func(c echo.Context) error {
		c.Set("entity_type_slug", c.Param("typeSlug"))
		return h.Index(c)
	}, campaigns.RequireRole(campaigns.RolePlayer))
}
