package records

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"

	"gopkg.in/yaml.v3"
)

// fakeCrafting is a campaign with a few materials, one recipe, the troll's
// drop list and a player's book. refuse makes every check fail with its
// words, as the service would.
type fakeCrafting struct {
	c       Crafting
	book    []CraftBookPage
	refuse  string
	checked int
	saved   []CraftRecipeInput
	removed []string
	tags    map[string][]string
	drops   map[string]CraftDropsInput
	dropped []string
}

func (f *fakeCrafting) Read(_ context.Context, _ string, a Actor) (*Crafting, error) {
	c := f.c
	if !a.CanAuthorDmOnly() {
		return &Crafting{Dice: c.Dice, Stats: c.Stats, Tiers: c.Tiers, Words: c.Words, Tags: c.Tags, Methods: c.Methods, Book: f.book}, nil
	}
	c.Director = true
	return &c, nil
}
func (f *fakeCrafting) check(save func()) error {
	f.checked++
	if f.refuse != "" {
		return apperror.NewBadRequest(f.refuse)
	}
	if save != nil {
		save()
	}
	return nil
}
func (f *fakeCrafting) SaveRecipe(_ context.Context, _ string, _ Actor, in CraftRecipeInput, check bool) error {
	if check {
		return f.check(nil)
	}
	return f.check(func() { f.saved = append(f.saved, in) })
}
func (f *fakeCrafting) DeleteRecipe(_ context.Context, _, id string, _ Actor) error {
	f.removed = append(f.removed, id)
	return nil
}
func (f *fakeCrafting) SetTags(_ context.Context, _, id string, _ Actor, tags []string) error {
	f.tags[id] = tags
	return nil
}
func (f *fakeCrafting) SaveDrops(_ context.Context, _, pageID string, _ Actor, in CraftDropsInput, check bool) error {
	if check {
		return f.check(nil)
	}
	return f.check(func() { f.drops[pageID] = in })
}
func (f *fakeCrafting) DeleteDrops(_ context.Context, _, pageID string, _ Actor) error {
	f.dropped = append(f.dropped, pageID)
	return nil
}

func newFakeCrafting() *fakeCrafting {
	return &fakeCrafting{
		c: Crafting{
			System: "Draw Steel", Dice: "2d10",
			Stats:    []CraftChoice{{"might", "Might"}, {"reason", "Reason"}},
			Tiers:    []string{"11 or less", "12 to 16", "17 or more"},
			Words:    CraftWords{Goal: "Project goal", Prerequisite: "Item prerequisite", Source: "Project source"},
			Tags:     []CraftChoice{{"herb", "Herb"}, {"bone", "Bone"}, {"remnant_part", "Remnant part"}, {"liquid", "Liquid"}, {"hide", "Hide"}},
			Methods:  []CraftChoice{{"steady", "Steady brew"}, {"volatile", "Volatile brew"}},
			Settings: CraftSettings{Look: "journal", Difficulty: "std", Bane: true, Skip: false},
			Items: []CraftItem{
				{ID: "tooth", Name: "Troll tooth", Tags: []string{"bone", "remnant_part"}},
				{ID: "hide", Name: "Troll hide"},
				{ID: "moss", Name: "Bloodmoss", Tags: []string{"herb"}},
				{ID: "potion", Name: "Healing potion"},
				{ID: "twin1", Name: "Twin blade"},
				{ID: "twin2", Name: "Twin blade"},
			},
			Characters: []CraftNamed{{ID: "mira", Name: "Mira"}, {ID: "tobin", Name: "Tobin"}},
			Recipes: []CraftRecipe{{ID: "r1", Name: "Healing potion", Draft: true, Method: "steady", Steps: 5, Rules: 2,
				Materials: []CraftMaterial{{ItemID: "moss", Qty: 2}, {Tag: "liquid", Qty: 1}}, ResultID: "potion", ResultQty: 1,
				Goal: 30, Stat: "reason", Cost: 25, Currency: "gp", Who: "all"}},
			Drops: []CraftDrops{{PageID: "troll", Page: "Troll", Kind: "Creature", Version: 3, Lines: []CraftDropLine{
				{ItemID: "tooth", Qty: 2}, {ItemID: "hide", Harvest: true, Stat: "might", Yields: [3]int{1, 2, 4}}}}},
			Tray:     []CraftSpoil{{Source: "Troll", Copy: 1, Copies: 2, Origin: "Ramparts fight", Lines: []string{"2 × Troll tooth", "Troll hide, to harvest"}}},
			Projects: []CraftProject{{Hero: "Mira", Name: "Healing potion", Points: 12, Goal: 30, Rolls: 1, Status: "active"}},
			Benches:  []CraftBench{{Name: "Mira's kitchen", Kind: "Apothecary bench"}},
		},
		book:  []CraftBookPage{{Name: "Healing potion", Kind: "Steady brew", Makes: "Healing potion", MakesQty: 1, Needs: []string{"2 Bloodmoss", "1 any liquid"}, Goal: 30, Roll: "2d10 + Reason"}},
		tags:  map[string][]string{},
		drops: map[string]CraftDropsInput{},
	}
}

