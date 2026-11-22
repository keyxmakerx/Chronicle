// Route registration for decks.
package decks

import (
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// moveRateLimit is how many moves one user may make a minute.
const moveRateLimit = 120

// RegisterRoutes mounts the routes under the campaign group. Every route needs
// a signed-in member of at least Player role and the Card decks add-on on; the
// service then decides who may see and do what (the co-Director grant and the
// per-deck player rules cannot be expressed by a role middleware), so the gate
// here is not the only one. CSRF is enforced by the global middleware.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		campaigns.RequireRole(campaigns.RolePlayer),
		addons.RequireAddon(addonSvc, AddonSlug),
	)
	cg.GET("/decks", h.List)
	cg.POST("/decks", h.Create)
	// Registered before /decks/:did so "table" is never read as a deck id.
	cg.GET("/decks/table", h.Table)
	cg.GET("/decks/:did", h.Get)
	cg.PUT("/decks/:did", h.Update)
	cg.DELETE("/decks/:did", h.Delete)
	cg.PUT("/decks/:did/players", h.SetPlayers)
	cg.POST("/decks/:did/cards", h.CreateCard)
	cg.PUT("/decks/:did/cards/:cid", h.UpdateCard)
	cg.DELETE("/decks/:did/cards/:cid", h.DeleteCard)
	// The moves share one per-user limit, far above honest play, so flipping
	// or drawing in a loop cannot flood the log or the database.
	moves := middleware.UserRateLimit(func(c *echo.Context) string { return auth.GetUserID(c) }, moveRateLimit, time.Minute)
	cg.POST("/decks/:did/draw", h.Draw, moves)
	cg.POST("/decks/:did/copies/:copy/:verb", h.Act, moves)
	cg.POST("/decks/:did/deal-card", h.DealCard, moves)
	cg.POST("/decks/:did/shuffle", h.Shuffle, moves)
	cg.POST("/decks/:did/gather", h.Gather, moves)
}
