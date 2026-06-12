// entity_ties_repository.go — MariaDB reads/writes for the entity<->event /
// entity<->era link tables (migration 009 / C-CAL-ENTITY-TIES-DATA-MODEL).
// Hand-written SQL per the conventions. Cascade-on-delete is DB-enforced via
// the ON DELETE CASCADE FKs, so there is no delete-fan-out here.
package calendar

import (
	"context"
	"database/sql"

	"github.com/keyxmakerx/chronicle/internal/permissions"
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

// entityVisibilityFilter returns the WHERE-clause fragment + args that gate
// tied-entity rows by the viewer's role and userID. It is a verbatim MIRROR of
// internal/plugins/entities/repository.go::visibilityFilter (which is unexported
// — rule 8 forbids importing another plugin's repo, so we replicate the policy
// rather than invent a new one). Keep the two in sync: the alias is `e`
// (entities), the "default" mode honors the legacy is_private flag (Scribe+ see
// all, players see public only), the "custom" mode checks entity_permissions
// for a role/user/group grant, and an additive tag-grant branch widens
// visibility when any tag the entity bears carries a matching tag_permissions
// grant (C-PERM-W1-TAG-GRANTS — additive only, never hides). Owners
// (role >= RoleOwner) get no filter — they see every tied entity, including
// dm_only / custom-restricted ones.
//
// SECURITY-SENSITIVE — any change here MUST be applied identically to
// entities/repository.go::visibilityFilter, with both test suites + the
// cross-mirror sync pin (TestEntityVisibilityFilter) updated. cordinator#32/#455.
func entityVisibilityFilter(role int, userID string) (string, []any) {
	if role >= permissions.RoleOwner {
		return "", nil
	}
	filter := ` AND (
		(e.visibility = 'default' AND (? >= 2 OR e.is_private = false))
		OR (e.visibility = 'custom' AND EXISTS (
			SELECT 1 FROM entity_permissions ep
			WHERE ep.entity_id = e.id
			AND (
				(ep.subject_type = 'role' AND CAST(ep.subject_id AS UNSIGNED) <= ?)
				OR (ep.subject_type = 'user' AND ep.subject_id = ?)
				OR (ep.subject_type = 'group' AND EXISTS (
					SELECT 1 FROM campaign_group_members cgm
					WHERE cgm.group_id = CAST(ep.subject_id AS UNSIGNED)
					AND cgm.user_id = ?
				))
			)
		))
		OR EXISTS (
			SELECT 1 FROM entity_tags etg
			JOIN tag_permissions tp ON tp.tag_id = etg.tag_id
			WHERE etg.entity_id = e.id
			AND (
				(tp.subject_type = 'role' AND CAST(tp.subject_id AS UNSIGNED) <= ?)
				OR (tp.subject_type = 'user' AND tp.subject_id = ?)
				OR (tp.subject_type = 'group' AND EXISTS (
					SELECT 1 FROM campaign_group_members cgmt
					WHERE cgmt.group_id = CAST(tp.subject_id AS UNSIGNED)
					AND cgmt.user_id = ?
				))
			)
		)
	)`
	return filter, []any{role, role, userID, userID, role, userID, userID}
}

// EntitiesForCalendar returns the DISTINCT entities tied to any event or era
// of the given calendar — the read behind the Calendars dashboard's read-only
// associations panel (C-APPS-CAL-DASH-W1). The link tables carry no
// calendar_id, so the calendar is reached through calendar_events.calendar_id /
// calendar_eras.calendar_id. DISTINCT collapses an entity tied via several
// events/eras to one row; participation_role is omitted (it's per-tie, not a
// single value at calendar scope). Ordered by entity name for a stable render.
//
// The result is gated by the viewer's role + userID via entityVisibilityFilter
// (cordinator#32 gap #1): a player must not learn the NAME of a dm_only /
// custom-restricted entity through this association panel just because it's tied
// to a calendar they can otherwise see. Owners/co-DMs (role >= RoleOwner) get
// the unfiltered set.
func (r *calendarRepo) EntitiesForCalendar(ctx context.Context, calendarID string, role int, userID string) ([]EntityTieRef, error) {
	visFilter, visArgs := entityVisibilityFilter(role, userID)
	args := append([]any{calendarID, calendarID}, visArgs...)
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT e.id, COALESCE(e.name, ''), COALESCE(et.slug, ''),
		        COALESCE(et.icon, ''), COALESCE(et.color, '')
		 FROM (
		     SELECT l.entity_id
		     FROM entity_event_links l
		     JOIN calendar_events ev ON ev.id = l.event_id
		     WHERE ev.calendar_id = ?
		     UNION
		     SELECT l.entity_id
		     FROM entity_era_links l
		     JOIN calendar_eras er ON er.id = l.era_id
		     WHERE er.calendar_id = ?
		 ) tied
		 JOIN entities e ON e.id = tied.entity_id
		 LEFT JOIN entity_types et ON et.id = e.entity_type_id
		 WHERE 1=1`+visFilter+`
		 ORDER BY e.name`, args...)
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
