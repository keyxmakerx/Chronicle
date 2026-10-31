package crafting

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// What a player reads for a find nobody has identified yet.
const unidentified = "Something unidentified"

// studySays is what each outcome of a study does, lowest first. On a one
// chance find the lowest loses it.
func studySays(once bool) []string {
	low := "not yet"
	if once {
		low = "lose it"
	}
	return []string{low, "learn what it is", "and which recipes use it"}
}

// kindsOf is an item's material kinds the crafting app knows, and the same
// in words: "bone, hide".
func kindsOf(tags []string) ([]string, string) {
	known := knownTags(tags)
	words := make([]string, len(known))
	for i, t := range known {
		words[i] = tagLabel(t)
	}
	return known, strings.Join(words, ", ")
}

// rolledTest is a test as the server rolled it.
type rolledTest struct {
	Dice  []int
	Sides int
	Terms []Term
	Total int
	Notes []string
}

// rollTest rolls the system's dice and adds stat from the hero's sheet; a
// characteristic the sheet has no number for adds nothing, and says so.
func (s *service) rollTest(ctx context.Context, campaignID string, e *EntityInfo, stat string, rules systems.CraftingRules) rolledTest {
	count, size, ok := systems.ParseDice(rules.Dice)
	if !ok {
		count, size = 2, 10
	}
	t := rolledTest{Sides: size, Notes: []string{}}
	sum := 0
	for i := 0; i < count; i++ {
		d := s.roll(size)
		t.Dice = append(t.Dice, d)
		sum += d
	}
	t.Terms = append(t.Terms, Term{Text: fmt.Sprintf("%dd%d", count, size), N: sum})
	if stat != "" {
		label := statLabel(stat, rules)
		if n := s.sheetStat(ctx, campaignID, e.ID, stat); n != nil {
			t.Terms = append(t.Terms, Term{Text: label + ", from " + e.Name + "’s sheet", N: *n})
		} else {
			t.Notes = append(t.Notes, label+" isn’t a number on "+e.Name+"’s sheet, so it added nothing.")
		}
	}
	for _, term := range t.Terms {
		t.Total += term.N
	}
	return t
}

// usedIn names, for each item, the recipes that use it by name or by one of
// its kinds: of the recipes the hero knows, or every recipe but drafts for
// the Director.
func (s *service) usedIn(ctx context.Context, campaignID string, v Viewer, heroID string, items map[string][]string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(items) == 0 {
		return out, nil
	}
	all, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	learned := map[string]bool{}
	if !v.Director && heroID != "" {
		known, err := s.repo.KnownBy(ctx, campaignID, []string{heroID})
		if err != nil {
			return nil, errInternal(err)
		}
		for _, id := range known[heroID] {
			learned[id] = true
		}
	}
	for i := range all {
		r := &all[i]
		if r.Draft || (!v.Director && r.Who == "learned" && !learned[r.ID]) {
			continue
		}
		for item, tags := range items {
			for _, m := range r.Materials {
				if m.ItemID == item || (m.Tag != "" && hasTag(tags, m.Tag)) {
					out[item] = append(out[item], r.Name)
					break
				}
			}
		}
	}
	return out, nil
}

