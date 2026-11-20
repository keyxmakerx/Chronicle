package systems

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// The words Chronicle uses to describe a change to an entry. Two readers need
// them: the Director, told in one line when the package changes a standard
// entry the campaign rewrote, and the save that asks whether to tell the
// players because a trait a hero holds was removed or altered. Both compare
// whole entries; nothing follows the package part by part.

// maxChangeParts keeps the line short: past it, the rest is counted.
const maxChangeParts = 5

// namedItem is one entry of a list of named items (a trait, a feature).
type namedItem struct {
	Name string
	Cost *int
	Text string
	// Extra holds any other keys a package item carries (an ability object,
	// a roll); a change there reads as the item reworded.
	Extra map[string]any
}

// samePlay reports whether two items read the same apart from cost.
func samePlay(a, b namedItem) bool {
	return strings.TrimSpace(a.Text) == strings.TrimSpace(b.Text) && sameJSON(a.Extra, b.Extra)
}

// namedLists reads every list of named items in properties, keyed by the
// list's key, each item keyed by its lowercased name.
func namedLists(p map[string]any) map[string]map[string]namedItem {
	out := map[string]map[string]namedItem{}
	for k, v := range p {
		list, ok := v.([]any)
		if !ok {
			continue
		}
		items := map[string]namedItem{}
		for _, raw := range list {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name, _ := m["name"].(string)
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			it := namedItem{Name: name}
			it.Text, _ = m["description"].(string)
			if c, ok := wholeNumber(m["cost"]); ok {
				it.Cost = &c
			}
			for k, v := range m {
				if k != "name" && k != "cost" && k != "description" {
					if it.Extra == nil {
						it.Extra = map[string]any{}
					}
					it.Extra[k] = v
				}
			}
			items[strings.ToLower(name)] = it
		}
		if len(items) > 0 {
			out[k] = items
		}
	}
	return out
}

// TraitChange is one trait a save removed or altered.
type TraitChange struct {
	// List is the property the trait sits in ("purchased_traits").
	List string `json:"list"`
	Name string `json:"name"`
	// Removed is true when the trait is gone; otherwise it was altered.
	Removed     bool `json:"removed,omitempty"`
	CostChanged bool `json:"costChanged,omitempty"`
	TextChanged bool `json:"textChanged,omitempty"`
	// CostBefore and CostAfter are set when the cost changed (nil = none).
	CostBefore *int `json:"costBefore,omitempty"`
	CostAfter  *int `json:"costAfter,omitempty"`
}

