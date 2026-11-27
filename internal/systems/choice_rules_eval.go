package systems

import "strings"

// This file is the one place the precedence of hero-choice rules is decided
// (ADR-075). It is pure, so the plan, the picker, every write check, the Directors'
// "as this player" view and the "heroes that would break" count all call it
// and cannot disagree.
//
// For one entry, the most specific rule wins, never the order rules were
// written: the player's own row for the entry, then the player's row for the
// entry's group, then the row for everyone about the entry, then the row for
// everyone about the group, then open. Who a rule is for counts before what
// it names, so "no Furies, except Pat" works.

type ruleRef struct{ source, key, target string }

// ruleIndex holds the rows that apply to one viewer: rows for everyone and
// rows for that player. Rows about other players never get this far.
type ruleIndex map[ruleRef]ChoiceRule

func indexRules(rows []ChoiceRule, viewer string) ruleIndex {
	ix := ruleIndex{}
	for _, r := range rows {
		if r.Target != "" && r.Target != viewer {
			continue
		}
		ix[ruleRef{r.Source, r.Key, r.Target}] = r
	}
	return ix
}

// resolved is what the rules make of one entry for one viewer.
type resolved struct {
	State string
	// Pinned marks an entry offered to this player by name: a per-player
	// open row, or an entry that is theirs (one they wrote, or kept to a
	// character they play). Pinned entries stay on a required short list.
	Pinned bool
	// Rule is the row that decided; nil when nothing did.
	Rule *ChoiceRule
}

// impliedOpen reports whether the entry is the viewer's own: one they wrote with
// a Director's permission, or kept to a character they play. Such an entry
// counts as open for them without a stored row.
func impliedOpen(c Choice) bool { return c.Mine || c.ForName != "" }

func (ix ruleIndex) resolve(c Choice, viewer string) resolved {
	lookAt := func(source, key, target string) (resolved, bool) {
		r, ok := ix[ruleRef{source, key, target}]
		if !ok {
			return resolved{}, false
		}
		return resolved{State: r.State, Pinned: r.State == RuleOpen && target != "", Rule: &r}, true
	}
	// A rewrite takes a standard entry's place, so a row about that standard
	// entry still holds for it; a row about the rewrite itself comes first.
	look := func(key, target string) (resolved, bool) {
		if v, ok := lookAt(c.Source, key, target); ok || key == RuleGroupKey {
			return v, ok
		}
		if c.Origin == OriginRewritten && c.StandsFor != "" {
			return lookAt(ChoiceSourcePackage, c.StandsFor, target)
		}
		return resolved{}, false
	}
	// A viewer with no account has no rows of their own.
	player := func(key string) (resolved, bool) {
		if viewer == "" {
			return resolved{}, false
		}
		return look(key, viewer)
	}
	if c.Key != "" {
		if v, ok := player(c.Key); ok {
			return v
		}
	}
	if impliedOpen(c) {
		return resolved{State: RuleOpen, Pinned: true}
	}
	if v, ok := player(RuleGroupKey); ok {
		return v
	}
	if c.Key != "" {
		if v, ok := look(c.Key, ""); ok {
			return v
		}
	}
	if v, ok := look(RuleGroupKey, ""); ok {
		return v
	}
	return resolved{State: RuleOpen}
}

// limitResult is a pick list after the rules: what stays, what went, and
// what the viewer is told about it.
type limitResult struct {
	Kept     []Choice
	Excluded []Choice
	NotHere  []NotHere
	// Required: some entry resolved to required, so the list is a short list.
	Required bool
	// Prefilled is the one entry a required list is left with.
	Prefilled string
	// Limited: at least one row applies to this viewer.
	Limited bool
}

// maxNotHereNames bounds the names a list's foot carries; the count stays
// whole.
const maxNotHereNames = 100

// applyChoiceRules narrows choices (the list the viewer can read) by the
// rows for this viewer. The caller passes only entries the viewer can read,
// so a group never reaches one they cannot. When any entry resolves to
// required the list is those entries plus the viewer's own pinned ones, minus
// anything the viewer's rows take away; it is filled in only when exactly
// one entry is left.
func applyChoiceRules(choices []Choice, rows []ChoiceRule, viewer string) limitResult {
	ix := indexRules(rows, viewer)
	res := limitResult{Limited: len(ix) > 0}
	if !res.Limited {
		res.Kept = choices
		return res
	}
	states, required := classifyChoices(choices, ix, viewer)
	res.Required = required
	for i, c := range choices {
		st := states[i]
		if st.listed {
			if st.State == RuleAskFirst {
				c.State = ChoiceStateAskFirst
			}
			res.Kept = append(res.Kept, c)
			continue
		}
		res.Excluded = append(res.Excluded, c)
		// Only an entry a rule names is "not here"; the many a short list
		// leaves out are not listed one by one.
		if st.State == RuleNotOffered {
			res.NotHere = append(res.NotHere, NotHere{Name: c.Name, Reason: ruleReason(st.Rule)})
		}
	}
	if res.Required && len(res.Kept) == 1 && res.Kept[0].State == "" {
		res.Prefilled = res.Kept[0].Name
	}
	if res.Kept == nil {
		res.Kept = []Choice{}
	}
	return res
}

// classified is a resolved entry plus whether it stays on the list.
type classified struct {
	resolved
	listed bool
}

// classifyChoices resolves every entry and decides which stay: without a
// required entry, all but the not-offered ones; with one, the required
// entries and the viewer's own pinned ones.
func classifyChoices(choices []Choice, ix ruleIndex, viewer string) ([]classified, bool) {
	out := make([]classified, len(choices))
	required := false
	for i, c := range choices {
		out[i].resolved = ix.resolve(c, viewer)
		if out[i].State == RuleRequired {
			required = true
		}
	}
	for i := range out {
		if required {
			out[i].listed = out[i].State == RuleRequired || out[i].Pinned
		} else {
			out[i].listed = out[i].State != RuleNotOffered
		}
	}
	return out, required
}

func ruleReason(r *ChoiceRule) string {
	if r == nil {
		return ""
	}
	return r.Reason
}

// fieldRules is the viewer-facing summary of a limitResult; nil when no rule
// applies.
func (r limitResult) fieldRules() *FieldRules {
	if !r.Limited {
		return nil
	}
	fr := &FieldRules{Limited: true, Required: r.Required, Prefilled: r.Prefilled, NotHereCount: len(r.NotHere)}
	names := r.NotHere
	if len(names) > maxNotHereNames {
		names = names[:maxNotHereNames]
	}
	fr.NotHere = names
	return fr
}

// choiceByName finds a choice by name, ignoring case and surrounding space,
// as a hero's stored value is matched everywhere.
func choiceByName(list []Choice, name string) *Choice {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil
	}
	for i := range list {
		if strings.ToLower(strings.TrimSpace(list[i].Name)) == name {
			return &list[i]
		}
	}
	return nil
}
