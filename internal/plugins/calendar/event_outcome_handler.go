// Package calendar - event_outcome_handler.go serves what became of events:
// the list the calendar asks about once today moves past them, and the
// Calendar settings switch that turns the question off. An event's status
// and line themselves travel on PUT .../events/:eid and the batch route.
package calendar

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// parseDayParam reads a "Y-M-D" query value. A year may be negative, so the
// first dash after a leading minus is the separator.
func parseDayParam(raw string) (DayDate, error) {
	bad := apperror.NewBadRequest("from and to are dates written year-month-day")
	neg := strings.HasPrefix(raw, "-")
	parts := strings.Split(strings.TrimPrefix(raw, "-"), "-")
	if len(parts) != 3 {
		return DayDate{}, bad
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return DayDate{}, bad
		}
		n[i] = v
	}
	if neg {
		n[0] = -n[0]
	}
	return DayDate{Year: n[0], Month: n[1], Day: n[2]}, nil
}

// ListPassedEventsAPI lists the events whose day lies in [from, to), for
// the sheet that asks what became of them. Owner only, like setting today.
// GET /campaigns/:id/calendars/:calid/events/passed?from=Y-M-D&to=Y-M-D
func (h *Handler) ListPassedEventsAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	from, err := parseDayParam(c.QueryParam("from"))
	if err != nil {
		return err
	}
	to, err := parseDayParam(c.QueryParam("to"))
	if err != nil {
		return err
	}
	out, err := h.svc.ListPassedEvents(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, from, to, ownViewer(c, cc))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SetAskWhenDayPassesAPI saves Calendar settings' "Ask what became of
// events" switch the moment it changes and answers with the part redrawn,
// saying Saved. The checkbox sends nothing when off, so ask_sent marks a
// submission that carried the control. Owner only.
// PUT /campaigns/:id/calendars/:calid/day-passes
func (h *Handler) SetAskWhenDayPassesAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if c.FormValue("ask_sent") == "" {
		return apperror.NewBadRequest("the switch was not sent")
	}
	ask := c.FormValue("ask") == "on"
	calID := c.Param("calid")
	if err := h.svc.SetAskWhenDayPasses(c.Request().Context(), calID, cc.Campaign.ID, ask); err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, dayPassesSwitch(cc.Campaign.ID, calID, ask, true))
}
