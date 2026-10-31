package crafting

import (
	"encoding/json"
	"strconv"
)

// A recipe's "If it goes wrong" and "Extras" settings, as the rules editor
// writes them and the AI reads them (craft_rules_logic.js cleanExtras). Like
// the rules, cleaning is forgiving: an unknown value falls back to the
// setting's default and a part the editor doesn't know is dropped, so what
// the server stores is what the editor would keep.
// test/js/fixtures/craft_extras_clean.json pins both sides to one answer.

// Botch costs: what botched work at the bench costs on top of its bane.
const (
	BotchBane        = "bane"
	BotchQuirk       = "baneAndQuirk"
	BotchIngredients = "baneAndIngredients"
)

// Botch is what botched work costs, and the table its quirk is rolled on.
type Botch struct {
	Costs      string `json:"costs"`
	QuirkTable string `json:"quirkTable,omitempty"`
}

// Mishaps is a chance, on every roll, of one row of a table striking
// mid-brew.
type Mishaps struct {
	Chance string `json:"chance"`
	Table  string `json:"table"`
}

// Quality is what a fine or a poor harvested part does to the roll.
type Quality struct {
	Fine string `json:"fine"`
	Poor string `json:"poor"`
}

// Substitute lets any item with a tag stand in for one of the recipe's
// materials (For is its index), at a cost.
type Substitute struct {
	For   int    `json:"for"`
	Use   string `json:"use"`
	Costs string `json:"costs"`
}

// Mastery is what a hero earns once they have brewed the recipe cleanly
// enough times.
type Mastery struct {
	AfterCleanBrews int    `json:"afterCleanBrews"`
	Gives           string `json:"gives"`
}

// Discover lets a hero find the recipe by experimenting.
type Discover struct {
	CloseWhen string `json:"closeWhen"`
	Hint      string `json:"hint,omitempty"`
	LearnedBy string `json:"learnedBy"`
	FailedTry string `json:"failedTry"`
}

// Keeps is how long what it makes lasts, counted on the campaign calendar.
type Keeps struct {
	Days        int    `json:"days"`
	ThenBecomes string `json:"thenBecomes"`
}

// Salvage is what breaking the made item down gives back.
type Salvage struct {
	Item string `json:"item"`
	Qty  int    `json:"qty"`
}

// Extras are a recipe's "If it goes wrong" and "Extras" settings. A nil
// pointer is a setting turned off.
type Extras struct {
	Botch         Botch        `json:"botch"`
	Mishaps       *Mishaps     `json:"mishaps"`
	Quality       *Quality     `json:"quality"`
	Substitutes   []Substitute `json:"substitutes"`
	Breakthrough  bool         `json:"breakthrough"`
	Mastery       *Mastery     `json:"mastery"`
	Discover      *Discover    `json:"discover"`
	Keeps         *Keeps       `json:"keeps"`
	MakersMark    bool         `json:"makersMark"`
	Salvage       []Salvage    `json:"salvage"`
	OthersCanHelp bool         `json:"othersCanHelp"`
}

// Extras limits, the editor's.
const (
	maxSubstitutes = 8
	maxSalvage     = 4
	maxKeepDays    = 360
	maxHint        = 240
)

var (
	qualityFine    = []string{"edge", "points", "extra", "none"}
	qualityPoor    = []string{"bane", "points", "none"}
	substituteCost = []string{"bane", "points", "none"}
	masteryAfter   = []int{2, 3, 5, 10}
	masteryGives   = []string{"skip", "edge", "points"}
	learnedBy      = []string{"maker", "party"}
	failedTry      = []string{"used", "half", "kept"}
)

// DefaultExtras is a recipe with nothing turned on: botched work costs only
// its bane.
func DefaultExtras() Extras {
	return Extras{Botch: Botch{Costs: BotchBane}, Substitutes: []Substitute{}, Salvage: []Salvage{}}
}

