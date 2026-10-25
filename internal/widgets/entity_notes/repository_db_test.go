package entity_notes

// Real-MariaDB test for the campaign-wide list. The audience rule is written
// twice, as noteACLFilter in SQL and NotePassesACL in Go; this runs both over
// the same rows so they can't drift.
//
// The test gets its own scratch schema, migrated with the core migrations,
// and drops it on cleanup. Skips when no server is reachable:
//
//	make test-db-up
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/widgets/entity_notes/ -run TestDB

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
)

func newScratchDB(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN (make test-db-up) to run the row-level tests")
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

	name := fmt.Sprintf("chronicle_enotes_%06d", rand.Intn(1000000)) //nolint:gosec // test schema name
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
		t.Skipf("core migrations did not apply: %v", err)
	}
	return db
}

func dbUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		t.Fatalf("random id: %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) sql.Result {
	t.Helper()
	res, err := db.Exec(q, args...)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return res
}

// TestDB_ListByCampaignMatchesACL writes one note per audience across two
// pages, plus one in another campaign, and checks for every viewer that the
// SQL list and NotePassesACL agree on every row.
func TestDB_ListByCampaignMatchesACL(t *testing.T) {
	db := newScratchDB(t)
	ctx := context.Background()
	repo := NewRepository(db)

	u := map[string]string{}
	for _, n := range []string{"gm", "ana", "bo", "cy"} {
		u[n] = dbUUID(t)
		mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, u[n], u[n]+"@example.test", n, "x")
	}
	camp, other := dbUUID(t), dbUUID(t)
	for _, c := range []string{camp, other} {
		mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, c, "Table", c, u["gm"])
	}
	page := func(campaignID, name string) string {
		res := mustExec(t, db, `INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?,?,?,?)`, campaignID, dbUUID(t), "Place", "Places")
		typeID, _ := res.LastInsertId()
		id := dbUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, visibility, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, false, 'default', ?, NOW(), NOW())`, id, campaignID, typeID, name, dbUUID(t), u["gm"])
		return id
	}
	inn, road, away := page(camp, "Inn"), page(camp, "Road"), page(other, "Elsewhere")

	all := map[string]*Note{}
	mk := func(campaignID, entityID, author string, aud Audience, shared ...string) {
		n := &Note{ID: dbUUID(t), EntityID: entityID, CampaignID: campaignID, AuthorUserID: u[author], Audience: aud, Title: author + " " + string(aud)}
		for _, s := range shared {
			n.SharedWith = append(n.SharedWith, u[s])
		}
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("create: %v", err)
		}
		all[n.ID] = n
	}
	mk(camp, inn, "ana", AudiencePrivate)
	mk(camp, inn, "ana", AudienceDMOnly)
	mk(camp, road, "ana", AudienceDMScribe)
	mk(camp, road, "ana", AudienceEveryone)
	mk(camp, road, "ana", AudienceCustom, "bo")
	mk(camp, inn, "gm", AudiencePrivate)
	mk(camp, inn, "cy", AudienceCustom, "gm", "bo")
	mk(other, away, "ana", AudienceEveryone)

	viewers := map[string]ViewerContext{
		"gm (Owner)":         {UserID: u["gm"], CampaignID: camp, IsOwner: true},
		"ana":                {UserID: u["ana"], CampaignID: camp},
		"bo":                 {UserID: u["bo"], CampaignID: camp},
		"cy (Scribe)":        {UserID: u["cy"], CampaignID: camp, IsScribe: true},
		"bo (co-DM granted)": {UserID: u["bo"], CampaignID: camp, IsDMGranted: true},
	}
	for name, v := range viewers {
		t.Run(name, func(t *testing.T) {
			listed, err := repo.ListByCampaign(ctx, v)
			if err != nil {
				t.Fatalf("ListByCampaign: %v", err)
			}
			got := map[string]bool{}
			for _, n := range listed {
				got[n.ID] = true
			}
			for id, n := range all {
				want := n.CampaignID == v.CampaignID && NotePassesACL(n, v)
				if got[id] != want {
					t.Errorf("%q in campaign %s: SQL lists it %v, the rule says %v", n.Title, n.CampaignID, got[id], want)
				}
			}
		})
	}

	// The checks that matter most, stated outright.
	gm, _ := repo.ListByCampaign(ctx, viewers["gm (Owner)"])
	for _, n := range gm {
		if n.AuthorUserID == u["ana"] && n.Audience == AudiencePrivate {
			t.Error("the owner must not see a player's private note")
		}
		if n.CampaignID != camp {
			t.Error("another campaign's note was listed")
		}
	}
}
