package rolls

// Row-level tests for the roll log: the audience rule in visibleTo, the
// replay key on a Foundry message, paging and the table's look. Each test
// gets its own migrated scratch schema and skips (never fails) when no
// server is reachable.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/rolls/ -run TestRepoIntegration

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

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
	name := fmt.Sprintf("chronicle_rolls_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
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
			t.Fatalf("rolls migrations did not apply: %v", res.Error)
		}
	}
	return db
}

func TestRepoIntegration(t *testing.T) {
	db := newScratchDB(t)
	ctx := context.Background()
	user, campaign := uuid.New().String(), uuid.New().String()
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, user, user+"@example.test", "Ana", "x"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, campaign, "Table", campaign, user); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	repo := NewRepository(db)
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	add := func(mode, roller, originID string, i int) string {
		r := Roll{ID: uuid.New().String(), CampaignID: campaign, Origin: OriginChronicle, Source: SourceTable, RollerID: roller,
			RollerName: "Ana", Label: mode, Formula: "1d20", Dice: []Die{{20, 7}}, Total: 7, Mode: mode, CreatedAt: at.Add(time.Duration(i) * time.Second)}
		if originID != "" {
			r.Origin, r.OriginID = OriginFoundry, originID
		}
		id, dup, err := repo.Insert(ctx, r)
		if err != nil || dup {
			t.Fatalf("insert %s: %v %v", mode, err, dup)
		}
		return id
	}
	add(ModePublic, user, "", 1)
	add(ModeGM, user, "", 2)
	add(ModeBlind, user, "", 3)
	add(ModeSelf, user, "", 4)
	fid := add(ModeGM, "", "msg-1", 5)

	t.Run("a replay of the same message is not added", func(t *testing.T) {
		r := Roll{ID: uuid.New().String(), CampaignID: campaign, Origin: OriginFoundry, OriginID: "msg-1", Source: SourceFoundry, Dice: []Die{}, Mode: ModePublic, CreatedAt: at}
		id, dup, err := repo.Insert(ctx, r)
		if err != nil || !dup || id != fid {
			t.Fatalf("replay: id %s dup %v err %v", id, dup, err)
		}
	})

	count := func(v Viewer) map[string]int {
		rows, err := repo.List(ctx, campaign, v, "", 50)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]int{}
		for _, r := range rows {
			out[r.Mode]++
		}
		return out
	}
	t.Run("the roller", func(t *testing.T) {
		got := count(Viewer{UserID: user})
		if got[ModePublic] != 1 || got[ModeGM] != 1 || got[ModeBlind] != 1 || got[ModeSelf] != 1 {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("a director", func(t *testing.T) {
		got := count(Viewer{UserID: uuid.New().String(), Director: true})
		if got[ModePublic] != 1 || got[ModeGM] != 2 || got[ModeBlind] != 1 || got[ModeSelf] != 0 {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("another player", func(t *testing.T) {
		got := count(Viewer{UserID: uuid.New().String()})
		if got[ModePublic] != 1 || len(got) != 1 {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("paging walks back", func(t *testing.T) {
		v := Viewer{UserID: user, Director: true}
		first, err := repo.List(ctx, campaign, v, "", 2)
		if err != nil || len(first) != 2 || first[0].Label != ModeGM || first[0].OriginID != "msg-1" || len(first[0].Dice) != 1 {
			t.Fatalf("first page %+v %v", first, err)
		}
		rest, err := repo.List(ctx, campaign, v, first[1].ID, 10)
		if err != nil || len(rest) != 3 || rest[0].Label != ModeBlind {
			t.Fatalf("rest %+v %v", rest, err)
		}
	})
	t.Run("the table's look", func(t *testing.T) {
		if l, err := repo.Look(ctx, campaign); err != nil || l != "" {
			t.Fatalf("unset look = %q %v", l, err)
		}
		if err := repo.SetLook(ctx, campaign, LookRune); err != nil {
			t.Fatal(err)
		}
		if err := repo.SetLook(ctx, campaign, LookNone); err != nil {
			t.Fatal(err)
		}
		if l, _ := repo.Look(ctx, campaign); l != LookNone {
			t.Fatalf("look = %q", l)
		}
	})
}
