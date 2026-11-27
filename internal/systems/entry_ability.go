package systems

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// An ability detail is an object of named parts, shaped like a package's own
// ability objects (a kit's signature ability), so a homemade one reads the
// same everywhere: the hero creator draws it, the Foundry sync turns it into
// a rollable ability, and it can be copied to another campaign as it is,
// because it holds words only, never an id.
//
// Every part is plain text except keywords, a list of short words, and cost,
// which may also be a whole number. A part outside the list is refused, so a
// typo never saves as a part nothing reads.

// abilityParts are the parts an ability detail may hold, in the order they
// read (the same order rules_table.go's partOrder reads a package's).
var abilityParts = []string{"name", "flavor", "type", "action_type", "cost", "keywords",
	"distance", "target", "trigger", "power_roll", "tier1", "tier2", "tier3", "effect", "special"}

// abilityLongParts are the parts written as sentences; the rest are a line.
var abilityLongParts = map[string]bool{"flavor": true, "trigger": true, "tier1": true, "tier2": true,
	"tier3": true, "effect": true, "special": true}

const (
	// maxEntryAbilities bounds the ability details on one entry.
	maxEntryAbilities = 4
	// maxAbilityKeywords bounds an ability's keywords.
	maxAbilityKeywords = 12
)

func isAbilityPart(k string) bool {
	for _, p := range abilityParts {
		if p == k {
			return true
		}
	}
	return false
}

// validateAbility checks one ability detail under key.
func validateAbility(key string, a map[string]any) error {
	name, _ := a["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxEntryItemName {
		return apperror.NewValidation(fmt.Sprintf("The ability in %q needs a name of at most %d characters.", key, maxEntryItemName))
	}
	for k, v := range a {
		if !isAbilityPart(k) {
			return apperror.NewValidation(fmt.Sprintf("The ability %q can only have these parts: %s.", name, strings.Join(abilityParts, ", ")))
		}
		switch k {
		case "name":
		case "keywords":
			list, ok := v.([]any)
			if !ok || len(list) > maxAbilityKeywords {
				return apperror.NewValidation(fmt.Sprintf("The keywords of %q must be a list of at most %d words.", name, maxAbilityKeywords))
			}
			for _, w := range list {
				s, isText := w.(string)
				if !isText || strings.TrimSpace(s) == "" || utf8.RuneCountInString(s) > maxEntryPropString {
					return apperror.NewValidation(fmt.Sprintf("Each keyword of %q must be a word of at most %d characters.", name, maxEntryPropString))
				}
			}
		case "cost":
			if c, ok := wholeNumber(v); ok {
				if c < 0 || c > 99 {
					return apperror.NewValidation(fmt.Sprintf("The cost of %q must be from 0 to 99.", name))
				}
				continue
			}
			if err := abilityText(name, k, v); err != nil {
				return err
			}
		default:
			if err := abilityText(name, k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func abilityText(name, part string, v any) error {
	s, ok := v.(string)
	limit := maxEntryPropString
	if abilityLongParts[part] {
		limit = maxEntryItemText
	}
	if !ok || utf8.RuneCountInString(s) > limit {
		return apperror.NewValidation(fmt.Sprintf("The %s of %q must be text of at most %d characters.", strings.ReplaceAll(part, "_", " "), name, limit))
	}
	return nil
}

// AbilityGuide is the plain-words note on ability details, for the people and
// tools that write entries.
func AbilityGuide() string {
	return "A detail may also be an ability, written as an object of parts, the way the game system's own entries hold one (a Draw Steel kit's `signature_ability`). " +
		"Its parts are " + strings.Join(abilityParts, ", ") + "; `name` is required, `keywords` is a list of words, `cost` is text or a whole number, and every other part is plain text as the book prints it " +
		"(for example `power_roll: Might or Agility`, `tier1: 3 + M damage`). Leave out the parts it does not have. Any other part is refused. " +
		"An ability holds only words, never an id, so it can be copied to another campaign as it is."
}
