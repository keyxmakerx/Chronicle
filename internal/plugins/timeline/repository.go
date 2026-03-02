package timeline

import (
	"context"
	"database/sql"
	"fmt"
)

// TimelineRepository defines persistence operations for timelines and related data.
type TimelineRepository interface {
	// Timeline CRUD.
	Create(ctx context.Context, t *Timeline) error
	GetByID(ctx context.Context, id string) (*Timeline, error)
	List(ctx context.Context, campaignID string, role int) ([]Timeline, error)
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
	UpdateEventLinkVisibility(ctx context.Context, timelineID, eventID string, visOverride *string, visRules *string) error

	// Entity groups.
	CreateEntityGroup(ctx context.Context, g *EntityGroup) error
	UpdateEntityGroup(ctx context.Context, g *EntityGroup) error
	DeleteEntityGroup(ctx context.Context, groupID int) error
	ListEntityGroups(ctx context.Context, timelineID string) ([]EntityGroup, error)
	AddGroupMember(ctx context.Context, groupID int, entityID string) error
	RemoveGroupMember(ctx context.Context, groupID int, entityID string) error
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

// scanTimeline reads a row into a Timeline struct.
func scanTimeline(scanner interface{ Scan(...any) error }) (*Timeline, error) {
	t := &Timeline{}
	err := scanner.Scan(
		&t.ID, &t.CampaignID, &t.CalendarID, &t.Name, &t.Description,
		&t.DescriptionHTML, &t.Color, &t.Icon, &t.Visibility, &t.VisibilityRules,
		&t.SortOrder, &t.ZoomDefault, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

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

// GetByID returns a single timeline by ID with calendar name joined.
func (r *timelineRepo) GetByID(ctx context.Context, id string) (*Timeline, error) {
	t := &Timeline{}
	err := r.db.QueryRowContext(ctx,
		`SELECT `+timelineCols+`, COALESCE(c.name, '')
		 FROM timelines t
		 LEFT JOIN calendars c ON c.id = t.calendar_id
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
// role >= 3 (Owner/Scribe) sees dm_only timelines; players see only 'everyone'.
func (r *timelineRepo) List(ctx context.Context, campaignID string, role int) ([]Timeline, error) {
	visFilter := "AND t.visibility = 'everyone'"
	if role >= 2 {
		visFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT `+timelineCols+`, COALESCE(c.name, ''),
		       (SELECT COUNT(*) FROM timeline_event_links tel WHERE tel.timeline_id = t.id)
		FROM timelines t
		LEFT JOIN calendars c ON c.id = t.calendar_id
		WHERE t.campaign_id = ? %s
		ORDER BY t.sort_order, t.name`, visFilter)

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

// Search returns timelines matching a name query, filtered by role-based visibility.
func (r *timelineRepo) Search(ctx context.Context, campaignID, query string, role int) ([]Timeline, error) {
	visFilter := "AND t.visibility = 'everyone'"
	if role >= 2 {
		visFilter = ""
	}

	q := fmt.Sprintf(`
		SELECT `+timelineCols+`, COALESCE(c.name, '')
		FROM timelines t
		LEFT JOIN calendars c ON c.id = t.calendar_id
		WHERE t.campaign_id = ? AND t.name LIKE ? %s
		ORDER BY t.name
		LIMIT 10`, visFilter)

	rows, err := r.db.QueryContext(ctx, q, campaignID, "%"+query+"%")
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

// eventLinkCols is the column list for event link queries with joined calendar event data.
const eventLinkCols = `tel.id, tel.timeline_id, tel.event_id, tel.display_order,
       tel.visibility_override, tel.visibility_rules, tel.label, tel.color_override,
       tel.created_at,
       ce.name, ce.description, ce.year, ce.month, ce.day,
       ce.category, ce.visibility, ce.entity_id,
       COALESCE(ent.name, ''), COALESCE(et.icon, '')`

// eventLinkJoins is the JOIN clause for event link queries.
const eventLinkJoins = `JOIN calendar_events ce ON ce.id = tel.event_id
     LEFT JOIN entities ent ON ent.id = ce.entity_id
     LEFT JOIN entity_types et ON et.id = ent.entity_type_id`

// scanEventLink reads a row into an EventLink struct.
func scanEventLink(scanner interface{ Scan(...any) error }) (*EventLink, error) {
	el := &EventLink{}
	err := scanner.Scan(
		&el.ID, &el.TimelineID, &el.EventID, &el.DisplayOrder,
		&el.VisibilityOverride, &el.VisibilityRules, &el.Label, &el.ColorOverride,
		&el.CreatedAt,
		&el.EventName, &el.EventDescription, &el.EventYear, &el.EventMonth, &el.EventDay,
		&el.EventCategory, &el.EventVisibility, &el.EventEntityID,
		&el.EventEntityName, &el.EventEntityIcon,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return el, err
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

// ListEventLinks returns all event links for a timeline with calendar event data joined.
// Filtered by role-based visibility on the underlying calendar event.
func (r *timelineRepo) ListEventLinks(ctx context.Context, timelineID string, role int) ([]EventLink, error) {
	visFilter := "AND ce.visibility = 'everyone'"
	if role >= 2 {
		visFilter = ""
	}

	query := fmt.Sprintf(`
		SELECT `+eventLinkCols+`
		FROM timeline_event_links tel
		`+eventLinkJoins+`
		WHERE tel.timeline_id = ? %s
		ORDER BY ce.year, ce.month, ce.day, tel.display_order`, visFilter)

	rows, err := r.db.QueryContext(ctx, query, timelineID)
	if err != nil {
		return nil, fmt.Errorf("list event links: %w", err)
	}
	defer rows.Close()

	var result []EventLink
	for rows.Next() {
		el, err := scanEventLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan event link: %w", err)
		}
		result = append(result, *el)
	}
	return result, rows.Err()
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

// UpdateEventLinkVisibility sets the visibility override and rules on an event link.
func (r *timelineRepo) UpdateEventLinkVisibility(ctx context.Context, timelineID, eventID string, visOverride *string, visRules *string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE timeline_event_links SET visibility_override = ?, visibility_rules = ?
		 WHERE timeline_id = ? AND event_id = ?`,
		visOverride, visRules, timelineID, eventID,
	)
	return err
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
	_, err := r.db.ExecContext(ctx,
		`UPDATE timeline_entity_groups SET name = ?, color = ?, sort_order = ?
		 WHERE id = ?`,
		g.Name, g.Color, g.SortOrder, g.ID,
	)
	return err
}

// DeleteEntityGroup removes an entity group and its members (cascaded by FK).
func (r *timelineRepo) DeleteEntityGroup(ctx context.Context, groupID int) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM timeline_entity_groups WHERE id = ?`, groupID)
	return err
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
		 LEFT JOIN entities e ON e.id = m.entity_id
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
func (r *timelineRepo) AddGroupMember(ctx context.Context, groupID int, entityID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO timeline_entity_group_members (group_id, entity_id) VALUES (?, ?)`,
		groupID, entityID,
	)
	return err
}

// RemoveGroupMember removes an entity from an entity group.
func (r *timelineRepo) RemoveGroupMember(ctx context.Context, groupID int, entityID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM timeline_entity_group_members WHERE group_id = ? AND entity_id = ?`,
		groupID, entityID,
	)
	return err
}
