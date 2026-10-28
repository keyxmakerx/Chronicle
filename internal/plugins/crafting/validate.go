package crafting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// Limits on what one recipe or bench may hold. Counts the engine clamps are
// clamped here too, to the same ranges; an id or a name that cannot be used
// is refused with what to fix, since a silent change would make the recipe
// ask for something else.
const (
	maxNameLen      = 120
	maxBenchNameLen = 80
	maxTextLen      = 200
	maxMaterials    = 12
	maxSteps        = 24
	maxQty          = 99
	maxGoal         = 999
	maxKnown        = 200
	maxCostAmount   = 1e9
	maxCurrencyLen  = 12
)

var (
	colorRe    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	currencyRe = regexp.MustCompile(`^\pL[\pL .]*$`)
	statRe     = regexp.MustCompile(`^\pL[\pL\pN '’.()/-]*$`)
)

// recipeFacts is what a recipe is checked against: the campaign's Armory
// items and characters, the ids the stored recipe already holds (an item
// deleted since still saves, and the editor says it is gone), and the
// system's characteristics.
type recipeFacts struct {
	Items          map[string]bool
	Characters     map[string]bool
	KeptItems      map[string]bool
	KeptCharacters map[string]bool
	Rules          systems.CraftingRules
}

func (f recipeFacts) item(id string) bool      { return f.Items[id] || f.KeptItems[id] }
func (f recipeFacts) character(id string) bool { return f.Characters[id] || f.KeptCharacters[id] }

// cleanRecipe checks a recipe the editor or an AI sent and returns it as it
// is stored.
func cleanRecipe(in RecipeInput, f recipeFacts) (Recipe, error) {
	var r Recipe
	var err error
	if r.Name, err = cleanName(in.Name, maxNameLen, "the recipe's name"); err != nil {
		return r, err
	}
	if in.Template != "" && !templateKnown(in.Template) {
		return r, errInvalid(fmt.Sprintf("there is no template %q", in.Template))
	}
	r.Template = in.Template
	r.Bench = in.Bench
	if r.Bench == "" {
		r.Bench = BenchKinds[0].Key
	}
	if benchKind(r.Bench) == nil {
		return r, errInvalid(fmt.Sprintf("there is no bench kind %q", in.Bench))
	}

	if len(in.Materials) > maxMaterials {
		return r, errInvalid(fmt.Sprintf("a recipe asks for at most %d materials", maxMaterials))
	}
	r.Materials = make([]Material, 0, len(in.Materials))
	for i, m := range in.Materials {
		switch {
		case m.ItemID != "" && m.Tag != "":
			return r, errInvalid(fmt.Sprintf("material %d is either an item or a tag, not both", i+1))
		case m.ItemID != "":
			if !f.item(m.ItemID) {
				return r, errInvalid(fmt.Sprintf("material %d is not an Armory item in this campaign", i+1))
			}
		case m.Tag != "":
			if !tagKnown(m.Tag) {
				return r, errInvalid(fmt.Sprintf("material %d: there is no crafting tag %q", i+1, m.Tag))
			}
		default:
			return r, errInvalid(fmt.Sprintf("material %d needs an item or a tag", i+1))
		}
		r.Materials = append(r.Materials, Material{ItemID: m.ItemID, Tag: m.Tag, Qty: clamp(m.Qty, 1, maxQty)})
	}

	if len(in.Steps) > maxSteps {
		return r, errInvalid(fmt.Sprintf("a recipe has at most %d steps", maxSteps))
	}
	r.Steps = make([]Step, 0, len(in.Steps))
	for i, s := range in.Steps {
		k := stepKind(s.T)
		if k == nil {
			return r, errInvalid(fmt.Sprintf("step %d: there is no step %q", i+1, s.T))
		}
		out := Step{T: k.Key}
		if k.Item && s.Material != nil && *s.Material >= 0 && *s.Material < len(r.Materials) {
			n := *s.Material
			out.Material = &n
		}
		if len(k.Into) > 0 {
			out.To = k.Into[0]
			if oneOfStr(k.Into, s.To) {
				out.To = s.To
			}
		}
		if o := k.Option; o != nil {
			v := map[string]int{"turns": s.Turns, "secs": s.Secs, "n": s.N}[o.Key]
			if v == 0 {
				v = o.Default
			}
			v = clamp(v, o.Min, o.Max)
			switch o.Key {
			case "turns":
				out.Turns = v
			case "secs":
				out.Secs = v
			case "n":
				out.N = v
			}
		}
		r.Steps = append(r.Steps, out)
	}

	if in.Result.ItemID != "" && !f.item(in.Result.ItemID) {
		return r, errInvalid("the item it makes is not an Armory item in this campaign")
	}
	r.Result = Result{ItemID: in.Result.ItemID, Qty: clamp(in.Result.Qty, 1, maxQty)}
	r.Goal = clamp(in.Goal, 1, maxGoal)

	if r.Stat, err = cleanStat(in.Stat, f.Rules); err != nil {
		return r, err
	}
	if r.Prerequisite, err = cleanText(in.Prerequisite, f.Rules.Labels.Prerequisite); err != nil {
		return r, err
	}
	if r.Source, err = cleanText(in.Source, f.Rules.Labels.Source); err != nil {
		return r, err
	}
	if r.Cost, err = cleanCost(in.Cost); err != nil {
		return r, err
	}

	switch in.Who {
	case "", "all":
		r.Who = "all"
	case "learned":
		r.Who = "learned"
	default:
		return r, errInvalid(`who knows it is "all" or "learned"`)
	}
	if len(in.Known) > maxKnown {
		return r, errInvalid(fmt.Sprintf("at most %d characters can be listed as knowing a recipe", maxKnown))
	}
	seen := map[string]bool{}
	r.Known = make([]string, 0, len(in.Known))
	for _, id := range in.Known {
		if seen[id] {
			continue
		}
		if !f.character(id) {
			return r, errInvalid("someone listed as knowing it is not a character in this campaign")
		}
		seen[id] = true
		r.Known = append(r.Known, id)
	}
	r.Colors = Colors{Base: cleanColor(in.Colors.Base), Paste: cleanColor(in.Colors.Paste), Mid: cleanColor(in.Colors.Mid), Final: cleanColor(in.Colors.Final)}
	return r, nil
}

