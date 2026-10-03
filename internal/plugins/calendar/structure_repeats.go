// Package calendar - structure_repeats.go plans what a structure save does
// to repeat-by-rule events and their one-off overrides. Both name months by
// position (a rule's month/months conditions, an override's occurrence and
// new month), so they have to follow their month the way events do, or "every
// Fireday in Harvestide" would quietly point at another month after a reorder.
package calendar

import (
	"fmt"
	"strings"
)

// StructureRule is one rule event as a structure save reads it. Raw is the
// stored JSON text, kept so the fingerprint notices any change to it and so
// the plan can run it through the strict parser.
type StructureRule struct {
	EventID string
	Name    string
	Raw     string
}

// StructureOverride is one skip/move override as a structure save reads it:
// the month it is keyed by and, for a move, the month it moves to.
type StructureOverride struct {
	EventID  string
	Year     int
	Month    int
	Day      int
	NewMonth *int
}

// RuleUpdate is a rule's rewritten JSON for the repository to store.
type RuleUpdate struct {
	EventID string
	JSON    string
}

// repeatsPlan is what planRepeats found: the rewrites to store and the
// preview counts and notes that describe them.
type repeatsPlan struct {
	updates []RuleUpdate
	// notes are the per-rule lines (removed month, removed moon or season),
	// capped like the event lists; the counts below are of every one.
	notes             []string
	rulesUpdated      int
	rulesRemovedMonth int
	rulesRemovedRef   int
	overridesMoved    int
	overridesRemoved  int
	overridesReplaced int
}

// remapRuleMonths rewrites the month numbers of r's conditions through
// place. It returns the months it found with no counterpart (left exactly as
// written, never dropped or guessed) and whether anything changed. A months
// list is de-duplicated after the rewrite because the strict parser refuses
// a repeated entry the next time the rule is saved.
func remapRuleMonths(r *RecurrenceRule, monthCount int, place func(int) (int, bool)) (removed []int, changed bool) {
	for i := range r.Match {
		c := &r.Match[i]
		switch c.Kind {
		case RuleMonth:
			if c.Month < 1 || c.Month > monthCount {
				continue
			}
			nm, ok := place(c.Month)
			if !ok {
				removed = append(removed, c.Month)
			} else if nm != c.Month {
				c.Month, changed = nm, true
			}
		case RuleMonths:
			out := make([]int, 0, len(c.Months))
			seen := map[int]bool{}
			for _, m := range c.Months {
				v := m
				if m >= 1 && m <= monthCount {
					nm, ok := place(m)
					if !ok {
						removed = append(removed, m)
					} else if nm != m {
						v, changed = nm, true
					}
				}
				if seen[v] {
					changed = true
					continue
				}
				seen[v] = true
				out = append(out, v)
			}
			c.Months = out
		}
	}
	return removed, changed
}

