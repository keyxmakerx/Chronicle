package crafting

import (
	"context"
	"time"
)

// Project statuses.
const (
	ProjectActive    = "active"
	ProjectMade      = "made"
	ProjectCancelled = "cancelled"
)

// How the steps went, as the bench reports them when the roll is made:
// clean work earns an edge, botched work a bane when the campaign gives
// banes, steady work or a slip is a plain roll, and skipped steps are a
// plain roll the campaign may refuse.
const (
	WorkClean   = "clean"
	WorkBotched = "botched"
	WorkSteady  = "steady"
	WorkSkipped = "skipped"
)

// Limits on projects.
const (
	// maxActiveProjects caps one hero's projects in progress.
	maxActiveProjects = 20
	// maxRollsReady caps the rolls a project can bank, far past any real
	// table's run of respites, so a stuck button can't grow it forever.
	maxRollsReady = 999
	// recentFinished is how many finished projects the Director's tab lists.
	recentFinished = 20
	maxTookLen     = 500
	// maxMadeNotes caps what a project keeps of what its item came out as.
	maxMadeNotes = 12
)

// Project is a hero's crafting project as stored. RecipeName, Goal, Stat
// and the result are what the recipe said when it started.
type Project struct {
	ID           string
	CampaignID   string
	CharacterID  string
	RecipeID     string
	RecipeName   string
	Goal         int
	Stat         string
	ResultItemID string
	ResultQty    int
	Points       int
	RollsReady   int
	Status       string
	Delivered    bool
	Took         string
	StartedBy    string
	StartedAt    time.Time
	FinishedAt   *time.Time
	Version      int
	Details      ProjectDetails
}

// ProjectDetails is what a project keeps beyond its numbers: what its item
// came out as (fine, marked, a quirk, made as something else), and the
// standing cost of each substitute it started with ("bane" or "points").
type ProjectDetails struct {
	Made []string `json:"made,omitempty"`
	Subs []string `json:"subs,omitempty"`
}

// ProjectView is a project as the block and the Projects tab show it.
type ProjectView struct {
	ID          string `json:"id"`
	CharacterID string `json:"characterId"`
	Hero        string `json:"hero"`
	RecipeID    string `json:"recipeId,omitempty"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Roll        string `json:"roll"`
	Goal        int    `json:"goal"`
	Points      int    `json:"points"`
	RollsReady  int    `json:"rollsReady"`
	Status      string `json:"status"`
	// Waits says why a ready roll can't be made today (a recipe's rule),
	// "" when it can.
	Waits string `json:"waits,omitempty"`
	// Makes names what it makes: "1 × Healing potion".
	Makes string `json:"makes"`
	// Undelivered is set on a finished project whose item could not be put
	// in the hero's pack, so the Director can give it by hand.
	Undelivered bool `json:"undelivered,omitempty"`
	// Came names what the item came out as: "Fine", a mark, a quirk.
	Came       []string   `json:"came,omitempty"`
	Took       string     `json:"took,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Version    int        `json:"version"`
}

// PackItem is one thing a hero carries.
type PackItem struct {
	ItemID string   `json:"itemId"`
	Name   string   `json:"name"`
	Qty    int      `json:"qty"`
	Tags   []string `json:"tags"`
}

// Pack is what a hero carries and how they pay, as the viewer may see it.
// PayKind is "" (no money field), "wealth", "purse" or "coins"; Have is in
// hundredths (a purse in copper).
type Pack struct {
	Name       string     `json:"name"`
	Items      []PackItem `json:"items"`
	PayKind    string     `json:"payKind"`
	Have       int64      `json:"have"`
	MoneyLabel string     `json:"moneyLabel"`
	// MoneyKey is the sheet's money field, which decides how a cost in
	// another coin converts; the Armory's own business, never sent.
	MoneyKey string `json:"-"`
}

// CraftNeed is one material line of a recipe a hero can start.
type CraftNeed struct {
	ItemID string `json:"itemId,omitempty"`
	Tag    string `json:"tag,omitempty"`
	Qty    int    `json:"qty"`
	Name   string `json:"name"`
	// Tags are a named item's crafting tags, which tint it on the bench.
	Tags []string `json:"tags,omitempty"`
	// Sub is what may stand in for a named item, and what that costs.
	Sub *CraftSub `json:"sub,omitempty"`
}