func craftRec(kind, action, name string, fields map[string]any) Record {
	if fields == nil {
		fields = map[string]any{}
	}
	return Record{Kind: kind, Action: action, Name: name, Fields: fields}
}

func craftPages() fakeEntities {
	pages := fakeEntities{}
	for id, name := range map[string]string{"troll": "Troll", "remnant": "Troll Remnant", "wolf": "Dire wolf"} {
		e := page(id, name, "Creature", false)
		pages[e.Slug] = &e
	}
	return pages
}

func TestMaterialKind_Plan(t *testing.T) {
	made := craftRec("material", ActionCreate, "", map[string]any{"item": "Frost lily", "kinds": []any{"herb"}})
	made.Made = map[string]bool{"frost-lily": true}
	tests := []struct {
		name    string
		r       Record
		a       Actor
		summary string
		err     string
	}{
		{"a new material, by the kinds' words", craftRec("material", ActionCreate, "", map[string]any{"item": "Troll hide", "kinds": []any{"Hide", "remnant part"}}), owner, "Hide, Remnant part", ""},
		{"tags is read too, and a repeat is dropped", craftRec("material", ActionCreate, "", map[string]any{"item": "troll hide", "tags": []any{"hide", "hide"}}), owner, "Hide", ""},
		{"update replaces the kinds", craftRec("material", ActionUpdate, "", map[string]any{"item": "Troll tooth", "kinds": "bone"}), owner, "now Bone", ""},
		{"remove names what uses it", craftRec("material", ActionDelete, "", map[string]any{"item": "Bloodmoss"}), owner, "Bloodmoss is no longer a material; Healing potion ask for it or its kind", ""},
		{"an item the paste makes", made, owner, "makes the new item a material: Herb", ""},
		{"already a material", craftRec("material", ActionCreate, "", map[string]any{"item": "Troll tooth", "kinds": []any{"bone"}}), owner, "", "already a material (Bone, Remnant part); use action: update"},
		{"not a material yet", craftRec("material", ActionUpdate, "", map[string]any{"item": "Troll hide", "kinds": []any{"hide"}}), owner, "", "not a material yet"},
		{"no kinds", craftRec("material", ActionCreate, "", map[string]any{"item": "Troll hide"}), owner, "", "it needs `kinds`"},
		{"an unknown kind", craftRec("material", ActionCreate, "", map[string]any{"item": "Troll hide", "kinds": []any{"slime"}}), owner, "", `"slime" is not a kind of material`},
		{"no such item", craftRec("material", ActionCreate, "", map[string]any{"item": "Dragon heart", "kinds": []any{"bone"}}), owner, "", "no Armory item called"},
		{"two items share the name", craftRec("material", ActionCreate, "", map[string]any{"item": "Twin blade", "kinds": []any{"bone"}}), owner, "", "two Armory items are called"},
		{"a player can't", craftRec("material", ActionCreate, "", map[string]any{"item": "Troll hide", "kinds": []any{"hide"}}), player, "", "only the campaign's Directors"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := MaterialKind{Svc: newFakeCrafting()}.Plan(context.Background(), camp, tt.a, tt.r)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" || p.Summary != tt.summary {
				t.Fatalf("plan = %+v, want %q", p, tt.summary)
			}
		})
	}
}

