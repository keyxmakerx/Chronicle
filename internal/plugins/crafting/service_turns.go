package crafting

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ---------- Experimenting ----------

// tryable lists the recipes a hero could still find by experimenting:
// approved, set to be found that way, and taught only to those who learn
// them, which this hero hasn't. A recipe without "Finding it by
// experimenting" is never found, hinted at or counted here.
func (s *service) tryable(ctx context.Context, campaignID, characterID string) ([]Recipe, error) {
	all, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	known, err := s.repo.KnownBy(ctx, campaignID, []string{characterID})
	if err != nil {
		return nil, errInternal(err)
	}
	learned := map[string]bool{}
	for _, id := range known[characterID] {
		learned[id] = true
	}
	var out []Recipe
	for _, r := range all {
		if !r.Draft && r.Extras.Discover != nil && r.Who == "learned" && !learned[r.ID] && len(r.Materials) > 0 {
			out = append(out, r)
		}
	}
	return out, nil
}

// tryRight counts the recipe's materials a try got right: the right item,
// or one carrying the right tag, in the right amount. Each thing put in
// counts for one material at most, so the best pairing is found.
func tryRight(mats []Material, picks []TryPick, tags map[string][]string) int {
	fits := func(m Material, p TryPick) bool {
		if p.Qty != m.Qty {
			return false
		}
		if m.ItemID != "" {
			return p.ItemID == m.ItemID
		}
		return hasTag(tags[p.ItemID], m.Tag)
	}
	owner := make([]int, len(picks))
	for i := range owner {
		owner[i] = -1
	}
	var place func(mi int, seen []bool) bool
	place = func(mi int, seen []bool) bool {
		for pi, p := range picks {
			if seen[pi] || !fits(mats[mi], p) {
				continue
			}
			seen[pi] = true
			if owner[pi] < 0 || place(owner[pi], seen) {
				owner[pi] = mi
				return true
			}
		}
		return false
	}
	n := 0
	for mi := range mats {
		if place(mi, make([]bool, len(picks))) {
			n++
		}
	}
	return n
}

// closeAt reads "k of n" as k; 0 when the recipe has no near miss.
func closeAt(s string) int {
	k, _, ok := strings.Cut(s, " of ")
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(k)
	if err != nil {
		return 0
	}
	return n
}

// tryPicks checks what a hero puts into a try: something, a handful of
// different things at most, each held in the amount put in. Repeats of one
// item add up.
func tryPicks(in TryInput, pack *Pack, who string) ([]TryPick, error) {
	held := map[string]PackItem{}
	for _, it := range pack.Items {
		h := held[it.ItemID]
		h.ItemID, h.Name, h.Qty = it.ItemID, it.Name, h.Qty+it.Qty
		held[it.ItemID] = h
	}
	var out []TryPick
	at := map[string]int{}
	for _, p := range in.Items {
		id := strings.TrimSpace(p.ItemID)
		if id == "" || p.Qty < 1 {
			continue
		}
		if i, ok := at[id]; ok {
			out[i].Qty += p.Qty
			continue
		}
		at[id] = len(out)
		out = append(out, TryPick{ItemID: id, Qty: p.Qty})
	}
	if len(out) == 0 {
		return nil, errInvalid("pick what goes in")
	}
	if len(out) > maxTryItems {
		return nil, errInvalid(fmt.Sprintf("a try takes at most %d different things", maxTryItems))
	}
	for _, p := range out {
		h, ok := held[p.ItemID]
		if p.Qty > maxTryQty {
			return nil, errInvalid(fmt.Sprintf("a try takes at most %d of one thing", maxTryQty))
		}
		if !ok || h.Qty < p.Qty {
			return nil, errInvalid(fmt.Sprintf("%s doesn’t hold that many", who))
		}
	}
	return out, nil
}

