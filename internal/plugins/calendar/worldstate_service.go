// worldstate_service.go — service-layer assembly + write for the world-state
// model (C-CAL-WORLDSTATE-SERVER-MODEL). The heart of the dispatch is
// BuildWorldStateSeed: it loads the existing calendar/moon/season state plus
// the migration-008 tables and hands them to the pure AssembleWorldStateSeed
// (worldstate.go) which emits the Part-8 shape the showcase consumes.
package calendar

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// BuildWorldStateSeed assembles the world-state seed for a date. role/userID
// gate GM-only celestial events (the load filtering happens in the pure
// assembler so it is exercised by the unit tests without a DB). A month/day
// of 0 defaults to the calendar's current date; year 0 is a valid year so it
// is only defaulted when month/day are also unset.
func (s *calendarService) BuildWorldStateSeed(ctx context.Context, calendarID string, year, month, day, role int, userID string) (*WorldStateSeed, error) {
	cal, err := s.GetCalendarByID(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return nil, apperror.NewNotFound("calendar not found")
	}

	// Default to the calendar's current date when the caller didn't pin one.
	if month < 1 && day < 1 {
		year, month, day = cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay
	}

	dayWeather, err := s.repo.GetDayWeather(ctx, calendarID, year, month, day)
	if err != nil {
		return nil, fmt.Errorf("get day weather: %w", err)
	}
	celestials, err := s.repo.GetCelestialEvents(ctx, calendarID, year, month, day)
	if err != nil {
		return nil, fmt.Errorf("get celestial events: %w", err)
	}
	moonPhases, err := s.repo.GetMoonPhasesForCalendar(ctx, calendarID)
	if err != nil {
		return nil, fmt.Errorf("get moon phases: %w", err)
	}

	return AssembleWorldStateSeed(cal, year, month, day, dayWeather, celestials, moonPhases, role), nil
}

// SetWorldState persists the writable world-state (live mood + date/time) and
// publishes calendar.worldstate.changed. Role gating (Owner-for-now, with a
// seam for Phase-3 co-GM) lives at the route layer; this method assumes the
// caller is authorized.
//
// The WS payload is deliberately minimal (date + mood, no celestial events)
// so a player WS subscriber never receives GM-only events through the change
// signal — clients re-GET the seed with their own role to refresh.
func (s *calendarService) SetWorldState(ctx context.Context, calendarID string, input WorldStateUpdateInput) error {
	cal, err := s.repo.GetByID(ctx, calendarID)
	if err != nil {
		return fmt.Errorf("get calendar: %w", err)
	}
	if cal == nil {
		return apperror.NewNotFound("calendar not found")
	}

	if input.Mood != nil {
		var intensity *float64
		// Persist intensity only when a color is set; a cleared color means
		// "no wash", so both columns go NULL together.
		if input.Mood.Color != nil {
			i := input.Mood.Intensity
			intensity = &i
		}
		if err := s.repo.SetMoodTint(ctx, calendarID, input.Mood.Color, intensity); err != nil {
			return fmt.Errorf("set mood tint: %w", err)
		}
	}

	if input.Time != nil {
		// Route the date/time write through UpdateCalendar so leap-year and
		// hour/minute invariants apply uniformly across every surface. Nil
		// sub-fields preserve the stored value (partial set).
		t := input.Time
		if err := s.UpdateCalendar(ctx, calendarID, UpdateCalendarInput{
			Name:             cal.Name,
			Description:      cal.Description,
			EpochName:        cal.EpochName,
			Mode:             cal.Mode,
			CurrentYear:      derefOr(t.Year, cal.CurrentYear),
			CurrentMonth:     derefOr(t.Month, cal.CurrentMonth),
			CurrentDay:       derefOr(t.Day, cal.CurrentDay),
			CurrentHour:      derefOr(t.Hour, cal.CurrentHour),
			CurrentMinute:    derefOr(t.Minute, cal.CurrentMinute),
			HoursPerDay:      cal.HoursPerDay,
			MinutesPerHour:   cal.MinutesPerHour,
			SecondsPerMinute: cal.SecondsPerMinute,
			LeapYearEvery:    cal.LeapYearEvery,
			LeapYearOffset:   cal.LeapYearOffset,
		}); err != nil {
			return err
		}
	}

	// Re-read so the change payload reflects the just-written state.
	updated, err := s.repo.GetByID(ctx, calendarID)
	if err == nil && updated != nil {
		cal = updated
	}
	s.events.PublishCalendarEvent("calendar.worldstate.changed", cal.CampaignID, calendarID, map[string]any{
		"date": map[string]int{
			"year":  cal.CurrentYear,
			"month": cal.CurrentMonth,
			"day":   cal.CurrentDay,
		},
		"moodTint": moodTintSeed(cal),
	})
	return nil
}

// derefOr returns *p when p is non-nil, else fallback. Used by SetWorldState
// for partial date/time sets.
func derefOr(p *int, fallback int) int {
	if p != nil {
		return *p
	}
	return fallback
}
