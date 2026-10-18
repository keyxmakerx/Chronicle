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
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Parts     int       `json:"parts"`
	Rules     int       `json:"rules"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}
