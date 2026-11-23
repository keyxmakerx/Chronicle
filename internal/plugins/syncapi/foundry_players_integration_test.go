package syncapi

// Replace swaps a campaign's whole player list, but must keep what an
// earlier report knew about a user that the new one doesn't (each Foundry
// session starts its counts empty). Skips when no server is reachable.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/syncapi/ -run TestFoundryPlayersIntegration

import (
	"context"
	"fmt"
	"strings"
	"sync"
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

// Several campaigns replacing their lists at once deadlock on shared index
// gaps; Replace retries, so every report lands and none errors.
func TestFoundryPlayersIntegration_ConcurrentCampaignsDoNotDeadlock(t *testing.T) {
	db := newSyncStatsScratchDB(t)
	ctx := context.Background()
	const campaigns, rounds, perList = 8, 6, 4
	uid := "u-fp-0000000000000000000000000000002"
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, uid, "fp2@example.test", "Ren", "x"); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, campaigns)
	for i := range ids {
		ids[i] = fmt.Sprintf("c-fp-%030d", i+10)
		if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, ids[i], "Busy", fmt.Sprintf("busy-%d", i), uid); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewFoundryPlayerRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup
		errs := make(chan error, campaigns)
		for _, cid := range ids {
			wg.Add(1)
			go func(cid string) {
				defer wg.Done()
				list := make([]FoundryPlayer, perList)
				for j := range list {
					list[j] = FoundryPlayer{FoundryUserID: fmt.Sprintf("f%d", j), Name: fmt.Sprintf("P%d-%d", round, j), ReportedAt: now}
				}
				errs <- repo.Replace(ctx, cid, list)
			}(cid)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}
	}
	for _, cid := range ids {
		got, err := repo.List(ctx, cid)
		if err != nil || len(got) != perList {
			t.Fatalf("campaign %s: %d rows, %v", cid, len(got), err)
		}
		for _, p := range got {
			if !strings.HasPrefix(p.Name, fmt.Sprintf("P%d-", rounds-1)) {
				t.Fatalf("campaign %s kept an old row: %+v", cid, p)
			}
		}
	}
}
