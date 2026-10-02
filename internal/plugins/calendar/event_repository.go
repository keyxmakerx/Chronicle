// Package calendar - event_repository.go persists calendar events: the
// events themselves, their date-range/search/upcoming reads, and (in
// entity_ties_repository.go, same aggregate) their optional ties to
// entities and eras.
package calendar

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// EventRepository defines persistence for calendar events and their links.
type EventRepository interface {
	CreateEvent(ctx context.Context, evt *Event) error
	GetEvent(ctx context.Context, id string) (*Event, error)
	// GetEventsByIDs batch-reads the events with the given ids that belong to
	// calendarID, in no particular order. Unknown ids and ids on another
	// calendar are simply absent. No visibility filtering: the service layer
	// applies the same per-event predicate GetEvent's callers use.
	GetEventsByIDs(ctx context.Context, calendarID string, ids []string) ([]Event, error)
	UpdateEvent(ctx context.Context, evt *Event) error
	DeleteEvent(ctx context.Context, id string) error
	ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int) ([]Event, error)
	ListEventsForYear(ctx context.Context, calendarID string, year int, role int) ([]Event, error)
	ListEventsForDateRange(ctx context.Context, calendarID string, year, startMonth, startDay, endMonth, endDay int, role int) ([]Event, error)
	ListEventsForEntity(ctx context.Context, entityID string, role int) ([]Event, error)
	ListUpcomingEvents(ctx context.Context, calendarID string, year, month, day int, role int, limit int) ([]Event, error)
	SearchEvents(ctx context.Context, calendarID, query string, role int) ([]Event, error)
	// ListAllEvents returns every event for a calendar with no role filter and
	// no date constraint: for a system caller (a public sync API, an export)
	// that gates by its own means and must apply visibility itself before any
	// of this reaches a player. Sort order is calendar-stable.
	ListAllEvents(ctx context.Context, calendarID string) ([]Event, error)
	// StrandedEventCounts reports, per calendar in a campaign, how many
	// events point at a month position the calendar no longer has. Calendars
	// with none are absent from the map, not present with a zero.
	StrandedEventCounts(ctx context.Context, campaignID string) (map[string]int, error)
	// EventDatesForCalendars batch-reads (year,month,day,name) for every
	// event in the given calendars in one query, role-filtered.
	EventDatesForCalendars(ctx context.Context, calIDs []string, role int) (map[string][]CalendarEventDate, error)

	UpdateEventVisibility(ctx context.Context, eventID string, visibility string, visRules *string) error

	// Entity ties. Cascade on entity/event/era delete is DB-enforced (ON
	// DELETE CASCADE), so there is no unlink-all.
	LinkEntityEvent(ctx context.Context, entityID, eventID, role string) error
	UnlinkEntityEvent(ctx context.Context, entityID, eventID string) error
	LinkEntityEra(ctx context.Context, entityID string, eraID int, role *string) error
	UnlinkEntityEra(ctx context.Context, entityID string, eraID int) error
	EntitiesForEvent(ctx context.Context, eventID string, role int, userID string) ([]EntityTieRef, error)
	EntitiesForEra(ctx context.Context, eraID int, role int, userID string) ([]EntityTieRef, error)
	EntitiesForCalendar(ctx context.Context, calendarID string, role int, userID string) ([]EntityTieRef, error)
	// EventsForEntity and ErasForEntity take campaignID explicitly: an entity
	// id alone carries no campaign to filter by, and every tie must be
	// confined to one campaign the same way EntitiesFor* are. EventsForEntity
	// is role-filtered like the other event lists.
	EventsForEntity(ctx context.Context, campaignID, entityID string, role int) ([]EntityEventTie, error)
	ErasForEntity(ctx context.Context, campaignID, entityID string) ([]EntityEraTie, error)
}

// eventRepo is the MariaDB implementation of EventRepository.
type eventRepo struct {
	db *sql.DB
}

// NewEventRepository creates a new MariaDB-backed event repository.
func NewEventRepository(db *sql.DB) EventRepository {
	return &eventRepo{db: db}
}

