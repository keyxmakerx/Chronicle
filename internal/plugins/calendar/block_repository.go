// block_repository.go — C-CALV4-SPINE-P2 (calendar-v4 wave 1, W-A).
//
// The batched read surface the calendar Block and the Bench need, as a NEW file
// with NEW methods on the existing *calendarRepo.
//
// WHY NOT repository.go / CalendarRepository. The CalendarRepository interface
// is ~60 methods and is implemented by a HAND-WRITTEN mockCalendarRepo in
// service_test.go. Widening it churns that mock and breaks the package's test
// build for every parallel calendar-v4 slice at once, which is precisely what
// the dispatch forbids. These methods therefore hang off the concrete
// *calendarRepo (Go allows methods on a type from any file in its package) and
// are published through the narrow BlockRepository interface in
// block_service.go, constructed by NewBlockRepository. Nothing existing is
// touched.
//
// WHY BATCHED. calendarService.eagerLoad (service.go :668-701) is NINE
// sequential per-calendar queries — months, weekdays, moons, seasons, eras,
// categories, cycles, festivals, weather. The Bench's subordinate rows come
// from ListByCampaignID (repository.go :255-274) as SHALLOW Calendar structs
// with no Months, yet every row must print an in-world date label
// ("Sithrel 9, Cycle 218"), which needs Months. Four calendars × nine
// unbatched queries, on the page the nav Calendar tab lands on, for every
// player. These loaders are O(1) queries in the number of calendars.
package calendar

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// NewBlockRepository returns the Block spine's narrow read surface over the
// same MariaDB handle the calendar repository uses. A separate constructor
// (rather than a type assertion on CalendarRepository) keeps the wiring in
// internal/app/routes.go statically checked — a silently-failed assertion there
// would disable the whole Block with no compile error.
func NewBlockRepository(db *sql.DB) BlockRepository {
	return &calendarRepo{db: db}
}

// blockInClause builds an IN (?,?,…) placeholder list plus its args, or
// reports false for an empty id set so callers can short-circuit without
// issuing `IN ()` (a syntax error in MariaDB).
func blockInClause(ids []string) (string, []any, bool) {
	if len(ids) == 0 {
		return "", nil, false
	}
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	return strings.Join(ph, ","), args, true
}

// blockIntInClause is blockInClause for int keys (cycle ids).
func blockIntInClause(ids []int) (string, []any, bool) {
	if len(ids) == 0 {
		return "", nil, false
	}
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	return strings.Join(ph, ","), args, true
}

// MonthsForCalendars batch-reads every calendar's months in ONE query, keyed by
// calendar id and ordered by sort_order within each calendar — the same order
// GetMonths (repository.go :411) returns, because month ORDER is the calendar's
// geometry and a different order would silently renumber every date.
func (r *calendarRepo) MonthsForCalendars(ctx context.Context, calIDs []string) (map[string][]Month, error) {
	out := map[string][]Month{}
	in, args, ok := blockInClause(calIDs)
	if !ok {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, calendar_id, name, days, sort_order, is_intercalary, leap_year_days
		 FROM calendar_months WHERE calendar_id IN (%s)
		 ORDER BY calendar_id, sort_order`, in), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Month
		if err := rows.Scan(&m.ID, &m.CalendarID, &m.Name, &m.Days, &m.SortOrder,
			&m.IsIntercalary, &m.LeapYearDays); err != nil {
			return nil, err
		}
		out[m.CalendarID] = append(out[m.CalendarID], m)
	}
	return out, rows.Err()
}

// WeekdaysForCalendars batch-reads weekdays. Order is load-bearing: the weekday
// index is a modulus over this slice.
func (r *calendarRepo) WeekdaysForCalendars(ctx context.Context, calIDs []string) (map[string][]Weekday, error) {
	out := map[string][]Weekday{}
	in, args, ok := blockInClause(calIDs)
	if !ok {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, calendar_id, name, sort_order, is_rest_day
		 FROM calendar_weekdays WHERE calendar_id IN (%s)
		 ORDER BY calendar_id, sort_order`, in), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var w Weekday
		if err := rows.Scan(&w.ID, &w.CalendarID, &w.Name, &w.SortOrder, &w.IsRestDay); err != nil {
			return nil, err
		}
		out[w.CalendarID] = append(out[w.CalendarID], w)
	}
	return out, rows.Err()
}