func TestMaterialKind_Apply(t *testing.T) {
	f := newFakeCrafting()
	k := MaterialKind{Svc: f}
	if err := k.Apply(context.Background(), camp, owner, craftRec("material", ActionCreate, "Troll hide", map[string]any{"kinds": []any{"hide"}})); err != nil {
		t.Fatal(err)
	}
	if err := k.Apply(context.Background(), camp, owner, craftRec("material", ActionDelete, "", map[string]any{"item": "Troll tooth"})); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.tags, map[string][]string{"hide": {"hide"}, "tooth": nil}) {
		t.Fatalf("tags set %v", f.tags)
	}
}

func TestRecipeKind_Plan(t *testing.T) {
	made := craftRec("recipe", ActionCreate, "Frost tonic", map[string]any{"makes": "Frost tonic", "materials": []any{"Frost lily"}, "goal": 10})
	made.Made = map[string]bool{"frost-tonic": true, "frost-lily": true}
	tests := []struct {
		name     string
		r        Record
		summary  string
		warn     string
		err      string
		noChecks bool
	}{
		{"a new recipe arrives as a draft", craftRec("recipe", ActionCreate, "Troll tonic", map[string]any{
			"makes": "2 Healing potion", "materials": []any{"2 Troll tooth", "1 any Liquid", map[string]any{"any": "herb", "qty": 3}},
			"method": "Steady brew", "goal": 40, "characteristic": "Might", "cost": "12.5 gp", "known_by": []any{"Mira"}}),
			"a draft players see once you approve it in Crafting: makes 2 × Healing potion from 2 Troll tooth, 1 any liquid and 3 any herb; goal 40; 2d10 + Might; a steady brew; costs 12.5 gp; must be learned (known by Mira)", "", "", false},
		{"no method warns", craftRec("recipe", ActionCreate, "Plain", map[string]any{"makes": "Healing potion", "goal": 5}),
			"a draft players see once you approve it in Crafting: makes 1 × Healing potion; goal 5; 2d10", "No `method`", "", false},
		{"an update keeps what isn't written", craftRec("recipe", ActionUpdate, "healing potion", map[string]any{"goal": 45}),
			"now makes 1 × Healing potion from 2 Bloodmoss and 1 any liquid; goal 45; 2d10 + Reason; costs 25 gp", "", "", false},
		{"a rename, and who everyone", craftRec("recipe", ActionUpdate, "Healing potion", map[string]any{"rename_to": "Greater healing", "who": "everyone", "cost": "none"}),
			"now renamed \"Greater healing\"; makes 1 × Healing potion from 2 Bloodmoss and 1 any liquid; goal 30; 2d10 + Reason", "", "", false},
		{"remove", craftRec("recipe", ActionDelete, "Healing potion", nil), "removes the recipe; heroes' projects from it carry on", "", "", true},
		{"items the paste makes skip the service's check", made,
			"a draft players see once you approve it in Crafting: makes 1 × Frost tonic from 1 Frost lily; goal 10; 2d10", "is made in this paste", "", true},
		{"a taken name", craftRec("recipe", ActionCreate, "HEALING POTION", map[string]any{"goal": 1}), "", "", "already a recipe called \"Healing potion\"; use action: update", true},
		{"no such recipe", craftRec("recipe", ActionUpdate, "Elixir", map[string]any{"goal": 1}), "", "", "no recipe called", true},
		{"a new recipe needs a goal", craftRec("recipe", ActionCreate, "Elixir", nil), "", "", "needs `goal`", true},
		{"an unknown method", craftRec("recipe", ActionCreate, "Elixir", map[string]any{"goal": 1, "method": "forge"}), "", "", `"forge" is not a method; the methods are steady, volatile`, true},
		{"an unknown kind of material", craftRec("recipe", ActionCreate, "Elixir", map[string]any{"goal": 1, "materials": []any{"1 any slime"}}), "", "", `"slime" is not a kind of material`, true},
		{"an unknown characteristic", craftRec("recipe", ActionCreate, "Elixir", map[string]any{"goal": 1, "characteristic": "Luck"}), "", "", "not one of the system's characteristics (Might, Reason)", true},
		{"an unknown item", craftRec("recipe", ActionCreate, "Elixir", map[string]any{"goal": 1, "makes": "Elixir"}), "", "", "makes: no Armory item called", true},
		{"an unknown character", craftRec("recipe", ActionCreate, "Elixir", map[string]any{"goal": 1, "known_by": []any{"Ghost"}}), "", "", "known_by: no character called", true},
		{"a bad cost", craftRec("recipe", ActionCreate, "Elixir", map[string]any{"goal": 1, "cost": "lots"}), "", "", "cost: write it as a number", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeCrafting()
			p := RecipeKind{Svc: f}.Plan(context.Background(), camp, owner, tt.r)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" || p.Summary != tt.summary {
				t.Fatalf("plan = %+v\nwant %q", p, tt.summary)
			}
			if tt.warn != "" && !strings.Contains(strings.Join(p.Warnings, " | "), tt.warn) {
				t.Fatalf("warnings %v, want %q", p.Warnings, tt.warn)
			}
			if (f.checked == 0) != tt.noChecks {
				t.Fatalf("the service checked %d times", f.checked)
			}
		})
	}
}

