// Package audit provides an audit log plugin that records user actions within
// campaigns. Every significant mutation (entity CRUD, membership changes,
// campaign updates) is captured as an AuditEntry and persisted to the
// audit_log table. The activity feed gives campaign owners visibility into
// who changed what and when.
//
// This is an optional plugin -- it does not modify campaign data, only records
// observations about changes made by other plugins.
package audit

import "time"

// --- Action Constants ---
// Each action string follows the pattern "resource.verb" for consistent
// filtering and display grouping.

const (
	// ActionEntityCreated is logged when a new entity is added to a campaign.
	ActionEntityCreated = "entity.created"

	// ActionEntityUpdated is logged when an entity's content or fields change.
	ActionEntityUpdated = "entity.updated"

	// ActionEntityDeleted is logged when an entity is removed from a campaign.
	ActionEntityDeleted = "entity.deleted"

	// ActionMemberJoined is logged when a user is added to a campaign.
	ActionMemberJoined = "member.joined"

	// ActionMemberLeft is logged when a user leaves or is removed from a campaign.
	ActionMemberLeft = "member.left"

	// ActionMemberRoleChanged is logged when a member's role is updated.
	ActionMemberRoleChanged = "member.role_changed"

	// ActionCampaignUpdated is logged when campaign settings are modified.
	ActionCampaignUpdated = "campaign.updated"

	// ActionEntityTypeCreated is logged when a new entity type is added to a campaign.
	ActionEntityTypeCreated = "entity_type.created"

	// ActionEntityTypeUpdated is logged when an entity type is modified.
	ActionEntityTypeUpdated = "entity_type.updated"

	// ActionEntityTypeDeleted is logged when an entity type is removed from a campaign.
	ActionEntityTypeDeleted = "entity_type.deleted"

	// ActionTagCreated is logged when a new tag is added to a campaign.
	ActionTagCreated = "tag.created"

	// ActionTagDeleted is logged when a tag is removed from a campaign.
	ActionTagDeleted = "tag.deleted"

	// --- Calendar plugin (V2 Wave 0 PR 4 / C-CAL-V2-AUDIT-LOG-INTEGRATION) ---
	// Naming: snake_case verb suffix after the resource. Counts-only
	// payload discipline — bulk Set* events log before/after counts in
	// Details, not full arrays, to keep audit_log row size bounded.

	ActionCalendarCreated            = "calendar.created"
	ActionCalendarUpdated            = "calendar.updated"
	ActionCalendarDeleted            = "calendar.deleted"
	ActionCalendarDefaultChanged     = "calendar.default_changed"
	ActionCalendarMonthsSet          = "calendar.months_set"
	ActionCalendarWeekdaysSet        = "calendar.weekdays_set"
	ActionCalendarMoonsSet           = "calendar.moons_set"
	ActionCalendarSeasonsSet         = "calendar.seasons_set"
	ActionCalendarErasSet            = "calendar.eras_set"
	ActionCalendarCategoriesSet      = "calendar.categories_set"
	ActionCalendarEraCreated         = "calendar.era_created"
	ActionCalendarEraUpdated         = "calendar.era_updated"
	ActionCalendarEraDeleted         = "calendar.era_deleted"
	ActionCalendarWeatherSet         = "calendar.weather_set"
	ActionCalendarCyclesSet          = "calendar.cycles_set"
	ActionCalendarFestivalsSet       = "calendar.festivals_set"
	ActionCalendarWeatherZonesSet    = "calendar.weather_zones_set"
	// ActionCalendarWeatherActiveZoneChanged covers SetActiveWeatherZone
	// (added in PR #360 alongside SetWeatherZones; refresh per coordinator
	// commit 672ef9e expanded the dispatch to log this separately).
	ActionCalendarWeatherActiveZoneChanged = "calendar.weather_active_zone_changed"
	ActionCalendarEventCreated             = "calendar.event_created"
	ActionCalendarEventUpdated             = "calendar.event_updated"
	ActionCalendarEventDeleted             = "calendar.event_deleted"
	ActionCalendarEventVisibilityChanged   = "calendar.event_visibility_changed"
	ActionCalendarDateAdvanced             = "calendar.date_advanced"
	ActionCalendarTimeAdvanced             = "calendar.time_advanced"
	ActionCalendarDateSet                  = "calendar.date_set"
	ActionCalendarImported                 = "calendar.imported"
	// Note: tier definitions are emitted by the campaigns plugin
	// (`campaign.event_tier_definitions.updated`) since the data lives in
	// `campaigns.settings`, not in a calendar table. Wave 0 PR 2 added
	// that emission at campaigns.UpdateEventTierDefinitionsAPI; the
	// dispatch's `calendar.tier_definitions_set` label is reconciled with
	// the campaign-plugin emission already in place.

	// --- Timeline plugin (V2 Wave 0 PR 4) ---

	ActionTimelineCreated                  = "timeline.created"
	ActionTimelineUpdated                  = "timeline.updated"
	ActionTimelineDeleted                  = "timeline.deleted"
	ActionTimelineEventLinked              = "timeline.event_linked"
	ActionTimelineEventUnlinked            = "timeline.event_unlinked"
	ActionTimelineStandaloneEventCreated   = "timeline.standalone_event_created"
	ActionTimelineStandaloneEventUpdated   = "timeline.standalone_event_updated"
	ActionTimelineStandaloneEventDeleted   = "timeline.standalone_event_deleted"
	ActionTimelineEntityGroupCreated       = "timeline.entity_group_created"
	ActionTimelineEntityGroupUpdated       = "timeline.entity_group_updated"
	ActionTimelineEntityGroupDeleted       = "timeline.entity_group_deleted"
	ActionTimelineEntityGroupMemberAdded   = "timeline.entity_group_member_added"
	ActionTimelineEntityGroupMemberRemoved = "timeline.entity_group_member_removed"
	ActionTimelineEventConnectionCreated   = "timeline.event_connection_created"
	ActionTimelineEventConnectionDeleted   = "timeline.event_connection_deleted"
	ActionTimelineVisibilityChanged        = "timeline.visibility_changed"
	ActionTimelineEventLinkVisibilityChanged = "timeline.event_link_visibility_changed"
)

// AuditEntry represents a single recorded action in the audit log.
// Each entry ties a user action to a campaign and optionally to a specific
// entity. The Details map holds action-specific metadata (e.g., old/new
// values for updates).
type AuditEntry struct {
	ID         int64          `json:"id"`
	CampaignID string         `json:"campaignId"`
	UserID     string         `json:"userId"`
	Action     string         `json:"action"`
	EntityType string         `json:"entityType,omitempty"`
	EntityID   string         `json:"entityId,omitempty"`
	EntityName string         `json:"entityName,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`

	// UserName is joined from the users table for display in the activity
	// feed. Not stored in audit_log -- populated at query time.
	UserName string `json:"userName,omitempty"`
}

// CampaignStats holds aggregate statistics for a campaign's content and
// activity. Used on the activity page header to give owners a quick overview.
type CampaignStats struct {
	// TotalEntities is the number of entities in the campaign.
	TotalEntities int `json:"totalEntities"`

	// TotalWords is an approximate word count across all entity HTML content.
	// Computed by counting spaces in entry_html -- rough but fast.
	TotalWords int64 `json:"totalWords"`

	// LastEditedAt is the timestamp of the most recent audit log entry.
	// Nil if the campaign has no activity yet.
	LastEditedAt *time.Time `json:"lastEditedAt,omitempty"`

	// ActiveEditors is the count of distinct users who performed actions
	// in the campaign within the last 30 days.
	ActiveEditors int `json:"activeEditors"`
}