// ChangedTraits lists the traits an update removed or altered, by name, so a
// save can ask whether to tell the players who hold them. A trait only added
// is not listed: nobody can hold it yet. Order is stable (list, then name).
func ChangedTraits(before, after *SystemEntry) []TraitChange {
	if before == nil || after == nil {
		return nil
	}
	was, now := namedLists(before.Properties), namedLists(after.Properties)
	out := []TraitChange{}
	for list, items := range was {
		for key, old := range items {
			cur, ok := now[list][key]
			if !ok {
				// A trait moved to another list is still held under its name.
				if moved, found := findItem(now, key); found {
					cur, ok = moved, true
				}
			}
			if !ok {
				out = append(out, TraitChange{List: list, Name: old.Name, Removed: true})
				continue
			}
			c := TraitChange{List: list, Name: old.Name}
			if !sameCost(old.Cost, cur.Cost) {
				c.CostChanged, c.CostBefore, c.CostAfter = true, old.Cost, cur.Cost
			}
			c.TextChanged = !samePlay(old, cur)
			if c.CostChanged || c.TextChanged {
				out = append(out, c)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].List != out[j].List {
			return out[i].List < out[j].List
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func findItem(lists map[string]map[string]namedItem, key string) (namedItem, bool) {
	for _, items := range lists {
		if it, ok := items[key]; ok {
			return it, true
		}
	}
	return namedItem{}, false
}

func sameCost(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// DescribeStandardChange says what changed between the snapshot of a standard
// entry and the entry today, in one short line such as "Grounded reworded;
// Iron Gut added; speed 5 → 6." Empty when nothing did.
func DescribeStandardChange(was, now StandardSnapshot) string {
	var parts []string
	if strings.TrimSpace(was.Name) != "" && strings.TrimSpace(was.Name) != strings.TrimSpace(now.Name) {
		parts = append(parts, "now called "+now.Name)
	}
	oldLists, newLists := namedLists(was.Properties), namedLists(now.Properties)
	var added, dropped, reworded, recosted []string
	all := func(lists map[string]map[string]namedItem) map[string]namedItem {
		out := map[string]namedItem{}
		for _, items := range lists {
			for k, v := range items {
				out[k] = v
			}
		}
		return out
	}
	oldItems, newItems := all(oldLists), all(newLists)
	for k, o := range oldItems {
		n, ok := newItems[k]
		switch {
		case !ok:
			dropped = append(dropped, o.Name)
		case !samePlay(o, n):
			reworded = append(reworded, o.Name)
		}
		if ok && !sameCost(o.Cost, n.Cost) {
			recosted = append(recosted, fmt.Sprintf("%s now costs %s", n.Name, costWord(n.Cost)))
		}
	}
	for k, n := range newItems {
		if _, ok := oldItems[k]; !ok {
			added = append(added, n.Name)
		}
	}
	for _, g := range []struct {
		names []string
		verb  string
	}{{reworded, "reworded"}, {added, "added"}, {dropped, "dropped"}} {
		if len(g.names) > 0 {
			sort.Strings(g.names)
			parts = append(parts, joinNames(g.names)+" "+g.verb)
		}
	}
	sort.Strings(recosted)
	parts = append(parts, recosted...)
	parts = append(parts, figureChanges(was.Properties, now.Properties)...)
	if strings.TrimSpace(was.Summary) != strings.TrimSpace(now.Summary) ||
		strings.TrimSpace(was.Description) != strings.TrimSpace(now.Description) {
		parts = append(parts, "text changed")
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) > maxChangeParts {
		rest := len(parts) - (maxChangeParts - 1)
		parts = append(parts[:maxChangeParts-1], fmt.Sprintf("%d more changes", rest))
	}
	line := strings.Join(parts, "; ") + "."
	return strings.ToUpper(line[:1]) + line[1:]
}

func costWord(c *int) string {
	if c == nil {
		return "nothing"
	}
	return fmt.Sprintf("%d", *c)
}

// figureChanges compares the scalar figures (size, speed...). Generated
// "_display" twins repeat a list already compared, so they are skipped.
func figureChanges(was, now map[string]any) []string {
	keys := map[string]bool{}
	for k := range was {
		keys[k] = true
	}
	for k := range now {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		if strings.HasSuffix(k, "_display") {
			continue
		}
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	var out []string
	for _, k := range sorted {
		o, okO := was[k]
		n, okN := now[k]
		if _, isList := o.([]any); isList {
			if _, named := namedLists(map[string]any{k: o})[k]; named {
				continue
			}
		}
		if _, isList := n.([]any); isList {
			if _, named := namedLists(map[string]any{k: n})[k]; named {
				continue
			}
		}
		label := strings.ReplaceAll(k, "_", " ")
		switch {
		case !okO:
			out = append(out, label+" added")
		case !okN:
			out = append(out, label+" dropped")
		case !reflect.DeepEqual(normalizeFigure(o), normalizeFigure(n)):
			ow, nw := figureWord(o), figureWord(n)
			if ow != "" && nw != "" {
				out = append(out, fmt.Sprintf("%s %s → %s", label, ow, nw))
			} else {
				out = append(out, label+" changed")
			}
		}
	}
	return out
}

// normalizeFigure makes 5 and 5.0 equal, as JSON and YAML read them apart.
func normalizeFigure(v any) any {
	if n, ok := wholeNumber(v); ok {
		return float64(n)
	}
	return v
}

// figureWord renders a short scalar; "" for anything long or nested.
func figureWord(v any) string {
	switch t := v.(type) {
	case string:
		if len([]rune(t)) <= 20 {
			return t
		}
	case bool:
		if t {
			return "yes"
		}
		return "no"
	case float64, int, int64:
		return fmt.Sprintf("%v", t)
	}
	return ""
}

func joinNames(names []string) string {
	switch len(names) {
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// KindPhrase is how a line names one of a kind: "a people" for ancestries,
// otherwise the field's own word ("a kit", "a culture").
func KindPhrase(fieldKey string) string {
	k := strings.ToLower(strings.ReplaceAll(fieldKey, "_", " "))
	switch k {
	case "ancestry", "race", "species":
		return "a people"
	case "":
		return "an entry"
	}
	if strings.ContainsRune("aeiou", rune(k[0])) {
		return "an " + k
	}
	return "a " + k
}

// NotInLine is the one line a left-out entry answers with everywhere:
// "Polder is not a people of Therin." plus the Director's sentence.
func NotInLine(name, fieldKey, campaignName, sentence string) string {
	line := fmt.Sprintf("%s is not %s of %s.", name, KindPhrase(fieldKey), campaignName)
	if s := strings.TrimSpace(sentence); s != "" {
		line += " " + s
	}
	return line
}
