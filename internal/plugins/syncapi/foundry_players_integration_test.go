package syncapi

// Replace swaps a campaign's whole player list, but must keep what an
// earlier report knew about a user that the new one doesn't (each Foundry
// session starts its counts empty). Skips when no server is reachable.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/syncapi/ -run TestFoundryPlayersIntegration

import (
	"context"
	"testing"
	"time"
)

func TestFoundryPlayersIntegration_ReplaceKeepsEarlierActivity(t *testing.T) {
	db := newSyncStatsScratchDB(t)
	ctx := context.Background()
	uid, cid := "u-fp-0000000000000000000000000000001", "c-fp-0000000000000000000000000000001"
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, uid, "fp@example.test", "Ren", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, cid, "Synced", "synced", uid); err != nil {
		t.Fatal(err)
	}
	repo := NewFoundryPlayerRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	synced := now.Add(-48 * time.Hour)
	failed := now.Add(-24 * time.Hour)
	first := []FoundryPlayer{
		{FoundryUserID: "f1", Name: "Ann", LastChangeAt: &synced, LastFailedAt: &failed, LastFailure: "HTTP 403 Forbidden", FailedCount: 2, ReportedAt: now.Add(-time.Hour)},
		{FoundryUserID: "f2", Name: "Gone", ReportedAt: now.Add(-time.Hour)},
	}
	if err := repo.Replace(ctx, cid, first); err != nil {
		t.Fatal(err)
	}
	// A fresh Foundry session: Ann reported with nothing yet, Gone left the world.
	if err := repo.Replace(ctx, cid, []FoundryPlayer{{FoundryUserID: "f1", Name: "Ann", Online: true, ReportedAt: now}}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.List(ctx, cid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want only the reported user, got %+v", got)
	}
	p := got[0]
	if p.LastChangeAt == nil || !p.LastChangeAt.Equal(synced) || p.LastFailedAt == nil || !p.LastFailedAt.Equal(failed) ||
		p.LastFailure != "HTTP 403 Forbidden" || p.FailedCount != 2 || !p.Online || !p.ReportedAt.Equal(now) {
		t.Fatalf("earlier activity not kept: %+v", p)
	}
}
