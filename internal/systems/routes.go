package systems

import (
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up system reference page and API routes.
// System routes are scoped to a campaign and gated by per-module
// addon checks (each module ID is an addon slug). Custom module
// upload/delete routes are also registered here for campaign owners.
func RegisterRoutes(e *echo.Echo, h *SystemHandler, addonSvc addons.AddonService, authSvc auth.AuthService, campaignSvc campaigns.CampaignService) {
	// System routes: /campaigns/:id/systems/:mod/...
	// The :mod param is the module ID (e.g., "dnd5e").
	mg := e.Group("/campaigns/:id/systems/:mod",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		requireSystemAddon(addonSvc, h.campaignSystems),
	)

	mg.GET("", h.Index)
	mg.GET("/search", h.SearchAPI)
	// The category browser at a stable address even when the system has a book
	// (static segment, before /:cat).
	mg.GET("/browse", h.Browse)
	// The Rulebook book, filtered per viewer (static segment, before /:cat).
	mg.GET("/book", h.BookAPI)
	registerBookEditorRoutes(mg, h)
	// What the campaign wrote, rewrote and left out (static, before /:cat).
	mg.GET("/before-you-play", h.BeforeYouPlay)
	// An entry by kind and name, for sheets that store names (static, before /:cat).
	mg.GET("/named/:kind", h.NamedEntry)
	mg.GET("/named/:kind/peek", h.NamedEntryPeek)
	mg.GET("/told", h.ToldAPI)
	mg.GET("/:cat", h.CategoryList)
	mg.GET("/:cat/:item", h.ItemDetail)
	mg.GET("/:cat/:item/peek", h.EntryPeek)

	// What a page's sheet needs to draw: the system's scripts, its raw data
	// and glossary, and tooltips. Anyone who may read the campaign's pages
	// gets them (a look-around pass, a public campaign's visitor), or the
	// sheet stays blank for them; Director-only categories stay hidden by
	// hiddenFromViewer, which treats every visitor as a player.
	pub := e.Group("/campaigns/:id/systems/:mod",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
		requireSystemAddon(addonSvc, h.campaignSystems),
	)
	pub.GET("/widgets/:slug", h.WidgetScriptAPI, campaigns.RequireViewAccess())
	pub.GET("/rules-glossary", h.RulesGlossaryAPI, campaigns.RequireViewAccess())
	pub.GET("/data/:file", h.SystemDataAPI, campaigns.RequireViewAccess())
	pub.GET("/:cat/:item/tooltip", h.TooltipAPI, campaigns.RequireViewAccess())
}

// registerBookEditorRoutes mounts the Rulebook editor. Every handler checks the
// editor rule itself (owner or co-Director), the page included, so the group's
// campaign-access middleware is never the only gate. Static segments, so they
// sit ahead of the /:cat param routes without colliding.
func registerBookEditorRoutes(mg *echo.Group, h *SystemHandler) {
	mg.GET("/book/edit", h.BookEditPage)
	// Rules-index pages and search; any reader, sliced to their own book.
	mg.GET("/book/index/:cat", h.BookIndexAPI)
	mg.GET("/book/find", h.BookFindAPI)
	mg.GET("/book/entry/:cat/:entry", h.BookEntryAPI)
	mg.GET("/book/source", h.BookSourceAPI)
	// Only the cover and fonts book.yaml declares; book/ is never served whole.
	mg.GET("/book/files/:name", h.BookFileAPI)
	mg.GET("/book/export", h.BookExport)
	mg.POST("/book/chapters", h.BookChapterCreate)
	mg.PUT("/book/chapters/:chapter", h.BookChapterUpdate)
	mg.DELETE("/book/chapters/:chapter", h.BookChapterDelete)
	mg.POST("/book/chapters/:chapter/pages", h.BookPageAdd)
	mg.PUT("/book/chapters/:chapter/pages/:key", h.BookPageSave)
	mg.DELETE("/book/chapters/:chapter/pages/:key", h.BookPageDelete)
	mg.POST("/book/chapters/:chapter/pages/:key/keep", h.BookPageKeep)
}

// RegisterHouseRulesRoute mounts the campaign home's House rules block. It is
// campaign-scoped rather than system-scoped because the home does not know the
// campaign's game system; the handler reads it from the campaign's settings.
func RegisterHouseRulesRoute(e *echo.Echo, h *SystemHandler, authSvc auth.AuthService, campaignSvc campaigns.CampaignService) {
	g := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	g.GET("/house-rules", h.HouseRulesBlock, campaigns.RequireRole(campaigns.RolePlayer))
}

// RegisterCustomSystemRoutes sets up campaign owner routes for uploading
// and managing custom game systems.
func RegisterCustomSystemRoutes(e *echo.Echo, ch *CampaignSystemHandler, authSvc auth.AuthService, campaignSvc campaigns.CampaignService) {
	cg := e.Group("/campaigns/:id/systems",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	cg.GET("/status", ch.SystemStatus)
	cg.POST("/preview", ch.PreviewSystem)
	cg.POST("/upload", ch.UploadSystem)
	cg.GET("/custom", ch.GetCustomSystem)
	cg.DELETE("/custom", ch.DeleteSystem)
}

// requireSystemAddon returns middleware that checks whether the module
// (identified by the :mod path param) is enabled as an addon for the
// campaign, OR is the campaign's custom uploaded module. This allows
// both built-in and custom systems to work through the same routes.
func requireSystemAddon(addonSvc addons.AddonService, cmm *CampaignSystemManager) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			sysID := c.Param("mod")
			campaignID := c.Param("id")

			// Check if it's an enabled built-in module addon.
			enabled, err := addonSvc.IsEnabledForCampaign(c.Request().Context(), campaignID, sysID)
			if err != nil {
				// Fail open on DB errors — let the request through.
				return next(c)
			}
			if enabled {
				return next(c)
			}

			// Check if it's the campaign's custom system.
			if cmm != nil {
				if manifest := cmm.GetManifest(campaignID); manifest != nil && manifest.ID == sysID {
					return next(c)
				}
			}

			// System not enabled and not a custom system.
			if middleware.IsHTMX(c) {
				return apperror.NewNotFound(sysID + " system is not enabled for this campaign")
			}
			return c.Redirect(303, "/campaigns/"+campaignID)
		}
	}
}
