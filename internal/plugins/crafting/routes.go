// Route registration for crafting.
package crafting

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the routes under the campaign group, behind the
// crafting addon. Every member reaches every route; the service lets only a
// Director (the Owner or a co-Director, which a role middleware cannot
// express) see the editors' data or change them, shows a player only the
// recipe book, and lets a player start and roll projects only for a
// character they have claimed. CSRF is enforced by the global middleware.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, AddonSlug),
	)
	member := campaigns.RequireRole(campaigns.RolePlayer)

	cg.GET("/crafting", h.Page, member)
	cg.GET("/crafting/library", h.Library, member)
	cg.GET("/crafting/book", h.Book, member)
	cg.GET("/crafting/catalog", h.Catalog, member)
	cg.PUT("/crafting/settings", h.SaveSettings, member)

	cg.GET("/crafting/recipes", h.ListRecipes, member)
	cg.POST("/crafting/recipes", h.CreateRecipe, member)
	cg.GET("/crafting/recipes/:rid", h.GetRecipe, member)
	cg.PUT("/crafting/recipes/:rid", h.UpdateRecipe, member)
	cg.PUT("/crafting/recipes/:rid/rules", h.SaveRules, member)
	cg.POST("/crafting/recipes/:rid/approve", h.Approve, member)
	cg.DELETE("/crafting/recipes/:rid", h.DeleteRecipe, member)

	cg.PUT("/crafting/materials/:eid", h.SetTags, member)
	cg.PUT("/crafting/materials/:eid/picture", h.SetItemPicture, member)
	cg.PUT("/crafting/pictures", h.SetKindPictures, member)

	cg.GET("/crafting/benches", h.ListBenches, member)
	cg.POST("/crafting/benches", h.CreateBench, member)
	cg.GET("/crafting/benches/:bid", h.GetBench, member)
	cg.PUT("/crafting/benches/:bid", h.UpdateBench, member)
	cg.PUT("/crafting/benches/:bid/look/:skin", h.SaveArrangement, member)
	cg.DELETE("/crafting/benches/:bid", h.DeleteBench, member)

	cg.GET("/crafting/characters/:eid", h.Character, member)
	cg.POST("/crafting/characters/:eid/projects", h.StartProject, member)
	cg.GET("/crafting/projects", h.Projects, member)
	cg.POST("/crafting/projects/:pid/roll", h.RollProject, member)
	cg.POST("/crafting/projects/:pid/rolls", h.GiveRoll, member)
	cg.PUT("/crafting/projects/:pid/points", h.SetPoints, member)
	cg.DELETE("/crafting/projects/:pid", h.CancelProject, member)
	cg.POST("/crafting/respites", h.LogRespite, member)
}
