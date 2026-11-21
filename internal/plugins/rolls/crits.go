package rolls

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// The crit a die carries, as the server judged it.
const (
	CritSucc = "succ"
	CritFail = "fail"
)

// CritSource gives the rules of the campaign's game system. It is implemented
// in internal/app over the enabled system's manifest, so this package never
// reads another plugin's data directly. A nil source means the default rules.
type CritSource interface {
	CritRules(ctx context.Context, campaignID string) []systems.CritRule
}

// critRules is the campaign's rules; an unset or failing source falls back to
// the default so a roll is never refused over a crit lookup.
func (s *service) critRules(ctx context.Context, campaignID string) []systems.CritRule {
	if s.crits == nil {
		return systems.DefaultCritRules()
	}
	return s.crits.CritRules(ctx, campaignID)
}

// termGroups is where each dice term of a formula starts and ends in the dice
// throw() rolled, so a rule that reads a pair as one result sees the pair the
// roller asked for and not two unrelated d10s from different terms.
func termGroups(terms []term) [][2]int {
	var groups [][2]int
	at := 0
	for _, t := range terms {
		if t.sides == 0 {
			continue
		}
		groups = append(groups, [2]int{at, at + t.count})
		at += t.count
	}
	return groups
}

// runGroups groups consecutive dice of one size, for rolls that arrive without
// their formula's terms (Foundry's, crafting's, the shop's). A run is the best
// guess at "one term", so a total rule only fires on a run of exactly its count.
func runGroups(dice []Die) [][2]int {
	var groups [][2]int
	for i := 0; i < len(dice); {
		j := i + 1
		for j < len(dice) && dice[j].Sides == dice[i].Sides {
			j++
		}
		groups = append(groups, [2]int{i, j})
		i = j
	}
	return groups
}

// markCrits sets Crit on every die from the rules. It overwrites whatever the
// dice carried, because a crit that came from a client is a claim, not a fact.
// The first rule that applies to a die decides it, even when its faces don't
// include the value, so an earlier rule can switch off a later, broader one.
func markCrits(rules []systems.CritRule, dice []Die, groups [][2]int) {
	for i := range dice {
		dice[i].Crit = ""
	}
	for _, g := range groups {
		if g[0] < 0 || g[1] > len(dice) || g[0] >= g[1] {
			continue
		}
		group := dice[g[0]:g[1]]
		for i := range group {
			for _, r := range rules {
				verdict, applies := judge(r, group, i)
				if applies {
					group[i].Crit = verdict
					break
				}
			}
		}
	}
}

// judge reports whether rule r covers die i of group and, if so, its crit.
func judge(r systems.CritRule, group []Die, i int) (verdict string, applies bool) {
	count, sides, _, ok := systems.ParseCritDice(r.Dice)
	if !ok || group[i].Sides != sides {
		return "", false
	}
	value := group[i].Value
	if r.Judge == systems.JudgeTotal {
		if len(group) != count {
			return "", false
		}
		value = 0
		for _, d := range group {
			if d.Sides != sides {
				return "", false
			}
			value += d.Value
		}
	}
	switch {
	case containsInt(r.Succ, value):
		return CritSucc, true
	case containsInt(r.Fail, value):
		return CritFail, true
	}
	return "", true
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