// CraftSub lets any item with a tag stand in for a named material; Costs is
// "bane", "points" (−2 to every roll) or "none".
type CraftSub struct {
	Tag   string `json:"tag"`
	Name  string `json:"name"`
	Costs string `json:"costs"`
}

// CraftRecipe is a recipe a hero knows, with what starting it takes, what a
// roll adds and what the bench plays. Rules the book doesn't show never
// reach it.
type CraftRecipe struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Template string      `json:"template"`
	Kind     string      `json:"kind"`
	Bench    string      `json:"bench"`
	Needs    []CraftNeed `json:"needs"`
	Steps    []Step      `json:"steps"`
	Colors   Colors      `json:"colors"`
	Goal     int         `json:"goal"`
	Stat     string      `json:"stat"`
	Roll     string      `json:"roll"`
	// StatValue is the hero's characteristic from their sheet; nil when the
	// recipe names none or the sheet has no number there.
	StatValue *int   `json:"statValue,omitempty"`
	Cost      *Cost  `json:"cost,omitempty"`
	Makes     string `json:"makes"`
	MakesQty  int    `json:"makesQty"`
	// Charge is the cost against this hero's money, in Pack.Have's units.
	Charge int64    `json:"charge"`
	Lines  []string `json:"lines"`
}

// BenchPlay is the campaign's bench of one kind as a player plays it: its
// arrangement for each look, nil when it has never been arranged.
type BenchPlay struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Desk    any    `json:"desk"`
	Journal any    `json:"journal"`
}

// CharacterCrafting is the Crafting block on a character's page.
type CharacterCrafting struct {
	Character Named         `json:"character"`
	Pack      Pack          `json:"pack"`
	Projects  []ProjectView `json:"projects"`
	Recipes   []CraftRecipe `json:"recipes"`
	Benches   []BenchPlay   `json:"benches"`
	// Settings are the campaign's bench settings: the look players start
	// in, the timing, whether botched work gives a bane and skipping.
	Settings Settings `json:"settings"`
	// Edge and Bane are what clean and botched work add to a roll.
	Edge int `json:"edge"`
	Bane int `json:"bane"`
	// CalendarID is the default calendar the bench's window shows, "" when
	// there is none or the viewer can't see it; Today is its date, only
	// then.
	CalendarID string `json:"calendarId,omitempty"`
	Today      string `json:"today,omitempty"`
	// Spoils are the party's spoils tray, as this viewer may see it; nil
	// when spoils aren't set up.
	Spoils   []SpoilView `json:"spoils"`
	Director bool        `json:"director"`
}

// StartInput starts a project. Picks fills each "any <tag>" material, by
// its index in the recipe's materials, with an item the hero carries.
type StartInput struct {
	RecipeID string         `json:"recipeId"`
	Picks    map[int]string `json:"picks"`
}

// RollInput makes a project's roll. Work is how the steps went (WorkClean
// and so on); Bench is the bench it was worked at, "" for none.
type RollInput struct {
	Work    string `json:"work"`
	Bench   string `json:"bench"`
	Version int    `json:"version"`
}

// Term is one line of what a roll adds up.
type Term struct {
	Text string `json:"text"`
	N    int    `json:"n"`
}

// RollResult is a roll as made: the dice as they fell, each term, and what
// it did to the project.
type RollResult struct {
	Project  ProjectView `json:"project"`
	Dice     []int       `json:"dice"`
	Sides    int         `json:"sides"`
	Terms    []Term      `json:"terms"`
	Total    int         `json:"total"`
	From     int         `json:"from"`
	To       int         `json:"to"`
	Finished bool        `json:"finished"`
	// Spoiled says a recipe's chance of "can't be made" came up: the roll
	// was used and added nothing.
	Spoiled bool `json:"spoiled,omitempty"`
	// Made names what reached the pack, "" when nothing did.
	Made string `json:"made,omitempty"`
	// Mishap is what struck mid-brew, nil when nothing did; Row is its row
	// of the table (from 0), which picks its look on the bench.
	Mishap *Mishap `json:"mishap,omitempty"`
	// Breakthrough says the dice came up high enough to roll again.
	Breakthrough bool `json:"breakthrough,omitempty"`
	// Lost says botched work cost the ingredients: the project is over.
	Lost  bool     `json:"lost,omitempty"`
	Notes []string `json:"notes"`
}

