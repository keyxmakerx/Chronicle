// Package chronicle provides Go type definitions and host function helpers
// for building Chronicle WASM plugins. It defines the JSON structures used
// to communicate with Chronicle's host functions and provides helper types
// for hook events, entities, tags, calendar events, relations, and KV storage.
//
// This package is intended to be imported by Go/TinyGo WASM plugins as a
// lightweight SDK. It does NOT contain the actual host function implementations
// (those are provided by the Chronicle runtime); it only defines the data
// contracts and convenience types.
//
// Usage in a plugin:
//
//	import "github.com/keyxmakerx/chronicle/sdk/go/chronicle"
//
//	func onHook() int32 {
//	    var event chronicle.HookEvent
//	    if err := pdk.InputJSON(&event); err != nil { ... }
//	    ...
//	}
package chronicle

import "encoding/json"

// ---------------------------------------------------------------------------
// Hook Events
// ---------------------------------------------------------------------------

// HookEvent is the payload Chronicle sends when a hook fires.
type HookEvent struct {
	Type       string `json:"type"`
	EntityID   string `json:"entity_id,omitempty"`
	CampaignID string `json:"campaign_id,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
}

// Hook event type constants.
const (
	HookEntityCreated       = "entity.created"
	HookEntityUpdated       = "entity.updated"
	HookEntityDeleted       = "entity.deleted"
	HookCalendarEventCreated = "calendar.event_created"
	HookCalendarEventUpdated = "calendar.event_updated"
	HookCalendarEventDeleted = "calendar.event_deleted"
	HookTagAdded            = "tag.added"
	HookTagRemoved          = "tag.removed"
)

// ---------------------------------------------------------------------------
// Entity types
// ---------------------------------------------------------------------------

// GetEntityInput is the input for the get_entity host function.
type GetEntityInput struct {
	EntityID string `json:"entity_id"`
}

// SearchEntitiesInput is the input for the search_entities host function.
type SearchEntitiesInput struct {
	CampaignID string `json:"campaign_id"`
	Query      string `json:"query"`
	Limit      int    `json:"limit,omitempty"`
}

// UpdateEntityFieldsInput is the input for the update_entity_fields host function.
type UpdateEntityFieldsInput struct {
	EntityID string          `json:"entity_id"`
	Fields   json.RawMessage `json:"fields"`
}

// ---------------------------------------------------------------------------
// Tag types
// ---------------------------------------------------------------------------

// SetEntityTagsInput is the input for the set_entity_tags host function.
type SetEntityTagsInput struct {
	EntityID string `json:"entity_id"`
	TagIDs   []int  `json:"tag_ids"`
}

// GetEntityTagsInput is the input for the get_entity_tags host function.
type GetEntityTagsInput struct {
	EntityID string `json:"entity_id"`
}

// Tag represents a tag returned by host functions.
type Tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug,omitempty"`
	Color string `json:"color,omitempty"`
}

// ---------------------------------------------------------------------------
// Calendar types
// ---------------------------------------------------------------------------

// CreateEventInput is the input for the create_event host function.
type CreateEventInput struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Year        int     `json:"year"`
	Month       int     `json:"month"`
	Day         int     `json:"day"`
	StartHour   *int    `json:"start_hour,omitempty"`
	StartMinute *int    `json:"start_minute,omitempty"`
	EntityID    *string `json:"entity_id,omitempty"`
}

// GetCalendarInput is the input for the get_calendar host function.
type GetCalendarInput struct {
	CampaignID string `json:"campaign_id"`
}

// ListEventsInput is the input for the list_events host function.
type ListEventsInput struct {
	CampaignID string `json:"campaign_id"`
	Limit      int    `json:"limit,omitempty"`
}

// ---------------------------------------------------------------------------
// Relation types
// ---------------------------------------------------------------------------

// CreateRelationInput is the input for the create_relation host function.
type CreateRelationInput struct {
	SourceEntityID      string          `json:"source_entity_id"`
	TargetEntityID      string          `json:"target_entity_id"`
	RelationType        string          `json:"relation_type"`
	ReverseRelationType string          `json:"reverse_relation_type,omitempty"`
	CreatedBy           string          `json:"created_by,omitempty"`
	Metadata            json.RawMessage `json:"metadata,omitempty"`
	DmOnly              bool            `json:"dm_only,omitempty"`
}

// ---------------------------------------------------------------------------
// KV Store types
// ---------------------------------------------------------------------------

// KVGetInput is the input for the kv_get host function.
type KVGetInput struct {
	Key string `json:"key"`
}

// KVSetInput is the input for the kv_set host function.
type KVSetInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// KVDeleteInput is the input for the kv_delete host function.
type KVDeleteInput struct {
	Key string `json:"key"`
}

// ---------------------------------------------------------------------------
// Message types
// ---------------------------------------------------------------------------

// SendMessageInput is the input for the send_message host function.
type SendMessageInput struct {
	TargetExtID string          `json:"target_ext_id"`
	TargetSlug  string          `json:"target_slug"`
	Payload     json.RawMessage `json:"payload"`
}

// MessageEnvelope is the payload received by on_message handlers.
type MessageEnvelope struct {
	SenderExtID string          `json:"sender_ext_id"`
	Payload     json.RawMessage `json:"payload"`
}

// ---------------------------------------------------------------------------
// Common response types
// ---------------------------------------------------------------------------

// OkResponse is a standard success response from host functions.
type OkResponse struct {
	OK bool `json:"ok"`
}

// ErrorResponse indicates a host function error.
type ErrorResponse struct {
	Error string `json:"error"`
}

// ---------------------------------------------------------------------------
// Capability constants
// ---------------------------------------------------------------------------

// Capability names that can be declared in the plugin manifest.
const (
	CapLog           = "log"
	CapEntityRead    = "entity_read"
	CapEntityWrite   = "entity_write"
	CapCalendarRead  = "calendar_read"
	CapCalendarWrite = "calendar_write"
	CapTagRead       = "tag_read"
	CapTagWrite      = "tag_write"
	CapRelationWrite = "relation_write"
	CapKVStore       = "kv_store"
	CapMessage       = "message"
)
