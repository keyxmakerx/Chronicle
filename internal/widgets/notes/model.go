// Package notes implements the floating notes widget for Chronicle. Notes are
// personal, per-user records scoped to a campaign and optionally to a specific
// entity (page). They support text blocks and interactive checklists, providing
// a Google Keep-style experience in a collapsible bottom-right panel.
//
// Notes are a Widget in Chronicle's three-tier extension architecture: they
// provide API endpoints for the frontend notes panel and are auto-mounted
// on every campaign page when the addon is enabled.
package notes

import "time"

// Note represents a single user note within a campaign.
type Note struct {
	ID         string    `json:"id"`
	CampaignID string    `json:"campaignId"`
	UserID     string    `json:"userId"`
	EntityID   *string   `json:"entityId,omitempty"` // nil = campaign-wide note
	Title      string    `json:"title"`
	Content    []Block   `json:"content"`
	Color      string    `json:"color"`
	Pinned     bool      `json:"pinned"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Block is a single content block within a note. Discriminated by Type.
type Block struct {
	Type  string          `json:"type"`            // "text" or "checklist"
	Value string          `json:"value,omitempty"` // For type "text"
	Items []ChecklistItem `json:"items,omitempty"` // For type "checklist"
}

// ChecklistItem is a single item in a checklist block.
type ChecklistItem struct {
	Text    string `json:"text"`
	Checked bool   `json:"checked"`
}

// --- Request DTOs ---

// CreateNoteRequest holds the data submitted when creating a new note.
type CreateNoteRequest struct {
	EntityID *string `json:"entityId,omitempty"`
	Title    string  `json:"title"`
	Content  []Block `json:"content"`
	Color    string  `json:"color,omitempty"`
}

// UpdateNoteRequest holds the data submitted when updating a note.
type UpdateNoteRequest struct {
	Title   *string  `json:"title,omitempty"`
	Content *[]Block `json:"content,omitempty"`
	Color   *string  `json:"color,omitempty"`
	Pinned  *bool    `json:"pinned,omitempty"`
}

// ToggleCheckRequest toggles a single checklist item's checked state.
type ToggleCheckRequest struct {
	BlockIndex int `json:"blockIndex"`
	ItemIndex  int `json:"itemIndex"`
}
