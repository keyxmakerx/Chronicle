package playerchanges

// Row-level tests: the filters in Live, folding, Undo marks, suggestions and
// packing a month away and back. Each test gets its own migrated scratch
// schema and skips (never fails) when no server is reachable.
//
//	make test-db-up
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/playerchanges/ -run Integration

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
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN (make test-db-up) to run the row-level tests")
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
	name := fmt.Sprintf("chronicle_pcx_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
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
			t.Fatalf("player changes migrations did not apply: %v", res.Error)
		}
	}
	return db
}

func TestRepositoryIntegration(t *testing.T) {
	db := newScratchDB(t)
	ctx := context.Background()
	owner, campaign, other := uuid.New().String(), uuid.New().String(), uuid.New().String()
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, owner, owner+"@example.test", "KM", "x"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{campaign, other} {
		if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, c, "Therin", c, owner); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewRepository(db)
	now := time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)
	add := func(campaignID, entity, field, source, kind string, at time.Time) {
		t.Helper()
		if err := repo.Insert(ctx, Change{CampaignID: campaignID, EntityID: entity, PageName: "Page " + entity[:4], UserID: "u1",
			UserName: "Bea", Kind: kind, FieldKey: field, FieldLabel: field, OldText: "1", NewText: "2",
			OldJSON: "1", NewJSON: "2", Source: source, App: "Claude", CreatedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	tyne, mill := uuid.New().String(), uuid.New().String()
	add(campaign, tyne, "hp", SourceWeb, KindField, now.Add(-time.Hour))
	add(campaign, tyne, "ancestry", SourceAI, KindField, now.Add(-2*time.Hour))
	add(campaign, mill, "", SourceWeb, KindRemoved, now.Add(-3*time.Hour))
	add(other, tyne, "hp", SourceWeb, KindField, now.Add(-time.Hour))
	// Two months back, to be packed.
	add(campaign, tyne, "hp", SourceWeb, KindField, time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC))
	add(campaign, tyne, "class", SourceWeb, KindField, time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC))

	since := now.AddDate(0, 0, -LiveDays)
	t.Run("Live filters in SQL and stays in its campaign", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			q    Query
			want int
		}{
			{"this month", Query{Since: since}, 3},
			{"one page", Query{Since: since, Pages: []string{mill}}, 1},
			{"by an AI", Query{Since: since, AI: true}, 1},
			{"removed", Query{Since: since, Removed: true}, 1},
			{"from long ago", Query{Since: now.AddDate(0, -6, 0)}, 5},
		} {
			got, err := repo.Live(ctx, campaign, tt.q)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.want {
				t.Errorf("%s: %d rows, want %d", tt.name, len(got), tt.want)
			}
		}
	})

	t.Run("fold, undo mark and suggestions", func(t *testing.T) {
		probe := Change{CampaignID: campaign, EntityID: tyne, UserID: "u1", Kind: KindField, FieldKey: "hp", Source: SourceWeb}
		last, err := repo.LastFor(ctx, probe, now.Add(-2*time.Hour))
		if err != nil || last == nil {
			t.Fatalf("LastFor = %v, %v", last, err)
		}
		last.NewText, last.NewJSON, last.CreatedAt = "5", "5", now
		if err := repo.UpdateNew(ctx, *last); err != nil {
			t.Fatal(err)
		}
		got, _ := repo.Get(ctx, campaign, last.ID)
		if got.NewText != "5" || got.OldText != "1" || !got.CreatedAt.Equal(now) {
			t.Errorf("after fold = %+v", got)
		}
		if g, _ := repo.Get(ctx, other, last.ID); g != nil {
			t.Error("Get must not reach another campaign's change")
		}
		if ok, _ := repo.MarkUndone(ctx, other, last.ID, owner, now); ok {
			t.Error("MarkUndone must not reach another campaign's change")
		}
		if ok, _ := repo.MarkUndone(ctx, campaign, last.ID, owner, now); !ok {
			t.Error("MarkUndone should mark the change")
		}
		if ok, _ := repo.MarkUndone(ctx, campaign, last.ID, owner, now); ok {
			t.Error("a change is undone once")
		}
		if l, _ := repo.LastFor(ctx, probe, now.Add(-2*time.Hour)); l != nil {
			t.Error("an undone change is never folded into")
		}
		pages, fields, err := repo.Recent(ctx, campaign, since)
		if err != nil || len(pages) != 2 || len(fields) != 2 {
			t.Errorf("Recent = %v / %v, %v", pages, fields, err)
		}
	})

	t.Run("size before anything is packed", func(t *testing.T) {
		if b, known, err := repo.MonthlyBytes(ctx); err != nil || !known || b <= 0 {
			t.Errorf("MonthlyBytes = %d, %v, %v; want an estimate from the live rows", b, known, err)
		}
	})

	t.Run("pack a month, read it, undo in it, prune it", func(t *testing.T) {
		keys, err := repo.DueMonths(ctx, since)
		if err != nil || len(keys) != 1 || keys[0].Month.Month() != time.August {
			t.Fatalf("DueMonths = %v, %v", keys, err)
		}
		keep := time.Date(2027, 4, 20, 0, 0, 0, 0, time.UTC)
		n, err := repo.PackMonth(ctx, keys[0], since, keep)
		if err != nil || n != 2 {
			t.Fatalf("PackMonth = %d, %v", n, err)
		}
		if live, _ := repo.Live(ctx, campaign, Query{Since: now.AddDate(0, -6, 0)}); len(live) != 3 {
			t.Errorf("packed rows must leave the live table; %d left", len(live))
		}
		packs, err := repo.Packs(ctx, campaign, now.AddDate(0, -6, 0))
		if err != nil || len(packs) != 1 || len(packs[0].Rows) != 2 || !packs[0].Rows[0].Packed {
			t.Fatalf("Packs = %+v, %v", packs, err)
		}
		// A shorter setting later never shortens a pack already made.
		add(campaign, tyne, "hp", SourceWeb, KindField, time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC))
		if _, err := repo.PackMonth(ctx, keys[0], since, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
		packs, _ = repo.Packs(ctx, campaign, now.AddDate(0, -6, 0))
		if len(packs[0].Rows) != 3 || !packs[0].KeepUntil.Equal(keep) {
			t.Errorf("merged pack = %d rows until %v; want 3 until %v", len(packs[0].Rows), packs[0].KeepUntil, keep)
		}
		at := now
		packs[0].Rows[0].UndoneAt = &at
		if err := repo.RewritePack(ctx, packs[0].PackKey, packs[0].Rows); err != nil {
			t.Fatal(err)
		}
		packs, _ = repo.Packs(ctx, campaign, now.AddDate(0, -6, 0))
		if packs[0].Rows[0].UndoneAt == nil {
			t.Error("RewritePack must keep the undo mark")
		}
		if n, _ := repo.PrunePacks(ctx, keep); n != 0 {
			t.Error("a pack is kept through its last day")
		}
		if n, _ := repo.PrunePacks(ctx, keep.AddDate(0, 0, 1)); n != 1 {
			t.Error("a pack past its day is removed")
		}
	})

	t.Run("off drops instead of packing", func(t *testing.T) {
		add(campaign, mill, "hp", SourceWeb, KindField, time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC))
		k := PackKey{CampaignID: campaign, Month: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)}
		if n, err := repo.PackMonth(ctx, k, since, time.Time{}); err != nil || n != 1 {
			t.Fatalf("PackMonth = %d, %v", n, err)
		}
		if packs, _ := repo.Packs(ctx, campaign, k.Month); len(packs) != 0 {
			t.Error("with Long-term trash off, old rows are dropped, not packed")
		}
	})

	t.Run("removed pages are kept while their pack is", func(t *testing.T) {
		ruin := uuid.New().String()
		add(campaign, ruin, "", SourceWeb, KindRemoved, time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC))
		kept, err := repo.KeptPages(ctx, now)
		if err != nil || !containsAll(kept, mill, ruin) {
			t.Fatalf("live removals are kept: %v, %v", kept, err)
		}
		keep := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
		k := PackKey{CampaignID: campaign, Month: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)}
		if _, err := repo.PackMonth(ctx, k, since, keep); err != nil {
			t.Fatal(err)
		}
		if kept, _ = repo.KeptPages(ctx, now); !containsAll(kept, ruin) {
			t.Errorf("a packed removal is still kept: %v", kept)
		}
		if kept, _ = repo.KeptPages(ctx, keep.AddDate(0, 0, 1)); containsAll(kept, ruin) {
			t.Error("a kept page past its day is no longer kept")
		}
		if err := repo.ForgetPage(ctx, campaign, ruin); err != nil {
			t.Fatal(err)
		}
		if kept, _ = repo.KeptPages(ctx, now); containsAll(kept, ruin) {
			t.Error("a page brought back is no longer kept")
		}
	})
}

func containsAll(list []string, want ...string) bool {
	for _, w := range want {
		if !contains(list, w) {
			return false
		}
	}
	return true
}
