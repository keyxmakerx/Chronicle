package entities

import (
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// WorldbuildingPrompt is a guided writing prompt that helps users flesh out
// their campaign content. Prompts can be global (available to all campaigns)
// or scoped to a single campaign, and optionally bound to a specific entity type.
type WorldbuildingPrompt struct {
	ID           int       `json:"id"`
	CampaignID   *string   `json:"campaign_id,omitempty"` // nil for global prompts.
	EntityTypeID *int      `json:"entity_type_id,omitempty"`
	Name         string    `json:"name"`
	PromptText   string    `json:"prompt_text"`
	Icon         string    `json:"icon"`
	SortOrder    int       `json:"sort_order"`
	IsGlobal     bool      `json:"is_global"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Joined field (optional).
	EntityTypeName string `json:"entity_type_name,omitempty"`
}

// CreatePromptInput holds input for creating a worldbuilding prompt.
type CreatePromptInput struct {
	Name         string `json:"name"`
	PromptText   string `json:"prompt_text"`
	EntityTypeID *int   `json:"entity_type_id,omitempty"` // nil = all types.
	Icon         string `json:"icon,omitempty"`
}

// UpdatePromptInput holds input for updating a worldbuilding prompt. It is a
// PARTIAL update: an absent key keeps the stored value, and every column is
// NOT NULL, so an explicit null also preserves.
type UpdatePromptInput struct {
	Name       patch.Field[string] `json:"name"`
	PromptText patch.Field[string] `json:"prompt_text"`
	Icon       patch.Field[string] `json:"icon"`
}
