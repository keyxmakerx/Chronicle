package sessions

import (
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// allNightsGroup is one heading of the calendar's "All game nights" list.
type allNightsGroup struct {
	Key    string          `json:"key"`
	Label  string          `json:"label"`
	Nights []allNightsItem `json:"nights"`
}

// allNightsItem is one night, or one repeating series, on that list. Day is
// its scheduled date; Date is the day the calendar opens it on. A night the
// calendar does not show (completed or cancelled) has no Date and links to
// its own page instead.
type allNightsItem struct {
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	Day       string `json:"day,omitempty"`
	Date      string `json:"date,omitempty"`
	When      string `json:"when"`
	Repeat    string `json:"repeat,omitempty"`
	Href      string `json:"href,omitempty"`
}

// AllGameNightsAPI is every game night in the campaign, grouped as
// Upcoming, Played, Completed and Cancelled, for the calendar's Game nights
// panel. Members only, like the rest of the game-night answers.
// GET /campaigns/:id/sessions/all
func (h *Handler) AllGameNightsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	list, err := h.svc.ListSessions(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	userID := auth.GetUserID(c)
	// Past nights are judged by the viewer's own date, so tonight's game
	// stays upcoming until their midnight.
	today := time.Now().In(timeutil.LoadLocation(h.resolveViewerTZ(c, userID))).Format("2006-01-02")
	viewerTZ := h.storedTZ(c.Request().Context(), userID)
	return c.JSON(http.StatusOK, allNightsGroups(cc.Campaign.ID, groupSessions(list, today), today, viewerTZ))
}

// allNightsGroups shapes the grouped sessions for the calendar. Only planned
// nights are on the calendar's grid, so only those carry a date to open.
func allNightsGroups(campaignID string, groups []SessionGroup, today, viewerTZ string) []allNightsGroup {
	out := make([]allNightsGroup, 0, len(groups))
	for _, g := range groups {
		ag := allNightsGroup{Key: g.Key, Label: g.Label, Nights: make([]allNightsItem, 0, len(g.Sessions))}
		for i := range g.Sessions {
			s := &g.Sessions[i]
			it := allNightsItem{SessionID: s.ID, Name: s.Name}
			if s.ScheduledDate != nil {
				it.Day = *s.ScheduledDate
				it.When = s.FormatScheduledFor(viewerTZ)
			}
			if s.IsRecurring {
				it.Repeat = s.RecurrenceLabel()
			}
			if s.Status == StatusPlanned {
				it.Date = sidebarNightDate(*s, today)
			}
			if it.Date == "" {
				it.Href = "/campaigns/" + url.PathEscape(campaignID) + "/sessions/" + url.PathEscape(s.ID)
			}
			ag.Nights = append(ag.Nights, it)
		}
		out = append(out, ag)
	}
	return out
}