// MoonsForCalendars batch-reads moons (the per-day disc source).
func (r *calendarRepo) MoonsForCalendars(ctx context.Context, calIDs []string) (map[string][]Moon, error) {
	out := map[string][]Moon{}
	in, args, ok := blockInClause(calIDs)
	if !ok {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, calendar_id, name, cycle_days, phase_offset, color,
		        base_design, tint, phase_source, size, orbit_speed
		 FROM calendar_moons WHERE calendar_id IN (%s)
		 ORDER BY calendar_id, id`, in), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Moon
		if err := rows.Scan(&m.ID, &m.CalendarID, &m.Name, &m.CycleDays, &m.PhaseOffset, &m.Color,
			&m.BaseDesign, &m.Tint, &m.PhaseSource, &m.Size, &m.OrbitSpeed); err != nil {
			return nil, err
		}
		out[m.CalendarID] = append(out[m.CalendarID], m)
	}
	return out, rows.Err()
}

// SeasonsForCalendars batch-reads seasons (the era band's row-0 suffix).
func (r *calendarRepo) SeasonsForCalendars(ctx context.Context, calIDs []string) (map[string][]Season, error) {
	out := map[string][]Season{}
	in, args, ok := blockInClause(calIDs)
	if !ok {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, calendar_id, name, start_month, start_day, end_month, end_day,
		        description, color, weather_effect
		 FROM calendar_seasons WHERE calendar_id IN (%s)
		 ORDER BY calendar_id, id`, in), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s Season
		if err := rows.Scan(&s.ID, &s.CalendarID, &s.Name, &s.StartMonth, &s.StartDay,
			&s.EndMonth, &s.EndDay, &s.Description, &s.Color, &s.WeatherEffect); err != nil {
			return nil, err
		}
		out[s.CalendarID] = append(out[s.CalendarID], s)
	}
	return out, rows.Err()
}

// ErasForCalendars batch-reads eras (the band labels and --bandhue source).
func (r *calendarRepo) ErasForCalendars(ctx context.Context, calIDs []string) (map[string][]Era, error) {
	out := map[string][]Era{}
	in, args, ok := blockInClause(calIDs)
	if !ok {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, calendar_id, name, start_year, end_year, description, color, sort_order
		 FROM calendar_eras WHERE calendar_id IN (%s)
		 ORDER BY calendar_id, sort_order, start_year`, in), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e Era
		if err := rows.Scan(&e.ID, &e.CalendarID, &e.Name, &e.StartYear, &e.EndYear,
			&e.Description, &e.Color, &e.SortOrder); err != nil {
			return nil, err
		}
		out[e.CalendarID] = append(out[e.CalendarID], e)
	}
	return out, rows.Err()
}

// EventCategoriesForCalendars batch-reads categories (a mark's locked
// hue+pattern+glyph key).
func (r *calendarRepo) EventCategoriesForCalendars(ctx context.Context, calIDs []string) (map[string][]EventCategory, error) {
	out := map[string][]EventCategory{}
	in, args, ok := blockInClause(calIDs)
	if !ok {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, calendar_id, slug, name, icon, color, sort_order
		 FROM calendar_event_categories WHERE calendar_id IN (%s)
		 ORDER BY calendar_id, sort_order`, in), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c EventCategory
		if err := rows.Scan(&c.ID, &c.CalendarID, &c.Slug, &c.Name, &c.Icon, &c.Color, &c.SortOrder); err != nil {
			return nil, err
		}
		out[c.CalendarID] = append(out[c.CalendarID], c)
	}
	return out, rows.Err()
}

