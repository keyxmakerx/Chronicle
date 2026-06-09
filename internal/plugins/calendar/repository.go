package calendar

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// CalendarRepository defines persistence operations for calendars and events.
type CalendarRepository interface {
	// Calendar CRUD.
	Create(ctx context.Context, cal *Calendar) error
	GetByCampaignID(ctx context.Context, campaignID string) (*Calendar, error)
	GetDefaultByCampaignID(ctx context.Context, campaignID string) (*Calendar, error)
	GetByID(ctx context.Context, id string) (*Calendar, error)
	ListByCampaignID(ctx context.Context, campaignID string) ([]Calendar, error)
	SetDefault(ctx context.Context, campaignID, calendarID string) error
	Update(ctx context.Context, cal *Calendar) error
	Delete(ctx context.Context, id string) error

	// Active-calendar pointer (V2 Wave 1 PR 1 / C-CAL-V2-SHELL-FOUNDATION).
	// Returns the user's last-selected calendar ID for the campaign, or
	// "" if none has been recorded. Service layer resolves "" → campaign
	// default. Set writes the pointer; the caller is responsible for
	// validating that calendarID belongs to campaignID.
	GetActiveCalendarID(ctx context.Context, userID, campaignID string) (string, error)
	SetActiveCalendar(ctx context.Context, userID, campaignID, calendarID string) error

	// Sidebar pin preference (V2 Wave 1.7A §G). Per-user-per-campaign
	// boolean piggybacked on the calendar_active row; defaults TRUE
	// for new rows + backfilled rows per migration 007.
	GetSidebarPinned(ctx context.Context, userID, campaignID string) (bool, error)
	SetSidebarPinned(ctx context.Context, userID, campaignID string, pinned bool) error

	// Months.
	SetMonths(ctx context.Context, calendarID string, months []MonthInput) error
	GetMonths(ctx context.Context, calendarID string) ([]Month, error)

	// Weekdays.
	SetWeekdays(ctx context.Context, calendarID string, weekdays []WeekdayInput) error
	GetWeekdays(ctx context.Context, calendarID string) ([]Weekday, error)

	// Moons.
	SetMoons(ctx context.Context, calendarID string, moons []MoonInput) error
	GetMoons(ctx context.Context, calendarID string) ([]Moon, error)

	// Seasons.
	SetSeasons(ctx context.Context, calendarID string, seasons []Season) error
	GetSeasons(ctx context.Context, calendarID string) ([]Season, error)

	// Eras.
	SetEras(ctx context.Context, calendarID string, eras []EraInput) error
	GetEras(ctx context.Context, calendarID string) ([]Era, error)
	// Per-era CRUD (V2 Wave 0 PR 2; complements bulk SetEras for AI Workspace
	// Wave 3 + UI per-card-edit use cases). All three publish
	// `structure.updated` at the service layer.
	CreateEra(ctx context.Context, calendarID string, input EraInput) (*Era, error)
	UpdateEra(ctx context.Context, eraID int, input EraInput) error
	// DeleteEra removes one era + returns the calendarID that owned it
	// so the service can publish a structure.updated event without an
	// extra round-trip. Returns "" if the era didn't exist.
	DeleteEra(ctx context.Context, eraID int) (calendarID string, err error)
	GetEraByID(ctx context.Context, eraID int) (*Era, error)

	// ApplyImport runs the entire calendar-import write workflow within
	// ONE transaction so a partial failure can't leave a calendar in a
	// mixed state (V1 tech debt addressed in V2 Wave 0 PR 2). Replaces
	// what the service-level ApplyImport used to do via 6 sequential
	// Set* calls (each opening its own transaction). The service
	// validates inputs upfront, mutates `cal` to reflect import-side
	// fields, then calls this method.
	ApplyImport(ctx context.Context, cal *Calendar, result *ImportResult) error

	// Event categories.
	SetEventCategories(ctx context.Context, calendarID string, cats []EventCategoryInput) error
	GetEventCategories(ctx context.Context, calendarID string) ([]EventCategory, error)

	// Weather.
	GetWeather(ctx context.Context, calendarID string) (*Weather, error)
	SetWeather(ctx context.Context, calendarID string, input WeatherInput) error

	// Weather zones (V2 Wave 0 PR 3 / C-CAL-WEATHER-ZONES). Zone
	// definitions are calendar-scoped; ApplyWeatherZones replaces the
	// full zone set in one transaction (delete-then-insert pattern
	// mirroring SetMonths / SetSeasons / etc.). SetActiveWeatherZone
	// updates the active-zone reference on the existing calendar_weather
	// row (columns added in migration 003; no schema change here).
	GetWeatherZones(ctx context.Context, calendarID string) ([]WeatherZone, error)
	ApplyWeatherZones(ctx context.Context, calendarID string, zones []WeatherZone) error
	SetActiveWeatherZone(ctx context.Context, calendarID string, zoneID, zoneName string) error

	// Cycles.
	SetCycles(ctx context.Context, calendarID string, cycles []CycleInput) error
	GetCycles(ctx context.Context, calendarID string) ([]Cycle, error)

	// Festivals.
	SetFestivals(ctx context.Context, calendarID string, festivals []FestivalInput) error
	GetFestivals(ctx context.Context, calendarID string) ([]Festival, error)

	// Events.
	CreateEvent(ctx context.Context, evt *Event) error
	GetEvent(ctx context.Context, id string) (*Event, error)
	UpdateEvent(ctx context.Context, evt *Event) error
	DeleteEvent(ctx context.Context, id string) error
	ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int) ([]Event, error)
	ListEventsForYear(ctx context.Context, calendarID string, year int, role int) ([]Event, error)
	ListEventsForDateRange(ctx context.Context, calendarID string, year, startMonth, startDay, endMonth, endDay int, role int) ([]Event, error)
	ListEventsForEntity(ctx context.Context, entityID string, role int) ([]Event, error)
	ListUpcomingEvents(ctx context.Context, calendarID string, year, month, day int, role int, limit int) ([]Event, error)
	SearchEvents(ctx context.Context, calendarID, query string, role int) ([]Event, error)
	// ListAllEvents returns every event for a calendar with no
	// role-based visibility filtering and no date constraint.
	// Used by the public Foundry-facing API (C-CALENDAR-ENDPOINTS):
	// that API gates by per-campaign signed token rather than user
	// role, and the wire response carries the `visibility` field
	// for the module to interpret. Sort order is calendar-stable
	// (year/month/day/start_hour/start_minute/name) so consumers
	// can rely on it for diffs.
	ListAllEvents(ctx context.Context, calendarID string) ([]Event, error)

	// Event visibility.
	UpdateEventVisibility(ctx context.Context, eventID string, visibility string, visRules *string) error

	// Entity ties (migration 009 / C-CAL-ENTITY-TIES-DATA-MODEL). Cascade
	// on entity/event/era delete is DB-enforced (ON DELETE CASCADE), so
	// there is no unlink-all method. Implementations in
	// entity_ties_repository.go.
	LinkEntityEvent(ctx context.Context, entityID, eventID, role string) error
	UnlinkEntityEvent(ctx context.Context, entityID, eventID string) error
	LinkEntityEra(ctx context.Context, entityID string, eraID int, role *string) error
	UnlinkEntityEra(ctx context.Context, entityID string, eraID int) error
	EntitiesForEvent(ctx context.Context, eventID string) ([]EntityTieRef, error)
	EntitiesForEra(ctx context.Context, eraID int) ([]EntityTieRef, error)
	EntitiesForCalendar(ctx context.Context, calendarID string) ([]EntityTieRef, error)
	EventsForEntity(ctx context.Context, entityID string) ([]EntityEventTie, error)
	ErasForEntity(ctx context.Context, entityID string) ([]EntityEraTie, error)
	// World-state model (migration 008 / C-CAL-WORLDSTATE-SERVER-MODEL).
	// All reads are scoped to a single date (year/month/day) except
	// GetMoonPhasesForCalendar which loads the named-phase vocab for every
	// moon of a calendar in one query. Implementations live in
	// worldstate_repository.go.
	GetDayWeather(ctx context.Context, calendarID string, year, month, day int) (*DayWeather, error)
	SetDayWeather(ctx context.Context, calendarID string, year, month, day int, weatherType string) error
	GetCelestialEvents(ctx context.Context, calendarID string, year, month, day int) ([]CelestialEvent, error)
	AddCelestialEvent(ctx context.Context, ce CelestialEvent) error
	GetMoonPhasesForCalendar(ctx context.Context, calendarID string) (map[int][]MoonPhaseVocab, error)
	GetSpecialDays(ctx context.Context, calendarID string, year, month, day int) ([]SpecialDay, error)
	SetMoodTint(ctx context.Context, calendarID string, color *string, intensity *float64) error
}

