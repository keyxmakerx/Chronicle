// Package extensions manages user-installable content extensions.
// Extensions are declarative content packs (no code execution) that provide
// calendar presets, entity type templates, entity packs, tag collections,
// marker icon packs, themes, and reference data. Site admins install
// extensions; campaign owners enable them per-campaign.
package extensions

import (
	"encoding/json"
	"time"
)

// Extension represents an installed content extension (site-wide).
type Extension struct {
	ID          string          `json:"id"`
	ExtID       string          `json:"ext_id"`       // Manifest ID (e.g., "dnd5e-srd-monsters").
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Manifest    json.RawMessage `json:"manifest"`     // Full manifest.json for reference.
	InstalledBy string          `json:"installed_by"`
	Status      string          `json:"status"`       // "active" or "disabled".
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// CampaignExtension tracks per-campaign extension activation.
type CampaignExtension struct {
	CampaignID      string          `json:"campaign_id"`
	ExtensionID     string          `json:"extension_id"`
	Enabled         bool            `json:"enabled"`
	AppliedContents json.RawMessage `json:"applied_contents,omitempty"` // Tracks which contributes were imported.
	EnabledAt       time.Time       `json:"enabled_at"`
	EnabledBy       *string         `json:"enabled_by,omitempty"`

	// Joined fields for display.
	ExtID       string `json:"ext_id,omitempty"`
	ExtName     string `json:"ext_name,omitempty"`
	ExtVersion  string `json:"ext_version,omitempty"`
	ExtStatus   string `json:"ext_status,omitempty"`
}

// Provenance tracks which extension created which database record.
// Enables clean uninstall and "provided by extension X" attribution.
type Provenance struct {
	ID          int64     `json:"id"`
	CampaignID  string    `json:"campaign_id"`
	ExtensionID string    `json:"extension_id"`
	TableName   string    `json:"table_name"`   // e.g., "entity_types", "entities", "tags".
	RecordID    string    `json:"record_id"`     // PK of the created record.
	RecordType  string    `json:"record_type"`   // Sub-type hint (e.g., "entity_pack:srd-monsters").
	CreatedAt   time.Time `json:"created_at"`
}

// ExtensionData holds extension-specific key-value data that doesn't fit
// existing tables (e.g., relation type suggestions, marker icon metadata).
type ExtensionData struct {
	ID          int64           `json:"id"`
	CampaignID  string          `json:"campaign_id"`
	ExtensionID string          `json:"extension_id"`
	Namespace   string          `json:"namespace"`  // e.g., "relation_types", "marker_icons".
	DataKey     string          `json:"data_key"`
	DataValue   json.RawMessage `json:"data_value"`
}

// ExtensionStatus constants.
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)
