package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/permissions"
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

// sessionsDefaultCalendarAdapter satisfies sessions.DefaultCalendarResolver:
// the calendar a session's in-world date is recorded against is the
// campaign's default calendar (the session form has no calendar picker).
type sessionsDefaultCalendarAdapter struct {
	svc calendar.CalendarService
}

// DefaultCalendarID reads at Owner level (which skips per-user rules): the
// answer is stored on a session row, not shown to anyone, so it must not
// depend on who happens to be saving. It returns "" for a campaign with no
// default calendar.
func (a *sessionsDefaultCalendarAdapter) DefaultCalendarID(ctx context.Context, campaignID string) (string, error) {
	cals, err := a.svc.ListCalendars(ctx, campaignID, permissions.RequestViewer(permissions.RoleOwner, ""))
	if err != nil {
		return "", err
	}
	for i := range cals {
		if cals[i].IsDefault {
			return cals[i].ID, nil
		}
	}
	return "", nil
}
