package homebrew

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

const camp = "camp-1"

var (
	dm     = Viewer{UserID: "dm-1", Director: true}
	player = Viewer{UserID: "jo-1"}
)

// fakeRepo is an in-memory Repository with the SQL one's semantics: names
// unique per campaign and updates guarded by the stored version.
type fakeRepo struct {
	recipes map[string]Recipe
	// err, when set, fails every call; loseRace makes Update report that
	// another writer got there first.
	err      error
	loseRace bool
	calls    int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{recipes: map[string]Recipe{}} }

func (f *fakeRepo) List(_ context.Context, c string) ([]Recipe, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := []Recipe{}
	for _, r := range f.recipes {
		if r.CampaignID == c {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *fakeRepo) Get(_ context.Context, c, id string) (Recipe, bool, error) {
	f.calls++
	if f.err != nil {
		return Recipe{}, false, f.err
	}
	r, ok := f.recipes[id]
	if !ok || r.CampaignID != c {
		return Recipe{}, false, nil
	}
	return r, true, nil
}

func (f *fakeRepo) Count(_ context.Context, c string) (int, error) {
	f.calls++
	if f.err != nil {
		return 0, f.err
	}
	n := 0
	for _, r := range f.recipes {
		if r.CampaignID == c {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) nameTaken(c, name, exceptID string) bool {
	for _, r := range f.recipes {
		if r.CampaignID == c && r.Name == name && r.ID != exceptID {
			return true
		}
	}
	return false
}

func (f *fakeRepo) Create(_ context.Context, r Recipe) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	if f.nameTaken(r.CampaignID, r.Name, "") {
		return errDuplicate
	}
	r.Version = 1
	r.CreatedAt = time.Unix(1000, 0)
	r.UpdatedAt = r.CreatedAt
	f.recipes[r.ID] = r
	return nil
}

func (f *fakeRepo) Update(_ context.Context, r Recipe, expected int) (bool, error) {
	f.calls++
	if f.err != nil {
		return false, f.err
	}
	if f.nameTaken(r.CampaignID, r.Name, r.ID) {
		return false, errDuplicate
	}
	cur, ok := f.recipes[r.ID]
	if f.loseRace || !ok || cur.Version != expected {
		return false, nil
	}
	cur.Name, cur.Doc, cur.Version = r.Name, r.Doc, cur.Version+1
	f.recipes[r.ID] = cur
	return true, nil
}

func (f *fakeRepo) Delete(_ context.Context, c, id string) (bool, error) {
	f.calls++
	if f.err != nil {
		return false, f.err
	}
	r, ok := f.recipes[id]
	if !ok || r.CampaignID != c {
		return false, nil
	}
	delete(f.recipes, id)
	return true, nil
}

func newTestService(t *testing.T) (Service, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	svc := NewService(repo, testRegistry(t)).(*service)
	n := 0
	svc.newID = func() string { n++; return fmt.Sprintf("rcp-%d", n) }
	return svc, repo
}

func in(name string) RecipeInput {
	return RecipeInput{Name: name, Doc: Doc{Parts: []Part{w("a")}, Rules: []Rule{rule("moon", "", "", Do{Action: "ping"})}}}
}

func TestServiceDirectorOnly(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, camp, dm, in("Seed")); err != nil {
		t.Fatal(err)
	}
	before := repo.calls
	tests := []struct {
		name string
		call func() error
	}{
		{"Catalog", func() error { _, err := svc.Catalog(player); return err }},
		{"List", func() error { _, err := svc.List(ctx, camp, player); return err }},
		{"Get", func() error { _, err := svc.Get(ctx, camp, "rcp-1", player); return err }},
		{"Create", func() error { _, err := svc.Create(ctx, camp, player, in("X")); return err }},
		{"Update", func() error { _, err := svc.Update(ctx, camp, "rcp-1", player, in("X")); return err }},
		{"Delete", func() error { return svc.Delete(ctx, camp, "rcp-1", player) }},
		{"zero-value viewer", func() error { _, err := svc.List(ctx, camp, Viewer{}); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codeOf(tt.call()); got != 403 {
				t.Errorf("code = %d, want 403", got)
			}
		})
	}
	if repo.calls != before {
		t.Error("a refused call still reached the repository")
	}
	if len(repo.recipes) != 1 {
		t.Errorf("recipes = %d, want the seed only", len(repo.recipes))
	}
}

func TestServiceCatalog(t *testing.T) {
	svc, _ := newTestService(t)
	c, err := svc.Catalog(dm)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Parts) == 0 || len(c.Events) == 0 || len(c.Actions) == 0 {
		t.Errorf("catalog is empty: %+v", c)
	}
}

func TestServiceList(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	t.Run("empty is a non-nil slice", func(t *testing.T) {
		got, err := svc.List(ctx, camp, dm)
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("got %v, %v", got, err)
		}
	})
	b := in("Beta")
	b.Doc.Rules = append(b.Doc.Rules, rule("moon", "", "", Do{Action: "ping"}))
	if _, err := svc.Create(ctx, camp, dm, b); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, camp, dm, in("Alpha")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "other-camp", dm, in("Elsewhere")); err != nil {
		t.Fatal(err)
	}
	got, err := svc.List(ctx, camp, dm)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "Alpha" || got[1].Name != "Beta" {
		t.Fatalf("list = %+v", got)
	}
	if got[0].Parts != 1 || got[0].Rules != 1 || got[1].Rules != 2 || got[1].Version != 1 || got[1].ID == "" {
		t.Errorf("summaries wrong: %+v", got)
	}
	t.Run("repo error", func(t *testing.T) {
		repo.err = errDB
		_, err := svc.List(ctx, camp, dm)
		assertInternal(t, err)
	})
}