// eventCols is the column list for event queries, joined to the linked
// entity's display fields and the event's kind's display fields.
//
// INVARIANT: this list and eventDests' returned slice must stay the same
// length and order; TestScanArityMatchesColumnLists (scan_contract_test.go)
// pins it. A silent mismatch here fails every event query at runtime with
// "sql: expected N destination arguments in Scan", invisible to any test
// that doesn't run against a real database.
const eventCols = `e.id, e.calendar_id, e.entity_id, e.name, e.description, e.description_html,
       e.year, e.month, e.day, e.start_hour, e.start_minute,
       e.end_year, e.end_month, e.end_day, e.end_hour, e.end_minute,
       e.is_recurring, e.recurrence_type,
       e.recurrence_interval, e.recurrence_end_year, e.recurrence_end_month,
       e.recurrence_end_day, e.recurrence_max_occurrences, e.recurrence_day_of_week,
       e.visibility, e.visibility_rules, e.kind_id, e.announced, e.tier,
       e.color, e.icon, e.all_day, e.payload,
       e.created_by, e.created_at, e.updated_at,
       COALESCE(ent.name, ''), COALESCE(et.icon, ''), COALESCE(et.color, ''),
       COALESCE(k.name, ''), COALESCE(k.slug, ''), COALESCE(k.icon, ''), COALESCE(k.color, '')`

// eventJoins is the LEFT JOIN clause for entity + kind display data, plus the
// INNER JOIN to calendars that both display joins guard against: a
// cross-campaign kind_id/entity_id should never happen once CreateEvent/
// UpdateEvent validate it, but the guard keeps a query honest even against
// old or hand-edited rows, rather than trusting the write path alone.
const eventJoins = `JOIN calendars c ON c.id = e.calendar_id
     LEFT JOIN entities ent ON ent.id = e.entity_id AND ent.campaign_id = c.campaign_id
     LEFT JOIN entity_types et ON et.id = ent.entity_type_id
     LEFT JOIN calendar_event_kinds k ON k.id = e.kind_id AND k.campaign_id = c.campaign_id`

// recurringCandidateClause widens a date-bounded event query to include every
// RECURRING row whose base date sits outside the window, because Event.OccursOn
// (not the SQL) decides where an instance lands. Derived from the
// Recurrence* constants so the accepted set is never a hand-typed literal.
var recurringCandidateClause = buildRecurringCandidateClause()

// spanningCandidateClause widens a MONTH-bounded event query to include every
// multi-day row whose stored [start, end] window overlaps that month even
// though its stored month is a different one. The radix is 10000 (not the
// house idiom's 100), since a user-authored month list can exceed 99 days.
const spanningCandidateClause = `(
		    e.end_year IS NOT NULL AND e.end_month IS NOT NULL AND e.end_day IS NOT NULL
		    AND (e.end_year * 100000000 + e.end_month * 10000 + e.end_day)
		        >= (? * 100000000 + ? * 10000 + 0)
		    AND (e.year * 100000000 + e.month * 10000 + e.day)
		        <= (? * 100000000 + ? * 10000 + 9999)
		  )`

func buildRecurringCandidateClause() string {
	quoted := make([]string, 0, len(RecurrenceTypes))
	for _, t := range RecurrenceTypes {
		quoted = append(quoted, "'"+strings.ReplaceAll(t, "'", "''")+"'")
	}
	return "(e.is_recurring = 1 AND e.recurrence_type IN (" + strings.Join(quoted, ",") + "))"
}

// validateEventRefs confirms kindID and entityID (either may be nil) belong
// to campaignID, the campaign of the event's own calendar. Returns
// apperror.NewValidation on a cross-campaign reference, so a write can never
// point an event at another campaign's kind or entity.
func validateEventRefs(ctx context.Context, db *sql.DB, campaignID string, kindID *int, entityID *string) error {
	if kindID != nil {
		var exists bool
		if err := db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM calendar_event_kinds WHERE id = ? AND campaign_id = ?)`,
			*kindID, campaignID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apperror.NewValidation("event kind does not belong to this campaign")
		}
	}
	if entityID != nil {
		var exists bool
		if err := db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM entities WHERE id = ? AND campaign_id = ?)`,
			*entityID, campaignID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apperror.NewValidation("entity does not belong to this campaign")
		}
	}
	return nil
}

// validEventPayload rejects a non-JSON payload before it ever reaches the
// database; calendar_events.payload has no CHECK constraint, so a malformed
// value would otherwise sit silently until ParsePayload fails to read it back.
func validEventPayload(payload *string) error {
	if payload != nil && *payload != "" && !json.Valid([]byte(*payload)) {
		return apperror.NewValidation("payload is not valid JSON")
	}
	return nil
}

