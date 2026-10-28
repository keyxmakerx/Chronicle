package crafting

import (
	"context"
	"time"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// How a drop line is taken: as it is, or harvested with a test whose
// three tiers each pay an amount.
const (
	DropTake    = "take"
	DropHarvest = "harvest"
)

// Spoil line statuses.
const (
	SpoilWaiting = "waiting"
	SpoilTaken   = "taken"
	SpoilCleared = "cleared"
)

// Limits on drop lists and the tray.
const (
	maxDropLines = 20
	// maxDropLists bounds the Spoils tab's list, far past a real table.
	maxDropLists = 1000
	// maxSpoilCopies caps how many copies one source drops at once, by
	// hand or from one fight row.
	maxSpoilCopies = 20
	// maxTrayLines caps the waiting lines, so a stuck button or a giant
	// fight can't fill the tray without end.
	maxTrayLines = 500
	recentTaken  = 20
	maxOriginLen = 200
	maxPageName  = 200
)

// DropLine is one thing a page drops. A taken line gives Qty; a harvest
// adds Stat ("" for none) to the system's dice and pays Yields[tier].
type DropLine struct {
	ItemID string `json:"itemId"`
	Mode   string `json:"mode"`
	Qty    int    `json:"qty,omitempty"`
	Stat   string `json:"stat,omitempty"`
	Yields []int  `json:"yields,omitempty"`
}

// DropList is what a page leaves when it is defeated, harvested or found.
// Also names a page whose list drops first, such as a Remnant's original;
// it is followed one step only.
type DropList struct {
	PageID  string
	Also    string
	Lines   []DropLine
	Version int
}

// DropInput saves a page's drop list; Version 0 makes a new one.
type DropInput struct {
	Also    string     `json:"also"`
	Lines   []DropLine `json:"lines"`
	Version int        `json:"version"`
}

// DropView is a drop list as the Spoils tab shows it.
type DropView struct {
	PageID   string     `json:"pageId"`
	Page     string     `json:"page"`
	Kind     string     `json:"kind"`
	Also     string     `json:"also,omitempty"`
	AlsoName string     `json:"alsoName,omitempty"`
	Lines    []DropLine `json:"lines"`
	Version  int        `json:"version"`
}

// Spoil is one thing that dropped, with its lines as the drop list read
// then. Copy and Copies number the copies of one creature that dropped
// together ("Troll, 1 of 2"); Origin is the fight's name, "" when the
// Director put it in by hand.
type Spoil struct {
	ID         string
	CampaignID string
	PageID     string
	PageName   string
	Copy       int
	Copies     int
	Origin     string
	CreatedBy  string
	CreatedAt  time.Time
	Lines      []SpoilLine
}

// SpoilLine is one line of a spoil. ItemName is the item's name when it
// dropped, for the Director's history; what a player is shown comes from
// what they may see now.
type SpoilLine struct {
	ID          int64
	SpoilID     string
	ItemID      string
	ItemName    string
	Mode        string
	Qty         int
	Stat        string
	Yields      [3]int
	Status      string
	CharacterID string
	TakenBy     string
	TakenQty    int
	Rolled      *int
	TakenAt     *time.Time
}

// Tier is one outcome of a harvest's test: the totals it covers and what
// it pays.
type Tier struct {
	Label string `json:"label"`
	Qty   int    `json:"qty"`
}

// SpoilLineView is a waiting line as the viewer may see it. An item a
// player can't see has no id or kinds and reads as unseen until taken.
type SpoilLineView struct {
	ID     int64    `json:"id"`
	ItemID string   `json:"itemId,omitempty"`
	Name   string   `json:"name"`
	Unseen bool     `json:"unseen,omitempty"`
	Tags   []string `json:"tags"`
	Mode   string   `json:"mode"`
	Qty    int      `json:"qty,omitempty"`
	Roll   string   `json:"roll,omitempty"`
	Tiers  []Tier   `json:"tiers,omitempty"`
}

// SpoilView is a spoil in the tray. Source is the page's name, or
// "Something the party found" for a player who can't see the page.
type SpoilView struct {
	ID     string          `json:"id"`
	Source string          `json:"source"`
	Copy   int             `json:"copy"`
	Copies int             `json:"copies"`
	Origin string          `json:"origin,omitempty"`
	At     time.Time       `json:"at"`
	Lines  []SpoilLineView `json:"lines"`
}

// TakenView is one line taken lately, for the Director.
type TakenView struct {
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// SpoilsPage is the Director's Spoils tab: the tray, what was taken lately,
// every drop list, and what the editor and the tray's buttons pick from.
type SpoilsPage struct {
	Tray   []SpoilView            `json:"tray"`
	Taken  []TakenView            `json:"taken"`
	Drops  []DropView             `json:"drops"`
	Heroes []Named                `json:"heroes"`
	Items  []MaterialItem         `json:"items"`
	Stats  []systems.CraftingStat `json:"stats"`
	Tiers  []string               `json:"tiers"`
	Dice   string                 `json:"dice"`
	// Truncated says a list of items or heroes was cut short.
	Truncated bool `json:"truncated,omitempty"`
}

// TakeInput takes or harvests a line for a hero.
type TakeInput struct {
	CharacterID string `json:"characterId"`
}

// TakeResult is a line taken or harvested: what reached the hero's pack
// and, for a harvest, the test as rolled.
type TakeResult struct {
	Hero  string   `json:"hero"`
	Got   string   `json:"got"`
	Qty   int      `json:"qty"`
	Dice  []int    `json:"dice,omitempty"`
	Sides int      `json:"sides,omitempty"`
	Terms []Term   `json:"terms,omitempty"`
	Total int      `json:"total,omitempty"`
	Tier  int      `json:"tier,omitempty"`
	Tiers []Tier   `json:"tiers,omitempty"`
	Notes []string `json:"notes"`
}

// PutInput puts a page's drop list in the tray by hand, Copies times.
type PutInput struct {
	PageID string `json:"pageId"`
	Copies int    `json:"copies"`
	Origin string `json:"origin"`
}

// PutResult says how many spoils went in the tray.
type PutResult struct {
	Spoils int `json:"spoils"`
}

// Fight is a fight that ended, as crafting hears it: each creature row,
// with how many of its copies were down.
type Fight struct {
	Name      string
	Creatures []FightCreature
}

// FightCreature is one creature row of a fight. PageID is "" for a row
// with no page.
type FightCreature struct {
	PageID string
	Name   string
	Down   int
	Copies int
}

// FightSpoils is what a fight left, a line per creature, for the Director.
type FightSpoils struct {
	Lines  []string
	Spoils int
}

// PageInfo is any page of the campaign, as a drop list names it.
type PageInfo struct {
	ID   string
	Name string
	Kind string
}

// Pages reads the campaign's pages of every kind for drop lists, and
// whether a viewer may see one. Implemented in internal/app over the
// entities plugin.
type Pages interface {
	// Page returns nil when there is no such page in this campaign.
	Page(ctx context.Context, campaignID, id string) (*PageInfo, error)
	CanSee(ctx context.Context, campaignID, id string, role int, userID string) (bool, error)
}
