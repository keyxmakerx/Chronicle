package records

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// ChoiceRulesAPI is the part of the hero-choice rules (systems.ChoiceRuleService)
// the kind needs: the Directors' marked list, the dry run and the write.
type ChoiceRulesAPI interface {
	Rules(ctx context.Context, campaignID, fieldKey string, actor systems.EntryActor) ([]systems.ChoiceRule, error)
	DirectorEntries(ctx context.Context, campaignID string, actor systems.EntryActor, fieldKey, asUser string) (*systems.DirectorEntryList, error)
	CheckMarks(ctx context.Context, campaignID string, actor systems.EntryActor, in systems.ChoiceMarksInput) (*systems.MarksCheck, error)
	SetMarks(ctx context.Context, campaignID string, actor systems.EntryActor, in systems.ChoiceMarksInput) ([]systems.ChoiceRule, error)
}

// HeroChoiceRuleKind sets the mark on one pick-list entry for every player:
// the same hero-choice rule a Director sets on the Own entries page. It is the
// only way an AI can take one of the game system's own entries off players'
// lists, since the package's entries cannot be edited or hidden as entries.
// Marks for a single player stay on that page, where the Director sees who
// they are for.
type HeroChoiceRuleKind struct {
	Svc ChoiceRulesAPI
}

func (HeroChoiceRuleKind) Name() string  { return "hero-choice-rule" }
func (HeroChoiceRuleKind) Label() string { return "Hero-choice rule" }
func (HeroChoiceRuleKind) Brief() string {
	return "which pick-list entries players may choose in the hero maker, the game system's own ancestries, classes, kits, cultures and careers included: take one off the list, make it ask first, make everyone take it, or open it again"
}

func (HeroChoiceRuleKind) Doc() string {
	return "A mark on one entry of a character pick list, for every player: the same mark a Director sets on the Own entries page. " +
		"It works on the game system's own entries (an ancestry, class, kit, culture, career...) as well as the campaign's own. " +
		"`entry` is the character field's key, as on `kind: system-entry`; `name` is the entry's name or slug as `what: system-entries` lists it, and that lookup also shows each entry's mark now. " +
		"`mark` is one of: `not-here` (players' hero maker leaves the entry out; the foot of their list says how many entries are not here and names them, with the `reason`), " +
		"`ask-first` (players may pick it only with a Director's yes), `everyone-takes-it` (the list becomes a short list of the entries marked so), or `open` (players pick it freely, the same as no mark). " +
		"Optional `reason` is shown to players; leaving it out keeps the stored one. `action: delete` removes the mark. " +
		"When the game system and the campaign both have an entry of that name, add `source: game` or `source: campaign`. " +
		"Directors always see every entry. Heroes that already hold an entry keep it; the review says which, and their sheets show it breaks the rule. " +
		"Marks for one player are set on the Own entries page, not here.\n\n" +
		"```\n---\nkind: hero-choice-rule\nentry: ancestry\nname: Time Raider\nmark: not-here\nreason: There are no Time Raiders on Therin.\n---\n```"
}

func (k HeroChoiceRuleKind) actor(a Actor) systems.EntryActor {
	return systems.EntryActor{UserID: a.UserID, IsDirector: a.CanAuthorDmOnly()}
}

// markWords maps the words a block may use to a stored state; the UI's
// labels and the stored names are both accepted.
var markWords = map[string]string{
	"not-here": systems.RuleNotOffered, "not-offered": systems.RuleNotOffered, "hidden": systems.RuleNotOffered, "hide": systems.RuleNotOffered,
	"ask-first":         systems.RuleAskFirst,
	"everyone-takes-it": systems.RuleRequired, "required": systems.RuleRequired,
	"open": "clear", "clear": "clear",
}

// markLabels are the Own entries page's names for each state.
var markLabels = map[string]string{
	systems.RuleNotOffered: "Not here", systems.RuleAskFirst: "Ask first",
	systems.RuleRequired: "Everyone takes it", systems.RuleOpen: "Open", "clear": "Open",
}

func markState(word string) (string, bool) {
	w := strings.ToLower(strings.TrimSpace(word))
	w = strings.NewReplacer("_", "-", " ", "-").Replace(w)
	st, ok := markWords[w]
	return st, ok
}

// heroChoicePlan is a checked record: the entry it names and the batch it
// writes.
type heroChoicePlan struct {
	entry systems.DirectorEntry
	in    systems.ChoiceMarksInput
	state string
}

// resolve checks the record and finds its entry; the error is the review's
// plain sentence.
func (k HeroChoiceRuleKind) resolve(ctx context.Context, campaignID string, a Actor, r Record) (*heroChoicePlan, error) {
	if !a.CanAuthorDmOnly() {
		return nil, apperror.NewForbidden("only the campaign's Directors can change who may pick what")
	}
	field := strings.ToLower(r.Str("entry"))
	if field == "" {
		return nil, apperror.NewValidation("a hero-choice rule needs `entry:` (the character field, for example ancestry)")
	}
	if r.Name == "" {
		return nil, apperror.NewValidation("a hero-choice rule needs `name:` (the entry it marks)")
	}
	state := "clear"
	if r.Action != ActionDelete {
		var ok bool
		if state, ok = markState(r.Str("mark")); !ok {
			return nil, apperror.NewValidation("`mark:` must be not-here, ask-first, everyone-takes-it or open")
		}
	}
	source := ""
	switch s := strings.ToLower(r.Str("source")); s {
	case "":
	case "game", "package", "system":
		source = systems.ChoiceSourcePackage
	case "campaign":
		source = systems.ChoiceSourceCampaign
	default:
		return nil, apperror.NewValidation("`source:` must be game or campaign")
	}
	list, err := k.Svc.DirectorEntries(ctx, campaignID, k.actor(a), field, "")
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(strings.TrimSpace(r.Name))
	var found []systems.DirectorEntry
	for _, e := range list.Entries {
		if source != "" && e.Source != source {
			continue
		}
		if strings.ToLower(strings.TrimSpace(e.Name)) == want || (e.Slug != "" && strings.ToLower(e.Slug) == want) {
			found = append(found, e)
		}
	}
	switch {
	case len(found) == 0:
		return nil, apperror.NewValidation("no " + field + " entry is called " + quote(r.Name) + "; ask `what: system-entries` with `field: " + field + "` for the names")
	case len(found) > 1:
		return nil, apperror.NewValidation("more than one " + field + " entry is called " + quote(r.Name) + "; add `source: game` or `source: campaign`")
	}
	mark := systems.ChoiceMark{Source: found[0].Source, Key: found[0].Key, State: state}
	if _, ok := r.Fields["reason"]; ok && state != "clear" {
		reason := r.Str("reason")
		mark.Reason = &reason
	}
	return &heroChoicePlan{entry: found[0], state: state, in: systems.ChoiceMarksInput{FieldKey: field, Marks: []systems.ChoiceMark{mark}}}, nil
}

