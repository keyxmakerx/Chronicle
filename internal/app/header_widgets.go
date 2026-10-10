package app

// Header widgets that show live world data (in-world date, today's weather,
// today's moons, the current era, the countdown to the next game night). This file is the one
// place the calendar and sessions services meet the header: the layouts
// package only ever sees formatted strings, and neither plugin imports the
// other. Everything here is read-only and fails quiet, because a bar widget
// must never cost the page.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// headerLiveTimeout bounds the header's data reads so a slow database can
// delay a page by at most this long, never hang it.
const headerLiveTimeout = 1500 * time.Millisecond

// headerCalendarService is the slice of calendar.CalendarService the header
// needs; calendar.CalendarService satisfies it structurally, and a test
// double does not have to implement the plugin's whole surface.
type headerCalendarService interface {
	GetDefaultCalendarForViewer(ctx context.Context, campaignID string, v permissions.Viewer) (*calendar.Calendar, error)
	ListDayWeather(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]calendar.DayWeather, error)
}

// headerNightsService is the slice of sessions.SessionService the header
// needs.
type headerNightsService interface {
	NextGameNight(ctx context.Context, campaignID string, now time.Time) (*sessions.NextNight, error)
}

// headerLiveRequest says which data this request's header can use. Calendar
// and Nights are the gates the caller has already checked (addon on, plugin
// healthy, viewer allowed); Widgets is what the owner put in the bar, so
// nothing is read for a widget that is not there.
type headerLiveRequest struct {
	CampaignID string
	Viewer     permissions.Viewer
	Widgets    []string
	Calendar   bool
	Nights     bool
	Now        time.Time
}

// buildTopbarLive reads today's world data for the widgets in req. It returns
// nil when there is nothing to show. A missing calendar, a calendar the
// viewer may not see, a day with no weather, or any read error leaves just
// that widget empty; errors other than "not found" are logged.
func buildTopbarLive(ctx context.Context, cal headerCalendarService, nights headerNightsService, req headerLiveRequest) *layouts.TopbarLiveData {
	want := map[string]bool{}
	for _, w := range req.Widgets {
		want[w] = true
	}
	live := &layouts.TopbarLiveData{}

	if req.Calendar && cal != nil && (want["date"] || want["weather"] || want["moon"] || want["era"]) {
		c, err := cal.GetDefaultCalendarForViewer(ctx, req.CampaignID, req.Viewer)
		if err != nil {
			logHeaderErr("world date", req.CampaignID, err)
		} else if c != nil {
			if want["date"] {
				live.Date = headerDateLabel(c)
			}
			if want["moon"] {
				live.Moons = headerMoons(c)
			}
			if want["era"] {
				live.Era = headerEraName(c)
			}
			if want["weather"] {
				days, err := cal.ListDayWeather(ctx, c.ID, req.CampaignID, c.CurrentYear, c.CurrentMonth, req.Viewer)
				if err != nil {
					logHeaderErr("weather", req.CampaignID, err)
				} else {
					live.Weather = headerWeatherLabel(days, c.CurrentDay)
				}
			}
		}
	}

	if req.Nights && nights != nil && want["session"] {
		n, err := nights.NextGameNight(ctx, req.CampaignID, req.Now)
		if err != nil {
			logHeaderErr("game night", req.CampaignID, err)
		} else if n != nil {
			live.NextNight.Label, live.NextNight.Title = headerNightLabel(n, req.Now)
		}
	}

	if live.Date == "" && live.Weather == "" && len(live.Moons) == 0 && live.Era == "" && live.NextNight.Label == "" {
		return nil
	}
	return live
}

// headerSkyCalendarID is the calendar the header's Sky background draws:
// the campaign's default calendar as this viewer may see it, or "" when there
// is none, the viewer may not see it, or the read fails. An empty id leaves
// the header on its still night fallback.
func headerSkyCalendarID(ctx context.Context, cal headerCalendarService, campaignID string, v permissions.Viewer) string {
	if cal == nil {
		return ""
	}
	c, err := cal.GetDefaultCalendarForViewer(ctx, campaignID, v)
	if err != nil {
		logHeaderErr("sky", campaignID, err)
		return ""
	}
	if c == nil {
		return ""
	}
	return c.ID
}

// logHeaderErr logs a failed header read. NotFound is the normal "no
// calendar yet / not visible to you" answer, not a fault.
func logHeaderErr(what, campaignID string, err error) {
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
		return
	}
	slog.Warn("header widget data unavailable",
		slog.String("source", what), slog.String("campaign_id", campaignID), slog.Any("error", err))
}

// headerDateLabel is the calendar's current date as "14 Frostfall 1203", or
// "" when the calendar has no month to name (an empty structure).
func headerDateLabel(c *calendar.Calendar) string {
	month := c.CurrentMonthName()
	if month == "" || c.CurrentDay <= 0 {
		return ""
	}
	return fmt.Sprintf("%d %s %d", c.CurrentDay, month, c.CurrentYear)
}

// headerMoons lists each moon's phase today. The calendar service has
// already dropped moons the viewer may not see.
func headerMoons(c *calendar.Calendar) []layouts.TopbarMoon {
	if len(c.Moons) == 0 {
		return nil
	}
	// Moon phases read the same day counter the calendar page uses, so the
	// bar never disagrees with the page.
	day := c.CurrentDayIndex()
	out := make([]layouts.TopbarMoon, 0, len(c.Moons))
	for i := range c.Moons {
		m := &c.Moons[i]
		if m.CycleDays <= 0 {
			continue
		}
		out = append(out, layouts.TopbarMoon{Name: m.Name, Phase: m.MoonPhaseName(day)})
	}
	return out
}

// headerEraName is the name of the era the calendar's current date falls in,
// or "" when it falls in none. The viewer read has already taken out eras a
// player may not see yet, and an era holding today has begun, so it is never
// one of them.
func headerEraName(c *calendar.Calendar) string {
	e := c.CurrentEra()
	if e == nil {
		return ""
	}
	return strings.TrimSpace(e.Name)
}

// headerWeatherLabel finds today's reading in a month's days and words it as
// "Clear, 4°". Only the current day is ever read, so a forecast can never
// leak into the bar.
func headerWeatherLabel(days []calendar.DayWeather, today int) string {
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

// headerNightLabel words the countdown to a game night as a short chip label
// and a fuller hover title. Day distance is counted on the night's own
// calendar (its zone), so "tomorrow" means tomorrow where the table plays.
func headerNightLabel(n *sessions.NextNight, now time.Time) (label, title string) {
	loc := n.At.Location()
	nowLocal := now.In(loc)
	today := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	nightDay := time.Date(n.At.Year(), n.At.Month(), n.At.Day(), 0, 0, 0, 0, loc)
	days := int(math.Round(nightDay.Sub(today).Hours() / 24))

	clock := ""
	if n.Time != "" {
		clock = " " + strings.ToLower(n.At.Format("3:04 pm"))
		clock = strings.Replace(clock, ":00", "", 1)
	}
	var when string
	switch {
	case days <= 0:
		when = "today" + clock
	case days == 1:
		when = "tomorrow" + clock
	case days < 7:
		when = n.At.Format("Mon") + clock
	default:
		when = fmt.Sprintf("in %d days", days)
	}
	label = "Session " + when

	title = n.Name + ": " + n.At.Format("Monday 2 January")
	if n.Time != "" {
		title += ", " + n.At.Format("3:04 pm")
		if n.TZ != "" {
			title += " (" + n.TZ + ")"
		}
	}
	return label, title
}
