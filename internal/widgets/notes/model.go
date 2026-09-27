// Package notes implements Chronicle's notes: the full-page Journal and the
// floating per-page Jot notes. Both are rows in one table; a note with no page
// (entity_id NULL) belongs to the Journal, a note on a page is that page's jot.
// Notes support folders, rich text (TipTap/ProseMirror), checklists, version
// history, audio attachments and a pessimistic edit lock with 5-minute expiry.
//
// Who can read a note is decided by Note.CanView and its SQL twin in the
// repository; see Visibility for the four audiences.
package notes

import (
	"encoding/json"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// LockTimeout is how long an edit lock remains valid without a heartbeat.
const LockTimeout = 5 * time.Minute

// MaxVersionsPerNote caps the version history per note. Oldest versions are
// pruned when this limit is exceeded.
const MaxVersionsPerNote = 50

// Visibility is who besides its owner can read a note. It is derived from
// three columns rather than stored as one, so the rows written before the GM
// state existed keep their exact audience and older sync clients that only
// know isShared/sharedWith keep working:
//
//	party   is_shared = TRUE
//	custom  shared_with names at least one person
//	gm      shared_with_gm = TRUE (the campaign Owner and co-DMs)
//	private none of the above: the owner alone, not even the GM
type Visibility string

// The four audiences a note can have.
const (
	VisibilityPrivate Visibility = "private"
	VisibilityGM      Visibility = "gm"
	VisibilityParty   Visibility = "party"
	VisibilityCustom  Visibility = "custom"
)

// Valid reports whether v is one of the four audiences.
func (v Visibility) Valid() bool {
	switch v {
	case VisibilityPrivate, VisibilityGM, VisibilityParty, VisibilityCustom:
		return true
	}
	return false
}

// derive fills the fields computed from stored columns. Called on every row
// the repository scans, so a Note never leaves the package without them.
func (n *Note) derive() {
	n.Archived = n.ArchivedAt != nil
	switch {
	case n.IsShared:
		n.Visibility = VisibilityParty
	case len(n.SharedWith) > 0:
		n.Visibility = VisibilityCustom
	case n.SharedWithGM:
		n.Visibility = VisibilityGM
	default:
		n.Visibility = VisibilityPrivate
	}
}

// applyVisibility sets the sharing columns for v. The columns are written
// together so the derived Visibility and the SQL filter can never disagree.
func (n *Note) applyVisibility(v Visibility, sharedWith []string) {
	n.IsShared = v == VisibilityParty
	n.SharedWithGM = v == VisibilityGM
	n.SharedWith = nil
	if v == VisibilityCustom {
		n.SharedWith = sharedWith
	}
	n.derive()
}

// Note represents a single note within a campaign: a Journal note, a jot on a
// page, or a folder.
type Note struct {
	ID           string     `json:"id"`
	CampaignID   string     `json:"campaignId"`
	UserID       string     `json:"userId"`
	EntityID     *string    `json:"entityId,omitempty"`     // nil = Journal note; set = that page's jot
	LinkedNoteID *string    `json:"linkedNoteId,omitempty"` // jot sent to the Journal: the note it became
	ParentID     *string    `json:"parentId,omitempty"`     // nil = top-level note/folder
	IsFolder     bool       `json:"isFolder"`               // true = folder container
	Title        string     `json:"title"`
	Content      []Block    `json:"content"`             // Legacy block content
	Entry        *string    `json:"entry,omitempty"`     // ProseMirror JSON (rich text)
	EntryHTML    *string    `json:"entryHtml,omitempty"` // Pre-rendered HTML from entry
	Color        string     `json:"color"`
	Pinned       bool       `json:"pinned"`
	ArchivedAt   *time.Time `json:"archivedAt,omitempty"`
	Archived     bool       `json:"archived"` // derived from ArchivedAt
	IsShared     bool       `json:"isShared"`
	SharedWith   []string   `json:"sharedWith,omitempty"` // User IDs this note is shared with (nil = use IsShared)
	SharedWithGM bool       `json:"-"`                    // read through Visibility
	Visibility   Visibility `json:"visibility"`           // derived from the three sharing columns
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

// CanView reports whether v may read this note within campaignID: its owner,
// anyone when it is shared with the party, a named person when it is shared
// with them, and the campaign's GMs (Owner or co-DM) when it is shared with
// the GM. Private is private from the GM too, so SkipsPerUserRules is never
// consulted, and an anonymous viewer sees nothing.
//
// Lives on the model because every path that addresses a single note (the web
// routes and the REST API v1 routes) must apply the identical predicate, and
// neither the repository nor the service filters the single-resource path
// (`WHERE id = ?`); a route that forgets it is an IDOR. The repository's
// visibleFilter is the SQL twin of this function and must stay in step
// (TestDB_ListVisibilityMatchesCanView pins them together).
func (n *Note) CanView(v permissions.Viewer, campaignID string) bool {
	if n.CampaignID != campaignID {
		return false
	}
	uid := v.UserID()
	if uid == "" {
		return false
	}
	if n.UserID == uid || n.IsShared {
		return true
	}
	for _, id := range n.SharedWith {
		if id == uid {
			return true
		}
	}
	return n.SharedWithGM && permissions.CanSeeDmOnly(v.Role())
}

// IsOwnedBy reports whether userID owns this note within campaignID. Owner-only
// operations — deleting, filing into a folder, archiving, and changing its
// sharing or pinned state — gate on this rather than on CanView, which also
// admits everyone the note is shared with.
func (n *Note) IsOwnedBy(userID, campaignID string) bool {
	return n.CampaignID == campaignID && n.UserID == userID
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

// NoteAttachment represents a file attached to a note (audio, etc.).
type NoteAttachment struct {
	ID           string    `json:"id"`
	NoteID       string    `json:"noteId"`
	CampaignID   string    `json:"campaignId"`
	FilePath     string    `json:"filePath"`
	OriginalName string    `json:"originalName"`
	MimeType     string    `json:"mimeType"`
	FileSize     int64     `json:"fileSize"`
	DurationSecs *int      `json:"durationSecs,omitempty"`
	Transcript   *string   `json:"transcript,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// --- Request DTOs ---

// CreateNoteRequest holds the data submitted when creating a new note.
type CreateNoteRequest struct {
	EntityID   *string  `json:"entityId,omitempty"`
	ParentID   *string  `json:"parentId,omitempty"`
	IsFolder   bool     `json:"isFolder,omitempty"`
	Title      string   `json:"title"`
	Content    []Block  `json:"content"`
	Color      string   `json:"color,omitempty"`
	IsShared   bool     `json:"isShared,omitempty"`
	SharedWith []string `json:"sharedWith,omitempty"` // Share with specific users
	// Visibility, when set, decides the audience instead of IsShared/SharedWith.
	// Absent means private: a new note is private to its writer until they
	// choose to share it.
	Visibility Visibility `json:"visibility,omitempty"`
}

// UpdateNoteRequest holds the data submitted when updating a note.
type UpdateNoteRequest struct {
	Title      *string  `json:"title,omitempty"`
	Content    *[]Block `json:"content,omitempty"`
	Entry      *string  `json:"entry,omitempty"`
	EntryHTML  *string  `json:"entryHtml,omitempty"`
	Color      *string  `json:"color,omitempty"`
	Pinned     *bool    `json:"pinned,omitempty"`
	IsShared   *bool    `json:"isShared,omitempty"`
	SharedWith []string `json:"sharedWith,omitempty"` // Share with specific users (empty = clear)
	ParentID   *string  `json:"parentId,omitempty"`   // move note into/out of folder ("" = top level)
	// Visibility sets the audience in one field; wins over IsShared/SharedWith
	// when both are sent. SharedWith supplies the people for "custom".
	Visibility *Visibility `json:"visibility,omitempty"`
	Archived   *bool       `json:"archived,omitempty"`
}

// StripOwnerOnly drops the fields only a note's owner may change, so a person
// the note is shared with can still save its title and body. Dropped, not
// refused: a client that echoes the whole note back must not fail the edit.
func (r *UpdateNoteRequest) StripOwnerOnly() {
	r.IsShared = nil
	r.SharedWith = nil
	r.Visibility = nil
	r.Pinned = nil
	r.ParentID = nil
	r.Archived = nil
}

// MarshalSharedWith converts a SharedWith slice to JSON for database storage.
// Returns nil if the slice is nil (preserving the is_shared fallback behavior).
func MarshalSharedWith(users []string) *string {
	if users == nil {
		return nil
	}
	data, err := json.Marshal(users)
	if err != nil {
		return nil
	}
	s := string(data)
	return &s
}

// UnmarshalSharedWith parses the JSON shared_with column from the database.
func UnmarshalSharedWith(raw *string) []string {
	if raw == nil || *raw == "" {
		return nil
	}
	var users []string
	if err := json.Unmarshal([]byte(*raw), &users); err != nil {
		return nil
	}
	return users
}

// ToggleCheckRequest toggles a single checklist item's checked state.
type ToggleCheckRequest struct {
	BlockIndex int `json:"blockIndex"`
	ItemIndex  int `json:"itemIndex"`
}
