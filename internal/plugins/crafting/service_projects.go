package crafting

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// ProjectDeps are what projects and spoils need beyond recipes: where they
// are kept, the Armory, the campaign's pages, the hero's sheet, the
// calendar, and who hears of a respite. Store and Armory are required for
// projects, Spoils, Armory and Pages for spoils; Sheets, Days and Respite
// may be nil (no characteristic is added, no calendar rule is on, no
// Homebrew rule runs). Roll and Now are for tests.
type ProjectDeps struct {
	Store  ProjectStore
	Armory Armory
	Spoils SpoilStore
	Pages  Pages
	Sheets Sheets
	Tables Tables
	Days   Days
	// Calendar names the default calendar as the viewer sees it, which
	// decides whether today's date may reach the block.
	Calendar CalendarSource
	Respite  RespiteHook
	Roll     func(n int) int
	Now      func() time.Time
}

func (s *service) projectsReady() error {
	if s.projects.Store == nil || s.projects.Armory == nil {
		return apperror.NewNotFound("crafting projects are not set up on this server")
	}
	return nil
}

func (s *service) roll(n int) int {
	if s.projects.Roll != nil {
		return s.projects.Roll(n)
	}
	return cryptoRoll(n)
}

func (s *service) now() time.Time {
	if s.projects.Now != nil {
		return s.projects.Now()
	}
	return time.Now().UTC()
}

