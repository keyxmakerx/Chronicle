package crafting

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// ---------- Fakes for making recipes where the work is ----------

// madeDir is fakeDir plus the items crafting made during a test.
type madeDir struct {
	fakeDir
	made map[string]EntityInfo
}

func (d *madeDir) Entity(ctx context.Context, cid, id string) (*EntityInfo, error) {
	if e, ok := d.made[id]; ok {
		return &e, nil
	}
	return d.fakeDir.Entity(ctx, cid, id)
}
func (d *madeDir) ListItems(ctx context.Context, cid string, role int, uid string) ([]Named, bool, error) {
	out, cut, err := d.fakeDir.ListItems(ctx, cid, role, uid)
	ids := make([]string, 0, len(d.made))
	for id := range d.made {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		out = append(out, Named{ID: id, Name: d.made[id].Name})
	}
	return out, cut, err
}

// fakeItems makes Armory items into the madeDir.
type fakeItems struct {
	dir *madeDir
	n   int
}

func (m *fakeItems) CreateItem(_ context.Context, _, _, name string) (string, error) {
	m.n++
	id := fmt.Sprintf("item-%d", m.n)
	m.dir.made[id] = EntityInfo{ID: id, Name: name, IsItem: true}
	return id, nil
}

// fakePacks keeps packs in memory.
type fakePacks struct{ rows map[string]PackRow }

func (f *fakePacks) CreatePack(_ context.Context, p PackRow) error { f.rows[p.ID] = p; return nil }
func (f *fakePacks) GetPack(_ context.Context, id string) (PackRow, bool, error) {
	p, ok := f.rows[id]
	return p, ok, nil
}
func (f *fakePacks) ListPacks(_ context.Context, ids []string, shared bool, _ int) ([]PackRow, error) {
	var out []PackRow
	for _, p := range f.rows {
		if oneOfStr(ids, p.CampaignID) || (shared && p.Shared) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (f *fakePacks) UpdatePack(_ context.Context, cid, id, name string, shared bool) (bool, error) {
	p, ok := f.rows[id]
	if !ok || p.CampaignID != cid {
		return false, nil
	}
	p.Name, p.Shared = name, shared
	f.rows[id] = p
	return true, nil
}
func (f *fakePacks) DeletePack(_ context.Context, cid, id string) error {
	if p, ok := f.rows[id]; ok && p.CampaignID == cid {
		delete(f.rows, id)
	}
	return nil
}

// fakeCampaigns: the Director "dm" runs c and other; nobody else runs
// anything.
type fakeCampaigns struct{}

func (fakeCampaigns) DirectedBy(_ context.Context, uid string) ([]Named, error) {
	if uid == "dm" {
		return []Named{{"c", "Here"}, {"other", "Therin"}}, nil
	}
	return nil, nil
}
func (fakeCampaigns) Directs(_ context.Context, cid, uid string) (bool, error) {
	return uid == "dm" && (cid == "c" || cid == "other"), nil
}

// itemPages lets players see every item but the secret one.
type itemPages struct{}

func (itemPages) Page(context.Context, string, string) (*PageInfo, error) { return nil, nil }
func (itemPages) CanSee(_ context.Context, _, id string, role int, _ string) (bool, error) {
	return id != "secret" || role >= 3, nil
}

func newMakingService() (*service, *fakeRepo, *fakePacks, *madeDir) {
	s, repo, _ := newTestService()
	dir := &madeDir{made: map[string]EntityInfo{}}
	packs := &fakePacks{rows: map[string]PackRow{}}
	s.dir = dir
	s.projects = ProjectDeps{Items: &fakeItems{dir: dir}, Packs: packs, Campaigns: fakeCampaigns{}, Pages: itemPages{}}
	return s, repo, packs, dir
}

func addRecipe(repo *fakeRepo, r Recipe) {
	if r.Version == 0 {
		r.Version = 1
	}
	if r.Who == "" {
		r.Who = "all"
	}
	repo.recipes[r.ID] = r
	repo.order = append(repo.order, r.ID)
}

// ---------- Writes stay the Director's ----------

func TestMakingWritesAreDirectorsOnly(t *testing.T) {
	s, repo, packs, _ := newMakingService()
	ctx := context.Background()
	addRecipe(repo, Recipe{ID: "r1", Name: "Tonic", Goal: 10})
	packs.rows["p1"] = PackRow{ID: "p1", CampaignID: "c", Name: "Mine", Body: "{}"}
	calls := map[string]func() error{
		"material": func() error { _, err := s.CreateMaterial(ctx, "c", player, MaterialInput{Name: "Moss"}); return err },
		"place":    func() error { _, err := s.PlaceItemBlock(ctx, "c", player); return err },
		"teach":    func() error { _, err := s.TeachParty(ctx, "c", player, "Tonic"); return err },
		"known":    func() error { _, err := s.KnownRecipes(ctx, "c", player, []string{"r1"}); return err },
		"generate": func() error { _, err := s.Generate(ctx, "c", player, GenerateInput{Seed: "x"}); return err },
		"packs":    func() error { _, err := s.ListPacks(ctx, "c", player); return err },
		"pack":     func() error { _, err := s.PackDetail(ctx, "c", "p1", player); return err },
		"save pack": func() error {
			_, err := s.SavePack(ctx, "c", player, SavePackInput{Name: "x", Recipes: []string{"r1"}})
			return err
		},
		"upload": func() error { _, err := s.UploadPack(ctx, "c", player, []byte("{}")); return err },
		"share pack": func() error {
			_, err := s.UpdatePack(ctx, "c", "p1", player, PackChange{Name: "x", Shared: true})
			return err
		},
		"drop pack": func() error { return s.DeletePack(ctx, "c", "p1", player) },
		"pack file": func() error { _, _, err := s.PackFile(ctx, "c", "p1", player); return err },
		"import":    func() error { _, err := s.ImportPack(ctx, "c", "p1", player, ImportInput{}); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); code(err) != http.StatusForbidden {
				t.Fatalf("a player got %v, want 403", err)
			}
		})
	}
	if len(packs.rows) != 1 || packs.rows["p1"].Shared || len(repo.recipes) != 1 {
		t.Fatal("a refused call still changed something")
	}
}

