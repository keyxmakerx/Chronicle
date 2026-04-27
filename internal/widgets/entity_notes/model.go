// Package entity_notes implements the player-facing entity-page notes
// widget. Each note belongs to one author and has an audience setting
// (private / dm_only / dm_scribe / everyone / custom) that controls
// which other campaign members can read it.
//
// The audience model intentionally separates *who can see a note* from
// *who wrote it*: the author always sees their own notes regardless
// of audience, and DMs cannot read another user's `private` notes.
// This is the "true private space" guarantee — without it, players
// have no place to keep notes the GM doesn't see.
//
// Distinct from:
//   - internal/widgets/notes/    — campaign-scoped floating notebook panel
//   - internal/widgets/posts/    — entity-scoped *shared* sub-content
//   - entities.player_notes col  — GM-authored player-visible note on the entity
//
// See db/migrations/000023_entity_notes.up.sql for the schema and the
// audience-enum docstring.
package entity_notes

import (
	"encoding/json"
	"time"
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
	ID           string          `json:"id"`
	EntityID     string          `json:"entityId"`
	CampaignID   string          `json:"campaignId"`
	AuthorUserID string          `json:"authorUserId"`
	Audience     Audience        `json:"audience"`
	// SharedWith is meaningful only when Audience == AudienceCustom.
	// Always emitted as an empty array (never null) for stable client UI.
	SharedWith []string        `json:"sharedWith"`
	Title      string          `json:"title,omitempty"`
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
	Audience   *Audience        `json:"audience,omitempty"`
	SharedWith []string         `json:"sharedWith,omitempty"`
	Title      *string          `json:"title,omitempty"`
	Body       json.RawMessage  `json:"body,omitempty"`
	BodyHTML   *string          `json:"bodyHtml,omitempty"`
	Pinned     *bool            `json:"pinned,omitempty"`
}
