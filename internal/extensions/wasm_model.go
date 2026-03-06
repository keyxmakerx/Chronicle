package extensions

import (
	"encoding/json"
	"time"
)

// WASMPlugin represents a loaded WASM logic extension.
// WASM plugins are sandboxed backend modules that execute via Extism/wazero
// with capability-based security: they can only call host functions that
// the manifest declares in its capabilities list.
type WASMPlugin struct {
	ID          string          `json:"id"`
	ExtID       string          `json:"ext_id"`       // Manifest ID of the parent extension.
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	WASMFile    string          `json:"wasm_file"`     // Relative path to .wasm file within extension.
	Status      string          `json:"status"`        // "loaded", "stopped", "error".
	Capabilities []string       `json:"capabilities"`  // Declared capabilities from manifest.
	Config      json.RawMessage `json:"config,omitempty"` // Plugin-specific config.
	LoadedAt    time.Time       `json:"loaded_at"`
	ErrorMsg    string          `json:"error_msg,omitempty"`
}

// WASMPluginStatus constants.
const (
	WASMStatusLoaded  = "loaded"
	WASMStatusStopped = "stopped"
	WASMStatusError   = "error"
)

// WASMContribution declares a WASM logic plugin in the extension manifest.
// Included in ManifestContributes.WASMPlugins.
type WASMContribution struct {
	Slug         string            `json:"slug"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	File         string            `json:"file"`          // Relative path to .wasm file.
	Capabilities []string          `json:"capabilities"`  // Required host function groups.
	Config       []WASMConfigField `json:"config,omitempty"`
	Hooks        []string          `json:"hooks,omitempty"` // Events this plugin listens to.
	MemoryLimitMB int             `json:"memory_limit_mb,omitempty"` // Override default 16 MB.
	TimeoutSecs   int             `json:"timeout_secs,omitempty"`   // Override default 30s.
}

// WASMConfigField describes a configuration option for a WASM plugin.
type WASMConfigField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"`              // "string", "number", "boolean".
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

// WASMCapability defines a group of related host functions.
// Plugins declare which capabilities they need; the runtime only exposes
// the matching host functions.
type WASMCapability string

// Capability constants — each grants access to a set of host functions.
const (
	// CapLog allows calling chronicle_log host function.
	CapLog WASMCapability = "log"

	// CapEntityRead allows get_entity, search_entities, list_entity_types.
	CapEntityRead WASMCapability = "entity_read"

	// CapEntityWrite allows update_entity_fields.
	CapEntityWrite WASMCapability = "entity_write"

	// CapCalendarRead allows get_calendar, list_events.
	CapCalendarRead WASMCapability = "calendar_read"

	// CapCalendarWrite allows create_event.
	CapCalendarWrite WASMCapability = "calendar_write"

	// CapTagRead allows list_tags, get_entity_tags.
	CapTagRead WASMCapability = "tag_read"

	// CapTagWrite allows set_entity_tags.
	CapTagWrite WASMCapability = "tag_write"

	// CapRelationWrite allows create_relation.
	CapRelationWrite WASMCapability = "relation_write"

	// CapKVStore allows kv_get, kv_set, kv_delete for per-plugin storage.
	CapKVStore WASMCapability = "kv_store"

	// CapMessage allows send_message for plugin-to-plugin communication.
	CapMessage WASMCapability = "message"
)

// AllCapabilities lists all valid capability strings for validation.
var AllCapabilities = map[string]bool{
	string(CapLog):           true,
	string(CapEntityRead):    true,
	string(CapEntityWrite):   true,
	string(CapCalendarRead):  true,
	string(CapCalendarWrite): true,
	string(CapTagRead):       true,
	string(CapTagWrite):      true,
	string(CapRelationWrite): true,
	string(CapKVStore):       true,
	string(CapMessage):       true,
}

// WASMCallRequest is sent from the host to invoke a WASM plugin function.
type WASMCallRequest struct {
	CampaignID string          `json:"campaign_id"`
	Function   string          `json:"function"`
	Input      json.RawMessage `json:"input,omitempty"`
}

// WASMCallResponse is returned from a WASM plugin function invocation.
type WASMCallResponse struct {
	Output json.RawMessage `json:"output,omitempty"`
	Logs   []string        `json:"logs,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// HookEvent represents an event that WASM plugins can register to receive.
type HookEvent struct {
	Type       string          `json:"type"`        // e.g., "entity.created", "calendar.event_created".
	CampaignID string          `json:"campaign_id"`
	Payload    json.RawMessage `json:"payload"`
	Timestamp  time.Time       `json:"timestamp"`
}

// Hook event type constants.
const (
	HookEntityCreated        = "entity.created"
	HookEntityUpdated        = "entity.updated"
	HookEntityDeleted        = "entity.deleted"
	HookCalendarEventCreated = "calendar.event_created"
	HookCalendarEventUpdated = "calendar.event_updated"
	HookCalendarEventDeleted = "calendar.event_deleted"
	HookTagAdded             = "tag.added"
	HookTagRemoved           = "tag.removed"
)

// ValidHookTypes lists all valid hook event types.
var ValidHookTypes = map[string]bool{
	HookEntityCreated:        true,
	HookEntityUpdated:        true,
	HookEntityDeleted:        true,
	HookCalendarEventCreated: true,
	HookCalendarEventUpdated: true,
	HookCalendarEventDeleted: true,
	HookTagAdded:             true,
	HookTagRemoved:           true,
}

// WASMPluginInfo is a summary returned by the plugin manager for admin display.
type WASMPluginInfo struct {
	ExtID        string    `json:"ext_id"`
	Slug         string    `json:"slug"`
	Name         string    `json:"name"`
	Version      string    `json:"version"`
	Status       string    `json:"status"`
	Capabilities []string  `json:"capabilities"`
	Hooks        []string  `json:"hooks"`
	LoadedAt     time.Time `json:"loaded_at"`
	ErrorMsg     string    `json:"error_msg,omitempty"`
}

// WASMLimits defines resource limits for WASM plugin execution.
type WASMLimits struct {
	MemoryLimitBytes uint64 // Default 16 MB.
	TimeoutMS        int64  // Default 30000 (30s).
	FuelLimit        uint64 // Instruction fuel limit (0 = unlimited).
}

// DefaultWASMLimits returns the default resource limits for WASM plugins.
func DefaultWASMLimits() WASMLimits {
	return WASMLimits{
		MemoryLimitBytes: 16 * 1024 * 1024, // 16 MB.
		TimeoutMS:        30000,             // 30 seconds.
		FuelLimit:        0,                 // No fuel metering by default.
	}
}
