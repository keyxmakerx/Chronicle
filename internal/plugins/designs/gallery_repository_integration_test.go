package designs

// Real-MariaDB tests for the gallery repository: the migration applies on a
// fresh schema, Browse lists visible entries only with its filters and paging,
// reports are once per reporter and cascade with the entry, and the duplicate
// and rate lookups read what the service writes.
//
//	make test-db-up
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/designs/ -run TestDBGallery

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func galleryEntry(user, kind, system, name, status string) *GalleryEntry {
	id := uuid.NewString()
	return &GalleryEntry{
		ID: id, PublisherUserID: user, SourceDesignID: uuid.NewString(), Kind: kind, SystemID: system, Name: name, MakerName: "Maker",
		Data: json.RawMessage(`{"part":"oak"}`), DataHash: id, Status: status, Reasons: []string{ReasonLink},
	}
}

func TestDBGalleryBrowseAndReports(t *testing.T) {
	db := scratch(t)
	r := NewGalleryRepository(db)
	ctx := context.Background()
	alice, bob := seedUser(t, db, "Alice"), seedUser(t, db, "Bob")
	vis := galleryEntry(alice, KindRoomTheme, "drawsteel", "Oak_Room 100%", StatusVisible)
	for _, e := range []*GalleryEntry{
		vis,
		galleryEntry(alice, "crafting-circle", "drawsteel", "Star", StatusVisible),
		galleryEntry(alice, KindRoomTheme, "dnd5e", "Tavern", StatusVisible),
		galleryEntry(alice, KindRoomTheme, "drawsteel", "Waiting", StatusPending),
	} {
		if err := r.Insert(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	list, total, err := r.Browse(ctx, BrowseQuery{SystemID: "drawsteel", Limit: 10})
	if err != nil || total != 2 || len(list) != 2 {
		t.Fatalf("drawsteel visible: %d/%d, %v", len(list), total, err)
	}
	if _, total, _ = r.Browse(ctx, BrowseQuery{AnySys: true, Limit: 10}); total != 3 {
		t.Fatalf("all systems visible = %d", total)
	}
	if _, total, _ = r.Browse(ctx, BrowseQuery{AnySys: true, Prefix: "crafting-", Limit: 10}); total != 1 {
		t.Fatalf("crafting = %d", total)
	}
	if _, total, _ = r.Browse(ctx, BrowseQuery{AnySys: true, Kinds: []string{KindRoomTheme}, Limit: 10}); total != 2 {
		t.Fatalf("rooms = %d", total)
	}
	// Wildcards in a search are literal.
	for text, want := range map[string]int{"oak_room": 1, "100%": 1, "%": 1, "_": 1, "mak": 3, "zz": 0} {
		if _, total, _ = r.Browse(ctx, BrowseQuery{AnySys: true, Text: text, Limit: 10}); total != want {
			t.Errorf("search %q = %d, want %d", text, total, want)
		}
	}
	if page, _, _ := r.Browse(ctx, BrowseQuery{AnySys: true, Limit: 2, Offset: 2}); len(page) != 1 {
		t.Fatalf("second page = %d", len(page))
	}
	if st, n, _ := r.ByStatus(ctx, StatusPending, 10, 0); n != 1 || st[0].Name != "Waiting" || st[0].Reasons[0] != ReasonLink {
		t.Fatalf("pending = %+v", st)
	}

	if ok, _ := r.AddReport(ctx, vis.ID, bob, ReportSpam, "note"); !ok {
		t.Fatal("first report refused")
	}
	if ok, _ := r.AddReport(ctx, vis.ID, bob, ReportSpam, "again"); ok {
		t.Fatal("second report from the same reporter accepted")
	}
	if by, _ := r.ReportedBy(ctx, bob, []string{vis.ID, "other"}); !by[vis.ID] || by["other"] {
		t.Fatalf("reportedBy = %v", by)
	}
	vis.Status, vis.Reasons, vis.ReviewedBy = StatusRemoved, []string{ReasonReported}, alice
	if err := r.Save(ctx, vis); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Get(ctx, vis.ID); got.Status != StatusRemoved || got.ReviewedBy != alice {
		t.Fatalf("saved = %+v", got)
	}
	if err := r.Delete(ctx, vis.ID); err != nil {
		t.Fatal(err)
	}
	if rs, _ := r.Reports(ctx, vis.ID); len(rs) != 0 {
		t.Fatal("reports survived the entry")
	}
}

func TestDBGalleryDuplicateAndEvents(t *testing.T) {
	db := scratch(t)
	r := NewGalleryRepository(db)
	ctx := context.Background()
	alice := seedUser(t, db, "Alice")
	e := galleryEntry(alice, KindRoomTheme, "drawsteel", "A", StatusVisible)
	e.DataHash = strings.Repeat("a", 64)
	if err := r.Insert(ctx, e); err != nil {
		t.Fatal(err)
	}
	if dup, _ := r.DuplicateExists(ctx, alice, e.DataHash, ""); !dup {
		t.Fatal("duplicate not found")
	}
	if dup, _ := r.DuplicateExists(ctx, alice, e.DataHash, e.ID); dup {
		t.Fatal("an entry is not its own duplicate")
	}
	if dup, _ := r.DuplicateExists(ctx, "someone-else", e.DataHash, ""); dup {
		t.Fatal("another user's data counted")
	}
	if got, _ := r.BySource(ctx, alice, e.SourceDesignID); got == nil || got.ID != e.ID {
		t.Fatal("BySource missed")
	}
	// The same library design cannot be published twice by one user.
	twin := galleryEntry(alice, KindRoomTheme, "drawsteel", "B", StatusVisible)
	twin.SourceDesignID = e.SourceDesignID
	if err := r.Insert(ctx, twin); err == nil {
		t.Fatal("unique source key not enforced")
	}
	for i := 0; i < 3; i++ {
		if err := r.AddEvent(ctx, alice); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := r.CountEvents(ctx, alice, time.Now().Add(-time.Hour)); n != 3 {
		t.Fatalf("events = %d", n)
	}
	if n, _ := r.CountEvents(ctx, alice, time.Now().Add(time.Hour)); n != 0 {
		t.Fatalf("future window = %d", n)
	}
}
