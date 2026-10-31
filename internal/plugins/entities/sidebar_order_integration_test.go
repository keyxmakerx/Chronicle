package entities

import (
	"context"
	"testing"
)

// TestChildOrderRepository_Integration round-trips the sidebar orders
// against a real MariaDB: a set, a change, A to Z removing the row, and the
// campaign's deletion taking its orders with it. Skipped under -short; skips
// when no database answers (see TestEntityTypeRepository_Integration).
func TestChildOrderRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := NewChildOrderRepository(db)

	userID, campaignID := testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "order-int-"+userID+"@example.test", "Order Int Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Order Int Test", "order-int-"+campaignID[:8], userID)
	defer mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)

	steps := []struct {
		kind, id, mode string
		want           map[string]string
	}{
		{OrderContainerCategory, "7", OrderHand, map[string]string{"category:7": OrderHand}},
		{OrderContainerFolder, "f-1", OrderNew, map[string]string{"category:7": OrderHand, "folder:f-1": OrderNew}},
		{OrderContainerCategory, "7", OrderNew, map[string]string{"category:7": OrderNew, "folder:f-1": OrderNew}},
		{OrderContainerFolder, "f-1", OrderAZ, map[string]string{"category:7": OrderNew}},
		{OrderContainerPage, "p-1", OrderAZ, map[string]string{"category:7": OrderNew}},
	}
	for _, s := range steps {
		if err := repo.Set(ctx, campaignID, s.kind, s.id, s.mode); err != nil {
			t.Fatalf("Set(%s %s %s): %v", s.kind, s.id, s.mode, err)
		}
		got, err := repo.ListByCampaign(ctx, campaignID)
		if err != nil {
			t.Fatalf("ListByCampaign: %v", err)
		}
		if len(got) != len(s.want) {
			t.Fatalf("after %s %s %s: got %v, want %v", s.kind, s.id, s.mode, got, s.want)
		}
		for k, v := range s.want {
			if got[k] != v {
				t.Fatalf("after %s %s %s: got %v, want %v", s.kind, s.id, s.mode, got, s.want)
			}
		}
	}

	mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
	if got, _ := repo.ListByCampaign(ctx, campaignID); len(got) != 0 {
		t.Errorf("a deleted campaign keeps its orders: %v", got)
	}
}
