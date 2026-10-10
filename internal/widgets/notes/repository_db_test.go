package notes

// Real-MariaDB tests for the notes repository. The visibility predicate is
// written twice, as Note.CanView in Go and visibleFilter in SQL; these tests
// run both over the same rows so the two can never disagree.
//
// Each test gets its own scratch schema, migrated with the core migrations,
// and drops it on cleanup. Skips when no server is reachable:
//
//	make test-db-up
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/widgets/notes/ -run TestDB

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func newNotesScratchDB(t *testing.T) *sql.DB {
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

	name := fmt.Sprintf("chronicle_notes_%06d", rand.Intn(1000000)) //nolint:gosec // test schema name
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

func newUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		t.Fatalf("random id: %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// seedNotesCampaign inserts a campaign and the named users; returns the
// campaign id and a name -> user id map.
func seedNotesCampaign(t *testing.T, db *sql.DB, names ...string) (string, map[string]string) {
	t.Helper()
	users := map[string]string{}
	for _, n := range names {
		id := newUUID(t)
		if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`,
			id, id+"@example.test", n, "x"); err != nil {
			t.Fatalf("seed user %s: %v", n, err)
		}
		users[n] = id
	}
	camp := newUUID(t)
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`,
		camp, "Table", camp, users[names[0]]); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return camp, users
}

// TestDB_ListVisibilityMatchesCanView writes one note per audience and asks,
// for every viewer, whether the SQL list and CanView agree on every row.
func TestDB_ListVisibilityMatchesCanView(t *testing.T) {
	db := newNotesScratchDB(t)
	ctx := context.Background()
	repo := NewNoteRepository(db)
	camp, u := seedNotesCampaign(t, db, "gm", "ana", "bo", "cy")

	mk := func(owner string, mutate func(n *Note)) *Note {
		n := &Note{ID: newUUID(t), CampaignID: camp, UserID: u[owner], Title: owner + " note", Content: []Block{}, Color: "#374151"}
		mutate(n)
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("create: %v", err)
		}
		return n
	}
	mk("ana", func(n *Note) {})                                            // private
	mk("ana", func(n *Note) { n.IsShared = true })                         // party
	mk("ana", func(n *Note) { n.SharedWith = []string{u["bo"]} })          // custom: bo
	mk("ana", func(n *Note) { n.SharedWithGM = true })                     // GM
	mk("gm", func(n *Note) {})                                             // the GM's private note
	mk("gm", func(n *Note) { n.SharedWithGM = true })                      // GM note by the GM
	mk("cy", func(n *Note) { n.SharedWith = []string{u["ana"], u["bo"]} }) // custom: ana + bo

	all, err := db.QueryContext(ctx, `SELECT id FROM notes WHERE campaign_id = ?`, camp)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for all.Next() {
		var id string
		_ = all.Scan(&id)
		ids = append(ids, id)
	}
	all.Close()

	viewers := map[string]permissions.Viewer{
		"gm (Owner)":  permissions.RequestViewer(permissions.RoleOwner, u["gm"]),
		"ana":         permissions.RequestViewer(permissions.RolePlayer, u["ana"]),
		"bo":          permissions.RequestViewer(permissions.RolePlayer, u["bo"]),
		"cy (Scribe)": permissions.RequestViewer(permissions.RoleScribe, u["cy"]),
	}
	for name, v := range viewers {
		t.Run(name, func(t *testing.T) {
			listed, err := repo.ListVisible(ctx, camp, v, ListScope{})
			if err != nil {
				t.Fatalf("ListVisible: %v", err)
			}
			got := map[string]bool{}
			for _, n := range listed {
				got[n.ID] = true
			}
			for _, id := range ids {
				n, err := repo.FindByID(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if want := n.CanView(v, camp); got[id] != want {
					t.Errorf("%q (%s): SQL lists it %v, CanView says %v", n.Title, n.Visibility, got[id], want)
				}
			}
		})
	}

	// The spot checks that matter most, stated outright.
	gmList, _ := repo.ListVisible(ctx, camp, viewers["gm (Owner)"], ListScope{})
	for _, n := range gmList {
		if n.UserID == u["ana"] && n.Visibility == VisibilityPrivate {
			t.Error("the GM must not see a player's private note")
		}
	}
	anon, _ := repo.ListVisible(ctx, camp, permissions.RequestViewer(permissions.RolePlayer, ""), ListScope{})
	if len(anon) != 0 {
		t.Errorf("an anonymous viewer must see nothing, got %d notes", len(anon))
	}
}

// TestDB_NewColumnsRoundTrip: the Journal columns survive a write and read,
// and existing-style rows read as active and not shared with the GM.
func TestDB_NewColumnsRoundTrip(t *testing.T) {
	db := newNotesScratchDB(t)
	ctx := context.Background()
	repo := NewNoteRepository(db)
	camp, u := seedNotesCampaign(t, db, "gm")

	// A row inserted the way the previous release wrote it: no new columns.
	legacy := newUUID(t)
	if _, err := db.Exec(`INSERT INTO notes (id, campaign_id, user_id, title, content, is_shared) VALUES (?,?,?,?,?,TRUE)`,
		legacy, camp, u["gm"], "Old shared note", "[]"); err != nil {
		t.Fatalf("legacy insert: %v", err)
	}
	old, err := repo.FindByID(ctx, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if old.Visibility != VisibilityParty || old.Archived || old.SharedWithGM || old.LinkedNoteID != nil {
		t.Errorf("an existing shared note must read as party, active, unlinked: %+v", old)
	}

	archived := time.Now().UTC().Truncate(time.Second)
	n := &Note{ID: newUUID(t), CampaignID: camp, UserID: u["gm"], Title: "t", Content: []Block{}, Color: "#374151",
		SharedWithGM: true, ArchivedAt: &archived, LinkedNoteID: &legacy}
	if err := repo.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByID(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Visibility != VisibilityGM || !got.Archived || got.LinkedNoteID == nil || *got.LinkedNoteID != legacy {
		t.Errorf("round trip lost a column: %+v", got)
	}
}

// TestDB_ScopesAndTree covers the list scopes and the unfiltered tree
// helpers the folder rules use.
func TestDB_ScopesAndTree(t *testing.T) {
	db := newNotesScratchDB(t)
	ctx := context.Background()
	repo := NewNoteRepository(db)
	camp, u := seedNotesCampaign(t, db, "gm")
	owner := permissions.RequestViewer(permissions.RoleOwner, u["gm"])

	page := newUUID(t)
	folder := &Note{ID: newUUID(t), CampaignID: camp, UserID: u["gm"], Title: "Folder", IsFolder: true, Content: []Block{}, Color: "#374151"}
	child := &Note{ID: newUUID(t), CampaignID: camp, UserID: u["gm"], Title: "Child", ParentID: &folder.ID, Content: []Block{}, Color: "#374151"}
	jot := &Note{ID: newUUID(t), CampaignID: camp, UserID: u["gm"], Title: "Jot", EntityID: &page, Content: []Block{}, Color: "#374151"}
	for _, n := range []*Note{folder, child, jot} {
		if err := repo.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	titles := func(scope ListScope) []string {
		ns, err := repo.ListVisible(ctx, camp, owner, scope)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, n := range ns {
			out = append(out, n.Title)
		}
		sort.Strings(out)
		return out
	}
	if got := titles(ListScope{Kind: "campaign"}); fmt.Sprint(got) != "[Child Folder]" {
		t.Errorf("campaign scope = %v, want the Journal notes and folders", got)
	}
	if got := titles(ListScope{Kind: "jots"}); fmt.Sprint(got) != "[Jot]" {
		t.Errorf("jots scope = %v", got)
	}
	if got := titles(ListScope{Kind: "entity", EntityID: page}); fmt.Sprint(got) != "[Jot]" {
		t.Errorf("entity scope = %v", got)
	}

	tree, err := repo.ListTree(ctx, camp)
	if err != nil || len(tree) != 3 {
		t.Fatalf("tree = %v, %v", tree, err)
	}
	if err := repo.ReparentToTop(ctx, []string{child.ID}); err != nil {
		t.Fatal(err)
	}
	moved, _ := repo.FindByID(ctx, child.ID)
	if moved.ParentID != nil {
		t.Error("ReparentToTop must clear parent_id")
	}
}

// TestDB_LinkQueries covers the queries behind backlinks, page references,
// link labels and the has-audio flag.
func TestDB_LinkQueries(t *testing.T) {
	db := newNotesScratchDB(t)
	ctx := context.Background()
	repo := NewNoteRepository(db)
	camp, u := seedNotesCampaign(t, db, "gm", "ana")

	target := &Note{ID: newUUID(t), CampaignID: camp, UserID: u["gm"], Title: "Thalrik", Content: []Block{}, Color: "#374151", IsShared: true}
	page := newUUID(t)
	shared := `<p>` + `<a data-note-id="` + target.ID + `">Journal note</a> and <a data-mention-id="` + page + `">@Ashkeep</a></p>`
	linkingParty := &Note{ID: newUUID(t), CampaignID: camp, UserID: u["gm"], Title: "Session 12", Content: []Block{}, Color: "#374151", IsShared: true, EntryHTML: &shared}
	linkingPrivate := &Note{ID: newUUID(t), CampaignID: camp, UserID: u["ana"], Title: "Ana's doubts", Content: []Block{}, Color: "#374151", EntryHTML: &shared}
	for _, n := range []*Note{target, linkingParty, linkingPrivate} {
		if err := repo.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	gmViewer := permissions.RequestViewer(permissions.RoleOwner, u["gm"])
	got, err := repo.ListVisibleLinking(ctx, camp, gmViewer, LinkNote, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != linkingParty.ID {
		t.Errorf("the GM's backlinks = %v; ana's private note must not appear", got)
	}
	anaViewer := permissions.RequestViewer(permissions.RolePlayer, u["ana"])
	if got, _ := repo.ListVisibleLinking(ctx, camp, anaViewer, LinkPage, page); len(got) != 2 {
		t.Errorf("ana sees both notes that mention the page, got %d", len(got))
	}
	if got, _ := repo.ListVisibleLinking(ctx, camp, gmViewer, LinkNote, `%" OR 1=1 -- `); got != nil {
		t.Error("a non-id target must query nothing")
	}

	found, err := repo.FindByIDs(ctx, []string{target.ID, linkingPrivate.ID, newUUID(t)})
	if err != nil || len(found) != 2 {
		t.Errorf("FindByIDs = %d rows, %v", len(found), err)
	}

	if _, err := db.Exec(`INSERT INTO note_attachments (id, note_id, campaign_id, file_path, original_name, mime_type, file_size) VALUES (?,?,?,?,?,?,?)`,
		newUUID(t), linkingParty.ID, camp, "a.mp3", "a.mp3", "audio/mpeg", 10); err != nil {
		t.Fatal(err)
	}
	withAudio, err := NewAttachmentRepository(db).NotesWithAttachments(ctx, camp)
	if err != nil || !withAudio[linkingParty.ID] || withAudio[target.ID] {
		t.Errorf("NotesWithAttachments = %v, %v", withAudio, err)
	}
}

// TestDB_ViewerReadsMediaMatchesCanView: whether a viewer reads a picture is
// whether they can read some note holding it, for every audience, and a note
// that only looks like it holds the picture (another id, another campaign, a
// wildcard) opens nothing.
func TestDB_ViewerReadsMediaMatchesCanView(t *testing.T) {
	db := newNotesScratchDB(t)
	ctx := context.Background()
	repo := NewNoteRepository(db)
	camp, u := seedNotesCampaign(t, db, "gm", "ana", "bo", "cy")
	otherCamp, _ := seedNotesCampaign(t, db, "dee")

	pic := newUUID(t)
	ana := permissions.RequestViewer(permissions.RolePlayer, u["ana"]) // uploaded the picture
	body := func(id string) *string {
		s := `<figure class="ce-img ce-img--w50 ce-img--center"><img src="/media/` + id + `" alt=""></figure>`
		return &s
	}
	mk := func(campaign, owner string, html *string, mutate func(n *Note)) {
		n := &Note{ID: newUUID(t), CampaignID: campaign, UserID: owner, Title: "n", Content: []Block{}, Color: "#374151", EntryHTML: html}
		mutate(n)
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	tests := []struct {
		name  string
		setup func()
		want  map[string]bool // viewer -> reads the picture
	}{
		{"private note: owner only, not even the GM", func() { mk(camp, u["ana"], body(pic), func(n *Note) {}) },
			map[string]bool{"ana": true, "gm": false, "bo": false, "cy": false}},
		{"party note", func() { mk(camp, u["ana"], body(pic), func(n *Note) { n.IsShared = true }) },
			map[string]bool{"ana": true, "gm": true, "bo": true, "cy": true}},
		{"note named to one person", func() { mk(camp, u["ana"], body(pic), func(n *Note) { n.SharedWith = []string{u["bo"]} }) },
			map[string]bool{"ana": true, "gm": false, "bo": true, "cy": false}},
		{"note shared with the GM", func() { mk(camp, u["ana"], body(pic), func(n *Note) { n.SharedWithGM = true }) },
			map[string]bool{"ana": true, "gm": true, "bo": false, "cy": false}},
		{"bo pastes ana's picture into bo's own private note", func() { mk(camp, u["bo"], body(pic), func(n *Note) {}) },
			map[string]bool{"ana": false, "gm": false, "bo": false, "cy": false}},
		{"bo pastes it into a party note ana can read", func() { mk(camp, u["bo"], body(pic), func(n *Note) { n.IsShared = true }) },
			map[string]bool{"ana": true, "gm": true, "bo": true, "cy": true}},
		{"bo pastes it into a note shared only with cy", func() { mk(camp, u["bo"], body(pic), func(n *Note) { n.SharedWith = []string{u["cy"]} }) },
			map[string]bool{"ana": false, "gm": false, "bo": false, "cy": false}},
		{"a private note holding some other picture", func() { mk(camp, u["ana"], body(newUUID(t)), func(n *Note) { n.IsShared = true }) },
			map[string]bool{"ana": false, "gm": false, "bo": false, "cy": false}},
		{"a party note in another campaign", func() { mk(otherCamp, u["ana"], body(pic), func(n *Note) { n.IsShared = true }) },
			map[string]bool{"ana": false, "gm": false, "bo": false, "cy": false}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.Exec(`DELETE FROM notes`); err != nil {
				t.Fatal(err)
			}
			tc.setup()
			for who, want := range tc.want {
				role := permissions.RolePlayer
				if who == "gm" {
					role = permissions.RoleOwner
				}
				got, err := repo.ViewerReadsMedia(ctx, camp, pic, permissions.RequestViewer(role, u[who]), ana)
				if err != nil {
					t.Fatalf("%s: %v", who, err)
				}
				if got != want {
					t.Errorf("%s reads the picture = %v, want %v", who, got, want)
				}
			}
		})
	}

	t.Run("anonymous and malformed ids read nothing", func(t *testing.T) {
		if _, err := db.Exec(`DELETE FROM notes`); err != nil {
			t.Fatal(err)
		}
		mk(camp, u["ana"], body(pic), func(n *Note) { n.IsShared = true })
		if ok, _ := repo.ViewerReadsMedia(ctx, camp, pic, permissions.RequestViewer(permissions.RolePlayer, ""), ana); ok {
			t.Error("an anonymous viewer read a party note's picture")
		}
		for _, bad := range []string{"%", "%%%%%%%%", `a" OR 1=1 -- `, ""} {
			if ok, _ := repo.ViewerReadsMedia(ctx, camp, bad, permissions.RequestViewer(permissions.RoleOwner, u["gm"]), ana); ok {
				t.Errorf("id %q matched a note", bad)
			}
		}
	})
}
