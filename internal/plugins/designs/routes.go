package designs

import (
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes mounts the library. Every route is Owner-only: players never
// reach saving or sharing, and the service checks the same rule again. The
// link routes sit outside a campaign because a link is opened before the
// reader has chosen which of their campaigns takes the design; the service
// requires them to own a campaign, and the one they copy into.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	owner := campaigns.RequireRole(campaigns.RoleOwner)

	cg.GET("/designs", h.List, owner)
	cg.POST("/designs", h.Save, owner)
	// Fixed paths first so they never read as a design id.
	cg.GET("/designs/owners", h.SearchOwners, owner)
	cg.GET("/designs/shared-with-me", h.SharedWithMe, owner)
	cg.POST("/designs/code/preview", h.PreviewCode, owner)
	cg.POST("/designs/code/take", h.TakeCode, owner)
	cg.GET("/designs/shared/:did/preview", h.PreviewShared, owner)
	cg.POST("/designs/shared/:did/take", h.TakeShared, owner)

	cg.GET("/designs/:did", h.Get, owner)
	cg.PATCH("/designs/:did", h.Update, owner)
	cg.DELETE("/designs/:did", h.Delete, owner)
	cg.GET("/designs/:did/share", h.ShareState, owner)
	cg.PUT("/designs/:did/share/link", h.SetLink, owner)
	cg.POST("/designs/:did/share/owners", h.AddOwner, owner)
	cg.DELETE("/designs/:did/share/owners/:uid", h.RemoveOwner, owner)
	cg.DELETE("/designs/:did/share", h.StopSharing, owner)

	lg := e.Group("/designs/link", auth.RequireAuth(authSvc))
	lg.GET("/:token", h.Link)
	lg.POST("/:token/take", h.TakeLink)
}