func (k HeroChoiceRuleKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if k.Svc == nil {
		return Plan{Error: "hero-choice rules can't be changed on this server yet"}
	}
	p, err := k.resolve(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	check, err := k.Svc.CheckMarks(ctx, campaignID, k.actor(a), p.in)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	out := Plan{Summary: heroChoiceSummary(p)}
	cur := systems.RuleOpen
	if p.entry.Everyone != nil {
		cur = p.entry.Everyone.State
	}
	if cur == p.state || (cur == systems.RuleOpen && p.state == "clear") {
		if p.in.Marks[0].Reason == nil || (p.entry.Everyone != nil && *p.in.Marks[0].Reason == p.entry.Everyone.Reason) {
			out.Warnings = append(out.Warnings, "it already has this mark for every player")
		}
	}
	if p.state == "clear" && p.entry.Inherited {
		out.Warnings = append(out.Warnings, "the mark on every entry of its source still applies to it ("+markLabels[p.entry.State]+")")
	}
	if len(p.entry.Players) > 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf("its %d mark(s) for single players stay and still win for those players", len(p.entry.Players)))
	}
	if check != nil && check.Count > 0 {
		out.Warnings = append(out.Warnings, brokenHeroesWarning(check))
	}
	return out
}

// heroChoiceSummary is the review's one line for a mark.
func heroChoiceSummary(p *heroChoicePlan) string {
	name := quote(p.entry.Name)
	from := "the campaign's own"
	if p.entry.Source == systems.ChoiceSourcePackage {
		from = "the game system's"
	}
	field := p.in.FieldKey
	var s string
	switch p.state {
	case systems.RuleNotOffered:
		s = "takes " + name + " (" + from + ") off players' " + field + " list in the hero maker (Not here)"
	case systems.RuleAskFirst:
		s = "players must ask a Director before picking " + name + " (" + from + ") as their " + field + " (Ask first)"
	case systems.RuleRequired:
		s = "puts " + name + " (" + from + ") on the short list every player's " + field + " is chosen from (Everyone takes it)"
	default:
		s = "removes the mark on " + name + " (" + from + "), so every player may pick it as their " + field
	}
	if m := p.in.Marks[0]; m.Reason != nil && *m.Reason != "" {
		s += "; players read: " + quote(*m.Reason)
	}
	return s
}

// brokenHeroesWarning names the heroes that already hold what the mark
// takes away. They are never changed; their sheets show the break.
func brokenHeroesWarning(c *systems.MarksCheck) string {
	var names []string
	for _, h := range c.Heroes {
		n := h.HeroName
		if h.PlayerName != "" {
			n += " (" + h.PlayerName + ")"
		}
		names = append(names, n)
	}
	sort.Strings(names)
	const shown = 8
	more := ""
	if len(names) > shown {
		more = fmt.Sprintf(" and %d more", len(names)-shown)
		names = names[:shown]
	}
	return fmt.Sprintf("%d hero(es) already hold it and keep it, flagged on their sheets: %s%s", c.Count, strings.Join(names, ", "), more)
}

func (k HeroChoiceRuleKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if k.Svc == nil {
		return apperror.NewBadRequest("hero-choice rules can't be changed on this server yet")
	}
	// Resolved again: the entry or its marks may have changed since review.
	p, err := k.resolve(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	_, err = k.Svc.SetMarks(ctx, campaignID, k.actor(a), p.in)
	return err
}

// Export lists the marks for every player, the ones a block can change. A
// mark on a whole source is listed so the AI knows why entries are marked,
// though only the Own entries page changes it.
func (k HeroChoiceRuleKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if k.Svc == nil || !a.CanAuthorDmOnly() {
		return "", nil
	}
	rows, err := k.Svc.Rules(ctx, campaignID, "", k.actor(a))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	single := 0
	for _, r := range rows {
		if r.Target != "" {
			single++
			continue
		}
		name := r.Name
		if r.IsGroup() {
			name = "every game-system entry"
			if r.Source == systems.ChoiceSourceCampaign {
				name = "every campaign entry"
			}
			name += " (set on the Own entries page)"
		}
		b.WriteString("- " + r.FieldKey + ": " + name + ", " + markLabels[r.State])
		if r.Reason != "" {
			b.WriteString(", reason " + quote(oneLine(r.Reason, 140)))
		}
		b.WriteString("\n")
	}
	if single > 0 {
		fmt.Fprintf(&b, "- and %d mark(s) for single players, set on the Own entries page\n", single)
	}
	return b.String(), nil
}
