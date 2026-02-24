// Package notes implements the player notes widget for Chronicle. Notes are
// per-user records scoped to a campaign and optionally to a specific entity
// (page). They support text blocks, interactive checklists, and rich text
// (TipTap/ProseMirror), providing a collaborative note-taking experience in
// a collapsible bottom-right panel.
//
// Notes can be shared with the entire campaign (is_shared=true) and support
// pessimistic edit locking with 5-minute auto-expiry. Version history tracks
// content snapshots on each save.
//
// Notes are a Widget in Chronicle's three-tier extension architecture: they
// provide API endpoints for the frontend notes panel and are auto-mounted
// on every campaign page when the addon is enabled.
package notes

import "time"

// LockTimeout is how long an edit lock remains valid without a heartbeat.
const LockTimeout = 5 * time.Minute

// MaxVersionsPerNote caps the version history per note. Oldest versions are
// pruned when this limit is exceeded.
const MaxVersionsPerNote = 50

// Note represents a single user note within a campaign.
type Note struct {
	ID           string     `json:"id"`
	CampaignID   string     `json:"campaignId"`
	UserID       string     `json:"userId"`
	EntityID     *string    `json:"entityId,omitempty"`    // nil = campaign-wide note
	Title        string     `json:"title"`
	Content      []Block    `json:"content"`               // Legacy block content
	Entry        *string    `json:"entry,omitempty"`       // ProseMirror JSON (rich text)
	EntryHTML    *string    `json:"entryHtml,omitempty"`   // Pre-rendered HTML from entry
	Color        string     `json:"color"`
	Pinned       bool       `json:"pinned"`
	IsShared     bool       `json:"isShared"`
	LastEditedBy *string    `json:"lastEditedBy,omitempty"`
	LockedBy     *string    `json:"lockedBy,omitempty"`
	LockedAt     *time.Time `json:"lockedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// IsLocked reports whether the note currently has an active (non-expired) lock.
func (n *Note) IsLocked() bool {
	if n.LockedBy == nil || n.LockedAt == nil {
		return false
	}
	return time.Since(*n.LockedAt) < LockTimeout
}

// IsLockedByUser reports whether the given user holds the active lock.
func (n *Note) IsLockedByUser(userID string) bool {
	return n.IsLocked() && *n.LockedBy == userID
}

// NoteVersion is a historical snapshot of a note's content at a point in time.
type NoteVersion struct {
	ID        string    `json:"id"`
	NoteID    string    `json:"noteId"`
	UserID    string    `json:"userId"`
	Title     string    `json:"title"`
	Content   []Block   `json:"content"`
	Entry     *string   `json:"entry,omitempty"`
	EntryHTML *string   `json:"entryHtml,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
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
	IsShared bool    `json:"isShared,omitempty"`
}

// UpdateNoteRequest holds the data submitted when updating a note.
type UpdateNoteRequest struct {
	Title     *string  `json:"title,omitempty"`
	Content   *[]Block `json:"content,omitempty"`
	Entry     *string  `json:"entry,omitempty"`
	EntryHTML *string  `json:"entryHtml,omitempty"`
	Color     *string  `json:"color,omitempty"`
	Pinned    *bool    `json:"pinned,omitempty"`
	IsShared  *bool    `json:"isShared,omitempty"`
}

// ToggleCheckRequest toggles a single checklist item's checked state.
type ToggleCheckRequest struct {
	BlockIndex int `json:"blockIndex"`
	ItemIndex  int `json:"itemIndex"`
}