// ---------- Drafts ----------

func TestCreateRecipeKeepsTheDraftFlag(t *testing.T) {
	tests := []struct {
		name  string
		draft bool
	}{{"draft", true}, {"ready", false}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _, _ := newTestService()
			r, err := s.CreateRecipe(context.Background(), "c", director, RecipeInput{Name: "Tonic", Goal: 5, Draft: tt.draft})
			if err != nil {
				t.Fatal(err)
			}
			if r.Draft != tt.draft {
				t.Fatalf("draft = %v, want %v", r.Draft, tt.draft)
			}
		})
	}
}

// ---------- The item page ----------

func TestItemCraftingShowsAPlayerOnlyTheirBook(t *testing.T) {
	s, repo, _, _ := newMakingService()
	ctx := context.Background()
	repo.tags["moss"] = []string{"herb"}
	addRecipe(repo, Recipe{ID: "ok", Name: "Healing tonic", Result: Result{ItemID: "potion", Qty: 1}, Goal: 10,
		Materials: []Material{{ItemID: "moss", Qty: 2}}})
	addRecipe(repo, Recipe{ID: "draft", Name: "Draft tonic", Draft: true, Result: Result{ItemID: "potion", Qty: 1}, Goal: 10,
		Materials: []Material{{Tag: "herb", Qty: 1}}})
	addRecipe(repo, Recipe{ID: "learned", Name: "Secret tonic", Who: "learned", Result: Result{ItemID: "potion", Qty: 1}, Goal: 10})

	tests := []struct {
		name      string
		item      string
		v         Viewer
		wantCode  int
		wantMade  int
		wantBook  []string
		wantUses  []string
		wantKinds int
	}{
		{name: "director sees every recipe", item: "potion", v: director, wantMade: 3, wantBook: nil},
		{name: "player sees only approved pages they may read", item: "potion", v: player, wantBook: []string{"Healing tonic"}},
		{name: "director sees uses, by name and by kind", item: "moss", v: director, wantUses: []string{"Healing tonic", "Draft tonic"}, wantKinds: 1},
		{name: "player sees no draft use", item: "moss", v: player, wantUses: []string{"Healing tonic"}},
		{name: "a hidden item is not found for a player", item: "secret", v: player, wantCode: http.StatusNotFound},
		{name: "a character is not an item", item: "mira", v: director, wantCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ItemCrafting(ctx, "c", tt.item, tt.v)
			if tt.wantCode != 0 {
				if code(err) != tt.wantCode {
					t.Fatalf("got %v, want %d", err, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got.MadeBy) != tt.wantMade {
				t.Errorf("madeBy = %d, want %d", len(got.MadeBy), tt.wantMade)
			}
			var book, uses []string
			for _, p := range got.Book {
				book = append(book, p.Name)
			}
			for _, u := range got.Uses {
				uses = append(uses, u.Name)
			}
			if !reflect.DeepEqual(book, tt.wantBook) {
				t.Errorf("book = %v, want %v", book, tt.wantBook)
			}
			if !reflect.DeepEqual(uses, tt.wantUses) {
				t.Errorf("uses = %v, want %v", uses, tt.wantUses)
			}
			if len(got.Kinds) != tt.wantKinds {
				t.Errorf("kinds = %v, want %d", got.Kinds, tt.wantKinds)
			}
			if !tt.v.Director && (len(got.Kinds) != 0 || len(got.Drops) != 0) {
				t.Error("a player was told the item's kinds or drops")
			}
		})
	}
}

func TestCreateMaterialMakesAPrivateItemWithKinds(t *testing.T) {
	s, repo, _, dir := newMakingService()
	ctx := context.Background()
	it, err := s.CreateMaterial(ctx, "c", director, MaterialInput{Name: " Ghost moss ", Kinds: []string{"herb", "herb", "fungus"}})
	if err != nil {
		t.Fatal(err)
	}
	if it.Name != "Ghost moss" || !reflect.DeepEqual(repo.tags[it.ID], []string{"herb", "fungus"}) || dir.made[it.ID].Name != "Ghost moss" {
		t.Fatalf("made %+v with tags %v", it, repo.tags[it.ID])
	}
	if _, err := s.CreateMaterial(ctx, "c", director, MaterialInput{Name: "X", Kinds: []string{"plutonium"}}); code(err) != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown kind got %v, want 422", err)
	}
}

