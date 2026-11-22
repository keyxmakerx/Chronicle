package entities

import (
	"encoding/json"
	"strconv"
	"strings"
)

// practice_plan.go turns a player's own creator plan into the plan a
// practice hero is made from, and words a saved practice hero for a page.

// practicePlan is the player's own limited plan with what the rules held back
// put back in. The input must be the plan as this player is served it, never
// the unlimited one: the held entries are a subset of what that player may
// read, so nothing they cannot read is revealed. Entries the rules keep from
// them come back labelled not_offered (including those a required short list
// leaves out); ask-first entries keep their state, which the creator marks but
// does not act on. No rule locks or forces a step in practice, so the rules
// of each step are dropped.
func practicePlan(plan *HeroPlan) *HeroPlan {
	out := &HeroPlan{Steps: []HeroStep{}, Practice: true}
	if plan == nil {
		return out
	}
	out.SystemID, out.SystemName = plan.SystemID, plan.SystemName
	for _, st := range plan.Steps {
		step := HeroStep{FieldKey: st.FieldKey, Label: st.Label, CanMake: st.CanMake, Needed: st.Needed, Parent: st.Parent, CountFrom: st.CountFrom}
		step.Choices = make([]HeroChoice, 0, len(st.Choices)+len(st.Held))
		step.Choices = append(step.Choices, st.Choices...)
		for _, c := range st.Held {
			c.State = HeroChoiceStateNotOffered
			c.Held = false
			step.Choices = append(step.Choices, c)
		}
		out.Steps = append(out.Steps, step)
	}
	return out
}

// practiceSummary words a practice hero in the order the party block does:
// ancestry then class, then ", level N" when the level is known. It is read
// from the picks alone, so a list of many makers needs no plan per maker.
func practiceSummary(picks map[string]HeroPick, level *int) string {
	read := func(keys []string) string {
		for _, k := range keys {
			if p, ok := picks[k]; ok && strings.TrimSpace(p.Name) != "" {
				return strings.Join(strings.Fields(p.Name), " ")
			}
		}
		return ""
	}
	parts := []string{}
	for _, v := range []string{read(partyAncestryKeys), read(partyClassKeys)} {
		if v != "" {
			parts = append(parts, v)
		}
	}
	line := strings.Join(parts, " ")
	if level != nil {
		if line != "" {
			line += ", "
		}
		line += "level " + strconv.Itoa(*level)
	}
	return line
}

// Words under a pick the rules keep from, or hold for a Director's word.
const (
	practiceNoteNotOffered = "Not offered at this table. Needs the Director's yes to play."
	practiceNoteAskFirst   = "Ask first at this table."
)

// practicePickRows is one row per pick, in the plan's step order, with the
// plain note under an entry the rules keep from, or hold for, this player.
// A pick whose step or entry has left the plan is shown without a note.
func practicePickRows(plan *HeroPlan, picks map[string]HeroPick) []PracticePickRow {
	rows := []PracticePickRow{}
	if plan == nil {
		return rows
	}
	for _, st := range plan.Steps {
		p, ok := picks[st.FieldKey]
		if !ok || strings.TrimSpace(p.Name) == "" {
			continue
		}
		row := PracticePickRow{Label: st.Label, Value: p.Name, Options: strings.Join(p.Options, ", ")}
		for _, c := range st.Choices {
			if !strings.EqualFold(c.Name, p.Name) {
				continue
			}
			switch c.State {
			case HeroChoiceStateNotOffered:
				row.Note = practiceNoteNotOffered
			case HeroChoiceStateAskFirst:
				row.Note = practiceNoteAskFirst
			}
			break
		}
		rows = append(rows, row)
	}
	return rows
}

// practiceNeedsYes reports whether a practice hero holds an entry the rules
// keep from its maker. Such a hero can become the real hero only with a
// Director's yes, which is the only way it ever becomes one.
func practiceNeedsYes(plan *HeroPlan, picks map[string]HeroPick) bool {
	for _, r := range practicePickRows(plan, picks) {
		if r.Note != "" {
			return true
		}
	}
	return false
}

