package systems

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities/cardborders"
)

// A campaign entry's details are shaped like a package entry's, so every
// consumer (the hero creator, pick lists, the Foundry sync) reads both the
// same way: scalars, plain lists of short words, and lists of named items
// (name, optional whole-number cost, optional plain-text description).
//
// A package entry can also hold richer values (a kit's signature ability
// object, an ability object on a trait). Nobody can type one in. A rewrite
// copies them verbatim from the package, which is trusted data, and a later
// save may keep them only unchanged: the "carried" rule. Create never has
// anything stored, so it never accepts one, and a player's own entry was
// created that way, so it never has one to carry.

// ValidateEntryProperties checks details for a new entry (AI import's review
// step). Nothing is carried.
func ValidateEntryProperties(p map[string]any) error { return validateEntryProperties(p, nil) }

// ValidateEntryPropertiesOnto checks details that will replace stored ones,
// letting values the validator does not understand through only when they
// are unchanged from stored (AI import's update review).
func ValidateEntryPropertiesOnto(p, stored map[string]any) error {
	return validateEntryProperties(p, stored)
}

// validateEntryProperties accepts the shapes above. stored is what the entry
// already holds (nil for a new entry); see the carried rule.
func validateEntryProperties(p, stored map[string]any) error {
	if len(p) > maxEntryProps {
		return apperror.NewValidation(fmt.Sprintf("At most %d details are allowed.", maxEntryProps))
	}
	if err := validateCardLook(p); err != nil {
		return err
	}
	lists, wordLists := 0, 0
	for k, v := range p {
		if !entryPropKeyPattern.MatchString(k) || len(k) > maxEntryPropKey {
			return apperror.NewValidation("Detail names must start with a letter and use only letters, digits and underscores.")
		}
		switch t := v.(type) {
		case string:
			if utf8.RuneCountInString(t) > maxEntryPropText {
				return apperror.NewValidation(fmt.Sprintf("The detail %q is longer than %d characters.", k, maxEntryPropText))
			}
		case bool, float64, int, int64:
		case []any:
			// An unchanged package list rides along whole, even one a typed
			// list could not hold (an over-long word, an item with an object).
			if carried(v, stored, k) {
				continue
			}
			if isWordList(t) {
				if wordLists++; wordLists > maxEntryWordLists {
					return apperror.NewValidation(fmt.Sprintf("At most %d lists of words are allowed.", maxEntryWordLists))
				}
				if err := validateWordList(k, t); err != nil {
					return err
				}
				continue
			}
			if lists++; lists > maxEntryLists {
				return apperror.NewValidation(fmt.Sprintf("At most %d lists are allowed.", maxEntryLists))
			}
			var prev []any
			if stored != nil {
				prev, _ = stored[k].([]any)
			}
			if err := validateEntryList(k, t, prev); err != nil {
				return err
			}
		default:
			if carried(v, stored, k) {
				continue
			}
			return apperror.NewValidation(fmt.Sprintf("The detail %q must be text, a number, yes/no or a list of named items.", k))
		}
	}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > maxEntryPropsBytes {
		return apperror.NewValidation("The details are too large.")
	}
	return nil
}

// validateCardLook refuses a card border that is not in the library and a
// card colour that is not six hex digits, naming the valid keys, so the hero
// creator's stage look never meets a value it would silently draw plain.
func validateCardLook(p map[string]any) error {
	if v, ok := p[cardborders.PropBorder]; ok {
		key, isText := v.(string)
		if _, known := cardborders.Get(key); !isText || !known {
			return apperror.NewValidation(fmt.Sprintf("The detail %q must be one of: %s.", cardborders.PropBorder, strings.Join(cardborders.Keys(), ", ")))
		}
	}
	if v, ok := p[cardborders.PropColour]; ok {
		if col, isText := v.(string); !isText || !cardborders.ValidColour(col) {
			return apperror.NewValidation(fmt.Sprintf("The detail %q must be a colour written as # and six hex digits, for example #c4243a.", cardborders.PropColour))
		}
	}
	return nil
}

// CardBorderGuide is the plain-words note on the two card details, with every
// border key and its shown name, for the people and tools that write entries.
func CardBorderGuide() string {
	var parts []string
	for _, b := range cardborders.All() {
		parts = append(parts, b.Key+" ("+b.Name+")")
	}
	return "`" + cardborders.PropBorder + "` names a card border from this list: " + strings.Join(parts, ", ") +
		". `" + cardborders.PropColour + "` is the colour its coloured parts take, written as # and six hex digits (for example #c4243a); leave it out for the border's own colour. " +
		"Only the hero creator's stage look draws borders, and an entry that names none is a plain card. A key or colour outside these is refused."
}

