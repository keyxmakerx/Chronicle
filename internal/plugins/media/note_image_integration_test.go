// note_image_integration_test.go drives the note-picture rule against a REAL
// MariaDB through the production pieces on both sides of the seam: media's
// Handler.checkMediaAccess and the notes service/repository that decide who
// can read a note. Only the adapter between them (a few lines in
// app/routes.go) is restated here.
//
// Skips when no database answers; see newADR058ScratchDB.
package media

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// seedNotePicture inserts a note_image row uploaded by uploadedBy.
func seedNotePicture(t *testing.T, db *sql.DB, campaignID, uploadedBy string) string {
	t.Helper()
	id := seedADR058MediaFile(t, db, campaignID, uploadedBy)
	mustADR058Exec(t, db, `UPDATE media_files SET usage_type = ?, thumbnail_paths = '{}' WHERE id = ?`, UsageNoteImage, id)
	return id
}

type dbNoteMedia struct{ svc notes.NoteService }

func (a dbNoteMedia) CanReadNoteMedia(ctx context.Context, campaignID, mediaID string, role int, userID string) (bool, error) {
	return a.svc.ViewerReadsMedia(ctx, campaignID, mediaID, permissions.RequestViewer(role, userID))
}

func TestDB_NotePictureAccess(t *testing.T) {
	db := newADR058ScratchDB(t)
	ctx := context.Background()
	camp, gm := seedADR058Campaign(t, db)
	seedADR058Member(t, db, camp, gm, "owner")
	ana, bo, cy := seedADR058User(t, db, "Ana"), seedADR058User(t, db, "Bo"), seedADR058User(t, db, "Cy")
	for _, id := range []string{ana, bo, cy} {
		seedADR058Member(t, db, camp, id, "player")
	}
	outsider := seedADR058User(t, db, "Outsider")
	player := func(id string) permissions.Viewer { return permissions.RequestViewer(permissions.RolePlayer, id) }

	// A picture Ana uploaded; the notes below are written through the real
	// notes service, so bindings come from real saves.
	pic := seedNotePicture(t, db, camp, ana)
	html := `<p><figure class="ce-img"><img src="/media/` + pic + `"></figure></p>`
	svc := notes.NewNoteService(notes.NewNoteRepository(db))

	reshare := func(noteID, owner string, vis notes.Visibility, with ...string) {
		if _, err := svc.Update(ctx, noteID, player(owner), notes.UpdateNoteRequest{Visibility: &vis, SharedWith: with}); err != nil {
			t.Fatal(err)
		}
	}
	// write makes a note owned by owner, saved by saver with the picture in
	// its text, then shared as vis (with named people for "custom").
	write := func(owner, saver string, vis notes.Visibility, with ...string) string {
		n, err := svc.Create(ctx, camp, player(owner), notes.CreateNoteRequest{Title: "n"})
		if err != nil {
			t.Fatal(err)
		}
		req := notes.UpdateNoteRequest{EntryHTML: &html}
		if _, err := svc.Update(ctx, n.ID, player(saver), req); err != nil {
			t.Fatal(err)
		}
		if vis != notes.VisibilityPrivate { // a new note already is private
			reshare(n.ID, owner, vis, with...)
		}
		return n.ID
	}
	clearNotes := func() { mustADR058Exec(t, db, `DELETE FROM notes WHERE campaign_id = ?`, camp) }

	h := &Handler{
		memberChecker:    &dbMemberChecker{db: db},
		service:          &fakeAccessMediaService{},
		entityVisibility: &dbEntityVisibility{repo: entities.NewEntityRepository(db)},
		noteMedia:        dbNoteMedia{svc: svc},
	}
	check := func(who string) error {
		f, err := NewMediaRepository(db).FindByID(ctx, pic)
		if err != nil {
			t.Fatal(err)
		}
		return h.checkMediaAccess(newADR058TestContext(who), f, false, "")
	}
	expect := func(label string, allowed map[string]bool) {
		t.Helper()
		for who, want := range allowed {
			if want {
				mustAllow(t, check(who), label)
			} else {
				mustDeny(t, check(who), label)
			}
		}
	}

	t.Run("the uploader's own note follows its sharing", func(t *testing.T) {
		clearNotes()
		id := write(ana, ana, notes.VisibilityPrivate)
		expect("private", map[string]bool{ana: true, gm: false, bo: false, cy: false, outsider: false})
		reshare(id, ana, notes.VisibilityParty)
		expect("party", map[string]bool{ana: true, gm: true, bo: true, cy: true, outsider: false})
		reshare(id, ana, notes.VisibilityCustom, bo)
		expect("named to Bo", map[string]bool{ana: true, gm: false, bo: true, cy: false, outsider: false})
		reshare(id, ana, notes.VisibilityGM)
		expect("shared with the GM", map[string]bool{ana: true, gm: true, bo: false, cy: false, outsider: false})
	})

	t.Run("an id pasted into someone else's note grants nothing, however it is shared", func(t *testing.T) {
		clearNotes()
		ownerNote := write(ana, ana, notes.VisibilityPrivate) // Ana unshares / never shared
		// Bo pastes the id into his own note and then shares that note.
		bos := write(bo, bo, notes.VisibilityPrivate)
		expect("Bo's private note", map[string]bool{bo: false, cy: false, gm: false, ana: true})
		reshare(bos, bo, notes.VisibilityCustom, ana)
		expect("Bo's note shared with Ana", map[string]bool{bo: false, cy: false, gm: false, ana: true})
		reshare(bos, bo, notes.VisibilityParty)
		expect("Bo's note shared with the party", map[string]bool{bo: false, cy: false, gm: false, ana: true})
		_ = ownerNote
	})

	t.Run("a picture Ana adds to Bo's party note is visible to the party", func(t *testing.T) {
		clearNotes()
		write(bo, ana, notes.VisibilityParty) // Bo's note; Ana, the collaborator, saves the picture in
		expect("collaborator picture", map[string]bool{ana: true, bo: true, cy: true, gm: true, outsider: false})
	})

	t.Run("a save without the picture drops the binding", func(t *testing.T) {
		clearNotes()
		id := write(ana, ana, notes.VisibilityParty)
		expect("before", map[string]bool{bo: true})
		empty := "<p>gone</p>"
		if _, err := svc.Update(ctx, id, player(ana), notes.UpdateNoteRequest{EntryHTML: &empty}); err != nil {
			t.Fatal(err)
		}
		expect("after", map[string]bool{bo: false, ana: true})
	})

	t.Run("a later save by someone else keeps the binding but cannot create one", func(t *testing.T) {
		clearNotes()
		id := write(ana, ana, notes.VisibilityParty)
		if _, err := svc.Update(ctx, id, player(bo), notes.UpdateNoteRequest{EntryHTML: &html}); err != nil {
			t.Fatal(err)
		}
		expect("kept", map[string]bool{bo: true, cy: true})
		// Bo's save into a note Ana never saved binds nothing.
		clearNotes()
		other := write(bo, bo, notes.VisibilityParty)
		_ = other
		expect("not bound", map[string]bool{bo: false, cy: false})
	})

	t.Run("Bo inserts Ana's picture, then Ana saves the note: still closed to the party", func(t *testing.T) {
		clearNotes()
		n, err := svc.Create(ctx, camp, player(bo), notes.CreateNoteRequest{Title: "Bo's"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Update(ctx, n.ID, player(bo), notes.UpdateNoteRequest{EntryHTML: &html}); err != nil {
			t.Fatal(err)
		}
		reshare(n.ID, bo, notes.VisibilityParty)
		// Ana, a collaborator, saves the note for her own reasons: a title change,
		// then a body edit that still carries the pasted id.
		retitled := "Party notes"
		if _, err := svc.Update(ctx, n.ID, player(ana), notes.UpdateNoteRequest{Title: &retitled}); err != nil {
			t.Fatal(err)
		}
		edited := html + "<p>more</p>"
		if _, err := svc.Update(ctx, n.ID, player(ana), notes.UpdateNoteRequest{EntryHTML: &edited}); err != nil {
			t.Fatal(err)
		}
		expect("Ana saved Bo's pasted id", map[string]bool{bo: false, cy: false, gm: false, ana: true})
	})

	t.Run("a restore never binds a picture", func(t *testing.T) {
		clearNotes()
		id := write(ana, ana, notes.VisibilityParty)
		empty := "<p>gone</p>"
		if _, err := svc.Update(ctx, id, player(ana), notes.UpdateNoteRequest{EntryHTML: &empty}); err != nil {
			t.Fatal(err)
		}
		expect("dropped", map[string]bool{bo: false})
		versions, err := svc.ListVersions(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		restored := false
		for _, v := range versions {
			if v.EntryHTML != nil && strings.Contains(*v.EntryHTML, pic) {
				if _, err := svc.RestoreVersion(ctx, id, v.ID, ana); err != nil {
					t.Fatal(err)
				}
				restored = true
				break
			}
		}
		if !restored {
			t.Fatal("no saved version names the picture to restore")
		}
		expect("after restore", map[string]bool{bo: false, cy: false, gm: false, ana: true})
	})

	t.Run("a page that mentions the file does not open it", func(t *testing.T) {
		clearNotes()
		write(ana, ana, notes.VisibilityPrivate)
		et := seedADR058EntityType(t, db, camp)
		mention := "/media/" + pic
		seedADR058Entity(t, db, camp, et, gm, adr058Entity{Name: "Open page", EntryHTML: &mention})
		expect("page mention", map[string]bool{bo: false, cy: false, gm: false, ana: true})
	})

	t.Run("uploader removed from the campaign: the picture stops loading for others", func(t *testing.T) {
		clearNotes()
		write(ana, ana, notes.VisibilityParty)
		expect("before", map[string]bool{bo: true})
		mustADR058Exec(t, db, `DELETE FROM campaign_members WHERE campaign_id = ? AND user_id = ?`, camp, ana)
		defer seedADR058Member(t, db, camp, ana, "player")
		expect("uploader gone", map[string]bool{bo: false, gm: false, ana: false})
	})

	t.Run("note deleted: only the uploader still has it, and nobody by membership", func(t *testing.T) {
		clearNotes()
		write(ana, ana, notes.VisibilityParty)
		clearNotes()
		expect("deleted", map[string]bool{ana: true, bo: false, gm: false})
		loose := seedNotePicture(t, db, camp, ana)
		f, _ := NewMediaRepository(db).FindByID(ctx, loose)
		mustDeny(t, h.checkMediaAccess(newADR058TestContext(bo), f, false, ""), "membership fallback")
	})

	t.Run("orphan cleanup keeps a picture a saved version still names", func(t *testing.T) {
		clearNotes()
		id := write(ana, ana, notes.VisibilityParty)
		gone := "<p>gone</p>"
		if _, err := svc.Update(ctx, id, player(ana), notes.UpdateNoteRequest{EntryHTML: &gone}); err != nil {
			t.Fatal(err)
		}
		mustADR058Exec(t, db, `UPDATE media_files SET created_at = NOW() - INTERVAL 3 DAY WHERE id = ?`, pic)
		cutoff := time.Now().UTC().Add(-notePictureGrace)
		got, err := NewMediaRepository(db).ListUnboundNotePictures(ctx, cutoff)
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range got {
			if g == pic {
				t.Fatal("collected a picture a note version still names")
			}
		}
		mustADR058Exec(t, db, `DELETE FROM note_versions WHERE note_id = ?`, id)
		got, _ = NewMediaRepository(db).ListUnboundNotePictures(ctx, cutoff)
		found := false
		for _, g := range got {
			found = found || g == pic
		}
		if !found {
			t.Error("a picture no note or version holds was not collected")
		}
	})

	t.Run("orphan cleanup collects pictures no note holds, after the grace period", func(t *testing.T) {
		clearNotes()
		bound := pic
		write(ana, ana, notes.VisibilityPrivate)
		stale := seedNotePicture(t, db, camp, ana)
		fresh := seedNotePicture(t, db, camp, ana)
		mustADR058Exec(t, db, `UPDATE media_files SET created_at = NOW() - INTERVAL 3 DAY WHERE id IN (?, ?)`, bound, stale)
		got, err := NewMediaRepository(db).ListUnboundNotePictures(ctx, time.Now().UTC().Add(-notePictureGrace))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != stale {
			t.Errorf("collected %v, want only the old unbound picture %s (not the bound one %s nor the fresh one %s)", got, stale, bound, fresh)
		}
	})
}

// Note pictures stay out of the media browser, the picker and content-hash
// dedup, so a Scribe or the owner never sees them and a page upload of the
// same bytes is a file of its own.
func TestDB_NotePicturesStayOutOfListingsAndDedup(t *testing.T) {
	db := newADR058ScratchDB(t)
	ctx := context.Background()
	camp, gm := seedADR058Campaign(t, db)
	page := seedADR058MediaFile(t, db, camp, gm)
	mustADR058Exec(t, db, `UPDATE media_files SET thumbnail_paths = '{}' WHERE id = ?`, page)
	note := seedNotePicture(t, db, camp, gm)
	hash := "ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12"
	mustADR058Exec(t, db, `UPDATE media_files SET content_hash = ? WHERE id IN (?, ?)`, hash, page, note)

	repo := NewMediaRepository(db)
	files, total, err := repo.ListByCampaign(ctx, camp, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(files) != 1 || files[0].ID != page {
		t.Errorf("listing = %d rows (total %d), want only the page picture %s", len(files), total, page)
	}
	for i := 0; i < 5; i++ { // the dedup lookup has no ORDER BY; ask repeatedly
		got, err := repo.FindByContentHash(ctx, camp, hash)
		if err != nil || got == nil || got.ID != page {
			t.Fatalf("dedup lookup = %+v, %v; want the page picture, never the note picture", got, err)
		}
	}
	// The orphan sweep deletes disk files no row names; a note picture has a
	// row, so it is known and survives.
	known, err := repo.ListAllFilenames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !known[note+".png"] {
		t.Error("the orphan cleanup would delete a note picture: its file is not among the tracked filenames")
	}
	mustADR058Exec(t, db, `DELETE FROM media_files WHERE id = ?`, page)
	if got, _ := repo.FindByContentHash(ctx, camp, hash); got != nil {
		t.Errorf("dedup matched the note picture %s", got.ID)
	}
}

// The admin storage list keeps a note picture's row for disk accounting but
// never carries its file name or thumbnails.
func TestDB_AdminListHidesNotePictureDetails(t *testing.T) {
	db := newADR058ScratchDB(t)
	camp, gm := seedADR058Campaign(t, db)
	note := seedNotePicture(t, db, camp, gm)
	mustADR058Exec(t, db, `UPDATE media_files SET original_name = 'secret-plan.png', thumbnail_paths = '{"300":"x_300.jpg"}' WHERE id = ?`, note)
	page := seedADR058MediaFile(t, db, camp, gm)
	mustADR058Exec(t, db, `UPDATE media_files SET thumbnail_paths = '{"300":"y_300.jpg"}' WHERE id = ?`, page)

	files, total, err := NewMediaRepository(db).ListAll(context.Background(), 50, 0)
	if err != nil || total != 2 || len(files) != 2 {
		t.Fatalf("list = %d rows, total %d, err %v; the row must stay for disk accounting", len(files), total, err)
	}
	for _, f := range files {
		switch f.ID {
		case note:
			if f.OriginalName == "secret-plan.png" || len(f.ThumbnailPaths) != 0 || f.FileSize == 0 {
				t.Errorf("note picture row leaks or lost its size: %+v", f.MediaFile)
			}
		case page:
			if f.OriginalName != "test.png" || f.ThumbnailPaths["300"] == "" {
				t.Errorf("ordinary row changed: %+v", f.MediaFile)
			}
		}
	}
}
