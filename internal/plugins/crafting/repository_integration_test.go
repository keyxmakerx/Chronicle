package crafting

// Row-level tests for the crafting tables: versioned writes that refuse a
// stale version, unique names, the known list, and what happens to recipes,
// tags and knowers when an Armory page is deleted. Only the database can
// show these. Each test gets its own migrated scratch schema and skips
// (never fails) when no server is reachable.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/crafting/ -run TestRepoIntegration

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
)

func newScratchDB(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN (tools/start-test-db.sh) to run the row-level tests")
	}
	cfg, err := mysql.ParseDSN(raw)
	if err != nil {
		t.Skipf("invalid CHRONICLE_TEST_DB_DSN: %v", err)
	}
	cfg.ParseTime = true
	serverCfg := *cfg
	serverCfg.DBName = ""
	admin, err := sql.Open("mysql", serverCfg.FormatDSN())
	if err != nil {
		t.Skipf("no test DB: %v", err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Skipf("no test DB reachable: %v", err)
	}
	name := fmt.Sprintf("chronicle_crafting_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })
	scratch := *cfg
	scratch.DBName = name
	db, err := sql.Open("mysql", scratch.FormatDSN())
	if err != nil {
		t.Fatalf("open scratch: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	if err := database.RunMigrations(db, scratch.FormatDSN(), filepath.Join(root, "db", "migrations")); err != nil {
		t.Skipf("core migrations did not apply: %v", err)
	}
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{{Slug: PluginSlug, MigrationsFS: sub}}) {
		if !res.Healthy {
			t.Fatalf("crafting migrations did not apply: %v", res.Error)
		}
	}
	return db
}

func newUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type repoFixture struct {
	db       *sql.DB
	repo     Repository
	user     string
	campaign string
	other    string
	typeID   int64
}

func newRepoFixture(t *testing.T) *repoFixture {
	db := newScratchDB(t)
	f := &repoFixture{db: db, repo: NewRepository(db), user: newUUID(t), campaign: newUUID(t), other: newUUID(t)}
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, f.user, f.user+"@example.test", "Ana", "x"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	for _, c := range []string{f.campaign, f.other} {
		if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, c, "Table", c, f.user); err != nil {
			t.Fatalf("seed campaign: %v", err)
		}
	}
	res, err := db.Exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural, icon, color) VALUES (?,?,?,?,?,?)`, f.campaign, "item", "Item", "Items", "fa-box", "#000000")
	if err != nil {
		t.Fatalf("seed type: %v", err)
	}
	f.typeID, _ = res.LastInsertId()
	return f
}

func (f *repoFixture) entity(t *testing.T, name string) string {
	t.Helper()
	id := newUUID(t)
	if _, err := f.db.Exec(`INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, created_by, created_at, updated_at) VALUES (?,?,?,?,?,?,NOW(),NOW())`,
		id, f.campaign, f.typeID, name, "s-"+id[:8], f.user); err != nil {
		t.Fatalf("seed entity: %v", err)
	}
	return id
}

func TestRepoIntegration_Settings(t *testing.T) {
	f := newRepoFixture(t)
	ctx := context.Background()
	got, err := f.repo.GetSettings(ctx, f.campaign)
	if err != nil || got != DefaultSettings() {
		t.Fatalf("before any save: %+v, %v", got, err)
	}
	want := Settings{Skin: "journal", Difficulty: "hard", Bane: false, Skip: false}
	for i := 0; i < 2; i++ { // the second save updates in place
		if err := f.repo.SaveSettings(ctx, f.campaign, want); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := f.repo.GetSettings(ctx, f.campaign); got != want {
		t.Fatalf("saved %+v, read %+v", want, got)
	}
	if got, _ := f.repo.GetSettings(ctx, f.other); got != DefaultSettings() {
		t.Fatal("one campaign's settings leaked into another's")
	}
}

func TestRepoIntegration_Recipes(t *testing.T) {
	f := newRepoFixture(t)
	ctx := context.Background()
	moss, potion, mira := f.entity(t, "Bloodmoss"), f.entity(t, "Healing potion"), f.entity(t, "Mira")
	r := Recipe{ID: newUUID(t), Name: "Healing potion", Template: "steady", Bench: "apothecary",
		Materials: []Material{{ItemID: moss, Qty: 1}, {Tag: "liquid", Qty: 2}}, Steps: []Step{{T: "add", Material: idx(0), To: "mortar"}, {T: "grind", Turns: 3}},
		Result: Result{ItemID: potion, Qty: 1}, Goal: 30, Stat: "reason", Cost: Cost{Amount: 12.5, Currency: "gp"}, Who: "learned", Known: []string{mira},
		Colors: Colors{Final: "#ff4d6d"}, Rules: []Rule{}, Looks: map[string]*Look{}}
	if err := f.repo.CreateRecipe(ctx, f.campaign, f.user, r); err != nil {
		t.Fatal(err)
	}
	dup := r
	dup.ID = newUUID(t)
	if err := f.repo.CreateRecipe(ctx, f.campaign, f.user, dup); !errors.Is(err, errDuplicate) {
		t.Fatalf("a second recipe with the name: %v", err)
	}
	dup.Known = nil
	if err := f.repo.CreateRecipe(ctx, f.other, f.user, dup); err != nil {
		t.Fatalf("the same name in another campaign: %v", err)
	}
	got, ok, err := f.repo.GetRecipe(ctx, f.campaign, r.ID)
	if err != nil || !ok {
		t.Fatalf("get: %v %v", ok, err)
	}
	if got.Version != 1 || got.Cost.Amount != 12.5 || got.Materials[1].Tag != "liquid" || *got.Steps[0].Material != 0 || got.Colors.Final != "#ff4d6d" ||
		strings.Join(got.Known, ",") != mira || got.Result.ItemID != potion {
		t.Fatalf("read back %+v", got)
	}
	if _, ok, _ := f.repo.GetRecipe(ctx, f.other, r.ID); ok {
		t.Fatal("a recipe was read through another campaign")
	}

	got.Goal = 45
	got.Known = []string{}
	if ok, err := f.repo.UpdateRecipe(ctx, f.campaign, f.user, got, 1); !ok || err != nil {
		t.Fatalf("update: %v %v", ok, err)
	}
	if ok, _ := f.repo.UpdateRecipe(ctx, f.campaign, f.user, got, 1); ok {
		t.Fatal("a stale version was written")
	}
	rules := []Rule{{When: []Cond{{Kind: "always"}}, Then: Effect{Kind: "edge"}, Players: "book"}}
	if ok, err := f.repo.UpdateRules(ctx, f.campaign, r.ID, rules, map[string]*Look{"success": {Spots: map[string]LookSpot{}}}, 2); !ok || err != nil {
		t.Fatalf("rules: %v %v", ok, err)
	}
	if err := f.repo.SetDraft(ctx, f.campaign, r.ID, true); err != nil {
		t.Fatal(err)
	}
	got, _, _ = f.repo.GetRecipe(ctx, f.campaign, r.ID)
	if got.Goal != 45 || len(got.Known) != 0 || len(got.Rules) != 1 || got.Looks["success"] == nil || !got.Draft || got.Version != 4 {
		t.Fatalf("after updates %+v", got)
	}

	// The item it makes is deleted: the recipe stays, asking for a new one.
	got.Known = []string{mira}
	if ok, _ := f.repo.UpdateRecipe(ctx, f.campaign, f.user, got, 4); !ok {
		t.Fatal("re-adding the knower")
	}
	if known, _ := f.repo.KnownBy(ctx, f.campaign, []string{mira}); strings.Join(known[mira], ",") != r.ID {
		t.Fatalf("known by Mira: %v", known)
	}
	if _, err := f.db.Exec(`DELETE FROM entities WHERE id IN (?, ?)`, potion, mira); err != nil {
		t.Fatal(err)
	}
	got, _, _ = f.repo.GetRecipe(ctx, f.campaign, r.ID)
	if got.Result.ItemID != "" || len(got.Known) != 0 || got.Materials[0].ItemID != moss {
		t.Fatalf("after deleting pages: result %q, known %v, materials %+v", got.Result.ItemID, got.Known, got.Materials)
	}

	list, err := f.repo.ListRecipes(ctx, f.campaign)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %d, %v", len(list), err)
	}
	if err := f.repo.DeleteRecipe(ctx, f.campaign, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := f.repo.GetRecipe(ctx, f.campaign, r.ID); ok {
		t.Fatal("deleted recipe still there")
	}
}

func TestRepoIntegration_TagsAndBenches(t *testing.T) {
	f := newRepoFixture(t)
	ctx := context.Background()
	moss := f.entity(t, "Bloodmoss")
	if err := f.repo.SetTags(ctx, f.campaign, moss, []string{"herb", "magic_essence"}); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.SetTags(ctx, f.campaign, moss, []string{"herb"}); err != nil {
		t.Fatal(err)
	}
	if tags, _ := f.repo.ListTags(ctx, f.campaign); strings.Join(tags[moss], ",") != "herb" {
		t.Fatalf("tags replaced as %v", tags)
	}
	if _, err := f.db.Exec(`DELETE FROM entities WHERE id = ?`, moss); err != nil {
		t.Fatal(err)
	}
	if tags, _ := f.repo.ListTags(ctx, f.campaign); len(tags) != 0 {
		t.Fatalf("a deleted item kept its tags: %v", tags)
	}

	b := Bench{ID: newUUID(t), Name: "Herbalist's bench", Kind: "apothecary"}
	if err := f.repo.CreateBench(ctx, f.campaign, b); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.CreateBench(ctx, f.campaign, Bench{ID: newUUID(t), Name: b.Name, Kind: "apothecary"}); !errors.Is(err, errDuplicate) {
		t.Fatalf("a second bench with the name: %v", err)
	}
	state := json.RawMessage(`{"look":{"wall":"stone"},"pieces":{}}`)
	if ok, err := f.repo.SaveArrangement(ctx, f.campaign, b.ID, "journal", state, 1); !ok || err != nil {
		t.Fatalf("arrangement: %v %v", ok, err)
	}
	if ok, _ := f.repo.SaveArrangement(ctx, f.campaign, b.ID, "desk", state, 1); ok {
		t.Fatal("a stale arrangement was written")
	}
	b.Name = "Renamed"
	if ok, err := f.repo.UpdateBench(ctx, f.campaign, b, 2); !ok || err != nil {
		t.Fatalf("rename: %v %v", ok, err)
	}
	got, ok, err := f.repo.GetBench(ctx, f.campaign, b.ID)
	if err != nil || !ok || got.Name != "Renamed" || got.Version != 3 || string(got.Journal) != string(state) || got.Desk != nil {
		t.Fatalf("bench read back %+v (desk %s, journal %s)", got, got.Desk, got.Journal)
	}
	if _, ok, _ := f.repo.GetBench(ctx, f.other, b.ID); ok {
		t.Fatal("a bench was read through another campaign")
	}
	if err := f.repo.DeleteBench(ctx, f.campaign, b.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := f.repo.ListBenches(ctx, f.campaign); len(list) != 0 {
		t.Fatalf("benches after delete: %v", list)
	}
}

func TestRepoIntegration_Projects(t *testing.T) {
	f := newRepoFixture(t)
	ctx := context.Background()
	store := f.repo.(ProjectStore)
	potion, mira, tobin := f.entity(t, "Healing potion"), f.entity(t, "Mira"), f.entity(t, "Tobin")
	r := Recipe{ID: newUUID(t), Name: "Healing potion", Template: "steady", Result: Result{ItemID: potion, Qty: 1}, Goal: 30,
		Who: "all", Rules: []Rule{}, Looks: map[string]*Look{}}
	if err := f.repo.CreateRecipe(ctx, f.campaign, f.user, r); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newProject := func(char string, at time.Time) Project {
		p := Project{ID: newUUID(t), CampaignID: f.campaign, CharacterID: char, RecipeID: r.ID, RecipeName: r.Name, Goal: 30, Stat: "reason",
			ResultItemID: potion, ResultQty: 1, Status: ProjectActive, Took: "2 × Bloodmoss, 25 gp", StartedBy: f.user, StartedAt: at}
		if err := store.CreateProject(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := newProject(mira, start)
	b := newProject(mira, start.Add(time.Hour))
	c := newProject(tobin, start.Add(2*time.Hour))

	got, ok, err := store.GetProject(ctx, f.campaign, a.ID)
	if err != nil || !ok || got.Version != 1 || got.RecipeID != r.ID || got.Took != a.Took || got.FinishedAt != nil || got.Status != ProjectActive {
		t.Fatalf("get: %+v %v %v", got, ok, err)
	}
	if _, ok, _ := store.GetProject(ctx, f.other, a.ID); ok {
		t.Fatal("a project was read through another campaign")
	}
	if n, _ := store.CountActive(ctx, f.campaign, mira); n != 2 {
		t.Fatalf("Mira's active projects = %d, want 2", n)
	}
	list, _ := store.ListProjects(ctx, f.campaign, "", ProjectActive, 0)
	if len(list) != 3 || list[0].ID != a.ID || list[2].ID != c.ID {
		t.Fatalf("active list order wrong: %v", list)
	}

	// Two respites stack; a finished project gets none.
	if n, err := store.LogRespite(ctx, f.campaign, f.user, "1 Frostmoot"); err != nil || n != 3 {
		t.Fatalf("first respite: %d %v", n, err)
	}
	got, _, _ = store.GetProject(ctx, f.campaign, a.ID)
	got.Points, got.RollsReady = 30, 0
	done := start.Add(24 * time.Hour)
	got.Status, got.Delivered, got.FinishedAt = ProjectMade, true, &done
	if ok, err := store.UpdateProject(ctx, got, got.Version); !ok || err != nil {
		t.Fatalf("finish: %v %v", ok, err)
	}
	if ok, _ := store.UpdateProject(ctx, got, got.Version); ok {
		t.Fatal("a stale version was written")
	}
	if n, err := store.LogRespite(ctx, f.campaign, f.user, ""); err != nil || n != 2 {
		t.Fatalf("second respite: %d %v", n, err)
	}
	gotB, _, _ := store.GetProject(ctx, f.campaign, b.ID)
	if gotB.RollsReady != 2 || gotB.Version != 3 {
		t.Fatalf("b after two respites: %+v", gotB)
	}
	gotA, _, _ := store.GetProject(ctx, f.campaign, a.ID)
	if gotA.RollsReady != 0 || gotA.Status != ProjectMade || !gotA.Delivered || gotA.FinishedAt == nil || !gotA.FinishedAt.Equal(done) {
		t.Fatalf("a after finishing: %+v", gotA)
	}
	if last, err := store.LastRespite(ctx, f.campaign); err != nil || last == nil || last.Projects != 2 || last.DayLabel != "" {
		t.Fatalf("last respite: %+v %v", last, err)
	}
	if last, _ := store.LastRespite(ctx, f.other); last != nil {
		t.Fatal("one campaign's respite showed in another")
	}
	if made, _ := store.ListProjects(ctx, f.campaign, mira, ProjectMade, 5); len(made) != 1 || made[0].ID != a.ID {
		t.Fatalf("made list: %v", made)
	}

	if err := store.AddRoll(ctx, f.campaign, b.ID, f.user, "7,8", `[{"text":"2d10","n":15}]`, 15, 0, 15); err != nil {
		t.Fatal(err)
	}

	// The recipe and the item it makes are deleted: the project keeps its
	// name and goal. The hero deleted: their projects and rolls go.
	if _, err := f.db.Exec(`DELETE FROM crafting_recipes WHERE id = ?`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`DELETE FROM entities WHERE id = ?`, potion); err != nil {
		t.Fatal(err)
	}
	gotB, _, _ = store.GetProject(ctx, f.campaign, b.ID)
	if gotB.RecipeID != "" || gotB.ResultItemID != "" || gotB.RecipeName != "Healing potion" || gotB.Goal != 30 {
		t.Fatalf("b after its recipe was deleted: %+v", gotB)
	}
	if _, err := f.db.Exec(`DELETE FROM entities WHERE id = ?`, mira); err != nil {
		t.Fatal(err)
	}
	var left, rolls int
	_ = f.db.QueryRow(`SELECT COUNT(*) FROM crafting_projects WHERE character_id = ?`, mira).Scan(&left)
	_ = f.db.QueryRow(`SELECT COUNT(*) FROM crafting_rolls`).Scan(&rolls)
	if left != 0 || rolls != 0 {
		t.Fatalf("a deleted hero left %d projects and %d rolls", left, rolls)
	}
}