// cleanName trims a name and checks it is there, short enough and plain.
func cleanName(s string, max int, what string) (string, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return "", errInvalid(what + " is empty")
	case len([]rune(s)) > max:
		return "", errInvalid(fmt.Sprintf("%s is longer than %d characters", what, max))
	case hasControl(s):
		return "", errInvalid(what + " has a character that cannot be shown")
	}
	return s, nil
}

func cleanText(s, label string) (string, error) {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > maxTextLen {
		return "", errInvalid(fmt.Sprintf("%s is longer than %d characters", strings.ToLower(label), maxTextLen))
	}
	if hasControl(s) {
		return "", errInvalid(strings.ToLower(label) + " has a character that cannot be shown")
	}
	return s, nil
}

// cleanStat checks the characteristic a project roll adds. A system that
// lists its characteristics allows only those; with none listed it is a
// short label, or nothing.
func cleanStat(s string, rules systems.CraftingRules) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if len(rules.Characteristics) > 0 {
		for _, c := range rules.Characteristics {
			if c.Key == s || strings.EqualFold(c.Label, s) {
				return c.Key, nil
			}
		}
		return "", errInvalid(fmt.Sprintf("the project roll adds one of the system's characteristics, not %q", s))
	}
	if len([]rune(s)) > 40 || !statRe.MatchString(s) {
		return "", errInvalid("the characteristic is up to 40 plain characters")
	}
	return s, nil
}

func cleanCost(c Cost) (Cost, error) {
	if math.IsNaN(c.Amount) || c.Amount < 0 || c.Amount > maxCostAmount {
		return Cost{}, errInvalid("the cost is a number of coins, zero or more")
	}
	cur := strings.TrimSpace(c.Currency)
	if len([]rune(cur)) > maxCurrencyLen || (cur != "" && !currencyRe.MatchString(cur)) {
		return Cost{}, errInvalid(fmt.Sprintf("the cost's coin is up to %d letters, like gp", maxCurrencyLen))
	}
	return Cost{Amount: math.Round(c.Amount*100) / 100, Currency: cur}, nil
}

func cleanColor(s string) string {
	if colorRe.MatchString(s) {
		return strings.ToLower(s)
	}
	return ""
}

func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// cleanSettings checks the campaign's crafting defaults.
func cleanSettings(s Settings) (Settings, error) {
	if !choiceKnown(Skins, s.Skin) {
		return s, errInvalid(`the look is "desk" or "journal"`)
	}
	if !choiceKnown(Difficulties, s.Difficulty) {
		return s, errInvalid(`the difficulty is "easy", "std" or "hard"`)
	}
	return s, nil
}

// cleanTags checks an item's crafting tags, in the library's order.
func cleanTags(tags []string) ([]string, error) {
	want := map[string]bool{}
	for _, t := range tags {
		if !tagKnown(t) {
			return nil, errInvalid(fmt.Sprintf("there is no crafting tag %q", t))
		}
		want[t] = true
	}
	out := make([]string, 0, len(want))
	for _, t := range Tags {
		if want[t.Key] {
			out = append(out, t.Key)
		}
	}
	return out, nil
}

