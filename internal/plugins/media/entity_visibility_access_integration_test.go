// entity_visibility_access_integration_test.go drives ADR-058's rule against
// a REAL MariaDB, through the REAL production components on both sides of
// the seam it wires together — never a reimplementation: media's
// FindReferences (whose UNION must include cover_image_path or a cover
// image looks unreferenced) and entities' FilterViewableEntityIDs (the
// visibility predicate sessions/relations/armory/npcs all call, reached via
// the local dbEntityVisibility adapter), both exercised through the real
// Handler.checkMediaAccess.
//
// Skips (never fails) when no database answers; uses a throwaway
// fully-migrated schema per run, never the shared dev database.
package media

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// newADR058ScratchDB creates a fresh, fully-migrated throwaway schema
// (core migrations only — entities and media tables both live in
// db/migrations/000001_baseline.up.sql) and drops it on cleanup. Skips
// rather than fails when no server answers.
func newADR058ScratchDB(t *testing.T) *sql.DB {
	t.Helper()

	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN (make test-db-up) to run the real-DB ADR-058 tests")
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

	name := fmt.Sprintf("chronicle_media_adr058_%06d", rand.Intn(1000000)) //nolint:gosec // test schema name
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

// --- Fixtures ---

func adr058DBID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		t.Fatalf("random id: %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// seedADR058Campaign inserts a user + a PRIVATE campaign (is_public
// defaults to FALSE per the baseline schema) and returns their ids.
func seedADR058Campaign(t *testing.T, db *sql.DB) (campaignID, ownerUserID string) {
	t.Helper()
	ownerUserID, campaignID = adr058DBID(t), adr058DBID(t)
	mustADR058Exec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`,
		ownerUserID, ownerUserID+"@example.test", "Owner", "x")
	mustADR058Exec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`,
		campaignID, "ADR-058 Test Campaign", campaignID, ownerUserID)
	return campaignID, ownerUserID
}

func seedADR058User(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	id := adr058DBID(t)
	mustADR058Exec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`,
		id, id+"@example.test", name, "x")
	return id
}

// seedADR058Member adds userID to campaignID with the given role
// ("owner"/"scribe"/"player" per the campaign_members CHECK constraint).
func seedADR058Member(t *testing.T, db *sql.DB, campaignID, userID, role string) {
	t.Helper()
	mustADR058Exec(t, db, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?,?,?)`,
		campaignID, userID, role)
}

// seedADR058DmGrant marks userID as co-DM/dm_only-granted in campaignID's
// settings JSON, the same shape campaigns.CampaignSettings.DmGrantIDs
// (de)serializes.
func seedADR058DmGrant(t *testing.T, db *sql.DB, campaignID, userID string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"dm_grant_ids": []string{userID}})
	if err != nil {
		t.Fatalf("marshal dm grant settings: %v", err)
	}
	mustADR058Exec(t, db, `UPDATE campaigns SET settings = ? WHERE id = ?`, string(payload), campaignID)
}

