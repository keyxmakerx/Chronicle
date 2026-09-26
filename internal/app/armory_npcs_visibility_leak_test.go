package app

// armory_npcs_visibility_leak_test.go pins that the Armory and NPC galleries
// list entities through the entities plugin's canonical
// FilterViewableEntityIDs (the same entityVisibilityFilterAdapter the
// sessions plugin uses), not a hand-rolled predicate — a hand-rolled
// `role < 2 AND is_private = false` filter ignores entities.visibility /
// entity_permissions, so a `visibility='custom'`-restricted entity (which
// SetEntityPermissions does not clear is_private for) still leaked to
// Players and anonymous visitors. Runs against a real database, in
// internal/app (armory/npcs may not import the entities repository
// directly, per plugin isolation), because a mock of the predicate would not
// prove the wiring is real.
//
// Skipped under -short. Run with `make docker-up && make migrate-up && go
// test ./internal/app/... -run CustomVisibilityLeak -v`, or against
// tools/start-test-db.sh's local MariaDB via CHRONICLE_TEST_DB_DSN.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	mrand "math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/npcs"
)

// TestArmoryGallery_CustomVisibilityLeak is the Armory half of finding 2.
func TestArmoryGallery_CustomVisibilityLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openGalleryTestDB(t)
	ctx := context.Background()

	fx := newGalleryFixture(t, db)
	defer fx.cleanup()

	itemTypeID := fx.entityType("gallery-item-type", "Item", "item", "")
	publicItemID := fx.entity(itemTypeID, "Visible Sword", "visible-sword", "default")
	restrictedItemID := fx.entity(itemTypeID, "Secret Wand", "secret-wand", "custom")
	fx.grantView(restrictedItemID, "user", fx.otherUserID)

	entityService := fx.entityService()
	armoryRepo := armory.NewArmoryRepository(db)
	visFilter := &entityVisibilityFilterAdapter{svc: entityService}
	armorySvc := armory.NewArmoryService(armoryRepo, &armoryItemTypeFinderAdapter{svc: entityService}, visFilter)

	// Only Owner bypasses a custom-visibility entity's permissions; a Scribe
	// needs a grant like anyone else, as entities.TestVisibilityFilter pins.
	cases := []struct {
		name      string
		role      int
		userID    string
		wantIDs   map[string]bool
		wantCount int
	}{
		{"anonymous viewer on a public campaign", permissions.RoleNone, "", map[string]bool{publicItemID: true}, 1},
		{"player", permissions.RolePlayer, "player-1", map[string]bool{publicItemID: true}, 1},
		{"scribe (co-DM), no grant — filtered same as Player for custom-mode", permissions.RoleScribe, "scribe-1", map[string]bool{publicItemID: true}, 1},
		{"owner", permissions.RoleOwner, fx.ownerID, map[string]bool{publicItemID: true, restrictedItemID: true}, 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cards, total, err := armorySvc.ListItems(ctx, fx.campaignID, tc.role, tc.userID, armory.DefaultItemListOptions())
			if err != nil {
				t.Fatalf("ListItems: %v", err)
			}
			got := map[string]bool{}
			for _, c := range cards {
				got[c.ID] = true
			}
			if got[restrictedItemID] && !tc.wantIDs[restrictedItemID] {
				t.Errorf("LEAK: restricted item %q returned to %s (role=%d); got IDs=%v", restrictedItemID, tc.name, tc.role, got)
			}
			for id, want := range tc.wantIDs {
				if got[id] != want {
					t.Errorf("item %q presence = %v, want %v for %s (role=%d)", id, got[id], want, tc.name, tc.role)
				}
			}
			if total != tc.wantCount {
				t.Errorf("ListItems total = %d, want %d for %s", total, tc.wantCount, tc.name)
			}

			count, err := armorySvc.CountItems(ctx, fx.campaignID, tc.role, tc.userID)
			if err != nil {
				t.Fatalf("CountItems: %v", err)
			}
			if count != tc.wantCount {
				t.Errorf("CountItems = %d, want %d for %s (list/count must never disagree)", count, tc.wantCount, tc.name)
			}
			if count != total {
				t.Errorf("CountItems (%d) disagrees with ListItems total (%d) for %s", count, total, tc.name)
			}
		})
	}
}