func TestRecipeKind_RefusedByTheService(t *testing.T) {
	f := newFakeCrafting()
	f.refuse = "a recipe asks for at most 12 materials"
	k := RecipeKind{Svc: f}
	r := craftRec("recipe", ActionUpdate, "Healing potion", map[string]any{"goal": 3})
	if p := k.Plan(context.Background(), camp, owner, r); p.Error != f.refuse {
		t.Fatalf("plan = %+v", p)
	}
	if err := k.Apply(context.Background(), camp, owner, r); err == nil || len(f.saved) != 0 {
		t.Fatalf("apply saved after a refusal: %v", err)
	}
	if p := k.Plan(context.Background(), camp, player, r); !strings.Contains(p.Error, "only the campaign's Directors") {
		t.Fatalf("a player's plan: %+v", p)
	}
}

func TestRecipeKind_Apply(t *testing.T) {
	f := newFakeCrafting()
	k := RecipeKind{Svc: f}
	ctx := context.Background()
	if err := k.Apply(ctx, camp, owner, craftRec("recipe", ActionUpdate, "Healing potion", map[string]any{"materials": []any{"Troll tooth"}, "who": "learned", "known_by": []any{"Tobin"}})); err != nil {
		t.Fatal(err)
	}
	want := CraftRecipeInput{ID: "r1", Name: "Healing potion", Materials: []CraftMaterial{{ItemID: "tooth", Qty: 1}}, ResultID: "potion", ResultQty: 1,
		Goal: 30, Stat: "reason", Cost: 25, Currency: "gp", Who: "learned", Known: []string{"tobin"}}
	if len(f.saved) != 1 || !reflect.DeepEqual(f.saved[0], want) {
		t.Fatalf("saved %+v\nwant  %+v", f.saved, want)
	}
	if err := k.Apply(ctx, camp, owner, craftRec("recipe", ActionDelete, "Healing potion", nil)); err != nil || !reflect.DeepEqual(f.removed, []string{"r1"}) {
		t.Fatalf("removed %v, %v", f.removed, err)
	}
	// Apply runs after the paste's pages save, so an item still pending
	// there failed to save and the recipe must not be saved without it.
	gone := craftRec("recipe", ActionCreate, "Frost tonic", map[string]any{"makes": "Frost tonic", "goal": 10})
	gone.Made = map[string]bool{"frost-tonic": true}
	if err := k.Apply(ctx, camp, owner, gone); err == nil || !strings.Contains(err.Error(), `"Frost tonic" was not made`) {
		t.Fatalf("a recipe naming an item that was never made: %v", err)
	}
	if len(f.saved) != 1 {
		t.Fatalf("saved %d recipes, want only the first", len(f.saved))
	}
}

