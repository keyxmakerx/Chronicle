// Route registration for quests and notice boards.
package quests

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the routes under the campaign group. Reads follow the
// page: anyone who can view the page (a public-campaign guest included) may
// read, and the service hides what the viewer must not see. Writes need a
// signed-in member; the service then decides per board and per item. CSRF is
// enforced by the global middleware.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	// Static "picker" wins over the :eid route; its handler wants a member.
	pub.GET("/quests/picker", h.Picker, campaigns.RequireViewAccess())
	pub.GET("/quests/:eid", h.GetQuest, campaigns.RequireViewAccess())
	pub.GET("/notice-boards/:eid", h.GetBoards, campaigns.RequireViewAccess())
	pub.GET("/category-boards/:tid", h.GetBoards, campaigns.RequireViewAccess())

	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	dm := campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool {
		return cc.CanControlWorldState()
	}, "only the campaign owner or a member with DM access may do this")
	member := campaigns.RequireRole(campaigns.RolePlayer)

	cg.PUT("/quests/:eid", h.PutQuest, dm)

	cg.POST("/notice-boards/:eid/boards", h.CreateBoard, dm)
	cg.PATCH("/notice-boards/:eid/boards/:bid", h.PatchBoard, dm)
	cg.DELETE("/notice-boards/:eid/boards/:bid", h.DeleteBoard, dm)
	cg.DELETE("/notice-boards/:eid/boards/:bid/player-items", h.ClearPlayerItems, dm)
	cg.PUT("/notice-boards/:eid/order", h.SetOrder, dm)
	cg.PUT("/notice-boards/:eid/looks", h.SetLooks, dm)

	cg.POST("/notice-boards/:eid/boards/:bid/items", h.CreateItem, member)
	cg.PATCH("/notice-boards/:eid/boards/:bid/items/:iid", h.PatchItem, member)
	cg.DELETE("/notice-boards/:eid/boards/:bid/items/:iid", h.DeleteItem, member)

	// Category homes: same handlers, same gates.
	cg.POST("/category-boards/:tid/boards", h.CreateBoard, dm)
	cg.PATCH("/category-boards/:tid/boards/:bid", h.PatchBoard, dm)
	cg.DELETE("/category-boards/:tid/boards/:bid", h.DeleteBoard, dm)
	cg.DELETE("/category-boards/:tid/boards/:bid/player-items", h.ClearPlayerItems, dm)
	cg.PUT("/category-boards/:tid/order", h.SetOrder, dm)
	cg.PUT("/category-boards/:tid/looks", h.SetLooks, dm)

	cg.POST("/category-boards/:tid/boards/:bid/items", h.CreateItem, member)
	cg.PATCH("/category-boards/:tid/boards/:bid/items/:iid", h.PatchItem, member)
	cg.DELETE("/category-boards/:tid/boards/:bid/items/:iid", h.DeleteItem, member)
}
