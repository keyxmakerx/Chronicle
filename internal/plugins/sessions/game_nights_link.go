package sessions

import (
	"context"
	"log/slog"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// RealWorldCalendarFinder names the calendar game nights are answered in:
// the campaign's real-world (wall-clock) calendar that this viewer may see,
// or "" when there is none. Implemented in internal/app over the calendar
// service, so this plugin never imports the calendar plugin.
type RealWorldCalendarFinder interface {
	RealWorldCalendarID(ctx context.Context, campaignID string, role int, userID string) (string, error)
}

// SetCalendarFinder wires the lookup GameNightsLink uses. Without it every
// game-night link falls back to the Sessions page.
func (h *Handler) SetCalendarFinder(f RealWorldCalendarFinder) {
	h.calendarFinder = f
}

// gameNightsTarget is where a game-night link lands. With a calendar it is
// that calendar's page, opened on the session's night (or, with no session
// named, on the next night); without one it is the Sessions page, or the
// session's own page, so a campaign with no real-world calendar loses
// nothing.
func gameNightsTarget(campaignID, calendarID, sessionID, date string) string {
	base := "/campaigns/" + url.PathEscape(campaignID)
	_, dateErr := time.Parse("2006-01-02", date)
	// A named session with no usable date has no day to open: its own page
	// is the honest landing, not some other night.
	if sessionID != "" && (calendarID == "" || dateErr != nil) {
		return base + "/sessions/" + url.PathEscape(sessionID)
	}
	if calendarID == "" {
		return base + "/sessions"
	}
	q := url.Values{}
	if sessionID != "" {
		q.Set("date", date)
		q.Set("night", sessionID)
	} else {
		q.Set("night", "next")
	}
	return base + "/calendars/" + url.PathEscape(calendarID) + "/view?" + q.Encode()
}

// GameNightsLink sends the sidebar's "Game nights" link and the RSVP card's
// links to the calendar, where members answer. Only members answer there,
// so a public viewer keeps the Sessions page.
// GET /campaigns/:id/game-nights[?session=ID&date=YYYY-MM-DD]
func (h *Handler) GameNightsLink(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	calID := ""
	if h.calendarFinder != nil && cc.MemberRole >= campaigns.RolePlayer {
		id, err := h.calendarFinder.RealWorldCalendarID(c.Request().Context(), cc.Campaign.ID, int(cc.MemberRole), auth.GetUserID(c))
		if err != nil {
			slog.Warn("game nights link: finding the real-world calendar failed", slog.Any("error", err))
		} else {
			calID = id
		}
	}
	return middleware.HTMXRedirect(c, gameNightsTarget(cc.Campaign.ID, calID, c.QueryParam("session"), c.QueryParam("date")))
}

// sidebarNightDate is the date the RSVP card's link opens a session on: a
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

// SessionPlan pre-fills the New Session form when the calendar's "Plan it"
// opens the Sessions page: a date, a start time and the zone that time is
// in. The zero value is no plan, and the form opens empty as before.
type SessionPlan struct {
	Date string
	Time string
	TZ   string
}

// Active reports whether there is a plan to open the form with.
func (p SessionPlan) Active() bool { return p.Date != "" }

// sessionPlanFrom reads ?plan_date=&plan_time=&plan_tz= and keeps only
// well-formed values: a bad date drops the whole plan, a bad time or zone
// drops just that field.
func sessionPlanFrom(date, clock, tz string) SessionPlan {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return SessionPlan{}
	}
	p := SessionPlan{Date: date}
	if _, err := time.Parse("15:04", clock); err == nil {
		p.Time = clock
	}
	if tz != "" && timeutil.IsValidLocation(tz) {
		p.TZ = tz
	}
	return p
}
