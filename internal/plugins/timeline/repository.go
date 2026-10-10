package timeline

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// TimelineRepository defines persistence operations for timelines and related data.
type TimelineRepository interface {
	// Timeline CRUD.
	Create(ctx context.Context, t *Timeline) error
	GetByID(ctx context.Context, id string) (*Timeline, error)
	List(ctx context.Context, campaignID string, role int) ([]Timeline, error)
	ListByCalendar(ctx context.Context, calendarID string, role int) ([]Timeline, error)
	Update(ctx context.Context, t *Timeline) error
	Delete(ctx context.Context, id string) error

	// Search.
	Search(ctx context.Context, campaignID, query string, role int) ([]Timeline, error)

	// Event links.
	LinkEvent(ctx context.Context, link *EventLink) error
	UnlinkEvent(ctx context.Context, timelineID, eventID string) error
	ListEventLinks(ctx context.Context, timelineID string, role int) ([]EventLink, error)
	CountEvents(ctx context.Context, timelineID string) (int, error)

	// Event link visibility.
	UpdateEventLinkVisibility(ctx context.Context, timelineID, eventID string, visOverride, visRules patch.Field[string]) error

	// Standalone events.
	CreateEvent(ctx context.Context, e *TimelineEvent) error
	GetEvent(ctx context.Context, eventID string) (*TimelineEvent, error)
	UpdateEvent(ctx context.Context, e *TimelineEvent) error
	DeleteEvent(ctx context.Context, eventID string) error
	ListStandaloneEvents(ctx context.Context, timelineID string, role int) ([]TimelineEvent, error)
	CountStandaloneEvents(ctx context.Context, timelineID string) (int, error)

	// Entity groups.
	CreateEntityGroup(ctx context.Context, g *EntityGroup) error
	UpdateEntityGroup(ctx context.Context, g *EntityGroup) error
	DeleteEntityGroup(ctx context.Context, groupID int, timelineID string) error
	ListEntityGroups(ctx context.Context, timelineID string) ([]EntityGroup, error)
	AddGroupMember(ctx context.Context, groupID int, timelineID, entityID string) error
	RemoveGroupMember(ctx context.Context, groupID int, timelineID, entityID string) error

	// Event connections.
	CreateConnection(ctx context.Context, c *EventConnection) error
	DeleteConnection(ctx context.Context, connectionID int, timelineID string) error
	ListConnections(ctx context.Context, timelineID string) ([]EventConnection, error)
}

// timelineRepo is the MariaDB implementation of TimelineRepository.
type timelineRepo struct {
	db *sql.DB
}

// NewTimelineRepository creates a new MariaDB-backed timeline repository.
func NewTimelineRepository(db *sql.DB) TimelineRepository {
	return &timelineRepo{db: db}
}

// --- Timeline CRUD ---

// timelineCols is the column list for timeline queries.
const timelineCols = `t.id, t.campaign_id, t.calendar_id, t.name, t.description,
       t.description_html, t.color, t.icon, t.visibility, t.visibility_rules,
       t.sort_order, t.zoom_default, t.created_by, t.created_at, t.updated_at`

// Create inserts a new timeline.
func (r *timelineRepo) Create(ctx context.Context, t *Timeline) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO timelines (id, campaign_id, calendar_id, name, description,
		        description_html, color, icon, visibility, visibility_rules,
		        sort_order, zoom_default, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.CampaignID, t.CalendarID, t.Name, t.Description,
		t.DescriptionHTML, t.Color, t.Icon, t.Visibility, t.VisibilityRules,
		t.SortOrder, t.ZoomDefault, t.CreatedBy,
	)
	return err
}

// GetByID returns a single timeline by ID. CalendarName is left blank here
// (” — not scanned from a join): calendars/calendar_events belong to the
// calendar plugin (rule 8, no cross-plugin table reads from this repo), so
// the service layer fills it afterward via CalendarEventLinkLister
// (service.go's fillCalendarName) when a caller needs it displayed.
func (r *timelineRepo) GetByID(ctx context.Context, id string) (*Timeline, error) {
	t := &Timeline{}
	err := r.db.QueryRowContext(ctx,
		`SELECT `+timelineCols+`, ''
		 FROM timelines t
		 WHERE t.id = ?`, id,
	).Scan(
		&t.ID, &t.CampaignID, &t.CalendarID, &t.Name, &t.Description,
		&t.DescriptionHTML, &t.Color, &t.Icon, &t.Visibility, &t.VisibilityRules,
		&t.SortOrder, &t.ZoomDefault, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
		&t.CalendarName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get timeline by id: %w", err)
	}
	return t, nil
}

