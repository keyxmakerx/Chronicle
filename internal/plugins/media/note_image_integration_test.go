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
	"testing"

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

	// A picture Ana put in her note, and a page that mentions the same file.
	pic := seedNotePicture(t, db, camp, ana)
	noteID := adr058DBID(t)
	html := `<p><figure class="ce-img"><img src="/media/` + pic + `"></figure></p>`
	mustADR058Exec(t, db, `INSERT INTO notes (id, campaign_id, user_id, title, content, entry_html) VALUES (?,?,?,?,?,?)`,
		noteID, camp, ana, "Ana's note", "[]", html)

	// A public page whose text also mentions the file must not open it.
	et := seedADR058EntityType(t, db, camp)
	mention := "/media/" + pic
	seedADR058Entity(t, db, camp, et, gm, adr058Entity{Name: "Open page", EntryHTML: &mention})

	h := &Handler{
		memberChecker:    &dbMemberChecker{db: db},
		service:          &fakeAccessMediaService{},
		entityVisibility: &dbEntityVisibility{repo: entities.NewEntityRepository(db)},
		noteMedia:        dbNoteMedia{svc: notes.NewNoteService(notes.NewNoteRepository(db))},
	}
	file := func() *MediaFile {
		f, err := NewMediaRepository(db).FindByID(ctx, pic)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	check := func(who string) error {
		return h.checkMediaAccess(newADR058TestContext(who), file(), false, "")
	}
	setSharing := func(set string, args ...any) {
		mustADR058Exec(t, db, `UPDATE notes SET is_shared = FALSE, shared_with_gm = FALSE, shared_with = NULL WHERE id = ?`, noteID)
		if set != "" {
			mustADR058Exec(t, db, `UPDATE notes SET `+set+` WHERE id = ?`, append(args, noteID)...)
		}
	}

	tests := []struct {
		name    string
		sharing func()
		allowed map[string]bool
	}{
		{"private note", func() { setSharing("") },
			map[string]bool{ana: true, gm: false, bo: false, cy: false, outsider: false}},
		{"note shared with the party", func() { setSharing("is_shared = TRUE") },
			map[string]bool{ana: true, gm: true, bo: true, cy: true, outsider: false}},
		{"note named to Bo", func() { setSharing("shared_with = JSON_ARRAY(?)", bo) },
			map[string]bool{ana: true, gm: false, bo: true, cy: false, outsider: false}},
		{"note shared with the GM", func() { setSharing("shared_with_gm = TRUE") },
			map[string]bool{ana: true, gm: true, bo: false, cy: false, outsider: false}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.sharing()
			for who, want := range tc.allowed {
				err := check(who)
				if want {
					mustAllow(t, err, tc.name)
				} else {
					mustDeny(t, err, tc.name)
				}
			}
		})
	}

	t.Run("note deleted: only the uploader still has it", func(t *testing.T) {
		mustADR058Exec(t, db, `DELETE FROM notes WHERE id = ?`, noteID)
		mustAllow(t, check(ana), "uploader")
		mustDeny(t, check(bo), "another player")
		mustDeny(t, check(gm), "the GM")
	})

	t.Run("picture in no note at all is not open to the campaign", func(t *testing.T) {
		loose := seedNotePicture(t, db, camp, ana)
		f, _ := NewMediaRepository(db).FindByID(ctx, loose)
		mustDeny(t, h.checkMediaAccess(newADR058TestContext(bo), f, false, ""), "membership fallback")
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
