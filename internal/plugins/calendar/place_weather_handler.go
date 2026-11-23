// Package calendar - place_weather_handler.go is the JSON API for each
// place's weather and the party's marks. Every route here is the Director's
// (CanAuthorDmOnly in routes.go): players read the party's weather through
// the ordinary day-weather route, which the service already narrows.
package calendar

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// placeWeather is the service's per-place half, or an error when this
// build's service has none.
func (h *Handler) placeWeather() (PlaceWeatherService, error) {
	pw, ok := h.svc.(PlaceWeatherService)
	if !ok {
		return nil, errNoPlaceWeather()
	}
	return pw, nil
}

// PlaceWeatherYearAPI returns the world climate, the places with their own,
// the party's marks and each place's readings for a year.
// GET /campaigns/:id/calendars/:calid/weather/places?year=
func (h *Handler) PlaceWeatherYearAPI(c *echo.Context) error {
	pw, err := h.placeWeather()
	if err != nil {
		return err
	}
	cc := campaigns.GetCampaignContext(c)
	year, err := strconv.Atoi(c.QueryParam("year"))
	if err != nil {
		return apperror.NewBadRequest("year is required")
	}
	out, err := pw.PlaceWeatherYear(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, year)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SetPlaceDayWeatherAPI paints or stores generated readings for one place.
// PUT /campaigns/:id/calendars/:calid/weather/places/:pid/days  {"days":[…]}
func (h *Handler) SetPlaceDayWeatherAPI(c *echo.Context) error {
	pw, err := h.placeWeather()
	if err != nil {
		return err
	}
	cc := campaigns.GetCampaignContext(c)
	var req struct {
		Days []DayWeatherInput `json:"days"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := pw.SetPlaceDayWeather(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, c.Param("pid"), req.Days); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ClearPlaceDayWeatherAPI removes one place's readings on a set of days.
// POST /campaigns/:id/calendars/:calid/weather/places/:pid/days/clear  {"days":[…]}
func (h *Handler) ClearPlaceDayWeatherAPI(c *echo.Context) error {
	pw, err := h.placeWeather()
	if err != nil {
		return err
	}
	cc := campaigns.GetCampaignContext(c)
	var req struct {
		Days []DayDate `json:"days"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := pw.ClearPlaceDayWeather(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, c.Param("pid"), req.Days); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// LockPlaceDayWeatherAPI locks or unlocks one place's readings. "locked"
// must be present, as for the world's days.
// POST /campaigns/:id/calendars/:calid/weather/places/:pid/days/lock  {"days":[…],"locked":true}
func (h *Handler) LockPlaceDayWeatherAPI(c *echo.Context) error {
	pw, err := h.placeWeather()
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
	n, err := pw.LockPlaceDayWeather(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, c.Param("pid"), req.Days, *req.Locked)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]int{"changed": n})
}

// partyMarkRequest is one day and, for a set, the page the party is at
// ("" or absent: nowhere with a climate of its own).
type partyMarkRequest struct {
	Year    int    `json:"year"`
	Month   int    `json:"month"`
	Day     int    `json:"day"`
	PlaceID string `json:"place_id"`
}

// SetPartyPlaceAPI marks where the party is from a day on.
// POST /campaigns/:id/calendars/:calid/weather/party  {"year","month","day","place_id"}
func (h *Handler) SetPartyPlaceAPI(c *echo.Context) error {
	pw, err := h.placeWeather()
	if err != nil {
		return err
	}
	cc := campaigns.GetCampaignContext(c)
	var req partyMarkRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	d := DayDate{Year: req.Year, Month: req.Month, Day: req.Day}
	if err := pw.SetPartyPlace(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, d, req.PlaceID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ClearPartyPlaceAPI takes back the mark on one day.
// POST /campaigns/:id/calendars/:calid/weather/party/clear  {"year","month","day"}
func (h *Handler) ClearPartyPlaceAPI(c *echo.Context) error {
	pw, err := h.placeWeather()
	if err != nil {
		return err
	}
	cc := campaigns.GetCampaignContext(c)
	var req partyMarkRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	d := DayDate{Year: req.Year, Month: req.Month, Day: req.Day}
	if err := pw.ClearPartyPlace(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, d); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