// TestNPCGallery_CustomVisibilityLeak is the NPC half of finding 2.
func TestNPCGallery_CustomVisibilityLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openGalleryTestDB(t)
	ctx := context.Background()

	fx := newGalleryFixture(t, db)
	defer fx.cleanup()

	charTypeID := fx.entityType("characters", "Character", "", "characters")
	publicNPCID := fx.entity(charTypeID, "Visible Guard", "visible-guard", "default")
	restrictedNPCID := fx.entity(charTypeID, "Secret Spy", "secret-spy", "custom")
	fx.grantView(restrictedNPCID, "user", fx.otherUserID)

	entityService := fx.entityService()
	npcRepo := npcs.NewNPCRepository(db)
	visFilter := &entityVisibilityFilterAdapter{svc: entityService}
	npcSvc := npcs.NewNPCService(npcRepo, &npcEntityTypeFinderAdapter{svc: entityService}, visFilter)

	// Only Owner bypasses a custom-visibility entity's permissions; a Scribe
	// needs a grant like anyone else, as entities.TestVisibilityFilter pins.
	cases := []struct {
		name      string
		role      int
		userID    string
		wantIDs   map[string]bool
		wantCount int
	}{
		{"anonymous viewer on a public campaign", permissions.RoleNone, "", map[string]bool{publicNPCID: true}, 1},
		{"player", permissions.RolePlayer, "player-1", map[string]bool{publicNPCID: true}, 1},
		{"scribe (co-DM), no grant — filtered same as Player for custom-mode", permissions.RoleScribe, "scribe-1", map[string]bool{publicNPCID: true}, 1},
		{"owner", permissions.RoleOwner, fx.ownerID, map[string]bool{publicNPCID: true, restrictedNPCID: true}, 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cards, total, err := npcSvc.ListNPCs(ctx, fx.campaignID, tc.role, tc.userID, npcs.DefaultNPCListOptions())
			if err != nil {
				t.Fatalf("ListNPCs: %v", err)
			}
			got := map[string]bool{}
			for _, c := range cards {
				got[c.ID] = true
			}
			if got[restrictedNPCID] && !tc.wantIDs[restrictedNPCID] {
				t.Errorf("LEAK: restricted NPC %q returned to %s (role=%d); got IDs=%v", restrictedNPCID, tc.name, tc.role, got)
			}
			for id, want := range tc.wantIDs {
				if got[id] != want {
					t.Errorf("NPC %q presence = %v, want %v for %s (role=%d)", id, got[id], want, tc.name, tc.role)
				}
			}
			if total != tc.wantCount {
				t.Errorf("ListNPCs total = %d, want %d for %s", total, tc.wantCount, tc.name)
			}

			count, err := npcSvc.CountNPCs(ctx, fx.campaignID, tc.role, tc.userID)
			if err != nil {
				t.Fatalf("CountNPCs: %v", err)
			}
			if count != tc.wantCount {
				t.Errorf("CountNPCs = %d, want %d for %s (list/count must never disagree)", count, tc.wantCount, tc.name)
			}
			if count != total {
				t.Errorf("CountNPCs (%d) disagrees with ListNPCs total (%d) for %s", count, total, tc.name)
			}
		})
	}
}

// --- fixtures & DB helpers (distinct names from other _test.go files in this
// module to avoid symbol collisions with the entities/cmd-server helpers of
// the same shape) ---

type galleryFixture struct {
	t           *testing.T
	db          *sql.DB
	ownerID     string
	otherUserID string
	campaignID  string
}

func newGalleryFixture(t *testing.T, db *sql.DB) *galleryFixture {
	t.Helper()
	fx := &galleryFixture{
		t:           t,
		db:          db,
		ownerID:     galleryTestUUID(t),
		otherUserID: galleryTestUUID(t),
		campaignID:  galleryTestUUID(t),
	}
	mustGalleryExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		fx.ownerID, "gallery-leak-owner-"+fx.ownerID+"@example.test", "Gallery Leak Owner", "x")
	mustGalleryExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		fx.otherUserID, "gallery-leak-other-"+fx.otherUserID+"@example.test", "Gallery Leak Other", "x")
	mustGalleryExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		fx.campaignID, "Gallery Leak Test", "gallery-leak-"+fx.campaignID[:8], fx.ownerID)
	return fx
}

