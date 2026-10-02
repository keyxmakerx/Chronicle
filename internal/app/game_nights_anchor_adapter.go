package app

import (
	"context"

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
func (a *gameNightsAnchorMoveAdapter) SessionsInWorldDateRange(ctx context.Context, campaignID string, fromYear, fromMonth, fromDay, toYear, toMonth, toDay, limit int) ([]calendar.AffectedSession, error) {
	list, err := a.svc.ListPlannedSessionsInWorldDateRange(ctx, campaignID,
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
