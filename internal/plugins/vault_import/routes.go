package vault_import

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the import under /campaigns/:id behind sign-in,
// campaign access and the owner role. The group is built here so the
// sign-in requirement for the upload route lives next to the route.
func RegisterRoutes(e *echo.Echo, h *Handler, authSvc auth.AuthService, campaignSvc campaigns.CampaignService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	RegisterOwnerRoutes(cg, h, campaigns.RequireRole(campaigns.RoleOwner))
}

// RegisterOwnerRoutes mounts Manage > Import on a /campaigns/:id group that
// already enforces sign-in and campaign access. Every route is owner-only:
// the import creates pages and uploads files under the owner's name. The two
// that take input or start work are also rate limited.
func RegisterOwnerRoutes(cg *echo.Group, h *Handler, requireOwner echo.MiddlewareFunc) {
	cg.GET("/import", h.Page, requireOwner)
	cg.GET("/import/markdown/dropzone", h.DropZone, requireOwner)
	cg.POST("/import/markdown/preview", h.Preview, requireOwner, middleware.RateLimit(10, time.Minute))
	cg.POST("/import/markdown/start", h.Start, requireOwner, middleware.RateLimit(10, time.Minute))
	cg.GET("/import/markdown/jobs/:job", h.Job, requireOwner)
}