func (fx *galleryFixture) cleanup() {
	// CASCADE on campaigns clears entities/entity_types/entity_permissions.
	mustGalleryExec(fx.t, fx.db, `DELETE FROM campaigns WHERE id = ?`, fx.campaignID)
	mustGalleryExec(fx.t, fx.db, `DELETE FROM users WHERE id IN (?, ?)`, fx.ownerID, fx.otherUserID)
}

// entityService builds the REAL entities.EntityService (repository +
// service, no fakes) — the same construction routes.go uses at boot.
func (fx *galleryFixture) entityService() entities.EntityService {
	entityRepo := entities.NewEntityRepository(fx.db)
	entityTypeRepo := entities.NewEntityTypeRepository(fx.db)
	entityPermRepo := entities.NewEntityPermissionRepository(fx.db)
	return entities.NewEntityService(entityRepo, entityTypeRepo, entityPermRepo)
}

// entityType inserts a minimal entity_types row and returns its ID.
func (fx *galleryFixture) entityType(slug, name, presetCategory, extraSlugForNPC string) int {
	fx.t.Helper()
	s := slug
	if extraSlugForNPC != "" {
		s = extraSlugForNPC
	}
	var presetVal any
	if presetCategory != "" {
		presetVal = presetCategory
	}
	res, err := fx.db.Exec(
		`INSERT INTO entity_types (campaign_id, slug, name, name_plural, preset_category) VALUES (?, ?, ?, ?, ?)`,
		fx.campaignID, s, name, name+"s", presetVal,
	)
	if err != nil {
		fx.t.Fatalf("insert entity_types: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		fx.t.Fatalf("entity_types last insert id: %v", err)
	}
	return int(id)
}

// entity inserts a minimal entities row with the given visibility mode and
// is_private=false — the exact shape of the bug: a "custom" entity that was
// never given is_private=true, because SetEntityPermissions never touches
// that column on the custom branch.
func (fx *galleryFixture) entity(entityTypeID int, name, slug, visibility string) string {
	fx.t.Helper()
	id := galleryTestUUID(fx.t)
	mustGalleryExec(fx.t, fx.db,
		`INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, visibility, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, false, ?, ?, NOW(), NOW())`,
		id, fx.campaignID, entityTypeID, name, slug, visibility, fx.ownerID,
	)
	return id
}

// grantView inserts an entity_permissions row. Used to restrict a
// visibility='custom' entity to a subject that is NEVER the viewer under
// test, so the entity is reachable only by Owner (which bypasses the filter
// entirely) and by that named subject.
func (fx *galleryFixture) grantView(entityID, subjectType, subjectID string) {
	fx.t.Helper()
	mustGalleryExec(fx.t, fx.db,
		`INSERT INTO entity_permissions (entity_id, subject_type, subject_id, permission) VALUES (?, ?, ?, 'view')`,
		entityID, subjectType, subjectID,
	)
}

// openGalleryTestDB returns a scratch schema with core migrations applied
// (the galleries read only core tables), dropped on cleanup. It never uses
// the DSN's own database: test-int-local's DSN names none.
func openGalleryTestDB(t *testing.T) *sql.DB {
	t.Helper()

	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		cfg := mysql.NewConfig()
		cfg.User = galleryGetenvDefault("DB_USER", "chronicle")
		cfg.Passwd = galleryGetenvDefault("DB_PASSWORD", "chronicle")
		cfg.Net = "tcp"
		cfg.Addr = galleryGetenvDefault("DB_HOST", "127.0.0.1:3306")
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
		t.Skipf("no test DB server reachable at %s: %v — run `make docker-up` or `make test-db-up`", cfg.Addr, err)
	}

	name := fmt.Sprintf("chronicle_gal_%06d", mrand.Intn(1000000)) //nolint:gosec // test schema name
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

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	if err := database.RunMigrations(db, scratchCfg.FormatDSN(), filepath.Join(root, "db", "migrations")); err != nil {
		t.Skipf("core migrations did not apply: %v", err)
	}
	return db
}

func galleryGetenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustGalleryExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func galleryTestUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
