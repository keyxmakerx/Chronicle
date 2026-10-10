package app

// trash_integration_test.go pins the site Trash against a real database: a
// trashed campaign is 404 on every campaign-gated route and on its media, Undo
// brings it back exactly, a purge waits out the retention and never touches
// something undone, "Empty trash now" needs the password re-check, and a file
// clean-up can be undone. Mocks of the repositories would not prove the SQL
// gates, so the real services run against a scratch schema.
//
// Skipped under -short and without CHRONICLE_TEST_DB_DSN (see
// openGalleryTestDB).

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/admin"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// trashUserFinder answers the one lookup MoveToTrash makes.
type trashUserFinder struct{ campaigns.UserFinder }

func (trashUserFinder) FindUserByID(_ context.Context, id string) (*campaigns.MemberUser, error) {
	return &campaigns.MemberUser{ID: id, DisplayName: "alex"}, nil
}

// trashAuth is a session-cookie auth stub: the cookie value is the user id,
// and re-check state is switchable.
type trashAuth struct {
	auth.AuthService
	adminID string
	reauth  bool
}

func (a *trashAuth) ValidateSession(_ context.Context, token string) (*auth.Session, error) {
	return &auth.Session{UserID: token, IsAdmin: token == a.adminID}, nil
}

func (a *trashAuth) IsReauthValid(context.Context, string) (bool, error) { return a.reauth, nil }

type trashFinder struct{ items []admin.OrphanedMediaItem }

func (f trashFinder) ScanOrphanedMedia(context.Context) ([]admin.OrphanedMediaItem, error) {
	return f.items, nil
}

type trashFixture struct {
	t          *testing.T
	db         *sql.DB
	ownerID    string
	memberID   string
	adminID    string
	campaignID string
	mediaDir   string
	campaigns  campaigns.CampaignService
	media      media.MediaService
	mediaRepo  media.MediaRepository
}

func newTrashFixture(t *testing.T) *trashFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("DB integration test")
	}
	db := openGalleryTestDB(t)
	fx := &trashFixture{
		t: t, db: db,
		ownerID: galleryTestUUID(t), memberID: galleryTestUUID(t), adminID: galleryTestUUID(t),
		campaignID: galleryTestUUID(t), mediaDir: t.TempDir(),
	}
	for i, id := range []string{fx.ownerID, fx.memberID, fx.adminID} {
		mustGalleryExec(t, db, `INSERT INTO users (id, email, display_name, password_hash, is_admin) VALUES (?, ?, ?, 'x', ?)`,
			id, id+"@example.test", "User "+id[:4], i == 2)
	}
	mustGalleryExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, 'Shattered Coast', ?, ?)`,
		fx.campaignID, "shattered-"+fx.campaignID[:8], fx.ownerID)
	mustGalleryExec(t, db, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'owner'), (?, ?, 'player')`,
		fx.campaignID, fx.ownerID, fx.campaignID, fx.memberID)

	fx.campaigns = campaigns.NewCampaignService(campaigns.NewCampaignRepository(db), trashUserFinder{}, nil, nil, "http://localhost")
	fx.mediaRepo = media.NewMediaRepository(db)
	fx.media = media.NewMediaService(fx.mediaRepo, fx.mediaDir, 1<<20)
	// The same wiring routes.go does, so a purged campaign loses its files.
	fx.campaigns.SetMediaCleaner(fx.media)
	return fx
}

// addFile inserts a media row (and its bytes on disk) and returns its id.
func (fx *trashFixture) addFile(campaignID *string, usage string, size int64) string {
	fx.t.Helper()
	id := galleryTestUUID(fx.t)
	name := id + ".png"
	if err := os.WriteFile(filepath.Join(fx.mediaDir, name), []byte("png"), 0o600); err != nil {
		fx.t.Fatal(err)
	}
	mustGalleryExec(fx.t, fx.db,
		`INSERT INTO media_files (id, campaign_id, uploaded_by, filename, original_name, mime_type, file_size, usage_type, thumbnail_paths) VALUES (?, ?, ?, ?, ?, 'image/png', ?, ?, '{}')`,
		id, campaignID, fx.ownerID, name, name, size, usage)
	return id
}

func (fx *trashFixture) fileOnDisk(id string) bool {
	_, err := os.Stat(filepath.Join(fx.mediaDir, id+".png"))
	return err == nil
}

