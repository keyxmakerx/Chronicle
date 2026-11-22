// card_picture_list_integration_test.go runs the picker's query-level
// exclusion against a real MariaDB: the total and the page agree when files are
// left out, and the usual leave-outs still apply. Skips when no database
// answers; see newADR058ScratchDB.
package media

import (
	"context"
	"testing"
)

func TestDB_ListByCampaignExcluding(t *testing.T) {
	db := newADR058ScratchDB(t)
	ctx := context.Background()
	camp, owner := seedADR058Campaign(t, db)
	other, _ := seedADR058Campaign(t, db)

	var ids []string
	for i := 0; i < 7; i++ {
		ids = append(ids, seedADR058MediaFile(t, db, camp, owner))
	}
	foreign := seedADR058MediaFile(t, db, other, owner)
	trashed := seedADR058MediaFile(t, db, camp, owner)
	mustADR058Exec(t, db, `UPDATE media_files SET deleted_at = NOW() WHERE id = ?`, trashed)
	note := seedADR058MediaFile(t, db, camp, owner)
	mustADR058Exec(t, db, `UPDATE media_files SET usage_type = ? WHERE id = ?`, UsageNoteImage, note)
	mustADR058Exec(t, db, `UPDATE media_files SET thumbnail_paths = '{}'`)
	repo := NewMediaRepository(db)

	// Nothing excluded is the old list exactly.
	files, total, err := repo.ListByCampaignExcluding(ctx, camp, nil, 50, 0)
	if err != nil || total != 7 || len(files) != 7 {
		t.Fatalf("no exclusion: total %d, %d files, %v; want 7", total, len(files), err)
	}
	files2, total2, err := repo.ListByCampaign(ctx, camp, 50, 0)
	if err != nil || total2 != total || len(files2) != len(files) {
		t.Fatalf("ListByCampaign differs from the unexcluded list: %d vs %d (%v)", total2, total, err)
	}

	// Leave three out, including a foreign, a trashed and a note picture id (which
	// were never listed): the total drops by exactly the three that were listed
	// and the pages hold the rest with no gaps.
	exclude := []string{ids[0], ids[3], ids[6], foreign, trashed, note}
	seen := map[string]bool{}
	for page := 0; page < 3; page++ {
		got, total, err := repo.ListByCampaignExcluding(ctx, camp, exclude, 2, page*2)
		if err != nil || total != 4 {
			t.Fatalf("page %d: total %d (%v), want 4", page, total, err)
		}
		wantLen := 2
		if page == 2 {
			wantLen = 0
		}
		if len(got) != wantLen {
			t.Errorf("page %d holds %d files, want %d", page, len(got), wantLen)
		}
		for _, f := range got {
			seen[f.ID] = true
		}
	}
	for _, id := range []string{ids[0], ids[3], ids[6]} {
		if seen[id] {
			t.Errorf("excluded file %s was listed", id)
		}
	}
	if len(seen) != 4 {
		t.Errorf("pages held %d distinct files, want 4", len(seen))
	}

	// Excluding every file lists nothing and counts nothing.
	if got, total, err := repo.ListByCampaignExcluding(ctx, camp, ids, 50, 0); err != nil || total != 0 || len(got) != 0 {
		t.Errorf("all excluded: total %d, %d files, %v", total, len(got), err)
	}
	// A long exclusion list works as one query.
	many := make([]string, 0, 5000)
	for i := 0; i < 5000; i++ {
		many = append(many, "no-such-file-"+itoa(i))
	}
	many = append(many, ids[1])
	if _, total, err := repo.ListByCampaignExcluding(ctx, camp, many, 50, 0); err != nil || total != 6 {
		t.Errorf("5001 excluded ids: total %d (%v), want 6", total, err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
