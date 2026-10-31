package crafting

import "context"

// What a hero does at the bench outside their own projects: experimenting
// to find a recipe they don't know, breaking a made item down for something
// back, and lending a hand on a party member's project. Experimenting and
// helping cost a turn, once at each respite the Director logs; breaking down
// costs nothing but the item.

// Turn kinds, as crafting_turns keeps them.
const (
	TurnTry  = "try"
	TurnHelp = "help"
)

// What a try that finds nothing costs, as Discover.FailedTry says.
const (
	TryUsed = "used"
	TryHalf = "half"
	TryKept = "kept"
)

// What a try came to.
const (
	TryFound   = "found"
	TryClose   = "close"
	TryNothing = "nothing"
)

// Experiment limits: what one try can put in.
const (
	maxTryItems = 6
	maxTryQty   = 20
)

// directorSight is the campaign role that sees every character
// (campaigns.RoleOwner), for deciding who a discovery teaches.
const directorSight = 3

// TurnStore keeps the turns heroes spend. A turn is keyed on the campaign's
// latest respite, so a new respite gives every turn back.
type TurnStore interface {
	// TakeTurn spends the turn; ok is false when it was already spent.
	TakeTurn(ctx context.Context, campaignID, characterID, kind, projectID string, respiteID int64) (ok bool, err error)
	// ReturnTurn gives back a turn whose work then failed.
	ReturnTurn(ctx context.Context, campaignID, characterID, kind, projectID string, respiteID int64) error
	// TurnsTaken lists the turns the character spent since the respite, as
	// kind and project ("try:", "help:<project id>").
	TurnsTaken(ctx context.Context, campaignID, characterID string, respiteID int64) (map[string]bool, error)
	// RespiteID is the campaign's latest respite, 0 before the first.
	RespiteID(ctx context.Context, campaignID string) (int64, error)
}

// TryPick is one item a hero puts into a try, and how many.
type TryPick struct {
	ItemID string `json:"itemId"`
	Qty    int    `json:"qty"`
}

// TryInput is what a hero puts into a try.
type TryInput struct {
	Items []TryPick `json:"items"`
}

// TryView is the block's "Try something new": shown when the campaign has a
// recipe this hero could still find; Ready says the turn is unspent.
type TryView struct {
	Ready bool `json:"ready"`
}

// TryResult is what a try came to. A near miss carries only the recipe's
// hint, never its name or id; Recipe and RecipeID are set only on a find.
type TryResult struct {
	Outcome  string `json:"outcome"`
	Recipe   string `json:"recipe,omitempty"`
	RecipeID string `json:"recipeId,omitempty"`
	Hint     string `json:"hint,omitempty"`
	// Learned names who learned a found recipe; Party says the recipe
	// teaches the whole party.
	Learned []string `json:"learned,omitempty"`
	Party   bool     `json:"party,omitempty"`
	// Cost is what the try cost (TryUsed, TryHalf, TryKept); Used is what
	// left the pack and Back what stayed in it, as "2 × Bloodmoss".
	Cost string   `json:"cost"`
	Used []string `json:"used"`
	Back []string `json:"back"`
}

// BreakInput breaks one made item down. Copy names the remembered copy to
// break, 0 for any.
type BreakInput struct {
	RecipeID string `json:"recipeId"`
	Copy     int64  `json:"copy"`
}

// BreakResult is what breaking an item down gave back.
type BreakResult struct {
	Item  string   `json:"item"`
	Back  []string `json:"back"`
	Notes []string `json:"notes"`
}

// HelpInput is a hand on a party member's project: the hero helping, and
// the project's version the block last read.
type HelpInput struct {
	CharacterID string `json:"characterId"`
	Version     int    `json:"version"`
}

// HelpView is a party member's project this hero may help with; Helped says
// they already did this respite.
type HelpView struct {
	Project ProjectView `json:"project"`
	Helped  bool        `json:"helped"`
}
