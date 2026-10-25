// foundry_scenes_handler.go — the GM's Foundry client reports the world's
// scenes (POST /sync/scenes). See foundry_scenes.go.

package syncapi

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SetFoundryScenes wires the scenes store. Without it the report endpoint
// answers 404, which the module reads as "this Chronicle doesn't take
// scenes" and stops reporting for the session.
func (h *SyncHistoryHandler) SetFoundryScenes(repo FoundrySceneRepository) {
	h.scenes = repo
}

// ReportScenes replaces the campaign's Foundry scene list with the GM
// client's report. Only an owner or DM-access key may send it: scene names
// can give away what is coming.
// POST /api/v1/campaigns/:id/sync/scenes
func (h *SyncHistoryHandler) ReportScenes(c echo.Context) error {
	if h.scenes == nil {
		return apperror.NewNotFound("scene reports are not available")
	}
	if !h.isDMEquivalent(c) {
		return apperror.NewForbidden("reporting Foundry scenes requires owner or DM access")
	}
	var req reportScenesRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	report, err := cleanScenes(req, h.now())
	if err != nil {
		return err
	}
	if err := h.scenes.Replace(c.Request().Context(), c.Param("id"), report); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]int{"stored": len(report.Scenes)})
}

// sceneJSON is one scene as the page editor and reading view get it.
type sceneJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Folder string `json:"folder"`
}

// ScenesJSON is the campaign's Foundry scenes, in Foundry's sidebar order,
// for the page editor's @ list and the "map present / needed" marks. Only
// those who may see DM-only material get it (403 otherwise), since scene
// names can give away what is coming; a player's page shows a scene mention
// as its plain name. `reported` is false when Foundry has never sent a list.
// GET /campaigns/:id/foundry/scenes
func (h *SyncHistoryHandler) ScenesJSON(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if !cc.CanAuthorDmOnly() {
		return apperror.NewForbidden("only the owner and those with DM access see Foundry scenes")
	}
	out := map[string]any{"reported": false, "truncated": false, "scenes": []sceneJSON{}}
	if h.scenes == nil {
		return c.JSON(http.StatusOK, out)
	}
	rep, err := h.scenes.Get(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	if rep != nil {
		list := make([]sceneJSON, 0, len(rep.Scenes))
		for _, s := range rep.Scenes {
			list = append(list, sceneJSON{ID: s.SceneID, Name: s.Name, Folder: s.Folder})
		}
		out = map[string]any{"reported": true, "truncated": rep.Truncated, "reportedAt": rep.ReportedAt.UTC(), "scenes": list}
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, out)
}