// isWordList reports whether every item is a string (an empty list is not
// one: it says nothing and is refused as an empty list of items).
func isWordList(items []any) bool {
	if len(items) == 0 {
		return false
	}
	for _, it := range items {
		if _, ok := it.(string); !ok {
			return false
		}
	}
	return true
}

func validateWordList(key string, items []any) error {
	if len(items) > maxEntryListItems {
		return apperror.NewValidation(fmt.Sprintf("The list %q can hold at most %d words.", key, maxEntryListItems))
	}
	for _, it := range items {
		if utf8.RuneCountInString(it.(string)) > maxEntryPropString {
			return apperror.NewValidation(fmt.Sprintf("Each word in %q can be at most %d characters.", key, maxEntryPropString))
		}
	}
	return nil
}

// carried reports whether stored holds exactly this value under key.
func carried(v any, stored map[string]any, key string) bool {
	if stored == nil {
		return false
	}
	prev, ok := stored[key]
	return ok && sameJSON(v, prev)
}

// sameJSON compares two decoded values as JSON, so 5 and 5.0 agree and map
// order never matters.
func sameJSON(a, b any) bool {
	ra, errA := json.Marshal(a)
	rb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	var na, nb any
	if json.Unmarshal(ra, &na) != nil || json.Unmarshal(rb, &nb) != nil {
		return false
	}
	return reflect.DeepEqual(na, nb)
}

// storedItem finds the item of the same name in the stored list.
func storedItem(prev []any, name string) map[string]any {
	for _, raw := range prev {
		if it, ok := raw.(map[string]any); ok {
			if n, _ := it["name"].(string); strings.EqualFold(strings.TrimSpace(n), name) {
				return it
			}
		}
	}
	return nil
}

// validateEntryList checks one list of named items. An item key other than
// name, cost and description passes only when the stored item of the same
// name in the same list holds the same value.
func validateEntryList(key string, items, prev []any) error {
	if len(items) == 0 || len(items) > maxEntryListItems {
		return apperror.NewValidation(fmt.Sprintf("The list %q needs 1 to %d items.", key, maxEntryListItems))
	}
	seen := map[string]bool{}
	for _, raw := range items {
		it, ok := raw.(map[string]any)
		if !ok {
			return apperror.NewValidation(fmt.Sprintf("Each item in %q needs a name.", key))
		}
		name, _ := it["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" || utf8.RuneCountInString(name) > maxEntryItemName {
			return apperror.NewValidation(fmt.Sprintf("Each item in %q needs a name of at most %d characters.", key, maxEntryItemName))
		}
		if seen[strings.ToLower(name)] {
			return apperror.NewValidation(fmt.Sprintf("%q is in %q twice.", name, key))
		}
		seen[strings.ToLower(name)] = true
		var was map[string]any
		for ik, iv := range it {
			switch ik {
			case "name":
			case "cost":
				if c, ok := wholeNumber(iv); !ok || c < 0 || c > maxEntryItemCost {
					return apperror.NewValidation(fmt.Sprintf("%q needs a whole-number cost from 0 to %d.", name, maxEntryItemCost))
				}
			case "description":
				d, ok := iv.(string)
				if !ok || utf8.RuneCountInString(d) > maxEntryItemText {
					return apperror.NewValidation(fmt.Sprintf("The text of %q can be at most %d characters.", name, maxEntryItemText))
				}
			default:
				if was == nil {
					was = storedItem(prev, name)
				}
				if prevV, ok := was[ik]; !ok || !sameJSON(iv, prevV) {
					return apperror.NewValidation(fmt.Sprintf("Items in %q can only have a name, cost and description.", key))
				}
			}
		}
	}
	return nil
}

// dropDerivedTwins removes every "<key>_display" detail whose "<key>" is also
// present: the package generates the twin from its base, so a copy would go
// stale the moment the base is edited. A twin with no base is the value
// itself and stays.
func dropDerivedTwins(p map[string]any) {
	for k := range p {
		if base, ok := strings.CutSuffix(k, "_display"); ok && base != "" {
			if _, has := p[base]; has {
				delete(p, k)
			}
		}
	}
}
