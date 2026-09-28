// Package calendar - repository.go is the calendar aggregate's persistence:
// the calendar row itself and its structural sub-resources (months,
// weekdays, moons, seasons, eras, cycles, festivals). Hand-written SQL, one
// repository per aggregate root, per the project convention. Events, event
// kinds and weather are separate aggregate roots with their own files.
package calendar

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// CalendarRepository defines persistence for calendars and their structure.
type CalendarRepository interface {
	// Calendar CRUD.
	Create(ctx context.Context, cal *Calendar) error
	GetByID(ctx context.Context, id string) (*Calendar, error)
	GetDefaultByCampaignID(ctx context.Context, campaignID string) (*Calendar, error)
	ListByCampaignID(ctx context.Context, campaignID string) ([]Calendar, error)
	SetDefault(ctx context.Context, campaignID, calendarID string) error
	Update(ctx context.Context, cal *Calendar) error
	Delete(ctx context.Context, id string) error
	// UpdateVisibility sets the per-calendar visibility + rules, the
	// calendar-level mirror of EventRepository.UpdateEventVisibility.
	UpdateVisibility(ctx context.Context, calendarID string, visibility string, visRules *string) error

	// Months.
	SetMonths(ctx context.Context, calendarID string, months []MonthInput) error
	GetMonths(ctx context.Context, calendarID string) ([]Month, error)

	// Weekdays.
	SetWeekdays(ctx context.Context, calendarID string, weekdays []WeekdayInput) error
	GetWeekdays(ctx context.Context, calendarID string) ([]Weekday, error)

	// Moons.
	SetMoons(ctx context.Context, calendarID string, moons []MoonInput) error
	GetMoons(ctx context.Context, calendarID string) ([]Moon, error)
	// SetMoonHidden toggles one moon's HiddenFromPlayers flag without
	// disturbing the rest of the moon set (a bulk SetMoons replace-all would
	// otherwise force every caller to resend the whole list for a one-flag
	// change). Scoped to calendarID so a moon id from another calendar can't
	// be toggled; returns apperror.NewNotFound when no row matches both ids.
	SetMoonHidden(ctx context.Context, calendarID string, moonID int, hidden bool) error

	// Seasons.
	SetSeasons(ctx context.Context, calendarID string, seasons []Season) error
	GetSeasons(ctx context.Context, calendarID string) ([]Season, error)

	// Eras (day-granular start/end; see Era's doc comment).
	SetEras(ctx context.Context, calendarID string, eras []EraInput) error
	CreateEra(ctx context.Context, calendarID string, input EraInput) (*Era, error)
	// UpdateEra and DeleteEra are scoped to calendarID so an era id can't be
	// reached through the wrong calendar.
	UpdateEra(ctx context.Context, calendarID string, eraID int, input EraInput) error
	DeleteEra(ctx context.Context, calendarID string, eraID int) error
	GetEraByID(ctx context.Context, eraID int) (*Era, error)
	GetEras(ctx context.Context, calendarID string) ([]Era, error)

	// Cycles (with their entries).
	SetCycles(ctx context.Context, calendarID string, cycles []CycleInput) error
	GetCycles(ctx context.Context, calendarID string) ([]Cycle, error)

	// Festivals.
	SetFestivals(ctx context.Context, calendarID string, festivals []FestivalInput) error
	GetFestivals(ctx context.Context, calendarID string) ([]Festival, error)

	// ApplyImport runs the calendar-import write workflow (calendar fields +
	// months/weekdays/moons/seasons/eras) in one transaction, so a partial
	// failure can't leave a calendar in a mixed state. The caller validates
	// inputs before calling this; validation after BeginTx defeats the point
	// of validating upfront. Cycles are not part of ImportResult (no import
	// format supplies them). Calendaria festivals are parsed and then
	// dropped rather than carried into the result: TODO(#771) add a
	// Festivals field to ImportResult and write it here via SetFestivals.
	// result.Events (#779) is deliberately NOT written by this method: an
	// event's kind slug must be resolved against the target campaign's
	// calendar_event_kinds, which is a cross-aggregate lookup this
	// single-aggregate repository has no business making — see
	// CalendarService.CreateCalendarFromImport, which calls this first and
	// then recreates the events itself via EventRepository.
	ApplyImport(ctx context.Context, cal *Calendar, result *ImportResult) error
}

// calendarRepo is the MariaDB implementation of CalendarRepository.
type calendarRepo struct {
	db *sql.DB
}

// NewCalendarRepository creates a new MariaDB-backed calendar repository.
func NewCalendarRepository(db *sql.DB) CalendarRepository {
	return &calendarRepo{db: db}
}

// calendarCols is the column list for calendar queries. scanCalendar reads
// this list POSITIONALLY, so a new column is appended here, never inserted
// mid-list.
const calendarCols = `id, campaign_id, mode, name, description, epoch_name, current_year,
        current_month, current_day, hours_per_day, minutes_per_hour, seconds_per_minute,
        current_hour, current_minute, leap_year_every, leap_year_offset,
        sort_order, is_default, created_at, updated_at,
        hemisphere, forecasts_enabled, month_starts_new_week,
        visibility, visibility_rules,
        tracks_real_time, real_time_zone,
        anchor_year, anchor_month, anchor_day, anchor_real_date`

