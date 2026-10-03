package entities

import (
	"context"
	"time"
)

// Version kinds. A page's history is a list of saved states of its title and
// text, newest first; the kind says what produced each one.
const (
	// VersionCreated is the state a page was created with.
	VersionCreated = "created"
	// VersionEdit is a save from the editor, the edit form or a sync client.
	// Saves by the same person close together share one row (see
	// versionCoalesceWindow), so typing with autosave doesn't flood history.
	VersionEdit = "edit"
	// VersionRestore is the state a restore from history saved.
	VersionRestore = "restore"
	// VersionBaseline is the state a page had before history was kept: the
	// first save of a page created before this feature records it, so that
	// save can be undone too. Who wrote it is unknown.
	VersionBaseline = "baseline"
)

// versionCoalesceWindow is how long one person's saves keep landing on the
// same history row. Ten minutes covers an editing session with autosave.
const versionCoalesceWindow = 10 * time.Minute

// MaxVersionsPerPage caps how many versions a page keeps; the oldest go first.
const MaxVersionsPerPage = 100

// EntityVersion is one saved state of a page's title and text. Field values
// are not versioned: sync clients like Foundry change them constantly.
type EntityVersion struct {
	ID        string    `json:"id"`
	EntityID  string    `json:"entity_id"`
	UserID    *string   `json:"user_id,omitempty"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Entry     *string   `json:"entry,omitempty"`
	EntryHTML *string   `json:"entry_html,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// UserName is the author's display name, joined on read; empty when the
	// author is unknown or their account is gone.
	UserName string `json:"user_name,omitempty"`
}

// TrashItem is one entry in a campaign's Trash: a deleted page and the
// sub-pages that went with it.
type TrashItem struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	TypeName      string    `json:"type_name"`
	TypeIcon      string    `json:"type_icon"`
	DeletedAt     time.Time `json:"deleted_at"`
	DeletedBy     *string   `json:"deleted_by,omitempty"`
	DeletedByName string    `json:"deleted_by_name,omitempty"`
	SubPages      int       `json:"sub_pages"`
}

// EntryConflict is what a save based on stale text gets back: the text that
// is stored now and who saved it, so the editor can offer to keep both.
type EntryConflict struct {
	Rev       int       `json:"rev"`
	Entry     *string   `json:"entry,omitempty"`
	EntryHTML *string   `json:"entry_html,omitempty"`
	ByName    string    `json:"by_name"`
	At        time.Time `json:"at"`
}

// Trash retention choices, in days, offered by the admin setting.
var TrashRetentionChoices = []int{30, 60, 90, 180, 365}

// DefaultTrashRetentionDays is used when the site setting is unset.
const DefaultTrashRetentionDays = 30

// TrashRetention reports how many days a deleted page waits in the Trash.
// The settings plugin implements it; entities can't import settings.
type TrashRetention interface {
	TrashRetentionDays(ctx context.Context) int
}

// actorKey carries the signed-in user into the service so a save can be
// credited in history without widening every service signature.
type actorKey struct{}

// WithActor returns ctx marked with the user whose request is saving a page.
func WithActor(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, actorKey{}, userID)
}

// actorFrom returns the user set by WithActor, or "" when the save has no
// known author (a background job, an import).
func actorFrom(ctx context.Context) string {
	id, _ := ctx.Value(actorKey{}).(string)
	return id
}
