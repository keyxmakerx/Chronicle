// entity_ties_repository.go — MariaDB reads/writes for the entity<->event /
// entity<->era link tables (migration 009 / C-CAL-ENTITY-TIES-DATA-MODEL).
// Hand-written SQL per the conventions. Cascade-on-delete is DB-enforced via
// the ON DELETE CASCADE FKs, so there is no delete-fan-out here.
package calendar

import (
	"context"
	"database/sql"
)

// LinkEntityEvent upserts an entity<->event tie with a role. Re-linking an
// existing pair updates the role (the unique key makes this idempotent).
func (r *calendarRepo) LinkEntityEvent(ctx context.Context, entityID, eventID, role string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO entity_event_links (entity_id, event_id, participation_role)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE participation_role = VALUES(participation_role)`,
		entityID, eventID, role)
	return err
}

// UnlinkEntityEvent removes an entity<->event tie. No-op if absent.
func (r *calendarRepo) UnlinkEntityEvent(ctx context.Context, entityID, eventID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM entity_event_links WHERE entity_id = ? AND event_id = ?`,
		entityID, eventID)
	return err
}

// LinkEntityEra upserts an entity<->era tie. role may be nil (era ties are
// coarser — a nil role stores NULL).
func (r *calendarRepo) LinkEntityEra(ctx context.Context, entityID string, eraID int, role *string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO entity_era_links (entity_id, era_id, participation_role)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE participation_role = VALUES(participation_role)`,
		entityID, eraID, role)
	return err
}

// UnlinkEntityEra removes an entity<->era tie. No-op if absent.
func (r *calendarRepo) UnlinkEntityEra(ctx context.Context, entityID string, eraID int) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM entity_era_links WHERE entity_id = ? AND era_id = ?`,
		entityID, eraID)
	return err
}

// EntitiesForCalendar returns the DISTINCT entities tied to any event or era
// of the given calendar — the read behind the Calendars dashboard's read-only
// associations panel (C-APPS-CAL-DASH-W1). The link tables carry no
// calendar_id, so the calendar is reached through calendar_events.calendar_id /
// calendar_eras.calendar_id. DISTINCT collapses an entity tied via several
// events/eras to one row; participation_role is omitted (it's per-tie, not a
// single value at calendar scope). Ordered by entity name for a stable render.
func (r *calendarRepo) EntitiesForCalendar(ctx context.Context, calendarID string) ([]EntityTieRef, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT ent.id, COALESCE(ent.name, ''), COALESCE(et.slug, ''),
		        COALESCE(et.icon, ''), COALESCE(et.color, '')
		 FROM (
		     SELECT l.entity_id
		     FROM entity_event_links l
		     JOIN calendar_events e ON e.id = l.event_id
		     WHERE e.calendar_id = ?
		     UNION
		     SELECT l.entity_id
		     FROM entity_era_links l
		     JOIN calendar_eras er ON er.id = l.era_id
		     WHERE er.calendar_id = ?
		 ) tied
		 JOIN entities ent ON ent.id = tied.entity_id
		 LEFT JOIN entity_types et ON et.id = ent.entity_type_id
		 ORDER BY ent.name`, calendarID, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EntityTieRef
	for rows.Next() {
		var ref EntityTieRef
		if err := rows.Scan(&ref.EntityID, &ref.EntityName, &ref.EntityType,
			&ref.EntityIcon, &ref.EntityColor); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// EntitiesForEvent returns every entity tied to an event with its display info
// (JOINs entities + entity_types — the same display source the event list
// uses). Ordered by entity name for a stable picker/chips render.
func (r *calendarRepo) EntitiesForEvent(ctx context.Context, eventID string) ([]EntityTieRef, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT l.entity_id, COALESCE(ent.name, ''), COALESCE(et.slug, ''),
		        COALESCE(et.icon, ''), COALESCE(et.color, ''), l.participation_role
		 FROM entity_event_links l
		 JOIN entities ent ON ent.id = l.entity_id
		 LEFT JOIN entity_types et ON et.id = ent.entity_type_id
		 WHERE l.event_id = ?
		 ORDER BY ent.name`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntityTieRefs(rows)
}

// EntitiesForEra returns every entity tied to an era with its display info.
func (r *calendarRepo) EntitiesForEra(ctx context.Context, eraID int) ([]EntityTieRef, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT l.entity_id, COALESCE(ent.name, ''), COALESCE(et.slug, ''),
		        COALESCE(et.icon, ''), COALESCE(et.color, ''), l.participation_role
		 FROM entity_era_links l
		 JOIN entities ent ON ent.id = l.entity_id
		 LEFT JOIN entity_types et ON et.id = ent.entity_type_id
		 WHERE l.era_id = ?
		 ORDER BY ent.name`, eraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntityTieRefs(rows)
}

// scanEntityTieRefs reads the shared entity-tie display projection. Role is a
// nullable column scanned through a sql.NullString.
func scanEntityTieRefs(rows *sql.Rows) ([]EntityTieRef, error) {
	var out []EntityTieRef
	for rows.Next() {
		var ref EntityTieRef
		var role sql.NullString
		if err := rows.Scan(&ref.EntityID, &ref.EntityName, &ref.EntityType,
			&ref.EntityIcon, &ref.EntityColor, &role); err != nil {
			return nil, err
		}
		if role.Valid && role.String != "" {
			s := role.String
			ref.ParticipationRole = &s
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// EventsForEntity returns every event tied to an entity (with the tie role).
// Reuses the event column projection + entity-display JOINs so the embedded
// Event carries the same display fields as the regular event lists.
func (r *calendarRepo) EventsForEntity(ctx context.Context, entityID string) ([]EntityEventTie, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventCols+`, l.participation_role
		 FROM entity_event_links l
		 JOIN calendar_events e ON e.id = l.event_id
		 `+eventJoins+`
		 WHERE l.entity_id = ?
		 ORDER BY e.year, e.month, e.day, e.name`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EntityEventTie
	for rows.Next() {
		var evt Event
		var role string
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
			&role,
		); err != nil {
			return nil, err
		}
		out = append(out, EntityEventTie{Event: evt, ParticipationRole: role})
	}
	return out, rows.Err()
}

// ErasForEntity returns every era tied to an entity (with the optional role).
func (r *calendarRepo) ErasForEntity(ctx context.Context, entityID string) ([]EntityEraTie, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT er.id, er.calendar_id, er.name, er.start_year, er.end_year,
		        er.description, er.color, er.sort_order, l.participation_role
		 FROM entity_era_links l
		 JOIN calendar_eras er ON er.id = l.era_id
		 WHERE l.entity_id = ?
		 ORDER BY er.start_year, er.sort_order`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EntityEraTie
	for rows.Next() {
		var era Era
		var role sql.NullString
		if err := rows.Scan(&era.ID, &era.CalendarID, &era.Name, &era.StartYear, &era.EndYear,
			&era.Description, &era.Color, &era.SortOrder, &role); err != nil {
			return nil, err
		}
		tie := EntityEraTie{Era: era}
		if role.Valid && role.String != "" {
			s := role.String
			tie.ParticipationRole = &s
		}
		out = append(out, tie)
	}
	return out, rows.Err()
}
