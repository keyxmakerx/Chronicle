package sessions

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SetSightingService wires per-night sightings. Without it the sightings
// routes answer 404.
func (h *Handler) SetSightingService(s SightingService) {
	h.sightings = s
}

func sightingViewer(c echo.Context) permissions.Viewer {
	cc := campaigns.GetCampaignContext(c)
	return permissions.RequestViewer(cc.VisibilityRole(), auth.GetUserID(c))
}

// SightingsAPI returns when the party saw one page:
// GET /campaigns/:id/sessions/sightings?entity=ID.
func (h *Handler) SightingsAPI(c echo.Context) error {
	if h.sightings == nil {
		return apperror.NewNotFound("sightings are not available")
	}
	entityID := c.QueryParam("entity")
	if entityID == "" {
		return apperror.NewBadRequest("entity is required")
	}
	cc := campaigns.GetCampaignContext(c)
	out, err := h.sightings.SightingsFor(c.Request().Context(), cc.Campaign.ID, entityID, sightingViewer(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// RecordSightingAPI answers one night for one page:
// PUT /campaigns/:id/sessions/:sid/sightings {entityId, date, state, source}.
func (h *Handler) RecordSightingAPI(c echo.Context) error {
	if h.sightings == nil {
		return apperror.NewNotFound("sightings are not available")
	}
	var in RecordSightingInput
	if err := c.Bind(&in); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	in.SessionID = c.Param("sid")
	cc := campaigns.GetCampaignContext(c)
	if err := h.sightings.RecordSighting(c.Request().Context(), cc.Campaign.ID, in, sightingViewer(c)); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ClearSightingAPI removes one night's answer for one page:
// DELETE /campaigns/:id/sessions/:sid/sightings?entity=ID&date=YYYY-MM-DD.
func (h *Handler) ClearSightingAPI(c echo.Context) error {
	if h.sightings == nil {
		return apperror.NewNotFound("sightings are not available")
	}
	cc := campaigns.GetCampaignContext(c)
	err := h.sightings.ClearSighting(c.Request().Context(), cc.Campaign.ID, c.Param("sid"), c.QueryParam("date"), c.QueryParam("entity"), sightingViewer(c))
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// SightingNightsAPI returns the played game nights, for a page's Last seen to
// draw every night and let the DM answer one:
// GET /campaigns/:id/sessions/sightings/nights.
func (h *Handler) SightingNightsAPI(c echo.Context) error {
	if h.sightings == nil {
		return apperror.NewNotFound("sightings are not available")
	}
	cc := campaigns.GetCampaignContext(c)
	out, err := h.sightings.NightsFor(c.Request().Context(), cc.Campaign.ID, gameNightsToday())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
