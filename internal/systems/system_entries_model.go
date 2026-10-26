package systems

import (
	"context"
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// System entries are the pick-list rows a campaign adds to its game system
// (an ancestry, kit, culture, career, class, race...). They are stored apart
// from the system package so a package update can never touch them; the
// picker merges both on read (see character_choices.go).

const (
	// EntryVisibilityEveryone lists the entry for every campaign member.
	EntryVisibilityEveryone = "everyone"
	// EntryVisibilityDirectors keeps the entry to the owner and co-Directors,
	// for an option the party has not discovered yet.
	EntryVisibilityDirectors = "directors"
	// EntryVisibilityCharacter shows the entry to Directors and to whoever
	// plays one character page (ForEntityID): a penciled option shown to the
	// one player it is meant for.
	EntryVisibilityCharacter = "character"

	// ChoiceSourceCampaign marks pick-list entries a campaign made itself.
	ChoiceSourceCampaign = "campaign"

	// EntryStatusAccepted is every Director's entry and a player's once a
	// Director accepts it: visibility alone decides who sees it.
	EntryStatusAccepted = "accepted"
	// EntryStatusDraft is a player's entry they are still writing, or one a
	// Director handed back with a note. Only its writer and the Directors
	// see a draft, whatever its visibility says.
	EntryStatusDraft = "draft"
	// EntryStatusSent is a player's draft handed in for a Director to accept.
	EntryStatusSent = "sent"
)

// SystemEntry is one campaign-made pick-list entry.
type SystemEntry struct {
	ID         int64  `json:"id"`
	CampaignID string `json:"campaignId"`
	SystemID   string `json:"systemId"`
	// FieldKey is the character field the entry is offered for ("ancestry").
	FieldKey string `json:"fieldKey"`
	// Slug is derived from the name once and never changes, so a rename does
	// not break anything that remembers the entry.
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	// Properties holds kind-specific details (size, speed...) as a flat object
	// of scalars, so every consumer can render a value without a schema.
	Properties map[string]any `json:"properties"`
	Visibility string         `json:"visibility"`
	// ForEntityID is the character page a 'character' entry is kept to;
	// empty for every other visibility.
	ForEntityID string `json:"forEntityId,omitempty"`
	// ForName is that character's name, filled for Directors on read so a
	// list can say who an entry is for. Not stored.
	ForName string `json:"forName,omitempty"`
	// Status is EntryStatusAccepted, or draft/sent for a player's entry a
	// Director has not accepted yet.
	Status string `json:"status"`
	// ReviewNote is what a Director asked the writer to change.
	ReviewNote string    `json:"reviewNote,omitempty"`
	CreatedBy  string    `json:"createdBy,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// EntryActor is who is asking. The service decides what they may do; the
// caller (handler or AI import) only states the facts about the person.
type EntryActor struct {
	UserID string
	// IsDirector is true for the campaign owner and co-Directors.
	IsDirector bool
}

// CreateSystemEntryInput is the body of a create.
type CreateSystemEntryInput struct {
	FieldKey    string         `json:"fieldKey"`
	Name        string         `json:"name"`
	Summary     string         `json:"summary"`
	Description string         `json:"description"`
	Properties  map[string]any `json:"properties"`
	// Visibility defaults to "everyone" when empty.
	Visibility string `json:"visibility"`
	// ForEntityID names the character page; required with visibility
	// "character" and refused with any other.
	ForEntityID string `json:"forEntityId"`
	// Status is for a player's own entry only: "sent" hands it in at once,
	// anything else keeps it a draft. A Director's entry is always accepted.
	Status string `json:"status"`
}

// UpdateSystemEntryInput is a PARTIAL update: an absent key keeps the stored
// value, an explicit null clears it, a present value replaces it. Name and
// visibility cannot be empty, so null on them is refused rather than ignored.
// The field key and slug are fixed once created.
type UpdateSystemEntryInput struct {
	Name        patch.Field[string]         `json:"name"`
	Summary     patch.Field[string]         `json:"summary"`
	Description patch.Field[string]         `json:"description"`
	Properties  patch.Field[map[string]any] `json:"properties"`
	Visibility  patch.Field[string]         `json:"visibility"`
	// ForEntityID changes the character a 'character' entry is kept to.
	// Moving to another visibility clears it.
	ForEntityID patch.Field[string] `json:"forEntityId"`
	// Status moves a player's entry: its writer between draft and sent, a
	// Director to accepted, or back to draft with a ReviewNote.
	Status     patch.Field[string] `json:"status"`
	ReviewNote patch.Field[string] `json:"reviewNote"`
}

// EntryGrant lets one player write one entry of their own for a pick list.
// It is spent (EntryID set) when they save it; a Director can give it again.
type EntryGrant struct {
	ID         int64  `json:"id"`
	CampaignID string `json:"-"`
	SystemID   string `json:"-"`
	FieldKey   string `json:"fieldKey"`
	UserID     string `json:"userId"`
	// Budget is the points total the player's entry gets (the "*_points"
	// property), so a player cannot grant themselves more; 0 for none.
	Budget int `json:"budget"`
	// EntryID is the entry the player made with it; 0 while unspent.
	EntryID   int64     `json:"entryId,omitempty"`
	GrantedBy string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

// Spent reports whether the player has already made their entry.
func (g *EntryGrant) Spent() bool { return g.EntryID != 0 }

// GrantInput is the body of a Director giving a player the permission.
// Giving it again to the same player for the same list renews it.
type GrantInput struct {
	FieldKey string `json:"fieldKey"`
	UserID   string `json:"userId"`
	Budget   int    `json:"budget"`
}

// EntryCharacter is a character page an entry can be kept to.
type EntryCharacter struct {
	ID   string
	Name string
	// OwnerUserID is the player who plays it; empty when nobody has claimed it.
	OwnerUserID string
}

// EntryCharacters finds the campaign's character pages for entries kept to
// one character. Implemented at the composition root over the entities
// plugin, which this package cannot import.
type EntryCharacters interface {
	// Character returns the character page with this id in the campaign, or
	// nil when there is none.
	Character(ctx context.Context, campaignID, entityID string) (*EntryCharacter, error)
	// CharacterNamed returns the campaign's one character page with this
	// name (any casing), or nil when there is none or more than one.
	CharacterNamed(ctx context.Context, campaignID, name string) (*EntryCharacter, error)
}