func (fx *trashFixture) rowExists(table, id string) bool {
	var n int
	if err := fx.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE id = ?", id).Scan(&n); err != nil {
		fx.t.Fatal(err)
	}
	return n > 0
}

func (fx *trashFixture) trashService(finder admin.UnusedUploadFinder) admin.TrashService {
	return admin.NewTrashService(fx.campaigns, fx.media, finder, admin.NewTrashBatchRepository(fx.db), nil)
}

func isNotFoundErr(err error) bool {
	var appErr *apperror.AppError
	return errors.As(err, &appErr) && appErr.Code == http.StatusNotFound
}

// status runs a request through e as the given user and returns the code.
func trashStatus(e *echo.Echo, method, target, userID string) int {
	req := httptest.NewRequest(method, target, nil)
	if userID != "" {
		req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: userID})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code
}

func trashEcho() *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) {
			_ = c.NoContent(appErr.Code)
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	return e
}

func TestSiteTrash_CampaignIsGoneForMembersAndBackAfterUndo(t *testing.T) {
	fx := newTrashFixture(t)
	ctx := context.Background()

	authStub := &trashAuth{adminID: fx.adminID}
	e := trashEcho()
	// /campaigns/:id/... stands for every campaign route (the entity routes
	// among them): they all sit behind this one membership gate.
	g := e.Group("/campaigns/:id", auth.RequireAuth(authStub), campaigns.RequireCampaignAccess(fx.campaigns))
	ok := func(c echo.Context) error { return c.NoContent(http.StatusOK) }
	g.GET("", ok)
	g.GET("/entities/:eid", ok)

	// A campaign file, served by the real media handler with a link signed
	// before the delete.
	fileID := fx.addFile(&fx.campaignID, "attachment", 10)
	signer := media.NewURLSigner("trash-test-secret")
	mh := media.NewHandler(fx.media)
	mh.SetURLSigner(signer)
	e.GET("/media/:id", mh.Serve)
	signed := signer.Sign(fileID, media.ViewerAnonymous, time.Hour)

	var updatedBefore time.Time
	if err := fx.db.QueryRow(`SELECT updated_at FROM campaigns WHERE id = ?`, fx.campaignID).Scan(&updatedBefore); err != nil {
		t.Fatal(err)
	}

	checks := func(label string, wantCampaign, wantMedia int) {
		t.Helper()
		for _, uid := range []string{fx.memberID, fx.ownerID} {
			if got := trashStatus(e, http.MethodGet, "/campaigns/"+fx.campaignID, uid); got != wantCampaign {
				t.Errorf("%s: campaign page for %s = %d, want %d", label, uid[:4], got, wantCampaign)
			}
			if got := trashStatus(e, http.MethodGet, "/campaigns/"+fx.campaignID+"/entities/x", uid); got != wantCampaign {
				t.Errorf("%s: entity route for %s = %d, want %d", label, uid[:4], got, wantCampaign)
			}
		}
		if got := trashStatus(e, http.MethodGet, signed, ""); got != wantMedia {
			t.Errorf("%s: signed media link = %d, want %d", label, got, wantMedia)
		}
	}
	checks("before", http.StatusOK, http.StatusOK)

	if err := fx.campaigns.MoveToTrash(ctx, fx.campaignID, fx.adminID); err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}
	checks("trashed", http.StatusNotFound, http.StatusNotFound)

	// The campaign stays out of every list the members and the public see.
	if list, _, err := fx.campaigns.List(ctx, fx.memberID, campaigns.DefaultListOptions()); err != nil {
		t.Fatal(err)
	} else {
		for _, c := range list {
			if c.ID == fx.campaignID {
				t.Error("trashed campaign still listed for a member")
			}
		}
	}
	// The lookups the sync API, the socket and the media gates are built on.
	if _, err := fx.campaigns.GetByID(ctx, fx.campaignID); !isNotFoundErr(err) {
		t.Errorf("GetByID = %v, want not found", err)
	}
	if m, err := fx.campaigns.GetMember(ctx, fx.campaignID, fx.ownerID); err == nil || m != nil {
		t.Errorf("GetMember on a trashed campaign = %v,%v, want an error", m, err)
	}
	// Its storage still counts: the rows and bytes stay in place.
	if !fx.rowExists("media_files", fileID) {
		t.Error("media row vanished while the campaign is in the trash")
	}
	trashed, err := fx.campaigns.ListTrashed(ctx)
	if err != nil || len(trashed) != 1 || trashed[0].ID != fx.campaignID {
		t.Fatalf("ListTrashed = %v, %v", trashed, err)
	}
	if trashed[0].DeletedByName != "alex" || trashed[0].StorageBytes != 10 {
		t.Errorf("trashed row = %+v, want deleted by alex with 10 bytes", trashed[0])
	}

	// Trashing twice is refused, not repeated.
	if err := fx.campaigns.MoveToTrash(ctx, fx.campaignID, fx.adminID); !isNotFoundErr(err) {
		t.Errorf("second MoveToTrash = %v, want not found", err)
	}

	if err := fx.campaigns.RestoreFromTrash(ctx, fx.campaignID); err != nil {
		t.Fatalf("RestoreFromTrash: %v", err)
	}
	checks("restored", http.StatusOK, http.StatusOK)

	var updatedAfter time.Time
	var deletedAt, purgeStarted sql.NullTime
	if err := fx.db.QueryRow(`SELECT updated_at, deleted_at, purge_started_at FROM campaigns WHERE id = ?`, fx.campaignID).
		Scan(&updatedAfter, &deletedAt, &purgeStarted); err != nil {
		t.Fatal(err)
	}
	if !updatedAfter.Equal(updatedBefore) {
		t.Errorf("updated_at moved from %v to %v across trash and undo", updatedBefore, updatedAfter)
	}
	if deletedAt.Valid || purgeStarted.Valid {
		t.Error("Undo left the trash markers set")
	}
	// Undoing something that is not in the trash is refused.
	if err := fx.campaigns.RestoreFromTrash(ctx, fx.campaignID); !isNotFoundErr(err) {
		t.Errorf("second Undo = %v, want not found", err)
	}
}

