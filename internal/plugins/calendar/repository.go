package calendar

import (
	"context"
	"database/sql"
	"fmt"
)

// CalendarRepository defines persistence operations for calendars and events.
type CalendarRepository interface {
	// Calendar CRUD.
	Create(ctx context.Context, cal *Calendar) error
	GetByCampaignID(ctx context.Context, campaignID string) (*Calendar, error)
	GetByID(ctx context.Context, id string) (*Calendar, error)
	Update(ctx context.Context, cal *Calendar) error
	Delete(ctx context.Context, id string) error

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

	// Events.
	CreateEvent(ctx context.Context, evt *Event) error
	GetEvent(ctx context.Context, id string) (*Event, error)
	UpdateEvent(ctx context.Context, evt *Event) error
	DeleteEvent(ctx context.Context, id string) error
	ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int) ([]Event, error)
	ListEventsForYear(ctx context.Context, calendarID string, year int, role int) ([]Event, error)
	ListEventsForEntity(ctx context.Context, entityID string, role int) ([]Event, error)
}

// calendarRepo is the MariaDB implementation of CalendarRepository.
type calendarRepo struct {
	db *sql.DB
}

// NewCalendarRepository creates a new MariaDB-backed calendar repository.
func NewCalendarRepository(db *sql.DB) CalendarRepository {
	return &calendarRepo{db: db}
}

// calendarCols is the column list for calendar queries.
const calendarCols = `id, campaign_id, name, description, epoch_name, current_year,
        current_month, current_day, leap_year_every, leap_year_offset, created_at, updated_at`

// scanCalendar reads a row into a Calendar struct.
func scanCalendar(scanner interface{ Scan(...any) error }) (*Calendar, error) {
	cal := &Calendar{}
	err := scanner.Scan(&cal.ID, &cal.CampaignID, &cal.Name, &cal.Description, &cal.EpochName,
		&cal.CurrentYear, &cal.CurrentMonth, &cal.CurrentDay,
		&cal.LeapYearEvery, &cal.LeapYearOffset,
		&cal.CreatedAt, &cal.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return cal, err
}

// Create inserts a new calendar.
func (r *calendarRepo) Create(ctx context.Context, cal *Calendar) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendars (id, campaign_id, name, description, epoch_name,
		        current_year, current_month, current_day, leap_year_every, leap_year_offset)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cal.ID, cal.CampaignID, cal.Name, cal.Description, cal.EpochName,
		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay,
		cal.LeapYearEvery, cal.LeapYearOffset,
	)
	return err
}

// GetByCampaignID returns the calendar for a campaign (one per campaign).
func (r *calendarRepo) GetByCampaignID(ctx context.Context, campaignID string) (*Calendar, error) {
	return scanCalendar(r.db.QueryRowContext(ctx,
		`SELECT `+calendarCols+` FROM calendars WHERE campaign_id = ?`, campaignID))
}

// GetByID returns a calendar by its ID.
func (r *calendarRepo) GetByID(ctx context.Context, id string) (*Calendar, error) {
	return scanCalendar(r.db.QueryRowContext(ctx,
		`SELECT `+calendarCols+` FROM calendars WHERE id = ?`, id))
}