// assertInternal checks err is a 500 whose client message hides the cause.
func assertInternal(t *testing.T, err error) {
	t.Helper()
	if codeOf(err) != 500 {
		t.Fatalf("code = %d (err %v), want 500", codeOf(err), err)
	}
	if strings.Contains(msgOf(err), "secret") || strings.Contains(msgOf(err), "db is down") {
		t.Errorf("client message leaks the cause: %q", msgOf(err))
	}
	if !errors.Is(err, errDB) {
		t.Error("the cause is lost for logging")
	}
}

func TestServiceGet(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	made, err := svc.Create(ctx, camp, dm, in("Alpha"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		campaign string
		id       string
		want     int
	}{
		{"found", camp, made.ID, 0},
		{"unknown id", camp, "rcp-999", 404},
		{"other campaign", "other", made.ID, 404},
		{"empty id", camp, "", 404},
		{"malformed id", camp, "a/b", 404},
		{"space in id", camp, "a b", 404},
		{"too long id", camp, strings.Repeat("a", MaxIDChars+1), 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := svc.Get(ctx, tt.campaign, tt.id, dm)
			if got := codeOf(err); got != tt.want {
				t.Fatalf("code = %d (err %v), want %d", got, err, tt.want)
			}
			if tt.want == 0 && (r == nil || r.Name != "Alpha") {
				t.Errorf("recipe = %+v", r)
			}
		})
	}
}

func TestServiceCreate(t *testing.T) {
	ctx := context.Background()
	t.Run("stores the normalised doc", func(t *testing.T) {
		svc, _ := newTestService(t)
		input := in("  Alpha  ")
		input.Doc.Parts[0].Label = "  Heat "
		r, err := svc.Create(ctx, camp, dm, input)
		if err != nil {
			t.Fatal(err)
		}
		if r.Name != "Alpha" || r.Doc.Parts[0].Label != "Heat" || r.Version != 1 || r.CreatedBy != "dm-1" || r.CampaignID != camp {
			t.Errorf("recipe = %+v", r)
		}
	})
	tests := []struct {
		name  string
		input RecipeInput
		setup func(*fakeRepo)
		want  int
	}{
		{"empty name", in(""), nil, 422},
		{"name too long", in(strings.Repeat("x", MaxNameChars+1)), nil, 422},
		{"invalid doc", RecipeInput{Name: "X", Doc: Doc{Parts: []Part{{Key: "A", Kind: "widget", Label: "A"}}}}, nil, 422},
		{"duplicate name", in("Alpha"), func(f *fakeRepo) {
			f.recipes["old"] = Recipe{ID: "old", CampaignID: camp, Name: "Alpha", Version: 1}
		}, 422},
		{"same name in another campaign is fine", in("Alpha"), func(f *fakeRepo) {
			f.recipes["old"] = Recipe{ID: "old", CampaignID: "other", Name: "Alpha", Version: 1}
		}, 0},
		{"at the recipe limit", in("New"), func(f *fakeRepo) { fill(f, MaxRecipes) }, 422},
		{"one below the limit", in("New"), func(f *fakeRepo) { fill(f, MaxRecipes-1) }, 0},
		{"repo error on create", in("New"), func(f *fakeRepo) { f.err = errDB }, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestService(t)
			if tt.setup != nil {
				tt.setup(repo)
			}
			_, err := svc.Create(ctx, camp, dm, tt.input)
			if got := codeOf(err); got != tt.want {
				t.Fatalf("code = %d (err %v), want %d", got, err, tt.want)
			}
			if tt.want == 500 {
				assertInternal(t, err)
			}
		})
	}
	t.Run("duplicate message names the recipe", func(t *testing.T) {
		svc, _ := newTestService(t)
		if _, err := svc.Create(ctx, camp, dm, in("Alpha")); err != nil {
			t.Fatal(err)
		}
		_, err := svc.Create(ctx, camp, dm, in("Alpha"))
		if !strings.Contains(msgOf(err), "Alpha") {
			t.Errorf("message = %q", msgOf(err))
		}
	})
}

