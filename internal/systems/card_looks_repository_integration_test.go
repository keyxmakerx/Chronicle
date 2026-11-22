package systems

import (
	"context"
	"strings"
	"testing"
)

// TestCardLookRepository_Integration runs the hand-written SQL against a real
// MariaDB: one row per (campaign, system, field, entry), an upsert replacing
// the same card's row, batches that apply together, derived-key deletion that
// matches the slug literally (LIKE wildcards included) and stays inside its
// campaign and system, and the cascade on campaign delete. Skipped under
// -short or when no server answers.
func TestCardLookRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openBookTestDB(t)
	ctx := context.Background()
	repo := NewCardLookRepository(db)

	userID, campA, campB := bookTestUUID(t), bookTestUUID(t), bookTestUUID(t)
	mustBookExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "look-"+userID+"@example.test", "Look Test", "x")
	for _, c := range []string{campA, campB} {
		mustBookExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
			c, "Look "+c, "look-"+c, userID)
	}
	row := func(camp, sys, field, key, border, colour string) CardLookRow {
		return CardLookRow{CampaignID: camp, SystemID: sys, FieldKey: field, EntryKey: key,
			StoredLook: StoredLook{Border: border, Colour: colour}, UpdatedBy: userID}
	}

	longKey := strings.Repeat("k", MaxCardLookKeyLen)
	if err := repo.Apply(ctx, []CardLookRow{
		row(campA, "sys", "ancestry", "hakaan", "fire", "#abcdef"),
		row(campA, "sys", "ancestry", "devil", "", ""),
		row(campA, "sys", "ancestry", longKey, "ice", ""),
		row(campA, "sys", "subclass", "fury--berserker", "vine", ""),
		row(campA, "sys", "subclass", "fury--reaver", "vine", ""),
		row(campA, "sys", "subclass", "furyx--other", "vine", ""),
		row(campA, "sys", "subclass", "fu_y--wild", "vine", ""),
		row(campA, "other", "subclass", "fury--berserker", "vine", ""),
		row(campB, "sys", "ancestry", "hakaan", "storm", ""),
	}, nil); err != nil {
		t.Fatal(err)
	}
	// The same card is replaced, not duplicated.
	if err := repo.Apply(ctx, []CardLookRow{row(campA, "sys", "ancestry", "hakaan", "ice", "")}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := repo.List(ctx, campA, "sys", "ancestry")
	if err != nil || len(got) != 3 {
		t.Fatalf("list = %v, %v; want 3 rows and nothing from campaign B or another field", got, err)
	}
	if got["hakaan"] != (StoredLook{Border: "ice"}) {
		t.Errorf("hakaan = %+v, want the replaced row", got["hakaan"])
	}
	if l, ok := got["devil"]; !ok || l.Border != "" {
		t.Errorf("an explicit no-border row must come back as a row: %+v %v", l, ok)
	}
	if _, ok := got[longKey]; !ok {
		t.Error("a key at the column's full width must round-trip untruncated")
	}
	if b, _ := repo.List(ctx, campB, "sys", "ancestry"); b["hakaan"].Border != "storm" {
		t.Errorf("campaign B's row = %+v", b)
	}

	// Delete in a batch together with an upsert.
	if err := repo.Apply(ctx, []CardLookRow{row(campA, "sys", "ancestry", "elf", "glass", "")},
		[]CardLookRow{row(campA, "sys", "ancestry", "devil", "", "")}); err != nil {
		t.Fatal(err)
	}
	if got, _ = repo.List(ctx, campA, "sys", "ancestry"); len(got) != 3 || got["elf"].Border != "glass" {
		t.Errorf("after batch: %v", got)
	}
	if _, ok := got["devil"]; ok {
		t.Error("devil should be deleted")
	}

	// Derived deletion: only <slug>--%, in this campaign and system, any field.
	// A '_' in the slug is a literal underscore, never a one-character wildcard.
	n, err := repo.DeleteDerived(ctx, campA, "sys", "fury")
	if err != nil || n != 2 {
		t.Fatalf("DeleteDerived(fury) = %d, %v; want 2", n, err)
	}
	if n, _ := repo.DeleteDerived(ctx, campA, "sys", "fu_y"); n != 1 {
		t.Errorf("DeleteDerived(fu_y) = %d, want exactly the fu_y-- row", n)
	}
	if n, _ := repo.DeleteDerived(ctx, campA, "sys", "fu%"); n != 0 {
		t.Errorf("a percent sign in the slug must not match as a wildcard, deleted %d", n)
	}
	if n, _ := repo.DeleteDerived(ctx, campA, "sys", ""); n != 0 {
		t.Errorf("an empty slug deleted %d rows", n)
	}
	if left, _ := repo.List(ctx, campA, "sys", "subclass"); len(left) != 1 || left["furyx--other"].Border == "" {
		t.Errorf("subclass rows left = %v, want only furyx--other", left)
	}
	if o, _ := repo.List(ctx, campA, "other", "subclass"); len(o) != 1 {
		t.Error("another system's row was deleted")
	}

	// The rows go with their campaign.
	mustBookExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campB)
	if b, _ := repo.List(ctx, campB, "sys", "ancestry"); len(b) != 0 {
		t.Errorf("rows survived their campaign: %v", b)
	}
}
