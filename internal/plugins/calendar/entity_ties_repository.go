// Package calendar - entity_ties_repository.go: MariaDB reads/writes for the
// entity<->event / entity<->era link tables. Hand-written SQL per the
// project conventions. Cascade-on-delete is DB-enforced via the ON DELETE
// CASCADE FKs, so there is no unlink-all-for-entity method here.
package calendar

import (
	"context"
	"database/sql"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// LinkEntityEvent upserts an entity<->event tie with a role. Re-linking an
// existing pair updates the role (the unique key makes this idempotent).
// The INSERT ... SELECT guards entityID against the event's own campaign,
// the same way CreateEvent guards kind_id/entity_id; a post-write existence
// check (not RowsAffected, which INSERT ... ON DUPLICATE KEY UPDATE reports
// as 0 for a legitimate no-op re-link with an unchanged role) tells a
// rejected guard apart from a successful no-op.
func (r *eventRepo) LinkEntityEvent(ctx context.Context, entityID, eventID, role string) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO entity_event_links (entity_id, event_id, participation_role)
		 SELECT ?, ?, ?
		 FROM calendar_events ev
		 JOIN calendars c ON c.id = ev.calendar_id
		 JOIN entities ent ON ent.id = ? AND ent.campaign_id = c.campaign_id AND ent.deleted_at IS NULL
		 WHERE ev.id = ?
		 ON DUPLICATE KEY UPDATE participation_role = VALUES(participation_role)`,
		entityID, eventID, role, entityID, eventID,
	); err != nil {
		return err
	}
	var exists bool
	if err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM entity_event_links WHERE entity_id = ? AND event_id = ?)`,
		entityID, eventID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return apperror.NewValidation("entity does not belong to the event's campaign, or the event does not exist")
	}
	return nil
}

// UnlinkEntityEvent removes an entity<->event tie. No-op if absent.
func (r *eventRepo) UnlinkEntityEvent(ctx context.Context, entityID, eventID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM entity_event_links WHERE entity_id = ? AND event_id = ?`,
		entityID, eventID)
	return err
}

// LinkEntityEra upserts an entity<->era tie. role may be nil (era ties are
// coarser: a nil role stores NULL). Guarded and existence-checked the same
// way LinkEntityEvent is, against the era's own calendar's campaign.
func (r *eventRepo) LinkEntityEra(ctx context.Context, entityID string, eraID int, role *string) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO entity_era_links (entity_id, era_id, participation_role)
		 SELECT ?, ?, ?
		 FROM calendar_eras er
		 JOIN calendars c ON c.id = er.calendar_id
		 JOIN entities ent ON ent.id = ? AND ent.campaign_id = c.campaign_id AND ent.deleted_at IS NULL
		 WHERE er.id = ?
		 ON DUPLICATE KEY UPDATE participation_role = VALUES(participation_role)`,
		entityID, eraID, role, entityID, eraID,
	); err != nil {
		return err
	}
	var exists bool
	if err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM entity_era_links WHERE entity_id = ? AND era_id = ?)`,
		entityID, eraID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return apperror.NewValidation("entity does not belong to the era's campaign, or the era does not exist")
	}
	return nil
}

// UnlinkEntityEra removes an entity<->era tie. No-op if absent.
func (r *eventRepo) UnlinkEntityEra(ctx context.Context, entityID string, eraID int) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM entity_era_links WHERE entity_id = ? AND era_id = ?`,
		entityID, eraID)
	return err
}

