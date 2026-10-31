package crafting

import (
	"fmt"
	"math"
	"strings"
)

// A recipe's rules read at the moment a project roll is made, the way the
// rules editor previews them (craft_rules_logic.js condOn, ruleOn and
// verdict): drafts count for nothing, a certain "can't be made" holds the
// roll, with any "only then" rule at least one must be on, and an edge and
// a bane cancel. The roll knows what the preview cannot: the bench it was
// worked at, the maker's skills, the time of day by the calendar's clock,
// and the chance, which is rolled here. Ingredient marks are still unknown,
// because an item in a pack keeps nothing of its own, so a rule needing one
// stays "maybe", as in the preview: it never holds a roll and never adds to
// one. TODO(#1883)

// tri is a condition's answer on the day.
type tri int

const (
	off tri = iota
	on
	maybe
)

// phaseName names a cycling moon's phase as the calendar does
// (Moon.MoonPhaseName): new runs from 15/16 of the cycle to 1/16.
func phaseName(p float64) string {
	names := []string{"new", "waxing crescent", "first quarter", "waxing gibbous", "full", "waning gibbous", "last quarter", "waning crescent"}
	if p < 0.0625 || p >= 0.9375 {
		return "new"
	}
	i := int(math.Floor((p-0.0625)/0.125)) + 1
	if i >= len(names) {
		return "new"
	}
	return names[i]
}

func (d *Day) moon(name string) *DayMoon {
	if d == nil {
		return nil
	}
	for i := range d.Moons {
		if d.Moons[i].Name == name {
			return &d.Moons[i]
		}
	}
	return nil
}

// Worked is what a roll knows of how it was worked: the bench's name ("" for
// none) and the maker's skills, lowercased (nil when the sheet lists none).
// A nil *Worked is a project listed before it is worked.
type Worked struct {
	Bench  string
	Skills map[string]bool
}

// condOn answers one condition on the day, as worked. With no calendar
// every calendar condition is off, as a name the calendar lacks is.
func condOn(d *Day, c Cond, w *Worked) tri {
	yes := func(b bool) tri {
		if b {
			return on
		}
		return off
	}
	switch c.Kind {
	case "always":
		return on
	case "moon":
		m := d.moon(c.Moon)
		if m == nil {
			return off
		}
		if m.Manual {
			return yes(m.State != "" && m.State == c.Phase)
		}
		switch c.Phase {
		case "waxing":
			return yes(m.Phase > 0.0625 && m.Phase < 0.4375)
		case "waning":
			return yes(m.Phase >= 0.5625 && m.Phase < 0.9375)
		}
		return yes(phaseName(m.Phase) == c.Phase)
	case "event":
		if d == nil {
			return off
		}
		for _, e := range d.Events {
			if e == c.Event {
				return on
			}
		}
		return off
	case "rolled":
		return yes(d != nil && d.Rolled[c.Event] == c.Result && c.Result != "")
	case "weather":
		return yes(d != nil && d.Weather != "" && d.Weather == c.Weather)
	case "season":
		return yes(d != nil && d.Season != "" && d.Season == c.Season)
	case "bench":
		if w == nil {
			return maybe
		}
		return yes(w.Bench != "" && w.Bench == c.Bench)
	case "time":
		if d == nil {
			return off
		}
		if d.Time == "" {
			return maybe
		}
		return yes(d.Time == c.Time)
	case "maker":
		if w == nil || w.Skills == nil {
			return maybe
		}
		return yes(w.Skills[strings.ToLower(strings.TrimSpace(c.Skill))])
	}
	return maybe
}

// ruleOn is off when any condition is off, maybe when one can't be known,
// and on otherwise.
func ruleOn(d *Day, r Rule, w *Worked) tri {
	out := on
	for _, c := range r.When {
		switch condOn(d, c, w) {
		case off:
			return off
		case maybe:
			out = maybe
		}
	}
	return out
}

// chanceIn reads "1 in N" as N; 1 for "always" (or anything else).
func chanceIn(s string) int {
	var n int
	if _, err := fmt.Sscanf(s, "1 in %d", &n); err != nil || n < 2 {
		return 1
	}
	return n
}

// ruleWords is how a roll's result names a rule: the book's line, or a
// plain sentence; a secret rule is never described to a player.
func ruleWords(r Rule, director bool) string {
	if r.Players == "secret" && !director {
		return "A rule the Director keeps hidden"
	}
	if r.BookLine != "" {
		return strings.TrimRight(r.BookLine, ".")
	}
	return strings.TrimRight(ruleSentence(r), ".")
}

// hold says why a recipe can't be worked on the day, "" when it can: a
// certain "can't be made", or "only then" rules none of which is on. Only
// rules that are certain hold a roll.
func hold(d *Day, rules []Rule, w *Worked, director bool) string {
	var only []Rule
	onlyOn := false
	for _, r := range rules {
		if r.Draft {
			continue
		}
		v := ruleOn(d, r, w)
		switch r.Then.Kind {
		case "only":
			only = append(only, r)
			onlyOn = onlyOn || v != off
		case "cannot":
			if v == on && chanceIn(r.Chance) == 1 {
				return ruleWords(r, director) + "."
			}
		}
	}
	if len(only) > 0 && !onlyOn {
		return ruleWords(only[0], director) + "."
	}
	return ""
}

// fired is a rule that took effect on a roll.
type fired struct {
	rule Rule
}

// effects rolls the chance of every rule that is certainly on and returns
// those that took effect, leaving out "only then" and certain "can't be
// made" rules, which hold decided before the roll. roll(n) is a die of n
// sides.
func effects(d *Day, rules []Rule, w *Worked, roll func(n int) int) []fired {
	var out []fired
	for _, r := range rules {
		if r.Draft || r.Then.Kind == "only" || ruleOn(d, r, w) != on {
			continue
		}
		n := chanceIn(r.Chance)
		if r.Then.Kind == "cannot" && n == 1 {
			continue
		}
		if n > 1 && roll(n) != 1 {
			continue
		}
		out = append(out, fired{rule: r})
	}
	return out
}