// List returns all timelines for a campaign, filtered by role-based visibility.
// Owners see dm_only timelines; others see only 'everyone'.
//
// EventCount here covers STANDALONE events only: the linked-event half
// (timeline_event_links -> calendar_events) can no longer be counted in SQL
// — calendar_events is not this plugin's table to JOIN (rule 8) — so it
// can't apply a dm_only predicate here either. The service layer restores
// the linked count, WITH its own visibility filter, for every viewer,
// Owner included (timelineService.recountEventsForViewer, which now always
// runs — see its doc comment): the two are restored together on purpose, so
// a viewer can never see a count that includes events their own list omits.
func (r *timelineRepo) List(ctx context.Context, campaignID string, role int) ([]Timeline, error) {
	visFilter := "AND t.visibility = 'everyone'"
	standaloneVisFilter := "AND te.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
		standaloneVisFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT `+timelineCols+`, '',
		       (SELECT COUNT(*) FROM timeline_events te WHERE te.timeline_id = t.id %s)
		FROM timelines t
		WHERE t.campaign_id = ? %s
		ORDER BY t.sort_order, t.name`, standaloneVisFilter, visFilter)

	rows, err := r.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list timelines: %w", err)
	}
	defer rows.Close()

	var result []Timeline
	for rows.Next() {
		t := Timeline{}
		if err := rows.Scan(
			&t.ID, &t.CampaignID, &t.CalendarID, &t.Name, &t.Description,
			&t.DescriptionHTML, &t.Color, &t.Icon, &t.Visibility, &t.VisibilityRules,
			&t.SortOrder, &t.ZoomDefault, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
			&t.CalendarName, &t.EventCount,
		); err != nil {
			return nil, fmt.Errorf("scan timeline: %w", err)
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// ListByCalendar returns the timelines bound to a specific calendar
// (timelines.calendar_id), role-filtered like List, and backs the
// cross-plugin "timelines for this calendar" read the Calendars dashboard
// consumes via a service interface.
//
// EventCount here covers standalone events only, exactly like List (see its
// doc comment) — the linked half is restored at the service layer instead,
// for every viewer, alongside the same visibility filter the event list uses.
func (r *timelineRepo) ListByCalendar(ctx context.Context, calendarID string, role int) ([]Timeline, error) {
	visFilter := "AND t.visibility = 'everyone'"
	standaloneVisFilter := "AND te.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
		standaloneVisFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT `+timelineCols+`, '',
		       (SELECT COUNT(*) FROM timeline_events te WHERE te.timeline_id = t.id %s)
		FROM timelines t
		WHERE t.calendar_id = ? %s
		ORDER BY t.sort_order, t.name`, standaloneVisFilter, visFilter)

	rows, err := r.db.QueryContext(ctx, query, calendarID)
	if err != nil {
		return nil, fmt.Errorf("list timelines by calendar: %w", err)
	}
	defer rows.Close()

	var result []Timeline
	for rows.Next() {
		t := Timeline{}
		if err := rows.Scan(
			&t.ID, &t.CampaignID, &t.CalendarID, &t.Name, &t.Description,
			&t.DescriptionHTML, &t.Color, &t.Icon, &t.Visibility, &t.VisibilityRules,
			&t.SortOrder, &t.ZoomDefault, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
			&t.CalendarName, &t.EventCount,
		); err != nil {
			return nil, fmt.Errorf("scan timeline: %w", err)
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// Update modifies an existing timeline.
func (r *timelineRepo) Update(ctx context.Context, t *Timeline) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE timelines SET name = ?, description = ?, description_html = ?,
		        color = ?, icon = ?, visibility = ?, visibility_rules = ?,
		        zoom_default = ?
		 WHERE id = ?`,
		t.Name, t.Description, t.DescriptionHTML,
		t.Color, t.Icon, t.Visibility, t.VisibilityRules,
		t.ZoomDefault, t.ID,
	)
	return err
}

// Delete removes a timeline and all associated data (cascaded by FK).
func (r *timelineRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM timelines WHERE id = ?`, id)
	return err
}

// --- Search ---

