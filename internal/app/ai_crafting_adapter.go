package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// aiCraftingAdapter implements records.CraftingAPI over the crafting
// service, which keeps every check: Directors only for everything but the
// recipe book, and the same rules the Crafting app's editors save through.
type aiCraftingAdapter struct {
	svc    crafting.Service
	addons aiAddonChecker // nil reads crafting as on
}

var _ records.CraftingAPI = aiCraftingAdapter{}

// Crafting's material kinds, recipes and drop lists sit before the item
// looks, which a recipe's result can carry.
func init() {
	addAIKinds(190, func(d *aiDeps) []records.Kind {
		if d.Crafting == nil {
			return nil
		}
		return []records.Kind{
			records.MaterialKind{Svc: d.Crafting},
			records.RecipeKind{Svc: d.Crafting},
			records.DropsKind{Svc: d.Crafting, Entities: d.Entities},
		}
	})
}

// craftViewer is the operator as the crafting service sees them. The
// service reads only the visibility role and the Director flag.
func craftViewer(a records.Actor) crafting.Viewer {
	return crafting.Viewer{UserID: a.UserID, MemberRole: a.Role, VisibilityRole: a.Role, Director: a.CanAuthorDmOnly()}
}

func (c aiCraftingAdapter) Read(ctx context.Context, campaignID string, a records.Actor) (*records.Crafting, error) {
	v := craftViewer(a)
	lib, err := c.svc.Library(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	out := craftingRules(lib)
	if c.addons != nil {
		on, err := c.addons.IsEnabledForCampaign(ctx, campaignID, crafting.AddonSlug)
		out.Off = err == nil && !on
	}
	if !lib.Director {
		book, err := c.svc.Book(ctx, campaignID, v)
		if err != nil {
			return nil, err
		}
		for _, p := range book {
			out.Book = append(out.Book, bookPage(p))
		}
		return out, nil
	}

	cat, err := c.svc.Catalog(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	out.ItemsCut = cat.Truncated
	tags := make(map[string][]string, len(cat.Items))
	for _, it := range cat.Items {
		out.Items = append(out.Items, records.CraftItem{ID: it.ID, Name: it.Name, Tags: it.Tags})
		tags[it.ID] = it.Tags
	}
	for _, ch := range cat.Characters {
		out.Characters = append(out.Characters, records.CraftNamed{ID: ch.ID, Name: ch.Name})
	}

	recipes, err := c.svc.ListRecipes(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	for i := range recipes {
		out.Recipes = append(out.Recipes, craftRecipe(&recipes[i], tags))
	}

	spoils, err := c.svc.Spoils(ctx, campaignID, v)
	switch {
	case notSetUp(err):
		out.DropsOff = true
	case err != nil:
		return nil, err
	default:
		for _, d := range spoils.Drops {
			out.Drops = append(out.Drops, craftDrops(d))
		}
		for _, s := range spoils.Tray {
			out.Tray = append(out.Tray, craftSpoil(s))
		}
	}

	projects, err := c.svc.Projects(ctx, campaignID, v)
	switch {
	case notSetUp(err):
	case err != nil:
		return nil, err
	default:
		for _, p := range projects.Active {
			out.Projects = append(out.Projects, records.CraftProject{Hero: p.Hero, Name: p.Name, Makes: p.Makes, Status: p.Status,
				Waits: p.Waits, Points: p.Points, Goal: p.Goal, Rolls: p.RollsReady})
		}
	}

	benches, err := c.svc.ListBenches(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	for _, b := range benches {
		kind := b.Kind
		for _, k := range crafting.BenchKinds {
			if k.Key == b.Kind {
				kind = k.Name
			}
		}
		out.Benches = append(out.Benches, records.CraftBench{Name: b.Name, Kind: kind})
	}
	return out, nil
}

// notSetUp says a part of crafting has no tables on this server, which the
// lookup reports instead of failing the whole answer.
func notSetUp(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusNotFound
}

// craftingRules is what everyone may read: the system's rules, the fixed
// lists and the campaign's defaults.
func craftingRules(lib *crafting.Library) *records.Crafting {
	rules := lib.Rules
	def := systems.DefaultCraftingRules()
	words := records.CraftWords{Goal: rules.Labels.Goal, Prerequisite: rules.Labels.Prerequisite, Source: rules.Labels.Source}
	if words.Goal == "" {
		words.Goal = def.Labels.Goal
	}
	if words.Prerequisite == "" {
		words.Prerequisite = def.Labels.Prerequisite
	}
	if words.Source == "" {
		words.Source = def.Labels.Source
	}
	out := &records.Crafting{Director: lib.Director, System: lib.System, Dice: rules.Dice, Tiers: crafting.TierLabels(rules.Tiers), Words: words,
		Settings: records.CraftSettings{Look: lib.Settings.Skin, Difficulty: lib.Settings.Difficulty, Bane: lib.Settings.Bane, Skip: lib.Settings.Skip}}
	for _, s := range rules.Characteristics {
		out.Stats = append(out.Stats, records.CraftChoice{Key: s.Key, Label: s.Label})
	}
	for _, t := range lib.Tags {
		out.Tags = append(out.Tags, records.CraftChoice{Key: t.Key, Label: t.Label})
	}
	for _, t := range lib.Templates {
		out.Methods = append(out.Methods, records.CraftChoice{Key: t.Key, Label: t.Name})
	}
	return out
}

// craftRecipe is a stored recipe as the lookup shows it. ByHand says its
// steps are no longer what its method lays, so sending `method` again
// would undo changes made in the editor.
func craftRecipe(r *crafting.Recipe, tags map[string][]string) records.CraftRecipe {
	out := records.CraftRecipe{ID: r.ID, Name: r.Name, Draft: r.Draft, Method: r.Template, Steps: len(r.Steps), Rules: len(r.Rules),
		ResultID: r.Result.ItemID, ResultQty: r.Result.Qty, Goal: r.Goal, Stat: r.Stat, Prerequisite: r.Prerequisite, Source: r.Source,
		Cost: r.Cost.Amount, Currency: r.Cost.Currency, Who: r.Who, Known: r.Known}
	for _, m := range r.Materials {
		out.Materials = append(out.Materials, records.CraftMaterial{ItemID: m.ItemID, Tag: m.Tag, Qty: m.Qty})
	}
	if r.Template != "" {
		laid, ok := crafting.LayTemplate(crafting.RecipeInput{Materials: r.Materials}, r.Template, tags)
		out.ByHand = !ok || laid.Bench != r.Bench || !reflect.DeepEqual(laid.Steps, r.Steps) || len(laid.Materials) != len(r.Materials)
	}
	return out
}

func craftDrops(d crafting.DropView) records.CraftDrops {
	out := records.CraftDrops{PageID: d.PageID, Page: d.Page, Kind: d.Kind, AlsoID: d.Also, Also: d.AlsoName, Version: d.Version}
	for _, l := range d.Lines {
		line := records.CraftDropLine{ItemID: l.ItemID, Harvest: l.Mode == crafting.DropHarvest, Qty: l.Qty, Stat: l.Stat, Study: l.Study, StudyStat: l.StudyStat, Once: l.Once}
		copy(line.Yields[:], l.Yields)
		out.Lines = append(out.Lines, line)
	}
	return out
}

// craftSpoil writes a tray entry's waiting lines in words: "2 Troll tooth",
// "harvest Troll hide, 2d10 + Might: 11 or less 1, 12 to 16 2, 17 or more 4",
// and for a find nobody has identified "…, not identified yet (study 2d10 +
// Reason, one chance)".
func craftSpoil(s crafting.SpoilView) records.CraftSpoil {
	out := records.CraftSpoil{Source: s.Source, Copy: s.Copy, Copies: s.Copies, Origin: s.Origin}
	for _, l := range s.Lines {
		w := fmt.Sprintf("%d %s", l.Qty, l.Name)
		if l.Mode == crafting.DropHarvest {
			w = "harvest " + l.Name + ", " + l.Roll
			for i, t := range l.Tiers {
				sep := ", "
				if i == 0 {
					sep = ": "
				}
				w += fmt.Sprintf("%s%s %d", sep, t.Label, t.Qty)
			}
		}
		if l.Locked && l.Study != nil {
			once := ""
			if l.Study.Once {
				once = ", one chance"
			}
			w += ", not identified yet (study " + l.Study.Roll + once + ")"
		}
		out.Lines = append(out.Lines, w)
	}
	return out
}

func bookPage(p crafting.BookPage) records.CraftBookPage {
	out := records.CraftBookPage{Name: p.Name, Kind: p.Kind, Makes: p.Makes, MakesQty: p.MakesQty, Goal: p.Goal, Roll: p.Roll,
		Prerequisite: p.Prerequisite, Source: p.Source, Lines: p.Lines}
	// "1 × an item you haven't seen" reads better than "1 an item…".
	for _, m := range p.Needs {
		out.Needs = append(out.Needs, fmt.Sprintf("%d × %s", m.Qty, m.Name))
	}
	if p.Cost != nil && p.Cost.Amount > 0 {
		out.Cost = strconv.FormatFloat(p.Cost.Amount, 'f', -1, 64)
		if p.Cost.Currency != "" {
			out.Cost += " " + p.Cost.Currency
		}
	}
	return out
}

func (c aiCraftingAdapter) SaveRecipe(ctx context.Context, campaignID string, a records.Actor, in records.CraftRecipeInput, check bool) error {
	r := crafting.AIRecipe{ID: in.ID, Name: in.Name, Method: in.Method, Result: crafting.Result{ItemID: in.ResultID, Qty: in.ResultQty},
		Goal: in.Goal, Stat: in.Stat, Prerequisite: in.Prerequisite, Source: in.Source,
		Cost: crafting.Cost{Amount: in.Cost, Currency: in.Currency}, Who: in.Who, Known: in.Known}
	for _, m := range in.Materials {
		r.Materials = append(r.Materials, crafting.Material{ItemID: m.ItemID, Tag: m.Tag, Qty: m.Qty})
	}
	_, err := c.svc.SaveAIRecipe(ctx, campaignID, craftViewer(a), r, check)
	return err
}

func (c aiCraftingAdapter) DeleteRecipe(ctx context.Context, campaignID, id string, a records.Actor) error {
	return c.svc.DeleteRecipe(ctx, campaignID, id, craftViewer(a))
}

func (c aiCraftingAdapter) SetTags(ctx context.Context, campaignID, itemID string, a records.Actor, tags []string) error {
	if tags == nil {
		tags = []string{}
	}
	_, err := c.svc.SetTags(ctx, campaignID, itemID, craftViewer(a), tags)
	return err
}

func (c aiCraftingAdapter) SaveDrops(ctx context.Context, campaignID, pageID string, a records.Actor, in records.CraftDropsInput, check bool) error {
	d := crafting.DropInput{Also: in.AlsoID, Lines: []crafting.DropLine{}, Version: in.Version}
	for _, l := range in.Lines {
		line := crafting.DropLine{ItemID: l.ItemID, Mode: crafting.DropTake, Qty: l.Qty}
		if l.Harvest {
			line = crafting.DropLine{ItemID: l.ItemID, Mode: crafting.DropHarvest, Stat: l.Stat, Yields: []int{l.Yields[0], l.Yields[1], l.Yields[2]}}
		}
		line.Study, line.StudyStat, line.Once = l.Study, l.StudyStat, l.Once
		d.Lines = append(d.Lines, line)
	}
	v := craftViewer(a)
	if check {
		return c.svc.CheckDrops(ctx, campaignID, pageID, v, d)
	}
	_, err := c.svc.SaveDrops(ctx, campaignID, pageID, v, d)
	return err
}

func (c aiCraftingAdapter) DeleteDrops(ctx context.Context, campaignID, pageID string, a records.Actor) error {
	return c.svc.DeleteDrops(ctx, campaignID, pageID, craftViewer(a))
}
