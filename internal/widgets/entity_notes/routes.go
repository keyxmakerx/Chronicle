package entity_notes

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// AddonSlug is the add-on that switches Player Notes on for a campaign.
const AddonSlug = "player-notes"

// RegisterRoutes mounts the entity_notes REST endpoints. All routes
// require campaign membership at minimum (RolePlayer); audience checks
// further restrict what each viewer can see/write inside the service.
//
// Reads (GET): RolePlayer — the audience filter handles per-row visibility.
// Writes (POST/PUT/DELETE): RolePlayer — the audience checks handle role
// gates per-write (e.g., players can author private/everyone/custom but
// not dm_only).
//
// Gated on the "player-notes" addon (ADR-056): a feature toggle, so off means
// off for every caller, with no first-party exception.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	g := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, AddonSlug),
		campaigns.RequireRole(campaigns.RolePlayer),
	)
	g.GET("/entities/:eid/notes", h.List)
	g.POST("/entities/:eid/notes", h.Create)
	g.GET("/entities/:eid/notes/:nid", h.Get)
	g.PUT("/entities/:eid/notes/:nid", h.Update)
	g.DELETE("/entities/:eid/notes/:nid", h.Delete)
}