// today reads the calendar; a calendar that can't be read leaves every
// calendar rule off rather than stopping the work.
func (s *service) today(ctx context.Context, campaignID string) *Day {
	if s.projects.Days == nil {
		return nil
	}
	d, err := s.projects.Days.Today(ctx, campaignID)
	if err != nil {
		slog.Warn("crafting: cannot read the calendar's today", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil
	}
	return d
}

func errConflictMsg(msg string) error { return apperror.NewConflict(msg) }

// hero loads a character the viewer may craft for: a Director for anyone,
// a player for a character they have claimed. Anyone else is told there is
// no such character, so another player's hero is never confirmed.
func (s *service) hero(ctx context.Context, campaignID, characterID string, v Viewer) (*EntityInfo, error) {
	e, err := s.dir.Entity(ctx, campaignID, strings.TrimSpace(characterID))
	if err != nil {
		return nil, errInternal(err)
	}
	if e == nil || !e.IsCharacter {
		return nil, errNotFound("character not found")
	}
	if v.Director {
		return e, nil
	}
	owned, err := s.dir.OwnedCharacterIDs(ctx, campaignID, v.UserID)
	if err != nil {
		return nil, errInternal(err)
	}
	if !owned[e.ID] {
		return nil, errNotFound("character not found")
	}
	return e, nil
}

// CanCraftFor reports whether the viewer may see a character's Crafting
// block, without saying why not.
func (s *service) CanCraftFor(ctx context.Context, campaignID, characterID string, v Viewer) (bool, error) {
	if s.projectsReady() != nil {
		return false, nil
	}
	_, err := s.hero(ctx, campaignID, characterID, v)
	if apperror.SafeCode(err) == 404 {
		return false, nil
	}
	return err == nil, err
}

// itemNames maps the items the viewer may see to their names.
func (s *service) itemNames(ctx context.Context, campaignID string, v Viewer) (map[string]string, error) {
	items, _, err := s.dir.ListItems(ctx, campaignID, v.VisibilityRole, v.UserID)
	if err != nil {
		return nil, errInternal(err)
	}
	names := make(map[string]string, len(items))
	for _, it := range items {
		names[it.ID] = it.Name
	}
	return names, nil
}

// knows reports whether a character may work a recipe: approved, and known
// to everyone or learned by them.
func (s *service) knows(ctx context.Context, campaignID string, r *Recipe, characterID string) (bool, error) {
	if r.Draft {
		return false, nil
	}
	if r.Who != "learned" {
		return true, nil
	}
	by, err := s.repo.KnownBy(ctx, campaignID, []string{characterID})
	if err != nil {
		return false, errInternal(err)
	}
	for _, id := range by[characterID] {
		if id == r.ID {
			return true, nil
		}
	}
	return false, nil
}

func makesText(qty int, itemID string, names map[string]string) string {
	n, ok := names[itemID]
	if !ok {
		n = "the item it makes"
	}
	return fmt.Sprintf("%d × %s", qty, n)
}

func statLabel(key string, rules systems.CraftingRules) string {
	for _, c := range rules.Characteristics {
		if c.Key == key {
			return c.Label
		}
	}
	return key
}

// view builds a project as the viewer sees it. recipe is nil when the
// recipe was deleted; its rules then no longer apply.
func (s *service) view(p Project, hero string, recipe *Recipe, day *Day, rules systems.CraftingRules, names map[string]string, director bool) ProjectView {
	pv := ProjectView{ID: p.ID, CharacterID: p.CharacterID, Hero: hero, RecipeID: p.RecipeID, Name: p.RecipeName,
		Roll: rollWords(p.Stat, rules), Goal: p.Goal, Points: p.Points, RollsReady: p.RollsReady, Status: p.Status,
		Makes: makesText(p.ResultQty, p.ResultItemID, names), Came: p.Details.Made, StartedAt: p.StartedAt, FinishedAt: p.FinishedAt, Version: p.Version}
	if director {
		pv.Took = p.Took
		pv.Undelivered = p.Status == ProjectMade && !p.Delivered
	}
	if recipe != nil {
		pv.Kind = templateName(recipe.Template)
		if p.Status == ProjectActive && p.RollsReady > 0 {
			pv.Waits = hold(day, recipe.Rules, nil, director)
		}
	}
	return pv
}

// recipesByID loads the campaign's recipes once for a list of projects.
func (s *service) recipesByID(ctx context.Context, campaignID string) (map[string]*Recipe, error) {
	all, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	out := make(map[string]*Recipe, len(all))
	for i := range all {
		out[all[i].ID] = &all[i]
	}
	return out, nil
}

// ---------- The character's block ----------

// Character is a hero's Crafting block: their pack, projects in progress,
// the recipes they know with what each takes, and the benches.
func (s *service) Character(ctx context.Context, campaignID, characterID string, v Viewer) (*CharacterCrafting, error) {
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
	tags, err := s.repo.ListTags(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	book, err := s.pictureBook(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	for i := range pack.Items {
		pack.Items[i].Tags = knownTags(tags[pack.Items[i].ItemID])
		pack.Items[i].Picture = book.item(pack.Items[i].ItemID, pack.Items[i].Tags)
	}
	names, err := s.itemNames(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	for _, it := range pack.Items {
		names[it.ItemID] = it.Name
	}
	recipes, err := s.recipesByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	rules, _ := s.systemRules(ctx, campaignID)
	day := s.today(ctx, campaignID)
	set, err := s.repo.GetSettings(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	out := &CharacterCrafting{Character: Named{ID: e.ID, Name: e.Name}, Pack: *pack, Projects: []ProjectView{}, Recipes: []CraftRecipe{},
		Benches: []BenchPlay{}, Settings: set, Edge: rules.Edge, Bane: rules.Bane, Director: v.Director}
	if s.projects.Calendar != nil {
		out.CalendarID = s.projects.Calendar.DefaultCalendarID(ctx, campaignID, v.VisibilityRole, v.UserID)
	}
	// The day is read with the Director's sight for the recipe's rules, so
	// its date reaches only a viewer who can see the calendar itself.
	if day != nil && out.CalendarID != "" {
		out.Today = day.Label
	}
	active, err := s.projects.Store.ListProjects(ctx, campaignID, e.ID, ProjectActive, 0)
	if err != nil {
		return nil, errInternal(err)
	}
	for _, p := range active {
		out.Projects = append(out.Projects, s.view(p, e.Name, recipes[p.RecipeID], day, rules, names, v.Director))
	}
	known, err := s.repo.KnownBy(ctx, campaignID, []string{e.ID})
	if err != nil {
		return nil, errInternal(err)
	}
	learned := map[string]bool{}
	for _, id := range known[e.ID] {
		learned[id] = true
	}
	stat := map[string]*int{}
	all, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	for i := range all {
		r := &all[i]
		if r.Draft || (r.Who == "learned" && !learned[r.ID]) {
			continue
		}
		cr := CraftRecipe{ID: r.ID, Name: r.Name, Template: r.Template, Kind: templateName(r.Template), Bench: r.Bench, Needs: []CraftNeed{},
			Steps: r.Steps, Colors: r.Colors, Goal: r.Goal, Stat: r.Stat, Roll: rollWords(r.Stat, rules), MakesQty: r.Result.Qty,
			Lines: bookPage(r, names, rules).Lines}
		for i, m := range r.Materials {
			n := CraftNeed{ItemID: m.ItemID, Tag: m.Tag, Qty: m.Qty, Name: materialName(m, names)}
			if m.ItemID != "" {
				n.Tags = knownTags(tags[m.ItemID])
				for _, sub := range r.Extras.Substitutes {
					if sub.For == i {
						n.Sub = &CraftSub{Tag: sub.Use, Name: materialName(Material{Tag: sub.Use}, names), Costs: sub.Costs}
					}
				}
				n.Picture = book.item(m.ItemID, n.Tags)
			} else {
				n.Picture = book.kind(m.Tag)
			}
			cr.Needs = append(cr.Needs, n)
		}
		if r.Result.ItemID != "" {
			cr.Makes = materialName(Material{ItemID: r.Result.ItemID}, names)
		}
		if r.Cost.Amount > 0 {
			c := r.Cost
			cr.Cost = &c
			cr.Charge = s.projects.Armory.Charge(pack, c)
		}
		if r.Stat != "" {
			if _, ok := stat[r.Stat]; !ok {
				stat[r.Stat] = s.sheetStat(ctx, campaignID, e.ID, r.Stat)
			}
			cr.StatValue = stat[r.Stat]
		}
		out.Recipes = append(out.Recipes, cr)
	}
	benches, err := s.repo.ListBenches(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	for _, b := range benches {
		out.Benches = append(out.Benches, BenchPlay{ID: b.ID, Name: b.Name, Kind: b.Kind, Desk: rawOrNil(b.Desk), Journal: rawOrNil(b.Journal)})
	}
	if s.spoilsReady() == nil {
		if out.Spoils, err = s.trayViews(ctx, campaignID, v, names, tags, rules); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func rawOrNil(r json.RawMessage) any {
	if len(r) == 0 || string(r) == "null" {
		return nil
	}
	return r
}

// sheetStat reads a characteristic, nil when it can't be read.
func (s *service) sheetStat(ctx context.Context, campaignID, characterID, key string) *int {
	if s.projects.Sheets == nil || key == "" {
		return nil
	}
	n, ok, err := s.projects.Sheets.Stat(ctx, campaignID, characterID, key)
	if err != nil {
		slog.Warn("crafting: cannot read a characteristic", slog.String("character_id", characterID), slog.Any("error", err))
		return nil
	}
	if !ok {
		return nil
	}
	return &n
}

// ---------- Starting ----------

// StartProject starts a recipe the hero knows: its materials and cost leave
// the hero's pack through the Armory, and the project waits for its first
// roll at the next respite.
func (s *service) StartProject(ctx context.Context, campaignID, characterID string, v Viewer, in StartInput) (*ProjectView, error) {
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
	if r.Goal < 1 {
		return nil, errInvalid("this recipe has no goal yet; ask the Director to set one")
	}
	made, err := s.dir.Entity(ctx, campaignID, r.Result.ItemID)
	if err != nil {
		return nil, errInternal(err)
	}
	if r.Result.ItemID == "" || made == nil || !made.IsItem {
		return nil, errInvalid("this recipe doesn't name an item it makes; ask the Director to pick one")
	}
	n, err := s.projects.Store.CountActive(ctx, campaignID, e.ID)
	if err != nil {
		return nil, errInternal(err)
	}
	if n >= maxActiveProjects {
		return nil, errInvalid(fmt.Sprintf("%s already has %d projects in progress; finish or cancel one first", e.Name, maxActiveProjects))
	}
	items, subs, err := s.materialsFor(ctx, campaignID, e, r, in.Picks, v)
	if err != nil {
		return nil, err
	}
	paid, err := s.projects.Armory.Spend(ctx, campaignID, v, e.ID, items, r.Cost, r.Name)
	if err != nil {
		return nil, err
	}
	var took []string
	for _, it := range items {
		took = append(took, fmt.Sprintf("%d × %s", it.Qty, it.Name))
	}
	if paid != "" {
		took = append(took, paid)
	}
	p := Project{ID: s.newID(), CampaignID: campaignID, CharacterID: e.ID, RecipeID: r.ID, RecipeName: r.Name, Goal: r.Goal, Stat: r.Stat,
		ResultItemID: r.Result.ItemID, ResultQty: r.Result.Qty, Status: ProjectActive, Took: clipText(strings.Join(took, ", "), maxTookLen),
		StartedBy: v.UserID, StartedAt: s.now(), Details: ProjectDetails{Subs: subs}}
	if err := s.projects.Store.CreateProject(ctx, p); err != nil {
		slog.Error("crafting: MATERIALS TAKEN BUT THE PROJECT WAS NOT SAVED",
			slog.String("campaign_id", campaignID), slog.String("character_id", e.ID), slog.String("recipe_id", r.ID),
			slog.String("took", p.Took), slog.Any("error", err))
		return nil, apperror.NewInternal(fmt.Errorf("crafting: the materials were taken but the project wasn't saved; tell your Director so they can give them back: %w", err))
	}
	p.Version = 1
	names := map[string]string{made.ID: made.Name}
	rules, _ := s.systemRules(ctx, campaignID)
	pv := s.view(p, e.Name, r, nil, rules, names, v.Director)
	pv.Took = p.Took
	return &pv, nil
}

// materialsFor turns a recipe's materials into Armory items: a named item
// as it is, an "any <tag>" slot filled with the item the player picked,
// which must carry the tag and be in the hero's pack. A named item the
// recipe lets a substitute stand in for is filled the same way when the
// player picks one, and the substitute's cost is returned for every roll
// ("bane" or "points"). Repeated items are added up by the Armory, which
// then checks the hero holds enough.
func (s *service) materialsFor(ctx context.Context, campaignID string, e *EntityInfo, r *Recipe, picks map[int]string, v Viewer) ([]PackItem, []string, error) {
	var (
		out  []PackItem
		subs []string
		pack *Pack
		tags map[string][]string
	)
	for i, m := range r.Materials {
		tag := m.Tag
		if m.ItemID != "" && strings.TrimSpace(picks[i]) != "" {
			for _, sub := range r.Extras.Substitutes {
				if sub.For == i {
					tag = sub.Use
					if sub.Costs != "none" {
						subs = append(subs, sub.Costs)
					}
				}
			}
		}
		if m.ItemID != "" && tag == "" {
			it, err := s.dir.Entity(ctx, campaignID, m.ItemID)
			if err != nil {
				return nil, nil, errInternal(err)
			}
			if it == nil || !it.IsItem {
				return nil, nil, errInvalid(r.Name + " asks for an item that was deleted; ask the Director to fix the recipe")
			}
			out = append(out, PackItem{ItemID: it.ID, Name: it.Name, Qty: m.Qty})
			continue
		}
		pick := strings.TrimSpace(picks[i])
		if pick == "" {
			return nil, nil, errInvalid("pick which " + tagLabel(tag) + " to use")
		}
		if pack == nil {
			var err error
			if pack, err = s.projects.Armory.Pack(ctx, campaignID, v, e.ID); err != nil {
				return nil, nil, err
			}
			if tags, err = s.repo.ListTags(ctx, campaignID); err != nil {
				return nil, nil, errInternal(err)
			}
		}
		var held *PackItem
		for j := range pack.Items {
			if pack.Items[j].ItemID == pick {
				held = &pack.Items[j]
			}
		}
		if held == nil || !hasTag(tags[pick], tag) {
			return nil, nil, errInvalid(fmt.Sprintf("pick a %s %s carries", tagLabel(tag), e.Name))
		}
		out = append(out, PackItem{ItemID: pick, Name: held.Name, Qty: m.Qty})
	}
	return out, subs, nil
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// ---------- Rolling ----------

// loadProject loads a project in progress the viewer may roll or cancel.
func (s *service) loadProject(ctx context.Context, campaignID, id string, v Viewer) (Project, *EntityInfo, error) {
	p, ok, err := s.projects.Store.GetProject(ctx, campaignID, strings.TrimSpace(id))
	if err != nil {
		return Project{}, nil, errInternal(err)
	}
	if !ok {
		return Project{}, nil, errNotFound("project not found")
	}
	e, err := s.hero(ctx, campaignID, p.CharacterID, v)
	if err != nil {
		if apperror.SafeCode(err) == 404 {
			return Project{}, nil, errNotFound("project not found")
		}
		return Project{}, nil, err
	}
	if p.Status != ProjectActive {
		return Project{}, nil, errConflictMsg("this project is already " + map[string]string{ProjectMade: "made", ProjectCancelled: "cancelled"}[p.Status])
	}
	return p, e, nil
}

var workNotes = map[string]string{
	WorkClean:   "Clean work earned the edge.",
	WorkBotched: "A botched step gave the bane.",
	WorkSteady:  "Steady work, so it was a plain roll.",
	WorkSkipped: "The steps were skipped, so it was a plain roll.",
}

// RollProject makes one of a project's ready rolls on the server: the
// system's dice, the characteristic from the hero's sheet, an edge or a
// bane from the bench and the recipe's rules, and the rules' points. When
// the points reach the goal the item goes into the hero's pack; points
// over the goal are dropped.
func (s *service) RollProject(ctx context.Context, campaignID, id string, v Viewer, in RollInput) (*RollResult, error) {
	if err := s.projectsReady(); err != nil {
		return nil, err
	}
	p, e, err := s.loadProject(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
	}
	if in.Version != p.Version {
		return nil, errConflict()
	}
	if _, ok := workNotes[in.Work]; !ok {
		return nil, errInvalid("say how the work went: clean, botched, steady or skipped")
	}
	if p.RollsReady < 1 {
		return nil, errConflictMsg("no roll is ready; the next comes with the next respite")
	}
	var recipe *Recipe
	if p.RecipeID != "" {
		if r, ok, err := s.repo.GetRecipe(ctx, campaignID, p.RecipeID); err != nil {
			return nil, errInternal(err)
		} else if ok {
			recipe = &r
		}
	}
	set, err := s.repo.GetSettings(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	// The bench plays only a recipe the hero still knows, so a recipe made a
	// draft or unlearned mid-project leaves a plain roll, as the block offers.
	benchable := false
	if recipe != nil && len(recipe.Steps) > 0 {
		if benchable, err = s.knows(ctx, campaignID, recipe, p.CharacterID); err != nil {
			return nil, err
		}
	}
	var x Extras
	if recipe != nil {
		x = recipe.Extras
	}
	mastered := s.mastered(ctx, campaignID, p, x)
	// A hero who has mastered a recipe that rewards mastery with skipping
	// may skip the bench steps whatever the Director asks.
	skipEarned := mastered && x.Mastery.Gives == "skip"
	if in.Work == WorkSkipped && !set.Skip && !v.Director && benchable && !skipEarned {
		return nil, errInvalid("your Director asks for the steps to be worked at the bench")
	}
	w := &Worked{}
	var benchRules []Rule
	if in.Bench != "" {
		if b, ok, err := s.repo.GetBench(ctx, campaignID, in.Bench); err != nil {
			return nil, errInternal(err)
		} else if ok {
			w.Bench = b.Name
			benchRules = b.Upgrades.Rules()
		}
	}
	w.Skills = s.skills(ctx, campaignID, e.ID)
	// The bench's upgrades work on every recipe made there, beside its own.
	var ruleList []Rule
	if recipe != nil {
		ruleList = append(ruleList, recipe.Rules...)
	}
	ruleList = append(ruleList, benchRules...)
	day := s.today(ctx, campaignID)
	if why := hold(day, ruleList, w, v.Director); why != "" {
		return nil, errConflictMsg(why + " The roll waits for another day.")
	}
	sys, _ := s.systemRules(ctx, campaignID)
	res := &RollResult{From: p.Points, Terms: []Term{}, Notes: []string{}, Dice: []int{}}
	fx := effects(day, ruleList, w, s.roll)
	for _, f := range fx {
		if f.rule.Then.Kind == "cannot" {
			res.Spoiled = true
			res.Terms = append(res.Terms, Term{Text: ruleWords(f.rule, v.Director)})
			res.Notes = append(res.Notes, "The recipe’s rule spoiled the work this time. The roll is used.")
		}
	}
	extra := 0
	if !res.Spoiled {
		extra = s.addUp(ctx, campaignID, e, p, in.Work, set, sys, fx, x, mastered, v, res)
		if in.Work == WorkSkipped && !benchable {
			res.Notes[0] = "There are no steps to work, so it was a plain roll."
		}
	}
	res.To = res.From + res.Total
	res.Finished = res.To >= p.Goal
	if res.Finished && res.To > p.Goal {
		over := res.To - p.Goal
		res.Notes = append(res.Notes, fmt.Sprintf("%d %s over the goal %s dropped.", over, plural(over, "point", "points"), map[bool]string{true: "is", false: "are"}[over == 1]))
		res.To = p.Goal
	}

	next := p
	next.Details.Made = append([]string(nil), p.Details.Made...)
	if becomes := s.befall(ctx, campaignID, &next, fx, x, in.Work, set, res, v); becomes != "" {
		next.ResultItemID = becomes
	}
	next.Points, next.RollsReady = res.To, p.RollsReady-1
	now := s.now()
	switch {
	case res.Finished:
		next.Status, next.RollsReady, next.FinishedAt = ProjectMade, 0, &now
	case res.Lost:
		next.Status, next.RollsReady, next.FinishedAt = ProjectCancelled, 0, &now
	}
	ok, err := s.projects.Store.UpdateProject(ctx, next, p.Version)
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errConflict()
	}
	next.Version = p.Version + 1
	terms, _ := json.Marshal(res.Terms)
	if err := s.projects.Store.AddRoll(ctx, campaignID, p.ID, v.UserID, in.Work, diceText(res.Dice), string(terms), res.Total, res.From, res.To); err != nil {
		slog.Error("crafting: a roll was made but not recorded", slog.String("project_id", p.ID), slog.Any("error", err))
	}
	s.discover(ctx, campaignID, e, fx, v, res)
	names, err := s.itemNames(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	if res.Finished {
		s.deliver(ctx, e, &next, extra, names, v, res)
	}
	res.Project = s.view(next, e.Name, recipe, day, sys, names, v.Director)
	return res, nil
}

// addUp rolls the dice and adds every term to res, returning how many more
// of the item a rule makes if this roll finishes the project.
func (s *service) addUp(ctx context.Context, campaignID string, e *EntityInfo, p Project, work string, set Settings, sys systems.CraftingRules, fx []fired, x Extras, mastered bool, v Viewer, res *RollResult) int {
	count, size, ok := systems.ParseDice(sys.Dice)
	if !ok {
		count, size = 2, 10
	}
	sum := 0
	for i := 0; i < count; i++ {
		d := s.roll(size)
		res.Dice = append(res.Dice, d)
		sum += d
	}
	res.Sides = size
	res.Terms = append(res.Terms, Term{Text: fmt.Sprintf("%dd%d", count, size), N: sum})
	// A breakthrough is the dice's top two totals, a natural 19 or 20 on
	// 2d10: roll them again and add it.
	if x.Breakthrough && sum >= count*size-1 {
		again := 0
		for i := 0; i < count; i++ {
			d := s.roll(size)
			res.Dice = append(res.Dice, d)
			again += d
		}
		res.Breakthrough = true
		res.Terms = append(res.Terms, Term{Text: fmt.Sprintf("A breakthrough: %dd%d again", count, size), N: again})
	}
	if p.Stat != "" {
		label := statLabel(p.Stat, sys)
		if n := s.sheetStat(ctx, campaignID, e.ID, p.Stat); n != nil {
			res.Terms = append(res.Terms, Term{Text: label + ", from " + e.Name + "’s sheet", N: *n})
		} else {
			res.Notes = append(res.Notes, label+" isn’t a number on "+e.Name+"’s sheet, so it added nothing.")
		}
	}
	var edges, banes []string
	note := workNotes[work]
	switch work {
	case WorkClean:
		edges = append(edges, "clean work at the bench")
	case WorkBotched:
		if set.Bane {
			banes = append(banes, "botched work")
		} else {
			note = "A step was botched, but banes are off, so it was a plain roll."
		}
	}
	extra := 0
	var points []Term
	if mastered {
		switch x.Mastery.Gives {
		case "edge":
			edges = append(edges, "mastery")
		case "points":
			points = append(points, Term{Text: "Mastery", N: 2})
		}
	}
	for _, c := range p.Details.Subs {
		switch c {
		case "bane":
			banes = append(banes, "a substitute ingredient")
		case "points":
			points = append(points, Term{Text: "A substitute ingredient", N: -2})
		}
	}
	for _, f := range fx {
		words := ruleWords(f.rule, v.Director)
		switch f.rule.Then.Kind {
		case "edge":
			edges = append(edges, words)
		case "bane":
			banes = append(banes, words)
		case "points":
			points = append(points, Term{Text: words, N: f.rule.Then.Amount})
		case "extra":
			extra += f.rule.Then.Amount
		case "say":
			if f.rule.Players != "secret" || v.Director {
				res.Notes = append(res.Notes, f.rule.Then.Text)
			}
		}
	}
	switch {
	case len(edges) > len(banes):
		res.Terms = append(res.Terms, Term{Text: "An edge: " + strings.Join(edges, "; "), N: sys.Edge})
	case len(banes) > len(edges):
		res.Terms = append(res.Terms, Term{Text: "A bane: " + strings.Join(banes, "; "), N: sys.Bane})
	case len(edges) > 0:
		res.Terms = append(res.Terms, Term{Text: "An edge and a bane cancel: " + strings.Join(append(edges, banes...), "; ")})
	}
	res.Terms = append(res.Terms, points...)
	res.Notes = append([]string{note}, res.Notes...)
	total := 0
	for _, t := range res.Terms {
		total += t.N
	}
	if total < 0 {
		total = 0
	}
	res.Total = total
	return extra
}

// deliver puts the made item in the hero's pack and records that it did. A
// failed give leaves the project made but undelivered, which the Director's
// tab shows so the item can be given by hand.
func (s *service) deliver(ctx context.Context, e *EntityInfo, p *Project, extra int, names map[string]string, v Viewer, res *RollResult) {
	if p.ResultItemID == "" {
		res.Notes = append(res.Notes, "The item this recipe makes was deleted, so nothing reached the pack. Tell your Director.")
		return
	}
	qty := p.ResultQty + extra
	if qty > maxQty {
		qty = maxQty
	}
	if err := s.projects.Armory.Give(ctx, p.CampaignID, v, e.ID, p.ResultItemID, qty, p.RecipeName); err != nil {
		slog.Error("crafting: PROJECT MADE BUT THE ITEM WAS NOT GIVEN",
			slog.String("project_id", p.ID), slog.String("character_id", e.ID), slog.Any("error", err))
		res.Notes = append(res.Notes, "The item couldn’t be put in "+e.Name+"’s pack. Your Director can give it from the Projects tab.")
		return
	}
	res.Made = makesText(qty, p.ResultItemID, names)
	done := *p
	done.Delivered = true
	if ok, err := s.projects.Store.UpdateProject(ctx, done, p.Version); err != nil || !ok {
		slog.Error("crafting: the item was given but the project was not marked delivered",
			slog.String("project_id", p.ID), slog.Any("error", err))
		return
	}
	p.Delivered, p.Version = true, p.Version+1
}

func diceText(d []int) string {
	parts := make([]string, len(d))
	for i, n := range d {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// ---------- Cancelling ----------

// CancelProject stops a project. What it took stays spent; the Director
// gives anything back by hand.
func (s *service) CancelProject(ctx context.Context, campaignID, id string, v Viewer) error {
	if err := s.projectsReady(); err != nil {
		return err
	}
	p, _, err := s.loadProject(ctx, campaignID, id, v)
	if err != nil {
		return err
	}
	now := s.now()
	next := p
	next.Status, next.RollsReady, next.FinishedAt = ProjectCancelled, 0, &now
	ok, err := s.projects.Store.UpdateProject(ctx, next, p.Version)
	if err != nil {
		return errInternal(err)
	}
	if !ok {
		return errConflict()
	}
	return nil
}

// ---------- The Director's Projects tab ----------

// Projects lists every project in progress and those finished lately.
func (s *service) Projects(ctx context.Context, campaignID string, v Viewer) (*ProjectsPage, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if err := s.projectsReady(); err != nil {
		return nil, err
	}
	active, err := s.projects.Store.ListProjects(ctx, campaignID, "", ProjectActive, 0)
	if err != nil {
		return nil, errInternal(err)
	}
	made, err := s.projects.Store.ListProjects(ctx, campaignID, "", ProjectMade, recentFinished)
	if err != nil {
		return nil, errInternal(err)
	}
	recipes, err := s.recipesByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	names, err := s.itemNames(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	sys, _ := s.systemRules(ctx, campaignID)
	day := s.today(ctx, campaignID)
	heroes := map[string]string{}
	heroName := func(id string) string {
		if n, ok := heroes[id]; ok {
			return n
		}
		n := "A deleted character"
		if e, err := s.dir.Entity(ctx, campaignID, id); err == nil && e != nil {
			n = e.Name
		}
		heroes[id] = n
		return n
	}
	out := &ProjectsPage{Active: []ProjectView{}, Finished: []ProjectView{}}
	for _, p := range active {
		out.Active = append(out.Active, s.view(p, heroName(p.CharacterID), recipes[p.RecipeID], day, sys, names, true))
	}
	for _, p := range made {
		out.Finished = append(out.Finished, s.view(p, heroName(p.CharacterID), recipes[p.RecipeID], day, sys, names, true))
	}
	if out.LastRespite, err = s.projects.Store.LastRespite(ctx, campaignID); err != nil {
		return nil, errInternal(err)
	}
	if day != nil {
		out.Today = day.Label
	}
	return out, nil
}

// LogRespite is the Crafting app's button: every project in progress gets
// one roll, and Homebrew rules set off by a respite run as from the drawer.
func (s *service) LogRespite(ctx context.Context, campaignID string, v Viewer) (*RespiteResult, error) {
	n, err := s.logRespite(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	if s.projects.Respite != nil {
		if err := s.projects.Respite.RespiteLogged(ctx, campaignID, v); err != nil {
			slog.Warn("crafting: the respite's Homebrew rules did not run", slog.String("campaign_id", campaignID), slog.Any("error", err))
		}
	}
	return &RespiteResult{Projects: n}, nil
}

// RespitePressed records a respite logged from the Homebrew drawer, whose
// own rules are already running.
func (s *service) RespitePressed(ctx context.Context, campaignID string, v Viewer) error {
	_, err := s.logRespite(ctx, campaignID, v)
	return err
}

func (s *service) logRespite(ctx context.Context, campaignID string, v Viewer) (int, error) {
	if err := directorOnly(v); err != nil {
		return 0, err
	}
	if err := s.projectsReady(); err != nil {
		return 0, err
	}
	label := ""
	if d := s.today(ctx, campaignID); d != nil {
		label = d.Label
	}
	n, err := s.projects.Store.LogRespite(ctx, campaignID, v.UserID, label)
	if err != nil {
		return 0, errInternal(err)
	}
	return n, nil
}

// directorProject loads any project in progress for a Director.
func (s *service) directorProject(ctx context.Context, campaignID, id string, v Viewer) (Project, error) {
	if err := directorOnly(v); err != nil {
		return Project{}, err
	}
	if err := s.projectsReady(); err != nil {
		return Project{}, err
	}
	p, _, err := s.loadProject(ctx, campaignID, id, v)
	return p, err
}

// GiveRoll gives one project an extra roll.
func (s *service) GiveRoll(ctx context.Context, campaignID, id string, v Viewer) (*ProjectView, error) {
	p, err := s.directorProject(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
	}
	if p.RollsReady >= maxRollsReady {
		return nil, errInvalid(fmt.Sprintf("a project can bank at most %d rolls", maxRollsReady))
	}
	next := p
	next.RollsReady++
	return s.saveDirector(ctx, campaignID, p, next, v)
}

// SetPoints sets a project's points below its goal; the item only ever
// comes from a roll.
func (s *service) SetPoints(ctx context.Context, campaignID, id string, v Viewer, in SetPointsInput) (*ProjectView, error) {
	p, err := s.directorProject(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
	}
	if in.Version != p.Version {
		return nil, errConflict()
	}
	if in.Points < 0 || in.Points >= p.Goal {
		return nil, errInvalid(fmt.Sprintf("points are a whole number from 0 to %d; the item comes from a roll", p.Goal-1))
	}
	next := p
	next.Points = in.Points
	return s.saveDirector(ctx, campaignID, p, next, v)
}

func (s *service) saveDirector(ctx context.Context, campaignID string, p, next Project, v Viewer) (*ProjectView, error) {
	ok, err := s.projects.Store.UpdateProject(ctx, next, p.Version)
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errConflict()
	}
	next.Version = p.Version + 1
	var recipe *Recipe
	if r, ok, err := s.repo.GetRecipe(ctx, campaignID, p.RecipeID); err == nil && ok {
		recipe = &r
	}
	names, err := s.itemNames(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	hero := "A deleted character"
	if e, err := s.dir.Entity(ctx, campaignID, p.CharacterID); err == nil && e != nil {
		hero = e.Name
	}
	sys, _ := s.systemRules(ctx, campaignID)
	pv := s.view(next, hero, recipe, s.today(ctx, campaignID), sys, names, true)
	return &pv, nil
}
