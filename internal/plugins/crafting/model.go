package crafting

import (
	"encoding/json"
	"time"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// MaxBodyBytes caps a request body. A recipe with forty rules and every
// look filled is well under it; a bench arrangement is the largest thing
// sent.
const MaxBodyBytes = 256 << 10

// Viewer is who is asking, as the service needs it.
type Viewer struct {
	UserID         string
	MemberRole     int
	VisibilityRole int
	// Director is the Owner or a co-Director: who may see and change every
	// recipe, bench and tag.
	Director bool
}

// Settings are a campaign's crafting defaults.
type Settings struct {
	// Skin is the look players start in ("desk" or "journal"); they may
	// switch.
	Skin string `json:"skin"`
	// Difficulty is how tight the bench's timing is.
	Difficulty string `json:"difficulty"`
	// Bane is whether botched work gives a bane.
	Bane bool `json:"bane"`
	// Skip is whether players may skip the steps and just roll.
	Skip bool `json:"skip"`
}

// DefaultSettings are what a campaign has before its Director changes any.
func DefaultSettings() Settings {
	return Settings{Skin: "desk", Difficulty: "std", Bane: true, Skip: true}
}

// Material is one thing a recipe asks for: a named Armory item, or any
// item carrying a tag ("any herb"), so players can use what they carry.
type Material struct {
	ItemID string `json:"itemId,omitempty"`
	Tag    string `json:"tag,omitempty"`
	Qty    int    `json:"qty"`
}

// Step is one step of a recipe, in the bench engine's terms. Material is
// an index into the recipe's materials, nil until one is picked.
type Step struct {
	T        string `json:"t"`
	Material *int   `json:"material,omitempty"`
	To       string `json:"to,omitempty"`
	Turns    int    `json:"turns,omitempty"`
	Secs     int    `json:"secs,omitempty"`
	N        int    `json:"n,omitempty"`
}

// Result is the Armory item a recipe makes and how many.
type Result struct {
	ItemID string `json:"itemId,omitempty"`
	Qty    int    `json:"qty"`
}

// Cost is what starting the project costs on top of its materials, in the
// money the campaign uses. A hero whose sheet keeps Wealth needs it rather
// than spending it, as at a shop.
type Cost struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// Colors are the brew's colours on the bench: the water, the ground paste,
// halfway, and done. Empty means the bench's own.
type Colors struct {
	Base  string `json:"base,omitempty"`
	Paste string `json:"paste,omitempty"`
	Mid   string `json:"mid,omitempty"`
	Final string `json:"final,omitempty"`
}

// Recipe is a recipe as its Director edits it.
type Recipe struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Template     string           `json:"template"`
	Bench        string           `json:"bench"`
	Materials    []Material       `json:"materials"`
	Steps        []Step           `json:"steps"`
	Result       Result           `json:"result"`
	Goal         int              `json:"goal"`
	Stat         string           `json:"stat"`
	Prerequisite string           `json:"prerequisite"`
	Source       string           `json:"source"`
	Cost         Cost             `json:"cost"`
	Who          string           `json:"who"`
	Known        []string         `json:"known"`
	Colors       Colors           `json:"colors"`
	Rules        []Rule           `json:"rules"`
	Looks        map[string]*Look `json:"looks"`
	Draft        bool             `json:"draft"`
	DraftPrompt  string           `json:"draftPrompt,omitempty"`
	SortOrder    int              `json:"sortOrder"`
	Version      int              `json:"version"`
	UpdatedAt    time.Time        `json:"updatedAt"`
}

// RecipeInput is what the editor sends to create or change a recipe. Rules
// and looks are saved on their own (RulesInput), so the recipe form and the
// rules editor never overwrite each other. Version is the one the editor
// last read; zero on create.
type RecipeInput struct {
	Name         string     `json:"name"`
	Template     string     `json:"template"`
	Bench        string     `json:"bench"`
	Materials    []Material `json:"materials"`
	Steps        []Step     `json:"steps"`
	Result       Result     `json:"result"`
	Goal         int        `json:"goal"`
	Stat         string     `json:"stat"`
	Prerequisite string     `json:"prerequisite"`
	Source       string     `json:"source"`
	Cost         Cost       `json:"cost"`
	Who          string     `json:"who"`
	Known        []string   `json:"known"`
	Colors       Colors     `json:"colors"`
	Version      int        `json:"version"`
}

