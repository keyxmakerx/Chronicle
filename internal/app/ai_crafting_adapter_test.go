package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// craftingFake answers the reads the adapter makes and records the writes.
type craftingFake struct {
	crafting.Service
	recipes    []crafting.Recipe
	spoilsErr  error
	catalogHit bool

	saved   crafting.AIRecipe
	check   bool
	drops   crafting.DropInput
	dropsBy string
	tags    []string
	as      crafting.Viewer
}

func (f *craftingFake) Library(_ context.Context, _ string, v crafting.Viewer) (*crafting.Library, error) {
	rules := systems.DefaultCraftingRules()
	rules.Characteristics = []systems.CraftingStat{{Key: "might", Label: "Might"}}
	rules.Labels.Goal = "Project goal"
	return &crafting.Library{Tags: crafting.Tags, Templates: crafting.Templates, Rules: rules, System: "Draw Steel",
		Settings: crafting.DefaultSettings(), Director: v.Director}, nil
}

func (f *craftingFake) Book(context.Context, string, crafting.Viewer) ([]crafting.BookPage, error) {
	return []crafting.BookPage{{Name: "Healing potion", Kind: "Steady brew", Makes: "Potion", MakesQty: 1, Goal: 45, Roll: "2d10 + Might",
		Needs: []crafting.BookMaterial{{Qty: 2, Name: "Troll tooth"}}, Cost: &crafting.Cost{Amount: 25, Currency: "gp"}}}, nil
}

func (f *craftingFake) Catalog(context.Context, string, crafting.Viewer) (*crafting.Catalog, error) {
	f.catalogHit = true
	return &crafting.Catalog{Items: []crafting.MaterialItem{{ID: "moss", Name: "Moss", Tags: []string{"herb"}}, {ID: "water", Name: "Water", Tags: []string{"liquid"}}},
		Characters: []crafting.Named{{ID: "mira", Name: "Mira"}}, Truncated: true}, nil
}

func (f *craftingFake) ListRecipes(context.Context, string, crafting.Viewer) ([]crafting.Recipe, error) {
	return f.recipes, nil
}

func (f *craftingFake) Spoils(context.Context, string, crafting.Viewer) (*crafting.SpoilsPage, error) {
	if f.spoilsErr != nil {
		return nil, f.spoilsErr
	}
	return &crafting.SpoilsPage{
		Drops: []crafting.DropView{{PageID: "troll", Page: "Troll", Version: 2, Lines: []crafting.DropLine{
			{ItemID: "moss", Mode: crafting.DropTake, Qty: 2},
			{ItemID: "water", Mode: crafting.DropHarvest, Stat: "might", Yields: []int{1, 2, 4}}}}},
		Tray: []crafting.SpoilView{{Source: "Troll", Copy: 1, Copies: 2, Lines: []crafting.SpoilLineView{
			{Name: "Moss", Mode: crafting.DropTake, Qty: 2},
			{Name: "Water", Mode: crafting.DropHarvest, Roll: "2d10 + Might", Tiers: []crafting.Tier{{Label: "11 or less", Qty: 1}, {Label: "12 to 16", Qty: 2}, {Label: "17 or more", Qty: 4}}}}}},
	}, nil
}

func (f *craftingFake) Projects(context.Context, string, crafting.Viewer) (*crafting.ProjectsPage, error) {
	return &crafting.ProjectsPage{Active: []crafting.ProjectView{{Hero: "Mira", Name: "Healing potion", Points: 10, Goal: 45, RollsReady: 1}}}, nil
}

func (f *craftingFake) ListBenches(context.Context, string, crafting.Viewer) ([]crafting.Bench, error) {
	return []crafting.Bench{{Name: "Mira's bench", Kind: "apothecary"}}, nil
}

func (f *craftingFake) SaveAIRecipe(_ context.Context, _ string, v crafting.Viewer, in crafting.AIRecipe, check bool) (*crafting.Recipe, error) {
	f.saved, f.check, f.as = in, check, v
	return nil, nil
}

func (f *craftingFake) CheckDrops(_ context.Context, _, pageID string, v crafting.Viewer, in crafting.DropInput) error {
	f.drops, f.dropsBy, f.as = in, "check", v
	return nil
}