// Update modifies an existing calendar's settings and current date.
func (r *calendarRepo) Update(ctx context.Context, cal *Calendar) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendars SET name = ?, description = ?, epoch_name = ?,
		        current_year = ?, current_month = ?, current_day = ?,
		        leap_year_every = ?, leap_year_offset = ?
		 WHERE id = ?`,
		cal.Name, cal.Description, cal.EpochName,
		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay,
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
			`INSERT INTO calendar_weekdays (calendar_id, name, sort_order)
			 VALUES (?, ?, ?)`,
			calendarID, w.Name, w.SortOrder,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetWeekdays returns all weekdays for a calendar ordered by sort_order.
func (r *calendarRepo) GetWeekdays(ctx context.Context, calendarID string) ([]Weekday, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, name, sort_order
		 FROM calendar_weekdays WHERE calendar_id = ? ORDER BY sort_order`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var weekdays []Weekday
	for rows.Next() {
		var w Weekday
		if err := rows.Scan(&w.ID, &w.CalendarID, &w.Name, &w.SortOrder); err != nil {
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

// GetMoons returns all moons for a calendar.
func (r *calendarRepo) GetMoons(ctx context.Context, calendarID string) ([]Moon, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, name, cycle_days, phase_offset, color
		 FROM calendar_moons WHERE calendar_id = ?`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var moons []Moon
	for rows.Next() {
		var m Moon
		if err := rows.Scan(&m.ID, &m.CalendarID, &m.Name, &m.CycleDays, &m.PhaseOffset, &m.Color); err != nil {
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
			`INSERT INTO calendar_seasons (calendar_id, name, start_month, start_day, end_month, end_day, description, color)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			calendarID, s.Name, s.StartMonth, s.StartDay, s.EndMonth, s.EndDay, s.Description, s.Color,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetSeasons returns all seasons for a calendar.
func (r *calendarRepo) GetSeasons(ctx context.Context, calendarID string) ([]Season, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, calendar_id, name, start_month, start_day, end_month, end_day, description, color
		 FROM calendar_seasons WHERE calendar_id = ?`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var seasons []Season
	for rows.Next() {
		var s Season
		if err := rows.Scan(&s.ID, &s.CalendarID, &s.Name, &s.StartMonth, &s.StartDay, &s.EndMonth, &s.EndDay, &s.Description, &s.Color); err != nil {
			return nil, err
		}
		seasons = append(seasons, s)
	}
	return seasons, rows.Err()
}

// eventCols is the column list for event queries (with entity join fields).
const eventCols = `e.id, e.calendar_id, e.entity_id, e.name, e.description,
       e.year, e.month, e.day, e.end_year, e.end_month, e.end_day,
       e.is_recurring, e.recurrence_type, e.visibility, e.category,
       e.created_by, e.created_at, e.updated_at,
       COALESCE(ent.name, ''), COALESCE(et.icon, ''), COALESCE(et.color, '')`

// eventJoins is the LEFT JOIN clause for entity display data.
const eventJoins = `LEFT JOIN entities ent ON ent.id = e.entity_id
     LEFT JOIN entity_types et ON et.id = ent.entity_type_id`

// CreateEvent inserts a new event.
func (r *calendarRepo) CreateEvent(ctx context.Context, evt *Event) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_events (id, calendar_id, entity_id, name, description,
		        year, month, day, end_year, end_month, end_day,
		        is_recurring, recurrence_type, visibility, category, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		evt.ID, evt.CalendarID, evt.EntityID, evt.Name, evt.Description,
		evt.Year, evt.Month, evt.Day, evt.EndYear, evt.EndMonth, evt.EndDay,
		evt.IsRecurring, evt.RecurrenceType, evt.Visibility, evt.Category, evt.CreatedBy,
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
	).Scan(&evt.ID, &evt.CalendarID, &evt.EntityID, &evt.Name, &evt.Description,
		&evt.Year, &evt.Month, &evt.Day, &evt.EndYear, &evt.EndMonth, &evt.EndDay,
		&evt.IsRecurring, &evt.RecurrenceType, &evt.Visibility, &evt.Category,
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
		 SET name = ?, description = ?, entity_id = ?, year = ?, month = ?, day = ?,
		     end_year = ?, end_month = ?, end_day = ?,
		     is_recurring = ?, recurrence_type = ?, visibility = ?, category = ?
		 WHERE id = ?`,
		evt.Name, evt.Description, evt.EntityID, evt.Year, evt.Month, evt.Day,
		evt.EndYear, evt.EndMonth, evt.EndDay,
		evt.IsRecurring, evt.RecurrenceType, evt.Visibility, evt.Category, evt.ID,
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
	// role >= 3 (Owner) sees dm_only events; others see only 'everyone'.
	visFilter := "AND e.visibility = 'everyone'"
	if role >= 3 {
		visFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ?
		  AND ((e.year = ? AND e.month = ? AND e.is_recurring = 0)
		       OR (e.month = ? AND e.is_recurring = 1 AND e.recurrence_type = 'yearly'))
		  %s
		ORDER BY e.day, e.name`, visFilter)

	rows, err := r.db.QueryContext(ctx, query, calendarID, year, month, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ListEventsForYear returns all events for a specific year, filtered by role.
func (r *calendarRepo) ListEventsForYear(ctx context.Context, calendarID string, year int, role int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if role >= 3 {
		visFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ?
		  AND (e.year = ? OR (e.is_recurring = 1 AND e.recurrence_type = 'yearly'))
		  %s
		ORDER BY e.month, e.day, e.name`, visFilter)

	rows, err := r.db.QueryContext(ctx, query, calendarID, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ListEventsForEntity returns all events linked to a specific entity.
// Used for the reverse entity-event lookup on entity pages.
func (r *calendarRepo) ListEventsForEntity(ctx context.Context, entityID string, role int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if role >= 3 {
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

// scanEvents reads event rows into a slice.
func scanEvents(rows *sql.Rows) ([]Event, error) {
	var events []Event
	for rows.Next() {
		var evt Event
		if err := rows.Scan(
			&evt.ID, &evt.CalendarID, &evt.EntityID, &evt.Name, &evt.Description,
			&evt.Year, &evt.Month, &evt.Day, &evt.EndYear, &evt.EndMonth, &evt.EndDay,
			&evt.IsRecurring, &evt.RecurrenceType, &evt.Visibility, &evt.Category,
			&evt.CreatedBy, &evt.CreatedAt, &evt.UpdatedAt,
			&evt.EntityName, &evt.EntityIcon, &evt.EntityColor,
		); err != nil {
			return nil, err
		}
		events = append(events, evt)
	}
	return events, rows.Err()
}
