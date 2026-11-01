package crafting

import (
	"encoding/json"
	"fmt"
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
// read is refused with its place in the list, so an AI never has a rule
// dropped without a word.
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