// Search returns timelines matching a name query, filtered by role-based
// (dm_only) visibility only — it cannot apply the per-user visibility_rules
// allow/deny list, which needs the row's parsed rules plus the caller's user
// id. The caller (service.go's SearchTimelines) MUST run the result through
// filterTimelinesByUser afterward, exactly as ListTimelines does for List.
// Do not hand-roll the allow/deny check into SQL: it is already implemented
// once, in canUserView, and ADR-058 is the rule against a second copy.
func (r *timelineRepo) Search(ctx context.Context, campaignID, query string, role int) ([]Timeline, error) {
	visFilter := "AND t.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	q := fmt.Sprintf(`
		SELECT `+timelineCols+`, ''
		FROM timelines t
		WHERE t.campaign_id = ? AND t.name LIKE ? %s
		ORDER BY t.name
		LIMIT 10`, visFilter)

	escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(query)
	rows, err := r.db.QueryContext(ctx, q, campaignID, "%"+escaped+"%")
	if err != nil {
		return nil, fmt.Errorf("search timelines: %w", err)
	}
	defer rows.Close()

	var result []Timeline
	for rows.Next() {
		t := Timeline{}
		if err := rows.Scan(
			&t.ID, &t.CampaignID, &t.CalendarID, &t.Name, &t.Description,
			&t.DescriptionHTML, &t.Color, &t.Icon, &t.Visibility, &t.VisibilityRules,
			&t.SortOrder, &t.ZoomDefault, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
			&t.CalendarName,
		); err != nil {
			return nil, fmt.Errorf("scan timeline: %w", err)
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// --- Event Links ---

// eventLinkOwnCols is timeline_event_links' own columns — no join to
// calendar_events (rule 8: that table belongs to the calendar plugin, not
// this repository). The Event* fields ListEventLinks used to join are left
// zero-valued here; the service layer fills them via CalendarEventLinkLister
// (service.go's enrichLinkedEvents) and drops any link whose event that
// lookup didn't return (not visible to the caller's role, or gone).
const eventLinkOwnCols = `id, timeline_id, event_id, display_order,
       visibility_override, visibility_rules, label, color_override, created_at`

// scanEventLinkOwn reads one timeline_event_links row (no calendar data).
func scanEventLinkOwn(scanner interface{ Scan(...any) error }) (*EventLink, error) {
	el := &EventLink{}
	err := scanner.Scan(
		&el.ID, &el.TimelineID, &el.EventID, &el.DisplayOrder,
		&el.VisibilityOverride, &el.VisibilityRules, &el.Label, &el.ColorOverride,
		&el.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return el, err
}

// ListEventLinks returns the timeline's OWN timeline_event_links rows — link
// metadata only (event id, display order, the link's own visibility
// override/rules, label, color override), not the calendar event data those
// ids point at, which the calendar plugin owns (rule 8). role is accepted
// but unused at this layer: the deleted `ce.visibility` filter needed
// calendar_events, so that half of the role gate now happens in the service
// layer instead, where CalendarEventLinkLister resolves each event through
// the calendar plugin's own role-filtered read. Every caller still passes
// role so a future SQL-only filter on this table's OWN columns (if one is
// ever needed) has nothing else to change.
//
// Ordered by display_order only — the caller's real ordering (by the linked
// event's in-world date) happens after enrichment, in
// service.go's sortEventLinks, since this repo has no date to sort by until
// then.
func (r *timelineRepo) ListEventLinks(ctx context.Context, timelineID string, role int) ([]EventLink, error) {
	_ = role
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventLinkOwnCols+`
		 FROM timeline_event_links
		 WHERE timeline_id = ?
		 ORDER BY display_order`, timelineID)
	if err != nil {
		return nil, fmt.Errorf("list event links: %w", err)
	}
	defer rows.Close()

	var result []EventLink
	for rows.Next() {
		el, err := scanEventLinkOwn(rows)
		if err != nil {
			return nil, fmt.Errorf("scan event link: %w", err)
		}
		result = append(result, *el)
	}
	return result, rows.Err()
}

// LinkEvent inserts a new event link.
func (r *timelineRepo) LinkEvent(ctx context.Context, link *EventLink) error {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO timeline_event_links (timeline_id, event_id, display_order, label, color_override)
		 VALUES (?, ?, ?, ?, ?)`,
		link.TimelineID, link.EventID, link.DisplayOrder, link.Label, link.ColorOverride,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting last insert id: %w", err)
	}
	link.ID = int(id)
	return nil
}

