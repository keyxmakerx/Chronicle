// Package timeline provides interactive visual timelines for campaigns.
// Each campaign can have multiple timelines that reference a calendar and
// display calendar events in a zoomable, draggable D3.js visualization.
// Events are linked (not copied) from the calendar, preserving a single
// source of truth. Entity groups organize events into swim-lanes.
package timeline

import (
	"encoding/json"
	"time"
)

// Zoom level constants for the timeline visualization.
const (
	ZoomEra     = "era"
	ZoomCentury = "century"
	ZoomDecade  = "decade"
	ZoomYear    = "year"
	ZoomMonth   = "month"
	ZoomDay     = "day"
)

// ValidZoomLevels is the ordered set of zoom levels from widest to narrowest.
var ValidZoomLevels = []string{ZoomEra, ZoomCentury, ZoomDecade, ZoomYear, ZoomMonth, ZoomDay}

// IsValidZoom returns true if the given string is a recognized zoom level.
func IsValidZoom(z string) bool {
	for _, v := range ValidZoomLevels {
		if v == z {
			return true
		}
	}
	return false
}

// Timeline is a named visual timeline within a campaign. It optionally
// references a calendar for date context and linked calendar events.
// When CalendarID is nil, the timeline only contains standalone events.
type Timeline struct {
	ID              string    `json:"id"`
	CampaignID      string    `json:"campaign_id"`
	CalendarID      *string   `json:"calendar_id,omitempty"`
	Name            string    `json:"name"`
	Description     *string   `json:"description,omitempty"`
	DescriptionHTML *string   `json:"description_html,omitempty"`
	Color           string    `json:"color"`
	Icon            string    `json:"icon"`
	Visibility      string    `json:"visibility"`
	VisibilityRules *string   `json:"visibility_rules,omitempty"`
	SortOrder       int       `json:"sort_order"`
	ZoomDefault     string    `json:"zoom_default"`
	CreatedBy       *string   `json:"created_by,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// Joined fields (populated by some queries).
	CalendarName string `json:"calendar_name,omitempty"`
	EventCount   int    `json:"event_count,omitempty"`
}

// GetCampaignID returns the campaign this timeline belongs to. Implements
// middleware.CampaignScoped for generic IDOR protection.
func (t *Timeline) GetCampaignID() string { return t.CampaignID }

// IsDMOnly returns true if this timeline is only visible to the DM.
func (t *Timeline) IsDMOnly() bool {
	return t.Visibility == "dm_only"
}

// HasCalendar returns true if this timeline is linked to a calendar.
func (t *Timeline) HasCalendar() bool {
	return t.CalendarID != nil && *t.CalendarID != ""
}

// ParseVisibilityRules parses the JSON visibility rules into a VisibilityRules struct.
// Returns nil if no rules are set.
func (t *Timeline) ParseVisibilityRules() *VisibilityRules {
	if t.VisibilityRules == nil || *t.VisibilityRules == "" {
		return nil
	}
	var rules VisibilityRules
	if err := json.Unmarshal([]byte(*t.VisibilityRules), &rules); err != nil {
		return nil
	}
	return &rules
}

// VisibilityRules defines per-user visibility overrides for timelines and events.
// If AllowedUsers is set, only those users can see the item (whitelist).
// If DeniedUsers is set, those users cannot see the item (blacklist).
// AllowedUsers takes precedence: if set, DeniedUsers is ignored.
type VisibilityRules struct {
	AllowedUsers []string `json:"allowed_users,omitempty"`
	DeniedUsers  []string `json:"denied_users,omitempty"`
}

// EventLink is the join between a timeline and a calendar event, with
// per-link overrides for display properties and visibility. Also used as the
// unified event view when merging linked calendar events with standalone events.
// Source distinguishes the origin: "calendar" or "standalone".
type EventLink struct {
	ID                 int       `json:"id"`
	TimelineID         string    `json:"timeline_id"`
	EventID            string    `json:"event_id"`
	Source             string    `json:"source"`
	DisplayOrder       int       `json:"display_order"`
	VisibilityOverride *string   `json:"visibility_override,omitempty"`
	VisibilityRules    *string   `json:"visibility_rules,omitempty"`
	Label              *string   `json:"label,omitempty"`
	ColorOverride      *string   `json:"color_override,omitempty"`
	CreatedAt          time.Time `json:"created_at"`

	// Calendar event fields (joined from calendar_events).
	EventName        string  `json:"event_name,omitempty"`
	EventDescription *string `json:"event_description,omitempty"`
	EventYear        int     `json:"event_year"`
	EventMonth       int     `json:"event_month"`
	EventDay         int     `json:"event_day"`
	EventEndYear     *int    `json:"event_end_year,omitempty"`
	EventEndMonth    *int    `json:"event_end_month,omitempty"`
	EventEndDay      *int    `json:"event_end_day,omitempty"`
	EventCategory    *string `json:"event_category,omitempty"`
	EventVisibility  string  `json:"event_visibility,omitempty"`
	EventEntityID    *string `json:"event_entity_id,omitempty"`
	EventEntityName  string  `json:"event_entity_name,omitempty"`
	EventEntityIcon  string  `json:"event_entity_icon,omitempty"`
}

// ParseVisibilityRules parses the JSON visibility rules on an event link.
// Returns nil if no rules are set.
func (el *EventLink) ParseVisibilityRules() *VisibilityRules {
	if el.VisibilityRules == nil || *el.VisibilityRules == "" {
		return nil
	}
	var rules VisibilityRules
	if err := json.Unmarshal([]byte(*el.VisibilityRules), &rules); err != nil {
		return nil
	}
	return &rules
}

// EffectiveVisibility returns the visibility override if set, otherwise
// falls back to the original calendar event's visibility.
func (el *EventLink) EffectiveVisibility() string {
	if el.VisibilityOverride != nil && *el.VisibilityOverride != "" {
		return *el.VisibilityOverride
	}
	return el.EventVisibility
}

// EffectiveLabel returns the display label — the override if set, otherwise
// the original calendar event name.
func (el *EventLink) EffectiveLabel() string {
	if el.Label != nil && *el.Label != "" {
		return *el.Label
	}
	return el.EventName
}

// EffectiveColor returns the display color — the override if set, otherwise
// empty string (use default).
func (el *EventLink) EffectiveColor() string {
	if el.ColorOverride != nil && *el.ColorOverride != "" {
		return *el.ColorOverride
	}
	return ""
}

// EntityGroup is a named group of entities used for swim-lane organization
// on the timeline visualization.
type EntityGroup struct {
	ID         int    `json:"id"`
	TimelineID string `json:"timeline_id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	SortOrder  int    `json:"sort_order"`

	// Eager-loaded members (populated by some queries).
	Members []EntityGroupMember `json:"members,omitempty"`
}

