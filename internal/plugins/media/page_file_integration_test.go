// page_file_integration_test.go drives the page file rule against a REAL
// MariaDB through the production pieces on both sides of the seam: media's
// PageFileService, repositories and upload pipeline, and the entities service
// (its visibility predicate, edit rule and Trash) that decides who may see and
// change a page. Only the adapter between them (a few lines in app/routes.go)
// is restated here.
//
// Skips when no database answers; see newADR058ScratchDB.
package media

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// dbPageAccess is the production pageAccessAdapter, restated: visibility from
// FilterViewableEntityIDs (which scopes to the campaign and leaves out the
// Trash), then the page's own edit rule.
type dbPageAccess struct{ svc entities.EntityService }

func (a dbPageAccess) PageAccess(ctx context.Context, campaignID, entityID string, role int, userID string) (PageAccess, error) {
	viewable, err := a.svc.FilterViewableEntityIDs(ctx, campaignID, []string{entityID}, role, userID)
	if err != nil || !viewable[entityID] {
		return PageAccess{}, err
	}
	ep, err := a.svc.CheckEntityAccess(ctx, entityID, role, userID)
	if err != nil {
		return PageAccess{}, err
	}
	return PageAccess{CanView: true, CanEdit: ep.CanEdit}, nil
}

// pageFileDB is a campaign with the cast and pages the access tests need.
type pageFileDB struct {
	db   *sql.DB
	svc  PageFileService
	repo MediaRepository
	mc   *dbMemberChecker
	camp string

	owner, scribe, editor, viewer, coDM, stranger string
	open, shared, private, trashed               string
	otherCamp, otherPage                         string
}