// CreateEvent inserts a new event. The INSERT ... SELECT ... FROM calendars
// guards kind_id and entity_id server-side: the SELECT produces a row only
// when the calendar exists and (each reference is nil, or exists in the
// SAME campaign as that calendar), so a write can never point an event at
// another campaign's kind or entity even if the caller's own check is
// skipped or wrong. Zero rows is reported as a validation error.
func (r *eventRepo) CreateEvent(ctx context.Context, evt *Event) error {
	if err := validEventPayload(evt.Payload); err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_events (id, calendar_id, entity_id, name, description, description_html,
		        year, month, day, start_hour, start_minute,
		        end_year, end_month, end_day, end_hour, end_minute,
		        is_recurring, recurrence_type,
		        recurrence_interval, recurrence_end_year, recurrence_end_month,
		        recurrence_end_day, recurrence_max_occurrences, recurrence_day_of_week,
		        visibility, visibility_rules, kind_id, announced, tier,
		        color, icon, all_day, payload, created_by)
		 SELECT ?, c.id, ?, ?, ?, ?,
		        ?, ?, ?, ?, ?,
		        ?, ?, ?, ?, ?,
		        ?, ?,
		        ?, ?, ?,
		        ?, ?, ?,
		        ?, ?, ?, ?, ?,
		        ?, ?, ?, ?, ?
		 FROM calendars c
		 WHERE c.id = ?
		   AND (? IS NULL OR EXISTS (SELECT 1 FROM calendar_event_kinds k WHERE k.id = ? AND k.campaign_id = c.campaign_id))
		   AND (? IS NULL OR EXISTS (SELECT 1 FROM entities en WHERE en.id = ? AND en.campaign_id = c.campaign_id))`,
		evt.ID, evt.EntityID, evt.Name, evt.Description, evt.DescriptionHTML,
		evt.Year, evt.Month, evt.Day, evt.StartHour, evt.StartMinute,
		evt.EndYear, evt.EndMonth, evt.EndDay, evt.EndHour, evt.EndMinute,
		evt.IsRecurring, evt.RecurrenceType,
		evt.RecurrenceInterval, evt.RecurrenceEndYear, evt.RecurrenceEndMonth,
		evt.RecurrenceEndDay, evt.RecurrenceMaxOccurrences, evt.RecurrenceDayOfWeek,
		evt.Visibility, evt.VisibilityRules, evt.KindID, evt.Announced, evt.Tier,
		evt.Color, evt.Icon, evt.AllDay, evt.Payload, evt.CreatedBy,
		evt.CalendarID,
		evt.KindID, evt.KindID,
		evt.EntityID, evt.EntityID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return apperror.NewValidation("calendar, event kind, or entity reference is invalid")
	}
	return nil
}

// eventDests returns the Scan destination list for eventCols, in the same
// order, so GetEvent/scanEvents and EventsForEntity's scan (which appends
// one trailing extra destination, in entity_ties_repository.go) read the
// exact same list and can't drift apart.
func eventDests(evt *Event) []any {
	return []any{
		&evt.ID, &evt.CalendarID, &evt.EntityID, &evt.Name, &evt.Description, &evt.DescriptionHTML,
		&evt.Year, &evt.Month, &evt.Day, &evt.StartHour, &evt.StartMinute,
		&evt.EndYear, &evt.EndMonth, &evt.EndDay, &evt.EndHour, &evt.EndMinute,
		&evt.IsRecurring, &evt.RecurrenceType,
		&evt.RecurrenceInterval, &evt.RecurrenceEndYear, &evt.RecurrenceEndMonth,
		&evt.RecurrenceEndDay, &evt.RecurrenceMaxOccurrences, &evt.RecurrenceDayOfWeek,
		&evt.Visibility, &evt.VisibilityRules, &evt.KindID, &evt.Announced, &evt.Tier,
		&evt.Color, &evt.Icon, &evt.AllDay, &evt.Payload,
		&evt.CreatedBy, &evt.CreatedAt, &evt.UpdatedAt,
		&evt.EntityName, &evt.EntityIcon, &evt.EntityColor,
		&evt.KindName, &evt.KindSlug, &evt.KindIcon, &evt.KindColor,
	}
}

// scanEventRow reads one event row (eventCols' shape) from a Scan-capable
// source, shared by GetEvent and scanEvents so the two can never drift apart.
func scanEventRow(scanner interface{ Scan(...any) error }) (*Event, error) {
	var evt Event
	if err := scanner.Scan(eventDests(&evt)...); err != nil {
		return nil, err
	}
	return &evt, nil
}

// GetEvent returns a single event by ID, or nil if not found.
func (r *eventRepo) GetEvent(ctx context.Context, id string) (*Event, error) {
	evt, err := scanEventRow(r.db.QueryRowContext(ctx,
		`SELECT `+eventCols+`
		 FROM calendar_events e `+eventJoins+`
		 WHERE e.id = ?`, id,
	))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return evt, err
}

// eventIDsChunk bounds the IN list of GetEventsByIDs so a timeline with a very
// large link set stays under the driver's placeholder and packet limits.
const eventIDsChunk = 500

// GetEventsByIDs returns the events with the given ids on calendarID. The
// calendar_id predicate is in SQL so an id from another calendar can never
// be returned, matching GetEventForViewer's calendar-membership check.
func (r *eventRepo) GetEventsByIDs(ctx context.Context, calendarID string, ids []string) ([]Event, error) {
	var out []Event
	for start := 0; start < len(ids); start += eventIDsChunk {
		end := start + eventIDsChunk
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		ph := make([]string, len(chunk))
		args := make([]any, 0, len(chunk)+1)
		args = append(args, calendarID)
		for i, id := range chunk {
			ph[i] = "?"
			args = append(args, id)
		}
		rows, err := r.db.QueryContext(ctx,
			`SELECT `+eventCols+`
			 FROM calendar_events e `+eventJoins+`
			 WHERE e.calendar_id = ? AND e.id IN (`+strings.Join(ph, ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		events, err := scanEvents(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, events...)
	}
	return out, nil
}

// UpdateEvent modifies an existing event. Validates kind_id/entity_id
// against the event's own calendar's campaign before writing (the same
// guard CreateEvent applies through SQL, applied here in Go since an UPDATE
// has no prior row to INSERT ... SELECT against). Returns
// apperror.NewNotFound if the event doesn't exist.
func (r *eventRepo) UpdateEvent(ctx context.Context, evt *Event) error {
	if err := validEventPayload(evt.Payload); err != nil {
		return err
	}
	var campaignID string
	if err := r.db.QueryRowContext(ctx,
		`SELECT c.campaign_id FROM calendar_events e JOIN calendars c ON c.id = e.calendar_id WHERE e.id = ?`,
		evt.ID).Scan(&campaignID); err != nil {
		if err == sql.ErrNoRows {
			return apperror.NewNotFound("event not found")
		}
		return err
	}
	if err := validateEventRefs(ctx, r.db, campaignID, evt.KindID, evt.EntityID); err != nil {
		return err
	}

	_, err := r.db.ExecContext(ctx,
		`UPDATE calendar_events
		 SET name = ?, description = ?, description_html = ?, entity_id = ?,
		     year = ?, month = ?, day = ?,
		     start_hour = ?, start_minute = ?,
		     end_year = ?, end_month = ?, end_day = ?, end_hour = ?, end_minute = ?,
		     is_recurring = ?, recurrence_type = ?,
		     recurrence_interval = ?, recurrence_end_year = ?, recurrence_end_month = ?,
		     recurrence_end_day = ?, recurrence_max_occurrences = ?, recurrence_day_of_week = ?,
		     visibility = ?, visibility_rules = ?, kind_id = ?, announced = ?, tier = ?,
		     color = ?, icon = ?, all_day = ?, payload = ?
		 WHERE id = ?`,
		evt.Name, evt.Description, evt.DescriptionHTML, evt.EntityID,
		evt.Year, evt.Month, evt.Day,
		evt.StartHour, evt.StartMinute,
		evt.EndYear, evt.EndMonth, evt.EndDay, evt.EndHour, evt.EndMinute,
		evt.IsRecurring, evt.RecurrenceType,
		evt.RecurrenceInterval, evt.RecurrenceEndYear, evt.RecurrenceEndMonth,
		evt.RecurrenceEndDay, evt.RecurrenceMaxOccurrences, evt.RecurrenceDayOfWeek,
		evt.Visibility, evt.VisibilityRules, evt.KindID, evt.Announced, evt.Tier,
		evt.Color, evt.Icon, evt.AllDay, evt.Payload, evt.ID,
	)
	return err
}

// DeleteEvent removes an event (cascades entity_event_links via FK).
func (r *eventRepo) DeleteEvent(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM calendar_events WHERE id = ?`, id)
	return err
}