// EntityGroupMember links an entity to an entity group.
type EntityGroupMember struct {
	ID       int    `json:"id"`
	GroupID  int    `json:"group_id"`
	EntityID string `json:"entity_id"`

	// Joined fields.
	EntityName string `json:"entity_name,omitempty"`
	EntityIcon string `json:"entity_icon,omitempty"`
}

// --- Request DTOs ---

// CreateTimelineInput is the validated input for creating a timeline.
type CreateTimelineInput struct {
	CampaignID  string
	CalendarID  *string
	Name        string
	Description *string
	Color       string
	Icon        string
	Visibility  string
	ZoomDefault string
	CreatedBy   string
}

// UpdateTimelineInput is the validated input for updating timeline settings.
type UpdateTimelineInput struct {
	Name            string
	Description     *string
	DescriptionHTML *string
	Color           string
	Icon            string
	Visibility      string
	VisibilityRules *string
	ZoomDefault     string
}

// LinkEventInput is the validated input for linking a calendar event to a timeline.
type LinkEventInput struct {
	Label         *string
	ColorOverride *string
}

// CreateEntityGroupInput is the validated input for creating an entity group.
type CreateEntityGroupInput struct {
	Name  string
	Color string
}

// UpdateEntityGroupInput is the validated input for updating an entity group.
type UpdateEntityGroupInput struct {
	Name  string
	Color string
}

// UpdateEventVisibilityInput is the validated input for updating event link visibility.
type UpdateEventVisibilityInput struct {
	VisibilityOverride *string
	VisibilityRules    *string
}

// --- Standalone Events ---