// scanCalendar reads a row into a Calendar struct. Returns (nil, nil) when
// the row doesn't exist, so callers that treat "no default calendar yet" as
// a legitimate state (GetDefaultByCampaignID) don't need a sentinel error;
// GetByID -- where a nil id genuinely means not-found -- converts that nil
// into apperror.NewNotFound itself rather than propagating it.
func scanCalendar(scanner interface{ Scan(...any) error }) (*Calendar, error) {
	cal := &Calendar{}
	err := scanner.Scan(&cal.ID, &cal.CampaignID, &cal.Mode,
		&cal.Name, &cal.Description, &cal.EpochName,
		&cal.CurrentYear, &cal.CurrentMonth, &cal.CurrentDay,
		&cal.HoursPerDay, &cal.MinutesPerHour, &cal.SecondsPerMinute,
		&cal.CurrentHour, &cal.CurrentMinute,
		&cal.LeapYearEvery, &cal.LeapYearOffset,
		&cal.SortOrder, &cal.IsDefault,
		&cal.CreatedAt, &cal.UpdatedAt,
		&cal.Hemisphere, &cal.ForecastsEnabled, &cal.MonthStartsNewWeek,
		&cal.Visibility, &cal.VisibilityRules,
		&cal.TracksRealTime, &cal.RealTimeZone,
		&cal.AnchorYear, &cal.AnchorMonth, &cal.AnchorDay, &cal.AnchorRealDate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return cal, err
}

// Create inserts a new calendar.
func (r *calendarRepo) Create(ctx context.Context, cal *Calendar) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendars (id, campaign_id, mode, name, description, epoch_name,
		        current_year, current_month, current_day,
		        hours_per_day, minutes_per_hour, seconds_per_minute,
		        current_hour, current_minute,
		        leap_year_every, leap_year_offset,
		        sort_order, is_default,
		        hemisphere, forecasts_enabled, month_starts_new_week,
		        visibility, visibility_rules,
		        tracks_real_time, real_time_zone)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cal.ID, cal.CampaignID, cal.Mode, cal.Name, cal.Description, cal.EpochName,
		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay,
		cal.HoursPerDay, cal.MinutesPerHour, cal.SecondsPerMinute,
		cal.CurrentHour, cal.CurrentMinute,
		cal.LeapYearEvery, cal.LeapYearOffset,
		cal.SortOrder, cal.IsDefault,
		cal.Hemisphere, cal.ForecastsEnabled, cal.MonthStartsNewWeek,
		cal.Visibility, cal.VisibilityRules,
		cal.TracksRealTime, cal.RealTimeZone,
	)
	return err
}

// GetByID returns a calendar by its ID. Returns apperror.NewNotFound when it
// doesn't exist: middleware.RequireInCampaign calls a method on the result,
// which would panic on a bare nil pointer. Campaign ownership is still the
// caller's job (RequireInCampaign against Calendar.GetCampaignID); this only
// answers "does this id exist".
func (r *calendarRepo) GetByID(ctx context.Context, id string) (*Calendar, error) {
	cal, err := scanCalendar(r.db.QueryRowContext(ctx,
		`SELECT `+calendarCols+` FROM calendars WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if cal == nil {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return cal, nil
}

// GetDefaultByCampaignID returns the default calendar for a campaign, or nil.
func (r *calendarRepo) GetDefaultByCampaignID(ctx context.Context, campaignID string) (*Calendar, error) {
	return scanCalendar(r.db.QueryRowContext(ctx,
		`SELECT `+calendarCols+` FROM calendars WHERE campaign_id = ? AND is_default = 1`, campaignID))
}

// ListByCampaignID returns all calendars for a campaign, ordered by sort_order.
func (r *calendarRepo) ListByCampaignID(ctx context.Context, campaignID string) ([]Calendar, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+calendarCols+` FROM calendars WHERE campaign_id = ? ORDER BY sort_order, name`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calendars []Calendar
	for rows.Next() {
		cal, err := scanCalendar(rows)
		if err != nil {
			return nil, err
		}
		if cal != nil {
			calendars = append(calendars, *cal)
		}
	}
	return calendars, rows.Err()
}

// SetDefault marks one calendar as the default for its campaign, unsetting
// the default flag on all other calendars in the same campaign. Both
// statements run in one transaction so a crash between them can never leave
// a campaign with two defaults or none. The blanket first UPDATE always
// clears calendarID's own flag too (if it belongs to campaignID), so the
// second UPDATE's row -- when it matches -- is always a real 0-to-1 change;
// RowsAffected != 1 therefore means calendarID doesn't exist in this
// campaign, and the transaction rolls back via the deferred Rollback.
func (r *calendarRepo) SetDefault(ctx context.Context, campaignID, calendarID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE calendars SET is_default = 0 WHERE campaign_id = ?`, campaignID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE calendars SET is_default = 1 WHERE id = ? AND campaign_id = ?`, calendarID, campaignID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return apperror.NewNotFound("calendar not found in campaign")
	}
	return tx.Commit()
}

// Update modifies an existing calendar's settings, current date/time and
// switches. Every mutable calendar-row column is written here; there is no
// narrower per-field writer, so the caller (a future service) is responsible
// for loading the current row, applying a partial-update's changes onto it,
// and passing the merged result, the same shape UpdateEvent already uses.
func (r *calendarRepo) Update(ctx context.Context, cal *Calendar) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendars SET name = ?, description = ?, epoch_name = ?, mode = ?,
		        current_year = ?, current_month = ?, current_day = ?,
		        hours_per_day = ?, minutes_per_hour = ?, seconds_per_minute = ?,
		        current_hour = ?, current_minute = ?,
		        leap_year_every = ?, leap_year_offset = ?,
		        hemisphere = ?, forecasts_enabled = ?, month_starts_new_week = ?,
		        visibility = ?, visibility_rules = ?,
		        tracks_real_time = ?, real_time_zone = ?,
		        anchor_year = ?, anchor_month = ?, anchor_day = ?, anchor_real_date = ?
		 WHERE id = ?`,
		cal.Name, cal.Description, cal.EpochName, cal.Mode,
		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay,
		cal.HoursPerDay, cal.MinutesPerHour, cal.SecondsPerMinute,
		cal.CurrentHour, cal.CurrentMinute,
		cal.LeapYearEvery, cal.LeapYearOffset,
		cal.Hemisphere, cal.ForecastsEnabled, cal.MonthStartsNewWeek,
		cal.Visibility, cal.VisibilityRules,
		cal.TracksRealTime, cal.RealTimeZone,
		cal.AnchorYear, cal.AnchorMonth, cal.AnchorDay, cal.AnchorRealDate,
		cal.ID,
	)
	return err
}

// Delete removes a calendar and all its structural sub-resources (cascaded
// by FK: months, weekdays, moons, seasons, eras -- and, through eras,
// entity_era_links; through events, entity_event_links; weather).
func (r *calendarRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM calendars WHERE id = ?`, id)
	return err
}

// UpdateVisibility sets the per-calendar visibility + rules. Bulk-replace:
// visibility_rules is written wholesale, mirroring
// EventRepository.UpdateEventVisibility.
func (r *calendarRepo) UpdateVisibility(ctx context.Context, calendarID string, visibility string, visRules *string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendars SET visibility = ?, visibility_rules = ? WHERE id = ?`,
		visibility, visRules, calendarID,
	)
	return err
}