// planRepeats works out the rule rewrites and override moves for a save.
// place maps an old month position to its new one (false: no counterpart).
// An override left in a removed month stays, unless a moved override now
// claims its date; there the moved one wins and the other is deleted, the
// same rule day weather follows, because the key cannot hold both.
func planRepeats(cal, next *Calendar, edit StructureEdit, rules []StructureRule, overrides []StructureOverride, place func(int) (int, bool)) repeatsPlan {
	var rp repeatsPlan

	keptMoons := map[int]bool{}
	for _, m := range edit.Moons {
		if m.ID != nil {
			keptMoons[*m.ID] = true
		}
	}
	keptSeasons := map[int]bool{}
	for _, s := range edit.Seasons {
		keptSeasons[s.ID] = true
	}
	removedMoon := map[int]string{}
	for _, m := range cal.Moons {
		if !keptMoons[m.ID] {
			removedMoon[m.ID] = m.Name
		}
	}
	removedSeason := map[int]string{}
	for _, s := range cal.Seasons {
		if !keptSeasons[s.ID] {
			removedSeason[s.ID] = s.Name
		}
	}

	note := func(s string) {
		if len(rp.notes) < maxStructurePreviewEvents {
			rp.notes = append(rp.notes, s)
		}
	}

	for _, sr := range rules {
		// A stored rule the strict parser refuses is left alone: rewriting
		// what cannot be read would be a guess.
		r, err := ParseRecurrenceRule([]byte(sr.Raw))
		if err != nil || r == nil {
			continue
		}
		removed, changed := remapRuleMonths(r, len(cal.Months), place)
		if changed {
			if v, err := (ruleValuer{r}).Value(); err == nil {
				rp.updates = append(rp.updates, RuleUpdate{EventID: sr.EventID, JSON: v.(string)})
				rp.rulesUpdated++
			}
		}
		if len(removed) > 0 {
			rp.rulesRemovedMonth++
			names := make([]string, 0, len(removed))
			for _, m := range removed {
				names = append(names, cal.MonthName(m))
			}
			if m := removed[0]; m > len(next.Months) {
				note(fmt.Sprintf("The repeat rule of %q names %s, which is removed; the rule is left as written and will match nothing there.", sr.Name, strings.Join(names, ", ")))
			} else {
				note(fmt.Sprintf("The repeat rule of %q names %s, which is removed; the rule is left as written and will match the month now in that place.", sr.Name, strings.Join(names, ", ")))
			}
		}

		var lost []string
		for _, c := range r.Match {
			switch c.Kind {
			case RuleMoonPhase:
				if n, ok := removedMoon[c.MoonID]; ok {
					lost = append(lost, "the moon "+n)
				}
			case RuleSeason, RuleSeasonStart:
				if n, ok := removedSeason[c.SeasonID]; ok && c.SeasonID != 0 {
					lost = append(lost, "the season "+n)
				}
			}
		}
		if len(lost) > 0 {
			rp.rulesRemovedRef++
			note(fmt.Sprintf("%q repeats by %s you're removing, so it will match nothing.", sr.Name, strings.Join(lost, " and ")))
		}
	}

	type slot struct {
		event            string
		year, month, day int
	}
	claimed := map[slot]bool{}
	for _, o := range overrides {
		if nm, ok := place(o.Month); ok && nm != o.Month {
			claimed[slot{o.EventID, o.Year, nm, o.Day}] = true
		}
	}
	for _, o := range overrides {
		nm, followed := place(o.Month)
		inRange := o.Month >= 1 && o.Month <= len(cal.Months)
		moved := followed && nm != o.Month
		removedHere := inRange && !followed
		if o.NewMonth != nil {
			if tm, ok := place(*o.NewMonth); ok && tm != *o.NewMonth {
				moved = true
			} else if !ok && *o.NewMonth >= 1 && *o.NewMonth <= len(cal.Months) {
				removedHere = true
			}
		}
		switch {
		case !followed && claimed[slot{o.EventID, o.Year, o.Month, o.Day}]:
			rp.overridesReplaced++
		case removedHere:
			rp.overridesRemoved++
		case moved:
			rp.overridesMoved++
		}
	}
	return rp
}

// repeatNotes words the plan for the preview's "Also" list.
func repeatNotes(rp repeatsPlan) []string {
	var notes []string
	rule := func(n int) string { return fmt.Sprintf("%d repeat %s", n, nounFor(n, "rule", "rules")) }
	change := func(n int) string {
		return fmt.Sprintf("%d one-off %s", n, nounFor(n, "change to a repeat (skipped or moved date)", "changes to repeats (skipped or moved dates)"))
	}
	if rp.rulesUpdated > 0 {
		notes = append(notes, rule(rp.rulesUpdated)+" follow the months they name to their new places.")
	}
	notes = append(notes, rp.notes...)
	if more := rp.rulesRemovedMonth + rp.rulesRemovedRef - len(rp.notes); more > 0 {
		notes = append(notes, fmt.Sprintf("And %d more repeat rules are affected the same way.", more))
	}
	if rp.overridesMoved > 0 {
		notes = append(notes, change(rp.overridesMoved)+" follow their month to its new place.")
	}
	if rp.overridesRemoved > 0 {
		notes = append(notes, change(rp.overridesRemoved)+" sit in removed months; they are left as they are.")
	}
	if rp.overridesReplaced > 0 {
		notes = append(notes, change(rp.overridesReplaced)+" in removed months will be deleted: a moved month's change takes their date.")
	}
	return notes
}