// FestivalsForCalendars batch-reads festivals (the intercalary-day names).
func (r *calendarRepo) FestivalsForCalendars(ctx context.Context, calIDs []string) (map[string][]Festival, error) {
	out := map[string][]Festival{}
	in, args, ok := blockInClause(calIDs)
	if !ok {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, calendar_id, name, month, day, after_month, description, color, icon, sort_order
		 FROM calendar_festivals WHERE calendar_id IN (%s)
		 ORDER BY calendar_id, sort_order`, in), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f Festival
		if err := rows.Scan(&f.ID, &f.CalendarID, &f.Name, &f.Month, &f.Day, &f.AfterMonth,
			&f.Description, &f.Color, &f.Icon, &f.SortOrder); err != nil {
			return nil, err
		}
		out[f.CalendarID] = append(out[f.CalendarID], f)
	}
	return out, rows.Err()
}

// CyclesForCalendars batch-reads cycles AND their entries in exactly TWO
// queries, whatever the calendar count. GetCycles (repository.go :1526) issues
// one entry query PER CYCLE on top of its cycle query, so the Bench's four
// calendars with three cycles each cost 1 + 12 there and 2 here.
func (r *calendarRepo) CyclesForCalendars(ctx context.Context, calIDs []string) (map[string][]Cycle, error) {
	out := map[string][]Cycle{}
	in, args, ok := blockInClause(calIDs)
	if !ok {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, calendar_id, name, cycle_length, type, sort_order
		 FROM calendar_cycles WHERE calendar_id IN (%s)
		 ORDER BY calendar_id, sort_order`, in), args...)
	if err != nil {
		return nil, err
	}
	// byID lets the entry pass write straight into the slice element the cycle
	// map already holds, so the entries land on the same struct the caller sees.
	byID := map[int]*Cycle{}
	var cycleIDs []int
	for rows.Next() {
		var c Cycle
		if err := rows.Scan(&c.ID, &c.CalendarID, &c.Name, &c.CycleLength, &c.Type, &c.SortOrder); err != nil {
			rows.Close()
			return nil, err
		}
		out[c.CalendarID] = append(out[c.CalendarID], c)
		cycleIDs = append(cycleIDs, c.ID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for calID := range out {
		for i := range out[calID] {
			byID[out[calID][i].ID] = &out[calID][i]
		}
	}

	inIDs, idArgs, ok := blockIntInClause(cycleIDs)
	if !ok {
		return out, nil
	}
	erows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, cycle_id, name, icon, year_offset, sort_order
		 FROM calendar_cycle_entries WHERE cycle_id IN (%s)
		 ORDER BY cycle_id, sort_order`, inIDs), idArgs...)
	if err != nil {
		return nil, err
	}
	defer erows.Close()
	for erows.Next() {
		var e CycleEntry
		if err := erows.Scan(&e.ID, &e.CycleID, &e.Name, &e.Icon, &e.YearOffset, &e.SortOrder); err != nil {
			return nil, err
		}
		if c := byID[e.CycleID]; c != nil {
			c.Entries = append(c.Entries, e)
		}
	}
	return out, erows.Err()
}

// EntitiesForEventsBatch is the batched tie read behind the Block's tie
// rendering: every entity tied to any of the given events, in ONE query.
//
// It is the N+1-free sibling of EntitiesForEvent (entity_ties_repository.go
// :170) and reuses that file's entityVisibilityFilter VERBATIM — including the
// `e` alias the filter's SQL fragment hardcodes — so a Player cannot learn a
// dm_only or custom-restricted entity's NAME through an event's tie list
// (C-CAL-ENTITY-TIES-LEAK-FIX). Reconfirmed against main as part of this
// slice's Step 0: both the event-side and era-side reads take (role, userID)
// and the handler forwards cc.VisibilityRole() + auth.GetUserID(c).
//
// The method is named *Batch rather than EntitiesForEvents because
// C-CALV4-TIEFIX-PB owns entity_ties_repository.go in the same wave and is
// adding its own viewer-filtered variant there; a colliding name across two
// parallel branches is a merge conflict in a file this slice must not touch.
func (r *calendarRepo) EntitiesForEventsBatch(ctx context.Context, eventIDs []string, role int, userID string) (map[string][]EntityTieRef, error) {
	out := map[string][]EntityTieRef{}
	in, args, ok := blockInClause(eventIDs)
	if !ok {
		return out, nil
	}
	visFilter, visArgs := entityVisibilityFilter(role, userID)
	args = append(args, visArgs...)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT l.event_id, l.entity_id, COALESCE(e.name, ''), COALESCE(et.slug, ''),
		        COALESCE(et.icon, ''), COALESCE(et.color, ''), l.participation_role
		 FROM entity_event_links l
		 JOIN entities e ON e.id = l.entity_id
		 LEFT JOIN entity_types et ON et.id = e.entity_type_id
		 WHERE l.event_id IN (%s)%s
		 ORDER BY l.event_id, e.name`, in, visFilter), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var eventID string
		var ref EntityTieRef
		var prole sql.NullString
		if err := rows.Scan(&eventID, &ref.EntityID, &ref.EntityName, &ref.EntityType,
			&ref.EntityIcon, &ref.EntityColor, &prole); err != nil {
			return nil, err
		}
		if prole.Valid {
			v := prole.String
			ref.ParticipationRole = &v
		}
		out[eventID] = append(out[eventID], ref)
	}
	return out, rows.Err()
}

