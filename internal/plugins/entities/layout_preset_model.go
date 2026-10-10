package entities

import (
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// LayoutPreset is a reusable page layout configuration that can be applied to
// any entity type. Presets are campaign-scoped and optionally built-in (seeded
// on campaign creation, cannot be edited or deleted).
type LayoutPreset struct {
	ID          int       `json:"id"`
	CampaignID  string    `json:"campaign_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	LayoutJSON  string    `json:"layout_json"` // Serialized EntityTypeLayout JSON.
	Icon        string    `json:"icon"`
	SortOrder   int       `json:"sort_order"`
	IsBuiltin   bool      `json:"is_builtin"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateLayoutPresetInput holds validated input for creating a layout preset.
type CreateLayoutPresetInput struct {
	CampaignID  string
	Name        string
	Description string
	LayoutJSON  string
	Icon        string
}

// UpdateLayoutPresetInput holds validated input for updating a layout preset.
// It is a PARTIAL update: an absent key keeps the stored value. Every column
// is NOT NULL, so an explicit null also preserves; an empty icon is a value
// and resets to the default.
type UpdateLayoutPresetInput struct {
	Name        patch.Field[string] `json:"name"`
	Description patch.Field[string] `json:"description"`
	LayoutJSON  patch.Field[string] `json:"layout_json"`
	Icon        patch.Field[string] `json:"icon"`
}
