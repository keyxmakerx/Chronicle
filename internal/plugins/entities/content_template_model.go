package entities

import (
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// ContentTemplate is a reusable content blueprint that pre-fills the editor
// when creating or editing an entity. Templates can be global (available to
// all campaigns) or scoped to a single campaign, and optionally bound to a
// specific entity type.
type ContentTemplate struct {
	ID           int       `json:"id"`
	CampaignID   *string   `json:"campaign_id,omitempty"` // nil for global templates.
	EntityTypeID *int      `json:"entity_type_id,omitempty"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	ContentJSON  string    `json:"content_json"` // TipTap/ProseMirror document JSON.
	ContentHTML  string    `json:"content_html"` // Pre-rendered HTML for preview.
	Icon         string    `json:"icon"`
	SortOrder    int       `json:"sort_order"`
	IsGlobal     bool      `json:"is_global"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Joined fields (optional).
	EntityTypeName string `json:"entity_type_name,omitempty"`
}

// CreateContentTemplateInput holds validated input for creating a content template.
type CreateContentTemplateInput struct {
	CampaignID   string
	EntityTypeID int // 0 = applies to all entity types.
	Name         string
	Description  string
	ContentJSON  string
	ContentHTML  string
	Icon         string
}

// UpdateContentTemplateInput holds validated input for updating a content template.
//
// It is a PARTIAL update: an absent key keeps the stored value. Every column
// is NOT NULL, so an explicit null also preserves.
type UpdateContentTemplateInput struct {
	Name        patch.Field[string] `json:"name"`
	Description patch.Field[string] `json:"description"`
	ContentJSON patch.Field[string] `json:"content_json"`
	ContentHTML patch.Field[string] `json:"content_html"`
	Icon        patch.Field[string] `json:"icon"`
}
