package crafting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// ---------- Fakes ----------

type fakeRepo struct {
	settings *Settings
	recipes  map[string]Recipe
	order    []string
	tags     map[string][]string
	benches  map[string]Bench
	kindPics map[string]MaterialPicture
	itemPics map[string]MaterialPicture
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{recipes: map[string]Recipe{}, tags: map[string][]string{}, benches: map[string]Bench{},
		kindPics: map[string]MaterialPicture{}, itemPics: map[string]MaterialPicture{}}
}

func (f *fakeRepo) GetSettings(context.Context, string) (Settings, error) {
	if f.settings == nil {
		return DefaultSettings(), nil
	}
	return *f.settings, nil
}
func (f *fakeRepo) SaveSettings(_ context.Context, _ string, s Settings) error {
	f.settings = &s
	return nil
}
func (f *fakeRepo) ListRecipes(context.Context, string) ([]Recipe, error) {
	out := []Recipe{}
	for _, id := range f.order {
		if r, ok := f.recipes[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeRepo) GetRecipe(_ context.Context, _, id string) (Recipe, bool, error) {
	r, ok := f.recipes[id]
	return r, ok, nil
}
func (f *fakeRepo) CreateRecipe(_ context.Context, _, _ string, r Recipe) error {
	for _, x := range f.recipes {
		if x.Name == r.Name {
			return errDuplicate
		}
	}
	r.Version = 1
	f.recipes[r.ID] = r
	f.order = append(f.order, r.ID)
	return nil
}
func (f *fakeRepo) UpdateRecipe(_ context.Context, _, _ string, r Recipe, expected int) (bool, error) {
	old, ok := f.recipes[r.ID]
	if !ok || old.Version != expected {
		return false, nil
	}
	r.Rules, r.Looks, r.Draft, r.Version = old.Rules, old.Looks, old.Draft, old.Version+1
	f.recipes[r.ID] = r
	return true, nil
}
func (f *fakeRepo) UpdateRules(_ context.Context, _, id string, rules []Rule, looks map[string]*Look, expected int) (bool, error) {
	r, ok := f.recipes[id]
	if !ok || r.Version != expected {
		return false, nil
	}
	r.Rules, r.Looks, r.Version = rules, looks, r.Version+1
	f.recipes[id] = r
	return true, nil
}
func (f *fakeRepo) SetDraft(_ context.Context, _, id string, draft bool) error {
	r := f.recipes[id]
	r.Draft, r.Version = draft, r.Version+1
	f.recipes[id] = r
	return nil
}
func (f *fakeRepo) DeleteRecipe(_ context.Context, _, id string) error {
	delete(f.recipes, id)
	return nil
}
func (f *fakeRepo) KnownBy(_ context.Context, _ string, ids []string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, cid := range ids {
		for _, id := range f.order {
			for _, k := range f.recipes[id].Known {
				if k == cid {
					out[cid] = append(out[cid], id)
				}
			}
		}
	}
	return out, nil
}
func (f *fakeRepo) ListTags(context.Context, string) (map[string][]string, error) { return f.tags, nil }
func (f *fakeRepo) SetTags(_ context.Context, _, itemID string, tags []string) error {
	f.tags[itemID] = tags
	return nil
}
func (f *fakeRepo) ListKindPictures(context.Context, string) (map[string]MaterialPicture, error) {
	return f.kindPics, nil
}
func (f *fakeRepo) SetKindPictures(_ context.Context, _ string, set map[string]*MaterialPicture) error {
	for k, p := range set {
		if p == nil {
			delete(f.kindPics, k)
		} else {
			f.kindPics[k] = *p
		}
	}
	return nil
}
func (f *fakeRepo) ListItemPictures(context.Context, string) (map[string]MaterialPicture, error) {
	return f.itemPics, nil
}
func (f *fakeRepo) SetItemPicture(_ context.Context, _, itemID string, p *MaterialPicture) error {
	if p == nil {
		delete(f.itemPics, itemID)
	} else {
		f.itemPics[itemID] = *p
	}
	return nil
}
func (f *fakeRepo) ListBenches(context.Context, string) ([]Bench, error) {
	out := []Bench{}
	for _, b := range f.benches {
		out = append(out, b)
	}
	return out, nil
}
func (f *fakeRepo) GetBench(_ context.Context, _, id string) (Bench, bool, error) {
	b, ok := f.benches[id]
	return b, ok, nil
}
func (f *fakeRepo) CreateBench(_ context.Context, _ string, b Bench) error {
	for _, x := range f.benches {
		if x.Name == b.Name {
			return errDuplicate
		}
	}
	b.Version = 1
	f.benches[b.ID] = b
	return nil
}
func (f *fakeRepo) UpdateBench(_ context.Context, _ string, b Bench, expected int) (bool, error) {
	old, ok := f.benches[b.ID]
	if !ok || old.Version != expected {
		return false, nil
	}
	old.Name, old.Kind, old.Version = b.Name, b.Kind, old.Version+1
	f.benches[b.ID] = old
	return true, nil
}
func (f *fakeRepo) SaveArrangement(_ context.Context, _, id, skin string, state json.RawMessage, expected int) (bool, error) {
	b, ok := f.benches[id]
	if !ok || b.Version != expected {
		return false, nil
	}
	if skin == "desk" {
		b.Desk = state
	} else {
		b.Journal = state
	}
	b.Version++
	f.benches[id] = b
	return true, nil
}
func (f *fakeRepo) DeleteBench(_ context.Context, _, id string) error {
	delete(f.benches, id)
	return nil
}

// fakeDir is a campaign with three items, two characters and one item the
// players cannot see. Mira belongs to the player.
type fakeDir struct{ lookups int }

var dirEntities = map[string]EntityInfo{
	"moss":   {ID: "moss", Name: "Bloodmoss", IsItem: true},
	"water":  {ID: "water", Name: "Spring water", IsItem: true},
	"potion": {ID: "potion", Name: "Healing potion", IsItem: true},
	"secret": {ID: "secret", Name: "Dragon's tear", IsItem: true},
	"mira":   {ID: "mira", Name: "Mira", IsCharacter: true},
	"tobin":  {ID: "tobin", Name: "Tobin", IsCharacter: true},
}

func (d *fakeDir) Entity(_ context.Context, _, id string) (*EntityInfo, error) {
	d.lookups++
	if e, ok := dirEntities[id]; ok {
		return &e, nil
	}
	return nil, nil
}
func (d *fakeDir) ListItems(_ context.Context, _ string, role int, _ string) ([]Named, bool, error) {
	out := []Named{{"moss", "Bloodmoss"}, {"water", "Spring water"}, {"potion", "Healing potion"}}
	if role >= 3 {
		out = append(out, Named{"secret", "Dragon's tear"})
	}
	return out, false, nil
}
func (d *fakeDir) ListCharacters(context.Context, string, int, string) ([]Named, bool, error) {
	return []Named{{"mira", "Mira"}, {"tobin", "Tobin"}}, false, nil
}
func (d *fakeDir) OwnedCharacterIDs(_ context.Context, _, userID string) (map[string]bool, error) {
	if userID == "player" {
		return map[string]bool{"mira": true}, nil
	}
	return map[string]bool{}, nil
}

type fakeRules struct{}

func (fakeRules) CraftingRules(context.Context, string) (systems.CraftingRules, string) {
	r := systems.DefaultCraftingRules()
	r.Characteristics = []systems.CraftingStat{{Key: "reason", Label: "Reason"}}
	return r, "Test system"
}

var (
	director = Viewer{UserID: "dm", MemberRole: 3, VisibilityRole: 3, Director: true}
	player   = Viewer{UserID: "player", MemberRole: 1, VisibilityRole: 1}
)

func newTestService() (*service, *fakeRepo, *fakeDir) {
	repo, dir := newFakeRepo(), &fakeDir{}
	n := 0
	s := &service{repo: repo, dir: dir, rules: fakeRules{}, newID: func() string { n++; return fmt.Sprintf("id-%d", n) }}
	return s, repo, dir
}

func code(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

// ---------- Tests ----------

func TestEveryWriteIsDirectorsOnly(t *testing.T) {
	s, repo, _ := newTestService()
	ctx := context.Background()
	repo.recipes["r1"] = Recipe{ID: "r1", Name: "x", Version: 1}
	repo.order = []string{"r1"}
	repo.benches["b1"] = Bench{ID: "b1", Name: "Bench", Kind: "apothecary", Version: 1}
	calls := map[string]func() error{
		"settings": func() error { _, err := s.SaveSettings(ctx, "c", player, DefaultSettings()); return err },
		"catalog":  func() error { _, err := s.Catalog(ctx, "c", player); return err },
		"list":     func() error { _, err := s.ListRecipes(ctx, "c", player); return err },
		"get":      func() error { _, err := s.GetRecipe(ctx, "c", "r1", player); return err },
		"create":   func() error { _, err := s.CreateRecipe(ctx, "c", player, RecipeInput{Name: "y"}); return err },
		"update": func() error {
			_, err := s.UpdateRecipe(ctx, "c", "r1", player, RecipeInput{Name: "y", Version: 1})
			return err
		},
		"rules":     func() error { _, err := s.SaveRules(ctx, "c", "r1", player, RulesInput{Version: 1}); return err },
		"approve":   func() error { _, err := s.Approve(ctx, "c", "r1", player); return err },
		"delete":    func() error { return s.DeleteRecipe(ctx, "c", "r1", player) },
		"tags":      func() error { _, err := s.SetTags(ctx, "c", "moss", player, []string{"herb"}); return err },
		"benches":   func() error { _, err := s.ListBenches(ctx, "c", player); return err },
		"bench":     func() error { _, err := s.GetBench(ctx, "c", "b1", player); return err },
		"new bench": func() error { _, err := s.CreateBench(ctx, "c", player, BenchInput{Name: "z"}); return err },
		"rename": func() error {
			_, err := s.UpdateBench(ctx, "c", "b1", player, BenchInput{Name: "z", Version: 1})
			return err
		},
		"arrangement": func() error {
			_, err := s.SaveArrangement(ctx, "c", "b1", "desk", player, ArrangementInput{State: json.RawMessage(`{}`), Version: 1})
			return err
		},
		"drop bench": func() error { return s.DeleteBench(ctx, "c", "b1", player) },
		"kind pictures": func() error {
			_, err := s.SetKindPictures(ctx, "c", player, KindPicturesInput{Kinds: map[string]*MaterialPicture{"herb": {Picture: "moss", Tint: "#b3263a"}}})
			return err
		},
		"item picture": func() error {
			_, err := s.SetItemPicture(ctx, "c", "moss", player, &MaterialPicture{Picture: "moss", Tint: "#b3263a"})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); code(err) != http.StatusForbidden {
				t.Fatalf("a player got %v, want 403", err)
			}
		})
	}
	if _, ok := repo.recipes["r1"]; !ok || len(repo.benches) != 1 || repo.settings != nil || len(repo.tags) != 0 ||
		len(repo.kindPics) != 0 || len(repo.itemPics) != 0 {
		t.Fatal("a refused call still changed something")
	}
}

func TestLibrary(t *testing.T) {
	s, _, _ := newTestService()
	lib, err := s.Library(context.Background(), "c", player)
	if err != nil {
		t.Fatal(err)
	}
	if lib.Director || lib.System != "Test system" || len(lib.Rules.Characteristics) != 1 || len(lib.Tags) != len(Tags) || len(lib.Pictures) != len(Pictures) || lib.Settings != DefaultSettings() {
		t.Fatalf("library = %+v", lib)
	}
}

func TestRecipeLifecycle(t *testing.T) {
	s, repo, dir := newTestService()
	ctx := context.Background()
	in := RecipeInput{Name: "Healing potion", Materials: []Material{{ItemID: "moss", Qty: 1}, {Tag: "liquid", Qty: 1}},
		Steps:  []Step{{T: "add", Material: idx(0), To: "mortar"}, {T: "grind"}, {T: "tip"}, {T: "pour", Material: idx(1)}},
		Result: Result{ItemID: "potion", Qty: 1}, Goal: 30, Stat: "Reason", Who: "learned", Known: []string{"mira"}}
	r, err := s.CreateRecipe(ctx, "c", director, in)
	if err != nil {
		t.Fatal(err)
	}
	if r.Stat != "reason" || r.Version != 1 || len(r.Rules) != 0 || r.Looks == nil {
		t.Fatalf("created %+v", r)
	}
	if _, err := s.CreateRecipe(ctx, "c", director, in); code(err) != http.StatusConflict || !strings.Contains(err.Error(), "already a recipe") {
		t.Fatalf("a second recipe with the name: %v", err)
	}
	bad := in
	bad.Name, bad.Result.ItemID = "Other", "nobody"
	if _, err := s.CreateRecipe(ctx, "c", director, bad); code(err) != http.StatusBadRequest && code(err) != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown item: %v", err)
	}

	// The item is deleted from the Armory; the recipe still saves with it.
	delete(dirEntities, "moss")
	defer func() { dirEntities["moss"] = EntityInfo{ID: "moss", Name: "Bloodmoss", IsItem: true} }()
	in.Version, in.Goal = r.Version, 45
	before := dir.lookups
	r2, err := s.UpdateRecipe(ctx, "c", r.ID, director, in)
	if err != nil {
		t.Fatalf("saving a recipe whose item was deleted: %v", err)
	}
	if r2.Goal != 45 || r2.Version != 2 {
		t.Fatalf("updated %+v", r2)
	}
	if dir.lookups != before {
		t.Errorf("a save looked up %d pages it already held", dir.lookups-before)
	}
	if _, err := s.UpdateRecipe(ctx, "c", r.ID, director, in); code(err) != http.StatusConflict {
		t.Fatalf("a stale version: %v", err)
	}

	rules := json.RawMessage(`[{"when":[{"kind":"moon","moon":"Pale","phase":"full"}],"then":{"kind":"edge"},"players":"secret","look":{"spots":{}}},{"when":[{"kind":"zodiac"}],"then":{"kind":"edge"}}]`)
	r3, err := s.SaveRules(ctx, "c", r.ID, director, RulesInput{Rules: rules, Looks: json.RawMessage(`{"success":{"spots":{"result":{"look":"bloom"}}}}`), Version: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(r3.Rules) != 1 || r3.Rules[0].Look != nil || r3.Looks["success"] == nil || r3.Version != 3 {
		t.Fatalf("rules saved as %+v / %+v", r3.Rules, r3.Looks)
	}
	if _, err := s.SaveRules(ctx, "c", r.ID, director, RulesInput{Rules: json.RawMessage(`{"when":[]}`), Version: 3}); err == nil {
		t.Fatal("rules that are not a list were accepted")
	}
	if repo.recipes[r.ID].Goal != 45 {
		t.Fatal("saving the rules changed the recipe")
	}

	if err := s.DeleteRecipe(ctx, "c", r.ID, director); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRecipe(ctx, "c", r.ID, director); code(err) != http.StatusNotFound {
		t.Fatalf("a deleted recipe: %v", err)
	}
}

func TestBookShowsOnlyWhatThePlayerMayRead(t *testing.T) {
	s, repo, _ := newTestService()
	ctx := context.Background()
	add := func(r Recipe) { repo.recipes[r.ID] = r; repo.order = append(repo.order, r.ID) }
	add(Recipe{ID: "open", Name: "Open", Who: "all", Bench: "apothecary", Goal: 10, Result: Result{ItemID: "secret", Qty: 1},
		Materials: []Material{{ItemID: "secret", Qty: 1}, {Tag: "herb", Qty: 2}}, Steps: []Step{{T: "add", Material: idx(0), To: "flask"}},
		Rules: []Rule{
			{When: []Cond{{Kind: "always"}}, Then: Effect{Kind: "edge"}, Players: "book", BookLine: "Brew it at dawn."},
			{When: []Cond{{Kind: "always"}}, Then: Effect{Kind: "mishap", Table: "Spills"}, Players: "secret"},
			{When: []Cond{{Kind: "always"}}, Then: Effect{Kind: "bane"}, Players: "book", Draft: true},
		}})
	add(Recipe{ID: "mira", Name: "Learned by Mira", Who: "learned", Known: []string{"mira"}, Bench: "apothecary", Goal: 10, Result: Result{Qty: 1}})
	add(Recipe{ID: "tobin", Name: "Learned by Tobin", Who: "learned", Known: []string{"tobin"}, Bench: "apothecary", Goal: 10, Result: Result{Qty: 1}})
	add(Recipe{ID: "draft", Name: "A draft", Who: "all", Draft: true, Bench: "apothecary", Goal: 10, Result: Result{Qty: 1}})

	pages, err := s.Book(ctx, "c", player)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range pages {
		names = append(names, p.Name)
	}
	if strings.Join(names, ",") != "Open,Learned by Mira" {
		t.Fatalf("the player's book holds %v", names)
	}
	open := pages[0]
	if open.Makes != "an item you haven’t seen" || open.Needs[0].Name != "an item you haven’t seen" || open.Needs[1].Name != "any herb" {
		t.Errorf("the book names a hidden item: %+v", open)
	}
	if strings.Join(open.Lines, "|") != "Brew it at dawn." {
		t.Errorf("the book shows %v; secret and draft rules must not appear", open.Lines)
	}
	if strings.Contains(fmt.Sprint(open.Method), "Dragon") {
		t.Errorf("the method names a hidden item: %v", open.Method)
	}
	if got := strings.Join(pages[1].KnownBy, ","); got != "Mira" {
		t.Errorf("known by %q, want only the player's own character", got)
	}

	pages, err = s.Book(ctx, "c", director)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 || pages[0].Makes != "Dragon's tear" {
		t.Fatalf("the Director's book: %d pages, makes %q", len(pages), pages[0].Makes)
	}
}

func TestTagsAndCatalog(t *testing.T) {
	s, repo, _ := newTestService()
	ctx := context.Background()
	got, err := s.SetTags(ctx, "c", "moss", director, []string{"herb", "metal"})
	if err != nil || strings.Join(got, ",") != "metal,herb" {
		t.Fatalf("SetTags = %v, %v", got, err)
	}
	if _, err := s.SetTags(ctx, "c", "mira", director, []string{"herb"}); code(err) != http.StatusNotFound {
		t.Fatalf("tagging a character: %v", err)
	}
	if _, err := s.SetTags(ctx, "c", "moss", director, []string{"dragon"}); err == nil {
		t.Fatal("an unknown tag was saved")
	}
	repo.tags["water"] = []string{"liquid", "bogus"}
	cat, err := s.Catalog(ctx, "c", director)
	if err != nil {
		t.Fatal(err)
	}
	tags := map[string]string{}
	for _, it := range cat.Items {
		tags[it.ID] = strings.Join(it.Tags, ",")
	}
	if tags["moss"] != "metal,herb" || tags["potion"] != "" || len(cat.Characters) != 2 {
		t.Fatalf("catalog = %+v", cat)
	}
	if tags["water"] != "liquid" {
		t.Errorf("a retired tag hid the item's others: %q", tags["water"])
	}
}

func TestBenches(t *testing.T) {
	s, _, _ := newTestService()
	ctx := context.Background()
	b, err := s.CreateBench(ctx, "c", director, BenchInput{Name: " The herbalist's bench "})
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "The herbalist's bench" || b.Kind != "apothecary" {
		t.Fatalf("created %+v", b)
	}
	if _, err := s.CreateBench(ctx, "c", director, BenchInput{Name: "Forge", Kind: "forge"}); err == nil {
		t.Fatal("a bench kind the engine cannot draw was accepted")
	}
	if _, err := s.CreateBench(ctx, "c", director, BenchInput{Name: "The herbalist's bench"}); code(err) != http.StatusConflict {
		t.Fatalf("a second bench with the name: %v", err)
	}
	b2, err := s.SaveArrangement(ctx, "c", b.ID, "desk", director, ArrangementInput{State: json.RawMessage(`{"look":{"wall":"stone"},"pieces":{"cat":{"name":"Pepper"}}}`), Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b2.Desk), `"Pepper"`) || b2.Version != 2 {
		t.Fatalf("arrangement saved as %s (v%d)", b2.Desk, b2.Version)
	}
	if _, err := s.SaveArrangement(ctx, "c", b.ID, "desk", director, ArrangementInput{State: json.RawMessage(`{}`), Version: 1}); code(err) != http.StatusConflict {
		t.Fatalf("a stale arrangement: %v", err)
	}
	if _, err := s.SaveArrangement(ctx, "c", b.ID, "forge", director, ArrangementInput{State: json.RawMessage(`{}`), Version: 2}); code(err) != http.StatusNotFound {
		t.Fatalf("an unknown look: %v", err)
	}
	if _, err := s.UpdateBench(ctx, "c", b.ID, director, BenchInput{Name: "Renamed", Version: 2}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteBench(ctx, "c", b.ID, director); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBench(ctx, "c", b.ID, director); code(err) != http.StatusNotFound {
		t.Fatalf("a deleted bench: %v", err)
	}
}
