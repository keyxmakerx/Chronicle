package campaigns

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// RegisterRoutes sets up all campaign-related routes on the given Echo instance.
// Campaign list and creation require auth only. Campaign-scoped routes require
// campaign membership via RequireCampaignAccess middleware.
func RegisterRoutes(e *echo.Echo, h *Handler, svc CampaignService, authSvc auth.AuthService) {
	// Campaign list and creation require authentication only.
	authed := e.Group("", auth.RequireAuth(authSvc))
	authed.GET("/campaigns", h.Index)
	authed.GET("/campaigns/new", h.NewForm)
	authed.POST("/campaigns", h.Create)

	// Accept transfer requires auth but not campaign membership (uses token).
	authed.GET("/campaigns/:id/accept-transfer", h.AcceptTransfer)

	// Campaign-scoped routes require membership.
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		RequireCampaignAccess(svc),
	)

	// All members can view the campaign and member list.
	cg.GET("", h.Show, RequireRole(RolePlayer))
	cg.GET("/members", h.Members, RequireRole(RolePlayer))

	// Owner-only routes.
	cg.GET("/edit", h.EditForm, RequireRole(RoleOwner))
	cg.PUT("", h.Update, RequireRole(RoleOwner))
	cg.DELETE("", h.Delete, RequireRole(RoleOwner))
	cg.GET("/settings", h.Settings, RequireRole(RoleOwner))

	// Member management (Owner only).
	cg.POST("/members", h.AddMember, RequireRole(RoleOwner))
	cg.DELETE("/members/:uid", h.RemoveMember, RequireRole(RoleOwner))
	cg.PUT("/members/:uid/role", h.UpdateRole, RequireRole(RoleOwner))

	// Ownership transfer (Owner only).
	cg.GET("/transfer", h.TransferForm, RequireRole(RoleOwner))
	cg.POST("/transfer", h.Transfer, RequireRole(RoleOwner))
	cg.POST("/cancel-transfer", h.CancelTransfer, RequireRole(RoleOwner))
}