// StudySpoil rolls a hero's study of a find that needs one: a player for a
// hero they claimed, a Director for anyone. The study's row is written
// before the dice, so a hero who already studied the item since the last
// respite is refused and can't roll twice at once. A landed study tells
// the whole party what the item is; a miss on a one chance find loses it.
func (s *service) StudySpoil(ctx context.Context, campaignID string, lineID int64, v Viewer, in TakeInput) (*StudyResult, error) {
	if err := s.spoilsReady(); err != nil {
		return nil, err
	}
	e, err := s.hero(ctx, campaignID, in.CharacterID, v)
	if err != nil {
		return nil, err
	}
	l, ok, err := s.projects.Spoils.GetLine(ctx, campaignID, lineID)
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok || l.Status == SpoilCleared || l.Status == SpoilLost {
		return nil, errNotFound("those spoils are gone")
	}
	if l.Status != SpoilWaiting {
		return nil, errConflictMsg("someone took that first")
	}
	if !l.Study {
		return nil, errInvalid("that needs no study: take it as it is")
	}
	item, err := s.dir.Entity(ctx, campaignID, l.ItemID)
	if err != nil {
		return nil, errInternal(err)
	}
	if item == nil || !item.IsItem {
		return nil, errNotFound("those spoils are gone")
	}
	known, err := s.projects.Spoils.KnownItems(ctx, campaignID, []string{l.ItemID})
	if err != nil {
		return nil, errInternal(err)
	}
	if _, ok := known[l.ItemID]; ok {
		return nil, errConflictMsg("someone already identified it")
	}
	id, ok, err := s.projects.Spoils.StartStudy(ctx, campaignID, Study{ItemID: l.ItemID, LineID: l.ID, CharacterID: e.ID, CreatedBy: v.UserID})
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errConflictMsg(e.Name + " tries again after the next respite")
	}
	rules, _ := s.systemRules(ctx, campaignID)
	t := s.rollTest(ctx, campaignID, e, l.StudyStat, rules)
	res := &StudyResult{Hero: e.Name, Dice: t.Dice, Sides: t.Sides, Terms: t.Terms, Total: t.Total, Tier: tierOf(t.Total, rules.Tiers),
		Tiers: TierLabels(rules.Tiers), Mode: l.Mode, Notes: t.Notes}
	outcome := StudyMiss
	switch {
	case res.Tier > 0:
		outcome = StudyKnown
		if err := s.projects.Spoils.MarkKnown(ctx, campaignID, KnownItem{ItemID: l.ItemID, CharacterID: e.ID, Recipes: res.Tier == 2, LearnedBy: v.UserID}); err != nil {
			return nil, errInternal(err)
		}
		tags, err := s.repo.ListTags(ctx, campaignID)
		if err != nil {
			return nil, errInternal(err)
		}
		res.Known, res.Item = true, item.Name
		_, res.Kinds = kindsOf(tags[l.ItemID])
		if res.Tier == 2 {
			uses, err := s.usedIn(ctx, campaignID, v, e.ID, map[string][]string{l.ItemID: tags[l.ItemID]})
			if err != nil {
				return nil, err
			}
			res.UsedIn = uses[l.ItemID]
		}
	case l.Once:
		lost, err := s.projects.Spoils.LoseLine(ctx, campaignID, l.ID, e.ID, v.UserID, t.Total, s.now())
		if err != nil {
			return nil, errInternal(err)
		}
		// The Director may have handed it out between the read and now;
		// then this was an ordinary miss.
		if lost {
			outcome, res.Lost = StudyLost, true
		}
	}
	if err := s.projects.Spoils.FinishStudy(ctx, campaignID, id, outcome, t.Total); err != nil {
		return nil, errInternal(err)
	}
	return res, nil
}

// RevealSpoil names a waiting find's item to the whole party without a
// roll, as a study that landed would.
func (s *service) RevealSpoil(ctx context.Context, campaignID string, lineID int64, v Viewer) error {
	if err := directorOnly(v); err != nil {
		return err
	}
	if err := s.spoilsReady(); err != nil {
		return err
	}
	l, ok, err := s.projects.Spoils.GetLine(ctx, campaignID, lineID)
	if err != nil {
		return errInternal(err)
	}
	if !ok || l.Status != SpoilWaiting {
		return errNotFound("those spoils are gone")
	}
	if !l.Study {
		return errInvalid("that needs no study, so there is nothing to reveal")
	}
	known, err := s.projects.Spoils.KnownItems(ctx, campaignID, []string{l.ItemID})
	if err != nil {
		return errInternal(err)
	}
	if _, ok := known[l.ItemID]; ok {
		return nil
	}
	if err := s.projects.Spoils.MarkKnown(ctx, campaignID, KnownItem{ItemID: l.ItemID, LearnedBy: v.UserID}); err != nil {
		return errInternal(err)
	}
	if err := s.projects.Spoils.LogStudy(ctx, campaignID, Study{ItemID: l.ItemID, LineID: l.ID, Outcome: StudyRevealed, CreatedBy: v.UserID}); err != nil {
		return errInternal(err)
	}
	return nil
}

// studyText is a study or reveal for the Director's Taken lately.
func studyText(st Study, hero, item string) string {
	rolled := ""
	if st.Rolled != nil {
		rolled = fmt.Sprintf(" (rolled %d)", *st.Rolled)
	}
	switch st.Outcome {
	case StudyRevealed:
		return item + " was revealed to the party"
	case StudyKnown:
		return fmt.Sprintf("%s studied %s%s: the party knows it now", hero, item, rolled)
	case StudyLost:
		return fmt.Sprintf("%s studied %s%s: lost, its one chance", hero, item, rolled)
	}
	return fmt.Sprintf("%s studied %s%s: not yet", hero, item, rolled)
}
