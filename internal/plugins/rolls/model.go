package rolls

import (
	"context"
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// Who may see a roll, the same four modes Foundry has.
const (
	// ModePublic is seen by everyone at the table.
	ModePublic = "public"
	// ModeGM is seen by the Directors and the roller.
	ModeGM = "gm"
	// ModeBlind is seen by the Directors; the roller learns only that it was made.
	ModeBlind = "blind"
	// ModeSelf is seen by the roller alone.
	ModeSelf = "self"
)

// Which app decided a roll's numbers.
const (
	OriginChronicle = "chronicle"
	OriginFoundry   = "foundry"
)

// What made a roll inside its app.
const (
	SourceTable    = "table"
	SourceCrafting = "crafting"
	SourceShop     = "shop"
	SourceFoundry  = "foundry"
)

// The table's dice looks. Each person picks their own; a new player starts on
// the table's.
const (
	LookQuick = "quick"
	LookRune  = "rune"
	LookNone  = "none"
)

// Bounds on one roll, so the log stays small and a hostile client can at worst
// fill its own campaign's log with short rows.
const (
	MaxDice       = 100
	MaxSides      = 1000
	MaxModifier   = 9999
	MaxTotal      = 1000000
	FormulaMax    = 120
	LabelMax      = 160
	NameMax       = 100
	OutcomeMax    = 160
	WhyMax        = 300
	FromMax       = 160
	OriginIDMax   = 80
	DefaultPage   = 50
	MaxPage       = 200
	tableDiceMax  = 50
	tableTermsMax = 20
)

// Die is one die as it fell. Crit is the server's judgement under the
// campaign's game system ("succ", "fail" or empty); it is stored with the die
// and never taken from a client.
type Die struct {
	Sides int    `json:"sides"`
	Value int    `json:"value"`
	Crit  string `json:"crit,omitempty"`
}

// Roll is one roll in the log.
type Roll struct {
	ID         string `json:"id"`
	CampaignID string `json:"-"`
	Origin     string `json:"origin"`
	OriginID   string `json:"originId,omitempty"`
	Source     string `json:"source"`
	RollerID   string `json:"rollerId,omitempty"`
	RollerName string `json:"rollerName"`
	Label      string `json:"label"`
	Formula    string `json:"formula"`
	Dice       []Die  `json:"dice,omitempty"`
	Modifier   int    `json:"modifier"`
	Total      int    `json:"total"`
	Outcome    string `json:"outcome,omitempty"`
	Why        string `json:"why,omitempty"`
	From       string `json:"from,omitempty"`
	Mode       string `json:"mode"`
	// CharacterID is the hero the roll was made for, when it came from a
	// hero's own numbers (the sheet, or the Table's power roll line). It
	// travels on the live message only, so Foundry can speak as the hero's
	// actor; the log does not keep it.
	CharacterID string `json:"characterId,omitempty"`
	// RollerDice is the 3D dice the roll is thrown in on every screen: the
	// roller's own pick where the campaign lets them pick, else the
	// campaign's, settled when the roll is made. Nil on rolls kept from
	// before rolls carried it.
	RollerDice *DiceLook `json:"rollerDice,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// View is a roll as one viewer is told it. A blind roll reaches its roller
// with Hidden set and no dice, modifier, total or outcome.
type View struct {
	ID         string `json:"id"`
	Origin     string `json:"origin"`
	OriginID   string `json:"originId,omitempty"`
	Source     string `json:"source"`
	RollerID   string `json:"rollerId,omitempty"`
	RollerName string `json:"rollerName"`
	Label      string `json:"label"`
	Formula    string `json:"formula"`
	Dice       []Die  `json:"dice,omitempty"`
	Modifier   *int   `json:"modifier,omitempty"`
	Total      *int   `json:"total,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	Why        string `json:"why,omitempty"`
	From       string `json:"from,omitempty"`
	Mode       string `json:"mode"`
	Hidden     bool   `json:"hidden,omitempty"`
	Mine       bool   `json:"mine,omitempty"`
	// CharacterID is Roll.CharacterID; it is absent from the log.
	CharacterID string `json:"characterId,omitempty"`
	// RollerDice is Roll.RollerDice. Old clients ignore it and throw the
	// campaign's dice.
	RollerDice *DiceLook `json:"rollerDice,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// DiceLook is one set of 3D dice: the die (set and colour) and the crit
// success and crit fail animations. An empty field is the dice engine's
// default. Own says the roller's own pick was used rather than the
// campaign's, so a screen can name whose dice they are.
type DiceLook struct {
	Set      string `json:"set"`
	Colour   string `json:"colour"`
	CritSucc string `json:"critSucc"`
	CritFail string `json:"critFail"`
	Own      bool   `json:"own,omitempty"`
}

// DicePick is one member's own dice in one campaign, kept with their account.
// An empty field follows the campaign's. Whether it is honoured is decided
// each time from the campaign's settings, so a narrowed offer never needs the
// stored pick cleaned, and widening it again brings the pick back.
type DicePick struct {
	Set      string `json:"set"`
	Colour   string `json:"colour"`
	CritSucc string `json:"critSucc"`
	CritFail string `json:"critFail"`
}

// IsZero is whether the pick follows the campaign's dice in every field.
func (p DicePick) IsZero() bool { return p == DicePick{} }

// MyDice is what the Table's My dice leaf reads: the stored pick, the dice
// the member's rolls are thrown in right now, and whether they may pick.
type MyDice struct {
	Pick      DicePick `json:"pick"`
	Effective DiceLook `json:"effective"`
	MayPick   bool     `json:"mayPick"`
}

// Viewer is who is asking. Director is true for the owner and members with
// DM access (CampaignContext.VisibilityRole at Owner).
type Viewer struct {
	UserID   string
	Name     string
	Director bool
}

// TableInput is a roll asked for at the Table tab. CharacterID names the
// hero whose numbers the roll uses, when it has one; it is kept only when the
// roller plays that hero or is a Director (CharacterSource).
type TableInput struct {
	Formula     string `json:"formula"`
	Label       string `json:"label"`
	Mode        string `json:"mode"`
	CharacterID string `json:"characterId"`
}

// CharacterIDMax bounds a character id from a client; entity ids are UUIDs.
const CharacterIDMax = 64

// CharacterSource says whether a viewer may roll as a page: a live page of
// the campaign that the viewer has claimed, or any of them for a Director.
// For a public roll the page must also be one every player may see, because
// Foundry shows the linked actor's name and picture to the whole table. It is
// implemented in internal/app over the entities plugin, so this plugin never
// reads pages itself.
type CharacterSource interface {
	MayRollAs(ctx context.Context, campaignID string, v Viewer, characterID, mode string) (bool, error)
}

// ReportInput is a roll another app decided and reports (Foundry). RollerID
// is resolved by the caller from the app's own user, or empty.
type ReportInput struct {
	Origin     string
	OriginID   string
	RollerID   string
	RollerName string
	Label      string
	Formula    string
	Dice       []Die
	Total      int
	Mode       string
}

// LogInput is a roll another plugin made on the server and adds to the log.
type LogInput struct {
	Source     string
	RollerID   string
	RollerName string
	Label      string
	Formula    string
	Dice       []Die
	Modifier   int
	Total      int
	Outcome    string
	Why        string
	From       string
	Mode       string
}

// Page is one page of the log, oldest first, with whether older rolls exist.
type Page struct {
	Rolls []View `json:"rolls"`
	More  bool   `json:"more"`
}

// Quality is how much detail the 3D dice draw. Empty means Auto, which the
// browser measures per device.
const (
	QualityAuto  = "auto"
	QualityBasic = "basic"
	QualityHD    = "hd"
)

// DiceSetKeys and the two crit lists name what the browser's dice engine can
// draw (static/js/dice/engine.js, SET_KEYS and CRIT_KEYS). The names live in
// both places because the server must refuse a name the engine would never
// load; TestDiceKeysMatchEngine in service_test.go keeps the two lists equal.
var (
	DiceSetKeys = []string{
		"jewelled", "confetti", "acrylic", "cage", "heartlight", "gothic", "obsidian", "moonstone", "oak", "nebula", "bone", "scale",
		"damascus", "cloisonne", "kintsugi", "leather", "runestone", "arcana", "relic", "agate", "neon", "amber", "botanical", "clockwork",
	}
	CritSuccKeys = []string{"storm", "phoenix", "dawn", "starfall", "bloom", "hoard", "koi", "prism", "festival", "encore", "fortune", "flutter"}
	CritFailKeys = []string{"grasp", "frost", "shatter", "hex", "crows", "abyss", "mimic", "engulf", "omen", "shackle", "melt", "dust"}
)

// Settings is the campaign's dice: the table's look (what a player who hasn't
// picked starts on) and the 3D dice every roll is drawn with. An empty string
// in a dice field means "the default", so the browser's own defaults apply.
type Settings struct {
	Look       string `json:"look"`
	DiceSet    string `json:"diceSet"`
	DiceColour string `json:"diceColour"`
	CritSucc   string `json:"critSucc"`
	CritFail   string `json:"critFail"`
	Quality    string `json:"quality"`
	// PlayersPick is whether members may pick their own dice from the sets
	// on offer; their rolls are then thrown in their pick on every screen.
	// Off, every roll is thrown in the campaign's dice.
	PlayersPick bool `json:"playersPick"`
	// DiceOffered is the sets a player may pick from, in catalogue order. It
	// is empty when every set is on offer. The campaign's own set is always
	// among them.
	DiceOffered []string `json:"diceOffered"`
}

// UpdateSettingsInput is a partial change to Settings: a key left out keeps
// what is stored. An empty string on a dice field puts it back to the default;
// an explicit null keeps it, since none of these columns can hold NULL.
type UpdateSettingsInput struct {
	Look        patch.Field[string] `json:"look"`
	DiceSet     patch.Field[string] `json:"diceSet"`
	DiceColour  patch.Field[string] `json:"diceColour"`
	CritSucc    patch.Field[string] `json:"critSucc"`
	CritFail    patch.Field[string] `json:"critFail"`
	Quality     patch.Field[string] `json:"quality"`
	PlayersPick patch.Field[bool]   `json:"playersPick"`
	// DiceOffered replaces the whole list; an empty list offers every set.
	DiceOffered patch.Field[[]string] `json:"diceOffered"`
}

// PowerRollLine is the Table's power roll line for one viewer: the game
// system's declared roll (2d10 plus a characteristic), and the viewer's own
// hero's numbers when they are known. Declared is false for a campaign whose
// system declares no power roll, and the Table then shows no line.
type PowerRollLine struct {
	Declared        bool        `json:"declared"`
	Dice            string      `json:"dice,omitempty"`
	Edge            int         `json:"edge,omitempty"`
	Bane            int         `json:"bane,omitempty"`
	Tiers           []int       `json:"tiers,omitempty"`
	Characteristics []PowerStat `json:"characteristics,omitempty"`
	Hero            string      `json:"hero,omitempty"`
	// HeroID is the hero's page id, which the Table sends back with a roll
	// of its numbers so the roll is the hero's.
	HeroID string `json:"heroId,omitempty"`
}

// PowerStat is one characteristic of the power roll. Value is nil when the
// viewer has no hero or the hero's sheet holds no number there.
type PowerStat struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value *int   `json:"value"`
}

// PowerRollSource reads a campaign's power roll and the viewer's own hero.
// It is implemented in internal/app over the game system and the claimed
// character, so this plugin reaches neither directly. canSeeGM is whether the
// viewer sees GM content; without it, a number the GM marked GM-only is left
// out.
type PowerRollSource interface {
	PowerRoll(ctx context.Context, campaignID, userID string, canSeeGM bool) (PowerRollLine, error)
}