// calendarRepo is the MariaDB implementation of CalendarRepository.
type calendarRepo struct {
	db *sql.DB
}

// NewCalendarRepository creates a new MariaDB-backed calendar repository.
func NewCalendarRepository(db *sql.DB) CalendarRepository {
	return &calendarRepo{db: db}
}

// calendarCols is the column list for calendar queries. mood_tint_* are the
// persisted live-mood columns added in migration 008
// (C-CAL-WORLDSTATE-SERVER-MODEL); appended last so the column order of the
// pre-008 prefix is unchanged.
const calendarCols = `id, campaign_id, mode, name, description, epoch_name, current_year,
        current_month, current_day, hours_per_day, minutes_per_hour, seconds_per_minute,
        current_hour, current_minute, leap_year_every, leap_year_offset,
        sort_order, is_default, created_at, updated_at,
        mood_tint_color, mood_tint_intensity, visibility, visibility_rules`

// scanCalendar reads a row into a Calendar struct.
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
		&cal.MoodTintColor, &cal.MoodTintIntensity,
		&cal.Visibility, &cal.VisibilityRules)
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
		        sort_order, is_default)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cal.ID, cal.CampaignID, cal.Mode, cal.Name, cal.Description, cal.EpochName,
		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay,
		cal.HoursPerDay, cal.MinutesPerHour, cal.SecondsPerMinute,
		cal.CurrentHour, cal.CurrentMinute,
		cal.LeapYearEvery, cal.LeapYearOffset,
		cal.SortOrder, cal.IsDefault,
	)
	return err
}

// GetByCampaignID returns the first calendar for a campaign (backward compat).
// Prefers the default calendar; falls back to oldest if none is marked default.
func (r *calendarRepo) GetByCampaignID(ctx context.Context, campaignID string) (*Calendar, error) {
	return scanCalendar(r.db.QueryRowContext(ctx,
		`SELECT `+calendarCols+` FROM calendars WHERE campaign_id = ? ORDER BY is_default DESC, sort_order ASC LIMIT 1`, campaignID))
}

// GetDefaultByCampaignID returns the default calendar for a campaign.
func (r *calendarRepo) GetDefaultByCampaignID(ctx context.Context, campaignID string) (*Calendar, error) {
	return scanCalendar(r.db.QueryRowContext(ctx,
		`SELECT `+calendarCols+` FROM calendars WHERE campaign_id = ? AND is_default = 1`, campaignID))
}

