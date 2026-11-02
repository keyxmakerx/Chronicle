package crafting

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// RulesChange is what an AI's list of rules does to a recipe's: the list to
// store, and how many rules it keeps as they were, adds as drafts and drops.
type RulesChange struct {
	Rules           []Rule
	Kept, New, Gone int
}

// MergeAIRules reads the whole list of "When… then…" rules an AI wrote for
// a recipe whose stored rules are stored. A rule written exactly as a stored
// one keeps it, approval and look included, so sending the list back with
// one rule changed leaves the others alone. Every other rule arrives as a
// draft, which counts for nothing until a Director approves it in the rules
// editor. Unlike the editor's forgiving CleanRules, a rule Chronicle can't
// read, or one with a value the editor would clamp, drop or replace, is
// refused with its place in the list, so an AI never has a rule dropped or
// changed without a word.
func MergeAIRules(stored []Rule, raw json.RawMessage) (RulesChange, error) {
	var list []json.RawMessage
	if s := strings.TrimSpace(string(raw)); s == "" || s == "null" {
		list = nil
	} else if json.Unmarshal(raw, &list) != nil {
		return RulesChange{}, errInvalid("rules are a list, one rule each")
	}
	if len(list) > maxRules {
		return RulesChange{}, errInvalid(fmt.Sprintf("a recipe holds at most %d rules", maxRules))
	}
	used := make([]bool, len(stored))
	out := RulesChange{Rules: make([]Rule, 0, len(list))}
	for i, item := range list {
		var m map[string]any
		if json.Unmarshal(item, &m) != nil || m == nil {
			return RulesChange{}, errInvalid(fmt.Sprintf("rule %d is not a rule: write `when` and `then`", i+1))
		}
		r, ok := cleanRule(m)
		if !ok {
			return RulesChange{}, errInvalid(fmt.Sprintf("rule %d: %s", i+1, unreadableRule(m)))
		}
		if p := ruleProblem(m); p != "" {
			return RulesChange{}, errInvalid(fmt.Sprintf("rule %d: %s", i+1, p))
		}
		if j := sameRule(stored, used, r); j >= 0 {
			used[j] = true
			out.Rules = append(out.Rules, stored[j])
			out.Kept++
			continue
		}
		r.Draft = true
		out.Rules = append(out.Rules, r)
		out.New++
	}
	for _, u := range used {
		if !u {
			out.Gone++
		}
	}
	return out, nil
}

// sameRule finds a stored rule, not yet matched, that says what r says. The
// draft mark and the look are the Director's, so they don't count.
func sameRule(stored []Rule, used []bool, r Rule) int {
	key := ruleKey(r)
	for j := range stored {
		if !used[j] && ruleKey(stored[j]) == key {
			return j
		}
	}
	return -1
}

func ruleKey(r Rule) string {
	r.Draft, r.Look = false, nil
	b, _ := json.Marshal(r)
	return string(b)
}

// unreadableRule says why cleanRule left a rule out, naming the words it
// takes.
func unreadableRule(m map[string]any) string {
	if t, _ := m["then"].(map[string]any); t == nil || !effectKinds[fmt.Sprint(t["kind"])] {
		return "`then` needs a `kind` of " + kindList(effectKinds)
	}
	return "`when` needs at least one condition with a `kind` of " + kindList(condKinds)
}

func kindList(set map[string]bool) string {
	var ks []string
	for _, k := range []string{"moon", "event", "rolled", "weather", "season", "time", "bench", "ingredient", "maker", "always",
		"only", "cannot", "edge", "bane", "points", "extra", "fine", "becomes", "mark", "quirk", "mishap", "discover", "say"} {
		if set[k] {
			ks = append(ks, k)
		}
	}
	return strings.Join(ks, ", ")
}

// AIRuleData is a stored rule in the form the AI reads and writes, the
// rules editor's ruleData.
func AIRuleData(r Rule) json.RawMessage {
	b, _ := json.Marshal(r)
	return b
}