// Experiment is a hero's try at a recipe they don't know, once at each
// respite. What goes in is matched against every recipe they could still
// find: all of it right finds the recipe and teaches it; enough of it right
// is a near miss, which shows only the recipe's hint. A try that finds
// nothing costs what the nearest recipe says (all, half or none of what
// went in); with no recipe near at all, it costs everything.
func (s *service) Experiment(ctx context.Context, campaignID, characterID string, v Viewer, in TryInput) (*TryResult, error) {
	if err := s.projectsReady(); err != nil {
		return nil, err
	}
	e, err := s.hero(ctx, campaignID, characterID, v)
	if err != nil {
		return nil, err
	}
	pack, err := s.projects.Armory.Pack(ctx, campaignID, v, e.ID)
	if err != nil {
		return nil, err
	}
	picks, err := tryPicks(in, pack, e.Name)
	if err != nil {
		return nil, err
	}
	cands, err := s.tryable(ctx, campaignID, e.ID)
	if err != nil {
		return nil, err
	}
	tags, err := s.repo.ListTags(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	var found, near *Recipe
	best, nearest := 0, 0
	for i := range cands {
		r := &cands[i]
		k := tryRight(r.Materials, picks, tags)
		if k == len(r.Materials) && len(picks) == k {
			found = r
			break
		}
		if c := closeAt(r.Extras.Discover.CloseWhen); c > 0 && k >= c && k > best {
			best = k
			near = r
		}
		if k > nearest {
			nearest = k
		}
	}
	out := &TryResult{Outcome: TryNothing, Cost: TryUsed, Used: []string{}, Back: []string{}}
	switch {
	case found != nil:
		out.Outcome = TryFound
	case near != nil:
		out.Outcome, out.Hint, out.Cost = TryClose, near.Extras.Discover.Hint, near.Extras.Discover.FailedTry
	case nearest > 0:
		// The recipe that came nearest says what a failed try costs, though
		// it wasn't near enough to say anything else.
		for i := range cands {
			if tryRight(cands[i].Materials, picks, tags) == nearest {
				out.Cost = cands[i].Extras.Discover.FailedTry
				break
			}
		}
	}

	rid, err := s.projects.Store.RespiteID(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	if ok, err := s.projects.Store.TakeTurn(ctx, campaignID, e.ID, TurnTry, "", rid); err != nil {
		return nil, errInternal(err)
	} else if !ok {
		return nil, errConflictMsg(e.Name + " has already experimented since the last respite. The next try comes with the next respite.")
	}

	names := map[string]string{}
	for _, it := range pack.Items {
		names[it.ItemID] = it.Name
	}
	var spend []PackItem
	for _, p := range picks {
		used := p.Qty
		switch out.Cost {
		case TryHalf:
			used = p.Qty / 2
		case TryKept:
			used = 0
		}
		if used > 0 {
			spend = append(spend, PackItem{ItemID: p.ItemID, Name: names[p.ItemID], Qty: used})
			out.Used = append(out.Used, fmt.Sprintf("%d × %s", used, names[p.ItemID]))
		}
		if back := p.Qty - used; back > 0 {
			out.Back = append(out.Back, fmt.Sprintf("%d × %s", back, names[p.ItemID]))
		}
	}
	if len(spend) > 0 {
		if _, err := s.projects.Armory.Use(ctx, campaignID, v, e.ID, spend, "in an experiment"); err != nil {
			if rerr := s.projects.Store.ReturnTurn(ctx, campaignID, e.ID, TurnTry, "", rid); rerr != nil {
				slog.Error("crafting: a failed try's turn was not given back", slog.String("character_id", e.ID), slog.Any("error", rerr))
			}
			return nil, err
		}
	}
	if found == nil {
		return out, nil
	}
	out.Recipe, out.RecipeID, out.Hint = found.Name, found.ID, found.Extras.Discover.Hint
	learners := []Named{{ID: e.ID, Name: e.Name}}
	if found.Extras.Discover.LearnedBy == "party" {
		out.Party = true
		party, err := s.dir.Party(ctx, campaignID, directorSight, "")
		if err != nil {
			slog.Warn("crafting: cannot list the party for a discovery", slog.String("campaign_id", campaignID), slog.Any("error", err))
		}
		for _, m := range party {
			if m.ID != e.ID {
				learners = append(learners, m)
			}
		}
	}
	for i, m := range learners {
		if err := s.repo.Learn(ctx, campaignID, found.ID, m.ID, v.UserID); err != nil {
			slog.Error("crafting: A DISCOVERY WAS NOT LEARNED", slog.String("recipe_id", found.ID), slog.String("character_id", m.ID), slog.Any("error", err))
			if i == 0 {
				return nil, apperror.NewInternal(fmt.Errorf("crafting: %s found %s but it wasn't saved; tell your Director so they can teach it: %w", e.Name, found.Name, err))
			}
			continue
		}
		out.Learned = append(out.Learned, m.Name)
	}
	return out, nil
}

// ---------- Breaking down ----------

// salvageItem finds the item a salvage line gives back: one of the recipe's
// named materials by its name, else any item of the campaign by its name.
// "" when there is none, as for an "any <tag>" material.
func (s *service) salvageItem(ctx context.Context, campaignID string, r *Recipe, name string) (string, error) {
	want := strings.ToLower(strings.TrimSpace(name))
	items, _, err := s.dir.ListItems(ctx, campaignID, directorSight, "")
	if err != nil {
		return "", errInternal(err)
	}
	byID := map[string]string{}
	for _, it := range items {
		byID[it.ID] = it.Name
	}
	for _, m := range r.Materials {
		if m.ItemID != "" && strings.ToLower(byID[m.ItemID]) == want {
			return m.ItemID, nil
		}
	}
	for _, it := range items {
		if strings.ToLower(it.Name) == want {
			return it.ID, nil
		}
	}
	return "", nil
}

// salvageText is what a recipe's item gives back, as the block shows it.
func salvageText(x Extras) []string {
	var out []string
	for _, sv := range x.Salvage {
		out = append(out, fmt.Sprintf("%d × %s", sv.Qty, sv.Item))
	}
	return out
}

// BreakDown takes one of a recipe's made items from the hero's pack, the
// remembered copy they picked or any, and gives back what the recipe's
// "Breaking it down" says. It takes no roll and no turn. The hero must know
// the recipe.
func (s *service) BreakDown(ctx context.Context, campaignID, characterID string, v Viewer, in BreakInput) (*BreakResult, error) {
	if err := s.projectsReady(); err != nil {
		return nil, err
	}
	e, err := s.hero(ctx, campaignID, characterID, v)
	if err != nil {
		return nil, err
	}
	r, err := s.load(ctx, campaignID, strings.TrimSpace(in.RecipeID))
	if err != nil {
		return nil, err
	}
	if ok, err := s.knows(ctx, campaignID, r, e.ID); err != nil {
		return nil, err
	} else if !ok {
		return nil, errNotFound("recipe not found")
	}
	if len(r.Extras.Salvage) == 0 || r.Result.ItemID == "" {
		return nil, errInvalid("what " + r.Name + " makes can’t be broken down")
	}
	pack, err := s.projects.Armory.Pack(ctx, campaignID, v, e.ID)
	if err != nil {
		return nil, err
	}
	var held *PackItem
	for i := range pack.Items {
		if pack.Items[i].ItemID == r.Result.ItemID {
			held = &pack.Items[i]
		}
	}
	if held == nil || held.Qty < 1 {
		return nil, errInvalid(e.Name + " holds none to break down")
	}
	it := PackItem{ItemID: held.ItemID, Name: held.Name, Qty: 1}
	if in.Copy > 0 {
		it.Use = []KeptPick{{ID: in.Copy, N: 1}}
	}
	type back struct {
		id  string
		qty int
		txt string
	}
	var gives []back
	out := &BreakResult{Item: held.Name, Back: []string{}, Notes: []string{}}
	for _, sv := range r.Extras.Salvage {
		id, err := s.salvageItem(ctx, campaignID, r, sv.Item)
		if err != nil {
			return nil, err
		}
		if id == "" {
			out.Notes = append(out.Notes, sv.Item+" isn’t an item in the Armory, so it couldn’t be given back. Tell your Director.")
			continue
		}
		gives = append(gives, back{id, sv.Qty, fmt.Sprintf("%d × %s", sv.Qty, sv.Item)})
	}
	// Nothing that would come back is a real item: keep the item rather
	// than break it for nothing.
	if len(gives) == 0 {
		return nil, errInvalid("nothing breaking down " + held.Name + " gives back is an item in the Armory, so it was kept. Tell your Director.")
	}
	if _, err := s.projects.Armory.Use(ctx, campaignID, v, e.ID, []PackItem{it}, "to break it down"); err != nil {
		return nil, err
	}
	for _, g := range gives {
		why := fmt.Sprintf("Got %s back by breaking down %s", g.txt, held.Name)
		if err := s.projects.Armory.GiveFor(ctx, campaignID, v, e.ID, e.ID, g.id, g.qty, why, Kept{}); err != nil {
			slog.Error("crafting: AN ITEM WAS BROKEN DOWN BUT WHAT IT GIVES BACK WAS NOT GIVEN",
				slog.String("character_id", e.ID), slog.String("item_id", g.id), slog.Any("error", err))
			out.Notes = append(out.Notes, g.txt+" couldn’t be put in "+e.Name+"’s pack. Tell your Director.")
			continue
		}
		out.Back = append(out.Back, g.txt)
	}
	return out, nil
}

// ---------- Helping ----------

// partyIDs is the party as the viewer sees it, by id.
func (s *service) partyIDs(ctx context.Context, campaignID string, v Viewer) (map[string]string, error) {
	party, err := s.dir.Party(ctx, campaignID, v.VisibilityRole, v.UserID)
	if err != nil {
		return nil, errInternal(err)
	}
	out := make(map[string]string, len(party))
	for _, m := range party {
		out[m.ID] = m.Name
	}
	return out, nil
}

// Help is one extra roll on a party member's project by a hero of the
// viewer's, once for each project at each respite, when the recipe lets
// others help. The roll adds the helper's characteristic and is a plain
// roll; the points and the made item are the owner's, and neither hero's
// ready rolls change.
func (s *service) Help(ctx context.Context, campaignID, projectID string, v Viewer, in HelpInput) (*RollResult, error) {
	if err := s.projectsReady(); err != nil {
		return nil, err
	}
	helper, err := s.hero(ctx, campaignID, in.CharacterID, v)
	if err != nil {
		return nil, err
	}
	p, ok, err := s.projects.Store.GetProject(ctx, campaignID, strings.TrimSpace(projectID))
	if err != nil {
		return nil, errInternal(err)
	}
	party, err := s.partyIDs(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	if !ok || party[p.CharacterID] == "" {
		return nil, errNotFound("project not found")
	}
	if p.CharacterID == helper.ID {
		return nil, errInvalid("this is " + helper.Name + "’s own project; roll it from In progress")
	}
	if p.Status != ProjectActive {
		return nil, errConflictMsg("this project is already " + map[string]string{ProjectMade: "made", ProjectCancelled: "cancelled"}[p.Status])
	}
	if in.Version != p.Version {
		return nil, errConflict()
	}
	r, found, err := s.repo.GetRecipe(ctx, campaignID, p.RecipeID)
	if err != nil {
		return nil, errInternal(err)
	}
	if p.RecipeID == "" || !found || !r.Extras.OthersCanHelp {
		return nil, errInvalid("this recipe doesn’t let others help")
	}
	owner, err := s.dir.Entity(ctx, campaignID, p.CharacterID)
	if err != nil {
		return nil, errInternal(err)
	}
	if owner == nil {
		return nil, errNotFound("project not found")
	}
	rid, err := s.projects.Store.RespiteID(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	if ok, err := s.projects.Store.TakeTurn(ctx, campaignID, helper.ID, TurnHelp, p.ID, rid); err != nil {
		return nil, errInternal(err)
	} else if !ok {
		return nil, errConflictMsg(helper.Name + " has already helped with this project since the last respite.")
	}
	benchID, err := s.benchFor(ctx, campaignID, r.Bench)
	if err != nil {
		return nil, err
	}
	res, err := s.rollOn(ctx, campaignID, p, owner, helper, RollInput{Work: WorkSteady, Bench: benchID, Version: in.Version}, v)
	if err != nil {
		if rerr := s.projects.Store.ReturnTurn(ctx, campaignID, helper.ID, TurnHelp, p.ID, rid); rerr != nil {
			slog.Error("crafting: a failed help's turn was not given back", slog.String("character_id", helper.ID), slog.Any("error", rerr))
		}
		return nil, err
	}
	if len(res.Notes) > 0 && res.Notes[0] == workNotes[WorkSteady] {
		res.Notes = res.Notes[1:]
	}
	res.Notes = append([]string{helper.Name + " lent a hand: one extra roll, a plain one, for " + owner.Name + "’s project."}, res.Notes...)
	return res, nil
}

// benchFor picks the bench a hand works at: the campaign's first bench of
// the recipe's kind, the one the block shows the owner, so its upgrades
// count for the helper's roll too. No bench of that kind is a plain roll.
func (s *service) benchFor(ctx context.Context, campaignID, kind string) (string, error) {
	if kind == "" {
		return "", nil
	}
	benches, err := s.repo.ListBenches(ctx, campaignID)
	if err != nil {
		return "", errInternal(err)
	}
	for _, b := range benches {
		if b.Kind == kind {
			return b.ID, nil
		}
	}
	return "", nil
}

// turnsFor reads which turns a hero has spent since the last respite.
func (s *service) turnsFor(ctx context.Context, campaignID, characterID string) (map[string]bool, error) {
	rid, err := s.projects.Store.RespiteID(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	t, err := s.projects.Store.TurnsTaken(ctx, campaignID, characterID, rid)
	if err != nil {
		return nil, errInternal(err)
	}
	return t, nil
}

// helpable lists the party's projects in progress a hero may help with:
// another party member's, of a recipe that lets others help.
func (s *service) helpable(ctx context.Context, campaignID string, e *EntityInfo, recipes map[string]*Recipe, turns map[string]bool, day *Day, names map[string]string, v Viewer) ([]HelpView, error) {
	out := []HelpView{}
	party, err := s.partyIDs(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	active, err := s.projects.Store.ListProjects(ctx, campaignID, "", ProjectActive, 0)
	if err != nil {
		return nil, errInternal(err)
	}
	rules, _ := s.systemRules(ctx, campaignID)
	for _, p := range active {
		r := recipes[p.RecipeID]
		if p.CharacterID == e.ID || party[p.CharacterID] == "" || r == nil || !r.Extras.OthersCanHelp {
			continue
		}
		pv := s.view(p, party[p.CharacterID], r, day, rules, names, v.Director)
		out = append(out, HelpView{Project: pv, Helped: turns[TurnHelp+":"+p.ID]})
	}
	return out, nil
}
