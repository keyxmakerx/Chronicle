package systems

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TestSystemEntryRepository_Integration runs the hand-written SQL against a
// real MariaDB: the unique slug, case-insensitive name lookup, visibility
// filtering, campaign scoping and the cascade on campaign delete. Skipped
// under -short or when no server answers (see openBookTestDB).
func TestSystemEntryRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openBookTestDB(t)
	ctx := context.Background()
	repo := NewSystemEntryRepository(db)

	userID, campA, campB := bookTestUUID(t), bookTestUUID(t), bookTestUUID(t)
	mustBookExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "entry-"+userID+"@example.test", "Entry Test", "x")
	for _, c := range []string{campA, campB} {
		mustBookExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
			c, "Entry "+c, "entry-"+c, userID)
	}

	mk := func(camp, slug, name, vis string) *SystemEntry {
		return &SystemEntry{CampaignID: camp, SystemID: "sys", FieldKey: "ancestry", Slug: slug, Name: name,
			Summary: "s", Description: "<p>d</p>", Properties: map[string]any{"size": "Medium", "speed": 5}, Visibility: vis, CreatedBy: userID}
	}
	open, hidden := mk(campA, "open", "Open", "everyone"), mk(campA, "hidden", "Hidden", "directors")
	for _, e := range []*SystemEntry{open, hidden, mk(campB, "open", "Open", "everyone")} {
		if err := repo.Create(ctx, e); err != nil || e.ID == 0 {
			t.Fatalf("create %s: id %d err %v", e.Slug, e.ID, err)
		}
	}

	// The same slug in the same field is a conflict, in another campaign it is fine.
	err := repo.Create(ctx, mk(campA, "open", "Other", "everyone"))
	if ae, ok := err.(*apperror.AppError); !ok || ae.Code != 409 {
		t.Fatalf("duplicate slug = %v, want 409", err)
	}

	got, err := repo.Get(ctx, campA, open.ID)
	if err != nil || got.Properties["size"] != "Medium" || got.Properties["speed"] != 5.0 || got.CreatedBy != userID {
		t.Fatalf("get = %+v err %v", got, err)
	}
	if _, err := repo.Get(ctx, campB, open.ID); err == nil {
		t.Fatal("get crossed campaigns")
	}

	if l, _ := repo.List(ctx, campA, "sys", "ancestry", false); len(l) != 1 || l[0].Slug != "open" {
		t.Fatalf("player list = %+v", l)
	}
	if l, _ := repo.List(ctx, campA, "sys", "", true); len(l) != 2 {
		t.Fatalf("director list = %d, want 2", len(l))
	}
	if l, _ := repo.List(ctx, campA, "other-sys", "", true); len(l) != 0 {
		t.Fatalf("another system's list = %d, want 0", len(l))
	}

	if e, _ := repo.FindByName(ctx, campA, "sys", "ancestry", "HIDDEN"); e == nil || e.ID != hidden.ID {
		t.Fatal("name lookup is not case-insensitive")
	}
	if ok, _ := repo.SlugExists(ctx, campA, "sys", "ancestry", "open"); !ok {
		t.Fatal("slug should exist")
	}
	if n, _ := repo.Count(ctx, campA, "sys", "ancestry"); n != 2 {
		t.Fatalf("count = %d", n)
	}

	hidden.Name, hidden.Properties, hidden.Visibility = "Renamed", map[string]any{}, "everyone"
	if err := repo.Update(ctx, hidden); err != nil {
		t.Fatal(err)
	}
	if g, _ := repo.Get(ctx, campA, hidden.ID); g.Name != "Renamed" || len(g.Properties) != 0 || g.Slug != "hidden" {
		t.Fatalf("after update = %+v", g)
	}

	if ok, _ := repo.Delete(ctx, campB, open.ID); ok {
		t.Fatal("delete crossed campaigns")
	}
	if ok, _ := repo.Delete(ctx, campA, open.ID); !ok {
		t.Fatal("delete failed")
	}

	// Deleting the campaign removes its entries.
	mustBookExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campA)
	if n, _ := repo.Count(ctx, campA, "sys", "ancestry"); n != 0 {
		t.Fatalf("entries survived campaign delete: %d", n)
	}
}
