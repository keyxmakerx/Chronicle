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

// Spoil line statuses. A lost line was a one chance find whose study
// failed: nobody gets it.
const (
	SpoilWaiting = "waiting"
	SpoilTaken   = "taken"
	SpoilCleared = "cleared"
	SpoilLost    = "lost"
)

// How a study went. A study's row is written before its dice are rolled,
// with no outcome, and filled in after; a reveal is the Director naming an
// item without a roll.
const (
	StudyMiss     = "miss"
	StudyKnown    = "known"
	StudyLost     = "lost"
	StudyRevealed = "revealed"
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
// Study says nobody may take or harvest it until a hero identifies it with
// a test adding StudyStat ("" for none); Once says the first failed study
// loses it.
type DropLine struct {
	ItemID    string `json:"itemId"`
	Mode      string `json:"mode"`
	Qty       int    `json:"qty,omitempty"`
	Stat      string `json:"stat,omitempty"`
	Yields    []int  `json:"yields,omitempty"`
	Study     bool   `json:"study,omitempty"`
	StudyStat string `json:"studyStat,omitempty"`
	Once      bool   `json:"once,omitempty"`
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
	Study       bool
	StudyStat   string
	Once        bool
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
	// Kinds names the tags in words: "bone, hide".
	Kinds string `json:"kinds,omitempty"`
	Mode  string `json:"mode"`
	Qty   int    `json:"qty,omitempty"`
	Roll  string `json:"roll,omitempty"`
	Tiers []Tier `json:"tiers,omitempty"`
	// Locked says it waits for a study. A player reads "Something
	// unidentified", with no id, name or kinds; the Director reads it all.
	Locked bool       `json:"locked,omitempty"`
	Study  *StudyView `json:"study,omitempty"`
	// Identified names who identified the item by study ("Tobin"), or says
	// the Director revealed it; UsedIn names the party's recipes that use
	// it, when a study showed them.
	Identified string   `json:"identified,omitempty"`
	UsedIn     []string `json:"usedIn,omitempty"`
}

// StudyView is a locked line's study: the test, what each outcome does,
// and who has tried since the last respite.
type StudyView struct {
	Roll  string   `json:"roll"`
	Tiers []string `json:"tiers"`
	// Says is what each outcome does, lowest first.
	Says []string `json:"says"`
	Once bool     `json:"once,omitempty"`
	// Tried lists the misses since the last respite: "Mira rolled 8".
	Tried []string `json:"tried,omitempty"`
	// Wait says why the hero in view can't study it now, "" when they can.
	Wait string `json:"wait,omitempty"`
}

// StudyResult is a study as rolled. Item, Kinds and UsedIn are filled only
// when it landed; Lost says a one chance find is gone.
type StudyResult struct {
	Hero   string   `json:"hero"`
	Dice   []int    `json:"dice"`
	Sides  int      `json:"sides"`
	Terms  []Term   `json:"terms"`
	Total  int      `json:"total"`
	Tier   int      `json:"tier"`
	Tiers  []string `json:"tiers"`
	Known  bool     `json:"known"`
	Lost   bool     `json:"lost,omitempty"`
	Item   string   `json:"item,omitempty"`
	Kinds  string   `json:"kinds,omitempty"`
	Mode   string   `json:"mode"`
	UsedIn []string `json:"usedIn,omitempty"`
	Notes  []string `json:"notes"`
}

// KnownItem is an item the party has identified.
type KnownItem struct {
	ItemID      string
	CharacterID string
	Recipes     bool
	LearnedBy   string
}

// Study is one study or reveal of an item.
type Study struct {
	ID          int64
	ItemID      string
	LineID      int64
	CharacterID string
	Outcome     string
	Rolled      *int
	CreatedBy   string
	At          time.Time
}

// SpoilView is a spoil in the tray. Source is the page's name, or
// "Something the party found" for a player who can't see the page; that
// player isn't told the fight's name either, or that it was put in by hand.
type SpoilView struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Copy   int    `json:"copy"`
	Copies int    `json:"copies"`
	Origin string `json:"origin,omitempty"`
	// ByHand says the Director put it in, not a fight.
	ByHand bool            `json:"byHand,omitempty"`
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
	Hero string `json:"hero"`
	// Item is the item's name, which the taker may see now it is theirs.
	Item  string   `json:"item"`
	Got   string   `json:"got"`
	Qty   int      `json:"qty"`
	Dice  []int    `json:"dice,omitempty"`
	Sides int      `json:"sides,omitempty"`
	Terms []Term   `json:"terms,omitempty"`
	Total int      `json:"total,omitempty"`
	Tier  int      `json:"tier"`
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