// CleanExtras reads a recipe's extras as the AI or the editor wrote them.
// materials is how many materials the recipe has, which bounds a
// substitute's For.
func CleanExtras(raw json.RawMessage, materials int) Extras {
	out := DefaultExtras()
	var m map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return out
	}
	if b, ok := m["botch"].(map[string]any); ok {
		switch c, _ := b["costs"].(string); c {
		case BotchQuirk:
			out.Botch = Botch{Costs: c, QuirkTable: str(b["quirkTable"], maxRuleText)}
			if out.Botch.QuirkTable == "" {
				out.Botch = Botch{Costs: BotchBane}
			}
		case BotchIngredients:
			out.Botch.Costs = c
		}
	}
	if x, ok := m["mishaps"].(map[string]any); ok {
		if t := str(x["table"], maxRuleText); t != "" {
			c, _ := x["chance"].(string)
			if c == "always" || !oneOfStr(chances, c) {
				c = "1 in 6"
			}
			out.Mishaps = &Mishaps{Chance: c, Table: t}
		}
	}
	if x, ok := m["quality"].(map[string]any); ok {
		out.Quality = &Quality{Fine: pick(x["fine"], qualityFine), Poor: pick(x["poor"], qualityPoor)}
	}
	if list, ok := m["substitutes"].([]any); ok {
		seen := map[int]bool{}
		for _, v := range list {
			x, ok := v.(map[string]any)
			if !ok || len(out.Substitutes) >= maxSubstitutes {
				continue
			}
			i, finite := number(x["for"])
			use, _ := x["use"].(string)
			if !finite || i < 0 || i >= materials || seen[i] || !tagKnown(use) {
				continue
			}
			seen[i] = true
			out.Substitutes = append(out.Substitutes, Substitute{For: i, Use: use, Costs: pick(x["costs"], substituteCost)})
		}
	}
	out.Breakthrough = m["breakthrough"] == true
	if x, ok := m["mastery"].(map[string]any); ok {
		n, _ := number(x["afterCleanBrews"])
		after := 3
		for _, a := range masteryAfter {
			if a == n {
				after = n
			}
		}
		out.Mastery = &Mastery{AfterCleanBrews: after, Gives: pick(x["gives"], masteryGives)}
	}
	if x, ok := m["discover"].(map[string]any); ok {
		close, _ := x["closeWhen"].(string)
		if !closeKnown(close, materials) {
			close = ""
			if materials > 1 {
				close = "1 of " + strconv.Itoa(materials)
			}
		}
		out.Discover = &Discover{CloseWhen: close, Hint: str(x["hint"], maxHint), LearnedBy: pick(x["learnedBy"], learnedBy), FailedTry: pick(x["failedTry"], failedTry)}
	}
	if x, ok := m["keeps"].(map[string]any); ok {
		n, finite := number(x["days"])
		if !finite {
			n = 30
		}
		out.Keeps = &Keeps{Days: clamp(n, 1, maxKeepDays), ThenBecomes: str(x["thenBecomes"], maxRuleText)}
	}
	out.MakersMark = m["makersMark"] == true
	if list, ok := m["salvage"].([]any); ok {
		for _, v := range list {
			x, ok := v.(map[string]any)
			if !ok || len(out.Salvage) >= maxSalvage {
				continue
			}
			item := str(x["item"], maxRuleText)
			if item == "" {
				continue
			}
			n, finite := number(x["qty"])
			if !finite {
				n = 1
			}
			out.Salvage = append(out.Salvage, Salvage{Item: item, Qty: clamp(n, 1, 2)})
		}
	}
	out.OthersCanHelp = m["othersCanHelp"] == true
	return out
}

// pick is v when it is one of opts, else the first.
func pick(v any, opts []string) string {
	s, _ := v.(string)
	if oneOfStr(opts, s) {
		return s
	}
	return opts[0]
}

// closeKnown is "k of n" for the recipe's n materials, 0 < k < n.
func closeKnown(s string, n int) bool {
	for k := 1; k < n; k++ {
		if s == strconv.Itoa(k)+" of "+strconv.Itoa(n) {
			return true
		}
	}
	return false
}
