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
	// Real-time seam (F2, C-REAL-CALENDAR-P2): every content-authoring branch
	// below (mood/weather/celestial add + clear) keys on cal.Current* as "today".
	// On a real-time calendar the raw GetByID row carries the STALE stored date,
	// so without this the GM's "weather for today" would land on the wrong day.
	// Apply the seam primitive directly (rather than swap to the heavier
	// eager-loading GetCalendarByID) so this hot write path stays cheap; it is a
	// no-op for every non-real-time calendar. RC-5 keeps this content authoring
	// enabled for real-time calendars — only the date-MOVING Advance/Time branches
	// (W4/W5, below) are guarded.
	s.applyRealTime(cal)

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
		// GM weather override (4b): authored weather (calendar_day_weather
		// upsert). Empty string clears to "clear". The target is the CURRENT
		// date unless input.WeatherDate names another day — the additive
		// "weather for this day" path (C-CAL-EDITOR-EXPANSION PR2). Absent
		// WeatherDate is byte-for-byte the prior behavior.
		wt := *input.Weather
		if wt == "" {
			wt = "clear"
		}
		wy, wm, wd := cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay
		if input.WeatherDate != nil {
			wy, wm, wd = input.WeatherDate.Year, input.WeatherDate.Month, input.WeatherDate.Day
			// Structural bounds only — the day must exist on this calendar
			// (leap-aware via MonthDays). The year is deliberately
			// unconstrained: fantasy calendars may use year 0 or negative
			// era years. Keeps a hand-crafted PUT from upserting weather
			// onto a nonexistent date no read path would ever surface.
			if wm < 1 || wm > len(cal.Months) || wd < 1 || wd > cal.MonthDays(wm-1, wy) {
				return apperror.NewValidation("weatherDate is not a date on this calendar")
			}
		}
		if err := s.repo.SetDayWeather(ctx, calendarID, wy, wm, wd, wt); err != nil {
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

	if input.ClearEvents {
		// C-CAL-GM-PANEL-REWORK B: the "stuck meteor" off switch — remove every
		// world-event on the CURRENT day so the rebuilt seed (and the band) drop
		// them. Mirrors the mood Clear; no new seed field needed.
		if err := s.repo.ClearCelestialEvents(ctx, calendarID, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay); err != nil {
			return fmt.Errorf("clear celestial events: %w", err)
		}
	}

	if input.ClearEventType != "" {
		// Per-type clear (the GM panel's per-event × chip). Validated against
		// the same vocabulary as trigger so arbitrary strings never reach SQL.
		if !isKnownCelestialType(input.ClearEventType) {
			return apperror.NewValidation("unknown celestial event type")
		}
		if err := s.repo.ClearCelestialEventsByType(ctx, calendarID, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay, input.ClearEventType); err != nil {
			return fmt.Errorf("clear celestial events by type: %w", err)
		}
	}

	if input.Advance != nil {
		// W4 (C-REAL-CALENDAR-P1): a real-time calendar's date is wall-clock
		// authoritative — reject the GM panel's advance verb. RC-5 keeps
		// worldstate CONTENT authoring (weather/celestial/mood, above) editable;
		// only the date-moving Advance/Time branches are guarded.
		if err := guardManualDateChange(cal); err != nil {
			return err
		}
		// C-CAL-GM-PANEL-REWORK D (deferred audit finding): clamp the relative
		// advance. The V1 advance endpoints bound their input; this PUT path did
		// not, so a huge days/hours/minutes spun advanceClock's day loop — an
		// authenticated CPU DoS. Legit GM verbs (±1 day, +8h) are unaffected.
		if err := validateAdvanceMagnitude(input.Advance); err != nil {
			return err
		}
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
		// W5 (C-REAL-CALENDAR-P1): reject the GM panel's absolute set-time on a
		// real-time calendar (its clock is authoritative). Content authoring
		// stays editable per RC-5.
		if err := guardManualDateChange(cal); err != nil {
			return err
		}
		// Route the date/time write through UpdateCalendar so leap-year and
		// hour/minute invariants apply uniformly across every surface. Nil
		// sub-fields preserve the stored value (partial set).
		t := input.Time
		newYear := derefOr(t.Year, cal.CurrentYear)
		newMonth := derefOr(t.Month, cal.CurrentMonth)
		newDay := derefOr(t.Day, cal.CurrentDay)
		// C-CAL-GM-PANEL-REWORK D (deferred audit finding): bound the absolute
		// set against the calendar's real structure so a GM PUT can't persist an
		// out-of-range month/day (e.g. month:999). UpdateCalendar only enforces
		// the >=1 lower bounds; the month-count / day-count upper bounds need the
		// months loaded.
		months, merr := s.repo.GetMonths(ctx, calendarID)
		if merr != nil {
			return fmt.Errorf("get months: %w", merr)
		}
		cal.Months = months
		if len(months) > 0 {
			if newMonth < 1 || newMonth > len(months) {
				return apperror.NewValidation("month is out of range for this calendar")
			}
			if md := cal.MonthDays(newMonth-1, newYear); newDay < 1 || newDay > md {
				return apperror.NewValidation("day is out of range for that month")
			}
		}
		if err := s.UpdateCalendar(ctx, calendarID, UpdateCalendarInput{
			Name:             cal.Name,
			Description:      cal.Description,
			EpochName:        cal.EpochName,
			Mode:             cal.Mode,
			CurrentYear:      newYear,
			CurrentMonth:     newMonth,
			CurrentDay:       newDay,
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

// knownCelestialTypes is the GM-triggerable celestial event vocabulary —
// the FULL catalog the engine renders (C-CAL-WORLDSTATE-GM-OVERHAUL; ids
// mirror cal-almanac.js SKY_FX / gmCelestialTypes). Expanding it is additive:
// the PUT wire shape is unchanged, old clients simply never send new ids.
var knownCelestialTypes = map[string]bool{
	"meteor-shower": true,
	"meteor-storm":  true,
	"shooting-star": true,
	"star-fall":     true,
	"comet":         true,
	"aurora":        true,
	"arcane-aurora": true,
	"eclipse-solar": true,
	"eclipse-lunar": true,
	"blood-moon":    true,
	"supermoon":     true,
	"harvest-moon":  true,
	"blue-moon":     true,
	"volcanic":      true,
	"plague":        true,
	"ice-age":       true,
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

// validateAdvanceMagnitude bounds a GM relative advance so one PUT can't spin
// advanceClock's day loop for an unbounded count (C-CAL-GM-PANEL-REWORK D —
// resolves the deferred audit DoS finding). Mirrors the V1 advance clamp:
// ±10 years of days, ±10 years of hours/minutes. Real GM verbs (±1 day, +8h)
// are far inside these bounds.
func validateAdvanceMagnitude(a *WorldStateAdvance) error {
	const maxDays, maxHours, maxMinutes = 3650, 87600, 5256000
	if absInt(a.Days) > maxDays || absInt(a.Hours) > maxHours || absInt(a.Minutes) > maxMinutes {
		return apperror.NewValidation("advance amount is out of range")
	}
	return nil
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
