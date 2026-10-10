package systems

import (
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

	// ChoiceSourceCampaign marks pick-list entries a campaign made itself.
	ChoiceSourceCampaign = "campaign"
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
	CreatedBy  string         `json:"createdBy,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
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
}
