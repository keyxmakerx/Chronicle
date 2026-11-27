package homebrew

import (
	"encoding/json"
	"time"
)

// Who sees a part. Set once per part in the Workshop.
const (
	SeeParty    = "party"    // everyone in the campaign
	SeeName     = "name"     // players see that it exists, not its details
	SeeDirector = "director" // Directors only
)

// When a rule's actions run.
const (
	TimingNow          = "now"
	TimingEncounterEnd = "encounter_end"
	TimingHeld         = "held" // waits in the Homebrew drawer for the Director
)

// Viewer is who is asking, as the handler read it from the campaign context.
type Viewer struct {
	UserID   string
	Director bool
	// VisibilityRole is passed on to owners whose actions check page
	// access as this person.
	VisibilityRole int
}

// Recipe is one saved recipe. Doc is the part the Workshop edits.
type Recipe struct {
	ID         string    `json:"id"`
	CampaignID string    `json:"-"`
	Name       string    `json:"name"`
	Doc        Doc       `json:"doc"`
	Version    int       `json:"version"`
	CreatedBy  string    `json:"-"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Doc is a recipe's parts and the rules wiring them.
type Doc struct {
	Parts []Part `json:"parts"`
	Rules []Rule `json:"rules"`
	// Layout is where each card sits on the Workshop canvas, keyed by card:
	// "p:<part key>", "r<rule>" for a rule's When, "r<rule>.<step>" for one
	// of its Thens (both counted from 0). Only the Workshop reads it.
	Layout map[string]Pos `json:"layout,omitempty"`
	// Off keeps a recipe that arrived from the Store from running until a
	// Director has pointed its unlinked parts at this campaign's own records.
	// Only the Store's add sets it; any save through the Workshop clears it
	// (and refuses while a part is still unlinked), so a Director's own
	// recipes are never off.
	Off bool `json:"off,omitempty"`
}

// Pos is a card's top-left corner on the canvas, in canvas pixels.
type Pos struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Part is one widget or owned record in a recipe. Data belongs to the kind's
// owner, which validates it; this package only checks the envelope.
type Part struct {
	Key   string          `json:"key"`
	Kind  string          `json:"kind"`
	Label string          `json:"label"`
	See   string          `json:"see"`
	Data  json.RawMessage `json:"data,omitempty"`
	// Unlinked lists the links this part had to a record of the campaign it
	// was shared from (a page, a deck), which sharing drops because an id
	// means nothing elsewhere. Only a recipe that arrived from the Store has
	// any; the Director points each at one of this campaign's own.
	Unlinked []Unlinked `json:"unlinked,omitempty"`
}

// Unlinked is one dropped link: what kind of thing the Director picks to
// restore it, in the words of a sentence ("a creature recipe page").
type Unlinked struct {
	Pick string `json:"pick"`
}

// RecipePart is one part of a saved recipe, with the recipe it belongs to.
type RecipePart struct {
	RecipeID string
	Part     Part
}

// Rule is one When/Then: an event, an optional test, and the actions it runs.
type Rule struct {
	When   When   `json:"when"`
	Timing string `json:"timing"`
	Then   []Do   `json:"then"`
}

// When names the event that starts a rule. Part, when set, is the part the
// event must come from.
type When struct {
	Event string          `json:"event"`
	Part  string          `json:"part,omitempty"`
	Args  json.RawMessage `json:"args,omitempty"`
	Test  string          `json:"test,omitempty"`
}

// Do is one action of a rule, on one part of the same recipe.
type Do struct {
	Action string          `json:"action"`
	Part   string          `json:"part,omitempty"`
	Args   json.RawMessage `json:"args,omitempty"`
}

// RecipeInput is a create or update body. Version is required on update and
// a mismatch is a conflict.
type RecipeInput struct {
	Name    string `json:"name"`
	Doc     Doc    `json:"doc"`
	Version int    `json:"version"`
}

// RecipeSummary is a recipe in a list: enough to show and open it.
type RecipeSummary struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Parts int    `json:"parts"`
	Rules int    `json:"rules"`
	// PartKinds are the kinds of part the recipe has, sorted, so a caller
	// can tell what a recipe can put on a page without loading it.
	PartKinds []string `json:"partKinds"`
	// Off is true for a recipe from the Store that has not been switched on.
	Off       bool      `json:"off"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}