func fill(f *fakeRepo, n int) {
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("fill-%d", i)
		f.recipes[id] = Recipe{ID: id, CampaignID: camp, Name: id, Version: 1}
	}
}

func TestServiceUpdate(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) (Service, *fakeRepo, string) {
		svc, repo := newTestService(t)
		r, err := svc.Create(ctx, camp, dm, in("Alpha"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Create(ctx, camp, dm, in("Taken")); err != nil {
			t.Fatal(err)
		}
		return svc, repo, r.ID
	}
	t.Run("success bumps the version", func(t *testing.T) {
		svc, _, id := setup(t)
		input := in("Renamed")
		input.Version = 1
		r, err := svc.Update(ctx, camp, id, dm, input)
		if err != nil {
			t.Fatal(err)
		}
		if r.Name != "Renamed" || r.Version != 2 {
			t.Errorf("recipe = %+v", r)
		}
	})
	tests := []struct {
		name   string
		id     string // empty = the created one
		camp   string
		mutate func(*RecipeInput)
		repo   func(*fakeRepo)
		want   int
	}{
		{"stale version", "", camp, func(i *RecipeInput) { i.Version = 7 }, nil, 409},
		{"zero version", "", camp, func(i *RecipeInput) { i.Version = 0 }, nil, 409},
		{"lost race", "", camp, nil, func(f *fakeRepo) { f.loseRace = true }, 409},
		{"unknown id", "nope", camp, nil, nil, 404},
		{"malformed id", "a/b", camp, nil, nil, 404},
		{"other campaign", "", "other", nil, nil, 404},
		{"duplicate name", "", camp, func(i *RecipeInput) { i.Name = "Taken" }, nil, 422},
		{"invalid doc", "", camp, func(i *RecipeInput) { i.Doc.Parts[0].Key = "BAD" }, nil, 422},
		{"empty name", "", camp, func(i *RecipeInput) { i.Name = "" }, nil, 422},
		{"repo error", "", camp, nil, func(f *fakeRepo) { f.err = errDB }, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, id := setup(t)
			if tt.id != "" {
				id = tt.id
			}
			input := in("Alpha")
			input.Version = 1
			if tt.mutate != nil {
				tt.mutate(&input)
			}
			if tt.repo != nil {
				tt.repo(repo)
			}
			_, err := svc.Update(ctx, tt.camp, id, dm, input)
			if got := codeOf(err); got != tt.want {
				t.Fatalf("code = %d (err %v), want %d", got, err, tt.want)
			}
			if tt.want == 500 {
				assertInternal(t, err)
			}
		})
	}
	t.Run("a failed update leaves the stored recipe alone", func(t *testing.T) {
		svc, repo, id := setup(t)
		input := in("Changed")
		input.Version = 99
		_, _ = svc.Update(ctx, camp, id, dm, input)
		if got := repo.recipes[id]; got.Name != "Alpha" || got.Version != 1 {
			t.Errorf("stored = %+v", got)
		}
	})
}

func TestServiceDelete(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestService(t)
	r, err := svc.Create(ctx, camp, dm, in("Alpha"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		campaign string
		id       string
		err      error
		want     int
	}{
		{"missing", camp, "rcp-999", nil, 404},
		{"malformed", camp, "a/b", nil, 404},
		{"other campaign", "other", r.ID, nil, 404},
		{"repo error", camp, r.ID, errDB, 500},
		{"found", camp, r.ID, nil, 0},
		{"already gone", camp, r.ID, nil, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo.err = tt.err
			err := svc.Delete(ctx, tt.campaign, tt.id, dm)
			if got := codeOf(err); got != tt.want {
				t.Fatalf("code = %d (err %v), want %d", got, err, tt.want)
			}
			if tt.want == 500 {
				assertInternal(t, err)
			}
		})
	}
}