// TiedEventIDsForEntity returns which of the given events are tied to one
// entity, in ONE query.
//
// No entity-visibility filter is applied and that is deliberate: the caller
// already holds the host entity (the Block is rendering ON its page, which is
// itself permission-gated), and the event set passed in has already been
// viewer-filtered. The answer therefore reveals nothing the viewer does not
// already have. Using the visibility-filtered join instead would make the tie
// COUNT depend on the host entity's own visibility, which is a different
// question and a subtler oracle.
func (r *calendarRepo) TiedEventIDsForEntity(ctx context.Context, entityID string, eventIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if strings.TrimSpace(entityID) == "" {
		return out, nil
	}
	in, args, ok := blockInClause(eventIDs)
	if !ok {
		return out, nil
	}
	args = append([]any{entityID}, args...)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT l.event_id FROM entity_event_links l
		 WHERE l.entity_id = ? AND l.event_id IN (%s)`, in), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// UpcomingEventsForCalendars batch-reads the FULL event rows (not just dates)
// for every calendar in one query, from each calendar's own current date
// forward, so the cross-calendar NEXT UP index can be filtered per viewer in Go.
//
// WHY FULL ROWS. The existing batch read EventDatesForCalendars
// (repository.go :1195) returns only (calendar, date, name) and filters base
// visibility in SQL ONLY — it never reaches filterEventsByUser, so an event
// carrying visibility_rules {allowed_users, denied_users} leaks its NAME to a
// player through UpcomingByCalendar. Selecting the rows means the service can
// run the same Go-side resolver every other surface runs.
//
// Ordering here is per calendar by date; the CROSS-calendar order is a service
// decision (see UpcomingAcrossCalendars) because two in-world calendars with
// unrelated epochs have no natural order and SQL cannot invent one.
//
// There is deliberately NO per-calendar row cap. A cap would have to be applied
// before recurrence expansion, and a recurring row whose BASE date sits in the
// past sorts ahead of genuinely-upcoming rows — so the cap would silently drop
// the events it was meant to keep. The WHERE clause is the narrowing.
func (r *calendarRepo) UpcomingEventsForCalendars(ctx context.Context, calIDs []string, role int) (map[string][]Event, error) {
	out := map[string][]Event{}
	in, args, ok := blockInClause(calIDs)
	if !ok {
		return out, nil
	}
	visFilter := "AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}
	// The date floor is each calendar's OWN current date, joined from the
	// calendars row so one query covers calendars whose epochs are unrelated.
	// Recurring rows are pulled in regardless of their stored date — their base
	// date may sit in the past while an instance is still ahead; Event.OccursOn
	// decides placement in Go.
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT `+eventCols+`
		FROM calendar_events e
		JOIN calendars c ON c.id = e.calendar_id `+eventJoins+`
		WHERE e.calendar_id IN (%s)
		  AND (
		    (e.year > c.current_year)
		    OR (e.year = c.current_year AND e.month > c.current_month)
		    OR (e.year = c.current_year AND e.month = c.current_month AND e.day >= c.current_day)
		    OR (e.is_recurring = 1 AND e.recurrence_type IN ('weekly','biweekly','monthly','custom'))
		  )
		  %s
		ORDER BY e.calendar_id, e.year, e.month, e.day,
		         COALESCE(e.start_hour, 99), COALESCE(e.start_minute, 99), e.name, e.id`,
		in, visFilter), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	evs, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	for _, e := range evs {
		out[e.CalendarID] = append(out[e.CalendarID], e)
	}
	return out, nil
}
