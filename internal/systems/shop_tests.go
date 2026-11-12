package systems

import (
	"fmt"
	"strings"
)

// Shop test styles: how a total becomes one of three outcomes.
const (
	// ShopTestTiers bands the total with two cut-offs, like a power roll.
	ShopTestTiers = "tiers"
	// ShopTestDC compares the total with a difficulty: meet it for the middle
	// outcome, beat it by 5 for the best.
	ShopTestDC = "dc"
)

// ShopTestStatDef is one sheet number a shop test may add. Score marks a D&D
// style ability score, where the test adds the modifier floor((value-10)/2)
// rather than the value itself.
type ShopTestStatDef struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Score bool   `json:"score,omitempty"`
}

// ShopTestDCDef is the three difficulties of the dc style. Zero means the
// default (10, 15, 20).
type ShopTestDCDef struct {
	Easy  int `json:"easy,omitempty"`
	Usual int `json:"usual,omitempty"`
	Hard  int `json:"hard,omitempty"`
}

// ShopTestsDef is the manifest's optional shop_tests block: the dice a player
// throws at a shop's counter to haggle or look closer, what the throw adds,
// and how the total reads. Every part is optional; ShopTestRules fills what is
// missing. Edge and Bane are pointers so a written 0 stays distinct from
// "not written".
type ShopTestsDef struct {
	Dice   string            `json:"dice,omitempty"`
	Style  string            `json:"style,omitempty"`
	Stats  []ShopTestStatDef `json:"stats,omitempty"`
	Haggle string            `json:"haggle,omitempty"`
	Look   string            `json:"look,omitempty"`
	Tiers  []int             `json:"tiers,omitempty"`
	Edge   *int              `json:"edge,omitempty"`
	Bane   *int              `json:"bane,omitempty"`
	DC     *ShopTestDCDef    `json:"dc,omitempty"`
}

// ShopTestRulesView is a shop test with every part decided, so callers never
// check for a missing piece.
type ShopTestRulesView struct {
	Dice   string            `json:"dice"`
	Style  string            `json:"style"`
	Stats  []ShopTestStatDef `json:"stats"`
	Haggle string            `json:"haggle"`
	Look   string            `json:"look"`
	Tiers  []int             `json:"tiers"`
	Edge   int               `json:"edge"`
	Bane   int               `json:"bane"`
	DC     ShopTestDCDef     `json:"dc"`
}

// Defaults for a shop test with nothing declared.
const (
	defaultShopTestDice = "1d20"
	defaultDCEasy       = 10
	defaultDCUsual      = 15
	defaultDCHard       = 20
)

// DefaultShopTestRules is the test of a campaign whose system declares neither
// shop_tests nor crafting: a d20 against a DC, adding nothing from the sheet,
// since no stat is known to be on it.
func DefaultShopTestRules() ShopTestRulesView {
	return ShopTestRulesView{
		Dice: defaultShopTestDice, Style: ShopTestDC, Stats: []ShopTestStatDef{},
		Tiers: []int{11, 16}, DC: ShopTestDCDef{Easy: defaultDCEasy, Usual: defaultDCUsual, Hard: defaultDCHard},
	}
}

// ShopTestRules decides how a shop test is thrown, in this order: the
// shop_tests block; else the crafting block's dice, characteristics, edge,
// bane and tiers (a power-roll style); else DefaultShopTestRules. Crafting is
// the fallback because a system that already says how its heroes roll should
// not need to say it twice.
func (m *SystemManifest) ShopTestRules() ShopTestRulesView {
	if m == nil {
		return DefaultShopTestRules()
	}
	if b := m.ShopTests; b != nil {
		return resolveShopTests(b)
	}
	if m.Crafting != nil {
		c := m.CraftingRules()
		v := ShopTestRulesView{
			Dice: c.Dice, Style: ShopTestTiers, Tiers: append([]int(nil), c.Tiers...),
			Edge: c.Edge, Bane: c.Bane, Stats: make([]ShopTestStatDef, 0, len(c.Characteristics)),
			DC: DefaultShopTestRules().DC,
		}
		for _, s := range c.Characteristics {
			v.Stats = append(v.Stats, ShopTestStatDef{Key: s.Key, Label: s.Label})
		}
		v.Haggle = pickShopStat(v.Stats, "presence")
		v.Look = pickShopStat(v.Stats, "reason")
		return v
	}
	return DefaultShopTestRules()
}

