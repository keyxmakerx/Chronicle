package records

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The lookups an AI preps a fight with: the fights already planned, the
// tests whose results pick their versions, and what Foundry can place.
// All three are the DM's, so Safe privacy leaves them out.

// encountersLookup lists the planned fights in the encounter block's own
// format, so an AI can send one back changed, with what Chronicle knows
// about each that the block doesn't carry.
func (run *lookupRun) encountersLookup(page string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Encounters"}
	if run.l.Encounters == nil {
		return ans, badRequestf("encounters can't be looked up on this server")
	}
	if !run.director() {
		return ans, badRequestf("encounters are left out in Safe privacy; choose Permitted to include them")
	}
	list, err := run.l.Encounters.All(run.ctx, run.campaignID, run.a)
	if err != nil {
		return ans, err
	}
	if page != "" {
		kept := list[:0:0]
		for _, e := range list {
			if sameName(e.PageName, page) {
				kept = append(kept, e)
			}
		}
		list = kept
		ans.Chip = "Encounters · " + page
	}
	report, haveReport := run.foundryReport()
	var b strings.Builder
	fmt.Fprintf(&b, "## Encounters (%d)\n\n", len(list))
	if len(list) == 0 {
		if page != "" {
			b.WriteString("None on " + quote(page) + ".\n")
		} else {
			b.WriteString("None yet.\n")
		}
	}
	for _, e := range list {
		writeEncounterBlock(&b, e)
		switch {
		case e.Playing != "":
			b.WriteString("Plays now: " + quote(e.Playing) + ", picked by the recorded result.\n")
		case len(e.Versions) > 0 && len(e.Open) > 0:
			b.WriteString("Plays now: the base creatures, until " + strings.Join(e.Open, ", ") + " has a result.\n")
		case len(e.Versions) > 0:
			b.WriteString("Plays now: the base creatures; the DM can pick a version in Foundry.\n")
		}
		if haveReport {
			if missing := cannotPlace(e, report); len(missing) > 0 {
				b.WriteString("Foundry can't place: " + strings.Join(missing, ", ") + " (no page and no Foundry monster by that name).\n")
			}
		}
		b.WriteString("\n")
	}
	ans.Text = b.String()
	return ans, nil
}

// cannotPlace names the creatures of every version that have no page and
// that Foundry did not report, once each.
func cannotPlace(e EncounterRef, r FoundryMonsterReport) []string {
	seen := map[string]bool{}
	var out []string
	check := func(cs []EncounterCreature) {
		for _, c := range cs {
			k := strings.ToLower(c.Name)
			if c.EntityID != "" || seen[k] {
				continue
			}
			seen[k] = true
			if r.Find(c.Name) == nil {
				out = append(out, quote(c.Name))
			}
		}
	}
	check(e.Creatures)
	for _, v := range e.Versions {
		check(v.Creatures)
	}
	return out
}

// testsLookup lists the tests on pages, with their results and the one
// recorded: what a version's `test` and `result` name.
func (run *lookupRun) testsLookup(page string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Tests"}
	if run.l.Tests == nil {
		return ans, badRequestf("tests can't be looked up on this server")
	}
	if !run.director() {
		return ans, badRequestf("tests are left out in Safe privacy; choose Permitted to include them")
	}
	list, err := run.l.Tests.Tests(run.ctx, run.campaignID, run.a)
	if err != nil {
		return ans, err
	}
	var lines []string
	for _, t := range list {
		if page != "" && !sameName(t.PageName, page) {
			continue
		}
		labels := make([]string, len(t.Results))
		for i, r := range t.Results {
			labels[i] = r.Label
		}
		line := "- " + t.Name + " (on " + t.PageName + "): results " + strings.Join(labels, ", ")
		if t.Recorded != "" {
			line += "; recorded: " + t.Recorded
		} else {
			line += "; no result yet"
		}
		lines = append(lines, line)
	}
	ans.Chip = fmt.Sprintf("Tests · %d", len(lines))
	var b strings.Builder
	fmt.Fprintf(&b, "## Tests (%d)\n\nA test is made with the Tests block on a page. An encounter version's `test` and `result` name one of these.\n\n", len(lines))
	if len(lines) == 0 {
		b.WriteString("None yet.\n")
	}
	writeLines(&b, lines)
	ans.Text = b.String()
	return ans, nil
}

// foundryReport reads Foundry's list once per paste; ok is false when there
// is none to read.
func (run *lookupRun) foundryReport() (FoundryMonsterReport, bool) {
	if run.monstersRead {
		return run.monsters, run.monsters.ReportedAt != nil
	}
	run.monstersRead = true
	if run.l.Monsters == nil || !run.director() {
		return FoundryMonsterReport{}, false
	}
	r, err := run.l.Monsters.Monsters(run.ctx, run.campaignID, run.a)
	if err != nil {
		return FoundryMonsterReport{}, false
	}
	run.monsters = r
	return r, r.ReportedAt != nil
}

// foundryMonsters lists what Foundry can place, optionally narrowed by a
// part of the name, a level and a role or organization.
func (run *lookupRun) foundryMonsters(r Record) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Foundry monsters"}
	if run.l.Monsters == nil {
		return ans, badRequestf("Foundry's monsters can't be looked up on this server")
	}
	if !run.director() {
		return ans, badRequestf("Foundry's monsters are left out in Safe privacy; choose Permitted to include them")
	}
	rep, err := run.l.Monsters.Monsters(run.ctx, run.campaignID, run.a)
	if err != nil {
		return ans, err
	}
	if rep.ReportedAt == nil {
		ans.Text = "## Foundry's monsters\n\nFoundry hasn't reported any yet. The list arrives when the DM opens the campaign's Foundry world with Chronicle Sync connected.\n"
		return ans, nil
	}
	name := strings.ToLower(firstNonEmpty(r.Str("name"), r.Name))
	level := strings.TrimSpace(r.Str("level"))
	role := strings.ToLower(strings.TrimSpace(r.Str("role")))
	var lines []string
	for _, m := range rep.Monsters {
		if name != "" && !strings.Contains(strings.ToLower(m.Name), name) {
			continue
		}
		if level != "" && m.Level != level {
			continue
		}
		if role != "" && !strings.EqualFold(m.Role, role) && !strings.EqualFold(m.Organization, role) {
			continue
		}
		lines = append(lines, "- "+m.line())
	}
	ans.Chip = fmt.Sprintf("Foundry monsters · %d", len(lines))
	var b strings.Builder
	fmt.Fprintf(&b, "## Foundry's monsters (%d)\n\nAs Foundry reported them %s. Name a creature in an encounter exactly as written here and Foundry can place it, with no Chronicle page needed. Write a Creature page only for a monster that is not here.\n\n", len(lines), reportedAgo(*rep.ReportedAt))
	if rep.Truncated {
		b.WriteString("Foundry had more than it could send, so some are missing; narrow with `name`, `level` or `role`.\n\n")
	}
	if len(lines) == 0 {
		b.WriteString("None match.\n")
	}
	writeLines(&b, lines)
	ans.Text = b.String()
	return ans, nil
}

// reportedAgo is how long ago Foundry reported, in words.
func reportedAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 2*time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + " minutes ago"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d.Hours())) + " hours ago"
	}
	return strconv.Itoa(int(d.Hours()/24)) + " days ago"
}