// ruleProblem is the first value in a readable rule that cleanRule would
// clamp, drop or replace with a default: a number out of range, a chance,
// phase, time or audience Chronicle doesn't know, a condition of an unknown
// kind, too many conditions, or text past its limit. "" when there is none.
// The editor can be forgiving because the Director sees the result; an AI's
// rule is checked before anyone looks at it, so it is refused instead.
func ruleProblem(m map[string]any) string {
	conds, ok := m["when"].([]any)
	if !ok {
		conds = []any{m["when"]}
	}
	if len(conds) > maxWhen {
		return fmt.Sprintf("`when` has %d conditions; a rule takes at most %d, and \"or\" is two rules", len(conds), maxWhen)
	}
	for j, c := range conds {
		if p := condProblem(c); p != "" {
			if len(conds) == 1 {
				return p
			}
			return fmt.Sprintf("condition %d: %s", j+1, p)
		}
	}
	t, _ := m["then"].(map[string]any)
	if p := thenProblem(t); p != "" {
		return p
	}
	if c, present := m["chance"]; present && c != nil {
		s, _ := c.(string)
		switch {
		case !oneOfStr(chances, s):
			return fmt.Sprintf("`chance` %s isn't one Chronicle rolls: write 1 in 2, 1 in 3, 1 in 4, 1 in 6, 1 in 10 or 1 in 20, or leave it out for always", quoteAny(c))
		case s != "always" && t["kind"] == "only":
			return "an `only` rule has no `chance`: leave it out"
		}
	}
	if p, present := m["players"]; present && p != nil {
		switch s, _ := p.(string); s {
		case "book", "whenItHappens", "shown", "secret":
		default:
			return fmt.Sprintf("`players` %s isn't book, whenItHappens or secret", quoteAny(p))
		}
	}
	for _, k := range []string{"bookLine", "line"} {
		if p := tooLong(m, k, maxRuleLine); p != "" {
			return p
		}
	}
	return ""
}

func condProblem(v any) string {
	c, _ := v.(map[string]any)
	k, _ := c["kind"].(string)
	if !condKinds[k] {
		return fmt.Sprintf("`kind` %s isn't a condition: write %s", quoteAny(c["kind"]), kindList(condKinds))
	}
	switch k {
	case "moon":
		if ph, present := c["phase"]; present && ph != nil && !moonPhases[lower(ph)] {
			return fmt.Sprintf("`phase` %s isn't a moon phase Chronicle knows", quoteAny(ph))
		}
	case "time":
		if !timesOfDay[lower(c["time"])] {
			return fmt.Sprintf("`time` %s isn't dawn, day, dusk or night", quoteAny(c["time"]))
		}
	}
	for _, f := range []string{"moon", "event", "result", "weather", "season", "bench", "mark", "skill"} {
		if p := tooLong(c, f, maxRuleText); p != "" {
			return p
		}
	}
	return ""
}

func thenProblem(t map[string]any) string {
	switch t["kind"] {
	case "points":
		return amountProblem(t, -10, 10, true)
	case "extra":
		return amountProblem(t, 1, 3, false)
	}
	for _, f := range []string{"item", "mark", "table", "recipe"} {
		if p := tooLong(t, f, maxRuleText); p != "" {
			return p
		}
	}
	return tooLong(t, "text", maxRuleLine)
}

// amountProblem checks an effect's `amount`, which may be left out for the
// editor's default.
func amountProblem(t map[string]any, lo, hi int, noZero bool) string {
	v, present := t["amount"]
	if !present || v == nil {
		return ""
	}
	var f float64
	switch x := v.(type) {
	case float64:
		f = x
	case string:
		p, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return fmt.Sprintf("`amount` %s isn't a number", quoteAny(v))
		}
		f = p
	default:
		return fmt.Sprintf("`amount` %s isn't a number", quoteAny(v))
	}
	kind := t["kind"]
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) || f < float64(lo) || f > float64(hi) || (noZero && f == 0) {
		rng := fmt.Sprintf("a whole number from %d to %d", lo, hi)
		if noZero {
			rng += ", not 0"
		}
		return fmt.Sprintf("a `%v` effect's `amount` is %s; %s is outside that", kind, rng, quoteAny(v))
	}
	return ""
}

func tooLong(m map[string]any, key string, max int) string {
	s, _ := m[key].(string)
	if n := len([]rune(strings.TrimSpace(s))); n > max {
		return fmt.Sprintf("`%s` is %d characters; it holds at most %d", key, n, max)
	}
	return ""
}

// quoteAny writes a value the AI sent as it would read it back.
func quoteAny(v any) string {
	if v == nil {
		return "(missing)"
	}
	if s, ok := v.(string); ok {
		return strconv.Quote(s)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
