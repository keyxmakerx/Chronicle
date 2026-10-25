// Package entity_notes implements the player-facing entity-page notes
// widget. Each note has an audience (private / dm_only / dm_scribe /
// everyone / custom) controlling who besides the author can read it. The
// author always sees their own notes; DMs cannot read another user's
// `private` notes — this is the "true private space" guarantee players
// rely on. See db/migrations/000023_entity_notes.up.sql for the audience
// enum. Distinct from internal/widgets/notes (campaign notebook panel) and
// internal/widgets/posts (entity-scoped shared content).
package entity_notes

import (
	"encoding/json"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Audience enumerates the visibility tiers a note can have.
// String values match the ENUM in db/migrations/000023_entity_notes.up.sql.
type Audience string

const (
	// AudiencePrivate — only the author can read or modify.
	AudiencePrivate Audience = "private"
	// AudienceDMOnly — RoleOwner and is_dm_granted users.
	AudienceDMOnly Audience = "dm_only"
	// AudienceDMScribe — RoleOwner, RoleScribe, and is_dm_granted users.
	AudienceDMScribe Audience = "dm_scribe"
	// AudienceEveryone — all campaign members.
	AudienceEveryone Audience = "everyone"
	// AudienceCustom — explicit user_ids in the SharedWith JSON list.
	AudienceCustom Audience = "custom"
)

// Valid reports whether the value is one of the accepted audience strings.
// Anything else should be rejected at the service layer before reaching
// the repo, so the DB enum is a defense-in-depth check.
func (a Audience) Valid() bool {
	switch a {
	case AudiencePrivate, AudienceDMOnly, AudienceDMScribe, AudienceEveryone, AudienceCustom:
		return true
	}
	return false
}

// Note is a single per-user, per-entity note.
type Note struct {
	ID           string   `json:"id"`
	EntityID     string   `json:"entityId"`
	CampaignID   string   `json:"campaignId"`
	AuthorUserID string   `json:"authorUserId"`
	Audience     Audience `json:"audience"`
	// SharedWith is meaningful only when Audience == AudienceCustom.
	// Always emitted as an empty array (never null) for stable client UI.
	SharedWith []string `json:"sharedWith"`
	Title      string   `json:"title,omitempty"`
	// Body is the TipTap ProseMirror JSON. Clients render it through
	// the same editor used for entity entry / posts entry.
	Body json.RawMessage `json:"body,omitempty"`
	// BodyHTML is server-sanitized HTML safe to inject via templ.Raw.
	BodyHTML  string    `json:"bodyHtml,omitempty"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CreateNoteRequest holds the data submitted when creating a new note.
// AuthorUserID, EntityID, CampaignID are bound from URL params + auth
// context, never from the request body.
type CreateNoteRequest struct {
	Audience   Audience        `json:"audience"`
	SharedWith []string        `json:"sharedWith,omitempty"`
	Title      string          `json:"title,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
	BodyHTML   string          `json:"bodyHtml,omitempty"`
	Pinned     bool            `json:"pinned,omitempty"`
}

// UpdateNoteRequest holds the partial-update payload. All fields are
// optional pointers so the service can distinguish "not set" from
// "explicitly cleared." Only the author may update a note.
type UpdateNoteRequest struct {
	Audience   *Audience       `json:"audience,omitempty"`
	SharedWith []string        `json:"sharedWith,omitempty"`
	Title      *string         `json:"title,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
	BodyHTML   *string         `json:"bodyHtml,omitempty"`
	Pinned     *bool           `json:"pinned,omitempty"`
}

// ViewerFor is the audience facts for v, a request-scoped viewer whose role
// is the visibility role: the Owner and a co-DM both sit at RoleOwner there
// and read the same notes, so either counts as DM-granted here.
func ViewerFor(campaignID string, v permissions.Viewer) ViewerContext {
	role := v.Role()
	return ViewerContext{
		UserID:      v.UserID(),
		CampaignID:  campaignID,
		IsScribe:    role == permissions.RoleScribe,
		IsDMGranted: role >= permissions.RoleOwner,
	}
}
