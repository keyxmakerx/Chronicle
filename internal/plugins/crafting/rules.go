package crafting

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// A recipe's "When… then…" rules and its moments' looks, cleaned the way the
// rules editor cleans them (craft_rules_logic.js cleanRule and ruleData), so
// what the server stores is exactly what the editor would keep. Like the
// editor it is forgiving: an unknown field is dropped, a number is clamped,
// a rule with no known condition or effect is left out. Names (moons,
// events, weather) are kept as written and never checked against the
// calendar: a name the calendar lacks keeps the rule off and the editor says
// so. test/js/fixtures/craft_rules_clean.json pins both sides to one answer.

// Rule limits, the editor's LIMITS.
const (
	maxRules     = 40
	maxWhen      = 3
	maxRuleText  = 120
	maxRuleLine  = 240
	maxLookWords = 48
)

var (
	condKinds   = setOf("moon", "event", "rolled", "weather", "season", "time", "bench", "ingredient", "maker", "always")
	effectKinds = setOf("only", "cannot", "edge", "bane", "points", "extra", "fine", "becomes", "mark", "quirk", "mishap", "discover", "say")
	chances     = []string{"always", "1 in 2", "1 in 3", "1 in 4", "1 in 6", "1 in 10", "1 in 20"}
	moonPhases  = setOf("new", "waxing crescent", "first quarter", "waxing gibbous", "full", "waning gibbous", "last quarter", "waning crescent",
		"half", "crescent", "dark", "vanished", "waxing", "waning")
	timesOfDay = setOf("dawn", "day", "dusk", "night")
)

// Cond is one condition of a rule. Only the fields its kind uses are set.
type Cond struct {
	Kind    string `json:"kind"`
	Moon    string `json:"moon,omitempty"`
	Phase   string `json:"phase,omitempty"`
	Event   string `json:"event,omitempty"`
	Result  string `json:"result,omitempty"`
	Weather string `json:"weather,omitempty"`
	Season  string `json:"season,omitempty"`
	Time    string `json:"time,omitempty"`
	Bench   string `json:"bench,omitempty"`
	Mark    string `json:"mark,omitempty"`
	Skill   string `json:"skill,omitempty"`
}

// Effect is what a rule does when it is on.
type Effect struct {
	Kind   string `json:"kind"`
	Amount int    `json:"amount,omitempty"`
	Item   string `json:"item,omitempty"`
	Mark   string `json:"mark,omitempty"`
	Table  string `json:"table,omitempty"`
	Recipe string `json:"recipe,omitempty"`
	Text   string `json:"text,omitempty"`
}

// LookSpot is what one spot shows at a moment: a look, a move, or both.
type LookSpot struct {
	Look string `json:"look,omitempty"`
	Move string `json:"move,omitempty"`
}

// Look is how a moment plays on the bench.
type Look struct {
	Spots map[string]LookSpot `json:"spots"`
	Words string              `json:"words,omitempty"`
}

// Rule is one rule in the form the AI reads and writes (ruleData): players
// is "book", "whenItHappens" or "secret", and a secret rule has no look.
type Rule struct {
	When     []Cond `json:"when"`
	Then     Effect `json:"then"`
	Chance   string `json:"chance,omitempty"`
	Players  string `json:"players"`
	BookLine string `json:"bookLine,omitempty"`
	Look     *Look  `json:"look,omitempty"`
	Draft    bool   `json:"draft,omitempty"`
}

// CleanRules reads a list of rules as the AI or the editor wrote them and
// returns the stored form; anything it cannot read is left out.
func CleanRules(raw json.RawMessage) []Rule {
	var list []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &list) != nil {
		return []Rule{}
	}
	out := make([]Rule, 0, len(list))
	for i, item := range list {
		if i >= maxRules {
			break
		}
		var m map[string]any
		if json.Unmarshal(item, &m) != nil {
			continue
		}
		if r, ok := cleanRule(m); ok {
			out = append(out, r)
		}
	}
	return out
}

func cleanRule(m map[string]any) (Rule, bool) {
	var conds []any
	if arr, ok := m["when"].([]any); ok {
		conds = arr
	} else {
		conds = []any{m["when"]}
	}
	var when []Cond
	for _, c := range conds {
		if cm, ok := c.(map[string]any); ok {
			if cc, ok := cleanCond(cm); ok {
				when = append(when, cc)
			}
		}
	}
	if len(when) > maxWhen {
		when = when[:maxWhen]
	}
	tm, _ := m["then"].(map[string]any)
	then, ok := cleanThen(tm)
	if len(when) == 0 || !ok {
		return Rule{}, false
	}
	for _, c := range when {
		if c.Kind == "always" {
			when = []Cond{{Kind: "always"}}
			break
		}
	}
	players := "book"
	switch p, _ := m["players"].(string); p {
	case "whenItHappens", "shown":
		players = "whenItHappens"
	case "secret":
		players = "secret"
	}
	r := Rule{When: when, Then: then, Players: players, Draft: m["draft"] == true}
	if then.Kind != "only" {
		if c, _ := m["chance"].(string); c != "always" && oneOfStr(chances, c) {
			r.Chance = c
		}
	}
	line := m["bookLine"]
	if line == nil {
		line = m["line"]
	}
	if players == "book" {
		r.BookLine = str(line, maxRuleLine)
	}
	if players != "secret" {
		if lm, ok := m["look"].(map[string]any); ok {
			r.Look = cleanLook(lm)
		}
	}
	return r, true
}

