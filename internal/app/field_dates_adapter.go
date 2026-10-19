package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fieldDatesAdapter gives pages' calendar date fields the campaign's
// calendar (entities.FieldCalendar). It reads as the system owner, like the
// quests' due dates: a date's label and day count are not secrets, and the
// fields' own flags decide who sees a value.
type fieldDatesAdapter struct {
	svc    calendar.CalendarService
	addons questAddonChecker
}

func (a fieldDatesAdapter) Dates(ctx context.Context, campaignID string) (entities.FieldDateBook, error) {
	on, err := a.addons.IsEnabledForCampaign(ctx, campaignID, calendar.PluginSlug)
	if err != nil || !on {
		return nil, err
	}
	cal, err := a.svc.GetPrimaryCalendarForViewer(ctx, campaignID, questCalendarViewer())
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(cal.Months) == 0 {
		return nil, nil
	}
	return fieldDateBook{cal: cal}, nil
}

// fieldDateBook is entities.FieldDateBook over a loaded calendar; the date
// maths is the calendar's own.
type fieldDateBook struct{ cal *calendar.Calendar }

func (b fieldDateBook) Valid(y, m, d int) bool   { return b.cal.ValidDate(y, m, d) }
func (b fieldDateBook) Label(y, m, d int) string { return b.cal.DateLabel(y, m, d) }
func (b fieldDateBook) Today() (int, int, int) {
	return b.cal.CurrentYear, b.cal.CurrentMonth, b.cal.CurrentDay
}
func (b fieldDateBook) DaysSince(y, m, d int) int {
	return b.cal.CurrentDayIndex() - b.cal.DayNumber(y, m, d)
}

func (b fieldDateBook) LabelAfter(y, m, d, n int) string {
	at, ok := b.cal.AddDays(calendar.DayDate{Year: y, Month: m, Day: d}, n)
	if !ok {
		return ""
	}
	return b.cal.DateLabel(at.Year, at.Month, at.Day)
}

func (b fieldDateBook) Months() []entities.FieldDateMonth {
	out := make([]entities.FieldDateMonth, 0, len(b.cal.Months))
	for _, m := range b.cal.Months {
		out = append(out, entities.FieldDateMonth{Name: m.Name, Days: m.Days})
	}
	return out
}
