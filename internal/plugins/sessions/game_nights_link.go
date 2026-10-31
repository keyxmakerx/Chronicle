package sessions

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RealWorldCalendarFinder names the calendar game nights are answered in:
// the campaign's real-world (wall-clock) calendar that this viewer may see,
// or "" when there is none. Implemented in internal/app over the calendar
// service, so this plugin never imports the calendar plugin.
type RealWorldCalendarFinder interface {
	RealWorldCalendarID(ctx context.Context, campaignID string, role int, userID string) (string, error)
}

// SetCalendarFinder wires the lookup GameNightsLink uses. Without it every
// game-night link falls back to the Calendar app.
func (h *Handler) SetCalendarFinder(f RealWorldCalendarFinder) {
	h.calendarFinder = f
}

// gameNightsTarget is where a member's game-night link lands. With a
// calendar it is that calendar's page, opened on the session's night (or,
// with no session named, on this month as usual). Game nights have no page
// of their own, so without one it is the Calendar app, or a named session's
// own page.
func gameNightsTarget(campaignID, calendarID, sessionID, date string) string {
	base := "/campaigns/" + url.PathEscape(campaignID)
	_, dateErr := time.Parse("2006-01-02", date)
	// A named session with no usable date has no day to open: its own page
	// is the honest landing, not some other night.
	if sessionID != "" && (calendarID == "" || dateErr != nil) {
		return base + "/sessions/" + url.PathEscape(sessionID)
	}
	if calendarID == "" {
		return base + "/apps/calendar"
	}
	view := base + "/calendars/" + url.PathEscape(calendarID) + "/view"
	if sessionID == "" {
		return view
	}
	q := url.Values{}
	q.Set("date", date)
	q.Set("night", sessionID)
	return view + "?" + q.Encode()
}

// GameNightsLink sends the sidebar's "Game nights" link, the RSVP card's
// links, the dashboard's game night list and the retired Game nights page's
// address to the calendar, where members see and answer them. Game nights
// are members' business, so anyone else goes to the campaign.
// GET /campaigns/:id/game-nights[?session=ID&date=YYYY-MM-DD]
// GET /campaigns/:id/sessions
func (h *Handler) GameNightsLink(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	sessionID := c.QueryParam("session")
	if cc.MemberRole < campaigns.RolePlayer {
		if sessionID != "" {
			return middleware.HTMXRedirect(c, keepTrashNotice(c, gameNightsTarget(cc.Campaign.ID, "", sessionID, "")))
		}
		return middleware.HTMXRedirect(c, keepTrashNotice(c, "/campaigns/"+url.PathEscape(cc.Campaign.ID)))
	}
	calID, found := h.realWorldCalendar(c)
	if calID == "" && found && sessionID == "" && cc.MemberRole >= campaigns.RoleOwner {
		// Game nights live in a real-world calendar; the owner is offered
		// one (a single press on the calendar wizard's review step) instead
		// of being sent to an empty calendar. The step opens over the
		// calendars page; the step's own URL is a dialog fragment.
		return middleware.HTMXRedirect(c, keepTrashNotice(c, "/campaigns/"+url.PathEscape(cc.Campaign.ID)+"/calendars?wizard=reallife"))
	}
	return middleware.HTMXRedirect(c, keepTrashNotice(c, gameNightsTarget(cc.Campaign.ID, calID, sessionID, c.QueryParam("date"))))
}

// keepTrashNotice carries ?trashed= and &kind= through the redirect, so a
// night moved to the Trash still gets page_safety.js's "Moved to Trash" and
// Undo on whichever campaign page it lands.
func keepTrashNotice(c echo.Context, target string) string {
	id := c.QueryParam("trashed")
	if id == "" {
		return target
	}
	q := url.Values{}
	q.Set("trashed", id)
	if kind := c.QueryParam("kind"); kind != "" {
		q.Set("kind", kind)
	}
	sep := "?"
	if strings.Contains(target, "?") {
		sep = "&"
	}
	return target + sep + q.Encode()
}

// realWorldCalendar is the calendar game nights are answered in for this
// viewer, or "" when there is none. found is false when the lookup didn't
// run or failed, so a caller can tell "no calendar" from "couldn't tell".
// Only members answer there, so a public viewer gets "".
func (h *Handler) realWorldCalendar(c echo.Context) (calID string, found bool) {
	cc := campaigns.GetCampaignContext(c)
	if h.calendarFinder == nil || cc.MemberRole < campaigns.RolePlayer {
		return "", false
	}
	id, err := h.calendarFinder.RealWorldCalendarID(c.Request().Context(), cc.Campaign.ID, int(cc.MemberRole), auth.GetUserID(c))
	if err != nil {
		slog.Warn("game nights link: finding the real-world calendar failed", slog.Any("error", err))
		return "", false
	}
	return id, true
}

// pollTarget is where a date poll opens on the calendar: the day named
// (its picked date, or the next date it suggests) with the poll picked out.
func pollTarget(campaignID, calendarID, pollID, date string) string {
	q := url.Values{}
	q.Set("poll", pollID)
	if _, err := time.Parse("2006-01-02", date); err == nil {
		q.Set("date", date)
	}
	return "/campaigns/" + url.PathEscape(campaignID) + "/calendars/" + url.PathEscape(calendarID) + "/view?" + q.Encode()
}

// sidebarNightDate is the date the dashboard block's link opens a session on: a
// repeating session's next night, or a one-off's own date.
func sidebarNightDate(s Session, today string) string {
	if s.IsRecurring {
		return nextUpcomingOccurrenceDate(&s, today)
	}
	if s.ScheduledDate != nil {
		return *s.ScheduledDate
	}
	return ""
}

// gameNightsToday is the date a night counts as past from: once it has
// ended in every zone (UTC-14 is the last), the same rule the nights API
// uses.
func gameNightsToday() string {
	return time.Now().UTC().Add(-14 * time.Hour).Format("2006-01-02")
}