func cleanCond(c map[string]any) (Cond, bool) {
	k, _ := c["kind"].(string)
	if !condKinds[k] {
		return Cond{}, false
	}
	switch k {
	case "moon":
		ph := lower(c["phase"])
		if !moonPhases[ph] {
			ph = "full"
		}
		return Cond{Kind: k, Moon: str(c["moon"], maxRuleText), Phase: ph}, true
	case "event":
		return Cond{Kind: k, Event: str(c["event"], maxRuleText)}, true
	case "rolled":
		return Cond{Kind: k, Event: str(c["event"], maxRuleText), Result: str(c["result"], maxRuleText)}, true
	case "weather":
		return Cond{Kind: k, Weather: str(c["weather"], maxRuleText)}, true
	case "season":
		return Cond{Kind: k, Season: str(c["season"], maxRuleText)}, true
	case "time":
		t := lower(c["time"])
		if !timesOfDay[t] {
			t = "night"
		}
		return Cond{Kind: k, Time: t}, true
	case "bench":
		return Cond{Kind: k, Bench: str(c["bench"], maxRuleText)}, true
	case "ingredient":
		return Cond{Kind: k, Mark: str(c["mark"], maxRuleText)}, true
	case "maker":
		return Cond{Kind: k, Skill: str(c["skill"], maxRuleText)}, true
	}
	return Cond{Kind: "always"}, true
}

func cleanThen(t map[string]any) (Effect, bool) {
	k, _ := t["kind"].(string)
	if t == nil || !effectKinds[k] {
		return Effect{}, false
	}
	e := Effect{Kind: k}
	n, finite := number(t["amount"])
	switch k {
	case "points":
		e.Amount = 2
		if finite && n != 0 {
			e.Amount = clamp(n, -10, 10)
		}
	case "extra":
		e.Amount = 1
		if finite {
			e.Amount = clamp(n, 1, 3)
		}
	case "becomes":
		e.Item = str(t["item"], maxRuleText)
	case "mark":
		e.Mark = str(t["mark"], maxRuleText)
	case "quirk", "mishap":
		e.Table = str(t["table"], maxRuleText)
	case "discover":
		e.Recipe = str(t["recipe"], maxRuleText)
	case "say":
		e.Text = str(t["text"], maxRuleLine)
	}
	return e, true
}

// cleanLook keeps the moment spots and looks the shared effects engine knows,
// the two moves, and short words.
func cleanLook(l map[string]any) *Look {
	out := &Look{Spots: map[string]LookSpot{}, Words: str(l["words"], maxLookWords)}
	sp, _ := l["spots"].(map[string]any)
	for _, k := range momentSpots {
		v, ok := sp[k].(map[string]any)
		if !ok {
			continue
		}
		var s LookSpot
		if look, _ := v["look"].(string); lookNames[look] {
			s.Look = look
		}
		if move, _ := v["move"].(string); lookMoves[move] {
			s.Move = move
		}
		if s.Look != "" || s.Move != "" {
			out.Spots[k] = s
		}
	}
	return out
}

// CleanLooks reads a recipe's moment looks, keeping the moments the bench
// plays.
func CleanLooks(raw json.RawMessage) map[string]*Look {
	out := map[string]*Look{}
	var m map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return out
	}
	for _, k := range Moments {
		if lm, ok := m[k].(map[string]any); ok {
			out[k] = cleanLook(lm)
		}
	}
	return out
}

// str is the editor's str(): a string trimmed, then cut to max characters,
// or "" for anything that is not a string.
func str(v any, max int) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return s
}

// lower is the editor's lower(): a string lowercased as it stands, untrimmed.
func lower(v any) string {
	s, _ := v.(string)
	return strings.ToLower(s)
}

// number reads a JSON number or numeric string, rounded, as the editor's
// Math.round(+v) does; finite is false when it is not a number.
func number(v any) (int, bool) {
	var f float64
	switch x := v.(type) {
	case float64:
		f = x
	case string:
		p, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0, false
		}
		f = p
	case bool:
		if x {
			f = 1
		}
	case nil:
		return 0, false
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return int(math.Floor(f + 0.5)), true
}

func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

func oneOfStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