// knownTags reads stored tags, keeping the ones the library still has, so
// a retired tag never hides an item's others.
func knownTags(tags []string) []string {
	have := map[string]bool{}
	for _, t := range tags {
		have[t] = true
	}
	out := []string{}
	for _, t := range Tags {
		if have[t.Key] {
			out = append(out, t.Key)
		}
	}
	return out
}

// Limits on a bench arrangement, the editor's own caps (craft_bench_editor.js)
// with room to spare, so a skin's settings and the Director's words fit and
// nothing larger is kept.
const (
	maxLookKeys   = 40
	maxLookValue  = 120
	maxLookList   = 24
	maxPieces     = 80
	maxPieceActs  = 12
	maxPieceName  = 60
	maxPieceText  = 200
	maxPieceMore  = 600
	maxActLabel   = 40
	maxActSay     = 80
	maxPieceShift = 900
)

var (
	optKeyRe = regexp.MustCompile(`^[a-z][a-zA-Z0-9]{0,31}$`)
	pieceRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,47}$`)
	fxRe     = regexp.MustCompile(`^(|shake|sparkle|smoke|glow|b:[a-z][a-z0-9_-]{0,23})$`)
	soundRe  = regexp.MustCompile(`^[a-z]{0,16}$`)
)

type arrangement struct {
	Look   map[string]any   `json:"look"`
	Pieces map[string]piece `json:"pieces"`
}

type piece struct {
	Name   string     `json:"name"`
	Text   string     `json:"text"`
	More   string     `json:"more"`
	DX     int        `json:"dx"`
	DY     int        `json:"dy"`
	Hidden bool       `json:"hidden"`
	Acts   []pieceAct `json:"acts"`
}

type pieceAct struct {
	Label string `json:"label"`
	FX    string `json:"fx"`
	Sound string `json:"sound"`
	Say   string `json:"say"`
}

// cleanArrangement keeps a build-mode state ({look, pieces}) within bounds:
// the look's settings are short plain values the skin checks again on load,
// and each piece keeps its words, its move and its actions made from the
// engine's fixed effects. A setting or piece it cannot read is dropped.
func cleanArrangement(raw json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Look   map[string]any             `json:"look"`
		Pieces map[string]json.RawMessage `json:"pieces"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &in) != nil {
		return nil, errInvalid("the arrangement is not readable")
	}
	out := arrangement{Look: map[string]any{}, Pieces: map[string]piece{}}
	for k, v := range in.Look {
		if len(out.Look) >= maxLookKeys || !optKeyRe.MatchString(k) {
			continue
		}
		switch x := v.(type) {
		case string:
			out.Look[k] = str(x, maxLookValue)
		case float64:
			if !math.IsNaN(x) && !math.IsInf(x, 0) {
				out.Look[k] = math.Round(x)
			}
		case bool:
			out.Look[k] = x
		case []any:
			list := []string{}
			for _, e := range x {
				if s, ok := e.(string); ok && len(list) < maxLookList {
					list = append(list, str(s, maxLookValue))
				}
			}
			out.Look[k] = list
		}
	}
	for id, rawPiece := range in.Pieces {
		if len(out.Pieces) >= maxPieces || !pieceRe.MatchString(id) {
			continue
		}
		var p map[string]any
		if json.Unmarshal(rawPiece, &p) != nil {
			continue
		}
		dx, _ := number(p["dx"])
		dy, _ := number(p["dy"])
		cp := piece{Name: str(p["name"], maxPieceName), Text: str(p["text"], maxPieceText), More: str(p["more"], maxPieceMore),
			DX: clamp(dx, -maxPieceShift, maxPieceShift), DY: clamp(dy, -500, 500), Hidden: p["hidden"] == true, Acts: []pieceAct{}}
		acts, _ := p["acts"].([]any)
		for _, a := range acts {
			am, ok := a.(map[string]any)
			if !ok || len(cp.Acts) >= maxPieceActs {
				continue
			}
			fx, _ := am["fx"].(string)
			sound, _ := am["sound"].(string)
			if !fxRe.MatchString(fx) {
				fx = ""
			}
			if !soundRe.MatchString(sound) {
				sound = ""
			}
			cp.Acts = append(cp.Acts, pieceAct{Label: str(am["label"], maxActLabel), FX: fx, Sound: sound, Say: str(am["say"], maxActSay)})
		}
		out.Pieces[id] = cp
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return nil, errInvalid("the arrangement is not readable")
	}
	return json.RawMessage(bytes.TrimSpace(buf.Bytes())), nil
}
