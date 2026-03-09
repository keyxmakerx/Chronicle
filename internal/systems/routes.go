package systems

import (
	"github.com/labstack/echo/v4"

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
	mg.GET("/:cat", h.CategoryList)
	mg.GET("/:cat/:item", h.ItemDetail)
	mg.GET("/:cat/:item/tooltip", h.TooltipAPI)
}

// RegisterCustomSystemRoutes sets up campaign owner routes for uploading
// and managing custom game systems.
func RegisterCustomSystemRoutes(e *echo.Echo, ch *CampaignSystemHandler, authSvc auth.AuthService, campaignSvc campaigns.CampaignService) {
	cg := e.Group("/campaigns/:id/systems",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

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
		return func(c echo.Context) error {
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