// practiceHoldsLine is the muted second line under a waiting ask: the picks a
// yes would approve that the maker's rules do not offer them. It names at most
// three and counts the rest; empty when there is nothing to say.
func practiceHoldsLine(maker string, names []string) string {
	if len(names) == 0 {
		return ""
	}
	shown := names
	if len(shown) > maxHeldNames {
		shown = shown[:maxHeldNames]
	}
	line := "Holds picks you don't offer " + maker + ": " + strings.Join(shown, ", ")
	if more := len(names) - len(shown); more > 0 {
		line += " and " + strconv.Itoa(more) + " more"
	}
	return line + "."
}

// practiceCopyFields reads a hero's field values into picks for each step of
// the plan: the step's current value when it names an entry the plan offers,
// and the options bought from it. hidden holds keys the viewer may not read,
// which are left out. The level comes from the "level" field when it is open
// to the viewer and a whole number.
func practiceCopyFields(plan *HeroPlan, data map[string]any, hidden map[string]bool) (map[string]HeroPick, *int) {
	picks := map[string]HeroPick{}
	if plan == nil {
		return picks, nil
	}
	for _, st := range plan.Steps {
		if hidden[st.FieldKey] {
			continue
		}
		name, _ := data[st.FieldKey].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var choice *HeroChoice
		for i := range st.Choices {
			if strings.EqualFold(st.Choices[i].Name, name) {
				choice = &st.Choices[i]
				break
			}
		}
		if choice == nil {
			continue
		}
		pick := HeroPick{Name: choice.Name}
		if raw, ok := data[st.FieldKey+heroOptionsSuffix].(string); ok && strings.TrimSpace(raw) != "" {
			pick.Options = practiceBoughtNames(raw, choice)
		}
		picks[st.FieldKey] = pick
	}
	dropUnfitPicks(plan, picks)
	var level *int
	if !hidden["level"] {
		switch v := data["level"].(type) {
		case float64:
			if v == float64(int(v)) && v >= 0 && v <= 1000 {
				n := int(v)
				level = &n
			}
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 && n <= 1000 {
				level = &n
			}
		}
	}
	return picks, level
}

// practiceBoughtNames reads the options a hero bought, keeping only those
// the entry still offers so a stale name cannot make the copy unsavable.
func practiceBoughtNames(raw string, choice *HeroChoice) []string {
	costs, _, ok := heroBuyList(choice.Properties)
	if !ok {
		return nil
	}
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := []string{}
	for _, n := range names {
		k := strings.ToLower(strings.TrimSpace(n))
		if _, known := costs[k]; known && !seen[k] {
			seen[k] = true
			out = append(out, strings.TrimSpace(n))
		}
	}
	return out
}

// dropUnfitPicks removes copied picks the hero's other picks no longer allow,
// such as a subclass under a class it has since changed or a kit on a class
// that uses none. A hero saved before its system declared these steps can
// hold such a pair, and the copy must still be savable.
func dropUnfitPicks(plan *HeroPlan, picks map[string]HeroPick) {
	steps := make(map[string]*HeroStep, len(plan.Steps))
	found := map[string]*HeroChoice{}
	for i := range plan.Steps {
		st := &plan.Steps[i]
		steps[st.FieldKey] = st
	}
	for _, dependent := range []bool{false, true} {
		for key, pick := range picks {
			st := steps[key]
			if st == nil || (st.Parent != "") != dependent {
				continue
			}
			for i := range st.Choices {
				if !strings.EqualFold(st.Choices[i].Name, pick.Name) {
					continue
				}
				if found[key] == nil {
					found[key] = &st.Choices[i]
				}
				if p := found[st.Parent]; dependent && p != nil && strings.EqualFold(st.Choices[i].Parent, p.Name) {
					found[key] = &st.Choices[i]
					break
				}
			}
		}
	}
	// Dropping one pick can change what another is allowed (a dropped
	// subclass no longer overrides its class's kit count), so repeat until
	// nothing more is dropped.
	chosen := func(k string) *HeroChoice { return found[k] }
	for dropped := true; dropped; {
		dropped = false
		for key := range picks {
			if c := found[key]; c != nil && checkHeroStepFit(steps[key], steps, c, chosen) != nil {
				delete(picks, key)
				delete(found, key)
				dropped = true
			}
		}
	}
}