// seedADR058EntityType inserts a minimal entity type and returns its
// auto-generated id.
func seedADR058EntityType(t *testing.T, db *sql.DB, campaignID string) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?,?,?,?)`,
		campaignID, "int-npc", "NPC", "NPCs")
	if err != nil {
		t.Fatalf("seed entity type: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("entity type last insert id: %v", err)
	}
	return int(id)
}

// adr058Entity describes one row to insert into entities for a test.
type adr058Entity struct {
	Name           string
	IsPrivate      bool
	ImagePath      *string
	CoverImagePath *string
	EntryHTML      *string
}

// seedADR058Entity inserts an entity with full column control (raw SQL,
// not entities.Repository.Create, because Create's INSERT list omits
// cover_image_path entirely — see repository.go's own Create). Returns
// the new entity's id.
func seedADR058Entity(t *testing.T, db *sql.DB, campaignID string, entityTypeID int, createdBy string, e adr058Entity) string {
	t.Helper()
	id := adr058DBID(t)
	slug := "int-" + id[:8]
	mustADR058Exec(t, db, `INSERT INTO entities
		(id, campaign_id, entity_type_id, name, slug, entry_html, image_path, cover_image_path, is_private, created_by, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,NOW(),NOW())`,
		id, campaignID, entityTypeID, e.Name, slug, e.EntryHTML, e.ImagePath, e.CoverImagePath, e.IsPrivate, createdBy)
	return id
}

// seedADR058MediaFile inserts a media_files row scoped to campaignID and
// returns its id.
func seedADR058MediaFile(t *testing.T, db *sql.DB, campaignID, uploadedBy string) string {
	t.Helper()
	id := adr058DBID(t)
	mustADR058Exec(t, db, `INSERT INTO media_files
		(id, campaign_id, uploaded_by, filename, original_name, mime_type, file_size, usage_type)
		VALUES (?,?,?,?,?,?,?,?)`,
		id, campaignID, uploadedBy, id+".png", "test.png", "image/png", 1024, UsageEntityImage)
	return id
}

func mustADR058Exec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// --- dbMemberChecker: a plain-SQL MemberChecker for these tests only ---

// dbMemberChecker reads campaign_members and campaigns.settings directly
// with SQL — see this file's header for why membership/DM-grant are a
// deliberate simplification here rather than routed through
// campaigns.CampaignService.
type dbMemberChecker struct {
	db *sql.DB
}

func (m *dbMemberChecker) IsCampaignMember(campaignID, userID string) bool {
	return m.MemberRole(campaignID, userID) > 0
}

func (m *dbMemberChecker) MemberRole(campaignID, userID string) int {
	var role string
	err := m.db.QueryRow(`SELECT role FROM campaign_members WHERE campaign_id = ? AND user_id = ?`, campaignID, userID).Scan(&role)
	if err != nil {
		return int(campaigns.RoleNone)
	}
	switch role {
	case "owner":
		return int(campaigns.RoleOwner)
	case "scribe":
		return int(campaigns.RoleScribe)
	case "player":
		return int(campaigns.RolePlayer)
	default:
		return int(campaigns.RoleNone)
	}
}

func (m *dbMemberChecker) IsUserDmGranted(campaignID, userID string) bool {
	var settingsJSON sql.NullString
	if err := m.db.QueryRow(`SELECT settings FROM campaigns WHERE id = ?`, campaignID).Scan(&settingsJSON); err != nil {
		return false
	}
	if !settingsJSON.Valid || settingsJSON.String == "" {
		return false
	}
	var parsed struct {
		DmGrantIDs []string `json:"dm_grant_ids"`
	}
	if err := json.Unmarshal([]byte(settingsJSON.String), &parsed); err != nil {
		return false
	}
	for _, id := range parsed.DmGrantIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// --- dbEntityVisibility: the REAL predicate, wrapped to satisfy media's seam ---

// dbEntityVisibility wraps the real entities.EntityRepository so the
// production predicate — the one this ADR requires media to consult
// instead of writing a fourth copy of — is what these tests actually run.
type dbEntityVisibility struct {
	repo entities.EntityRepository
}

func (d *dbEntityVisibility) FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error) {
	return d.repo.FilterViewableEntityIDs(ctx, campaignID, entityIDs, role, userID)
}

// newADR058RealHandler wires a Handler whose reference lookup AND
// visibility filter are both backed by the real db.
func newADR058RealHandler(db *sql.DB) *Handler {
	mediaRepo := NewMediaRepository(db)
	mediaSvc := NewMediaService(mediaRepo, "", 0) // FilePath/disk never touched by checkMediaAccess
	return &Handler{
		service:          mediaSvc,
		entityVisibility: &dbEntityVisibility{repo: entities.NewEntityRepository(db)},
		memberChecker:    &dbMemberChecker{db: db},
	}
}

// --- Tests ---

func TestADR058Integration_HiddenEntityImage_PlayerDenied(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := newADR058ScratchDB(t)
	campaignID, owner := seedADR058Campaign(t, db)
	player := seedADR058User(t, db, "Player")
	seedADR058Member(t, db, campaignID, player, "player")
	typeID := seedADR058EntityType(t, db, campaignID)

	fileID := seedADR058MediaFile(t, db, campaignID, owner)
	seedADR058Entity(t, db, campaignID, typeID, owner, adr058Entity{
		Name: "Secret Villain", IsPrivate: true, ImagePath: &fileID,
	})

	h := newADR058RealHandler(db)
	mediaCampaignID := campaignID
	file := &MediaFile{ID: fileID, CampaignID: &mediaCampaignID, CampaignIsPublic: boolPtr(false)}
	c := newADR058TestContext(player)

	mustDeny(t, accessErr(h.checkMediaAccess(c, file, false, "")), "[real DB] Player reading an image used only by a dm_only entity")
}

func TestADR058Integration_VisibleEntityImage_PlayerAllowed(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := newADR058ScratchDB(t)
	campaignID, owner := seedADR058Campaign(t, db)
	player := seedADR058User(t, db, "Player")
	seedADR058Member(t, db, campaignID, player, "player")
	typeID := seedADR058EntityType(t, db, campaignID)

	fileID := seedADR058MediaFile(t, db, campaignID, owner)
	seedADR058Entity(t, db, campaignID, typeID, owner, adr058Entity{
		Name: "Town Square", IsPrivate: false, ImagePath: &fileID,
	})

	h := newADR058RealHandler(db)
	mediaCampaignID := campaignID
	file := &MediaFile{ID: fileID, CampaignID: &mediaCampaignID, CampaignIsPublic: boolPtr(false)}
	c := newADR058TestContext(player)

	mustAllow(t, accessErr(h.checkMediaAccess(c, file, false, "")), "[real DB] Player reading an image used by a visible entity")
}

// TestADR058Integration_SharedHiddenAndVisible_ReadableAtAll is decision 1
// against the real predicate: ONE file, referenced by BOTH a dm_only
// entity and a visible one, must stay readable.
func TestADR058Integration_SharedHiddenAndVisible_ReadableAtAll(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := newADR058ScratchDB(t)
	campaignID, owner := seedADR058Campaign(t, db)
	player := seedADR058User(t, db, "Player")
	seedADR058Member(t, db, campaignID, player, "player")
	typeID := seedADR058EntityType(t, db, campaignID)

	fileID := seedADR058MediaFile(t, db, campaignID, owner)
	htmlRef := fmt.Sprintf(`<p>see <img src="/media/%s"></p>`, fileID)
	seedADR058Entity(t, db, campaignID, typeID, owner, adr058Entity{
		Name: "Secret Villain", IsPrivate: true, ImagePath: &fileID,
	})
	seedADR058Entity(t, db, campaignID, typeID, owner, adr058Entity{
		Name: "Town Square", IsPrivate: false, EntryHTML: &htmlRef,
	})

	h := newADR058RealHandler(db)
	mediaCampaignID := campaignID
	file := &MediaFile{ID: fileID, CampaignID: &mediaCampaignID, CampaignIsPublic: boolPtr(false)}
	c := newADR058TestContext(player)

	mustAllow(t, accessErr(h.checkMediaAccess(c, file, false, "")), "[real DB] image shared by a hidden entity's image_path AND a visible entity's entry_html")
}

// TestADR058Integration_CoverImageOnly_PlayerDenied proves, against the
// real repository query, that a file referenced ONLY via cover_image_path
// on a dm_only entity is denied — FindReferences must include that column
// or the file looks unreferenced and leaks via the plain-membership grant.
func TestADR058Integration_CoverImageOnly_PlayerDenied(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := newADR058ScratchDB(t)
	campaignID, owner := seedADR058Campaign(t, db)
	player := seedADR058User(t, db, "Player")
	seedADR058Member(t, db, campaignID, player, "player")
	typeID := seedADR058EntityType(t, db, campaignID)

	fileID := seedADR058MediaFile(t, db, campaignID, owner)
	seedADR058Entity(t, db, campaignID, typeID, owner, adr058Entity{
		Name: "Secret Lair", IsPrivate: true, CoverImagePath: &fileID,
	})

	// FindReferences must report this entity for a cover-only reference.
	refs, err := NewMediaRepository(db).FindReferences(context.Background(), campaignID, fileID)
	if err != nil {
		t.Fatalf("FindReferences: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("FindReferences must report the cover-image reference; got %d refs: %+v", len(refs), refs)
	}

	h := newADR058RealHandler(db)
	mediaCampaignID := campaignID
	file := &MediaFile{ID: fileID, CampaignID: &mediaCampaignID, CampaignIsPublic: boolPtr(false)}
	c := newADR058TestContext(player)

	mustDeny(t, accessErr(h.checkMediaAccess(c, file, false, "")), "[real DB] Player reading a COVER image used only by a dm_only entity")
}

// TestADR058Integration_PathFormCoverImage_PlayerDenied: an image field
// stored in disk-path form ("2026/09/<id>.png", as an import or a restored
// backup might write it) instead of the bare media ID must still be found
// by FindReferences; otherwise the file looks unreferenced and
// checkMediaAccess falls back to the role-blind membership grant ADR-058
// rules out.
func TestADR058Integration_PathFormCoverImage_PlayerDenied(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := newADR058ScratchDB(t)
	campaignID, owner := seedADR058Campaign(t, db)
	player := seedADR058User(t, db, "Player")
	seedADR058Member(t, db, campaignID, player, "player")
	typeID := seedADR058EntityType(t, db, campaignID)

	fileID := seedADR058MediaFile(t, db, campaignID, owner)
	pathForm := "2026/09/" + fileID + ".png"
	seedADR058Entity(t, db, campaignID, typeID, owner, adr058Entity{
		Name: "Secret Lair", IsPrivate: true, CoverImagePath: &pathForm,
	})

	// FindReferences must report this entity even though the stored
	// value is the disk-path form, not the bare id.
	refs, err := NewMediaRepository(db).FindReferences(context.Background(), campaignID, fileID)
	if err != nil {
		t.Fatalf("FindReferences: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("FindReferences must match the path-form cover image reference; got %d refs: %+v", len(refs), refs)
	}

	h := newADR058RealHandler(db)
	mediaCampaignID := campaignID
	file := &MediaFile{ID: fileID, CampaignID: &mediaCampaignID, CampaignIsPublic: boolPtr(false)}
	c := newADR058TestContext(player)

	mustDeny(t, accessErr(h.checkMediaAccess(c, file, false, "")), "[real DB] Player reading a path-form COVER image used only by a dm_only entity")
}

// TestADR058Integration_CoDM_SeesWhatDMSees drives the real promotion
// path: a co-DM (player role + a real campaigns.settings dm_grant_ids
// entry) reading a hidden entity's image.
func TestADR058Integration_CoDM_SeesWhatDMSees(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := newADR058ScratchDB(t)
	campaignID, owner := seedADR058Campaign(t, db)
	coDM := seedADR058User(t, db, "CoDM")
	seedADR058Member(t, db, campaignID, coDM, "player")
	seedADR058DmGrant(t, db, campaignID, coDM)
	typeID := seedADR058EntityType(t, db, campaignID)

	fileID := seedADR058MediaFile(t, db, campaignID, owner)
	seedADR058Entity(t, db, campaignID, typeID, owner, adr058Entity{
		Name: "Secret Villain", IsPrivate: true, ImagePath: &fileID,
	})

	h := newADR058RealHandler(db)
	mediaCampaignID := campaignID
	file := &MediaFile{ID: fileID, CampaignID: &mediaCampaignID, CampaignIsPublic: boolPtr(false)}
	c := newADR058TestContext(coDM)

	mustAllow(t, accessErr(h.checkMediaAccess(c, file, false, "")), "[real DB] co-DM reading a dm_only entity's image")
}

// TestADR058Integration_Unreferenced_MembershipStillApplies confirms a
// file no entity references at all (an avatar-shaped upload) still works
// via plain membership against the real DB.
func TestADR058Integration_Unreferenced_MembershipStillApplies(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := newADR058ScratchDB(t)
	campaignID, owner := seedADR058Campaign(t, db)
	player := seedADR058User(t, db, "Player")
	seedADR058Member(t, db, campaignID, player, "player")

	fileID := seedADR058MediaFile(t, db, campaignID, owner) // no entity ever references it

	h := newADR058RealHandler(db)
	mediaCampaignID := campaignID
	file := &MediaFile{ID: fileID, CampaignID: &mediaCampaignID, CampaignIsPublic: boolPtr(false)}
	c := newADR058TestContext(player)

	mustAllow(t, accessErr(h.checkMediaAccess(c, file, false, "")), "[real DB] unreferenced file via plain membership")
}

// TestADR058Integration_ReferenceLookupError_Denies forces a REAL query
// error (the scratch DB connection is closed before the call) and
// confirms checkMediaAccess still denies rather than defaulting to
// "unreferenced ⇒ membership" or any other lenient guess.
func TestADR058Integration_ReferenceLookupError_Denies(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := newADR058ScratchDB(t)
	campaignID, owner := seedADR058Campaign(t, db)
	player := seedADR058User(t, db, "Player")
	seedADR058Member(t, db, campaignID, player, "player")
	fileID := seedADR058MediaFile(t, db, campaignID, owner)

	h := newADR058RealHandler(db)
	db.Close() // every subsequent query now fails for real

	mediaCampaignID := campaignID
	file := &MediaFile{ID: fileID, CampaignID: &mediaCampaignID, CampaignIsPublic: boolPtr(false)}
	c := newADR058TestContext(player)

	mustDeny(t, accessErr(h.checkMediaAccess(c, file, false, "")), "[real DB, connection closed] reference lookup failure must deny, not grant via membership")
}
