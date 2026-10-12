package calendar

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// maxAdvanceDays bounds one step. addDays walks day by day, and the DM
// Screen's steps are hours or a day; a larger jump is a Set today job.
const maxAdvanceDays = 366

// advanceClock moves the calendar's current date and time forward by minutes,
// rolling minutes into hours into days with the calendar's own hours per day
// and minutes per hour, and days into months and years with its month lengths
// and leap rule. Geometry must be loaded. It reports false when the calendar
// cannot hold the move (no day or hour length, or no month with days).
func (c *Calendar) advanceClock(minutes int) (date DayDate, hour, minute int, ok bool) {
	if c.HoursPerDay < 1 || c.MinutesPerHour < 1 || minutes < 0 {
		return DayDate{}, 0, 0, false
	}
	perDay := c.HoursPerDay * c.MinutesPerHour
	if minutes/perDay > maxAdvanceDays {
		return DayDate{}, 0, 0, false
	}
	total := c.CurrentHour*c.MinutesPerHour + c.CurrentMinute + minutes
	days, rem := total/perDay, total%perDay
	date, ok = c.addDays(DayDate{Year: c.CurrentYear, Month: c.CurrentMonth, Day: c.CurrentDay}, days)
	if !ok {
		return DayDate{}, 0, 0, false
	}
	return date, rem / c.MinutesPerHour, rem % c.MinutesPerHour, true
}

// AdvanceCurrent moves the current date and time forward by hours and whole
// days of this calendar's own length (so "a day" is its hours per day, not
// 24), keeping the time of day when only days are added. It refuses what
// SetCurrentDate refuses: a real-time calendar answers 422. The write goes
// through SetCurrentDate, so the date is validated against the calendar's
// geometry again.
//
// It reads then writes without a lock, like Set today; two simultaneous steps
// can land as one.
func (s *calendarService) AdvanceCurrent(ctx context.Context, calendarID, campaignID string, hours, days int) error {
	if hours < 0 || days < 0 {
		return apperror.NewBadRequest("a calendar can only be moved forward")
	}
	cal, err := s.calendarInCampaign(ctx, calendarID, campaignID)
	if err != nil {
		return err
	}
	if cal.UsesRealTime() {
		return apperror.NewValidation("this calendar tracks real-world time; its date cannot be set by hand")
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return err
	}
	if cal.HoursPerDay < 1 || cal.MinutesPerHour < 1 {
		return apperror.NewBadRequest("this calendar's day has no hours")
	}
	if days > maxAdvanceDays || hours > maxAdvanceDays*cal.HoursPerDay {
		return apperror.NewBadRequest("that is too far to step at once; use Set today")
	}
	minutes := (hours + days*cal.HoursPerDay) * cal.MinutesPerHour
	date, hour, minute, ok := cal.advanceClock(minutes)
	if !ok {
		return apperror.NewBadRequest("this calendar cannot move that far")
	}
	return s.SetCurrentDate(ctx, calendarID, campaignID, date.Year, date.Month, date.Day, hour, minute)
}
