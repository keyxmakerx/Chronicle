package calendar

// The campaign home's "Today in the world" block: the in-world date, today's
// weather, the moons and the next event on the calendar.
//
// It reads through the same service calls the header's date, weather and moon
// widgets and the Coming up card use, so what a viewer may see is decided by
// the calendar and not here: only today's weather is ever read (a forecast
// cannot leak), moons a player may not know are already gone from the
// calendar, and the next event is the first one the viewer's own filter
// leaves. Each line is worded before it reaches the template, and a line with
// no data is left out.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// todayTimeout bounds the block's reads so a slow database delays only this
// card, never the rest of the home.
const todayTimeout = 3 * time.Second

// todaySource is the slice of CalendarService the block reads.
type todaySource interface {
	GetDefaultCalendarForViewer(ctx context.Context, campaignID string, v permissions.Viewer) (*Calendar, error)
	ListDayWeather(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]DayWeather, error)
	ListUpcomingEvents(ctx context.Context, calendarID, campaignID string, limit int, v permissions.Viewer) ([]Event, error)
}

// todayView is what the template draws.
type todayView struct {
	Date    string
	Weather string
	// Moons are worded one per moon: "Selune, waning crescent".
	Moons []string
	// Next is "Festival of Lanterns in 3 days"; empty when nothing is coming.
	Next         string
	CalendarHref string
	// SetUp is true only for the owner of a campaign with no calendar yet,
	// who is told where it comes from; everyone else gets nothing.
	SetUp bool
}

// Empty reports that there is nothing to draw.
func (v todayView) Empty() bool {
	return !v.SetUp && v.Date == "" && v.Weather == "" && len(v.Moons) == 0 && v.Next == ""
}

// todayMoons words each moon's phase today. A hand-set moon with no state
// this viewer may know yet, and a moon with no cycle, are left out, as the
// header leaves them.
func todayMoons(c *Calendar) []string {
	day := c.CurrentDayIndex()
	var out []string
	for i := range c.Moons {
		m := &c.Moons[i]
		if m.Manual() && m.StateOn(day) == "" || !m.Manual() && m.CycleDays <= 0 {
			continue
		}
		out = append(out, fmt.Sprintf("%s, %s", m.Name, strings.ToLower(m.MoonPhaseName(day))))
	}
	return out
}

// todayWeather words today's reading as "Light rain, 4°". Only the current
// day is looked at.
func todayWeather(days []DayWeather, today int) string {
	for _, d := range days {
		if d.Day != today {
			continue
		}
		var parts []string
		if d.PresetLabel != nil && strings.TrimSpace(*d.PresetLabel) != "" {
			parts = append(parts, strings.TrimSpace(*d.PresetLabel))
		}
		if d.TemperatureCelsius != nil {
			parts = append(parts, fmt.Sprintf("%d°", int(math.Round(*d.TemperatureCelsius))))
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

// todayInDays words a distance in in-world days.
func todayInDays(days int) string {
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "tomorrow"
	}
	return fmt.Sprintf("in %d days", days)
}

// buildToday reads each part independently and words it. A part that fails is
// logged and left out; the owner of a campaign with no calendar is told so.
func buildToday(ctx context.Context, src todaySource, campaignID string, v permissions.Viewer, owner bool) todayView {
	cal, err := src.GetDefaultCalendarForViewer(ctx, campaignID, v)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			// A viewer who may see no calendar is not told one exists.
			return todayView{SetUp: owner}
		}
		slog.Warn("today in the world: calendar unavailable",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return todayView{}
	}
	if cal == nil {
		return todayView{SetUp: owner}
	}
	out := todayView{
		CalendarHref: "/campaigns/" + url.PathEscape(campaignID) + "/calendars/" + url.PathEscape(cal.ID) + "/view",
	}
	if cal.CurrentMonthName() != "" && cal.CurrentDay > 0 {
		out.Date = cal.FullDateLabel()
	}
	out.Moons = todayMoons(cal)

	days, err := src.ListDayWeather(ctx, cal.ID, campaignID, cal.CurrentYear, cal.CurrentMonth, v)
	if err != nil {
		slog.Warn("today in the world: weather unavailable",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
	} else {
		out.Weather = todayWeather(days, cal.CurrentDay)
	}

	events, err := src.ListUpcomingEvents(ctx, cal.ID, campaignID, 1, v)
	if err != nil {
		slog.Warn("today in the world: events unavailable",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
	} else if len(events) > 0 && strings.TrimSpace(events[0].Name) != "" {
		e := events[0]
		gap := cal.AbsoluteDay(e.Year, e.Month, e.Day) - cal.CurrentAbsoluteDay()
		out.Next = strings.TrimSpace(e.Name) + " " + todayInDays(gap)
	}
	if out.Date == "" && out.Weather == "" && len(out.Moons) == 0 && out.Next == "" {
		return todayView{}
	}
	return out
}

// TodayInWorldBlock draws the block, or an empty answer that removes the
// placeholder. It reads as the players' audience for an owner who is viewing
// as a player, and always through the viewer's own filter otherwise. A guest
// of a public campaign gets it only where the owner placed it on the Guests
// home, and then only what the public may see of the calendar.
// GET /campaigns/:id/today-in-world
func (h *Handler) TodayInWorldBlock(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	// A visitor sees the block only where the owner placed it on the Guests
	// home; the route itself is reachable on any public campaign.
	if !cc.IsMember && !campaigns.GuestHomeHasBlock(cc.Campaign, campaigns.BlockTodayInWorld) {
		return c.HTML(http.StatusOK, "")
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), todayTimeout)
	defer cancel()
	v := viewerFrom(c, cc)
	owner := cc.MemberRole >= campaigns.RoleOwner
	if campaigns.ViewingAsPlayer(c, cc) {
		v = permissions.RequestViewer(int(campaigns.RolePlayer), v.UserID())
		owner = false
	}
	view := buildToday(ctx, h.svc, cc.Campaign.ID, v, owner)
	if view.Empty() {
		return c.HTML(http.StatusOK, "")
	}
	return middleware.Render(c, http.StatusOK, todayInWorldBlock(view))
}
