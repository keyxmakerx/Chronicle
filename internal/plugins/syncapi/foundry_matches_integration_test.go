package syncapi

// The match store round-trips the DM's choices, clears a page's row when
// its choice goes back to the default, and loses the row with its page.
// Skips when no server is reachable.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/syncapi/ -run TestFoundryMatchesIntegration

import (
	"context"
	"testing"
)

func TestFoundryMatchesIntegration_Store(t *testing.T) {
	db := newSyncStatsScratchDB(t)
	ctx := context.Background()
	uid, cid, eid := "u-fm-0000000000000000000000000000001", "c-fm-0000000000000000000000000000001", "e-fm-0000000000000000000000000000001"
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, []any{uid, "fm@example.test", "Ren", "x"}},
		{`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, []any{cid, "Matched", "matched", uid}},
		{`INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?,?,?,?)`, []any{cid, "creature", "Creature", "Creatures"}},
		{`INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, created_by, created_at, updated_at)
			SELECT ?, ?, id, 'Stalker', 'stalker', ?, NOW(), NOW() FROM entity_types WHERE campaign_id = ?`, []any{eid, cid, uid, cid}},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewFoundryMatchRepository(db)
	if ch, err := repo.Get(ctx, cid, eid); err != nil || ch != nil {
		t.Fatalf("no row yet: %+v %v", ch, err)
	}
	want := FoundryMatchChoice{EntityID: eid, Name: "Stalker", Pack: "Monsters", Use: MatchUsePage, NotSame: []string{"Stalker Brute"}}
	if err := repo.Put(ctx, cid, want); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, cid, eid)
	if err != nil || got == nil || got.Name != "Stalker" || got.Pack != "Monsters" || got.Use != MatchUsePage || len(got.NotSame) != 1 {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if all, err := repo.List(ctx, cid); err != nil || all[eid].Use != MatchUsePage {
		t.Fatalf("list: %+v %v", all, err)
	}
	if err := repo.Put(ctx, cid, FoundryMatchChoice{EntityID: eid}); err != nil {
		t.Fatal(err)
	}
	if ch, _ := repo.Get(ctx, cid, eid); ch != nil {
		t.Fatalf("a default choice keeps no row: %+v", ch)
	}
	if err := repo.Put(ctx, cid, want); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM entities WHERE id = ?`, eid); err != nil {
		t.Fatal(err)
	}
	if ch, _ := repo.Get(ctx, cid, eid); ch != nil {
		t.Fatalf("the row goes with its page: %+v", ch)
	}
}
