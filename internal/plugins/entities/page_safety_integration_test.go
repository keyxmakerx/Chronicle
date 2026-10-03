package entities

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// TestPageSafety_Integration runs history, the Trash and the save-clash
// check against a real MariaDB: the SQL (recursive sub-page walk, the
// conditional revision UPDATE, the left-to-right SET in Update) is the part
// mocks can't check. Skipped under -short or when no database answers.
func TestPageSafety_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	ctx := context.Background()

	sam, kim := testUUID(t), testUUID(t)
	campaignID := testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		sam, "sam-"+sam+"@example.test", "Sam", "x")
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		kim, "kim-"+kim+"@example.test", "Kim", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Safety Test", "safety-"+campaignID[:8], sam)

	typeRepo := NewEntityTypeRepository(db)
	et := &EntityType{
		CampaignID: campaignID, Slug: "places", Name: "Place", NamePlural: "Places",
		Icon: "fa-map", Color: "#111111", Fields: []FieldDefinition{}, Layout: DefaultLayout(),
		SortOrder: 1, Enabled: true,
	}
	mustCreate(t, typeRepo, ctx, et)

	entityRepo := NewEntityRepository(db)
	safetyRepo := NewPageSafetyRepository(db)
	svc := NewEntityService(entityRepo, typeRepo, NewEntityPermissionRepository(db))
	svc.SetPageSafety(safetyRepo)
	safety := NewPageSafetyService(safetyRepo, entityRepo, svc, nil)

	asSam, asKim := WithActor(ctx, sam), WithActor(ctx, kim)
	doc := func(text string) (string, string) {
		return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`,
			"<p>" + text + "</p>"
	}

	mill, err := svc.Create(asSam, campaignID, sam, CreateEntityInput{Name: "Old Mill", EntityTypeID: et.ID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	cellar, err := svc.Create(asSam, campaignID, sam, CreateEntityInput{Name: "Mill Cellar", EntityTypeID: et.ID, ParentID: mill.ID})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	t.Run("saves are versioned, one row per person per sitting", func(t *testing.T) {
		j, h := doc("The cellar door is open.")
		if err := svc.UpdateEntry(asSam, mill.ID, j, h); err != nil {
			t.Fatal(err)
		}
		j, h = doc("The cellar door is open. Lamp oil.")
		if err := svc.UpdateEntry(asSam, mill.ID, j, h); err != nil {
			t.Fatal(err)
		}
		j, h = doc("The cellar door is barred from the inside. Lamp oil.")
		if err := svc.UpdateEntry(asKim, mill.ID, j, h); err != nil {
			t.Fatal(err)
		}
		hist, _, err := safety.History(ctx, mill.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(hist) != 3 {
			t.Fatalf("want 3 versions (created, Sam's sitting, Kim's), got %d", len(hist))
		}
		if hist[0].UserName != "Kim" || hist[1].UserName != "Sam" || hist[2].Kind != VersionCreated {
			t.Errorf("unexpected order/authors: %+v", []string{hist[0].UserName, hist[1].UserName, hist[2].Kind})
		}
		if hist[0].Added != 1 || hist[0].Removed != 1 {
			t.Errorf("Kim's change: want +1 -1 lines, got +%d -%d", hist[0].Added, hist[0].Removed)
		}
		var added string
		for _, p := range hist[0].Diff {
			if p.Op == '+' {
				added += p.Text
			}
		}
		if !strings.Contains(added, "barred") {
			t.Errorf("diff should mark the added words, got %+v", hist[0].Diff)
		}
	})

	t.Run("a stale save is refused and names who saved", func(t *testing.T) {
		rev, err := svc.EntryRev(ctx, mill.ID)
		if err != nil {
			t.Fatal(err)
		}
		stale := rev - 1
		j, h := doc("Sam's late edit")
		_, conflict, err := svc.SaveEntry(asSam, mill.ID, j, h, &stale)
		if err != nil {
			t.Fatal(err)
		}
		if conflict == nil || conflict.ByName != "Kim" || conflict.Rev != rev {
			t.Fatalf("want conflict by Kim at rev %d, got %+v", rev, conflict)
		}
		got, _ := svc.GetByID(ctx, mill.ID)
		if strings.Contains(*got.EntryHTML, "late edit") {
			t.Error("a refused save must not write")
		}
		newRev, conflict, err := svc.SaveEntry(asSam, mill.ID, j, h, &rev)
		if err != nil || conflict != nil || newRev != rev+1 {
			t.Fatalf("save at current rev: rev=%d conflict=%+v err=%v", newRev, conflict, err)
		}
	})

	t.Run("a rename alone keeps the text revision", func(t *testing.T) {
		before, _ := svc.EntryRev(ctx, mill.ID)
		input := UpdateEntityInput{}
		if err := input.Name.UnmarshalJSON([]byte(`"The Old Mill"`)); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Update(asSam, mill.ID, input); err != nil {
			t.Fatal(err)
		}
		after, _ := svc.EntryRev(ctx, mill.ID)
		if after != before {
			t.Errorf("entry_rev moved on a rename: %d -> %d", before, after)
		}
	})

	t.Run("restore puts an old version back as a new version", func(t *testing.T) {
		hist, _, _ := safety.History(ctx, mill.ID, "")
		var sams *VersionView
		for i := range hist {
			if hist[i].UserName == "Sam" && strings.Contains(derefStr(hist[i].EntryHTML), "Lamp oil") &&
				strings.Contains(derefStr(hist[i].EntryHTML), "open") {
				sams = &hist[i]
			}
		}
		if sams == nil {
			t.Fatal("Sam's first sitting not found")
		}
		if err := safety.RestoreVersion(asKim, mill.ID, sams.ID); err != nil {
			t.Fatal(err)
		}
		got, _ := svc.GetByID(ctx, mill.ID)
		if derefStr(got.EntryHTML) != derefStr(sams.EntryHTML) || got.Name != "Old Mill" {
			t.Errorf("restored page: name=%q html=%q", got.Name, derefStr(got.EntryHTML))
		}
		latest, _ := safetyRepo.LatestVersion(ctx, mill.ID)
		if latest.Kind != VersionRestore {
			t.Errorf("latest kind = %q, want restore", latest.Kind)
		}
		if err := safety.RestoreVersion(ctx, cellar.ID, sams.ID); !isNotFound(err) {
			t.Errorf("a version of another page must be not found, got %v", err)
		}
	})

	t.Run("delete trashes the page with its sub-pages; restore brings both", func(t *testing.T) {
		if err := svc.Delete(asSam, mill.ID); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{mill.ID, cellar.ID} {
			if _, err := svc.GetByID(ctx, id); !isNotFound(err) {
				t.Errorf("trashed page %s still readable: %v", id, err)
			}
		}
		list, total, err := svc.List(ctx, campaignID, 0, permissions.RoleOwner, sam, ListOptions{Page: 1, PerPage: 50})
		if err != nil || total != 0 || len(list) != 0 {
			t.Errorf("owner list should be empty while trashed: total=%d err=%v", total, err)
		}
		items, err := safety.ListTrash(ctx, campaignID)
		if err != nil || len(items) != 1 || items[0].SubPages != 1 || items[0].DeletedByName != "Sam" {
			t.Fatalf("trash: %+v err=%v", items, err)
		}
		if _, err := safety.RestoreFromTrash(ctx, campaignID, mill.ID); err != nil {
			t.Fatal(err)
		}
		got, err := svc.GetByID(ctx, cellar.ID)
		if err != nil || derefStr(got.ParentID) != mill.ID {
			t.Errorf("sub-page should come back under its page: %+v err=%v", got, err)
		}
		if _, err := safety.RestoreFromTrash(ctx, "other-campaign", mill.ID); !isNotFound(err) {
			t.Errorf("restore from another campaign must be not found, got %v", err)
		}
	})

	t.Run("a page restored without its trashed parent moves to the top", func(t *testing.T) {
		if err := svc.Delete(asSam, cellar.ID); err != nil {
			t.Fatal(err)
		}
		if err := svc.Delete(asSam, mill.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := safety.RestoreFromTrash(ctx, campaignID, cellar.ID); err != nil {
			t.Fatal(err)
		}
		got, err := svc.GetByID(ctx, cellar.ID)
		if err != nil || got.ParentID != nil {
			t.Errorf("want cellar at top level, got %+v err=%v", got, err)
		}
	})

	t.Run("a page kind with pages in the Trash can't be deleted", func(t *testing.T) {
		kind := &EntityType{
			CampaignID: campaignID, Slug: "ruins", Name: "Ruin", NamePlural: "Ruins",
			Icon: "fa-dungeon", Color: "#222222", Fields: []FieldDefinition{}, Layout: DefaultLayout(),
			SortOrder: 2, Enabled: true,
		}
		mustCreate(t, typeRepo, ctx, kind)
		ruin, err := svc.Create(asSam, campaignID, sam, CreateEntityInput{Name: "Old Tower", EntityTypeID: kind.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.Delete(asSam, ruin.ID); err != nil {
			t.Fatal(err)
		}
		var ae *apperror.AppError
		err = svc.DeleteEntityType(ctx, kind.ID)
		if !errors.As(err, &ae) || ae.Code != 409 || !strings.Contains(ae.Message, "1 of its pages are in the Trash") {
			t.Fatalf("want a 409 naming the trashed page, got %v", err)
		}
	})

	t.Run("a refused save returns the stored text and revision", func(t *testing.T) {
		rev, _ := svc.EntryRev(ctx, cellar.ID)
		j, h := doc("first")
		if _, c, err := svc.SaveEntry(asSam, cellar.ID, j, h, &rev); err != nil || c != nil {
			t.Fatalf("save: conflict=%+v err=%v", c, err)
		}
		j2, h2 := doc("second")
		_, c, err := svc.SaveEntry(asKim, cellar.ID, j2, h2, &rev)
		if err != nil || c == nil || c.Rev != rev+1 || derefStr(c.EntryHTML) != h {
			t.Fatalf("want conflict at rev %d with %q, got %+v err=%v", rev+1, h, c, err)
		}
	})

	t.Run("purge removes pages that waited out the retention", func(t *testing.T) {
		mustExec(t, db, `UPDATE entities SET deleted_at = ? WHERE id = ?`, time.Now().UTC().AddDate(0, 0, -31), mill.ID)
		safety.(*pageSafetyService).purgeOnce(ctx)
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM entities WHERE id = ?`, mill.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Error("purge left the expired page")
		}
		if _, err := svc.GetByID(ctx, cellar.ID); err != nil {
			t.Errorf("purge must not touch live pages: %v", err)
		}
	})

	t.Run("first save of an older page keeps what it replaced", func(t *testing.T) {
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, entry, entry_html,
			created_by, created_at, updated_at) VALUES (?, ?, ?, 'Legacy', ?, ?, '<p>old text</p>', ?, NOW(), NOW())`,
			id, campaignID, et.ID, "legacy-"+id[:8], `{"type":"doc"}`, sam)
		j, h := doc("new text")
		if err := svc.UpdateEntry(asKim, id, j, h); err != nil {
			t.Fatal(err)
		}
		hist, _, _ := safety.History(ctx, id, "")
		if len(hist) != 2 || hist[1].Kind != VersionBaseline || derefStr(hist[1].EntryHTML) != "<p>old text</p>" {
			t.Errorf("want edit + baseline of the old text, got %+v", hist)
		}
	})
}

func isNotFound(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == 404
}
