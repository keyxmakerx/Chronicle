// Package calendar - moon_marks_handler.go serves the Director's writes to a
// hand-set moon over JSON. The rules are in moon_marks_service.go; these
// only bind, call and answer with the moon as the Director now sees it.
package calendar

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// markService returns the moon mark half of the service, or an error when
// the service in use does not have one (a test fake).
func (h *Handler) markService() (MoonMarkService, error) {
	ms, ok := h.svc.(MoonMarkService)
	if !ok {
		return nil, apperror.NewInternalMessage("hand-set moons are not available", nil)
	}
	return ms, nil
}

func moonIDParam(c echo.Context) (int, error) {
	id, err := strconv.Atoi(c.Param("moonID"))
	if err != nil {
		return 0, apperror.NewBadRequest("invalid moon id")
	}
	return id, nil
}

// SetMoonModeAPI switches a moon between its cycle and being set by hand.
// PUT /campaigns/:id/calendars/:calid/moons/:moonID/mode
// {"mode":"manual","state_names":{"vanished":"Gone below"}}
func (h *Handler) SetMoonModeAPI(c echo.Context) error {
	ms, err := h.markService()
	if err != nil {
		return err
	}
	moonID, err := moonIDParam(c)
	if err != nil {
		return err
	}
	var req struct {
		Mode       string            `json:"mode"`
		StateNames map[string]string `json:"state_names"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	cc := campaigns.GetCampaignContext(c)
	out, err := ms.SetMoonMode(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, moonID, req.Mode, req.StateNames, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SetMoonMarkAPI sets a hand-set moon's state from a day on.
// PUT /campaigns/:id/calendars/:calid/moons/:moonID/marks
// {"year":1043,"month":3,"day":22,"state":"vanished","shown":true}
func (h *Handler) SetMoonMarkAPI(c echo.Context) error {
	ms, err := h.markService()
	if err != nil {
		return err
	}
	moonID, err := moonIDParam(c)
	if err != nil {
		return err
	}
	var req MoonMark
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	cc := campaigns.GetCampaignContext(c)
	out, err := ms.SetMoonMark(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, moonID, req, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// DeleteMoonMarkAPI removes the mark on a day.
// DELETE /campaigns/:id/calendars/:calid/moons/:moonID/marks?year=&month=&day=
func (h *Handler) DeleteMoonMarkAPI(c echo.Context) error {
	ms, err := h.markService()
	if err != nil {
		return err
	}
	moonID, err := moonIDParam(c)
	if err != nil {
		return err
	}
	var d DayDate
	for _, p := range []struct {
		key string
		dst *int
	}{{"year", &d.Year}, {"month", &d.Month}, {"day", &d.Day}} {
		n, err := strconv.Atoi(c.QueryParam(p.key))
		if err != nil {
			return apperror.NewBadRequest(p.key + " is required")
		}
		*p.dst = n
	}
	cc := campaigns.GetCampaignContext(c)
	out, err := ms.DeleteMoonMark(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, moonID, d, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