// ---------- Rules: teach and the part's list ----------

func TestTeachParty(t *testing.T) {
	tests := []struct {
		name   string
		recipe Recipe
		ask    string
		want   string
		known  int
		bad    bool
	}{
		{name: "everyone knows it already", recipe: Recipe{ID: "r", Name: "Tonic", Who: "all"}, ask: "tonic", want: "Everyone already knows Tonic."},
		{name: "teaches each hero", recipe: Recipe{ID: "r", Name: "Tonic", Who: "learned"}, ask: "Tonic", want: "Taught Tonic to Mira and Tobin.", known: 2},
		{name: "says a draft is still hidden", recipe: Recipe{ID: "r", Name: "Tonic", Who: "learned", Draft: true}, ask: "Tonic",
			want: "Taught Tonic to Mira and Tobin. It is still a draft, so players see it once you approve it.", known: 2},
		{name: "an unknown recipe is refused", recipe: Recipe{ID: "r", Name: "Tonic"}, ask: "Elixir", bad: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, repo, _ := newTestService()
			addRecipe(repo, tt.recipe)
			got, err := s.TeachParty(context.Background(), "c", director, tt.ask)
			if tt.bad {
				if code(err) != http.StatusUnprocessableEntity {
					t.Fatalf("got %v, want 422", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want || len(repo.recipes["r"].Known) != tt.known {
				t.Fatalf("got %q with %d known, want %q with %d", got, len(repo.recipes["r"].Known), tt.want, tt.known)
			}
		})
	}
}

func TestKnownRecipesDropsGoneAndRepeated(t *testing.T) {
	s, repo, _ := newTestService()
	addRecipe(repo, Recipe{ID: "a", Name: "A"})
	addRecipe(repo, Recipe{ID: "b", Name: "B"})
	got, err := s.KnownRecipes(context.Background(), "c", director, []string{"b", "gone", "a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"b", "a"}) {
		t.Fatalf("got %v", got)
	}
}

// ---------- The generator ----------

func genTestFacts() genFacts {
	items := []MaterialItem{
		{ID: "silver", Name: "Silverleaf", Tags: []string{"herb"}},
		{ID: "blood", Name: "Bloodroot", Tags: []string{"herb"}},
		{ID: "moon", Name: "Moonwater", Tags: []string{"liquid"}},
	}
	f := genFacts{items: items, byKind: map[string][]MaterialItem{}, tags: map[string][]string{}, taken: map[string]bool{}, byName: map[string]string{}}
	for _, it := range items {
		f.tags[it.ID] = it.Tags
		f.byName[strings.ToLower(it.Name)] = it.ID
		for _, k := range it.Tags {
			f.byKind[k] = append(f.byKind[k], it)
		}
	}
	return f
}

func TestGenerateIsSeeded(t *testing.T) {
	in, mk, err := cleanGenerate(GenerateInput{Make: "potions", Kinds: []string{"herb", "liquid"}, Count: 8, Seed: "ash-4127"})
	if err != nil {
		t.Fatal(err)
	}
	f := genTestFacts()
	a, b := generateRows(in, mk, f), generateRows(in, mk, f)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("the same seed gave two lists")
	}
	other := in
	other.Seed = "moss-1000"
	if reflect.DeepEqual(a, generateRows(other, mk, f)) {
		t.Fatal("another seed gave the same list")
	}

	// A reroll changes its row and leaves the rows before it alone; later
	// rows keep their materials and method (only a clashing name moves).
	re := in
	re.Rerolls = append([]int{}, in.Rerolls...)
	re.Rerolls[3] = 1
	c := generateRows(re, mk, f)
	if reflect.DeepEqual(a[3], c[3]) {
		t.Fatal("a reroll left its row as it was")
	}
	for i := range a {
		if i < 3 && !reflect.DeepEqual(a[i], c[i]) {
			t.Fatalf("row %d changed on another row's reroll", i)
		}
		if i > 3 && (!reflect.DeepEqual(a[i].Materials, c[i].Materials) || a[i].Template != c[i].Template) {
			t.Fatalf("row %d's recipe changed on another row's reroll", i)
		}
	}
	names := map[string]bool{}
	for _, r := range a {
		if names[r.Name] {
			t.Fatalf("two rows are called %q", r.Name)
		}
		names[r.Name] = true
	}
}

func TestGenerateGoalsComeOnlyFromPublishedOrOwn(t *testing.T) {
	potions := makeKind("potions")
	tests := []struct {
		name     string
		goals    []systems.CraftingGoal
		own      int
		echelon  int
		want     int
		wantFrom string
	}{
		{name: "published for the echelon and kind", goals: []systems.CraftingGoal{{Echelon: 1, Goal: 20}, {Echelon: 1, For: "consumable", Goal: 45}}, echelon: 1, want: 45, wantFrom: "system"},
		{name: "published for the echelon", goals: []systems.CraftingGoal{{Echelon: 2, Goal: 90}, {Echelon: 2, For: "weapon", Goal: 300}}, echelon: 2, want: 90, wantFrom: "system"},
		{name: "another echelon falls to the Director's own", goals: []systems.CraftingGoal{{Echelon: 1, Goal: 20}}, own: 30, echelon: 3, want: 30, wantFrom: "recipes"},
		{name: "nothing published, nothing written", echelon: 1, want: 0, wantFrom: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, from := genGoal(genFacts{goals: tt.goals, ownGoal: tt.own}, potions, tt.echelon)
			if g != tt.want || from != tt.wantFrom {
				t.Fatalf("got %d from %q, want %d from %q", g, from, tt.want, tt.wantFrom)
			}
		})
	}
	if got := middleGoal([]Recipe{{Goal: 10}, {Goal: 50, Draft: true}, {Goal: 30}, {Goal: 20}}); got != 20 {
		t.Fatalf("middle of the approved goals = %d, want 20", got)
	}
}

func TestGenerateNamesMissingKinds(t *testing.T) {
	in, mk, err := cleanGenerate(GenerateInput{Make: "salves", Kinds: []string{"resin"}, Count: 3, Seed: "x"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range generateRows(in, mk, genTestFacts()) {
		if !oneOfStr(r.Missing, "resin") {
			t.Fatalf("row %q doesn't say it needs a resin the campaign lacks: %v", r.Name, r.Missing)
		}
		for _, m := range r.Materials {
			if m.Tag == "resin" && m.Name != "any resin" {
				t.Fatalf("a missing kind reads %q", m.Name)
			}
		}
	}
}

func TestCleanGenerate(t *testing.T) {
	tests := []struct {
		name      string
		in        GenerateInput
		bad       bool
		wantCount int
		wantKinds []string
	}{
		{name: "defaults", in: GenerateInput{Seed: "s"}, wantCount: 6, wantKinds: MakeKinds[0].Kinds},
		{name: "count is capped", in: GenerateInput{Seed: "s", Count: 99, Make: "gear"}, wantCount: 12, wantKinds: makeKind("gear").Kinds},
		{name: "no seed", in: GenerateInput{}, bad: true},
		{name: "a long seed", in: GenerateInput{Seed: strings.Repeat("a", 41)}, bad: true},
		{name: "an unknown make", in: GenerateInput{Seed: "s", Make: "boats"}, bad: true},
		{name: "an unknown kind", in: GenerateInput{Seed: "s", Kinds: []string{"plutonium"}}, bad: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := cleanGenerate(tt.in)
			if tt.bad {
				if code(err) != http.StatusUnprocessableEntity {
					t.Fatalf("got %v, want 422", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Count != tt.wantCount || len(got.Rerolls) != tt.wantCount || !reflect.DeepEqual(got.Kinds, tt.wantKinds) {
				t.Fatalf("got count %d, rerolls %d, kinds %v", got.Count, len(got.Rerolls), got.Kinds)
			}
		})
	}
}

// ---------- Recipe packs ----------

func packJSON(t *testing.T, b packBody) string {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func goodPack() packBody {
	return packBody{Format: PackFormat, Version: packVersion, Name: "Hedge alchemy",
		Materials: []PackMaterial{{Name: "Bloodmoss", Kinds: []string{"herb"}}, {Name: "Ghost moss", Kinds: []string{"fungus"}}, {Name: "Night tonic"}},
		Recipes: []PackRecipe{
			{Name: "Night tonic", Lines: []PackLine{{Material: "Bloodmoss", Qty: 2}, {Material: "Ghost moss", Qty: 1}}, Makes: "Night tonic", MakesQty: 1, Goal: 30, Who: "all"},
			{Name: "Herb paste", Lines: []PackLine{{Kind: "herb", Qty: 1}}, Goal: 15, Who: "all"},
		}}
}

func TestDecodePackChecksEverything(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(b *packBody)
		raw   string
		valid bool
	}{
		{name: "a good pack", edit: func(*packBody) {}, valid: true},
		{name: "not json", raw: "<html>", valid: false},
		{name: "another format", edit: func(b *packBody) { b.Format = "foundry-items" }},
		{name: "a newer version", edit: func(b *packBody) { b.Version = 2 }},
		{name: "no recipes", edit: func(b *packBody) { b.Recipes = nil }},
		{name: "a line for a material it doesn't list", edit: func(b *packBody) { b.Recipes[0].Lines[0].Material = "Dragon scale" }},
		{name: "a line that is both", edit: func(b *packBody) { b.Recipes[1].Lines[0].Material = "Bloodmoss" }},
		{name: "an empty line", edit: func(b *packBody) { b.Recipes[1].Lines[0] = PackLine{Qty: 1} }},
		{name: "an unknown kind", edit: func(b *packBody) { b.Materials[0].Kinds = []string{"plutonium"} }},
		{name: "two recipes of one name", edit: func(b *packBody) { b.Recipes[1].Name = "night tonic" }},
		{name: "makes what it doesn't list", edit: func(b *packBody) { b.Recipes[0].Makes = "Gold" }},
		{name: "a control character in a name", edit: func(b *packBody) { b.Materials[1].Name = "Ghost\u0000moss" }},
		{name: "too many recipes", edit: func(b *packBody) {
			for i := 0; i <= maxPackRecipes; i++ {
				b.Recipes = append(b.Recipes, PackRecipe{Name: fmt.Sprintf("R%d", i), Lines: []PackLine{{Kind: "herb", Qty: 1}}})
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := tt.raw
			if raw == "" {
				b := goodPack()
				tt.edit(&b)
				raw = packJSON(t, b)
			}
			_, err := decodePack([]byte(raw))
			if tt.valid != (err == nil) {
				t.Fatalf("err = %v, want valid %v", err, tt.valid)
			}
			if err != nil && code(err) != http.StatusUnprocessableEntity {
				t.Fatalf("a bad pack got %v, want 422", err)
			}
		})
	}
}

func TestBuildPackCarriesOnlyNames(t *testing.T) {
	one := 1
	all := []Recipe{
		{ID: "r1", Name: "Tonic", Materials: []Material{{ItemID: "gone", Qty: 1}, {ItemID: "moss", Qty: 2}, {Tag: "liquid", Qty: 1}},
			Steps: []Step{{T: "add", Material: &one}}, Result: Result{ItemID: "potion", Qty: 2}, Goal: 30, Who: "learned", Known: []string{"mira"}},
		{ID: "r2", Name: "Not chosen"},
	}
	items := []Named{{"moss", "Bloodmoss"}, {"potion", "Healing potion"}}
	b := buildPack("Mine", all, []string{"r1"}, items, map[string][]string{"moss": {"herb"}})
	if len(b.Recipes) != 1 {
		t.Fatalf("recipes = %d", len(b.Recipes))
	}
	r := b.Recipes[0]
	wantLines := []PackLine{{Material: "Bloodmoss", Qty: 2}, {Kind: "liquid", Qty: 1}}
	if !reflect.DeepEqual(r.Lines, wantLines) || r.Makes != "Healing potion" || r.MakesQty != 2 {
		t.Fatalf("lines %v makes %q", r.Lines, r.Makes)
	}
	// The step pointed at line 1 (Bloodmoss); with the gone item dropped it
	// points at line 0.
	if r.Steps[0].Material == nil || *r.Steps[0].Material != 0 {
		t.Fatalf("step points at %v", r.Steps[0].Material)
	}
	raw, _ := json.Marshal(b)
	for _, leak := range []string{"mira", `"moss"`, `"potion"`, `"r1"`, "Not chosen"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("the pack carries %q: %s", leak, raw)
		}
	}
}

func TestPacksAreReadOnlyByThoseWhoMay(t *testing.T) {
	s, _, packs, _ := newMakingService()
	ctx := context.Background()
	body := packJSON(t, goodPack())
	packs.rows["here"] = PackRow{ID: "here", CampaignID: "c", Name: "Here", Body: body}
	packs.rows["mine"] = PackRow{ID: "mine", CampaignID: "other", Name: "Mine elsewhere", Body: body}
	packs.rows["shared"] = PackRow{ID: "shared", CampaignID: "stranger", Name: "Shared", Shared: true, Body: body}
	packs.rows["private"] = PackRow{ID: "private", CampaignID: "stranger", Name: "Private", Body: body}
	granted := Viewer{UserID: "granted", MemberRole: 1, VisibilityRole: 3, Director: true}
	tests := []struct {
		pack     string
		v        Viewer
		wantFrom string
		wantCode int
	}{
		{pack: "here", v: director, wantFrom: "here"},
		{pack: "mine", v: director, wantFrom: "campaign"},
		{pack: "shared", v: director, wantFrom: "server"},
		{pack: "private", v: director, wantCode: http.StatusNotFound},
		{pack: "missing", v: director, wantCode: http.StatusNotFound},
		{pack: "mine", v: granted, wantCode: http.StatusNotFound},
		{pack: "here", v: granted, wantFrom: "here"},
	}
	for _, tt := range tests {
		t.Run(tt.pack+" as "+tt.v.UserID, func(t *testing.T) {
			d, err := s.PackDetail(ctx, "c", tt.pack, tt.v)
			if tt.wantCode != 0 {
				if code(err) != tt.wantCode {
					t.Fatalf("got %v, want %d", err, tt.wantCode)
				}
				if _, err := s.ImportPack(ctx, "c", tt.pack, tt.v, ImportInput{}); code(err) != tt.wantCode {
					t.Fatalf("import got %v, want %d", err, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if d.From != tt.wantFrom || d.Matches["Bloodmoss"] != "moss" {
				t.Fatalf("from %q, matches %v", d.From, d.Matches)
			}
		})
	}
	list, err := s.ListPacks(ctx, "c", director)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range list {
		got = append(got, p.ID+":"+p.From)
	}
	if want := []string{"here:here", "mine:campaign", "shared:server"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("list = %v, want %v", got, want)
	}
	if _, err := s.UpdatePack(ctx, "c", "shared", director, PackChange{Name: "Taken over"}); code(err) != http.StatusNotFound {
		t.Fatalf("renaming another campaign's pack got %v, want 404", err)
	}
}

func TestImportPackArrivesAsDrafts(t *testing.T) {
	s, repo, packs, dir := newMakingService()
	ctx := context.Background()
	addRecipe(repo, Recipe{ID: "old", Name: "Herb paste", Goal: 5})
	packs.rows["p"] = PackRow{ID: "p", CampaignID: "other", Name: "Hedge alchemy", Body: packJSON(t, goodPack())}

	res, err := s.ImportPack(ctx, "c", "p", director, ImportInput{Matches: map[string]string{"Bloodmoss": "moss"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Made) != 1 || res.Made[0].Name != "Night tonic" {
		t.Fatalf("made %v", res.Made)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Name != "Herb paste" {
		t.Fatalf("skipped %v", res.Skipped)
	}
	// Ghost moss and the result are new Director-only items, with the pack's kinds.
	if len(res.Items) != 2 {
		t.Fatalf("new items %v", res.Items)
	}
	r := repo.recipes[res.Made[0].ID]
	if !r.Draft {
		t.Fatal("a brought-in recipe isn't a draft")
	}
	if r.Materials[0].ItemID != "moss" || dir.made[r.Materials[1].ItemID].Name != "Ghost moss" || !reflect.DeepEqual(repo.tags[r.Materials[1].ItemID], []string{"fungus"}) {
		t.Fatalf("materials %v", r.Materials)
	}

	// Matching a material to something that isn't an item skips the recipe.
	packs.rows["q"] = PackRow{ID: "q", CampaignID: "c", Name: "Again", Body: packJSON(t, func() packBody {
		b := goodPack()
		b.Recipes[0].Name = "Another tonic"
		b.Recipes = b.Recipes[:1]
		return b
	}())}
	res, err = s.ImportPack(ctx, "c", "q", director, ImportInput{Matches: map[string]string{"Bloodmoss": "mira"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Made) != 0 || len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Reason, "isn't an Armory item") {
		t.Fatalf("made %v skipped %v", res.Made, res.Skipped)
	}
}

func TestSaveAndUploadPacks(t *testing.T) {
	s, repo, packs, _ := newMakingService()
	ctx := context.Background()
	addRecipe(repo, Recipe{ID: "r1", Name: "Tonic", Materials: []Material{{ItemID: "moss", Qty: 1}}, Goal: 10})
	pv, err := s.SavePack(ctx, "c", director, SavePackInput{Name: "Mine", Recipes: []string{"r1"}, Shared: true})
	if err != nil {
		t.Fatal(err)
	}
	if !pv.Shared || pv.Count != 1 || pv.From != "here" || packs.rows[pv.ID].Source != "saved" {
		t.Fatalf("saved %+v", pv)
	}
	if _, err := s.SavePack(ctx, "c", director, SavePackInput{Name: "None", Recipes: []string{"elsewhere"}}); code(err) != http.StatusUnprocessableEntity {
		t.Fatalf("a pack of another campaign's recipe got %v, want 422", err)
	}
	raw, name, err := s.PackFile(ctx, "c", pv.ID, director)
	if err != nil {
		t.Fatal(err)
	}
	if name != "mine.recipes.json" {
		t.Fatalf("file name %q", name)
	}
	up, err := s.UploadPack(ctx, "c", director, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !up.File || up.Shared || up.Count != 1 {
		t.Fatalf("uploaded %+v", up)
	}
	if _, err := s.UploadPack(ctx, "c", director, make([]byte, maxPackBytes+1)); code(err) != http.StatusUnprocessableEntity {
		t.Fatalf("an oversized file got %v, want 422", err)
	}
}

func TestFileName(t *testing.T) {
	tests := map[string]string{"Hedge alchemy": "hedge-alchemy", "../../etc/passwd": "etcpasswd", "\"; rm": "rm", "": "recipe-pack", "Élan": "lan"}
	for in, want := range tests {
		if got := fileName(in); got != want {
			t.Errorf("fileName(%q) = %q, want %q", in, got, want)
		}
	}
}