// Mishap is a row of a mishap table that struck on a roll.
type Mishap struct {
	Table string `json:"table"`
	Row   int    `json:"row"`
	Text  string `json:"text"`
}

// ProjectsPage is the Director's Projects tab.
type ProjectsPage struct {
	Active   []ProjectView `json:"active"`
	Finished []ProjectView `json:"finished"`
	// LastRespite is when the last respite was logged and the calendar's
	// date then; nil before the first.
	LastRespite *Respite `json:"lastRespite,omitempty"`
	Today       string   `json:"today,omitempty"`
}

// Respite is one logged respite.
type Respite struct {
	DayLabel string    `json:"dayLabel,omitempty"`
	Projects int       `json:"projects"`
	At       time.Time `json:"at"`
}

// RespiteResult says how many projects a respite gave a roll.
type RespiteResult struct {
	Projects int `json:"projects"`
}

// SetPointsInput sets a project's points, below its goal.
type SetPointsInput struct {
	Points  int `json:"points"`
	Version int `json:"version"`
}

// Day is the campaign calendar's today, as a project roll reads it: the
// moons, the events and rolled results on the day, its weather and season,
// and the time of day by the calendar's clock ("dawn", "day", "dusk" or
// "night"; "" when unknown).
type Day struct {
	Time    string
	Label   string
	Moons   []DayMoon
	Events  []string
	Rolled  map[string]string
	Weather string
	Season  string
}

// DayMoon is one moon today: a cycling moon's phase (0 new, 0.5 full), or a
// hand-set moon's state ("" before its first mark).
type DayMoon struct {
	Name   string
	Manual bool
	Phase  float64
	State  string
}

// Armory is the Armory as crafting needs it: a hero's pack, taking a
// project's materials and cost, and putting the made item in the pack. Each
// call checks the viewer may act for the character. Implemented in
// internal/app over the Armory's stash service.
type Armory interface {
	Pack(ctx context.Context, campaignID string, v Viewer, characterID string) (*Pack, error)
	// Charge is a cost against the pack, in Pack.Have's units.
	Charge(p *Pack, c Cost) int64
	// Spend takes the materials and the cost, all or nothing, and says what
	// was paid ("25 gp"), "" when no coins changed hands.
	Spend(ctx context.Context, campaignID string, v Viewer, characterID string, items []PackItem, c Cost, label string) (string, error)
	Give(ctx context.Context, campaignID string, v Viewer, characterID, itemID string, qty int, label string) error
	// GiveSpoil puts what a hero took or harvested from the spoils tray in
	// their pack.
	GiveSpoil(ctx context.Context, campaignID string, v Viewer, characterID, itemID string, qty int, harvested bool) error
}

// Sheets reads a hero's sheet: a characteristic (ok is false when the field
// holds no number), and the skills it lists (ok is false when it lists
// none, so a rule about a skill stays unknown rather than off).
type Sheets interface {
	Stat(ctx context.Context, campaignID, characterID, key string) (value int, ok bool, err error)
	Skills(ctx context.Context, campaignID, characterID string) (skills []string, ok bool, err error)
}

// TableRow is one row of a rolling table, with its relative chance.
type TableRow struct {
	Text   string
	Weight float64
}

// Tables reads the campaign's Rolling Tables, which a recipe's quirks and
// mishaps are rolled on. Implemented in internal/app over the rolltables
// service.
type Tables interface {
	// All lists the campaign's tables with their rows' words, for the rules
	// editor.
	All(ctx context.Context, campaignID string) ([]CatalogTable, error)
	// Rows returns a table's rows by its name; ok is false when the campaign
	// has no table by that name.
	Rows(ctx context.Context, campaignID, name string) (rows []TableRow, ok bool, err error)
}

// Days reads the campaign calendar's today; nil when the campaign has none.
type Days interface {
	Today(ctx context.Context, campaignID string) (*Day, error)
}

// RespiteHook tells the rest of Chronicle a respite was logged from the
// Crafting app, so Homebrew rules set off by a respite run as they would
// from the drawer.
type RespiteHook interface {
	RespiteLogged(ctx context.Context, campaignID string, v Viewer) error
}