func (f *craftingFake) SaveDrops(_ context.Context, _, pageID string, v crafting.Viewer, in crafting.DropInput) (*crafting.DropView, error) {
	f.drops, f.dropsBy, f.as = in, "save", v
	return nil, nil
}

func (f *craftingFake) SetTags(_ context.Context, _, _ string, v crafting.Viewer, tags []string) ([]string, error) {
	f.tags, f.as = tags, v
	return tags, nil
}

var (
	craftOwner  = records.Actor{UserID: "owner", Role: 3}
	craftPlayer = records.Actor{UserID: "pat", Role: 1}
)

// A player reads the rules and the recipe book, never the Director's
// catalog, recipes or tray.
func TestAICraftingAdapterReadAsAPlayer(t *testing.T) {
	f := &craftingFake{}
	a := aiCraftingAdapter{svc: f, addons: fakeAddonSwitch{on: map[string]bool{}}}
	c, err := a.Read(context.Background(), "camp", craftPlayer)
	if err != nil {
		t.Fatal(err)
	}
	if c.Director || f.catalogHit || len(c.Items) > 0 || len(c.Recipes) > 0 {
		t.Errorf("a player got the Director's parts: %+v", c)
	}
	if !c.Off {
		t.Error("the add-on is off, so Off must be set")
	}
	if c.Words.Goal != "Project goal" || c.Words.Source != "Project source" || c.Dice != "2d10" {
		t.Errorf("rules %+v", c)
	}
	if !reflect.DeepEqual(c.Tiers, []string{"11 or less", "12 to 16", "17 or more"}) {
		t.Errorf("tiers %v", c.Tiers)
	}
	want := records.CraftBookPage{Name: "Healing potion", Kind: "Steady brew", Makes: "Potion", MakesQty: 1, Goal: 45, Roll: "2d10 + Might",
		Needs: []string{"2 × Troll tooth"}, Cost: "25 gp"}
	if len(c.Book) != 1 || !reflect.DeepEqual(c.Book[0], want) {
		t.Errorf("book %+v", c.Book)
	}
}

func TestAICraftingAdapterReadAsTheDirector(t *testing.T) {
	tags := map[string][]string{"moss": {"herb"}, "water": {"liquid"}}
	mats := []crafting.Material{{ItemID: "moss", Qty: 1}, {ItemID: "water", Qty: 1}}
	laid, ok := crafting.LayTemplate(crafting.RecipeInput{Materials: mats}, "steady", tags)
	if !ok {
		t.Fatal("no steady method")
	}
	asLaid := crafting.Recipe{ID: "r1", Name: "Tonic", Template: "steady", Bench: laid.Bench, Materials: laid.Materials, Steps: laid.Steps,
		Rules: []crafting.Rule{{}}, Result: crafting.Result{ItemID: "moss", Qty: 2}, Cost: crafting.Cost{Amount: 5, Currency: "gp"}, Draft: true}
	changed := asLaid
	changed.ID, changed.Name = "r2", "Strong tonic"
	changed.Steps = append([]crafting.Step(nil), laid.Steps...)
	changed.Steps[1].Turns = 5

	f := &craftingFake{recipes: []crafting.Recipe{asLaid, changed}, spoilsErr: apperror.NewNotFound("crafting spoils are not set up on this server")}
	a := aiCraftingAdapter{svc: f, addons: fakeAddonSwitch{on: map[string]bool{crafting.AddonSlug: true}}}
	c, err := a.Read(context.Background(), "camp", craftOwner)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Director || c.Off || !c.ItemsCut || len(c.Items) != 2 || len(c.Characters) != 1 {
		t.Fatalf("director read %+v", c)
	}
	if len(c.Recipes) != 2 {
		t.Fatalf("recipes %+v", c.Recipes)
	}
	r := c.Recipes[0]
	if r.ByHand || r.Method != "steady" || r.Steps != len(laid.Steps) || r.Rules != 1 || !r.Draft || r.Cost != 5 || r.Currency != "gp" || r.ResultQty != 2 {
		t.Errorf("as laid %+v", r)
	}
	if !c.Recipes[1].ByHand {
		t.Error("a step changed in the editor must read as by hand")
	}
	if !c.DropsOff || len(c.Drops) != 0 {
		t.Error("spoils not set up must read as DropsOff, not fail the read")
	}
	if len(c.Projects) != 1 || c.Projects[0].Rolls != 1 {
		t.Errorf("projects %+v", c.Projects)
	}
	if len(c.Benches) != 1 || c.Benches[0].Kind != "Apothecary bench" {
		t.Errorf("benches %+v", c.Benches)
	}
	if len(c.Methods) != len(crafting.Templates) || c.Methods[0].Label != "Steady brew" {
		t.Errorf("methods %+v", c.Methods)
	}
}