// entityVisibilityFilter returns the WHERE-clause fragment + args that gate
// tied-entity rows by the viewer's role and userID. This is a verbatim
// MIRROR of internal/plugins/entities/repository.go's unexported
// visibilityFilter: cross-plugin repo access is forbidden (plugins reach
// each other only through service interfaces), so the policy is replicated
// rather than imported. Keep the two in sync: the alias is `e` (entities),
// the "default" mode honors the legacy is_private flag (role >= Scribe sees
// all, others see public only), the "custom" mode checks entity_permissions
// for a role/user/group/public grant, and an additive tag-grant branch widens
// visibility when a tag the entity bears carries a matching tag_permissions
// grant, never hides. Owners (role >= RoleOwner) get no filter.
//
// SECURITY-SENSITIVE: any change here must be applied identically to
// entities/repository.go's visibilityFilter, with both test suites updated.
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
				OR (ep.subject_type = 'public')
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
				OR (tp.subject_type = 'public')
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

// entityTieCols is the column list for an entity-tie display projection with
// no role (EntitiesForCalendar, where a single entity may carry several
// roles across its ties and no one value applies at calendar scope).
const entityTieCols = `e.id, COALESCE(e.name, ''), COALESCE(et.slug, ''),
        COALESCE(et.icon, ''), COALESCE(et.color, '')`

// entityTieWithRoleCols is entityTieCols plus the tie's own role, for a
// single event's or era's ties (EntitiesForEvent, EntitiesForEra), scanned
// by the shared scanEntityTieRefs.
const entityTieWithRoleCols = `l.entity_id, COALESCE(e.name, ''), COALESCE(et.slug, ''),
        COALESCE(et.icon, ''), COALESCE(et.color, ''), l.participation_role`

