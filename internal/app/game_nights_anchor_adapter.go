package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

// gameNightsAnchorMoveAdapter satisfies calendar.GameNightsAffectedByAnchorMove
// from the sessions service, so the calendar's anchor-move preview can name
// real sessions without importing the sessions plugin.
type gameNightsAnchorMoveAdapter struct {
	svc sessions.SessionService
}

// SessionsInWorldDateRange lists planned sessions whose stored in-world date
// falls in the range. A session whose date is incomplete is never returned
// by the sessions query, so every result has all three parts set.
func (a *gameNightsAnchorMoveAdapter) SessionsInWorldDateRange(ctx context.Context, campaignID, calendarID string, fromYear, fromMonth, fromDay, toYear, toMonth, toDay, limit int) ([]calendar.AffectedSession, error) {
	list, err := a.svc.ListPlannedSessionsInWorldDateRange(ctx, campaignID, calendarID,
		sessions.WorldDate{Year: fromYear, Month: fromMonth, Day: fromDay},
		sessions.WorldDate{Year: toYear, Month: toMonth, Day: toDay}, limit)
	if err != nil {
		return nil, err
	}
	out := make([]calendar.AffectedSession, 0, len(list))
	for _, s := range list {
		if !s.HasCalendarDate() {
			continue
		}
		out = append(out, calendar.AffectedSession{
			Name:         s.Name,
			OldWorldYear: *s.CalendarYear, OldWorldMonth: *s.CalendarMonth, OldWorldDay: *s.CalendarDay,
		})
	}
	return out, nil
}

// realWorldCalendarFinderAdapter satisfies sessions.RealWorldCalendarFinder:
// the calendar a game-night link opens is the campaign's real-world calendar
// this viewer may see, its default calendar first when that one qualifies.
type realWorldCalendarFinderAdapter struct {
	svc calendar.CalendarService
}

func (a *realWorldCalendarFinderAdapter) RealWorldCalendarID(ctx context.Context, campaignID string, role int, userID string) (string, error) {
	cals, err := a.svc.ListCalendars(ctx, campaignID, permissions.RequestViewer(role, userID))
	if err != nil {
		return "", err
	}
	return pickRealWorldCalendar(cals), nil
}

// pickRealWorldCalendar returns the default calendar if it follows the real
// clock, else the first one that does (cals is in sidebar order), else "".
func pickRealWorldCalendar(cals []calendar.Calendar) string {
	first := ""
	for i := range cals {
		if !cals[i].UsesRealTime() {
			continue
		}
		if cals[i].IsDefault {
			return cals[i].ID
		}
		if first == "" {
			first = cals[i].ID
		}
	}
	return first
}

// sessionMonthRemapAdapter satisfies calendar.SessionMonthRemapper from the
// sessions service, so a structure edit that reorders months moves game nights
// with them without the calendar importing the sessions plugin.
type sessionMonthRemapAdapter struct {
	svc sessions.SessionService
}

func (a *sessionMonthRemapAdapter) RemapMonthPositions(ctx context.Context, campaignID, calendarID string, remap map[int]int) error {
	_, err := a.svc.RemapMonthPositions(ctx, campaignID, calendarID, remap)
	return err
}

// sessionsDefaultCalendarAdapter satisfies sessions.DefaultCalendarResolver:
// the calendar a session's in-world date is recorded against is the
// campaign's primary calendar, the default or else the first (the session
// form has no calendar picker). Most campaigns never mark a default, so
// requiring one would leave their game nights unstamped and outside every
// calendar-scoped query.
type sessionsDefaultCalendarAdapter struct {
	svc calendar.CalendarService
}

// DefaultCalendarID reads at Owner level (which skips per-user rules): the
// answer is stored on a session row, not shown to anyone, so it must not
// depend on who happens to be saving. It returns "" for a campaign with no
// calendar.
func (a *sessionsDefaultCalendarAdapter) DefaultCalendarID(ctx context.Context, campaignID string) (string, error) {
	cal, err := a.svc.GetPrimaryCalendarForViewer(ctx, campaignID, permissions.RequestViewer(permissions.RoleOwner, ""))
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == http.StatusNotFound {
			return "", nil
		}
		return "", err
	}
	return cal.ID, nil
}

// armoryNightsAdapter satisfies armory.NightLister: a hero's log groups its
// lines under the campaign's game nights that have a date and were not
// cancelled.
type armoryNightsAdapter struct {
	svc sessions.SessionService
}

func (a *armoryNightsAdapter) Nights(ctx context.Context, campaignID string) ([]armory.Night, error) {
	list, err := a.svc.ListSessions(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]armory.Night, 0, len(list))
	for _, s := range list {
		if s.Status == sessions.StatusCancelled || s.ScheduledDate == nil || *s.ScheduledDate == "" {
			continue
		}
		out = append(out, armory.Night{ID: s.ID, Name: s.Name, Date: *s.ScheduledDate})
	}
	return out, nil
}

// sessionEventSourceAdapter satisfies sessions.SessionEventSource: the
// Session-kind events of the campaign's primary calendar, the one a night's
// in-world date is recorded against, worded in that calendar's own months.
type sessionEventSourceAdapter struct {
	svc calendar.CalendarService
}

func (a *sessionEventSourceAdapter) SessionEvents(ctx context.Context, campaignID string, role int) ([]sessions.SessionEvent, error) {
	cal, err := a.svc.GetPrimaryCalendarForViewer(ctx, campaignID, permissions.RequestViewer(role, ""))
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	events, err := a.svc.ListEventsForCalendar(ctx, campaignID, cal.ID, role)
	if err != nil {
		return nil, err
	}
	return sessionEventsOf(cal, events), nil
}

// sessionEventsOf keeps the one-off events of the Session kind. A repeating
// event stands for many days, so it is not one night and is left out.
func sessionEventsOf(cal *calendar.Calendar, events []calendar.Event) []sessions.SessionEvent {
	var out []sessions.SessionEvent
	for i := range events {
		e := &events[i]
		if !strings.EqualFold(e.KindSlug, sessions.SessionEventKindSlug) || e.IsRecurring || strings.TrimSpace(e.Name) == "" {
			continue
		}
		ev := sessions.SessionEvent{ID: e.ID, Name: e.Name, CalendarID: e.CalendarID, Year: e.Year, Month: e.Month, Day: e.Day, EntityName: e.EntityName}
		if e.EntityID != nil {
			ev.EntityID = *e.EntityID
		}
		if cal.Mode == calendar.ModeRealLife {
			// A real-life calendar's date is a real day; keep it only when it is one.
			if t := time.Date(e.Year, time.Month(e.Month), e.Day, 12, 0, 0, 0, time.UTC); t.Year() == e.Year && int(t.Month()) == e.Month && t.Day() == e.Day {
				ev.RealDate = t.Format("2006-01-02")
				ev.When = t.Format("2 Jan 2006")
			}
		}
		if ev.When == "" {
			ev.When = storyDateWords(cal, e.Year, e.Month, e.Day)
		}
		out = append(out, ev)
	}
	return out
}

// storyDateWords words a story calendar's date, "14 Hammer, 1492", with the
// month's number when the calendar has no month at that position.
func storyDateWords(cal *calendar.Calendar, year, month, day int) string {
	m := strconv.Itoa(month)
	if month >= 1 && month <= len(cal.Months) {
		m = cal.Months[month-1].Name
	}
	return strconv.Itoa(day) + " " + m + ", " + strconv.Itoa(year)
}
