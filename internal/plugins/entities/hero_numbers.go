package entities

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// HeroNumbers is how a new hero's starting numbers come from the creator's
// picks, as the game system declares it (systems.CreatorNumbers). Core keeps
// no game system's field or property names: they arrive here.
type HeroNumbers struct {
	Characteristics *HeroCharacteristics `json:"characteristics,omitempty"`
	Sums            []HeroSum            `json:"sums,omitempty"`
}

// HeroCharacteristics is the creator's characteristics step: the entry
// picked at Step fixes some Fields at FixedValue (its FixedProperty lists
// them) and offers arrays (its ArraysProperty) for the rest.
type HeroCharacteristics struct {
	Label          string      `json:"label"`
	Step           string      `json:"step"`
	Fields         []HeroField `json:"fields"`
	FixedProperty  string      `json:"fixedProperty,omitempty"`
	FixedValue     int         `json:"fixedValue"`
	ArraysProperty string      `json:"arraysProperty"`
}

// HeroField is one number field the creator writes.
type HeroField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// HeroSum sets each of Fields to the total of Parts, or half of it.
type HeroSum struct {
	Fields []string      `json:"fields"`
	Parts  []HeroSumPart `json:"parts"`
	Half   bool          `json:"half,omitempty"`
}

// HeroSumPart reads a number property of the entry picked at Step.
type HeroSumPart struct {
	Step     string `json:"step"`
	Property string `json:"property"`
}

// addHeroNumbers puts the starting numbers into a new hero's fields. A pick
// already writes the field it names, so a number never overwrites one.
func addHeroNumbers(plan *HeroPlan, req HeroRequest, fields map[string]any) error {
	if plan == nil {
		return addHeroNumbersFrom(nil, nil, req.Characteristics, fields)
	}
	resolved, err := resolveHeroPicks(plan, req.Picks, false)
	if err != nil {
		return err
	}
	return addHeroNumbersFrom(plan.Numbers, resolved, req.Characteristics, fields)
}

func addHeroNumbersFrom(n *HeroNumbers, resolved map[string]heroPickResult, placed map[string]int, fields map[string]any) error {
	nums, err := heroNumberFields(n, resolved, placed)
	if err != nil {
		return err
	}
	for k, v := range nums {
		if _, taken := fields[k]; !taken {
			fields[k] = v
		}
	}
	return nil
}

// heroNumberFields works out the numbers a new hero starts with from what
// the creator resolved, and checks the characteristics the player placed:
// every fixed one at its value and the rest exactly one of the entry's
// arrays. Placed values are refused when nothing was picked that offers
// arrays. An ask-first pick isn't the hero's yet, so it adds nothing.
func heroNumberFields(n *HeroNumbers, resolved map[string]heroPickResult, placed map[string]int) (map[string]any, error) {
	out := map[string]any{}
	if n == nil {
		if len(placed) > 0 {
			return nil, apperror.NewBadRequest("this game system's creator doesn't place characteristics")
		}
		return out, nil
	}
	entry := func(step string) *HeroChoice {
		r, ok := resolved[step]
		if !ok || r.choice == nil || r.choice.State == HeroChoiceStateAskFirst {
			return nil
		}
		return r.choice
	}
	for _, s := range n.Sums {
		total, found := 0, false
		for _, p := range s.Parts {
			if e := entry(p.Step); e != nil {
				if v, ok := propNumber(e.Properties[p.Property]); ok {
					total += v
					found = true
				}
			}
		}
		if !found {
			continue
		}
		if s.Half {
			total = int(math.Floor(float64(total) / 2))
		}
		for _, f := range s.Fields {
			out[f] = total
		}
	}
	if len(placed) == 0 {
		return out, nil
	}
	c := n.Characteristics
	var from *HeroChoice
	if c != nil {
		from = entry(c.Step)
	}
	if from == nil {
		return nil, apperror.NewBadRequest("choose the entry that gives the characteristics before placing them")
	}
	fixed, rest, arrays := heroArrays(c, from)
	if len(arrays) == 0 {
		return nil, apperror.NewBadRequest(fmt.Sprintf("%s offers no characteristic arrays", from.Name))
	}
	known := map[string]bool{}
	for _, f := range c.Fields {
		known[f.Key] = true
	}
	for k := range placed {
		if !known[k] {
			return nil, apperror.NewBadRequest(fmt.Sprintf("%q isn't a characteristic", k))
		}
	}
	for _, f := range fixed {
		if v, ok := placed[f.Key]; !ok || v != c.FixedValue {
			return nil, apperror.NewBadRequest(fmt.Sprintf("%s is %d for %s", f.Label, c.FixedValue, from.Name))
		}
	}
	got := make([]int, 0, len(rest))
	for _, f := range rest {
		v, ok := placed[f.Key]
		if !ok {
			return nil, apperror.NewBadRequest("place a number on every characteristic, or none")
		}
		got = append(got, v)
	}
	if !matchesAnArray(got, arrays) {
		return nil, apperror.NewBadRequest(fmt.Sprintf("those numbers aren't one of %s's arrays", from.Name))
	}
	for k, v := range placed {
		out[k] = v
	}
	return out, nil
}

// heroArrays splits the characteristic fields into the ones the entry fixes
// and the rest, and reads the entry's arrays, keeping only arrays as long as
// the rest. A fixed name matches a field by key or label, ignoring case.
func heroArrays(c *HeroCharacteristics, e *HeroChoice) (fixed, rest []HeroField, arrays [][]int) {
	names := map[string]bool{}
	if c.FixedProperty != "" {
		for _, v := range propList(e.Properties[c.FixedProperty]) {
			names[strings.ToLower(strings.TrimSpace(v))] = true
		}
	}
	for _, f := range c.Fields {
		if names[strings.ToLower(f.Key)] || names[strings.ToLower(f.Label)] {
			fixed = append(fixed, f)
		} else {
			rest = append(rest, f)
		}
	}
	for _, a := range propList(e.Properties[c.ArraysProperty]) {
		if nums, ok := parseArray(a); ok && len(nums) == len(rest) {
			arrays = append(arrays, nums)
		}
	}
	return fixed, rest, arrays
}

// parseArray reads "2, 1, 0, −1": numbers split by commas, with a minus sign,
// an en dash or a hyphen for negatives and an optional plus.
func parseArray(s string) ([]int, bool) {
	s = strings.NewReplacer("−", "-", "–", "-", "+", "").Replace(s)
	var out []int
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, false
		}
		out = append(out, v)
	}
	return out, len(out) > 0
}

// matchesAnArray reports whether got holds the same numbers as one of the
// arrays, in any order: the player chooses where each one goes.
func matchesAnArray(got []int, arrays [][]int) bool {
	g := append([]int(nil), got...)
	sort.Ints(g)
	for _, a := range arrays {
		b := append([]int(nil), a...)
		sort.Ints(b)
		same := len(b) == len(g)
		for i := 0; same && i < len(b); i++ {
			same = b[i] == g[i]
		}
		if same {
			return true
		}
	}
	return false
}

// propNumber reads a whole number from an entry property: a JSON number or
// numeric text.
func propNumber(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		if t == math.Trunc(t) {
			return int(t), true
		}
	case int:
		return t, true
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(strings.Replace(t, "−", "-", 1))); err == nil {
			return n, true
		}
	}
	return 0, false
}

// propList reads an entry property that holds a list of text, or one text.
func propList(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	case string:
		return []string{t}
	}
	return nil
}