// UnlinkEvent removes an event link.
func (r *timelineRepo) UnlinkEvent(ctx context.Context, timelineID, eventID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM timeline_event_links WHERE timeline_id = ? AND event_id = ?`,
		timelineID, eventID,
	)
	return err
}

// CountEvents returns the number of events linked to a timeline.
func (r *timelineRepo) CountEvents(ctx context.Context, timelineID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM timeline_event_links WHERE timeline_id = ?`,
		timelineID,
	).Scan(&count)
	return count, err
}

// --- Event Link Visibility ---

// UpdateEventLinkVisibility writes only the visibility columns the caller
// named: an absent field is left out of the SET clause, an explicit null
// writes NULL, a value replaces. Writing both unconditionally is how a
// visibility-only flip used to erase the per-user rules.
func (r *timelineRepo) UpdateEventLinkVisibility(ctx context.Context, timelineID, eventID string, visOverride, visRules patch.Field[string]) error {
	var sets []string
	var args []any
	if visOverride.Present() {
		sets = append(sets, "visibility_override = ?")
		args = append(args, visOverride.Ptr(nil))
	}
	if visRules.Present() {
		sets = append(sets, "visibility_rules = ?")
		args = append(args, visRules.Ptr(nil))
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, timelineID, eventID)
	_, err := r.db.ExecContext(ctx,
		`UPDATE timeline_event_links SET `+strings.Join(sets, ", ")+`
		 WHERE timeline_id = ? AND event_id = ?`,
		args...,
	)
	return err
}

// --- Standalone Events ---

// standaloneEventCols is the column list for standalone event queries.
const standaloneEventCols = `te.id, te.timeline_id, te.entity_id, te.name, te.description,
       te.description_html, te.year, te.month, te.day,
       te.start_hour, te.start_minute, te.end_year, te.end_month, te.end_day,
       te.end_hour, te.end_minute, te.is_recurring, te.recurrence_type,
       te.category, te.visibility, te.visibility_rules, te.display_order, te.label, te.color,
       te.created_by, te.created_at, te.updated_at,
       COALESCE(ent.name, ''), COALESCE(et.icon, '')`

// standaloneEventJoins is the JOIN clause for standalone event queries.
const standaloneEventJoins = `LEFT JOIN entities ent ON ent.id = te.entity_id AND ent.deleted_at IS NULL
     LEFT JOIN entity_types et ON et.id = ent.entity_type_id`

// scanStandaloneEvent reads a row into a TimelineEvent struct.
func scanStandaloneEvent(scanner interface{ Scan(...any) error }) (*TimelineEvent, error) {
	e := &TimelineEvent{}
	err := scanner.Scan(
		&e.ID, &e.TimelineID, &e.EntityID, &e.Name, &e.Description,
		&e.DescriptionHTML, &e.Year, &e.Month, &e.Day,
		&e.StartHour, &e.StartMinute, &e.EndYear, &e.EndMonth, &e.EndDay,
		&e.EndHour, &e.EndMinute, &e.IsRecurring, &e.RecurrenceType,
		&e.Category, &e.Visibility, &e.VisibilityRules, &e.DisplayOrder, &e.Label, &e.Color,
		&e.CreatedBy, &e.CreatedAt, &e.UpdatedAt,
		&e.EntityName, &e.EntityIcon,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// CreateEvent inserts a new standalone timeline event.
func (r *timelineRepo) CreateEvent(ctx context.Context, e *TimelineEvent) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO timeline_events (id, timeline_id, entity_id, name, description,
		        description_html, year, month, day,
		        start_hour, start_minute, end_year, end_month, end_day,
		        end_hour, end_minute, is_recurring, recurrence_type,
		        category, visibility, visibility_rules, display_order, label, color, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.TimelineID, e.EntityID, e.Name, e.Description,
		e.DescriptionHTML, e.Year, e.Month, e.Day,
		e.StartHour, e.StartMinute, e.EndYear, e.EndMonth, e.EndDay,
		e.EndHour, e.EndMinute, e.IsRecurring, e.RecurrenceType,
		e.Category, e.Visibility, e.VisibilityRules, e.DisplayOrder, e.Label, e.Color, e.CreatedBy,
	)
	return err
}

// GetEvent returns a standalone event by ID with entity data joined.
func (r *timelineRepo) GetEvent(ctx context.Context, eventID string) (*TimelineEvent, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+standaloneEventCols+`
		 FROM timeline_events te
		 `+standaloneEventJoins+`
		 WHERE te.id = ?`, eventID,
	)
	return scanStandaloneEvent(row)
}

