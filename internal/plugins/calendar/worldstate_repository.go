// worldstate_repository.go — MariaDB reads/writes for the world-state model
// tables (migration 008 / C-CAL-WORLDSTATE-SERVER-MODEL). Hand-written SQL
// per the project conventions; one method per query, all scoped by
// calendar_id (or moon_id) so a caller can never reach across calendars.
package calendar

import (
	"context"
	"database/sql"
)

// GetDayWeather returns the per-date authored weather row, or nil when none
// exists (the common case — most days carry no bespoke weather).
func (r *calendarRepo) GetDayWeather(ctx context.Context, calendarID string, year, month, day int) (*DayWeather, error) {
	var dw DayWeather
	err := r.db.QueryRowContext(ctx,
		`SELECT calendar_id, year, month, day, weather_type
		 FROM calendar_day_weather
		 WHERE calendar_id = ? AND year = ? AND month = ? AND day = ?`,
		calendarID, year, month, day,
	).Scan(&dw.CalendarID, &dw.Year, &dw.Month, &dw.Day, &dw.WeatherType)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &dw, nil
}

// SetDayWeather upserts the per-date authored weather (one row per date via
// the unique key). Used by the world-state PUT.
func (r *calendarRepo) SetDayWeather(ctx context.Context, calendarID string, year, month, day int, weatherType string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_day_weather (calendar_id, year, month, day, weather_type)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE weather_type = VALUES(weather_type)`,
		calendarID, year, month, day, weatherType,
	)
	return err
}

// GetCelestialEvents returns every celestial event on the given date,
// ordered by start hour then name for a stable seed. Visibility filtering is
// applied at the service/assembler layer (role-aware), not here.
func (r *calendarRepo) GetCelestialEvents(ctx context.Context, calendarID string, year, month, day int) ([]CelestialEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, year, month, day, type, start_hour, duration_hours, name, visibility
		 FROM calendar_celestial_events
		 WHERE calendar_id = ? AND year = ? AND month = ? AND day = ?
		 ORDER BY start_hour, name`,
		calendarID, year, month, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CelestialEvent
	for rows.Next() {
		var ce CelestialEvent
		if err := rows.Scan(&ce.ID, &ce.CalendarID, &ce.Year, &ce.Month, &ce.Day,
			&ce.Type, &ce.StartHour, &ce.DurationHours, &ce.Name, &ce.Visibility); err != nil {
			return nil, err
		}
		out = append(out, ce)
	}
	return out, rows.Err()
}

// AddCelestialEvent inserts a GM-triggered celestial event (the panel's
// trigger-world-event, 4c). The date + visibility are set by the caller; the
// id/created_at default in the table.
func (r *calendarRepo) AddCelestialEvent(ctx context.Context, ce CelestialEvent) error {
	vis := ce.Visibility
	if vis == "" {
		vis = "everyone"
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_celestial_events
		   (calendar_id, year, month, day, type, start_hour, duration_hours, name, visibility)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ce.CalendarID, ce.Year, ce.Month, ce.Day, ce.Type,
		ce.StartHour, ce.DurationHours, ce.Name, vis)
	return err
}

// ClearCelestialEvents removes every celestial event on a calendar's given date
// (C-CAL-GM-PANEL-REWORK B). Calendar-scoped; the date triple is exact so only
// the current day's events are removed.
func (r *calendarRepo) ClearCelestialEvents(ctx context.Context, calendarID string, year, month, day int) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM calendar_celestial_events
		 WHERE calendar_id = ? AND year = ? AND month = ? AND day = ?`,
		calendarID, year, month, day)
	return err
}

// ClearCelestialEventsByType removes one event TYPE's rows on a calendar's
// given date (C-CAL-WORLDSTATE-GM-OVERHAUL — the GM panel's per-event clear
// chip; the clear-all stays ClearCelestialEvents).
func (r *calendarRepo) ClearCelestialEventsByType(ctx context.Context, calendarID string, year, month, day int, eventType string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM calendar_celestial_events
		 WHERE calendar_id = ? AND year = ? AND month = ? AND day = ? AND type = ?`,
		calendarID, year, month, day, eventType)
	return err
}

// GetMoonPhasesForCalendar loads the named-phase vocab for every moon of a
// calendar in one join, returning it keyed by moon id. Moons with no authored
// vocab are simply absent from the map (the assembler falls back to
// procedural names). Joins through calendar_moons so the query stays
// calendar-scoped even though calendar_moon_phases keys on moon_id.
func (r *calendarRepo) GetMoonPhasesForCalendar(ctx context.Context, calendarID string) (map[int][]MoonPhaseVocab, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.moon_id, p.name, p.start_pct, p.end_pct, p.glyph
		 FROM calendar_moon_phases p
		 JOIN calendar_moons m ON m.id = p.moon_id
		 WHERE m.calendar_id = ?
		 ORDER BY p.moon_id, p.sort_order, p.start_pct`,
		calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int][]MoonPhaseVocab)
	for rows.Next() {
		var p MoonPhaseVocab
		if err := rows.Scan(&p.MoonID, &p.Name, &p.StartPct, &p.EndPct, &p.Glyph); err != nil {
			return nil, err
		}
		out[p.MoonID] = append(out[p.MoonID], p)
	}
	return out, rows.Err()
}

// GetSpecialDays returns the special-moon-day flags on the given date.
func (r *calendarRepo) GetSpecialDays(ctx context.Context, calendarID string, year, month, day int) ([]SpecialDay, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT calendar_id, year, month, day, kind
		 FROM calendar_special_days
		 WHERE calendar_id = ? AND year = ? AND month = ? AND day = ?
		 ORDER BY kind`,
		calendarID, year, month, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SpecialDay
	for rows.Next() {
		var sd SpecialDay
		if err := rows.Scan(&sd.CalendarID, &sd.Year, &sd.Month, &sd.Day, &sd.Kind); err != nil {
			return nil, err
		}
		out = append(out, sd)
	}
	return out, rows.Err()
}

// SetMoodTint persists the live mood-tint columns on the calendar row. Both
// nil clears the mood (no wash). Used by the world-state PUT.
func (r *calendarRepo) SetMoodTint(ctx context.Context, calendarID string, color *string, intensity *float64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendars SET mood_tint_color = ?, mood_tint_intensity = ? WHERE id = ?`,
		color, intensity, calendarID)
	return err
}
