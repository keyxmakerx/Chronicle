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
