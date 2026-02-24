package notes

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all note-related routes on the given Echo instance.
// Note routes are scoped to a campaign and require campaign membership.
// All members can manage their own notes and interact with shared notes.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	player := campaigns.RequireRole(campaigns.RolePlayer)

	// CRUD — own notes + shared note access.
	cg.GET("/notes", h.List, player)
	cg.POST("/notes", h.Create, player)
	cg.PUT("/notes/:noteId", h.Update, player)
	cg.DELETE("/notes/:noteId", h.Delete, player)
	cg.POST("/notes/:noteId/toggle", h.ToggleCheck, player)

	// Edit locking — pessimistic lock for shared notes.
	cg.POST("/notes/:noteId/lock", h.Lock, player)
	cg.POST("/notes/:noteId/unlock", h.Unlock, player)
	cg.POST("/notes/:noteId/heartbeat", h.Heartbeat, player)
	cg.POST("/notes/:noteId/force-unlock", h.ForceUnlock, player) // owner check inside handler

	// Version history.
	cg.GET("/notes/:noteId/versions", h.ListVersions, player)
	cg.GET("/notes/:noteId/versions/:vid", h.GetVersion, player)
	cg.POST("/notes/:noteId/versions/:vid/restore", h.RestoreVersion, player)
}
