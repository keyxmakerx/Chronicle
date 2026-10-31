// picture_trash_integration_test.go drives a campaign picture through the
// campaign Trash against a real MariaDB: trashed pictures leave the lists and
// upload dedup but still open for their pages, Restore brings them back, and
// the purge removes the row and the file.
//
// Skips when no database answers; see newADR058ScratchDB.
package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDB_PictureTrash(t *testing.T) {
	db := newADR058ScratchDB(t)
	ctx := context.Background()
	camp, owner := seedADR058Campaign(t, db)
	other, _ := seedADR058Campaign(t, db)
	pic := seedADR058MediaFile(t, db, camp, owner)
	mustADR058Exec(t, db, `UPDATE media_files SET content_hash = ?, thumbnail_paths = '{}' WHERE id = ?`, "ab12", pic)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, pic+".png"), []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := NewMediaRepository(db)
	svc := NewMediaService(repo, dir, 1<<20)

	if err := svc.DeleteCampaignMedia(ctx, other, pic); err == nil {
		t.Fatal("another campaign trashed the picture")
	}
	if err := svc.DeleteCampaignMedia(ctx, camp, pic); err != nil {
		t.Fatal(err)
	}

	if files, total, err := repo.ListByCampaign(ctx, camp, 50, 0); err != nil || total != 0 || len(files) != 0 {
		t.Errorf("media browser still lists the trashed picture: %d %v", total, err)
	}
	if hit, err := repo.FindByContentHash(ctx, camp, "ab12"); err != nil || hit != nil {
		t.Errorf("a new upload would reuse the trashed picture: %v %v", hit, err)
	}
	if f, err := repo.FindByID(ctx, pic); err != nil || f.InTrash() {
		t.Errorf("pages using the picture must still open it: %v %v", f, err)
	}
	if list, err := svc.ListTrashedPictures(ctx, other); err != nil || len(list) != 0 {
		t.Errorf("another campaign's Trash lists the picture: %v %v", list, err)
	}
	list, err := svc.ListTrashedPictures(ctx, camp)
	if err != nil || len(list) != 1 || list[0].ID != pic || list[0].Name != "test.png" {
		t.Fatalf("Trash lists %v (%v), want the picture", list, err)
	}

	if err := svc.RestoreCampaignPicture(ctx, other, pic); err == nil {
		t.Error("another campaign restored the picture")
	}
	if err := svc.RestoreCampaignPicture(ctx, camp, pic); err != nil {
		t.Fatal(err)
	}
	if err := svc.RestoreCampaignPicture(ctx, camp, pic); err == nil {
		t.Error("a picture not in the Trash restored again")
	}
	if _, total, _ := repo.ListByCampaign(ctx, camp, 50, 0); total != 1 {
		t.Errorf("restored picture missing from the browser (total %d)", total)
	}

	if err := svc.DeleteCampaignMedia(ctx, camp, pic); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.PurgeTrashedPictures(ctx, time.Now().UTC().Add(-time.Hour)); err != nil || n != 0 {
		t.Errorf("purged %d (%v) before the cutoff passed", n, err)
	}
	if n, err := svc.PurgeTrashedPictures(ctx, time.Now().UTC().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("purged %d (%v), want 1", n, err)
	}
	if _, err := repo.FindByID(ctx, pic); err == nil {
		t.Error("purged picture's row is still there")
	}
	if _, err := os.Stat(filepath.Join(dir, pic+".png")); !os.IsNotExist(err) {
		t.Errorf("purged picture's file is still on disk: %v", err)
	}
}
