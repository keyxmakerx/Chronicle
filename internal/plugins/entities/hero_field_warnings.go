package entities

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// hero_field_warnings.go lets a reader outside the creator (AI Import's
// review) ask whether a hero's sheet values would have passed the creator.
// It asks resolveHeroPicks, the creator's own check, so what counts as on the
// list or under the class never has a second definition. The answer is words
// for a warning, never a refusal: home-made and Foundry-made values exist.

// maxHeroSuggestions bounds the closest names a warning offers.
const maxHeroSuggestions = 3

// HeroFieldWarnings names each written pick-list field of a hero whose value
// the plan would not accept, with the closest names the plan does list.
// fields is the hero's sheet as it will stand (a dependent step reads its
// parent from there, so a subclass is checked against a class the page
// already had); written is the keys the change sets, the only ones
// reported. A nil plan or a value that is not text gives no warning.
func HeroFieldWarnings(plan *HeroPlan, fields map[string]any, written []string) []string {
	if plan == nil {
		return nil
	}
	steps := make(map[string]*HeroStep, len(plan.Steps))
	for i := range plan.Steps {
		steps[plan.Steps[i].FieldKey] = &plan.Steps[i]
	}
	text := func(key string) string {
		s, _ := fields[key].(string)
		return strings.TrimSpace(s)
	}
	keys := append([]string(nil), written...)
	sort.Strings(keys)
	var out []string
	for _, key := range keys {
		step, name := steps[key], text(key)
		if step == nil || name == "" {
			continue
		}
		picks := map[string]HeroPick{key: {Name: name}}
		parent := ""
		if step.Parent != "" {
			// A parent that is itself off the list is reported on its own
			// row, not again on every step under it.
			if p := text(step.Parent); p != "" {
				if _, err := resolveHeroPicks(plan, map[string]HeroPick{step.Parent: {Name: p}}, true); err == nil {
					picks[step.Parent] = HeroPick{Name: p}
					parent = p
				}
			}
			if parent == "" {
				continue
			}
		}
		_, err := resolveHeroPicks(plan, picks, true)
		if err == nil {
			continue
		}
		var ae *apperror.AppError
		reason := err.Error()
		if errors.As(err, &ae) {
			reason = ae.Message
		}
		w := fmt.Sprintf("%s: %s.", step.Label, strings.TrimSuffix(reason, "."))
		if near := closestHeroNames(step, parent, name); len(near) > 0 {
			w += " Closest on the campaign's list: " + strings.Join(near, ", ") + "."
		}
		out = append(out, w+" It is only a warning, so a home-made or Foundry-made value can stay.")
	}
	return out
}

// closestHeroNames is the step's entry names nearest to name, under parent
// when the step lives inside another step's entries.
func closestHeroNames(step *HeroStep, parent, name string) []string {
	type scored struct {
		name string
		dist int
	}
	want := strings.ToLower(name)
	var all []scored
	for _, c := range step.Choices {
		if parent != "" && !strings.EqualFold(c.Parent, parent) {
			continue
		}
		have := strings.ToLower(c.Name)
		d := editDistance(want, have)
		if strings.Contains(have, want) || strings.Contains(want, have) {
			d = 0
		}
		all = append(all, scored{c.Name, d})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].dist < all[j].dist })
	var out []string
	for _, s := range all {
		if len(out) == maxHeroSuggestions {
			break
		}
		out = append(out, s.name)
	}
	return out
}

// editDistance is the Levenshtein distance between two strings, by rune.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
