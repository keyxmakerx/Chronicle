// Route registration for cut scenes.
package cutscenes

import (
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Per-user limits, far above honest use, so a loop cannot flood players'
// screens or the database.
const (
	showRateLimit    = 30
	watchedRateLimit = 120
	answerRateLimit  = 30
)

// RegisterRoutes mounts the routes under the campaign group. Every route needs
// a signed-in member of at least Player role and the Cut scenes add-on on.
// The Director routes then turn anyone who is not the owner or a co-DM away
// with a 404 (the co-DM grant cannot be expressed by a role middleware), and
// the service decides again; a showing is readable only by the people it was
// for. CSRF is enforced by the global middleware.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		campaigns.RequireRole(campaigns.RolePlayer),
		addons.RequireAddon(addonSvc, AddonSlug),
	)
	user := func(c *echo.Context) string { return auth.GetUserID(c) }
	shows := middleware.UserRateLimit(user, showRateLimit, time.Minute)
	watched := middleware.UserRateLimit(user, watchedRateLimit, time.Minute)
	foundryPreviews := middleware.UserRateLimit(user, showRateLimit, time.Minute)
	answers := middleware.UserRateLimit(user, answerRateLimit, time.Minute)

	// Director only (404 for anyone else). Static paths win over :sid, so
	// none of "new", "mine", "showings" or "preview" is read as a scene id.
	cg.GET("/cutscenes", h.List)
	cg.GET("/cutscenes/new", h.New)
	cg.POST("/cutscenes", h.Create)
	cg.POST("/cutscenes/preview", h.Preview)
	cg.POST("/cutscenes/preview/foundry", h.PreviewInFoundry, foundryPreviews)
	cg.GET("/cutscenes/showings", h.Showings)
	cg.POST("/cutscenes/showings/:shid/stop", h.Stop)
	cg.POST("/cutscenes/showings/:shid/answers/:aid", h.Resolve)
	cg.POST("/cutscenes/showings/:shid/moments/:m/go", h.Go)
	cg.GET("/cutscenes/:sid", h.Editor)
	cg.POST("/cutscenes/:sid", h.Save)
	cg.POST("/cutscenes/:sid/delete", h.Delete)
	cg.GET("/cutscenes/:sid/live", h.Live)
	cg.GET("/cutscenes/:sid/show", h.ShowSheet)
	cg.POST("/cutscenes/:sid/show", h.Show, shows)

	// Any member: their own showings, and one showing they were shown.
	cg.GET("/cutscenes/mine", h.Mine)
	cg.GET("/cutscenes/showings/:shid", h.ReadShowing)
	cg.POST("/cutscenes/showings/:shid/watched", h.Watched, watched)
	cg.POST("/cutscenes/showings/:shid/moments/:m/answer", h.Answer, answers)
}