// UpdateEvent modifies an existing standalone timeline event.
func (r *timelineRepo) UpdateEvent(ctx context.Context, e *TimelineEvent) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE timeline_events SET entity_id = ?, name = ?, description = ?,
		        description_html = ?, year = ?, month = ?, day = ?,
		        start_hour = ?, start_minute = ?, end_year = ?, end_month = ?, end_day = ?,
		        end_hour = ?, end_minute = ?, is_recurring = ?, recurrence_type = ?,
		        category = ?, visibility = ?, visibility_rules = ?, display_order = ?, label = ?, color = ?
		 WHERE id = ?`,
		e.EntityID, e.Name, e.Description,
		e.DescriptionHTML, e.Year, e.Month, e.Day,
		e.StartHour, e.StartMinute, e.EndYear, e.EndMonth, e.EndDay,
		e.EndHour, e.EndMinute, e.IsRecurring, e.RecurrenceType,
		e.Category, e.Visibility, e.VisibilityRules, e.DisplayOrder, e.Label, e.Color,
		e.ID,
	)
	return err
}

// DeleteEvent removes a standalone timeline event.
func (r *timelineRepo) DeleteEvent(ctx context.Context, eventID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM timeline_events WHERE id = ?`, eventID)
	return err
}

// ListStandaloneEvents returns all standalone events for a timeline, filtered by role.
func (r *timelineRepo) ListStandaloneEvents(ctx context.Context, timelineID string, role int) ([]TimelineEvent, error) {
	visFilter := "AND te.visibility = 'everyone'"
	if permissions.CanSeeDmOnly(role) {
		visFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT `+standaloneEventCols+`
		FROM timeline_events te
		`+standaloneEventJoins+`
		WHERE te.timeline_id = ? %s
		ORDER BY te.year, te.month, te.day, te.display_order`, visFilter)

	rows, err := r.db.QueryContext(ctx, query, timelineID)
	if err != nil {
		return nil, fmt.Errorf("list standalone events: %w", err)
	}
	defer rows.Close()

	var result []TimelineEvent
	for rows.Next() {
		e, err := scanStandaloneEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan standalone event: %w", err)
		}
		result = append(result, *e)
	}
	return result, rows.Err()
}

// CountStandaloneEvents returns the number of standalone events on a timeline.
func (r *timelineRepo) CountStandaloneEvents(ctx context.Context, timelineID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM timeline_events WHERE timeline_id = ?`,
		timelineID,
	).Scan(&count)
	return count, err
}

// --- Entity Groups ---

// CreateEntityGroup inserts a new entity group.
func (r *timelineRepo) CreateEntityGroup(ctx context.Context, g *EntityGroup) error {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO timeline_entity_groups (timeline_id, name, color, sort_order)
		 VALUES (?, ?, ?, ?)`,
		g.TimelineID, g.Name, g.Color, g.SortOrder,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting last insert id: %w", err)
	}
	g.ID = int(id)
	return nil
}

// UpdateEntityGroup modifies an existing entity group.
func (r *timelineRepo) UpdateEntityGroup(ctx context.Context, g *EntityGroup) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE timeline_entity_groups SET name = ?, color = ?, sort_order = ?
		 WHERE id = ? AND timeline_id = ?`,
		g.Name, g.Color, g.SortOrder, g.ID, g.TimelineID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("entity group not found")
	}
	return nil
}

