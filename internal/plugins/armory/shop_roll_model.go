// shop_roll_model.go holds the types of the shop counter rolls: Haggle and
// Look closer. The server throws the dice, so the page only shows them.
package armory

import (
	"context"
	"regexp"
	"strconv"
)

// The two counter actions.
const (
	RollHaggle = "haggle"
	RollLook   = "look"
)

// Limits on an owner's hidden note and extra, in characters.
const maxGoodSecretChars = 400

// ShopTestStat is one sheet number a shop test may add. Score marks a D&D
// ability score, where the test adds floor((value-10)/2).
type ShopTestStat struct {
	Key   string
	Label string
	Score bool
}

// ShopTestRules is how a shop test is thrown under the campaign's game
// system. It mirrors the systems package's resolved rules so this plugin never
// imports it; internal/app converts.
type ShopTestRules struct {
	Dice   string
	Style  string // "tiers" or "dc"
	Stats  []ShopTestStat
	Haggle string // default stat keys; empty adds nothing
	Look   string
	Tiers  []int // style tiers: totals up to Tiers[0] are tier 1, up to Tiers[1] tier 2
	Edge   int   // style tiers: what an easy keeper adds
	Bane   int   // style tiers: what a hard keeper adds
	// DCEasy, DCUsual and DCHard are the difficulties of style dc.
	DCEasy, DCUsual, DCHard int
}

// Test styles, as the systems package names them.
const (
	testStyleTiers = "tiers"
	testStyleDC    = "dc"
)

// ShopTestRulesReader decides the rules for a campaign: its enabled game
// system's, else the defaults. Implemented in internal/app.
type ShopTestRulesReader interface {
	ShopTestRules(ctx context.Context, campaignID string) ShopTestRules
}

// SheetStatReader reads one number off a character's sheet. ok is false when
// the field is missing or holds no number. Implemented in internal/app over
// the same sheet reader crafting uses.
type SheetStatReader interface {
	Stat(ctx context.Context, campaignID, characterID, key string) (value int, ok bool, err error)
}

// ShopRollLogEntry is a counter roll as the Table's log shows it. It never
// carries a hidden note or extra.
type ShopRollLogEntry struct {
	RollerID   string
	RollerName string
	Label      string
	Formula    string
	Dice       []RollDie
	Modifier   int
	Total      int
	Outcome    string
	From       string
}

// ShopRollLog adds a roll to the campaign's public roll log and returns the
// log's id for it ("" when it could not be logged). A failed log never fails
// the roll. Implemented in internal/app over the rolls plugin.
type ShopRollLog interface {
	Rolled(ctx context.Context, campaignID string, e ShopRollLogEntry) string
}

// GoodSecret is the owner's hidden note on a shop good, and the extra a great
// look reveals as well. Players read it only through a reveal.
type GoodSecret struct {
	Note  string `json:"note"`
	Extra string `json:"extra"`
}

// ShopRoll is one stored counter roll.
type ShopRoll struct {
	ID           int64
	CampaignID   string
	ShopEntityID string
	RelationID   int
	CharacterID  string
	Action       string
	ShelfDay     int64
	Tier         int
	// Pct is a haggle's price change in percent (negative is a discount).
	Pct   int
	Total int
}

// RollDie is one die as it fell.
type RollDie struct {
	Sides int `json:"sides"`
	Value int `json:"value"`
}

// RollInput is the body of a counter roll.
type RollInput struct {
	RelationID  RelationRef `json:"relationId"`
	CharacterID string      `json:"characterId"`
	Action      string      `json:"action"`
}

// RollStatView says what a test adds from the sheet. Value, Missing and Mod
// are absent when no character is chosen (the owner's settings view).
type RollStatView struct {
	Stat    string `json:"stat"`
	Label   string `json:"label"`
	Value   *int   `json:"value,omitempty"`
	Missing *bool  `json:"missing,omitempty"`
	Mod     *int   `json:"mod,omitempty"`
}

// RollTestView describes the test: the dice, what each action adds, and what
// the total is held against.
type RollTestView struct {
	Dice   string       `json:"dice"`
	Style  string       `json:"style"`
	Haggle RollStatView `json:"haggle"`
	Look   RollStatView `json:"look"`
	// Against is "Easy", "Usual" or "Hard" under style tiers, "DC 15" under dc.
	Against string `json:"against"`
	// StubbornMod is what the keeper's stubbornness adds under style tiers
	// (the edge, 0 or the bane); 0 under dc, where it moves the DC instead.
	StubbornMod int `json:"stubbornMod"`
}

// RollStatChoice is a stat the owner may pick for an action.
type RollStatChoice struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// RollDefaults are the system's own stat keys for each action.
type RollDefaults struct {
	Haggle string `json:"haggle"`
	Look   string `json:"look"`
}

// GoodHaggleView is a haggle made today. Price and Was are unit prices in the
// listing's own units.
type GoodHaggleView struct {
	Tier  int     `json:"tier"`
	Pct   int     `json:"pct"`
	Price float64 `json:"price"`
	Was   float64 `json:"was"`
}

// GoodLookView is what a character has uncovered about a good. Note and Extra
// are empty at tier 1 and where nothing was written. Today says a look was
// made today, so another is refused.
type GoodLookView struct {
	Tier  int    `json:"tier"`
	Note  string `json:"note"`
	Extra string `json:"extra"`
	Today bool   `json:"today"`
}

// RollGoodView is one good's counter state for the character.
type RollGoodView struct {
	Haggle *GoodHaggleView `json:"haggle"`
	Look   *GoodLookView   `json:"look"`
}

// RollsView is the response of GET .../rolls. Goods is keyed by relation id.
type RollsView struct {
	Haggle   bool                    `json:"haggle"`
	Look     bool                    `json:"look"`
	Test     RollTestView            `json:"test"`
	Stats    []RollStatChoice        `json:"stats"`
	Defaults RollDefaults            `json:"defaults"`
	Goods    map[string]RollGoodView `json:"goods"`
}

// RollHaggleResult is a haggle's effect. Price and Was are numbers in the
// listing's units; Currency is the listing's currency.
type RollHaggleResult struct {
	Pct      int     `json:"pct"`
	Price    float64 `json:"price"`
	Was      float64 `json:"was"`
	Currency string  `json:"currency"`
}

// RollLookResult is what a look uncovered. Both are empty at tier 1.
type RollLookResult struct {
	Note  string `json:"note"`
	Extra string `json:"extra"`
}

// RollResult is the response of POST .../rolls.
type RollResult struct {
	Action      string            `json:"action"`
	Dice        []RollDie         `json:"dice"`
	Modifier    int               `json:"modifier"`
	Total       int               `json:"total"`
	Tier        int               `json:"tier"`
	Label       string            `json:"label"`
	Outcome     string            `json:"outcome"`
	StatMissing bool              `json:"statMissing"`
	Haggle      *RollHaggleResult `json:"haggle,omitempty"`
	Look        *RollLookResult   `json:"look,omitempty"`
	RollID      string            `json:"rollId"`
}

// diceRe reads NdS; the systems package has already validated a manifest's
// dice, so this only guards against a caller handing in something else.
var diceRe = regexp.MustCompile(`^([1-9]\d?)d([1-9]\d{0,2})$`)

// parseDice reads "2d10" into its count and sides.
func parseDice(s string) (count, sides int, ok bool) {
	m := diceRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, false
	}
	count, _ = strconv.Atoi(m[1])
	sides, _ = strconv.Atoi(m[2])
	return count, sides, true
}