func TestDropsKind_Plan(t *testing.T) {
	made := craftRec("drops", ActionCreate, "", map[string]any{"page": "Ice wyrm", "lines": []any{"1 Troll tooth"}})
	made.Made = map[string]bool{"ice-wyrm": true}
	tests := []struct {
		name    string
		r       Record
		summary string
		err     string
	}{
		{"a new list, with an unquoted harvest YAML read as a map", craftRec("drops", ActionCreate, "", map[string]any{"page": "Dire wolf", "lines": []any{
			"2 Troll tooth", map[string]any{"harvest Troll hide with Might": "1/2/4"}, "harvest Bloodmoss: 1 / 1 / 2"}}),
			"drops 2 × Troll tooth; harvest Troll hide (2d10 + Might: 1, 2 or 4); harvest Bloodmoss (2d10: 1, 1 or 2)", ""},
		{"the map forms", craftRec("drops", ActionCreate, "", map[string]any{"page": "Dire wolf", "lines": []any{
			map[string]any{"item": "Troll tooth", "qty": 3}, map[string]any{"item": "Troll hide", "harvest": "Reason", "pays": []any{1, 3, 5}}}}),
			"drops 3 × Troll tooth; harvest Troll hide (2d10 + Reason: 1, 3 or 5)", ""},
		{"also, from another page", craftRec("drops", ActionCreate, "Troll Remnant", map[string]any{"also": "Troll"}), "drops nothing of its own; and what Troll drops", ""},
		{"an update keeps the lines", craftRec("drops", ActionUpdate, "", map[string]any{"page": "Troll", "also": "Troll Remnant"}),
			"now drops 2 × Troll tooth; harvest Troll hide (2d10 + Might: 1, 2 or 4); and what Troll Remnant drops", ""},
		{"remove", craftRec("drops", ActionDelete, "", map[string]any{"page": "Troll"}), "takes the drop list away; spoils already in the tray stay", ""},
		{"a page the paste makes", made, "drops 1 × Troll tooth", ""},
		{"already has one", craftRec("drops", ActionCreate, "", map[string]any{"page": "Troll", "lines": []any{"1 Troll tooth"}}), "", "already has a drop list; use action: update"},
		{"has none", craftRec("drops", ActionUpdate, "", map[string]any{"page": "Dire wolf", "lines": []any{"1 Troll tooth"}}), "", "has no drop list"},
		{"no lines or also", craftRec("drops", ActionCreate, "", map[string]any{"page": "Dire wolf"}), "", "it needs `lines`"},
		{"a harvest without what it pays", craftRec("drops", ActionCreate, "", map[string]any{"page": "Dire wolf", "lines": []any{"harvest Troll hide with Might"}}), "", "write a harvest as"},
		{"also itself", craftRec("drops", ActionUpdate, "", map[string]any{"page": "Troll", "also": "Troll"}), "", "can't also drop its own list"},
		{"also a missing page", craftRec("drops", ActionCreate, "", map[string]any{"page": "Dire wolf", "also": "Nowhere"}), "", "also: no page called"},
		{"no such page", craftRec("drops", ActionCreate, "", map[string]any{"page": "Nowhere", "lines": []any{"1 Troll tooth"}}), "", "no page called"},
		{"an unknown item", craftRec("drops", ActionCreate, "", map[string]any{"page": "Dire wolf", "lines": []any{"2 Wolf pelt"}}), "", "no Armory item called \"Wolf pelt\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := DropsKind{Svc: newFakeCrafting(), Entities: craftPages()}.Plan(context.Background(), camp, owner, tt.r)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" || p.Summary != tt.summary {
				t.Fatalf("plan = %+v\nwant %q", p, tt.summary)
			}
		})
	}
}