// GetByID returns a calendar by its ID.
func (r *calendarRepo) GetByID(ctx context.Context, id string) (*Calendar, error) {
	return scanCalendar(r.db.QueryRowContext(ctx,
		`SELECT `+calendarCols+` FROM calendars WHERE id = ?`, id))
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
// the default flag on all other calendars in the same campaign.
func (r *calendarRepo) SetDefault(ctx context.Context, campaignID, calendarID string) error {
	// Unset all defaults in campaign.
	if _, err := r.db.ExecContext(ctx,
		`UPDATE calendars SET is_default = 0 WHERE campaign_id = ?`, campaignID); err != nil {
		return err
	}
	// Set the chosen one.
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendars SET is_default = 1 WHERE id = ? AND campaign_id = ?`, calendarID, campaignID)
	return err
}

// GetActiveCalendarID returns the user's last-selected calendar ID
// for a campaign, or "" if no row exists yet. Service layer falls
// back to the campaign's default calendar when this returns "".
func (r *calendarRepo) GetActiveCalendarID(ctx context.Context, userID, campaignID string) (string, error) {
	var calendarID string
	err := r.db.QueryRowContext(ctx,
		`SELECT calendar_id FROM calendar_active WHERE user_id = ? AND campaign_id = ?`,
		userID, campaignID).Scan(&calendarID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return calendarID, nil
}

// GetSidebarPinned returns the user-per-campaign sidebar pin
// preference. Defaults to TRUE when no row exists (matches the
// viewport-default pin behavior; operators on narrow viewports
// dismiss via toggle which writes FALSE).
func (r *calendarRepo) GetSidebarPinned(ctx context.Context, userID, campaignID string) (bool, error) {
	var pinned bool
	err := r.db.QueryRowContext(ctx,
		`SELECT sidebar_pinned FROM calendar_active WHERE user_id = ? AND campaign_id = ?`,
		userID, campaignID).Scan(&pinned)
	if err == sql.ErrNoRows {
		// No active-cal row yet → default to pinned per migration 007 default.
		return true, nil
	}
	if err != nil {
		return true, err
	}
	return pinned, nil
}

// SetSidebarPinned writes the pin preference. Upserts via the same
// calendar_active row used for active-cal pointers; preserves any
// existing calendar_id reference (empty string fallback on first
// write so a user who toggles pin before ever selecting a calendar
// still gets a row).
func (r *calendarRepo) SetSidebarPinned(ctx context.Context, userID, campaignID string, pinned bool) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_active (user_id, campaign_id, calendar_id, sidebar_pinned)
		 VALUES (?, ?, '', ?)
		 ON DUPLICATE KEY UPDATE sidebar_pinned = VALUES(sidebar_pinned)`,
		userID, campaignID, pinned)
	return err
}

// SetActiveCalendar writes the user-per-campaign active-calendar
// pointer. Upserts so the first switch creates the row and subsequent
// switches overwrite. Caller must validate calendarID belongs to
// campaignID before calling.
func (r *calendarRepo) SetActiveCalendar(ctx context.Context, userID, campaignID, calendarID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_active (user_id, campaign_id, calendar_id)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE calendar_id = VALUES(calendar_id)`,
		userID, campaignID, calendarID)
	return err
}

// Update modifies an existing calendar's settings and current date/time.
func (r *calendarRepo) Update(ctx context.Context, cal *Calendar) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendars SET name = ?, description = ?, epoch_name = ?,
		        current_year = ?, current_month = ?, current_day = ?,
		        hours_per_day = ?, minutes_per_hour = ?, seconds_per_minute = ?,
		        current_hour = ?, current_minute = ?,
		        leap_year_every = ?, leap_year_offset = ?
		 WHERE id = ?`,
		cal.Name, cal.Description, cal.EpochName,
		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay,
		cal.HoursPerDay, cal.MinutesPerHour, cal.SecondsPerMinute,
		cal.CurrentHour, cal.CurrentMinute,
		cal.LeapYearEvery, cal.LeapYearOffset, cal.ID,
	)
	return err
}

