package trackers

// The tracker_players SQL is hand-written, so the fake the service tests use
// proves nothing about it; this runs it against a migrated scratch schema.
// Skips when CHRONICLE_TEST_DB_DSN is unset or no server answers.
//
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/trackers/ -run Integration

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
)

type playersFixture struct {
	db                         *sql.DB
	owner, tam, ilse, campaign string
	page, page2                string
	kind                       string
}

func newPlayersFixture(t *testing.T) *playersFixture {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN to run the trackers repository tests")
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
	name := fmt.Sprintf("chronicle_trackers_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
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
			t.Fatalf("trackers migrations did not apply: %v", res.Error)
		}
	}

	fx := &playersFixture{db: db,
		owner:    "00000000-0000-0000-0000-000000000001",
		tam:      "00000000-0000-0000-0000-000000000002",
		ilse:     "00000000-0000-0000-0000-000000000003",
		campaign: "00000000-0000-0000-0000-000000000010",
		page:     "00000000-0000-0000-0000-000000000011",
		page2:    "00000000-0000-0000-0000-000000000012",
		kind:     "00000000-0000-0000-0000-000000000020",
	}
	for i, u := range []string{fx.owner, fx.tam, fx.ilse} {
		fx.exec(t, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, u, fmt.Sprintf("u%d@example.test", i), "U", "x")
	}
	fx.exec(t, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, fx.campaign, "One", "one", fx.owner)
	fx.exec(t, `INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?,?,?,?)`, fx.campaign, "place", "Place", "Places")
	for _, p := range []string{fx.page, fx.page2} {
		fx.exec(t, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, visibility, created_by, created_at, updated_at)
			VALUES (?, ?, (SELECT id FROM entity_types WHERE campaign_id = ? AND slug = 'place'), ?, ?, false, 'default', ?, NOW(), NOW())`,
			p, fx.campaign, fx.campaign, "Page "+p[len(p)-1:], "p"+p[len(p)-1:], fx.owner)
	}
	zero, ten := 0, 10
	if err := NewRepository(db).CreateKind(context.Background(), Kind{
		ID: fx.kind, CampaignID: fx.campaign, Name: "Hunger", Shape: ShapeCount, Min: &zero, Max: &ten,
		Steps: []Step{}, Bands: []Band{}, Visibility: SeeDirectors,
	}); err != nil {
		t.Fatalf("create kind: %v", err)
	}
	return fx
}

func (fx *playersFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := fx.db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func (fx *playersFixture) placeTracker(t *testing.T, repo Repository, id, page, name, visibility string) {
	t.Helper()
	if err := repo.CreateTracker(context.Background(), Tracker{
		ID: id, CampaignID: fx.campaign, EntityID: page, KindID: fx.kind, Name: name, Visibility: visibility,
	}); err != nil {
		t.Fatalf("create tracker %s: %v", name, err)
	}
}

func TestRepositoryIntegration_TrackerPlayers(t *testing.T) {
	fx := newPlayersFixture(t)
	ctx := context.Background()
	repo := NewRepository(fx.db)

	const shown = "00000000-0000-0000-0000-000000000031"
	const other = "00000000-0000-0000-0000-000000000032"
	const onPage2 = "00000000-0000-0000-0000-000000000033"
	fx.placeTracker(t, repo, shown, fx.page, "Vampiric hunger", SeePlayers)
	fx.placeTracker(t, repo, other, fx.page, "Dread", SeePlayers)
	fx.placeTracker(t, repo, onPage2, fx.page2, "Omen", SeePlayers)

	// The grant time is checked on its own below; every other comparison is
	// about who and what they may do.
	bare := func(ps []Player) []Player {
		var out []Player
		for _, p := range ps {
			out = append(out, Player{UserID: p.UserID, CanChange: p.CanChange})
		}
		return out
	}
	players := func(t *testing.T, tr Tracker) []Player {
		t.Helper()
		return bare(tr.Players)
	}
	both := []Player{{UserID: fx.tam, CanChange: true}, {UserID: fx.ilse, CanChange: false}}

	t.Run("SetPlayers stores, replaces and clears", func(t *testing.T) {
		steps := []struct {
			name string
			set  []Player
			want []Player
		}{
			{"stores", both, both},
			{"replaces", []Player{{UserID: fx.ilse, CanChange: true}}, []Player{{UserID: fx.ilse, CanChange: true}}},
			{"clears", nil, nil},
			{"stores again after a clear", []Player{{UserID: fx.tam}}, []Player{{UserID: fx.tam}}},
		}
		for _, s := range steps {
			if err := repo.SetPlayers(ctx, fx.campaign, shown, s.set); err != nil {
				t.Fatalf("%s: %v", s.name, err)
			}
			got, found, err := repo.GetTracker(ctx, fx.campaign, shown)
			if err != nil || !found {
				t.Fatalf("%s: get found=%v err=%v", s.name, found, err)
			}
			if !reflect.DeepEqual(players(t, got), s.want) {
				t.Errorf("%s: players = %+v, want %+v", s.name, got.Players, s.want)
			}
			// A sibling tracker is never touched by this tracker's list.
			sib, _, _ := repo.GetTracker(ctx, fx.campaign, other)
			if len(sib.Players) != 0 {
				t.Errorf("%s: sibling tracker picked up %+v", s.name, sib.Players)
			}
		}
	})

	t.Run("a grant keeps the time it was given, and a new one is stamped now", func(t *testing.T) {
		old := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		before := time.Now().UTC().Add(-2 * time.Second)
		if err := repo.SetPlayers(ctx, fx.campaign, shown, []Player{{UserID: fx.tam, GrantedAt: old}, {UserID: fx.ilse}}); err != nil {
			t.Fatal(err)
		}
		got, _, _ := repo.GetTracker(ctx, fx.campaign, shown)
		if len(got.Players) != 2 {
			t.Fatalf("%+v", got.Players)
		}
		for _, p := range got.Players {
			switch p.UserID {
			case fx.tam:
				if !p.GrantedAt.Equal(old) {
					t.Errorf("kept grant time = %v want %v", p.GrantedAt, old)
				}
			case fx.ilse:
				if p.GrantedAt.Before(before) || p.GrantedAt.After(time.Now().UTC().Add(2*time.Second)) {
					t.Errorf("new grant time = %v, want about now", p.GrantedAt)
				}
			}
		}
	})

	t.Run("a bad user id rolls the whole replacement back", func(t *testing.T) {
		if err := repo.SetPlayers(ctx, fx.campaign, shown, both); err != nil {
			t.Fatal(err)
		}
		err := repo.SetPlayers(ctx, fx.campaign, shown, []Player{{UserID: fx.tam}, {UserID: "00000000-0000-0000-0000-0000000000ff"}})
		if err == nil {
			t.Fatal("an unknown user must fail the foreign key")
		}
		got, _, _ := repo.GetTracker(ctx, fx.campaign, shown)
		if !reflect.DeepEqual(bare(got.Players), both) {
			t.Fatalf("a failed replace must leave the old list: %+v", got.Players)
		}
	})

	t.Run("every read path attaches Players", func(t *testing.T) {
		if err := repo.SetPlayers(ctx, fx.campaign, shown, both); err != nil {
			t.Fatal(err)
		}
		if err := repo.SetPlayers(ctx, fx.campaign, onPage2, []Player{{UserID: fx.tam}}); err != nil {
			t.Fatal(err)
		}
		find := func(ts []Tracker, id string) Tracker {
			for _, tr := range ts {
				if tr.ID == id {
					return tr
				}
			}
			t.Fatalf("tracker %s missing from %d rows", id, len(ts))
			return Tracker{}
		}
		page, err := repo.ListTrackers(ctx, fx.campaign, fx.page)
		if err != nil || len(page) != 2 {
			t.Fatalf("page list: %d %v", len(page), err)
		}
		if !reflect.DeepEqual(bare(find(page, shown).Players), both) || len(find(page, other).Players) != 0 {
			t.Errorf("ListTrackers players wrong: %+v", page)
		}
		all, err := repo.ListAllTrackers(ctx, fx.campaign)
		if err != nil || len(all) != 3 {
			t.Fatalf("all list: %d %v", len(all), err)
		}
		if !reflect.DeepEqual(bare(find(all, shown).Players), both) ||
			!reflect.DeepEqual(bare(find(all, onPage2).Players), []Player{{UserID: fx.tam}}) ||
			len(find(all, other).Players) != 0 {
			t.Errorf("ListAllTrackers players wrong: %+v", all)
		}
		byID, _, err := repo.GetTracker(ctx, fx.campaign, shown)
		if err != nil || !reflect.DeepEqual(bare(byID.Players), both) {
			t.Errorf("GetTracker players: %+v %v", byID.Players, err)
		}
		byKind, found, err := repo.TrackerForKind(ctx, fx.campaign, fx.page2, fx.kind)
		if err != nil || !found || !reflect.DeepEqual(bare(byKind.Players), []Player{{UserID: fx.tam}}) {
			t.Errorf("TrackerForKind players: %+v found=%v err=%v", byKind.Players, found, err)
		}
	})

	t.Run("rows left behind are not attached once the tracker is not players-visible", func(t *testing.T) {
		if err := repo.UpdateTracker(ctx, Tracker{ID: shown, CampaignID: fx.campaign, Name: "Vampiric hunger", Visibility: SeeEveryone}); err != nil {
			t.Fatal(err)
		}
		got, _, _ := repo.GetTracker(ctx, fx.campaign, shown)
		if len(got.Players) != 0 {
			t.Fatalf("a non-players tracker must not carry a list: %+v", got.Players)
		}
	})

	t.Run("deleting a tracker cascades its rows", func(t *testing.T) {
		count := func(id string) int {
			var n int
			if err := fx.db.QueryRow(`SELECT COUNT(*) FROM tracker_players WHERE tracker_id = ?`, id).Scan(&n); err != nil {
				t.Fatal(err)
			}
			return n
		}
		if err := repo.SetPlayers(ctx, fx.campaign, other, both); err != nil {
			t.Fatal(err)
		}
		if count(other) != 2 {
			t.Fatalf("setup: %d rows", count(other))
		}
		if err := repo.DeleteTracker(ctx, fx.campaign, other); err != nil {
			t.Fatal(err)
		}
		if count(other) != 0 {
			t.Fatal("deleting a tracker must delete its player rows")
		}
		if count(onPage2) != 1 {
			t.Fatal("another tracker's rows must survive")
		}
		// Deleting the page cascades through the tracker as well.
		fx.exec(t, `DELETE FROM entities WHERE id = ?`, fx.page2)
		if count(onPage2) != 0 {
			t.Fatal("deleting the page must delete the rows")
		}
	})

	t.Run("deleting a user removes their grants only", func(t *testing.T) {
		if err := repo.SetPlayers(ctx, fx.campaign, shown, both); err != nil {
			t.Fatal(err)
		}
		fx.exec(t, `DELETE FROM users WHERE id = ?`, fx.ilse)
		if err := repo.UpdateTracker(ctx, Tracker{ID: shown, CampaignID: fx.campaign, Name: "Vampiric hunger", Visibility: SeePlayers}); err != nil {
			t.Fatal(err)
		}
		got, _, _ := repo.GetTracker(ctx, fx.campaign, shown)
		if !reflect.DeepEqual(bare(got.Players), []Player{{UserID: fx.tam, CanChange: true}}) {
			t.Fatalf("players after account delete: %+v", got.Players)
		}
	})
}

func bare2(ps []Player) []Player {
	var out []Player
	for _, p := range ps {
		out = append(out, Player{UserID: p.UserID, CanChange: p.CanChange})
	}
	return out
}

func TestMigrationIntegration_TrackerPlayersAppliedTwice(t *testing.T) {
	fx := newPlayersFixture(t)
	up, err := MigrationsFS.ReadFile("migrations/003_tracker_players.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(fx.db)
	const id = "00000000-0000-0000-0000-000000000031"
	fx.placeTracker(t, repo, id, fx.page, "Vampiric hunger", SeePlayers)
	if err := repo.SetPlayers(context.Background(), fx.campaign, id, []Player{{UserID: fx.tam, CanChange: true}}); err != nil {
		t.Fatal(err)
	}
	// The fixture already applied it once; two more runs must change nothing.
	for i := 0; i < 2; i++ {
		if _, err := fx.db.Exec(string(up)); err != nil {
			t.Fatalf("re-apply %d: %v", i+1, err)
		}
	}
	got, _, err := repo.GetTracker(context.Background(), fx.campaign, id)
	if err != nil || !reflect.DeepEqual(bare2(got.Players), []Player{{UserID: fx.tam, CanChange: true}}) {
		t.Fatalf("rows after re-applying: %+v %v", got.Players, err)
	}
}