func TestDropsKind_Apply(t *testing.T) {
	f := newFakeCrafting()
	k := DropsKind{Svc: f, Entities: craftPages()}
	ctx := context.Background()
	if err := k.Apply(ctx, camp, owner, craftRec("drops", ActionUpdate, "Troll", map[string]any{"lines": []any{"1 Bloodmoss"}})); err != nil {
		t.Fatal(err)
	}
	want := CraftDropsInput{Lines: []CraftDropLine{{ItemID: "moss", Qty: 1}}, Version: 3}
	if !reflect.DeepEqual(f.drops["troll"], want) {
		t.Fatalf("saved %+v", f.drops["troll"])
	}
	if err := k.Apply(ctx, camp, owner, craftRec("drops", ActionDelete, "Troll", nil)); err != nil || !reflect.DeepEqual(f.dropped, []string{"troll"}) {
		t.Fatalf("removed %v, %v", f.dropped, err)
	}
	gone := craftRec("drops", ActionUpdate, "Troll", map[string]any{"lines": []any{"1 Frost lily"}})
	gone.Made = map[string]bool{"frost-lily": true}
	if err := k.Apply(ctx, camp, owner, gone); err == nil || !strings.Contains(err.Error(), `"Frost lily" was not made`) {
		t.Fatalf("a drop list naming an item that was never made: %v", err)
	}
	f.c.DropsOff = true
	if p := k.Plan(ctx, camp, owner, craftRec("drops", ActionCreate, "Dire wolf", map[string]any{"lines": []any{"1 Bloodmoss"}})); !strings.Contains(p.Error, "can't be changed") {
		t.Fatalf("drop lists off: %+v", p)
	}
}

// blockRecord reads a block the lookup printed back as the importer would.
func blockRecord(t *testing.T, block, action string) Record {
	t.Helper()
	fm := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(block), "---\n"), "\n---")
	fields := map[string]any{}
	if err := yaml.Unmarshal([]byte(fm), &fields); err != nil {
		t.Fatalf("the block is not YAML: %v\n%s", err, block)
	}
	name, _ := fields["name"].(string)
	return Record{Kind: fields["kind"].(string), Action: action, Name: name, Fields: fields}
}