// --- Months ---

// monthCols is the column list for month queries; scanMonth reads it
// positionally, see calendarCols' doc comment for why appends are safe and
// insertions are not.
const monthCols = `id, calendar_id, name, days, sort_order, is_intercalary, leap_year_days`

func scanMonth(scanner interface{ Scan(...any) error }) (*Month, error) {
	var m Month
	err := scanner.Scan(&m.ID, &m.CalendarID, &m.Name, &m.Days, &m.SortOrder, &m.IsIntercalary, &m.LeapYearDays)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// SetMonths replaces all months for a calendar (delete + bulk insert).
func (r *calendarRepo) SetMonths(ctx context.Context, calendarID string, months []MonthInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_months WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, m := range months {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_months (calendar_id, name, days, sort_order, is_intercalary, leap_year_days)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			calendarID, m.Name, m.Days, m.SortOrder, m.IsIntercalary, m.LeapYearDays,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetMonths returns all months for a calendar ordered by sort_order.
func (r *calendarRepo) GetMonths(ctx context.Context, calendarID string) ([]Month, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+monthCols+` FROM calendar_months WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var months []Month
	for rows.Next() {
		m, err := scanMonth(rows)
		if err != nil {
			return nil, err
		}
		months = append(months, *m)
	}
	return months, rows.Err()
}

// --- Weekdays ---

// weekdayCols is the column list for weekday queries; scanWeekday reads it
// positionally, see calendarCols' doc comment for why appends are safe and
// insertions are not.
const weekdayCols = `id, calendar_id, name, sort_order, is_rest_day`

func scanWeekday(scanner interface{ Scan(...any) error }) (*Weekday, error) {
	var w Weekday
	err := scanner.Scan(&w.ID, &w.CalendarID, &w.Name, &w.SortOrder, &w.IsRestDay)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// SetWeekdays replaces all weekdays for a calendar.
func (r *calendarRepo) SetWeekdays(ctx context.Context, calendarID string, weekdays []WeekdayInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_weekdays WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, w := range weekdays {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_weekdays (calendar_id, name, sort_order, is_rest_day)
			 VALUES (?, ?, ?, ?)`,
			calendarID, w.Name, w.SortOrder, w.IsRestDay,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetWeekdays returns all weekdays for a calendar ordered by sort_order.
func (r *calendarRepo) GetWeekdays(ctx context.Context, calendarID string) ([]Weekday, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+weekdayCols+` FROM calendar_weekdays WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var weekdays []Weekday
	for rows.Next() {
		w, err := scanWeekday(rows)
		if err != nil {
			return nil, err
		}
		weekdays = append(weekdays, *w)
	}
	return weekdays, rows.Err()
}

// --- Shared upsert-by-id plumbing (moons, eras) ---
//
// SetMoons/SetEras and ApplyImport all need the same shape: replace a
// calendar's child rows from a caller-supplied list while an id shared
// between the existing rows and the input is updated in place rather than
// deleted and reinserted, so anything keyed to that row's id (a hidden
// flag, an entity link) survives a re-save. dbExecutor lets the same
// upsert code run inside SetMoons/SetEras' own transaction or inside
// ApplyImport's shared one.

// dbExecutor is the subset of *sql.DB and *sql.Tx the upsert helpers need.
type dbExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// existingIDs returns the ids currently in table for calendarID. table is
// always a package-controlled literal (never caller input), so building the
// query string with it carries no injection risk.
func existingIDs(ctx context.Context, ex dbExecutor, table, calendarID string) (map[int]bool, error) {
	rows, err := ex.QueryContext(ctx, `SELECT id FROM `+table+` WHERE calendar_id = ?`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// deleteUnkept removes every id in existing that the upsert didn't keep,
// scoped to calendarID so it can never reach another calendar's rows.
func deleteUnkept(ctx context.Context, ex dbExecutor, table, calendarID string, existing, keep map[int]bool) error {
	for id := range existing {
		if keep[id] {
			continue
		}
		if _, err := ex.ExecContext(ctx,
			`DELETE FROM `+table+` WHERE id = ? AND calendar_id = ?`, id, calendarID,
		); err != nil {
			return err
		}
	}
	return nil
}

// --- Moons ---

// moonCols is the column list for moon queries. scanMoon reads it
// positionally; see calendarCols' doc comment for why appends are safe and
// insertions are not.
const moonCols = `id, calendar_id, name, cycle_days, phase_offset, color,
        base_design, tint, phase_source, size, orbit_speed, hidden_from_players`

func scanMoon(scanner interface{ Scan(...any) error }) (*Moon, error) {
	var m Moon
	err := scanner.Scan(&m.ID, &m.CalendarID, &m.Name, &m.CycleDays, &m.PhaseOffset, &m.Color,
		&m.BaseDesign, &m.Tint, &m.PhaseSource, &m.Size, &m.OrbitSpeed, &m.HiddenFromPlayers)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// upsertMoons applies a MoonInput list against calendar_moons by id: a
// matched id is updated in place, never touching HiddenFromPlayers or the
// render params (this input carries no render params at all); an id absent
// from the input is deleted; an entry with no id, or an id that isn't
// actually in this calendar, is inserted fresh with HiddenFromPlayers from
// the input and the column defaults for the render params.
// moonBaseDesignOrDefault, moonPhaseSourceOrDefault, moonSizeOrDefault and
// moonOrbitSpeedOrDefault mirror calendar_moons' own column DEFAULTs
// (migrations/020_calv5_schema.up.sql) exactly. A plain INSERT with a Go
// zero value ("" / 0) would write that zero literally rather than letting
// MySQL's DEFAULT apply — it only applies when the column is omitted from
// the statement, not when it's given an explicit empty/zero — so every
// MoonInput builder that predates the render params (#805; every import
// format, the wizard's build step) would otherwise insert an invalid empty
// design/phase-source and a zero size, instead of the sensible look a
// hand-created moon gets. Tint has no default (NULL is a normal "no tint"
// value) and needs no such fallback.
func moonBaseDesignOrDefault(v string) string {
	if v == "" {
		return "moon-realistic-selene"
	}
	return v
}

func moonPhaseSourceOrDefault(v string) string {
	if v == "" {
		return "css-clip"
	}
	return v
}

func moonSizeOrDefault(v float64) float64 {
	if v == 0 {
		return 1
	}
	return v
}

func moonOrbitSpeedOrDefault(v float64) float64 {
	if v == 0 {
		return 1
	}
	return v
}

func upsertMoons(ctx context.Context, ex dbExecutor, calendarID string, moons []MoonInput) error {
	existing, err := existingIDs(ctx, ex, "calendar_moons", calendarID)
	if err != nil {
		return err
	}

	keep := make(map[int]bool, len(moons))
	for _, m := range moons {
		if m.ID != nil && existing[*m.ID] {
			if _, err := ex.ExecContext(ctx,
				`UPDATE calendar_moons SET name = ?, cycle_days = ?, phase_offset = ?, color = ?
				 WHERE id = ? AND calendar_id = ?`,
				m.Name, m.CycleDays, m.PhaseOffset, m.Color, *m.ID, calendarID,
			); err != nil {
				return err
			}
			keep[*m.ID] = true
			continue
		}
		res, err := ex.ExecContext(ctx,
			`INSERT INTO calendar_moons (calendar_id, name, cycle_days, phase_offset, color, hidden_from_players,
			        base_design, tint, phase_source, size, orbit_speed)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			calendarID, m.Name, m.CycleDays, m.PhaseOffset, m.Color, m.HiddenFromPlayers,
			moonBaseDesignOrDefault(m.BaseDesign), m.Tint, moonPhaseSourceOrDefault(m.PhaseSource),
			moonSizeOrDefault(m.Size), moonOrbitSpeedOrDefault(m.OrbitSpeed),
		)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		keep[int(id)] = true
	}
	return deleteUnkept(ctx, ex, "calendar_moons", calendarID, existing, keep)
}

// SetMoons upserts a calendar's moons; see upsertMoons for the exact
// per-row rule.
func (r *calendarRepo) SetMoons(ctx context.Context, calendarID string, moons []MoonInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := upsertMoons(ctx, tx, calendarID, moons); err != nil {
		return err
	}
	return tx.Commit()
}

// GetMoons returns all moons for a calendar, including the render params and
// HiddenFromPlayers. A read path serving players (rather than the GM) must
// filter out hidden moons itself; this method returns every moon regardless
// of caller.
func (r *calendarRepo) GetMoons(ctx context.Context, calendarID string) ([]Moon, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+moonCols+` FROM calendar_moons WHERE calendar_id = ?`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var moons []Moon
	for rows.Next() {
		m, err := scanMoon(rows)
		if err != nil {
			return nil, err
		}
		moons = append(moons, *m)
	}
	return moons, rows.Err()
}

// SetMoonHidden toggles a single moon's visibility to players, scoped to
// calendarID. A production DSN without clientFoundRows counts changed rows,
// not matched rows, so re-setting the same value already stored reports
// zero rows too; an existence check tells that apart from a genuine
// not-found (wrong id, or an id from another calendar) before erroring.
func (r *calendarRepo) SetMoonHidden(ctx context.Context, calendarID string, moonID int, hidden bool) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE calendar_moons SET hidden_from_players = ? WHERE id = ? AND calendar_id = ?`,
		hidden, moonID, calendarID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var exists bool
		if err := r.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM calendar_moons WHERE id = ? AND calendar_id = ?)`,
			moonID, calendarID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apperror.NewNotFound("moon not found in calendar")
		}
	}
	return nil
}