// Delete removes a calendar and all child records (cascaded by FK).
func (r *calendarRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM calendars WHERE id = ?`, id)
	return err
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
		`SELECT id, calendar_id, name, days, sort_order, is_intercalary, leap_year_days
		 FROM calendar_months WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var months []Month
	for rows.Next() {
		var m Month
		if err := rows.Scan(&m.ID, &m.CalendarID, &m.Name, &m.Days, &m.SortOrder, &m.IsIntercalary, &m.LeapYearDays); err != nil {
			return nil, err
		}
		months = append(months, m)
	}
	return months, rows.Err()
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
		`SELECT id, calendar_id, name, sort_order, is_rest_day
		 FROM calendar_weekdays WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var weekdays []Weekday
	for rows.Next() {
		var w Weekday
		if err := rows.Scan(&w.ID, &w.CalendarID, &w.Name, &w.SortOrder, &w.IsRestDay); err != nil {
			return nil, err
		}
		weekdays = append(weekdays, w)
	}
	return weekdays, rows.Err()
}

// SetMoons replaces all moons for a calendar.
func (r *calendarRepo) SetMoons(ctx context.Context, calendarID string, moons []MoonInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_moons WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, m := range moons {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_moons (calendar_id, name, cycle_days, phase_offset, color)
			 VALUES (?, ?, ?, ?, ?)`,
			calendarID, m.Name, m.CycleDays, m.PhaseOffset, m.Color,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetMoons returns all moons for a calendar, including the moon-library
// render params added in migration 008 (base_design/tint/phase_source/size/
// orbit_speed). Existing moons read the column defaults.
func (r *calendarRepo) GetMoons(ctx context.Context, calendarID string) ([]Moon, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, name, cycle_days, phase_offset, color,
		        base_design, tint, phase_source, size, orbit_speed
		 FROM calendar_moons WHERE calendar_id = ?`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var moons []Moon
	for rows.Next() {
		var m Moon
		if err := rows.Scan(&m.ID, &m.CalendarID, &m.Name, &m.CycleDays, &m.PhaseOffset, &m.Color,
			&m.BaseDesign, &m.Tint, &m.PhaseSource, &m.Size, &m.OrbitSpeed); err != nil {
			return nil, err
		}
		moons = append(moons, m)
	}
	return moons, rows.Err()
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
		`SELECT id, calendar_id, name, start_month, start_day, end_month, end_day, description, color, weather_effect
		 FROM calendar_seasons WHERE calendar_id = ?`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var seasons []Season
	for rows.Next() {
		var s Season
		if err := rows.Scan(&s.ID, &s.CalendarID, &s.Name, &s.StartMonth, &s.StartDay, &s.EndMonth, &s.EndDay, &s.Description, &s.Color, &s.WeatherEffect); err != nil {
			return nil, err
		}
		seasons = append(seasons, s)
	}
	return seasons, rows.Err()
}

// SetEras replaces all eras for a calendar.
func (r *calendarRepo) SetEras(ctx context.Context, calendarID string, eras []EraInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_eras WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, e := range eras {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_eras (calendar_id, name, start_year, end_year, description, color, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			calendarID, e.Name, e.StartYear, e.EndYear, e.Description, e.Color, e.SortOrder,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CreateEra inserts a single era and returns the persisted row (with ID).
// Auto-assigns sort_order to len(existing) so the new era sorts after
// existing ones. Per-era CRUD complements SetEras (replace-all); both
// shapes coexist.
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
		`INSERT INTO calendar_eras (calendar_id, name, start_year, end_year, description, color, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		calendarID, input.Name, input.StartYear, input.EndYear, input.Description, input.Color, input.SortOrder,
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

// UpdateEra updates a single era by ID. All fields from EraInput are
// applied (no nil-preserve semantics — caller provides the full intended
// shape). Returns sql.ErrNoRows-equivalent (apperror.NotFound) if the
// era doesn't exist; the service layer maps.
func (r *calendarRepo) UpdateEra(ctx context.Context, eraID int, input EraInput) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE calendar_eras
		   SET name = ?, start_year = ?, end_year = ?, description = ?, color = ?, sort_order = ?
		 WHERE id = ?`,
		input.Name, input.StartYear, input.EndYear, input.Description, input.Color, input.SortOrder, eraID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteEra removes a single era + re-sorts remaining eras within the
// same calendar so sort_order stays contiguous (0..N-1). Done in one
// transaction so a partial reorder can't strand the calendar's eras
// in an inconsistent sort order.
func (r *calendarRepo) DeleteEra(ctx context.Context, eraID int) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var calendarID string
	if err := tx.QueryRowContext(ctx,
		`SELECT calendar_id FROM calendar_eras WHERE id = ?`, eraID,
	).Scan(&calendarID); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM calendar_eras WHERE id = ?`, eraID); err != nil {
		return "", err
	}

	// Re-sort remaining eras to be contiguous. SET sort_order = (
	// row_number() over (...) - 1) — emulated in MariaDB via a session
	// variable. Eras within a calendar are typically a small set
	// (handfuls), so the linear-scan reorder is cheap.
	if _, err := tx.ExecContext(ctx, `SET @i := -1`); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE calendar_eras SET sort_order = (@i := @i + 1)
		 WHERE calendar_id = ? ORDER BY sort_order, start_year`, calendarID); err != nil {
		return "", err
	}

	return calendarID, tx.Commit()
}

// GetEraByID returns one era by ID, or nil if not found.
func (r *calendarRepo) GetEraByID(ctx context.Context, eraID int) (*Era, error) {
	var e Era
	err := r.db.QueryRowContext(ctx,
		`SELECT id, calendar_id, name, start_year, end_year, description, color, sort_order
		 FROM calendar_eras WHERE id = ?`, eraID,
	).Scan(&e.ID, &e.CalendarID, &e.Name, &e.StartYear, &e.EndYear, &e.Description, &e.Color, &e.SortOrder)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ApplyImport runs the entire calendar-import write workflow within one
// transaction. The caller (service layer) is responsible for validating
// inputs BEFORE this method is called — validation here is impossible
// because the tx is already open; rolling back due to validation defeats
// the point of upfront validation.
//
// Pre-Wave-0-PR-2 service.ApplyImport called s.SetMonths / SetWeekdays /
// SetMoons / SetSeasons / SetEras sequentially; each of those opened its
// own transaction; a failure midway through left the calendar with
// (some) replaced months + (untouched) old weekdays + etc. — partial
// state operators couldn't recover from cleanly. This method takes a
// single tx so on any error the whole import rolls back and the
// calendar stays at its pre-import state.
//
// SQL is duplicated from the corresponding Set* methods rather than
// extracted into shared helpers, keeping the existing Set* call sites
// untouched (zero risk to their tests + WS publish semantics). When
// Wave 3 AI Workspace lands an event-import path, the same pattern can
// extend with `applyEventsTx` etc. without touching Set*.
func (r *calendarRepo) ApplyImport(ctx context.Context, cal *Calendar, result *ImportResult) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin import tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Calendar fields (UPDATE).
	if _, err := tx.ExecContext(ctx,
		`UPDATE calendars SET name = ?, description = ?, epoch_name = ?,
		        current_year = ?, current_month = ?, current_day = ?,
		        hours_per_day = ?, minutes_per_hour = ?, seconds_per_minute = ?,
		        current_hour = ?, current_minute = ?,
		        leap_year_every = ?, leap_year_offset = ?
		 WHERE id = ?`,
		cal.Name, cal.Description, cal.EpochName,
		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay,
		cal.HoursPerDay, cal.MinutesPerHour, cal.SecondsPerMinute,
		cal.CurrentHour, cal.CurrentMinute,
		cal.LeapYearEvery, cal.LeapYearOffset, cal.ID,
	); err != nil {
		return fmt.Errorf("update calendar: %w", err)
	}

	// 2. Months (optional — V1 ApplyImport skipped if empty; we do too).
	if len(result.Months) > 0 {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM calendar_months WHERE calendar_id = ?`, cal.ID); err != nil {
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

	// 3. Weekdays (optional).
	if len(result.Weekdays) > 0 {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM calendar_weekdays WHERE calendar_id = ?`, cal.ID); err != nil {
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

	// 4. Moons (always replace; result.Moons may be empty to clear).
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM calendar_moons WHERE calendar_id = ?`, cal.ID); err != nil {
		return fmt.Errorf("delete moons: %w", err)
	}
	for _, m := range result.Moons {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_moons (calendar_id, name, cycle_days, phase_offset, color)
			 VALUES (?, ?, ?, ?, ?)`,
			cal.ID, m.Name, m.CycleDays, m.PhaseOffset, m.Color,
		); err != nil {
			return fmt.Errorf("insert moon %q: %w", m.Name, err)
		}
	}

	// 5. Seasons.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM calendar_seasons WHERE calendar_id = ?`, cal.ID); err != nil {
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

	// 6. Eras.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM calendar_eras WHERE calendar_id = ?`, cal.ID); err != nil {
		return fmt.Errorf("delete eras: %w", err)
	}
	for _, e := range result.Eras {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_eras (calendar_id, name, start_year, end_year, description, color, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			cal.ID, e.Name, e.StartYear, e.EndYear, e.Description, e.Color, e.SortOrder,
		); err != nil {
			return fmt.Errorf("insert era %q: %w", e.Name, err)
		}
	}

	return tx.Commit()
}

// GetEras returns all eras for a calendar ordered by sort_order.
func (r *calendarRepo) GetEras(ctx context.Context, calendarID string) ([]Era, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, name, start_year, end_year, description, color, sort_order
		 FROM calendar_eras WHERE calendar_id = ? ORDER BY sort_order, start_year`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var eras []Era
	for rows.Next() {
		var e Era
		if err := rows.Scan(&e.ID, &e.CalendarID, &e.Name, &e.StartYear, &e.EndYear, &e.Description, &e.Color, &e.SortOrder); err != nil {
			return nil, err
		}
		eras = append(eras, e)
	}
	return eras, rows.Err()
}

// SetEventCategories replaces all event categories for a calendar (delete + bulk insert).
func (r *calendarRepo) SetEventCategories(ctx context.Context, calendarID string, cats []EventCategoryInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_event_categories WHERE calendar_id = ?`, calendarID); err != nil {
		return err
	}
	for _, c := range cats {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_event_categories (calendar_id, slug, name, icon, color, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			calendarID, c.Slug, c.Name, c.Icon, c.Color, c.SortOrder,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetEventCategories returns all event categories for a calendar ordered by sort_order.
func (r *calendarRepo) GetEventCategories(ctx context.Context, calendarID string) ([]EventCategory, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, slug, name, icon, color, sort_order
		 FROM calendar_event_categories WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []EventCategory
	for rows.Next() {
		var c EventCategory
		if err := rows.Scan(&c.ID, &c.CalendarID, &c.Slug, &c.Name, &c.Icon, &c.Color, &c.SortOrder); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// eventCols is the column list for event queries (with entity join fields).
// Wave 1.6 added e.tier between e.category and e.color to close the
// PR #358 schema-only gap surfaced by PR #368 stop-and-flag #2.
const eventCols = `e.id, e.calendar_id, e.entity_id, e.name, e.description, e.description_html,
       e.year, e.month, e.day, e.start_hour, e.start_minute,
       e.end_year, e.end_month, e.end_day, e.end_hour, e.end_minute,
       e.is_recurring, e.recurrence_type,
       e.recurrence_interval, e.recurrence_end_year, e.recurrence_end_month,
       e.recurrence_end_day, e.recurrence_max_occurrences,
       e.visibility, e.visibility_rules, e.category, e.tier,
       e.color, e.icon, e.all_day,
       e.created_by, e.created_at, e.updated_at,
       COALESCE(ent.name, ''), COALESCE(et.icon, ''), COALESCE(et.color, '')`

// eventJoins is the LEFT JOIN clause for entity display data.
const eventJoins = `LEFT JOIN entities ent ON ent.id = e.entity_id
     LEFT JOIN entity_types et ON et.id = ent.entity_type_id`

// CreateEvent inserts a new event.
func (r *calendarRepo) CreateEvent(ctx context.Context, evt *Event) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_events (id, calendar_id, entity_id, name, description, description_html,
		        year, month, day, start_hour, start_minute,
		        end_year, end_month, end_day, end_hour, end_minute,
		        is_recurring, recurrence_type,
		        recurrence_interval, recurrence_end_year, recurrence_end_month,
		        recurrence_end_day, recurrence_max_occurrences,
		        visibility, visibility_rules, category, tier,
		        color, icon, all_day, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		evt.ID, evt.CalendarID, evt.EntityID, evt.Name, evt.Description, evt.DescriptionHTML,
		evt.Year, evt.Month, evt.Day, evt.StartHour, evt.StartMinute,
		evt.EndYear, evt.EndMonth, evt.EndDay, evt.EndHour, evt.EndMinute,
		evt.IsRecurring, evt.RecurrenceType,
		evt.RecurrenceInterval, evt.RecurrenceEndYear, evt.RecurrenceEndMonth,
		evt.RecurrenceEndDay, evt.RecurrenceMaxOccurrences,
		evt.Visibility, evt.VisibilityRules, evt.Category, evt.Tier,
		evt.Color, evt.Icon, evt.AllDay, evt.CreatedBy,
	)
	return err
}

// GetEvent returns a single event by ID.
func (r *calendarRepo) GetEvent(ctx context.Context, id string) (*Event, error) {
	evt := &Event{}
	err := r.db.QueryRowContext(ctx,
		`SELECT `+eventCols+`
		 FROM calendar_events e `+eventJoins+`
		 WHERE e.id = ?`, id,
	).Scan(&evt.ID, &evt.CalendarID, &evt.EntityID, &evt.Name, &evt.Description, &evt.DescriptionHTML,
		&evt.Year, &evt.Month, &evt.Day, &evt.StartHour, &evt.StartMinute,
		&evt.EndYear, &evt.EndMonth, &evt.EndDay, &evt.EndHour, &evt.EndMinute,
		&evt.IsRecurring, &evt.RecurrenceType,
		&evt.RecurrenceInterval, &evt.RecurrenceEndYear, &evt.RecurrenceEndMonth,
		&evt.RecurrenceEndDay, &evt.RecurrenceMaxOccurrences,
		&evt.Visibility, &evt.VisibilityRules, &evt.Category, &evt.Tier,
		&evt.Color, &evt.Icon, &evt.AllDay,
		&evt.CreatedBy, &evt.CreatedAt, &evt.UpdatedAt,
		&evt.EntityName, &evt.EntityIcon, &evt.EntityColor)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return evt, err
}

// UpdateEvent modifies an existing event.
func (r *calendarRepo) UpdateEvent(ctx context.Context, evt *Event) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendar_events
		 SET name = ?, description = ?, description_html = ?, entity_id = ?,
		     year = ?, month = ?, day = ?,
		     start_hour = ?, start_minute = ?,
		     end_year = ?, end_month = ?, end_day = ?, end_hour = ?, end_minute = ?,
		     is_recurring = ?, recurrence_type = ?,
		     recurrence_interval = ?, recurrence_end_year = ?, recurrence_end_month = ?,
		     recurrence_end_day = ?, recurrence_max_occurrences = ?,
		     visibility = ?, visibility_rules = ?, category = ?, tier = ?,
		     color = ?, icon = ?, all_day = ?
		 WHERE id = ?`,
		evt.Name, evt.Description, evt.DescriptionHTML, evt.EntityID,
		evt.Year, evt.Month, evt.Day,
		evt.StartHour, evt.StartMinute,
		evt.EndYear, evt.EndMonth, evt.EndDay, evt.EndHour, evt.EndMinute,
		evt.IsRecurring, evt.RecurrenceType,
		evt.RecurrenceInterval, evt.RecurrenceEndYear, evt.RecurrenceEndMonth,
		evt.RecurrenceEndDay, evt.RecurrenceMaxOccurrences,
		evt.Visibility, evt.VisibilityRules, evt.Category, evt.Tier,
		evt.Color, evt.Icon, evt.AllDay, evt.ID,
	)
	return err
}

// DeleteEvent removes an event.
func (r *calendarRepo) DeleteEvent(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM calendar_events WHERE id = ?`, id)
	return err
}

// ListEventsForMonth returns all events for a specific month, filtered by role.
// Recurring events that match the month (any year) are included.
func (r *calendarRepo) ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int) ([]Event, error) {
	// Owners see all events including dm_only; others see only 'everyone'.
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	// Recurring events appear ONCE at their stored (year, month, day) —
	// the SQL no longer expands yearly recurring rules across other
	// years. V3 will ship unified recurring expansion; see
	// decisions/2026-05-28-cal-timeline-v2-design.md Q-V2-6 resolution.
	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ?
		  AND e.year = ? AND e.month = ?
		  %s
		ORDER BY e.day, COALESCE(e.start_hour, 99), COALESCE(e.start_minute, 99), e.name`, visFilter)

	rows, err := r.db.QueryContext(ctx, query, calendarID, year, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ListEventsForYear returns all events for a specific year, filtered by role.
func (r *calendarRepo) ListEventsForYear(ctx context.Context, calendarID string, year int, role int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	// Recurring events surface only in their stored year. V3 will ship
	// unified recurring expansion; see Q-V2-6 resolution.
	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ?
		  AND e.year = ?
		  %s
		ORDER BY e.month, e.day, COALESCE(e.start_hour, 99), COALESCE(e.start_minute, 99), e.name`, visFilter)

	rows, err := r.db.QueryContext(ctx, query, calendarID, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ListAllEvents returns every event for a calendar with no role
// filter and no date constraint. Public Foundry API consumer:
// see the CalendarRepository interface comment for why.
func (r *calendarRepo) ListAllEvents(ctx context.Context, calendarID string) ([]Event, error) {
	query := `
		SELECT ` + eventCols + `
		FROM calendar_events e ` + eventJoins + `
		WHERE e.calendar_id = ?
		ORDER BY e.year, e.month, e.day, COALESCE(e.start_hour, 99), COALESCE(e.start_minute, 99), e.name`

	rows, err := r.db.QueryContext(ctx, query, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ListEventsForDateRange returns events within a date range (same year).
// Handles single-month or cross-month ranges within the same year.
//
// Recurring events appear once at their stored (year, month, day); V3
// will ship unified recurring expansion. See Q-V2-6 resolution at
// decisions/2026-05-28-cal-timeline-v2-design.md.
func (r *calendarRepo) ListEventsForDateRange(ctx context.Context, calendarID string, year, startMonth, startDay, endMonth, endDay int, role int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	// Use composite date value (month*100 + day) for range comparison.
	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ?
		  AND e.year = ? AND (e.month * 100 + e.day) >= ? AND (e.month * 100 + e.day) <= ?
		  %s
		ORDER BY e.month, e.day, COALESCE(e.start_hour, 99), COALESCE(e.start_minute, 99), e.name`, visFilter)

	startVal := startMonth*100 + startDay
	endVal := endMonth*100 + endDay

	rows, err := r.db.QueryContext(ctx, query, calendarID, year, startVal, endVal)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanEvents(rows)
}

// ListEventsForEntity returns all events linked to a specific entity.
// Used for the reverse entity-event lookup on entity pages.
func (r *calendarRepo) ListEventsForEntity(ctx context.Context, entityID string, role int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.entity_id = ?
		  %s
		ORDER BY e.year, e.month, e.day`, visFilter)

	rows, err := r.db.QueryContext(ctx, query, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ListUpcomingEvents returns events on or after the given date, ordered
// chronologically.
//
// Recurring events appear once at their stored (year, month, day); they
// surface here only if the stored date is on/after the supplied cursor.
// V3 will ship unified recurring expansion; see Q-V2-6 resolution at
// decisions/2026-05-28-cal-timeline-v2-design.md.
func (r *calendarRepo) ListUpcomingEvents(ctx context.Context, calendarID string, year, month, day int, role int, limit int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ?
		  AND (
		    e.year > ? OR
		    (e.year = ? AND e.month > ?) OR
		    (e.year = ? AND e.month = ? AND e.day >= ?)
		  )
		  %s
		ORDER BY e.year, e.month, e.day, e.name
		LIMIT ?`, visFilter)

	rows, err := r.db.QueryContext(ctx, query,
		calendarID,
		year, year, month, year, month, day,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// scanEvents reads event rows into a slice.
func scanEvents(rows *sql.Rows) ([]Event, error) {
	var events []Event
	for rows.Next() {
		var evt Event
		if err := rows.Scan(
			&evt.ID, &evt.CalendarID, &evt.EntityID, &evt.Name, &evt.Description, &evt.DescriptionHTML,
			&evt.Year, &evt.Month, &evt.Day, &evt.StartHour, &evt.StartMinute,
			&evt.EndYear, &evt.EndMonth, &evt.EndDay, &evt.EndHour, &evt.EndMinute,
			&evt.IsRecurring, &evt.RecurrenceType,
			&evt.RecurrenceInterval, &evt.RecurrenceEndYear, &evt.RecurrenceEndMonth,
			&evt.RecurrenceEndDay, &evt.RecurrenceMaxOccurrences,
			&evt.Visibility, &evt.VisibilityRules, &evt.Category, &evt.Tier,
			&evt.Color, &evt.Icon, &evt.AllDay,
			&evt.CreatedBy, &evt.CreatedAt, &evt.UpdatedAt,
			&evt.EntityName, &evt.EntityIcon, &evt.EntityColor,
		); err != nil {
			return nil, err
		}
		events = append(events, evt)
	}
	return events, rows.Err()
}

// UpdateEventVisibility sets the visibility and per-user rules on an event.
func (r *calendarRepo) UpdateEventVisibility(ctx context.Context, eventID string, visibility string, visRules *string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendar_events SET visibility = ?, visibility_rules = ? WHERE id = ?`,
		visibility, visRules, eventID,
	)
	return err
}

// SearchEvents returns events matching a name query, filtered by role-based visibility.
func (r *calendarRepo) SearchEvents(ctx context.Context, calendarID, query string, role int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	q := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ? AND e.name LIKE ? %s
		ORDER BY e.name
		LIMIT 10`, visFilter)

	escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(query)
	rows, err := r.db.QueryContext(ctx, q, calendarID, "%"+escaped+"%")
	if err != nil {
		return nil, fmt.Errorf("search events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// --- Weather ---

// GetWeather returns the current weather state for a calendar, or nil if none set.
func (r *calendarRepo) GetWeather(ctx context.Context, calendarID string) (*Weather, error) {
	w := &Weather{}
	var windSpeedKPHf sql.NullFloat64
	var windSpeedTier, windDir sql.NullString
	var windDirDegi sql.NullInt32
	var precipType sql.NullString
	var precipIntensity sql.NullFloat64

	err := r.db.QueryRowContext(ctx,
		`SELECT id, calendar_id, preset_id, preset_label, icon, color,
		        temperature_celsius, wind_speed_kph, wind_speed_tier,
		        wind_direction, wind_direction_degrees,
		        precipitation_type, precipitation_intensity,
		        zone_id, zone_name, description, updated_at
		 FROM calendar_weather WHERE calendar_id = ?`, calendarID,
	).Scan(&w.ID, &w.CalendarID, &w.PresetID, &w.PresetLabel, &w.Icon, &w.Color,
		&w.TemperatureCelsius, &windSpeedKPHf, &windSpeedTier,
		&windDir, &windDirDegi,
		&precipType, &precipIntensity,
		&w.ZoneID, &w.ZoneName, &w.Description, &w.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Build Wind struct if any wind data is present.
	if windSpeedKPHf.Valid || windDir.Valid {
		wind := &Wind{}
		if windSpeedKPHf.Valid {
			v := windSpeedKPHf.Float64
			wind.SpeedKPH = &v
		}
		if windSpeedTier.Valid {
			wind.SpeedTier = &windSpeedTier.String
		}
		if windDir.Valid {
			wind.Direction = &windDir.String
		}
		if windDirDegi.Valid {
			v := int(windDirDegi.Int32)
			wind.DirectionDegrees = &v
		}
		w.Wind = wind
	}

	// Build Precipitation struct if any precipitation data is present.
	if precipType.Valid {
		p := &Precipitation{Type: &precipType.String}
		if precipIntensity.Valid {
			p.Intensity = &precipIntensity.Float64
		}
		w.Precipitation = p
	}

	return w, nil
}

// SetWeather upserts the current weather state for a calendar.
func (r *calendarRepo) SetWeather(ctx context.Context, calendarID string, input WeatherInput) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_weather (calendar_id, preset_id, preset_label, icon, color,
		        temperature_celsius, wind_speed_kph, wind_speed_tier,
		        wind_direction, wind_direction_degrees,
		        precipitation_type, precipitation_intensity,
		        zone_id, zone_name, description)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		        preset_id = VALUES(preset_id), preset_label = VALUES(preset_label),
		        icon = VALUES(icon), color = VALUES(color),
		        temperature_celsius = VALUES(temperature_celsius),
		        wind_speed_kph = VALUES(wind_speed_kph), wind_speed_tier = VALUES(wind_speed_tier),
		        wind_direction = VALUES(wind_direction), wind_direction_degrees = VALUES(wind_direction_degrees),
		        precipitation_type = VALUES(precipitation_type), precipitation_intensity = VALUES(precipitation_intensity),
		        zone_id = VALUES(zone_id), zone_name = VALUES(zone_name),
		        description = VALUES(description)`,
		calendarID, input.PresetID, input.PresetLabel, input.Icon, input.Color,
		input.TemperatureCelsius, input.WindSpeedKPH, input.WindSpeedTier,
		input.WindDirection, input.WindDirectionDeg,
		input.PrecipitationType, input.PrecipitationIntensity,
		input.ZoneID, input.ZoneName, input.Description,
	)
	return err
}

// --- Weather zones (V2 Wave 0 PR 3 / C-CAL-WEATHER-ZONES) ---

// GetWeatherZones returns all zone definitions for a calendar ordered
// by zone_id (stable for the wire-contract response).
func (r *calendarRepo) GetWeatherZones(ctx context.Context, calendarID string) ([]WeatherZone, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT calendar_id, zone_id, name, payload, created_at, updated_at
		 FROM calendar_weather_zones WHERE calendar_id = ? ORDER BY zone_id`,
		calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var zones []WeatherZone
	for rows.Next() {
		var z WeatherZone
		var payloadRaw []byte
		if err := rows.Scan(&z.CalendarID, &z.ZoneID, &z.Name, &payloadRaw, &z.CreatedAt, &z.UpdatedAt); err != nil {
			return nil, err
		}
		if len(payloadRaw) > 0 {
			if err := json.Unmarshal(payloadRaw, &z.Payload); err != nil {
				return nil, fmt.Errorf("unmarshal zone %q payload: %w", z.ZoneID, err)
			}
		}
		zones = append(zones, z)
	}
	return zones, rows.Err()
}

// ApplyWeatherZones replaces the full zone set for a calendar in one
// transaction (delete-then-insert pattern mirroring SetMonths /
// SetSeasons / etc.). Validation runs at the service layer before
// this method is called.
func (r *calendarRepo) ApplyWeatherZones(ctx context.Context, calendarID string, zones []WeatherZone) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM calendar_weather_zones WHERE calendar_id = ?`, calendarID); err != nil {
		return fmt.Errorf("delete zones: %w", err)
	}
	for _, z := range zones {
		payloadJSON, err := json.Marshal(z.Payload)
		if err != nil {
			return fmt.Errorf("marshal zone %q payload: %w", z.ZoneID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_weather_zones (calendar_id, zone_id, name, payload)
			 VALUES (?, ?, ?, ?)`,
			calendarID, z.ZoneID, z.Name, string(payloadJSON),
		); err != nil {
			return fmt.Errorf("insert zone %q: %w", z.ZoneID, err)
		}
	}
	return tx.Commit()
}

// SetActiveWeatherZone updates the active-zone reference on the
// existing calendar_weather row. zone_id + zone_name columns were
// added in migration 003; passing "" for both clears the active zone.
// Upserts so a calendar with no prior weather row still gets the
// active-zone reference recorded.
func (r *calendarRepo) SetActiveWeatherZone(ctx context.Context, calendarID, zoneID, zoneName string) error {
	var zoneIDPtr, zoneNamePtr any
	if zoneID != "" {
		zoneIDPtr = zoneID
	}
	if zoneName != "" {
		zoneNamePtr = zoneName
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_weather (calendar_id, zone_id, zone_name)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		        zone_id = VALUES(zone_id),
		        zone_name = VALUES(zone_name)`,
		calendarID, zoneIDPtr, zoneNamePtr)
	return err
}

// --- Cycles ---

// SetCycles replaces all cycles and their entries for a calendar.
func (r *calendarRepo) SetCycles(ctx context.Context, calendarID string, cycles []CycleInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete existing cycles (entries cascade via FK).
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
		`SELECT id, calendar_id, name, cycle_length, type, sort_order
		 FROM calendar_cycles WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cycles []Cycle
	for rows.Next() {
		var c Cycle
		if err := rows.Scan(&c.ID, &c.CalendarID, &c.Name, &c.CycleLength, &c.Type, &c.SortOrder); err != nil {
			return nil, err
		}
		cycles = append(cycles, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load entries for each cycle.
	for i := range cycles {
		entryRows, err := r.db.QueryContext(ctx,
			`SELECT id, cycle_id, name, icon, year_offset, sort_order
			 FROM calendar_cycle_entries WHERE cycle_id = ? ORDER BY sort_order`, cycles[i].ID)
		if err != nil {
			return nil, err
		}
		for entryRows.Next() {
			var e CycleEntry
			if err := entryRows.Scan(&e.ID, &e.CycleID, &e.Name, &e.Icon, &e.YearOffset, &e.SortOrder); err != nil {
				entryRows.Close()
				return nil, err
			}
			cycles[i].Entries = append(cycles[i].Entries, e)
		}
		entryRows.Close()
		if err := entryRows.Err(); err != nil {
			return nil, err
		}
	}
	return cycles, nil
}

// --- Festivals ---

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
		`SELECT id, calendar_id, name, month, day, after_month, description, color, icon, sort_order
		 FROM calendar_festivals WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var festivals []Festival
	for rows.Next() {
		var f Festival
		if err := rows.Scan(&f.ID, &f.CalendarID, &f.Name, &f.Month, &f.Day, &f.AfterMonth,
			&f.Description, &f.Color, &f.Icon, &f.SortOrder); err != nil {
			return nil, err
		}
		festivals = append(festivals, f)
	}
	return festivals, rows.Err()
}
