// routes.go registers Armory gallery endpoints on the Echo router.
// The gallery page is readable by Players; all routes are gated behind
// the "armory" addon.
package armory

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// AddonSlug is the addon that gates every Armory feature; other plugins
// check it through this constant rather than repeating the name.
const AddonSlug = "armory"

// RegisterRoutes sets up Armory gallery routes on the Echo instance.
// Public-capable routes use AllowPublicCampaignAccess so public campaigns
// show items to unauthenticated visitors. All routes are gated behind the
// "armory" addon — campaign owners can enable/disable via the Plugin Hub.
func RegisterRoutes(e *echo.Echo, h *Handler, th *TransactionHandler, ih *InstanceHandler, sh *StashHandler, rh *ShopRoomHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	// Public-capable routes: gallery view (Player+).
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, AddonSlug),
	)
	pub.GET("/armory", h.Index, campaigns.RequireViewAccess())
	pub.GET("/armory/count", h.CountAPI, campaigns.RequireViewAccess())

	// A shop's room layout is readable wherever the shop is; the service hides
	// it from viewers who cannot see the shop entity.
	pub.GET("/armory/shops/:eid/room", rh.Get, campaigns.RequireViewAccess())

	// Authenticated routes for instances and transactions.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, AddonSlug),
	)

	// Instance management: Scribe+ (owner included) creates, renames, deletes
	// collections and edits their items; Player may only list.
	cg.GET("/armory/instances", ih.ListInstances, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/armory/instances/manage", h.ManageInstances, campaigns.RequireRole(campaigns.RoleScribe))
	cg.POST("/armory/instances", ih.CreateInstance, campaigns.RequireRole(campaigns.RoleScribe))
	cg.PUT("/armory/instances/:iid", ih.UpdateInstance, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/armory/instances/:iid", ih.DeleteInstance, campaigns.RequireRole(campaigns.RoleScribe))
	cg.GET("/armory/items/:eid/collections", ih.ItemCollections, campaigns.RequireRole(campaigns.RoleScribe))
	cg.POST("/armory/instances/:iid/items", ih.AddItem, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/armory/instances/:iid/items/:eid", ih.RemoveItem, campaigns.RequireRole(campaigns.RoleScribe))

	// Only the campaign Owner arranges a shop room.
	cg.PUT("/armory/shops/:eid/room", rh.Put, campaigns.RequireRole(campaigns.RoleOwner))

	// Transaction routes.
	// Purchase is the player-initiated buy path: a Player buys an item from
	// a shop with their own character. CreateTransaction below is the
	// admin-mediated path (gift/transfer/restock) and stays Scribe-gated.
	// No prior ADR or comment justified the previous Scribe gate on
	// Purchase — buyer ownership is enforced server-side in transaction_service.
	cg.POST("/armory/purchase", th.Purchase, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/armory/transactions", th.CreateTransaction, campaigns.RequireRole(campaigns.RoleScribe))
	cg.GET("/armory/transactions", th.ListTransactions, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/armory/shops/:eid/transactions", th.ListShopTransactions, campaigns.RequireRole(campaigns.RolePlayer))

	// Stashes and moves. Player+ reaches every route; what a caller may see or
	// change is decided in the service (stash visibility, acting as a
	// character, and Owner-only answers), never by the route role alone, so a
	// DM-granted co-DM is not shut out by a raw-role gate.
	cg.GET("/armory/stashes", sh.Page, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/armory/stashes", sh.CreateStash, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/armory/stashes/:sid", sh.UpdateStash, campaigns.RequireRole(campaigns.RolePlayer))
	cg.DELETE("/armory/stashes/:sid", sh.DeleteStash, campaigns.RequireRole(campaigns.RolePlayer))
	cg.PUT("/armory/stashes/:sid/viewers", sh.SetViewers, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/armory/stashes/:sid/items", sh.AddItem, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/armory/stashes/:sid/history", sh.StashHistory, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/armory/move", sh.MoveDialog, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/armory/moves", sh.Move, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/armory/moves/:mid/approve", sh.Approve, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/armory/moves/:mid/decline", sh.Decline, campaigns.RequireRole(campaigns.RolePlayer))
	cg.POST("/armory/downtime", sh.SetDowntime, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/armory/characters/:eid/panel", sh.CharacterPanel, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/armory/characters/:eid/history", sh.CharacterHistory, campaigns.RequireRole(campaigns.RolePlayer))
}