// --- Seasons ---

// seasonCols is the column list for season queries; scanSeason reads it
// positionally, see calendarCols' doc comment for why appends are safe and
// insertions are not.
const seasonCols = `id, calendar_id, name, start_month, start_day, end_month, end_day, description, color, weather_effect`

func scanSeason(scanner interface{ Scan(...any) error }) (*Season, error) {
	var s Season
	err := scanner.Scan(&s.ID, &s.CalendarID, &s.Name, &s.StartMonth, &s.StartDay, &s.EndMonth, &s.EndDay,
		&s.Description, &s.Color, &s.WeatherEffect)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// SetSeasons replaces all seasons for a calendar.
func (r *calendarRepo) SetSeasons(ctx context.Context, calendarID string, seasons []Season) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_seasons WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, s := range seasons {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_seasons (calendar_id, name, start_month, start_day, end_month, end_day, description, color, weather_effect)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			calendarID, s.Name, s.StartMonth, s.StartDay, s.EndMonth, s.EndDay, s.Description, s.Color, s.WeatherEffect,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetSeasons returns all seasons for a calendar.
func (r *calendarRepo) GetSeasons(ctx context.Context, calendarID string) ([]Season, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+seasonCols+` FROM calendar_seasons WHERE calendar_id = ?`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var seasons []Season
	for rows.Next() {
		s, err := scanSeason(rows)
		if err != nil {
			return nil, err
		}
		seasons = append(seasons, *s)
	}
	return seasons, rows.Err()
}

// --- Eras ---

// eraCols is the column list for era queries.
const eraCols = `id, calendar_id, name, start_year, start_month, start_day,
        end_year, end_month, end_day, description, color, sort_order`

// eraColsQualified is eraCols qualified with the calendar_eras alias `er`,
// for a query that also joins another table sharing a column name with it
// (calendars has its own name/description too) and would otherwise get
// "ambiguous column" from the database. Same order, so eraDests still
// applies; used by ErasForEntity in entity_ties_repository.go.
const eraColsQualified = `er.id, er.calendar_id, er.name, er.start_year, er.start_month, er.start_day,
        er.end_year, er.end_month, er.end_day, er.description, er.color, er.sort_order`

// eraDests returns the Scan destination list for eraCols/eraColsQualified,
// in the same order, so scanEra and ErasForEntity's scan (which appends one
// trailing extra destination) read the exact same list and can't drift apart.
func eraDests(e *Era) []any {
	return []any{&e.ID, &e.CalendarID, &e.Name, &e.StartYear, &e.StartMonth, &e.StartDay,
		&e.EndYear, &e.EndMonth, &e.EndDay, &e.Description, &e.Color, &e.SortOrder}
}

func scanEra(scanner interface{ Scan(...any) error }) (*Era, error) {
	var e Era
	if err := scanner.Scan(eraDests(&e)...); err != nil {
		return nil, err
	}
	return &e, nil
}

// upsertEras applies an EraInput list against calendar_eras by id: a
// matched id is updated in place, preserving entity_era_links (which
// cascade-delete only when the era row itself is deleted); an id absent
// from the input is deleted; an entry with no id, or an id that isn't
// actually in this calendar, is inserted fresh.
func upsertEras(ctx context.Context, ex dbExecutor, calendarID string, eras []EraInput) error {
	existing, err := existingIDs(ctx, ex, "calendar_eras", calendarID)
	if err != nil {
		return err
	}

	keep := make(map[int]bool, len(eras))
	for _, e := range eras {
		if e.ID != nil && existing[*e.ID] {
			if _, err := ex.ExecContext(ctx,
				`UPDATE calendar_eras SET name = ?, start_year = ?, start_month = ?, start_day = ?,
				        end_year = ?, end_month = ?, end_day = ?, description = ?, color = ?, sort_order = ?
				 WHERE id = ? AND calendar_id = ?`,
				e.Name, e.StartYear, e.StartMonth, e.StartDay, e.EndYear, e.EndMonth, e.EndDay,
				e.Description, e.Color, e.SortOrder, *e.ID, calendarID,
			); err != nil {
				return err
			}
			keep[*e.ID] = true
			continue
		}
		res, err := ex.ExecContext(ctx,
			`INSERT INTO calendar_eras (calendar_id, name, start_year, start_month, start_day, end_year, end_month, end_day, description, color, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			calendarID, e.Name, e.StartYear, e.StartMonth, e.StartDay, e.EndYear, e.EndMonth, e.EndDay, e.Description, e.Color, e.SortOrder,
		)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		keep[int(id)] = true
	}
	return deleteUnkept(ctx, ex, "calendar_eras", calendarID, existing, keep)
}

// SetEras upserts a calendar's eras; see upsertEras for the exact per-row
// rule. Import formats never supply ids (EraInput.ID is always nil coming
// from an import), so ApplyImport calling this still fully replaces a
// calendar's eras whenever the import carries any at all; only a caller
// that round-trips GetEras' ids back through EraInput.ID (a future
// eras-editing UI) gets the link-preserving behavior on every field.
func (r *calendarRepo) SetEras(ctx context.Context, calendarID string, eras []EraInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := upsertEras(ctx, tx, calendarID, eras); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateEra inserts a single era and returns the persisted row (with ID).
// Auto-assigns sort_order to len(existing) when the input leaves it at zero,
// so a new era sorts after existing ones by default.
func (r *calendarRepo) CreateEra(ctx context.Context, calendarID string, input EraInput) (*Era, error) {
	if input.SortOrder == 0 {
		var maxSort sql.NullInt64
		if err := r.db.QueryRowContext(ctx,
			`SELECT MAX(sort_order) FROM calendar_eras WHERE calendar_id = ?`,
			calendarID).Scan(&maxSort); err != nil {
			return nil, err
		}
		if maxSort.Valid {
			input.SortOrder = int(maxSort.Int64) + 1
		}
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_eras (calendar_id, name, start_year, start_month, start_day, end_year, end_month, end_day, description, color, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		calendarID, input.Name, input.StartYear, input.StartMonth, input.StartDay,
		input.EndYear, input.EndMonth, input.EndDay, input.Description, input.Color, input.SortOrder,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetEraByID(ctx, int(id))
}

// UpdateEra updates a single era by ID, scoped to calendarID so a caller
// can't update an era belonging to another calendar. Every EraInput field
// is applied; there is no nil-preserve semantics, the caller supplies the
// full intended shape. Returns apperror.NewNotFound if the era doesn't
// exist in this calendar. A production DSN without clientFoundRows counts
// changed rows, not matched rows, so a save with no actual changes also
// reports zero rows; an existence check tells that apart from a genuine
// not-found before returning an error.
func (r *calendarRepo) UpdateEra(ctx context.Context, calendarID string, eraID int, input EraInput) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE calendar_eras
		   SET name = ?, start_year = ?, start_month = ?, start_day = ?,
		       end_year = ?, end_month = ?, end_day = ?,
		       description = ?, color = ?, sort_order = ?
		 WHERE id = ? AND calendar_id = ?`,
		input.Name, input.StartYear, input.StartMonth, input.StartDay,
		input.EndYear, input.EndMonth, input.EndDay,
		input.Description, input.Color, input.SortOrder, eraID, calendarID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var exists bool
		if err := r.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM calendar_eras WHERE id = ? AND calendar_id = ?)`,
			eraID, calendarID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apperror.NewNotFound("era not found in calendar")
		}
	}
	return nil
}

// DeleteEra removes a single era, scoped to calendarID, and re-sorts the
// remaining eras of the same calendar so sort_order stays contiguous
// (0..N-1), in one transaction so a partial reorder can't strand the
// calendar's eras out of order. Returns apperror.NewNotFound if the era
// doesn't exist in this calendar.
func (r *calendarRepo) DeleteEra(ctx context.Context, calendarID string, eraID int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`DELETE FROM calendar_eras WHERE id = ? AND calendar_id = ?`, eraID, calendarID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return apperror.NewNotFound("era not found in calendar")
	}

	// Re-sort remaining eras to be contiguous, emulating ROW_NUMBER() via a
	// session variable; a calendar's eras are a handful of rows, so the
	// linear-scan reorder is cheap.
	if _, err := tx.ExecContext(ctx, `SET @i := -1`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE calendar_eras SET sort_order = (@i := @i + 1)
		 WHERE calendar_id = ? ORDER BY sort_order, start_year, start_month, start_day`, calendarID); err != nil {
		return err
	}

	return tx.Commit()
}

// GetEraByID returns one era by ID, or nil if not found. Unscoped by
// calendar: a caller that got the id from GetEras (already campaign- and
// calendar-scoped) can look it up directly; a caller taking an id from an
// untrusted source must not use this to establish that the era belongs to
// a given calendar.
func (r *calendarRepo) GetEraByID(ctx context.Context, eraID int) (*Era, error) {
	e, err := scanEra(r.db.QueryRowContext(ctx, `SELECT `+eraCols+` FROM calendar_eras WHERE id = ?`, eraID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// GetEras returns all eras for a calendar ordered by sort_order.
func (r *calendarRepo) GetEras(ctx context.Context, calendarID string) ([]Era, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eraCols+` FROM calendar_eras WHERE calendar_id = ? ORDER BY sort_order, start_year, start_month, start_day`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var eras []Era
	for rows.Next() {
		e, err := scanEra(rows)
		if err != nil {
			return nil, err
		}
		eras = append(eras, *e)
	}
	return eras, rows.Err()
}

// --- Cycles ---

// cycleCols and cycleEntryCols are the column lists for cycle/cycle-entry
// queries; scanCycle/scanCycleEntry read them positionally, see
// calendarCols' doc comment for why appends are safe and insertions are not.
const cycleCols = `id, calendar_id, name, cycle_length, type, sort_order`
const cycleEntryCols = `id, cycle_id, name, icon, year_offset, sort_order`

func scanCycle(scanner interface{ Scan(...any) error }) (*Cycle, error) {
	var c Cycle
	err := scanner.Scan(&c.ID, &c.CalendarID, &c.Name, &c.CycleLength, &c.Type, &c.SortOrder)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func scanCycleEntry(scanner interface{ Scan(...any) error }) (*CycleEntry, error) {
	var e CycleEntry
	err := scanner.Scan(&e.ID, &e.CycleID, &e.Name, &e.Icon, &e.YearOffset, &e.SortOrder)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// SetCycles replaces all cycles and their entries for a calendar.
func (r *calendarRepo) SetCycles(ctx context.Context, calendarID string, cycles []CycleInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Entries cascade via FK when their cycle is deleted.
	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_cycles WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, c := range cycles {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_cycles (calendar_id, name, cycle_length, type, sort_order)
			 VALUES (?, ?, ?, ?, ?)`,
			calendarID, c.Name, c.CycleLength, c.Type, c.SortOrder)
		if err != nil {
			return err
		}
		cycleID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for _, e := range c.Entries {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO calendar_cycle_entries (cycle_id, name, icon, year_offset, sort_order)
				 VALUES (?, ?, ?, ?, ?)`,
				cycleID, e.Name, e.Icon, e.YearOffset, e.SortOrder,
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// GetCycles returns all cycles with their entries for a calendar.
func (r *calendarRepo) GetCycles(ctx context.Context, calendarID string) ([]Cycle, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cycleCols+` FROM calendar_cycles WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cycles []Cycle
	for rows.Next() {
		c, err := scanCycle(rows)
		if err != nil {
			return nil, err
		}
		cycles = append(cycles, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range cycles {
		entryRows, err := r.db.QueryContext(ctx,
			`SELECT `+cycleEntryCols+` FROM calendar_cycle_entries WHERE cycle_id = ? ORDER BY sort_order`, cycles[i].ID)
		if err != nil {
			return nil, err
		}
		for entryRows.Next() {
			e, err := scanCycleEntry(entryRows)
			if err != nil {
				entryRows.Close()
				return nil, err
			}
			cycles[i].Entries = append(cycles[i].Entries, *e)
		}
		entryRows.Close()
		if err := entryRows.Err(); err != nil {
			return nil, err
		}
	}
	return cycles, nil
}

// --- Festivals ---

// festivalCols is the column list for festival queries; scanFestival reads
// it positionally, see calendarCols' doc comment for why appends are safe
// and insertions are not.
const festivalCols = `id, calendar_id, name, month, day, after_month, description, color, icon, sort_order`

func scanFestival(scanner interface{ Scan(...any) error }) (*Festival, error) {
	var f Festival
	err := scanner.Scan(&f.ID, &f.CalendarID, &f.Name, &f.Month, &f.Day, &f.AfterMonth,
		&f.Description, &f.Color, &f.Icon, &f.SortOrder)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// SetFestivals replaces all festivals for a calendar.
func (r *calendarRepo) SetFestivals(ctx context.Context, calendarID string, festivals []FestivalInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_festivals WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, f := range festivals {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_festivals (calendar_id, name, month, day, after_month, description, color, icon, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			calendarID, f.Name, f.Month, f.Day, f.AfterMonth, f.Description, f.Color, f.Icon, f.SortOrder,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetFestivals returns all festivals for a calendar.
func (r *calendarRepo) GetFestivals(ctx context.Context, calendarID string) ([]Festival, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+festivalCols+` FROM calendar_festivals WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var festivals []Festival
	for rows.Next() {
		f, err := scanFestival(rows)
		if err != nil {
			return nil, err
		}
		festivals = append(festivals, *f)
	}
	return festivals, rows.Err()
}

// --- Import ---

// ApplyImport runs the entire calendar-import write workflow within one
// transaction. SQL is duplicated from the corresponding Set* methods (moons
// and eras share the upsert helpers instead) rather than calling them
// directly, so those methods' own transaction boundaries and tests are
// untouched by this one; a future sub-resource added to ImportResult needs
// the same treatment here.
func (r *calendarRepo) ApplyImport(ctx context.Context, cal *Calendar, result *ImportResult) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin import tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE calendars SET name = ?, description = ?, epoch_name = ?, mode = ?,
		        current_year = ?, current_month = ?, current_day = ?,
		        hours_per_day = ?, minutes_per_hour = ?, seconds_per_minute = ?,
		        current_hour = ?, current_minute = ?,
		        leap_year_every = ?, leap_year_offset = ?,
		        hemisphere = ?, forecasts_enabled = ?, month_starts_new_week = ?,
		        tracks_real_time = ?, real_time_zone = ?
		 WHERE id = ?`,
		cal.Name, cal.Description, cal.EpochName, cal.Mode,
		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay,
		cal.HoursPerDay, cal.MinutesPerHour, cal.SecondsPerMinute,
		cal.CurrentHour, cal.CurrentMinute,
		cal.LeapYearEvery, cal.LeapYearOffset,
		cal.Hemisphere, cal.ForecastsEnabled, cal.MonthStartsNewWeek,
		cal.TracksRealTime, cal.RealTimeZone, cal.ID,
	); err != nil {
		return fmt.Errorf("update calendar: %w", err)
	}

	if len(result.Months) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_months WHERE calendar_id = ?`, cal.ID); err != nil {
			return fmt.Errorf("delete months: %w", err)
		}
		for _, m := range result.Months {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO calendar_months (calendar_id, name, days, sort_order, is_intercalary, leap_year_days)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				cal.ID, m.Name, m.Days, m.SortOrder, m.IsIntercalary, m.LeapYearDays,
			); err != nil {
				return fmt.Errorf("insert month %q: %w", m.Name, err)
			}
		}
	}

	if len(result.Weekdays) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_weekdays WHERE calendar_id = ?`, cal.ID); err != nil {
			return fmt.Errorf("delete weekdays: %w", err)
		}
		for _, w := range result.Weekdays {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO calendar_weekdays (calendar_id, name, sort_order, is_rest_day)
				 VALUES (?, ?, ?, ?)`,
				cal.ID, w.Name, w.SortOrder, w.IsRestDay,
			); err != nil {
				return fmt.Errorf("insert weekday %q: %w", w.Name, err)
			}
		}
	}

	// Moons upsert by id, preserving HiddenFromPlayers and the render params
	// for any moon the import re-describes by id; an import with an empty
	// moon list still clears the calendar's moons, as it did before upsert.
	if err := upsertMoons(ctx, tx, cal.ID, result.Moons); err != nil {
		return fmt.Errorf("apply moons: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_seasons WHERE calendar_id = ?`, cal.ID); err != nil {
		return fmt.Errorf("delete seasons: %w", err)
	}
	for _, s := range result.Seasons {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_seasons (calendar_id, name, start_month, start_day, end_month, end_day, description, color, weather_effect)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			cal.ID, s.Name, s.StartMonth, s.StartDay, s.EndMonth, s.EndDay, s.Description, s.Color, s.WeatherEffect,
		); err != nil {
			return fmt.Errorf("insert season %q: %w", s.Name, err)
		}
	}

	// Eras upsert by id (no import format supplies ids today, so this still
	// inserts every era fresh when the import carries any); skip entirely
	// when the import carries none, so a format with no era concept can't
	// wipe out eras the campaign already has, matching months and weekdays
	// above.
	if len(result.Eras) > 0 {
		if err := upsertEras(ctx, tx, cal.ID, result.Eras); err != nil {
			return fmt.Errorf("apply eras: %w", err)
		}
	}

	// Cycles (with their entries) and festivals (#771): same skip-when-empty
	// rule as eras above, and the same replace-all SQL SetCycles/SetFestivals
	// use (duplicated per this method's own doc comment, not called
	// directly, so their transaction boundaries stay untouched by this one).
	if len(result.Cycles) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_cycles WHERE calendar_id = ?`, cal.ID); err != nil {
			return fmt.Errorf("delete cycles: %w", err)
		}
		for _, c := range result.Cycles {
			res, err := tx.ExecContext(ctx,
				`INSERT INTO calendar_cycles (calendar_id, name, cycle_length, type, sort_order)
				 VALUES (?, ?, ?, ?, ?)`,
				cal.ID, c.Name, c.CycleLength, c.Type, c.SortOrder)
			if err != nil {
				return fmt.Errorf("insert cycle %q: %w", c.Name, err)
			}
			cycleID, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("insert cycle %q: %w", c.Name, err)
			}
			for _, e := range c.Entries {
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO calendar_cycle_entries (cycle_id, name, icon, year_offset, sort_order)
					 VALUES (?, ?, ?, ?, ?)`,
					cycleID, e.Name, e.Icon, e.YearOffset, e.SortOrder,
				); err != nil {
					return fmt.Errorf("insert cycle %q entry %q: %w", c.Name, e.Name, err)
				}
			}
		}
	}

	if len(result.Festivals) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_festivals WHERE calendar_id = ?`, cal.ID); err != nil {
			return fmt.Errorf("delete festivals: %w", err)
		}
		for _, f := range result.Festivals {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO calendar_festivals (calendar_id, name, month, day, after_month, description, color, icon, sort_order)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				cal.ID, f.Name, f.Month, f.Day, f.AfterMonth, f.Description, f.Color, f.Icon, f.SortOrder,
			); err != nil {
				return fmt.Errorf("insert festival %q: %w", f.Name, err)
			}
		}
	}

	return tx.Commit()
}