// scanEvents reads event rows into a slice, sharing scanEventRow with GetEvent
// so the two destination lists can never drift apart.
func scanEvents(rows *sql.Rows) ([]Event, error) {
	var events []Event
	for rows.Next() {
		evt, err := scanEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *evt)
	}
	return events, rows.Err()
}

// ListEventsForMonth returns all events for a specific month, filtered by
// role. Recurring/spanning candidates from anywhere in the calendar are
// widened in via recurringCandidateClause/spanningCandidateClause; the exact
// placement is decided in Go by Event.OccursOn.
func (r *eventRepo) ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ?
		  AND (
		    (e.year = ? AND e.month = ?)
		    OR `+recurringCandidateClause+`
		    OR `+spanningCandidateClause+`
		  )
		  %s
		ORDER BY e.day, COALESCE(e.start_hour, 99), COALESCE(e.start_minute, 99), e.name`, visFilter)

	rows, err := r.db.QueryContext(ctx, query, calendarID, year, month, year, month, year, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ListEventsForYear returns all events for a specific year, filtered by role.
// Recurring events surface only in their stored year here; this does not
// expand recurrence across years.
func (r *eventRepo) ListEventsForYear(ctx context.Context, calendarID string, year int, role int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

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

// ListAllEvents returns every event for a calendar with no role filter and no
// date constraint. See the EventRepository doc comment on who may call this.
func (r *eventRepo) ListAllEvents(ctx context.Context, calendarID string) ([]Event, error) {
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

// StrandedEventCounts returns, per calendar in a campaign, how many events
// point at a month position the calendar no longer has. One query for the
// whole campaign. Calendars with none are absent from the map.
func (r *eventRepo) StrandedEventCounts(ctx context.Context, campaignID string) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.calendar_id, COUNT(*)
		FROM calendar_events e
		JOIN calendars c ON c.id = e.calendar_id
		LEFT JOIN (
			SELECT calendar_id, COUNT(*) AS n
			FROM calendar_months
			GROUP BY calendar_id
		) m ON m.calendar_id = e.calendar_id
		WHERE c.campaign_id = ?
		  AND (e.month < 1 OR e.month > COALESCE(m.n, 0))
		GROUP BY e.calendar_id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// ListEventsForDateRange returns events within a date range in the same year.
func (r *eventRepo) ListEventsForDateRange(ctx context.Context, calendarID string, year, startMonth, startDay, endMonth, endDay int, role int) ([]Event, error) {
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e `+eventJoins+`
		WHERE e.calendar_id = ?
		  AND (
		    (e.year = ? AND (e.month * 100 + e.day) >= ? AND (e.month * 100 + e.day) <= ?)
		    OR `+recurringCandidateClause+`
		  )
		  %s
		ORDER BY e.month, e.day, COALESCE(e.start_hour, 99), COALESCE(e.start_minute, 99), e.name`, visFilter)

	startVal := startMonth*100 + startDay
	endVal := endMonth*100 + endDay

	rows, err := r.db.QueryContext(ctx, query, calendarID, year, startVal, endVal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ListEventsForEntity returns all events linked via the legacy single-entity
// column (calendar_events.entity_id): distinct from EventsForEntity, which
// reads the many-to-many entity_event_links.
func (r *eventRepo) ListEventsForEntity(ctx context.Context, entityID string, role int) ([]Event, error) {
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

// ListUpcomingEvents returns events on or after the given date, chronological.
func (r *eventRepo) ListUpcomingEvents(ctx context.Context, calendarID string, year, month, day int, role int, limit int) ([]Event, error) {
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

// EventDatesForCalendars batch-reads (year,month,day,name) for every event in
// the given calendars in ONE query, role-filtered. Ordered by date so the
// caller can pick each calendar's soonest upcoming event using that
// calendar's own current date.
func (r *eventRepo) EventDatesForCalendars(ctx context.Context, calIDs []string, role int) (map[string][]CalendarEventDate, error) {
	out := map[string][]CalendarEventDate{}
	if len(calIDs) == 0 {
		return out, nil
	}
	ph := make([]string, len(calIDs))
	args := make([]any, len(calIDs))
	for i, id := range calIDs {
		ph[i] = "?"
		args[i] = id
	}
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}
	q := fmt.Sprintf(`SELECT e.calendar_id, e.year, e.month, e.day, e.name
		FROM calendar_events e
		WHERE e.calendar_id IN (%s) %s
		ORDER BY e.calendar_id, e.year, e.month, e.day, e.name`,
		strings.Join(ph, ","), visFilter)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d CalendarEventDate
		if err := rows.Scan(&d.CalendarID, &d.Year, &d.Month, &d.Day, &d.Name); err != nil {
			return nil, err
		}
		out[d.CalendarID] = append(out[d.CalendarID], d)
	}
	return out, rows.Err()
}

// UpdateEventVisibility sets the visibility and per-user rules on an event.
func (r *eventRepo) UpdateEventVisibility(ctx context.Context, eventID string, visibility string, visRules *string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendar_events SET visibility = ?, visibility_rules = ? WHERE id = ?`,
		visibility, visRules, eventID,
	)
	return err
}

// SearchEvents returns events matching a name query, filtered by role.
func (r *eventRepo) SearchEvents(ctx context.Context, calendarID, query string, role int) ([]Event, error) {
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