// EntitiesForCalendar returns the DISTINCT entities tied to any event or era
// of the given calendar, guarded to that calendar's own campaign (c.id = ?
// joins in the calendar so e.campaign_id = c.campaign_id can never surface
// an entity tied in from another campaign). The link tables carry no
// calendar_id, so the calendar is reached through calendar_events.calendar_id
// / calendar_eras.calendar_id. DISTINCT collapses an entity tied via several
// events/eras to one row; participation_role is omitted (it's per-tie, not a
// single value at calendar scope). Gated by entityVisibilityFilter so a
// player can't learn a hidden entity's name just because it's tied to a
// calendar they can otherwise see.
func (r *eventRepo) EntitiesForCalendar(ctx context.Context, calendarID string, role int, userID string) ([]EntityTieRef, error) {
	visFilter, visArgs := entityVisibilityFilter(role, userID)
	args := append([]any{calendarID, calendarID, calendarID}, visArgs...)
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT `+entityTieCols+`
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
		 JOIN calendars c ON c.id = ?
		 JOIN entities e ON e.id = tied.entity_id AND e.campaign_id = c.campaign_id AND e.deleted_at IS NULL
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

// EntitiesForEvent returns every entity tied to an event with its display
// info, gated by entityVisibilityFilter and guarded to the event's own
// campaign (JOIN calendars c ... AND e.campaign_id = c.campaign_id), so a
// stale or hand-inserted cross-campaign tie can never surface here even
// though LinkEntityEvent itself no longer creates one. Ordered by entity name.
func (r *eventRepo) EntitiesForEvent(ctx context.Context, eventID string, role int, userID string) ([]EntityTieRef, error) {
	visFilter, visArgs := entityVisibilityFilter(role, userID)
	args := append([]any{eventID}, visArgs...)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+entityTieWithRoleCols+`
		 FROM entity_event_links l
		 JOIN calendar_events ev ON ev.id = l.event_id
		 JOIN calendars c ON c.id = ev.calendar_id
		 JOIN entities e ON e.id = l.entity_id AND e.campaign_id = c.campaign_id AND e.deleted_at IS NULL
		 LEFT JOIN entity_types et ON et.id = e.entity_type_id
		 WHERE l.event_id = ?`+visFilter+`
		 ORDER BY e.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntityTieRefs(rows)
}

// EntitiesForEra returns every entity tied to an era with its display info,
// gated by entityVisibilityFilter and campaign-guarded the same way
// EntitiesForEvent is.
func (r *eventRepo) EntitiesForEra(ctx context.Context, eraID int, role int, userID string) ([]EntityTieRef, error) {
	visFilter, visArgs := entityVisibilityFilter(role, userID)
	args := append([]any{eraID}, visArgs...)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+entityTieWithRoleCols+`
		 FROM entity_era_links l
		 JOIN calendar_eras er ON er.id = l.era_id
		 JOIN calendars c ON c.id = er.calendar_id
		 JOIN entities e ON e.id = l.entity_id AND e.campaign_id = c.campaign_id AND e.deleted_at IS NULL
		 LEFT JOIN entity_types et ON et.id = e.entity_type_id
		 WHERE l.era_id = ?`+visFilter+`
		 ORDER BY e.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntityTieRefs(rows)
}

// scanEntityTieRefs reads the shared entity-tie display projection. Role is
// a nullable column scanned through a sql.NullString.
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

// EventsForEntity returns every event tied to an entity within campaignID
// (with the tie role), role-filtered like every other event list so a
// dm_only event never reaches a player through its tie. campaignID is
// required because an entity id alone carries no campaign to filter by;
// c.campaign_id comes from eventJoins, which already joins calendars for its
// own display-field guard. Reuses eventCols/eventJoins so the embedded Event
// carries the same display fields the regular event lists do.
func (r *eventRepo) EventsForEntity(ctx context.Context, campaignID, entityID string, role int) ([]EntityEventTie, error) {
	visFilter := " AND e.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventCols+`, l.participation_role
		 FROM entity_event_links l
		 JOIN calendar_events e ON e.id = l.event_id
		 `+eventJoins+`
		 WHERE l.entity_id = ? AND c.campaign_id = ?`+visFilter+`
		 ORDER BY e.year, e.month, e.day, e.name`, entityID, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EntityEventTie
	for rows.Next() {
		var role string
		evt, err := scanEventRowWithExtra(rows, &role)
		if err != nil {
			return nil, err
		}
		out = append(out, EntityEventTie{Event: *evt, ParticipationRole: role})
	}
	return out, rows.Err()
}

// scanEventRowWithExtra scans an eventCols row plus one caller-supplied extra
// destination appended after it (here, l.participation_role), spreading the
// exact same eventDests(*Event) list scanEventRow itself scans so the two
// can never drift apart.
func scanEventRowWithExtra(rows *sql.Rows, extra *string) (*Event, error) {
	var evt Event
	if err := rows.Scan(append(eventDests(&evt), extra)...); err != nil {
		return nil, err
	}
	return &evt, nil
}

// ErasForEntity returns every era tied to an entity within campaignID (with
// the optional role), for the same reason EventsForEntity takes it.
func (r *eventRepo) ErasForEntity(ctx context.Context, campaignID, entityID string) ([]EntityEraTie, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eraColsQualified+`, l.participation_role
		 FROM entity_era_links l
		 JOIN calendar_eras er ON er.id = l.era_id
		 JOIN calendars c ON c.id = er.calendar_id
		 WHERE l.entity_id = ? AND c.campaign_id = ?
		 ORDER BY er.start_year, er.start_month, er.start_day, er.sort_order`, entityID, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EntityEraTie
	for rows.Next() {
		var role sql.NullString
		era, err := scanEraWithExtra(rows, &role)
		if err != nil {
			return nil, err
		}
		tie := EntityEraTie{Era: *era}
		if role.Valid && role.String != "" {
			s := role.String
			tie.ParticipationRole = &s
		}
		out = append(out, tie)
	}
	return out, rows.Err()
}

// scanEraWithExtra scans an eraColsQualified row plus one caller-supplied
// extra destination (here, l.participation_role), spreading the exact same
// eraDests(*Era) list scanEra itself scans so the two can never drift apart.
func scanEraWithExtra(rows *sql.Rows, extra *sql.NullString) (*Era, error) {
	var e Era
	if err := rows.Scan(append(eraDests(&e), extra)...); err != nil {
		return nil, err
	}
	return &e, nil
}