func TestSiteTrash_PurgeWaitsForRetentionAndNeverTouchesUndone(t *testing.T) {
	fx := newTrashFixture(t)
	ctx := context.Background()
	svc := fx.trashService(trashFinder{})

	// A second campaign, so one can be undone while the other is purged.
	otherID := galleryTestUUID(t)
	mustGalleryExec(t, fx.db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, 'Other', ?, ?)`,
		otherID, "other-"+otherID[:8], fx.ownerID)
	fileID := fx.addFile(&fx.campaignID, "attachment", 5)

	for _, id := range []string{fx.campaignID, otherID} {
		if err := fx.campaigns.MoveToTrash(ctx, id, fx.adminID); err != nil {
			t.Fatal(err)
		}
	}

	// Inside the retention nothing is purged.
	res, err := svc.PurgeDue(ctx)
	if err != nil || res.Campaigns != 0 {
		t.Fatalf("early PurgeDue = %+v, %v; want nothing", res, err)
	}
	if !fx.rowExists("campaigns", fx.campaignID) || !fx.rowExists("campaigns", otherID) {
		t.Fatal("a campaign was purged before its retention ended")
	}

	// Age both past the default 30 days, then undo one: only the other goes.
	mustGalleryExec(t, fx.db, `UPDATE campaigns SET deleted_at = ? WHERE id IN (?, ?)`,
		time.Now().UTC().AddDate(0, 0, -31), fx.campaignID, otherID)
	if _, err := svc.UndoCampaign(ctx, otherID); err != nil {
		t.Fatalf("UndoCampaign: %v", err)
	}
	res, err = svc.PurgeDue(ctx)
	if err != nil || res.Campaigns != 1 || res.Failed != 0 {
		t.Fatalf("PurgeDue = %+v, %v; want 1 campaign purged", res, err)
	}
	if fx.rowExists("campaigns", fx.campaignID) {
		t.Error("the aged campaign was not purged")
	}
	if fx.rowExists("media_files", fileID) || fx.fileOnDisk(fileID) {
		t.Error("the purged campaign's file was left behind")
	}
	if !fx.rowExists("campaigns", otherID) {
		t.Fatal("the undone campaign was purged")
	}

	// A purge that has begun cannot be undone, and an undone campaign cannot
	// be claimed.
	if ok, err := fx.claim(otherID); err != nil || ok {
		t.Errorf("claiming a live campaign = %v, %v; want false", ok, err)
	}
	if err := fx.campaigns.MoveToTrash(ctx, otherID, fx.adminID); err != nil {
		t.Fatal(err)
	}
	purged, err := fx.campaigns.PurgeTrashed(ctx, otherID)
	if err != nil || !purged {
		t.Fatalf("PurgeTrashed = %v, %v", purged, err)
	}
	if fx.rowExists("campaigns", otherID) {
		t.Error("PurgeTrashed left the row")
	}
	if purged, err := fx.campaigns.PurgeTrashed(ctx, otherID); err != nil || purged {
		t.Errorf("second PurgeTrashed = %v, %v; want false, nil", purged, err)
	}
}

// claim stamps purge_started_at the way the purge's first step does, to prove
// Undo is refused afterwards.
func (fx *trashFixture) claim(id string) (bool, error) {
	repo := campaigns.NewCampaignRepository(fx.db)
	return repo.ClaimForPurge(context.Background(), id, time.Now().UTC())
}

func TestSiteTrash_UndoRefusedOnceTheFinalDeleteBegan(t *testing.T) {
	fx := newTrashFixture(t)
	ctx := context.Background()
	if err := fx.campaigns.MoveToTrash(ctx, fx.campaignID, fx.adminID); err != nil {
		t.Fatal(err)
	}
	if ok, err := fx.claim(fx.campaignID); err != nil || !ok {
		t.Fatalf("claim = %v, %v", ok, err)
	}
	if err := fx.campaigns.RestoreFromTrash(ctx, fx.campaignID); !isNotFoundErr(err) {
		t.Errorf("Undo after the claim = %v, want not found", err)
	}
	// A claimed campaign is resumed by the next purge whatever its age.
	ids, err := fx.campaigns.ListPurgeDue(ctx, time.Now().UTC().AddDate(0, 0, -30), false)
	if err != nil || len(ids) != 1 || ids[0] != fx.campaignID {
		t.Errorf("ListPurgeDue = %v, %v; want the claimed campaign", ids, err)
	}
}

func TestSiteTrash_FileCleanupCanBeUndoneAndPurgeSparesIt(t *testing.T) {
	fx := newTrashFixture(t)
	ctx := context.Background()

	a := fx.addFile(nil, "attachment", 100)
	b := fx.addFile(nil, "attachment", 50)
	inCampaign := fx.addFile(&fx.campaignID, "attachment", 7)
	avatar := fx.addFile(nil, "avatar", 3)
	finder := trashFinder{items: []admin.OrphanedMediaItem{
		{ID: a, MimeType: "image/png"},
		{ID: b, MimeType: "image/png"},
		// A scan result is never trusted blindly: a campaign file and an
		// avatar are skipped by the SQL even when listed.
		{ID: inCampaign, MimeType: "image/png"},
		{ID: avatar, MimeType: "image/png"},
	}}
	svc := fx.trashService(finder)

	batch, err := svc.TrashUnusedUploads(ctx, admin.TrashActor{UserID: fx.adminID, Name: "alex"})
	if err != nil || batch == nil {
		t.Fatalf("TrashUnusedUploads = %v, %v", batch, err)
	}
	if batch.Items != 2 || batch.Bytes != 150 {
		t.Errorf("batch = %d files, %d bytes; want 2 files, 150 bytes", batch.Items, batch.Bytes)
	}
	// Files stay on disk and in the table while trashed.
	for _, id := range []string{a, b} {
		if !fx.rowExists("media_files", id) || !fx.fileOnDisk(id) {
			t.Errorf("file %s was removed by the clean-up", id[:4])
		}
	}
	// A trashed file is not served.
	e := trashEcho()
	mh := media.NewHandler(fx.media)
	e.GET("/media/:id", mh.Serve)
	if got := trashStatus(e, http.MethodGet, "/media/"+a, ""); got != http.StatusNotFound {
		t.Errorf("trashed file = %d, want 404", got)
	}
	// The campaign file and the avatar were not touched.
	var n int
	if err := fx.db.QueryRow(`SELECT COUNT(*) FROM media_files WHERE trash_batch_id IS NOT NULL`).Scan(&n); err != nil || n != 2 {
		t.Errorf("files in a batch = %d, %v; want 2", n, err)
	}

	ov, err := svc.Overview(ctx)
	if err != nil || len(ov.Entries) != 1 || ov.Entries[0].Kind != admin.TrashFiles {
		t.Fatalf("Overview = %+v, %v", ov, err)
	}

	// Undo restores exactly: the same rows, no batch mark.
	if _, err := svc.UndoBatch(ctx, batch.ID); err != nil {
		t.Fatalf("UndoBatch: %v", err)
	}
	if err := fx.db.QueryRow(`SELECT COUNT(*) FROM media_files WHERE trash_batch_id IS NOT NULL`).Scan(&n); err != nil || n != 0 {
		t.Errorf("files still in a batch after Undo = %d, %v", n, err)
	}
	if ov, _ := svc.Overview(ctx); len(ov.Entries) != 0 {
		t.Errorf("Overview after Undo = %+v, want empty", ov.Entries)
	}
	if _, err := svc.UndoBatch(ctx, batch.ID); !isNotFoundErr(err) {
		t.Errorf("second UndoBatch = %v, want not found", err)
	}

	// Even a purge of everything leaves an undone batch's files alone.
	if _, err := svc.EmptyNow(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{a, b, inCampaign, avatar} {
		if !fx.rowExists("media_files", id) || !fx.fileOnDisk(id) {
			t.Errorf("file %s was deleted although its clean-up was undone", id[:4])
		}
	}

	// A fresh clean-up that is left to age is deleted for good.
	batch2, err := svc.TrashUnusedUploads(ctx, admin.TrashActor{UserID: fx.adminID})
	if err != nil || batch2 == nil {
		t.Fatalf("second clean-up = %v, %v", batch2, err)
	}
	res, err := svc.PurgeDue(ctx)
	if err != nil || res.Batches != 0 {
		t.Fatalf("early PurgeDue = %+v, %v; want nothing", res, err)
	}
	mustGalleryExec(t, fx.db, `UPDATE trash_batches SET created_at = ? WHERE id = ?`, time.Now().UTC().AddDate(0, 0, -31), batch2.ID)
	res, err = svc.PurgeDue(ctx)
	if err != nil || res.Batches != 1 {
		t.Fatalf("aged PurgeDue = %+v, %v; want 1 batch", res, err)
	}
	for _, id := range []string{a, b} {
		if fx.rowExists("media_files", id) || fx.fileOnDisk(id) {
			t.Errorf("file %s survived the purge", id[:4])
		}
	}
	if !fx.rowExists("media_files", inCampaign) || !fx.rowExists("media_files", avatar) {
		t.Error("the purge reached a file that was never in the batch")
	}
	if fx.rowExists("trash_batches", batch2.ID) {
		t.Error("the purged batch row was left behind")
	}
}

func TestSiteTrash_EmptyNowNeedsThePasswordRecheck(t *testing.T) {
	fx := newTrashFixture(t)
	ctx := context.Background()
	if err := fx.campaigns.MoveToTrash(ctx, fx.campaignID, fx.adminID); err != nil {
		t.Fatal(err)
	}

	authStub := &trashAuth{adminID: fx.adminID}
	h := admin.NewHandler(nil, fx.campaigns, nil)
	h.SetTrashService(fx.trashService(trashFinder{}))
	e := trashEcho()
	admin.RegisterRoutes(e, h, authStub, nil)

	// Without the re-check: refused, and nothing is deleted.
	req := httptest.NewRequest(http.MethodDelete, "/admin/trash", nil)
	req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: fx.adminID})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || rec.Header().Get("HX-Trigger") != "reauth-required" {
		t.Fatalf("without re-check: status %d, HX-Trigger %q; want 403 reauth-required", rec.Code, rec.Header().Get("HX-Trigger"))
	}
	if !fx.rowExists("campaigns", fx.campaignID) {
		t.Fatal("the campaign was deleted without the re-check")
	}

	// A non-admin never reaches it.
	req = httptest.NewRequest(http.MethodDelete, "/admin/trash", nil)
	req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: fx.memberID})
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("a non-admin emptied the trash")
	}

	// With the re-check it empties, whatever the campaign's age.
	authStub.reauth = true
	req = httptest.NewRequest(http.MethodDelete, "/admin/trash", nil)
	req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: fx.adminID})
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("with re-check: status %d, body %s", rec.Code, rec.Body.String())
	}
	if fx.rowExists("campaigns", fx.campaignID) {
		t.Error("Empty now did not remove the trashed campaign")
	}
}