func TestCraftingBlocksGoBackUnchanged(t *testing.T) {
	f := newFakeCrafting()
	f.c.Recipes[0].Who, f.c.Recipes[0].Known = "learned", []string{"mira"}
	f.c.Recipes[0].Prerequisite, f.c.Recipes[0].Source = "A healer's kit", "Old notes: \"brew\""
	c, _ := f.Read(context.Background(), camp, owner)
	ctx := context.Background()

	rec := blockRecord(t, recipeBlock(c, c.Recipes[0]), ActionUpdate)
	p := RecipeKind{Svc: f}.Plan(ctx, camp, owner, rec)
	if p.Error != "" || p.Summary != "now makes 1 × Healing potion from 2 Bloodmoss and 1 any liquid; goal 30; 2d10 + Reason; a steady brew; costs 25 gp; must be learned (known by Mira)" {
		t.Fatalf("the recipe block read back as %+v", p)
	}
	in, _, _ := RecipeKind{Svc: f}.input(c, rec, &c.Recipes[0])
	want := CraftRecipeInput{ID: "r1", Name: "Healing potion", Method: "steady", Materials: c.Recipes[0].Materials, ResultID: "potion", ResultQty: 1,
		Goal: 30, Stat: "reason", Prerequisite: "A healer's kit", Source: "Old notes: \"brew\"", Cost: 25, Currency: "gp", Who: "learned", Known: []string{"mira"}}
	if !reflect.DeepEqual(in, want) {
		t.Fatalf("read back %+v\nwant      %+v", in, want)
	}

	drops := blockRecord(t, dropsBlock(c, c.Drops[0]), ActionUpdate)
	k := DropsKind{Svc: f, Entities: craftPages()}
	if err := k.Apply(ctx, camp, owner, drops); err != nil {
		t.Fatal(err)
	}
	if got := f.drops["troll"]; !reflect.DeepEqual(got.Lines, c.Drops[0].Lines) {
		t.Fatalf("the drops block read back as %+v", got.Lines)
	}

	mat := blockRecord(t, materialBlock(c, c.Items[0]), ActionUpdate)
	if p := (MaterialKind{Svc: f}).Plan(ctx, camp, owner, mat); p.Summary != "now Bone, Remnant part" {
		t.Fatalf("the material block read back as %+v", p)
	}
}

func TestCraftingExports(t *testing.T) {
	f := newFakeCrafting()
	ctx := context.Background()
	for _, k := range []Kind{RecipeKind{Svc: f}, MaterialKind{Svc: f}, DropsKind{Svc: f}} {
		got, err := k.Export(ctx, camp, owner)
		if err != nil || !strings.Contains(got, "kind: "+k.Name()+"\n") {
			t.Fatalf("%s export: %q, %v", k.Name(), got, err)
		}
		if got, _ := k.Export(ctx, camp, player); got != "" {
			t.Fatalf("a player got a %s export: %q", k.Name(), got)
		}
	}
}

