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

	if input.Weather != nil {
		// GM weather override (4b): authored weather for the CURRENT date
		// (calendar_day_weather upsert). Empty string clears to "clear".
		wt := *input.Weather
		if wt == "" {
			wt = "clear"
		}
		if err := s.repo.SetDayWeather(ctx, calendarID, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay, wt); err != nil {
			return fmt.Errorf("set day weather: %w", err)
		}
	}

	if input.TriggerEvent != nil {
		// GM trigger-world-event (4c): add a celestial event on the CURRENT
		// date. Type is validated against the known set; visibility was
		// already capability-resolved by the handler.
		te := input.TriggerEvent
		if !isKnownCelestialType(te.Type) {
			return apperror.NewValidation("unknown celestial event type")
		}
		if err := s.repo.AddCelestialEvent(ctx, CelestialEvent{
			CalendarID:    calendarID,
			Year:          cal.CurrentYear,
			Month:         cal.CurrentMonth,
			Day:           cal.CurrentDay,
			Type:          te.Type,
			StartHour:     te.StartHour,
			DurationHours: te.DurationHours,
			Name:          te.Name,
			Visibility:    te.Visibility,
		}); err != nil {
			return fmt.Errorf("add celestial event: %w", err)
		}
	}

	if input.Advance != nil {
		// Relative clock move (GM panel verbs) with full signed rollover.
		// Computed here + written through UpdateCalendar so the same
		// leap-year/range invariants apply. Loads months for variable-length
		// day rollover.
		months, err := s.repo.GetMonths(ctx, calendarID)
		if err != nil {
			return fmt.Errorf("get months: %w", err)
		}
		if len(months) == 0 {
			return apperror.NewValidation("calendar has no months configured")
		}
		cal.Months = months
		y, mo, d, h, mi := advanceClock(cal, input.Advance.Days, input.Advance.Hours, input.Advance.Minutes)
		if err := s.UpdateCalendar(ctx, calendarID, UpdateCalendarInput{
			Name:             cal.Name,
			Description:      cal.Description,
			EpochName:        cal.EpochName,
			Mode:             cal.Mode,
			CurrentYear:      y,
			CurrentMonth:     mo,
			CurrentDay:       d,
			CurrentHour:      h,
			CurrentMinute:    mi,
			HoursPerDay:      cal.HoursPerDay,
			MinutesPerHour:   cal.MinutesPerHour,
			SecondsPerMinute: cal.SecondsPerMinute,
			LeapYearEvery:    cal.LeapYearEvery,
			LeapYearOffset:   cal.LeapYearOffset,
		}); err != nil {
			return err
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

// advanceClock moves the calendar's current date/time by a signed
// days/hours/minutes delta, returning the new (year, month, day, hour, minute)
// with full rollover in BOTH directions. Requires cal.Months populated for
// the variable-length day rollover (leap-year aware via cal.MonthDays). The
// GM panel's Part-6 verbs (+1hr / +1day / +long-rest / step-back) all funnel
// through here so rollover lives server-side (the source of truth), not in JS.
func advanceClock(cal *Calendar, days, hours, minutes int) (year, month, day, hour, minute int) {
	hpd := cal.HoursPerDay
	if hpd <= 0 {
		hpd = 24
	}
	mph := cal.MinutesPerHour
	if mph <= 0 {
		mph = 60
	}
	minutesPerDay := hpd * mph

	// Fold the current within-day time + the hour/minute deltas into a single
	// minute count, then floor-divide into a (signed) day carry + a positive
	// minute-of-day. Floored math is required so negative deltas borrow a day.
	totalMin := cal.CurrentHour*mph + cal.CurrentMinute + hours*mph + minutes
	dayDelta := days + floorDiv(totalMin, minutesPerDay)
	minOfDay := floorMod(totalMin, minutesPerDay)
	hour = minOfDay / mph
	minute = minOfDay % mph

	year = cal.CurrentYear
	monthIdx := cal.CurrentMonth - 1
	day = cal.CurrentDay
	n := len(cal.Months)

	if dayDelta >= 0 {
		for i := 0; i < dayDelta; i++ {
			day++
			if day > cal.MonthDays(monthIdx, year) {
				day = 1
				monthIdx++
				if monthIdx >= n {
					monthIdx = 0
					year++
				}
			}
		}
	} else {
		for i := 0; i < -dayDelta; i++ {
			day--
			if day < 1 {
				monthIdx--
				if monthIdx < 0 {
					monthIdx = n - 1
					year--
				}
				day = cal.MonthDays(monthIdx, year)
			}
		}
	}
	return year, monthIdx + 1, day, hour, minute
}

// floorDiv / floorMod are floored (not truncated-toward-zero) integer
// division — needed so a negative minute total borrows a whole day correctly.
func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}
func floorMod(a, b int) int {
	m := a % b
	if m != 0 && ((m < 0) != (b < 0)) {
		m += b
	}
	return m
}

// knownCelestialTypes is the GM-triggerable celestial event vocabulary (4c).
// Matches the showcase CELESTIAL_EFFECTS MUST tier + blood-moon.
var knownCelestialTypes = map[string]bool{
	"meteor-shower": true,
	"eclipse-solar": true,
	"eclipse-lunar": true,
	"blood-moon":    true,
}

// isKnownCelestialType guards the trigger-event write against arbitrary types.
func isKnownCelestialType(t string) bool { return knownCelestialTypes[t] }

// derefOr returns *p when p is non-nil, else fallback. Used by SetWorldState
// for partial date/time sets.
func derefOr(p *int, fallback int) int {
	if p != nil {
		return *p
	}
	return fallback
}
