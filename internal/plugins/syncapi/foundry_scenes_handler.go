// foundry_scenes_handler.go — the GM's Foundry client reports the world's
// scenes (POST /sync/scenes). See foundry_scenes.go.

package syncapi

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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
