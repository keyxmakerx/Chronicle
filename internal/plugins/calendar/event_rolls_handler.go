// Package calendar - event_rolls_handler.go serves event rolls over JSON:
// reading a roll and its nights, and the Director's writes. The rules are in
// event_rolls_service.go; these only bind, call and answer.
package calendar

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// rollService returns the roll half of the service, or an error when the
// service in use does not have one (a test fake).
func (h *Handler) rollService() (EventRollService, error) {
	rs, ok := h.svc.(EventRollService)
	if !ok {
		return nil, apperror.NewInternalMessage("event rolls are not available", nil)
	}
	return rs, nil
}

func queryYear(c echo.Context) (int, error) {
	y, err := strconv.Atoi(c.QueryParam("year"))
	if err != nil {
		return 0, apperror.NewBadRequest("year is required")
	}
	return y, nil
}

// GetEventRollAPI returns an event's roll and its nights in a year.
// GET /campaigns/:id/calendars/:calid/events/:eid/roll?year=
func (h *Handler) GetEventRollAPI(c echo.Context) error {
	rs, err := h.rollService()
	if err != nil {
		return err
	}
	cc := campaigns.GetCampaignContext(c)
	year, err := queryYear(c)
	if err != nil {
		return err
	}
	out, err := rs.GetEventRoll(c.Request().Context(), c.Param("eid"), c.Param("calid"), cc.Campaign.ID, year, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// ListMonthRollsAPI returns the nights of every rolling event in a month,
// by event id, for the calendar's day cells.
// GET /campaigns/:id/calendars/:calid/rolls?year=&month=
func (h *Handler) ListMonthRollsAPI(c echo.Context) error {
	rs, err := h.rollService()
	if err != nil {
		return err
	}
	cc := campaigns.GetCampaignContext(c)
	year, err := queryYear(c)
	if err != nil {
		return err
	}
	month, err := strconv.Atoi(c.QueryParam("month"))
	if err != nil || month < 1 {
		return apperror.NewBadRequest("month is required")
	}
	out, err := rs.ListMonthRolls(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, year, month, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SetEventRollAPI creates, replaces or (with "config": null) removes an
// event's roll, and answers with the roll for year.
// PUT /campaigns/:id/calendars/:calid/events/:eid/roll  {"config":{...}|null,"year":N}
func (h *Handler) SetEventRollAPI(c echo.Context) error {
	rs, err := h.rollService()
	if err != nil {
		return err
	}
	cc := campaigns.GetCampaignContext(c)
	var req struct {
		Config json.RawMessage `json:"config"`
		Year   int             `json:"year"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	var cfg *RollConfig
	if len(req.Config) > 0 && string(req.Config) != "null" {
		cfg = &RollConfig{}
		if err := json.Unmarshal(req.Config, cfg); err != nil {
			return apperror.NewBadRequest("invalid roll")
		}
	}
	out, err := rs.SetEventRoll(c.Request().Context(), c.Param("eid"), c.Param("calid"), cc.Campaign.ID, cfg, req.Year, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// RollEventNightsAPI rolls nights ahead: a whole year, or the given dates.
// POST /campaigns/:id/calendars/:calid/events/:eid/roll/nights  {"year":N} | {"dates":[...]}
func (h *Handler) RollEventNightsAPI(c echo.Context) error {
	rs, err := h.rollService()
	if err != nil {
		return err
	}
	cc := campaigns.GetCampaignContext(c)
	var in RollNightsInput
	if err := c.Bind(&in); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	out, err := rs.RollEventNights(c.Request().Context(), c.Param("eid"), c.Param("calid"), cc.Campaign.ID, in, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// LockEventRollNightsAPI locks or unlocks stored nights.
// POST /campaigns/:id/calendars/:calid/events/:eid/roll/nights/lock  {"days":[...],"locked":bool}
func (h *Handler) LockEventRollNightsAPI(c echo.Context) error {
	rs, err := h.rollService()
	if err != nil {
		return err
	}
	cc := campaigns.GetCampaignContext(c)
	var req struct {
		Days   []DayDate `json:"days"`
		Locked *bool     `json:"locked"`
	}
	if err := c.Bind(&req); err != nil || req.Locked == nil {
		return apperror.NewBadRequest("days and locked are required")
	}
	n, err := rs.LockEventRollNights(c.Request().Context(), c.Param("eid"), c.Param("calid"), cc.Campaign.ID, req.Days, *req.Locked, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]int{"changed": n})
}

// TellEventRollNightAPI says whether players are told one night; "told":
// null goes back to what the night's result says.
// POST /campaigns/:id/calendars/:calid/events/:eid/roll/nights/tell  {"year","month","day","told"}
func (h *Handler) TellEventRollNightAPI(c echo.Context) error {
	rs, err := h.rollService()
	if err != nil {
		return err
	}
	cc := campaigns.GetCampaignContext(c)
	var req struct {
		DayDate
		Told *bool `json:"told"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := rs.TellEventRollNight(c.Request().Context(), c.Param("eid"), c.Param("calid"), cc.Campaign.ID, req.DayDate, req.Told, viewerFrom(c, cc)); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ClearEventRollNightsAPI deletes a year's stored nights.
// POST /campaigns/:id/calendars/:calid/events/:eid/roll/nights/clear  {"year":N,"keep_locked":bool}
func (h *Handler) ClearEventRollNightsAPI(c echo.Context) error {
	rs, err := h.rollService()
	if err != nil {
		return err
	}
	cc := campaigns.GetCampaignContext(c)
	var req struct {
		Year       int  `json:"year"`
		KeepLocked bool `json:"keep_locked"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	n, err := rs.ClearEventRollNights(c.Request().Context(), c.Param("eid"), c.Param("calid"), cc.Campaign.ID, req.Year, req.KeepLocked, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]int{"cleared": n})
}
