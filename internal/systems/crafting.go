package systems

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// CraftingRules is how a game system rolls and counts a crafting project:
// the arithmetic only. Recipes, benches and steps are the campaign's own and
// work under any system's rules; a system with no block gets
// DefaultCraftingRules.
type CraftingRules struct {
	// Dice is the project roll, written NdS ("2d10").
	Dice string `json:"dice"`
	// Characteristics are the character sheet fields a recipe may add to
	// the roll, by field key, with the label shown in the editor.
	Characteristics []CraftingStat `json:"characteristics,omitempty"`
	// Edge and Bane are what clean and botched work add to the roll.
	Edge int `json:"edge"`
	Bane int `json:"bane"`
	// Tiers are the two highest totals of the first and second outcome of a
	// tiered test such as harvesting: a total at or below Tiers[0] is the
	// first tier, at or below Tiers[1] the second, anything higher the third.
	Tiers []int `json:"tiers,omitempty"`
	// Labels name the project's fields in the system's own words.
	Labels CraftingLabels `json:"labels"`
}

// CraftingStat is one characteristic a project roll can add.
type CraftingStat struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// CraftingLabels are a system's names for a project's goal, the item it
// needs before it can start, and where the knowledge comes from.
type CraftingLabels struct {
	Goal         string `json:"goal,omitempty"`
	Prerequisite string `json:"prerequisite,omitempty"`
	Source       string `json:"source,omitempty"`
}

// DefaultCraftingRules is the crafting a campaign gets when its system
// declares none (or it has no system): a 2d10 roll with no characteristic,
// an edge of +2 and a bane of -2, and the 11 / 16 tiers of a power roll.
func DefaultCraftingRules() CraftingRules {
	return CraftingRules{
		Dice: "2d10", Edge: 2, Bane: -2, Tiers: []int{11, 16},
		Labels: CraftingLabels{Goal: "Goal", Prerequisite: "Item prerequisite", Source: "Project source"},
	}
}

// CraftingRules is the system's crafting rules with every empty part filled from
// DefaultCraftingRules, so callers never check for nil.
func (m *SystemManifest) CraftingRules() CraftingRules {
	def := DefaultCraftingRules()
	if m == nil || m.Crafting == nil {
		return def
	}
	r := *m.Crafting
	r.Characteristics = append([]CraftingStat(nil), r.Characteristics...)
	if r.Dice == "" {
		r.Dice = def.Dice
	}
	if len(r.Tiers) == 0 {
		r.Tiers = def.Tiers
	}
	if r.Labels.Goal == "" {
		r.Labels.Goal = def.Labels.Goal
	}
	if r.Labels.Prerequisite == "" {
		r.Labels.Prerequisite = def.Labels.Prerequisite
	}
	if r.Labels.Source == "" {
		r.Labels.Source = def.Labels.Source
	}
	return r
}

// Limits on a crafting block, so its words fit the editor's labels.
const (
	maxCraftingStats = 12
	maxCraftingLabel = 40
	maxCraftingBonus = 10
)

// diceRe is a small dice pool: one to four dice of a common size.
var diceRe = regexp.MustCompile(`^([1-4])d(4|6|8|10|12|20)$`)

// craftingLabelRe is plain words: letters, digits, spaces and a little
// punctuation, never markup, since labels are dropped into sentences.
var craftingLabelRe = regexp.MustCompile(`^\pL[\pL\pN '’.-]*$`)

// ParseDice reads NdS into its count and size.
func ParseDice(s string) (count, size int, ok bool) {
	m := diceRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, false
	}
	count, _ = strconv.Atoi(m[1])
	size, _ = strconv.Atoi(m[2])
	return count, size, true
}

// validateCrafting rejects a crafting block the host could not roll with.
func validateCrafting(c *CraftingRules) error {
	if c == nil {
		return nil
	}
	if c.Dice != "" {
		if _, _, ok := ParseDice(c.Dice); !ok {
			return fmt.Errorf("dice %q must be one to four d4, d6, d8, d10, d12 or d20, written like 2d10", c.Dice)
		}
	}
	if c.Edge < -maxCraftingBonus || c.Edge > maxCraftingBonus || c.Bane < -maxCraftingBonus || c.Bane > maxCraftingBonus {
		return fmt.Errorf("edge and bane must be between -%d and %d", maxCraftingBonus, maxCraftingBonus)
	}
	if len(c.Characteristics) > maxCraftingStats {
		return fmt.Errorf("too many characteristics (%d, max %d)", len(c.Characteristics), maxCraftingStats)
	}
	seen := map[string]bool{}
	for i, s := range c.Characteristics {
		if !slugPattern.MatchString(s.Key) {
			return fmt.Errorf("characteristic %d: key %q must contain only lowercase letters, numbers, hyphens, and underscores", i, s.Key)
		}
		if seen[s.Key] {
			return fmt.Errorf("characteristic %d: key %q is listed twice", i, s.Key)
		}
		seen[s.Key] = true
		if err := checkCraftingLabel(s.Label); err != nil || s.Label == "" {
			return fmt.Errorf("characteristic %q: label must be up to %d plain words", s.Key, maxCraftingLabel)
		}
	}
	if len(c.Tiers) != 0 {
		if len(c.Tiers) != 2 || c.Tiers[0] >= c.Tiers[1] || c.Tiers[0] < 1 {
			return fmt.Errorf("tiers must be two rising totals, like [11, 16]")
		}
	}
	for _, l := range []string{c.Labels.Goal, c.Labels.Prerequisite, c.Labels.Source} {
		if err := checkCraftingLabel(l); err != nil {
			return fmt.Errorf("labels: %w", err)
		}
	}
	return nil
}

func checkCraftingLabel(s string) error {
	if s == "" {
		return nil
	}
	if strings.TrimSpace(s) != s || len([]rune(s)) > maxCraftingLabel || !craftingLabelRe.MatchString(s) {
		return fmt.Errorf("%q must be up to %d plain words", s, maxCraftingLabel)
	}
	return nil
}