func TestAICraftingAdapterReadsDropsAndTheTray(t *testing.T) {
	f := &craftingFake{}
	c, err := aiCraftingAdapter{svc: f}.Read(context.Background(), "camp", craftOwner)
	if err != nil {
		t.Fatal(err)
	}
	if c.DropsOff || len(c.Drops) != 1 {
		t.Fatalf("drops %+v", c.Drops)
	}
	d := c.Drops[0]
	wantLines := []records.CraftDropLine{{ItemID: "moss", Qty: 2}, {ItemID: "water", Harvest: true, Stat: "might", Yields: [3]int{1, 2, 4}}}
	if d.Version != 2 || !reflect.DeepEqual(d.Lines, wantLines) {
		t.Errorf("drop list %+v", d)
	}
	wantTray := []string{"2 Moss", "harvest Water, 2d10 + Might: 11 or less 1, 12 to 16 2, 17 or more 4"}
	if len(c.Tray) != 1 || !reflect.DeepEqual(c.Tray[0].Lines, wantTray) || c.Tray[0].Copies != 2 {
		t.Errorf("tray %+v", c.Tray)
	}
}

func TestAICraftingAdapterWrites(t *testing.T) {
	f := &craftingFake{}
	a := aiCraftingAdapter{svc: f}
	ctx := context.Background()

	in := records.CraftRecipeInput{ID: "r1", Name: "Tonic", Method: "steady", Materials: []records.CraftMaterial{{ItemID: "moss", Qty: 2}, {Tag: "liquid", Qty: 1}},
		ResultID: "potion", ResultQty: 1, Goal: 45, Stat: "might", Cost: 5, Currency: "gp", Who: "learned", Known: []string{"mira"}}
	if err := a.SaveRecipe(ctx, "camp", craftOwner, in, true); err != nil {
		t.Fatal(err)
	}
	want := crafting.AIRecipe{ID: "r1", Name: "Tonic", Method: "steady", Materials: []crafting.Material{{ItemID: "moss", Qty: 2}, {Tag: "liquid", Qty: 1}},
		Result: crafting.Result{ItemID: "potion", Qty: 1}, Goal: 45, Stat: "might", Cost: crafting.Cost{Amount: 5, Currency: "gp"}, Who: "learned", Known: []string{"mira"}}
	if !reflect.DeepEqual(f.saved, want) || !f.check {
		t.Errorf("recipe sent %+v check=%v", f.saved, f.check)
	}
	if !f.as.Director || f.as.UserID != "owner" {
		t.Errorf("acted as %+v, want the owner as Director", f.as)
	}

	drops := records.CraftDropsInput{AlsoID: "wolf", Version: 2, Lines: []records.CraftDropLine{{ItemID: "moss", Qty: 2}, {ItemID: "water", Harvest: true, Stat: "might", Yields: [3]int{1, 2, 4}}}}
	for _, check := range []bool{true, false} {
		if err := a.SaveDrops(ctx, "camp", "troll", craftOwner, drops, check); err != nil {
			t.Fatal(err)
		}
		wantBy := map[bool]string{true: "check", false: "save"}[check]
		wantIn := crafting.DropInput{Also: "wolf", Version: 2, Lines: []crafting.DropLine{
			{ItemID: "moss", Mode: crafting.DropTake, Qty: 2},
			{ItemID: "water", Mode: crafting.DropHarvest, Stat: "might", Yields: []int{1, 2, 4}}}}
		if f.dropsBy != wantBy || !reflect.DeepEqual(f.drops, wantIn) {
			t.Errorf("check=%v: %s %+v", check, f.dropsBy, f.drops)
		}
	}

	if err := a.SetTags(ctx, "camp", "moss", craftPlayer, nil); err != nil {
		t.Fatal(err)
	}
	if f.tags == nil || len(f.tags) != 0 || f.as.Director {
		t.Errorf("no tags must clear them as the player who asked: %v %+v", f.tags, f.as)
	}
}
