package systems

import (
	"fmt"
	"regexp"
	"strconv"
)

// CritRule says which rolled dice a game system counts as a critical success
// or failure. Chronicle's server applies it to every roll it records, so both
// dice looks and the log mark the same crits; a browser never decides one.
type CritRule struct {
	// Dice names the dice the rule covers: "d20" (every d20) or, with a count,
	// "2d10" (a group of exactly that many).
	Dice string `json:"dice"`
	// Judge is "each" (default: every die by its own face) or "total" (a
	// group of Dice's count is judged by its sum, because the system reads
	// the pair or pool as one result, like Draw Steel's 2d10 power roll).
	Judge string `json:"judge,omitempty"`
	// Succ and Fail are the faces (or, for "total", the sums) that count.
	Succ []int `json:"succ,omitempty"`
	Fail []int `json:"fail,omitempty"`
}

// Judge values of a CritRule.
const (
	JudgeEach  = "each"
	JudgeTotal = "total"
)

// DefaultCritRules is what a campaign gets when its system declares none or
// it has no system: a natural 20 on a d20 is a success, a natural 1 a failure.
func DefaultCritRules() []CritRule {
	return []CritRule{{Dice: "d20", Judge: JudgeEach, Succ: []int{20}, Fail: []int{1}}}
}

// CritRules is the system's crit rules. A block that is absent means the
// default; one that is present but empty (`"crits": []`) means the system has
// no crits at all, which is why the field is a pointer: a nil slice and an
// empty one must stay different.
func (m *SystemManifest) CritRules() []CritRule {
	if m == nil || m.Crits == nil {
		return DefaultCritRules()
	}
	out := make([]CritRule, 0, len(*m.Crits))
	for _, r := range *m.Crits {
		r.Succ = append([]int(nil), r.Succ...)
		r.Fail = append([]int(nil), r.Fail...)
		out = append(out, r)
	}
	return out
}

// Limits on a crits block, so a manifest cannot make the judge do much work.
const (
	maxCritRules  = 8
	maxCritValues = 20
)

// critDiceRe is NdS or dS; the count is optional here, unlike ParseDice,
// because an "each" rule covers every die of a size.
var critDiceRe = regexp.MustCompile(`^([1-4])?d(4|6|8|10|12|20)$`)

// ParseCritDice reads a rule's Dice. explicit says the count was written, which
// a "total" rule needs so it knows how big a group to sum.
func ParseCritDice(s string) (count, sides int, explicit, ok bool) {
	m := critDiceRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, false, false
	}
	count = 1
	if m[1] != "" {
		count, _ = strconv.Atoi(m[1])
		explicit = true
	}
	sides, _ = strconv.Atoi(m[2])
	return count, sides, explicit, true
}

// validateCrits rejects a crits block the judge could not apply.
func validateCrits(rules *[]CritRule) error {
	if rules == nil {
		return nil
	}
	if len(*rules) > maxCritRules {
		return fmt.Errorf("too many rules (%d, max %d)", len(*rules), maxCritRules)
	}
	for i, r := range *rules {
		count, sides, explicit, ok := ParseCritDice(r.Dice)
		if !ok {
			return fmt.Errorf("rule %d: dice %q must be d4, d6, d8, d10, d12 or d20, optionally with a count of one to four, like 2d10", i, r.Dice)
		}
		lo, hi := 1, sides
		switch r.Judge {
		case "", JudgeEach:
		case JudgeTotal:
			if !explicit {
				return fmt.Errorf("rule %d: a total is judged over a group, so dice needs a count, like 2d10", i)
			}
			lo, hi = count, count*sides
		default:
			return fmt.Errorf("rule %d: judge %q must be \"each\" or \"total\"", i, r.Judge)
		}
		if len(r.Succ) == 0 && len(r.Fail) == 0 {
			return fmt.Errorf("rule %d: list at least one succ or fail value", i)
		}
		for name, vals := range map[string][]int{"succ": r.Succ, "fail": r.Fail} {
			if len(vals) > maxCritValues {
				return fmt.Errorf("rule %d: too many %s values (%d, max %d)", i, name, len(vals), maxCritValues)
			}
			for _, v := range vals {
				if v < lo || v > hi {
					return fmt.Errorf("rule %d: %s value %d can't come up on %s (%d to %d)", i, name, v, r.Dice, lo, hi)
				}
			}
		}
	}
	return nil
}
