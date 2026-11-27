package dmscreen

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// playerViewerOf is the viewer of the players' screen. A DM-team member
// viewing the campaign as a player is read as a player, so their screen
// shows only what a player's would.
func playerViewerOf(c *echo.Context, cc *campaigns.CampaignContext) Viewer {
	role := cc.VisibilityRole()
	if campaigns.ViewingAsPlayer(c, cc) {
		role = int(campaigns.RolePlayer)
	}
	return Viewer{UserID: auth.GetUserID(c), Role: role}
}

// ShowPlayer handles GET /campaigns/:id/player-screen and returns the
// players' screen fragment, which the same dock widget mounts.
func (h *Handler) ShowPlayer(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	view, err := h.svc.BuildPlayer(c.Request().Context(), cc.Campaign.ID, playerViewerOf(c, cc))
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, PlayerPanel(view))
}

// ShowPlayerNote handles GET /campaigns/:id/player-screen/notes: the
// caller's own note, never anyone else's.
func (h *Handler) ShowPlayerNote(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	nv, err := h.svc.PlayerNote(c.Request().Context(), cc.Campaign.ID, playerViewerOf(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, nv)
}

// SavePlayerNote handles PUT /campaigns/:id/player-screen/notes, with the
// same body as the DM Screen's note.
func (h *Handler) SavePlayerNote(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var req saveNoteRequest
	if err := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 256<<10)).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	nv, err := h.svc.SavePlayerNote(c.Request().Context(), cc.Campaign.ID, playerViewerOf(c, cc), req.Text, req.Version)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, nv)
}
