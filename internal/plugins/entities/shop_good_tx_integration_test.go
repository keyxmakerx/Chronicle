package entities

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/database"
)

// Shop good pages made inside one transaction (Fill the shelves) are kept
// together or not at all, and what must wait for the commit (history, the
// live event) never runs for a rolled-back batch.
func TestShopGoods_AtomicBatch_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	userID, camp := testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`, userID, "tx-"+userID+"@example.test", "Tx", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, camp, "Tx", "tx-"+camp[:8], userID)
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, camp)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)
	}()
	res, err := db.ExecContext(ctx, `INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?, 'item', 'Item', 'Items')`, camp)
	if err != nil {
		t.Fatal(err)
	}
	typeID, _ := res.LastInsertId()
	svc := NewEntityService(NewEntityRepository(db), NewEntityTypeRepository(db), NewEntityPermissionRepository(db)).(*entityService)

	count := func() int {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entities WHERE campaign_id = ? AND shop_good = 1`, camp).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	boom := errors.New("the third listing failed")
	ran := false
	err = database.RunInTx(ctx, db, func(ctx context.Context) error {
		database.AfterCommit(ctx, func() { ran = true })
		for _, n := range []string{"Rope", "Lamp", "Rope"} { // the last reuses the first inside the transaction
			if _, err := svc.EnsureShopGood(ctx, camp, userID, int(typeID), n, false); err != nil {
				return err
			}
		}
		if count() != 0 {
			t.Error("uncommitted pages visible from another connection")
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if got := count(); got != 0 || ran {
		t.Fatalf("after rollback: %d shop goods left, after-commit ran = %v", got, ran)
	}

	err = database.RunInTx(ctx, db, func(ctx context.Context) error {
		for _, n := range []string{"Rope", "Lamp", "Rope"} {
			if _, err := svc.EnsureShopGood(ctx, camp, userID, int(typeID), n, true); err != nil {
				return err
			}
		}
		database.AfterCommit(ctx, func() { ran = true })
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 2 || !ran {
		t.Fatalf("after commit: %d shop goods (want 2, the repeated name shares one page), after-commit ran = %v", got, ran)
	}
	var hidden int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entities WHERE campaign_id = ? AND shop_good = 1 AND is_private = 1`, camp).Scan(&hidden)
	if hidden != 2 {
		t.Errorf("%d of the pages are hidden, want both (they were made for hidden listings)", hidden)
	}
}