// TimelineEvent is a standalone event that belongs directly to a timeline
// without requiring a calendar. Provides full parity with calendar events
// (multi-day spans, times, recurrence) while being owned 1:N by a timeline.
type TimelineEvent struct {
	ID              string    `json:"id"`
	TimelineID      string    `json:"timeline_id"`
	EntityID        *string   `json:"entity_id,omitempty"`
	Name            string    `json:"name"`
	Description     *string   `json:"description,omitempty"`
	DescriptionHTML *string   `json:"description_html,omitempty"`
	Year            int       `json:"year"`
	Month           int       `json:"month"`
	Day             int       `json:"day"`
	StartHour       *int      `json:"start_hour,omitempty"`
	StartMinute     *int      `json:"start_minute,omitempty"`
	EndYear         *int      `json:"end_year,omitempty"`
	EndMonth        *int      `json:"end_month,omitempty"`
	EndDay          *int      `json:"end_day,omitempty"`
	EndHour         *int      `json:"end_hour,omitempty"`
	EndMinute       *int      `json:"end_minute,omitempty"`
	IsRecurring     bool      `json:"is_recurring"`
	RecurrenceType  *string   `json:"recurrence_type,omitempty"`
	Category        *string   `json:"category,omitempty"`
	Visibility      string    `json:"visibility"`
	VisibilityRules *string   `json:"visibility_rules,omitempty"`
	DisplayOrder    int       `json:"display_order"`
	Label           *string   `json:"label,omitempty"`
	Color           *string   `json:"color,omitempty"`
	CreatedBy       *string   `json:"created_by,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// Joined fields (populated by queries that LEFT JOIN entities).
	EntityName string `json:"entity_name,omitempty"`
	EntityIcon string `json:"entity_icon,omitempty"`
}

// CreateTimelineEventInput is the validated input for creating a standalone event.
type CreateTimelineEventInput struct {
	Name            string
	Description     *string
	DescriptionHTML *string
	EntityID        *string
	Year            int
	Month           int
	Day             int
	StartHour       *int
	StartMinute     *int
	EndYear         *int
	EndMonth        *int
	EndDay          *int
	EndHour         *int
	EndMinute       *int
	IsRecurring     bool
	RecurrenceType  *string
	Category        *string
	Visibility      string
	Label           *string
	Color           *string
	CreatedBy       string
}

// UpdateTimelineEventInput is the validated input for updating a standalone event.
type UpdateTimelineEventInput struct {
	Name            string
	Description     *string
	DescriptionHTML *string
	EntityID        *string
	Year            int
	Month           int
	Day             int
	StartHour       *int
	StartMinute     *int
	EndYear         *int
	EndMonth        *int
	EndDay          *int
	EndHour         *int
	EndMinute       *int
	IsRecurring     bool
	RecurrenceType  *string
	Category        *string
	Visibility      string
	VisibilityRules *string
	Label           *string
	Color           *string
}

// ToEventLink converts a standalone TimelineEvent to an EventLink for
// unified rendering in the D3 visualization and event list.
func (e *TimelineEvent) ToEventLink() EventLink {
	return EventLink{
		TimelineID:       e.TimelineID,
		EventID:          e.ID,
		Source:           "standalone",
		DisplayOrder:     e.DisplayOrder,
		VisibilityRules:  e.VisibilityRules,
		Label:            e.Label,
		ColorOverride:    e.Color,
		CreatedAt:        e.CreatedAt,
		EventName:        e.Name,
		EventDescription: e.Description,
		EventYear:        e.Year,
		EventMonth:       e.Month,
		EventDay:         e.Day,
		EventEndYear:     e.EndYear,
		EventEndMonth:    e.EndMonth,
		EventEndDay:      e.EndDay,
		EventCategory:    e.Category,
		EventVisibility:  e.Visibility,
		EventEntityID:    e.EntityID,
		EventEntityName:  e.EntityName,
		EventEntityIcon:  e.EntityIcon,
	}
}

// --- View Data ---

// TimelineListData holds all data needed to render the timeline list page.
type TimelineListData struct {
	CampaignID string
	Timelines  []Timeline
	IsOwner    bool
	IsScribe   bool
	CSRFToken  string
}

// TimelineViewData holds all data needed to render a single timeline page.
type TimelineViewData struct {
	CampaignID   string
	Timeline     *Timeline
	Events       []EventLink
	EntityGroups []EntityGroup
	IsOwner      bool
	IsScribe     bool
	CSRFToken    string
	Members      []MemberRef
}

// MemberRef is a lightweight reference to a campaign member for the
// visibility rules user selector.
type MemberRef struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}
