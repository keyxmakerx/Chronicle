package campaigns

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	mrand "math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
)

// TestNavPins_Integration round-trips members' own sidebar pins through a
// real MariaDB with every core migration applied (000031 adds the column):
// each member reads back only their own pins, a refused update stores
// nothing, clearing writes NULL, and pins leave with the membership.
//
// Skipped under -short. DSN from CHRONICLE_TEST_DB_DSN, else the DB_* env
// vars; with no database reachable it skips. A migration that fails to apply
// FAILS the test, since the migration is part of what it checks.
func TestNavPins_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openNavPinsTestDB(t)
	ctx := context.Background()
	repo := NewCampaignRepository(db)

	owner, p1, p2 := navPinsUUID(t), navPinsUUID(t), navPinsUUID(t)
	campaignID := navPinsUUID(t)
	for _, u := range []string{owner, p1, p2} {
		navPinsExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
			u, "navpins-"+u+"@example.test", "Nav Pins", "x")
	}
	navPinsExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Nav Pins", "nav-pins-"+campaignID[:8], owner)
	navPinsExec(t, db, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'owner'), (?, ?, 'player'), (?, ?, 'player')`,
		campaignID, owner, campaignID, p1, campaignID, p2)

	if pins, err := repo.GetMemberNavPins(ctx, campaignID, p1); err != nil || pins != nil {
		t.Fatalf("a new member has no pins: %v, %v", pins, err)
	}

	svc := NewCampaignService(repo, nil, nil, nil, "")
	svc.SetNavSectionsSource(fakeNavSections{secs: playerSidebar()})
	cc := &CampaignContext{Campaign: &Campaign{ID: campaignID}, MemberRole: RolePlayer, IsMember: true}

	if _, err := svc.UpdateNavPins(ctx, cc, p1, []string{"cat:2", "app:maps"}); err != nil {
		t.Fatalf("UpdateNavPins: %v", err)
	}
	// Writing the same value again changes no row, which is not an error.
	if _, err := svc.UpdateNavPins(ctx, cc, p1, []string{"cat:2", "app:maps"}); err != nil {
		t.Fatalf("UpdateNavPins (unchanged): %v", err)
	}
	if pins, err := svc.NavPins(ctx, campaignID, p1); err != nil || !reflect.DeepEqual(pins, []string{"cat:2", "app:maps"}) {
		t.Fatalf("p1 pins = %v, %v", pins, err)
	}
	if pins, err := svc.NavPins(ctx, campaignID, p2); err != nil || pins != nil {
		t.Fatalf("p2 must not see p1's pins: %v, %v", pins, err)
	}
	if _, err := svc.UpdateNavPins(ctx, cc, p1, []string{"cat:3"}); err == nil {
		t.Fatalf("a row hidden from players must be refused")
	}
	if pins, _ := svc.NavPins(ctx, campaignID, p1); !reflect.DeepEqual(pins, []string{"cat:2", "app:maps"}) {
		t.Fatalf("a refused update must store nothing: %v", pins)
	}
	ownerCC := &CampaignContext{Campaign: &Campaign{ID: campaignID}, MemberRole: RoleOwner, IsMember: true}
	if _, err := svc.UpdateNavPins(ctx, ownerCC, owner, []string{"app:maps"}); err == nil {
		t.Fatalf("the owner must be refused")
	}

	if _, err := svc.UpdateNavPins(ctx, cc, p1, nil); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	var raw sql.NullString
	if err := db.QueryRow(`SELECT nav_pins FROM campaign_members WHERE campaign_id = ? AND user_id = ?`, campaignID, p1).Scan(&raw); err != nil {
		t.Fatalf("reading raw pins: %v", err)
	}
	if raw.Valid {
		t.Fatalf("clearing must store NULL, got %q", raw.String)
	}

	// Pins go with the membership.
	if _, err := svc.UpdateNavPins(ctx, cc, p2, []string{"app:characters"}); err != nil {
		t.Fatalf("p2 UpdateNavPins: %v", err)
	}
	if err := repo.RemoveMember(ctx, campaignID, p2); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	navPinsExec(t, db, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'player')`, campaignID, p2)
	if pins, _ := svc.NavPins(ctx, campaignID, p2); pins != nil {
		t.Fatalf("a member who left and came back starts without pins: %v", pins)
	}
}

func openNavPinsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		cfg := mysql.NewConfig()
		cfg.User = navPinsGetenv("DB_USER", "chronicle")
		cfg.Passwd = navPinsGetenv("DB_PASSWORD", "chronicle")
		cfg.Net = "tcp"
		cfg.Addr = navPinsGetenv("DB_HOST", "127.0.0.1:3306")
		raw = cfg.FormatDSN()
	}
	cfg, err := mysql.ParseDSN(raw)
	if err != nil {
		t.Skipf("CHRONICLE_TEST_DB_DSN is not a valid DSN: %v", err)
	}
	cfg.ParseTime = true

	serverCfg := *cfg
	serverCfg.DBName = ""
	admin, err := sql.Open("mysql", serverCfg.FormatDSN())
	if err != nil {
		t.Skipf("no test DB (sql.Open: %v)", err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Skipf("no test DB server reachable at %s: %v — run `make test-db-up`", cfg.Addr, err)
	}

	name := fmt.Sprintf("chronicle_navpins_%06d", mrand.Intn(1000000)) //nolint:gosec // test schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })

	scratchCfg := *cfg
	scratchCfg.DBName = name
	db, err := sql.Open("mysql", scratchCfg.FormatDSN())
	if err != nil {
		t.Fatalf("opening scratch schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	if err := database.RunMigrations(db, scratchCfg.FormatDSN(), filepath.Join(root, "db", "migrations")); err != nil {
		t.Fatalf("core migrations did not apply: %v", err)
	}
	return db
}

func navPinsGetenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func navPinsExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func navPinsUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("uuid: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