// resolveShopTests fills a declared block's gaps.
func resolveShopTests(b *ShopTestsDef) ShopTestRulesView {
	def := DefaultShopTestRules()
	v := ShopTestRulesView{
		Dice: b.Dice, Style: b.Style, Stats: append([]ShopTestStatDef{}, b.Stats...),
		Haggle: b.Haggle, Look: b.Look, Tiers: append([]int(nil), b.Tiers...), Edge: 2, Bane: -2, DC: def.DC,
	}
	if v.Dice == "" {
		v.Dice = def.Dice
	}
	if v.Style == "" {
		v.Style = def.Style
	}
	if len(v.Tiers) == 0 {
		v.Tiers = def.Tiers
	}
	if b.Edge != nil {
		v.Edge = *b.Edge
	}
	if b.Bane != nil {
		v.Bane = *b.Bane
	}
	if b.DC != nil {
		if b.DC.Easy != 0 {
			v.DC.Easy = b.DC.Easy
		}
		if b.DC.Usual != 0 {
			v.DC.Usual = b.DC.Usual
		}
		if b.DC.Hard != 0 {
			v.DC.Hard = b.DC.Hard
		}
	}
	if v.Haggle == "" {
		v.Haggle = pickShopStat(v.Stats, "presence")
	}
	if v.Look == "" {
		v.Look = pickShopStat(v.Stats, "reason")
	}
	return v
}

// pickShopStat is the preferred key when the system lists it, else the first
// listed stat, else "" (nothing to add).
func pickShopStat(stats []ShopTestStatDef, preferred string) string {
	for _, s := range stats {
		if s.Key == preferred {
			return s.Key
		}
	}
	if len(stats) > 0 {
		return stats[0].Key
	}
	return ""
}

// Limits on a shop_tests block, in step with the crafting block's.
const (
	maxShopTestStats = 12
	maxShopTestDC    = 40
)

// validateShopTests rejects a block the host could not throw with.
func validateShopTests(b *ShopTestsDef) error {
	if b == nil {
		return nil
	}
	if b.Dice != "" {
		if _, _, ok := ParseDice(b.Dice); !ok {
			return fmt.Errorf("dice %q must be one to four d4, d6, d8, d10, d12 or d20, written like 1d20", b.Dice)
		}
	}
	if b.Style != "" && b.Style != ShopTestTiers && b.Style != ShopTestDC {
		return fmt.Errorf("style %q must be %q or %q", b.Style, ShopTestTiers, ShopTestDC)
	}
	if len(b.Stats) > maxShopTestStats {
		return fmt.Errorf("too many stats (%d, max %d)", len(b.Stats), maxShopTestStats)
	}
	keys := map[string]bool{}
	for i, s := range b.Stats {
		if !slugPattern.MatchString(s.Key) {
			return fmt.Errorf("stat %d: key %q must contain only lowercase letters, numbers, hyphens, and underscores", i, s.Key)
		}
		if keys[s.Key] {
			return fmt.Errorf("stat %d: key %q is listed twice", i, s.Key)
		}
		keys[s.Key] = true
		if err := checkCraftingLabel(s.Label); err != nil || strings.TrimSpace(s.Label) == "" {
			return fmt.Errorf("stat %q: label must be up to %d plain words", s.Key, maxCraftingLabel)
		}
	}
	for name, k := range map[string]string{"haggle": b.Haggle, "look": b.Look} {
		if k != "" && !keys[k] {
			return fmt.Errorf("%s names %q, which is not one of the listed stats", name, k)
		}
	}
	if len(b.Tiers) != 0 {
		if len(b.Tiers) != 2 || b.Tiers[0] >= b.Tiers[1] || b.Tiers[0] < 1 {
			return fmt.Errorf("tiers must be two rising totals, like [11, 16]")
		}
	}
	for name, p := range map[string]*int{"edge": b.Edge, "bane": b.Bane} {
		if p != nil && (*p < -maxCraftingBonus || *p > maxCraftingBonus) {
			return fmt.Errorf("%s must be between -%d and %d", name, maxCraftingBonus, maxCraftingBonus)
		}
	}
	if d := b.DC; d != nil {
		for name, n := range map[string]int{"easy": d.Easy, "usual": d.Usual, "hard": d.Hard} {
			if n < 0 || n > maxShopTestDC {
				return fmt.Errorf("dc %s must be between 1 and %d", name, maxShopTestDC)
			}
		}
		// Compare resolved values so a half-written block cannot invert them.
		r := resolveShopTests(b).DC
		if r.Easy > r.Usual || r.Usual > r.Hard {
			return fmt.Errorf("dc must rise from easy to usual to hard")
		}
	}
	return nil
}
