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

// RegisterRoutes sets up Armory gallery routes on the Echo instance.
// Public-capable routes use AllowPublicCampaignAccess so public campaigns
// show items to unauthenticated visitors. All routes are gated behind the
// "armory" addon — campaign owners can enable/disable via the Plugin Hub.
func RegisterRoutes(e *echo.Echo, h *Handler, th *TransactionHandler, ih *InstanceHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	// Public-capable routes: gallery view (Player+).
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "armory"),
	)
	pub.GET("/armory", h.Index, campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/armory/count", h.CountAPI, campaigns.RequireRole(campaigns.RolePlayer))

	// Authenticated routes for instances and transactions.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "armory"),
	)

	// Instance management (Owner for create/update/delete, Player for list).
	cg.GET("/armory/instances", ih.ListInstances, campaigns.RequireRole(campaigns.RolePlayer))
	cg.GET("/armory/instances/manage", h.ManageInstances, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/armory/instances", ih.CreateInstance, campaigns.RequireRole(campaigns.RoleOwner))
	cg.PUT("/armory/instances/:iid", ih.UpdateInstance, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/armory/instances/:iid", ih.DeleteInstance, campaigns.RequireRole(campaigns.RoleOwner))
	cg.POST("/armory/instances/:iid/items", ih.AddItem, campaigns.RequireRole(campaigns.RoleScribe))
	cg.DELETE("/armory/instances/:iid/items/:eid", ih.RemoveItem, campaigns.RequireRole(campaigns.RoleScribe))

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
}