// RulesInput saves a recipe's rules and moment looks.
type RulesInput struct {
	Rules   json.RawMessage `json:"rules"`
	Looks   json.RawMessage `json:"looks"`
	Version int             `json:"version"`
}

// Bench is a named place of one bench kind, with each look's arrangement
// from build mode (nil until arranged).
type Bench struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Desk      json.RawMessage `json:"desk"`
	Journal   json.RawMessage `json:"journal"`
	SortOrder int             `json:"sortOrder"`
	Version   int             `json:"version"`
}

// BenchInput names a bench and its kind.
type BenchInput struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Version int    `json:"version"`
}

// ArrangementInput saves one look's arrangement of a bench.
type ArrangementInput struct {
	State   json.RawMessage `json:"state"`
	Version int             `json:"version"`
}

// MaterialItem is an Armory item with its crafting tags, for the Materials
// tab and the recipe editor's pickers.
type MaterialItem struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Tags []string `json:"tags"`
	// Picture is the item's own picture; nil when it follows its first kind.
	Picture *MaterialPicture `json:"picture,omitempty"`
}

// MaterialPicture is a picture from the library (Pictures) and its tint, a
// #rrggbb colour. A kind wears one by default; an item may wear its own.
type MaterialPicture struct {
	Picture string `json:"picture"`
	Tint    string `json:"tint"`
}

// KindPicturesInput changes several kinds' pictures at once, so swapping two
// kinds is one save. A kind mapped to null goes back to its default.
type KindPicturesInput struct {
	Kinds map[string]*MaterialPicture `json:"kinds"`
}

// Named is a page picked by id and shown by name: a character a recipe is
// known by, an item.
type Named struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Catalog is what the Director's editors pick from: the campaign's Armory
// items with their tags, and its characters. Truncated says a list was cut
// short, never silently.
type Catalog struct {
	Items      []MaterialItem `json:"items"`
	Characters []Named        `json:"characters"`
	Truncated  bool           `json:"truncated"`
}

// Library is the fixed lists and the campaign's settings and system rules,
// read by both the editors and the recipe book.
type Library struct {
	// Tags carry each kind's picture as this campaign has it.
	Tags         []Tag                 `json:"tags"`
	TagGroups    []Choice              `json:"tagGroups"`
	Pictures     []Picture             `json:"pictures"`
	Tools        []Tool                `json:"tools"`
	Steps        []StepKind            `json:"steps"`
	BenchKinds   []BenchKind           `json:"benchKinds"`
	Templates    []Template            `json:"templates"`
	Skins        []Choice              `json:"skins"`
	Difficulties []Choice              `json:"difficulties"`
	Moments      []string              `json:"moments"`
	Rules        systems.CraftingRules `json:"rules"`
	// System names the game system the rules come from, "" when none is
	// enabled and the defaults apply.
	System   string   `json:"system"`
	Settings Settings `json:"settings"`
	Director bool     `json:"director"`
}

// BookPage is a recipe as a player reads it in their recipe book: what it
// needs, the method, the project, and the rules written in the book. Rules
// shown only when they happen, secret rules and drafts never reach it.
type BookPage struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Kind         string         `json:"kind"`
	Bench        string         `json:"bench"`
	Needs        []BookMaterial `json:"needs"`
	Method       []string       `json:"method"`
	Makes        string         `json:"makes"`
	MakesQty     int            `json:"makesQty"`
	Goal         int            `json:"goal"`
	Roll         string         `json:"roll"`
	Graded       bool           `json:"graded"`
	Prerequisite string         `json:"prerequisite,omitempty"`
	Source       string         `json:"source,omitempty"`
	Cost         *Cost          `json:"cost,omitempty"`
	Lines        []string       `json:"lines"`
	// KnownBy names the viewer's characters who know a "must be learned"
	// recipe; empty when everyone knows it.
	KnownBy []string `json:"knownBy,omitempty"`
}

// BookMaterial is one line of "You need".
type BookMaterial struct {
	Qty  int    `json:"qty"`
	Name string `json:"name"`
}
