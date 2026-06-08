// routes.go — the binding UI routes (C-WIDGET-BINDING-P4a). Campaign-scoped;
// all writes are Scribe+. Generic across widget types (the handler validates
// host_type/widget_type against the registry). No addon gate — the affordance
// only appears on already-rendered, addon-gated widget blocks.
package widgetbindings

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the binding picker + bind/create/unbind endpoints under
// the campaign group. Picker is Scribe+ (the create-or-pick affordance is a
// Scribe action); mutations are Scribe+. CSRF is enforced by the global
// middleware (boot.js attaches the token to HTMX requests).
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	cg.GET("/bindings/picker", h.PickerAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.POST("/bindings", h.BindAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.POST("/bindings/create", h.CreateBindAPI, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/bindings", h.UnbindAPI, campaigns.RequireRole(campaigns.RoleScribe))
}