// DeleteEntityGroup removes an entity group and its members (cascaded by FK).
// timelineID scoping prevents cross-timeline IDOR.
func (r *timelineRepo) DeleteEntityGroup(ctx context.Context, groupID int, timelineID string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM timeline_entity_groups WHERE id = ? AND timeline_id = ?`, groupID, timelineID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("entity group not found")
	}
	return nil
}

// ListEntityGroups returns all entity groups for a timeline with members loaded.
func (r *timelineRepo) ListEntityGroups(ctx context.Context, timelineID string) ([]EntityGroup, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, timeline_id, name, color, sort_order
		 FROM timeline_entity_groups
		 WHERE timeline_id = ?
		 ORDER BY sort_order, name`,
		timelineID,
	)
	if err != nil {
		return nil, fmt.Errorf("list entity groups: %w", err)
	}
	defer rows.Close()

	var groups []EntityGroup
	for rows.Next() {
		g := EntityGroup{}
		if err := rows.Scan(&g.ID, &g.TimelineID, &g.Name, &g.Color, &g.SortOrder); err != nil {
			return nil, fmt.Errorf("scan entity group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Eager-load members for all groups.
	for i := range groups {
		members, err := r.listGroupMembers(ctx, groups[i].ID)
		if err != nil {
			return nil, fmt.Errorf("list group members for %d: %w", groups[i].ID, err)
		}
		groups[i].Members = members
	}
	return groups, nil
}

// listGroupMembers returns all members of an entity group with entity display data.
func (r *timelineRepo) listGroupMembers(ctx context.Context, groupID int) ([]EntityGroupMember, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT m.id, m.group_id, m.entity_id,
		        COALESCE(e.name, ''), COALESCE(et.icon, '')
		 FROM timeline_entity_group_members m
		 LEFT JOIN entities e ON e.id = m.entity_id AND e.deleted_at IS NULL
		 LEFT JOIN entity_types et ON et.id = e.entity_type_id
		 WHERE m.group_id = ?
		 ORDER BY e.name`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []EntityGroupMember
	for rows.Next() {
		m := EntityGroupMember{}
		if err := rows.Scan(&m.ID, &m.GroupID, &m.EntityID, &m.EntityName, &m.EntityIcon); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// AddGroupMember adds an entity to an entity group.
// Verifies the group belongs to the given timeline to prevent cross-timeline IDOR.
func (r *timelineRepo) AddGroupMember(ctx context.Context, groupID int, timelineID, entityID string) error {
	// Verify group belongs to this timeline before inserting.
	var exists int
	if err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM timeline_entity_groups WHERE id = ? AND timeline_id = ?`,
		groupID, timelineID,
	).Scan(&exists); err != nil {
		return apperror.NewNotFound("entity group not found")
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO timeline_entity_group_members (group_id, entity_id) VALUES (?, ?)`,
		groupID, entityID,
	)
	return err
}

// RemoveGroupMember removes an entity from an entity group.
// Verifies the group belongs to the given timeline to prevent cross-timeline IDOR.
func (r *timelineRepo) RemoveGroupMember(ctx context.Context, groupID int, timelineID, entityID string) error {
	// Verify group belongs to this timeline before deleting.
	var exists int
	if err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM timeline_entity_groups WHERE id = ? AND timeline_id = ?`,
		groupID, timelineID,
	).Scan(&exists); err != nil {
		return apperror.NewNotFound("entity group not found")
	}
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM timeline_entity_group_members WHERE group_id = ? AND entity_id = ?`,
		groupID, entityID,
	)
	return err
}

// --- Event Connections ---

// CreateConnection inserts a new event connection.
func (r *timelineRepo) CreateConnection(ctx context.Context, c *EventConnection) error {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO timeline_event_connections
			(timeline_id, source_id, target_id, source_type, target_type, label, color, style)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.TimelineID, c.SourceID, c.TargetID, c.SourceType, c.TargetType,
		c.Label, c.Color, c.Style,
	)
	if err != nil {
		return fmt.Errorf("create connection: %w", err)
	}
	id, _ := result.LastInsertId()
	c.ID = int(id)
	return nil
}

// DeleteConnection removes an event connection.
// Verifies the connection belongs to the given timeline to prevent IDOR.
func (r *timelineRepo) DeleteConnection(ctx context.Context, connectionID int, timelineID string) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM timeline_event_connections WHERE id = ? AND timeline_id = ?`,
		connectionID, timelineID,
	)
	if err != nil {
		return fmt.Errorf("delete connection: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("connection not found")
	}
	return nil
}

// ListConnections returns all event connections for a timeline.
func (r *timelineRepo) ListConnections(ctx context.Context, timelineID string) ([]EventConnection, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, timeline_id, source_id, target_id, source_type, target_type,
				label, color, style, created_at
		 FROM timeline_event_connections
		 WHERE timeline_id = ?
		 ORDER BY created_at`,
		timelineID,
	)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var connections []EventConnection
	for rows.Next() {
		var c EventConnection
		if err := rows.Scan(
			&c.ID, &c.TimelineID, &c.SourceID, &c.TargetID,
			&c.SourceType, &c.TargetType,
			&c.Label, &c.Color, &c.Style, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan connection: %w", err)
		}
		connections = append(connections, c)
	}
	return connections, rows.Err()
}