func newPageFileDB(t *testing.T) *pageFileDB {
	t.Helper()
	db := newADR058ScratchDB(t)
	p := &pageFileDB{db: db, mc: &dbMemberChecker{db: db}}
	var gm string
	p.camp, gm = seedADR058Campaign(t, db)
	p.owner = gm
	seedADR058Member(t, db, p.camp, p.owner, "owner")
	p.scribe, p.editor, p.viewer, p.coDM = seedADR058User(t, db, "Scribe"), seedADR058User(t, db, "Editor"), seedADR058User(t, db, "Viewer"), seedADR058User(t, db, "CoDM")
	seedADR058Member(t, db, p.camp, p.scribe, "scribe")
	for _, id := range []string{p.editor, p.viewer, p.coDM} {
		seedADR058Member(t, db, p.camp, id, "player")
	}
	seedADR058DmGrant(t, db, p.camp, p.coDM)
	p.stranger = seedADR058User(t, db, "Stranger") // in no campaign here

	typeID := seedADR058EntityType(t, db, p.camp)
	page := func(name string, private bool) string {
		return seedADR058Entity(t, db, p.camp, typeID, p.owner, adr058Entity{Name: name, IsPrivate: private})
	}
	p.open, p.shared, p.private, p.trashed = page("Open", false), page("Shared", false), page("Private", true), page("Trashed", false)

	// "Shared" is custom: the editor may edit, the viewer may only read, and
	// scribes may edit through a role grant.
	mustADR058Exec(t, db, `UPDATE entities SET visibility = 'custom' WHERE id = ?`, p.shared)
	for _, g := range [][]string{{"user", p.editor, "edit"}, {"user", p.viewer, "view"}, {"role", "2", "edit"}} {
		mustADR058Exec(t, db, `INSERT INTO entity_permissions (entity_id, subject_type, subject_id, permission) VALUES (?,?,?,?)`, p.shared, g[0], g[1], g[2])
	}

	// A second campaign with a page of its own.
	otherOwner := seedADR058User(t, db, "OtherOwner")
	p.otherCamp = adr058DBID(t)
	mustADR058Exec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, p.otherCamp, "Other", p.otherCamp, otherOwner)
	seedADR058Member(t, db, p.otherCamp, otherOwner, "owner")
	otherType := adr058Entity{Name: "Elsewhere"}
	res, err := db.Exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?,?,?,?)`, p.otherCamp, "int-x", "X", "Xs")
	if err != nil {
		t.Fatal(err)
	}
	otherTypeID, _ := res.LastInsertId()
	p.otherPage = seedADR058Entity(t, db, p.otherCamp, int(otherTypeID), otherOwner, otherType)

	p.repo = NewMediaRepository(db)
	mediaSvc := NewMediaService(p.repo, t.TempDir(), 10<<20)
	entitySvc := entities.NewEntityService(entities.NewEntityRepository(db), entities.NewEntityTypeRepository(db), entities.NewEntityPermissionRepository(db))
	p.svc = NewPageFileService(NewPageFileRepository(db), mediaSvc, dbPageAccess{svc: entitySvc})
	return p
}

// who builds the viewer the handler would build: the promoted role.
func (p *pageFileDB) who(user string) PageFileViewer {
	return PageFileViewer{UserID: user, Role: promotedVisibilityRole(p.mc, p.camp, user)}
}

func (p *pageFileDB) attach(t *testing.T, user, page, name string, gm bool) *PageFile {
	t.Helper()
	f, err := p.svc.Attach(context.Background(), PageFileUpload{
		CampaignID: p.camp, EntityID: page, Viewer: p.who(user), Name: name, Bytes: []byte("hello " + name), GMOnly: gm,
	})
	if err != nil {
		t.Fatalf("attach %s: %v", name, err)
	}
	return f
}

func (p *pageFileDB) trash(t *testing.T, page string, on bool) {
	t.Helper()
	if on {
		mustADR058Exec(t, p.db, `UPDATE entities SET deleted_at = NOW() WHERE id = ?`, page)
	} else {
		mustADR058Exec(t, p.db, `UPDATE entities SET deleted_at = NULL WHERE id = ?`, page)
	}
}

func TestDB_PageFileMigrationIsIdempotent(t *testing.T) {
	db := newADR058ScratchDB(t)
	up, err := os.ReadFile(filepath.Join("..", "..", "..", "db", "migrations", "000046_page_files.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	// The scratch schema already ran it once; a partial re-run must not fail.
	if _, err := db.Exec(string(up)); err != nil {
		t.Fatalf("re-running 000046 up: %v", err)
	}
}

func TestDB_PageFileAccess(t *testing.T) {
	p := newPageFileDB(t)
	ctx := context.Background()

	pub := p.attach(t, p.owner, p.open, "handout.txt", false)
	gm := p.attach(t, p.owner, p.open, "plot.txt", true)
	onShared := p.attach(t, p.owner, p.shared, "shared.txt", false)
	onPrivate := p.attach(t, p.owner, p.private, "dm.txt", false)
	onTrashed := p.attach(t, p.owner, p.trashed, "old.txt", false)
	p.trash(t, p.trashed, true)

	type openCase struct {
		label string
		user  string
		file  *PageFile
		page  string
		camp  string
		want  bool
	}
	cases := []openCase{
		// The page's visible file follows the page.
		{"owner / visible file", p.owner, pub, p.open, p.camp, true},
		{"scribe / visible file", p.scribe, pub, p.open, p.camp, true},
		{"editor-player / visible file on an open page", p.editor, pub, p.open, p.camp, true},
		{"view-only player / visible file", p.viewer, pub, p.open, p.camp, true},
		{"co-DM / visible file", p.coDM, pub, p.open, p.camp, true},
		{"a stranger with no seat in the campaign / visible file", p.stranger, pub, p.open, p.camp, false},
		// A GM-only file: the GM tier only.
		{"owner / GM-only file", p.owner, gm, p.open, p.camp, true},
		{"scribe / GM-only file", p.scribe, gm, p.open, p.camp, true},
		{"co-DM / GM-only file", p.coDM, gm, p.open, p.camp, true},
		{"editor-player / GM-only file", p.editor, gm, p.open, p.camp, false},
		{"view-only player / GM-only file", p.viewer, gm, p.open, p.camp, false},
		// A page with custom sharing.
		{"editor-player / file on a shared page", p.editor, onShared, p.shared, p.camp, true},
		{"view-only player with a view grant / file on a shared page", p.viewer, onShared, p.shared, p.camp, true},
		{"co-DM without a grant on the page / file on a shared page", p.coDM, onShared, p.shared, p.camp, true}, // promoted to owner
		// A private page.
		{"owner / file on a private page", p.owner, onPrivate, p.private, p.camp, true},
		{"scribe / file on a private page", p.scribe, onPrivate, p.private, p.camp, true},
		{"editor-player / file on a private page", p.editor, onPrivate, p.private, p.camp, false},
		{"view-only player / file on a private page", p.viewer, onPrivate, p.private, p.camp, false},
		// A trashed page hides its files from everyone, the owner included.
		{"owner / file on a trashed page", p.owner, onTrashed, p.trashed, p.camp, false},
		{"scribe / file on a trashed page", p.scribe, onTrashed, p.trashed, p.camp, false},
		// The wrong page or campaign never opens a file.
		{"owner / right file, wrong page", p.owner, pub, p.shared, p.camp, false},
		{"owner / right file, another campaign's route", p.owner, pub, p.open, p.otherCamp, false},
		{"owner / another campaign's page id", p.owner, pub, p.otherPage, p.camp, false},
	}
	for _, tt := range cases {
		t.Run(tt.label, func(t *testing.T) {
			v := p.who(tt.user)
			_, _, err := p.svc.Open(ctx, tt.camp, tt.page, tt.file.ID, v)
			if tt.want && err != nil {
				t.Fatalf("want it opened, got %v", err)
			}
			if !tt.want {
				if apperror.SafeCode(err) != http.StatusNotFound {
					t.Fatalf("want a 404, got %v", err)
				}
			}
		})
	}

	// Restoring the page from the Trash brings its files back; trashing it
	// again hides them again.
	t.Run("a restored page gets its files back", func(t *testing.T) {
		p.trash(t, p.trashed, false)
		if _, _, err := p.svc.Open(ctx, p.camp, p.trashed, onTrashed.ID, p.who(p.viewer)); err != nil {
			t.Fatalf("after restore: %v", err)
		}
		p.trash(t, p.trashed, true)
		if _, _, err := p.svc.Open(ctx, p.camp, p.trashed, onTrashed.ID, p.who(p.owner)); err == nil {
			t.Fatal("a re-trashed page still opens its file")
		}
	})

	// The listing: the same rule, for the same people.
	t.Run("listing", func(t *testing.T) {
		names := func(user, page string) ([]string, *PageFileListing, error) {
			l, err := p.svc.List(ctx, p.camp, page, p.who(user))
			if err != nil {
				return nil, nil, err
			}
			var n []string
			for _, f := range l.Files {
				n = append(n, f.Name)
			}
			return n, l, nil
		}
		if n, l, err := names(p.viewer, p.open); err != nil || len(n) != 1 || n[0] != "handout.txt" || l.CanAttach {
			t.Errorf("view-only player: %v %+v %v, want only handout.txt and no attach", n, l, err)
		}
		if n, l, err := names(p.scribe, p.open); err != nil || len(n) != 2 || !l.CanAttach || !l.CanMarkGMOnly {
			t.Errorf("scribe: %v %+v %v, want both files and full controls", n, l, err)
		}
		if _, l, err := names(p.editor, p.shared); err != nil || !l.CanAttach || l.CanMarkGMOnly {
			t.Errorf("editor-player on the shared page: %+v %v, want attach but no GM mark", l, err)
		}
		if _, _, err := names(p.editor, p.private); apperror.SafeCode(err) != http.StatusNotFound {
			t.Errorf("player on a private page: %v, want 404", err)
		}
		if _, _, err := names(p.owner, p.trashed); apperror.SafeCode(err) != http.StatusNotFound {
			t.Errorf("owner on a trashed page: %v, want 404", err)
		}
	})

	// Who may add: anyone who can edit the page, and nobody who cannot.
	t.Run("attach", func(t *testing.T) {
		attach := []struct {
			label string
			user  string
			page  string
			want  int
		}{
			{"owner on the open page", p.owner, p.open, 0},
			{"scribe on the open page", p.scribe, p.open, 0},
			{"co-DM on the open page", p.coDM, p.open, 0},
			{"editor-player on the open page (view only there)", p.editor, p.open, http.StatusForbidden},
			{"view-only player on the open page", p.viewer, p.open, http.StatusForbidden},
			{"editor-player on the page they may edit", p.editor, p.shared, 0},
			{"view-only player on the shared page", p.viewer, p.shared, http.StatusForbidden},
			{"scribe on the shared page (role grant)", p.scribe, p.shared, 0},
			{"editor-player on a private page", p.editor, p.private, http.StatusNotFound},
			{"scribe on a private page", p.scribe, p.private, 0},
			{"owner on a trashed page", p.owner, p.trashed, http.StatusNotFound},
			{"owner on another campaign's page", p.owner, p.otherPage, http.StatusNotFound},
		}
		for _, tt := range attach {
			t.Run(tt.label, func(t *testing.T) {
				_, err := p.svc.Attach(ctx, PageFileUpload{CampaignID: p.camp, EntityID: tt.page, Viewer: p.who(tt.user), Name: "n.txt", Bytes: []byte("x")})
				if code(err) != tt.want {
					t.Fatalf("status %d (%v), want %d", code(err), err, tt.want)
				}
			})
		}
	})

	// Marking a file GM only takes it away from the editor-player at once.
	t.Run("marking a file GM only hides it", func(t *testing.T) {
		f := p.attach(t, p.editor, p.shared, "mine.txt", false)
		if _, _, err := p.svc.Open(ctx, p.camp, p.shared, f.ID, p.who(p.viewer)); err != nil {
			t.Fatal(err)
		}
		if _, err := p.svc.SetGMOnly(ctx, p.camp, p.shared, f.ID, p.who(p.editor), true); apperror.SafeCode(err) != http.StatusForbidden {
			t.Fatalf("an editor-player marking GM only: %v, want 403", err)
		}
		if _, err := p.svc.SetGMOnly(ctx, p.camp, p.shared, f.ID, p.who(p.scribe), true); err != nil {
			t.Fatal(err)
		}
		for _, who := range []string{p.viewer, p.editor} {
			if _, _, err := p.svc.Open(ctx, p.camp, p.shared, f.ID, p.who(who)); apperror.SafeCode(err) != http.StatusNotFound {
				t.Errorf("after marking, %s still opens it: %v", who, err)
			}
		}
		if _, err := p.svc.Remove(ctx, p.camp, p.shared, f.ID, p.who(p.editor)); apperror.SafeCode(err) != http.StatusNotFound {
			t.Errorf("an editor-player removed a file they can no longer see: %v", err)
		}
	})
}

// Page files stay out of every campaign-wide listing and lookup, and count in
// the quota and the admin storage view.
func TestDB_PageFilesStayOutOfListingsAndDedup(t *testing.T) {
	p := newPageFileDB(t)
	ctx := context.Background()
	f := p.attach(t, p.owner, p.open, "handout.txt", true)

	files, total, err := p.repo.ListByCampaign(ctx, p.camp, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range files {
		if m.ID == f.ID {
			t.Error("a page file is in the campaign media list (picker, browser, sync API, export)")
		}
	}
	if total != 0 {
		t.Errorf("total = %d, want the page file not counted", total)
	}

	var hash string
	if err := p.db.QueryRow(`SELECT content_hash FROM media_files WHERE id = ?`, f.ID).Scan(&hash); err != nil || hash == "" {
		t.Fatalf("content hash: %q %v", hash, err)
	}
	if m, err := p.repo.FindByContentHash(ctx, p.camp, hash); err != nil || m != nil {
		t.Errorf("dedup found a page file: %v %v", m, err)
	}

	if _, n, err := p.repo.GetCampaignUsage(ctx, p.camp); err != nil || n != 1 {
		t.Errorf("quota counts %d files (%v), want the page file counted", n, err)
	}
	all, _, err := p.repo.ListAll(ctx, 10, 0)
	if err != nil || len(all) != 1 || !all[0].IsPageFile() {
		t.Errorf("admin storage list = %v (%v), want the page file listed for accounting", all, err)
	}

	// The owner's media browser delete refuses it too.
	svc := NewMediaService(p.repo, t.TempDir(), 10<<20)
	if err := svc.DeleteCampaignMedia(ctx, p.camp, f.ID); apperror.SafeCode(err) != http.StatusNotFound {
		t.Errorf("DeleteCampaignMedia removed a page file: %v", err)
	}
}

// The binding goes with the page, the sweep collects the file left behind, and
// a bound file is never swept.
func TestDB_PageFileLifecycle(t *testing.T) {
	p := newPageFileDB(t)
	ctx := context.Background()
	kept := p.attach(t, p.owner, p.open, "kept.txt", false)
	orphan := p.attach(t, p.owner, p.shared, "orphan.txt", false)

	// Purge a page (as the Trash purger does): its binding row goes with it.
	mustADR058Exec(t, p.db, `DELETE FROM entities WHERE id = ?`, p.shared)
	var n int
	if err := p.db.QueryRow(`SELECT COUNT(*) FROM page_files WHERE media_id = ?`, orphan.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("binding survived its page: %d %v", n, err)
	}

	future := time.Now().UTC().Add(time.Hour)
	ids, err := p.repo.ListUnboundPageFiles(ctx, future)
	if err != nil || len(ids) != 1 || ids[0] != orphan.ID {
		t.Fatalf("unbound page files = %v (%v), want only the purged page's file", ids, err)
	}
	if ids, _ := p.repo.ListUnboundPageFiles(ctx, time.Now().UTC().Add(-pageFileGrace)); len(ids) != 0 {
		t.Errorf("a file inside the grace period was listed: %v", ids)
	}

	svc := NewMediaService(p.repo, t.TempDir(), 10<<20)
	if err := svc.Delete(ctx, orphan.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.repo.FindByID(ctx, kept.ID); err != nil {
		t.Errorf("a bound page file was lost: %v", err)
	}

	// Removing through the page removes the binding with the file.
	if _, err := p.svc.Remove(ctx, p.camp, p.open, kept.ID, p.who(p.owner)); err != nil {
		t.Fatal(err)
	}
	if err := p.db.QueryRow(`SELECT COUNT(*) FROM page_files`).Scan(&n); err != nil || n != 0 {
		t.Errorf("bindings left after removal: %d %v", n, err)
	}
}
