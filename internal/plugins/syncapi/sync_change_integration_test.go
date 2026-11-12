package syncapi

// The change feed's watermark arithmetic is hand-written SQL; this runs it
// against a migrated scratch schema. Skips when no server is reachable.
//
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/syncapi/ -run TestSyncChangeIntegration

import (
	"context"
	"testing"
	"time"
)

func TestSyncChangeIntegration_HeadAndGap(t *testing.T) {
	db := newSyncStatsScratchDB(t)
	ctx := context.Background()
	uid, cid, other := "u-chg-000000000000000000000000000001", "c-chg-000000000000000000000000000001", "c-chg-000000000000000000000000000002"
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, []any{uid, "chg@example.test", "Ren", "x"}},
		{`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, []any{cid, "Quiet", "quiet", uid}},
		{`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, []any{other, "Busy", "busy", uid}},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewSyncChangeRepository(db)

	last, err := repo.Append(ctx, cid, "entity", "e1", "updated")
	if err != nil {
		t.Fatal(err)
	}
	// Age the row past retention, as a campaign quiet for over 30 days would.
	if _, err := db.Exec(`UPDATE sync_changes SET created_at = ? WHERE campaign_id = ?`, time.Now().UTC().Add(-40*24*time.Hour), cid); err != nil {
		t.Fatal(err)
	}
	if n, err := repo.PruneOlderThan(ctx, time.Now().UTC().Add(-changeRetention)); err != nil || n != 1 {
		t.Fatalf("prune = %d, %v; want 1 row", n, err)
	}

	// Every row is gone: the head must stay at the watermark, not fall to 0.
	head, err := repo.Head(ctx, cid)
	if err != nil {
		t.Fatal(err)
	}
	if head != last {
		t.Fatalf("head after full prune = %d, want the watermark %d", head, last)
	}

	// A gap raises the watermark to a seq later rows cannot reuse.
	if _, err := repo.Append(ctx, other, "entity", "x", "updated"); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkGap(ctx, cid); err != nil {
		t.Fatal(err)
	}
	pruned, err := repo.PrunedThrough(ctx, cid)
	if err != nil {
		t.Fatal(err)
	}
	if pruned <= last {
		t.Fatalf("watermark after gap = %d, want above %d so a cursor at the old head resets", pruned, last)
	}
	if head, _ = repo.Head(ctx, cid); head != pruned {
		t.Fatalf("head after gap = %d, want watermark %d", head, pruned)
	}
	after, err := repo.Append(ctx, cid, "entity", "e2", "updated")
	if err != nil {
		t.Fatal(err)
	}
	if after <= pruned {
		t.Fatalf("row after gap has seq %d, must exceed watermark %d to stay replayable", after, pruned)
	}
	if head, _ = repo.Head(ctx, cid); head != after {
		t.Fatalf("head = %d, want newest row %d", head, after)
	}
}