func TestLookup_Crafting(t *testing.T) {
	f := newFakeCrafting()
	_, looks := lookKind()
	l := &Lookups{Crafting: f, Looks: looks}
	ctx := context.Background()
	ans := l.Answer(ctx, camp, owner, []Record{lookup("crafting", nil)})[0]
	for _, want := range []string{
		"Rolls use 2d10 plus one characteristic: Might, Reason (from Draw Steel). A harvest's three outcomes: 11 or less, 12 to 16, 17 or more.",
		"Defaults: players start in the journal look; the timing is standard; botched work gives a bane; players may not skip",
		"Kinds of material: Herb, Bone, Remnant part, Liquid, Hide.",
		"- Healing potion (a draft): makes 1 × Healing potion from 2 Bloodmoss and 1 any liquid; goal 30; 2d10 + Reason",
		"- Troll tooth: Bone, Remnant part; dropped by Troll",
		"- Bloodmoss: Herb; used by Healing potion",
		"- Troll: drops 2 × Troll tooth; harvest Troll hide (2d10 + Might: 1, 2 or 4)",
		"### The spoils tray (2 things waiting)", "- Troll (1 of 2), from Ramparts fight: 2 × Troll tooth; Troll hide, to harvest",
		"- Mira: Healing potion, 12 of 30 points, 1 roll ready",
		"- Mira's kitchen (Apothecary bench)",
	} {
		if !strings.Contains(ans.Text, want) {
			t.Fatalf("the overview misses %q:\n%s", want, ans.Text)
		}
	}
	if strings.Contains(ans.Text, "not stored") || ans.Chip != "Crafting · 1 recipe" {
		t.Fatalf("chip %q, or a stale line:\n%s", ans.Chip, ans.Text)
	}

	parts := l.Answer(ctx, camp, owner, []Record{
		lookup("crafting", map[string]any{"part": "recipes"}),
		lookup("materials", nil),
		lookup("drops", map[string]any{"page": "Troll"}),
		lookup("crafting", map[string]any{"part": "looks"}),
		lookup("crafting", map[string]any{"name": "Troll"}),
		lookup("crafting", map[string]any{"part": "stations"}),
		lookup("crafting", map[string]any{"part": "recipes", "name": "Elixir"}),
	})
	wants := []string{
		"Healing potion: a draft, which players don't see until a Director approves it in Crafting; 2 rules in Crafting's rules editor, which an update keeps.\n\n```\n---\nkind: recipe\nname: \"Healing potion\"",
		"kind: material\nitem: \"Troll tooth\"\nkinds: [bone, remnant part]",
		"kind: drops\npage: \"Troll\"\nlines:\n  - \"2 Troll tooth\"\n  - \"harvest Troll hide with Might: 1/2/4\"",
		"page: \"Flametongue\"",
		"kind: drops\npage: \"Troll\"",
	}
	for i, want := range wants {
		if parts[i].Error != "" || !strings.Contains(parts[i].Text, want) {
			t.Fatalf("part %d: %+v\nwant %q", i, parts[i], want)
		}
	}
	if !strings.Contains(parts[5].Error, "the parts are recipes, materials, drops, looks") || !strings.Contains(parts[6].Error, "no recipe called") {
		t.Fatalf("refusals: %q, %q", parts[5].Error, parts[6].Error)
	}

	// What players see: the book and the looks they may see, and no part.
	pl := l.Answer(ctx, camp, player, []Record{lookup("crafting", nil), lookup("recipes", nil), lookup("looks", nil)})
	if !strings.Contains(pl[0].Text, "- Healing potion (Steady brew): makes 1 × Healing potion. Needs 2 Bloodmoss and 1 any liquid. Project goal 30; roll 2d10 + Reason") ||
		strings.Contains(pl[0].Text, "Troll") || strings.Contains(pl[0].Text, "Defaults") {
		t.Fatalf("a player's crafting:\n%s", pl[0].Text)
	}
	if !strings.Contains(pl[1].Error, "sees what players see") {
		t.Fatalf("a player's recipes: %+v", pl[1])
	}
	if pl[2].Error != "" || strings.Contains(pl[2].Text, "Secret blade") {
		t.Fatalf("a player's looks: %+v", pl[2])
	}

	none := (&Lookups{}).Answer(ctx, camp, owner, []Record{lookup("crafting", nil), lookup("looks", nil)})
	if !strings.Contains(none[0].Text, "can't be looked up on this server") || !strings.Contains(none[1].Text, "can't be looked up right now") {
		t.Fatalf("no services: %+v", none)
	}
}

// The doc's example blocks must plan cleanly against a campaign that has
// what they name.
func TestCraftingDocExamplesPlan(t *testing.T) {
	f := newFakeCrafting()
	f.c.Items = append(f.c.Items, CraftItem{ID: "tonic", Name: "Troll-blood tonic"}, CraftItem{ID: "ash", Name: "Remnant ash"})
	f.c.Drops = nil
	pages := craftPages()
	for _, k := range []Kind{RecipeKind{Svc: f}, MaterialKind{Svc: f}, DropsKind{Svc: f, Entities: pages}} {
		doc := k.Doc()
		for _, part := range strings.Split(doc, "```\n")[1:] {
			if !strings.HasPrefix(part, "---\n") {
				continue
			}
			r := blockRecord(t, part[:strings.LastIndex(part, "---")+3], ActionCreate)
			if k.Name() == "material" {
				r.Fields["item"] = "Troll hide"
			}
			if p := k.Plan(context.Background(), camp, owner, r); p.Error != "" {
				t.Errorf("%s's example: %s\n%s", k.Name(), p.Error, part)
			}
		}
	}
	_ = entities.Slugify
}
