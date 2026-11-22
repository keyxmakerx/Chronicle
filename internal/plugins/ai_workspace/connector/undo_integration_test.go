package connector

// Row-level tests for Undo's table: the one-shot claim, scoping to the
// owner's own request, the purge, and the migration's up/down/up.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/ai_workspace/connector/ -run TestUndoIntegration

import (
	"context"
	"fmt"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
)

// undoRig is an applied request of the person's own key with two kept
// befores, a page update that held GM-only text and a page the AI created.
type undoRig struct {
	svc        *Service
	camp, user string
	id         int64
	exec       func(q string, args ...any)
}

func newUndoRig(t *testing.T) undoRig {
	t.Helper()
	db := newScratchDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	camp, user := seedCampaign(t, db)
	svc := NewService(repo, owners{camp: user})
	svc.SetUndo(NewUndoRepository(db))
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := repo.Replace(ctx, &Connection{CampaignID: camp, UserID: user, KeyHash: fmt.Sprintf("%064d", 1), KeyHint: "0001",
		Privacy: aiexport.PrivacyModePermitted, CanAsk: true, CreatedAt: now, ExpiresAt: now.Add(KeyLifetime)}); err != nil {
		t.Fatal(err)
	}
	conn, err := repo.Current(ctx, camp, user)
	if err != nil || conn == nil {
		t.Fatalf("current: %v", err)
	}
	id, err := repo.AddRequest(ctx, &Request{CampaignID: camp, ConnectionID: conn.ID, Summary: "x", Counts: "2 to change", Blocks: 2, Body: "b", CreatedAt: now}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.MoveRequest(ctx, camp, user, id, RequestWaiting, RequestApplying, "", now); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	gm := `{"name":"Lyra","entryHtml":"<p>GM secret</p>"}`
	if err := svc.KeepBefores(ctx, camp, user, id, []Before{
		{Kind: "page", TargetID: "p1", TargetName: "Lyra", Action: "update", BeforeJSON: &gm, AfterHash: "h1"},
		{Kind: "page", TargetID: "p2", TargetName: "Sable Quay", Action: "create", AfterHash: "h2"},
	}); err != nil {
		t.Fatalf("keep: %v", err)
	}
	if err := svc.Answered(ctx, camp, user, id, "2 applied"); err != nil {
		t.Fatal(err)
	}
	return undoRig{svc: svc, camp: camp, user: user, id: id, exec: func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}}
}

func TestUndoIntegration_SummaryAndScope(t *testing.T) {
	r := newUndoRig(t)
	ctx := context.Background()
	sums, err := r.svc.UndoSummaries(ctx, r.camp, r.user, []int64{r.id})
	if err != nil || len(sums[r.id].Left) != 2 || sums[r.id].UndoneAt != nil {
		t.Fatalf("summary = %+v, %v", sums[r.id], err)
	}
	// The summary names what and which kind, never the kept text.
	if sums[r.id].Left[0].BeforeJSON != nil {
		t.Error("a summary carried before text")
	}
	// Another person, or another campaign, sees nothing of it.
	if s, _ := r.svc.UndoSummaries(ctx, r.camp, "someone-else", []int64{r.id}); len(s) != 0 {
		t.Errorf("another person read it: %+v", s)
	}
	if _, _, err := r.svc.ClaimUndo(ctx, r.camp, "someone-else", r.id); err == nil {
		t.Error("another person claimed the undo")
	}
	if _, _, err := r.svc.ClaimUndo(ctx, "other-camp", r.user, r.id); err == nil {
		t.Error("another campaign claimed the undo")
	}
}

func TestUndoIntegration_ClaimRunsOnce(t *testing.T) {
	r := newUndoRig(t)
	ctx := context.Background()
	var won, lost atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, left, err := r.svc.ClaimUndo(ctx, r.camp, r.user, r.id); err == nil && len(left) == 2 {
				won.Add(1)
			} else {
				lost.Add(1)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 || lost.Load() != 7 {
		t.Fatalf("won %d, lost %d; want 1 and 7", won.Load(), lost.Load())
	}
	// A given-up claim can be taken again, and the before text comes back
	// only through the claim.
	r.svc.ReleaseUndo(ctx, r.camp, r.user, r.id)
	_, left, err := r.svc.ClaimUndo(ctx, r.camp, r.user, r.id)
	if err != nil || len(left) != 2 || left[len(left)-1].BeforeJSON == nil {
		t.Fatalf("second claim: %+v, %v", left, err)
	}
	// One put back, one not: the next claim holds only the rest.
	if err := r.svc.BeforeUndone(ctx, left[0].ID, r.user); err != nil {
		t.Fatal(err)
	}
	r.svc.ReleaseUndo(ctx, r.camp, r.user, r.id)
	_, rest, err := r.svc.ClaimUndo(ctx, r.camp, r.user, r.id)
	if err != nil || len(rest) != 1 {
		t.Fatalf("third claim: %+v, %v", rest, err)
	}
	// Done: the summary says when, and nothing is left to claim.
	_ = r.svc.BeforeUndone(ctx, rest[0].ID, r.user)
	sums, _ := r.svc.UndoSummaries(ctx, r.camp, r.user, []int64{r.id})
	if sums[r.id].UndoneAt == nil {
		t.Error("an undone request shows no time")
	}
}

func TestUndoIntegration_OldRequestsAndPurge(t *testing.T) {
	r := newUndoRig(t)
	ctx := context.Background()
	// A request applied before any before was kept offers nothing.
	repo := r.svc.repo
	conn, _ := repo.Current(ctx, r.camp, r.user)
	now := time.Now().UTC()
	old, err := repo.AddRequest(ctx, &Request{CampaignID: r.camp, ConnectionID: conn.ID, Summary: "old", Blocks: 1, Body: "b2", CreatedAt: now}, 20)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = repo.MoveRequest(ctx, r.camp, r.user, old, RequestWaiting, RequestAnswered, "1 applied", now)
	sums, _ := r.svc.UndoSummaries(ctx, r.camp, r.user, []int64{old})
	if len(sums[old].Left) != 0 || sums[old].UndoneAt != nil {
		t.Errorf("old request summary = %+v", sums[old])
	}
	if _, _, err := r.svc.ClaimUndo(ctx, r.camp, r.user, old); err == nil {
		t.Error("an old request was undone")
	}
	// Past 30 days the before is purged and the request offers nothing.
	r.exec(`UPDATE ai_request_befores SET keep_until = ? WHERE request_id = ?`, now.Add(-time.Hour), r.id)
	if s, _ := r.svc.UndoSummaries(ctx, r.camp, r.user, []int64{r.id}); len(s[r.id].Left) != 0 {
		t.Error("an expired before still shows")
	}
	n, err := r.svc.undoStore.Purge(ctx, now)
	if err != nil || n != 2 {
		t.Errorf("purged %d, %v; want 2", n, err)
	}
	// Befores keep for 30 days from when they were made.
	if BeforesKept != 30*24*time.Hour {
		t.Errorf("BeforesKept = %v", BeforesKept)
	}
}

func TestUndoIntegration_BeforesGoWithTheirRequest(t *testing.T) {
	r := newUndoRig(t)
	r.exec(`DELETE FROM ai_connection_requests WHERE id = ?`, r.id)
	sums, _ := r.svc.UndoSummaries(context.Background(), r.camp, r.user, []int64{r.id})
	if len(sums) != 0 {
		t.Errorf("summary of a deleted request = %+v", sums)
	}
}

// 007 is idempotent (the scratch schema ran it twice already), comes down
// cleanly and goes up again.
func TestUndoIntegration_MigrationUpDownUp(t *testing.T) {
	db := newScratchDB(t)
	read := func(name string) string {
		b, err := fs.ReadFile(MigrationsFS, "migrations/"+name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	has := func() (table, column bool) {
		_ = db.QueryRow(`SELECT COUNT(*) > 0 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'ai_request_befores'`).Scan(&table)
		_ = db.QueryRow(`SELECT COUNT(*) > 0 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'ai_connection_requests' AND column_name = 'undone_at'`).Scan(&column)
		return
	}
	if tb, col := has(); !tb || !col {
		t.Fatalf("after up: table %v column %v", tb, col)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(read("007_ai_request_undo.down.sql")); err != nil {
			t.Fatalf("down %d: %v", i+1, err)
		}
	}
	if tb, col := has(); tb || col {
		t.Fatalf("after down: table %v column %v", tb, col)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(read("007_ai_request_undo.up.sql")); err != nil {
			t.Fatalf("up %d: %v", i+1, err)
		}
	}
	if tb, col := has(); !tb || !col {
		t.Fatalf("after up again: table %v column %v", tb, col)
	}
}

// A claim whose undo stopped part way (a restart) is not "Undone", and once it
// is stale it can be claimed again; a fresh one cannot, and a finished one
// never can.
func TestUndoIntegration_StaleClaimCanBeTakenAgain(t *testing.T) {
	r := newUndoRig(t)
	ctx := context.Background()
	_, left, err := r.svc.ClaimUndo(ctx, r.camp, r.user, r.id)
	if err != nil || len(left) != 2 {
		t.Fatalf("claim: %+v %v", left, err)
	}
	// One change is put back, then the server stops: no release.
	if err := r.svc.BeforeUndone(ctx, left[0].ID, r.user); err != nil {
		t.Fatal(err)
	}
	sums, _ := r.svc.UndoSummaries(ctx, r.camp, r.user, []int64{r.id})
	if sums[r.id].UndoneAt != nil || len(sums[r.id].Left) != 1 {
		t.Fatalf("a part-done undo reads as %+v", sums[r.id])
	}
	if _, _, err := r.svc.ClaimUndo(ctx, r.camp, r.user, r.id); err == nil {
		t.Fatal("a fresh claim was taken over")
	}
	r.exec(`UPDATE ai_connection_requests SET undone_at = ? WHERE id = ?`, time.Now().UTC().Add(-undoStale-time.Minute), r.id)
	_, left, err = r.svc.ClaimUndo(ctx, r.camp, r.user, r.id)
	if err != nil || len(left) != 1 {
		t.Fatalf("stale claim: %+v %v", left, err)
	}
	// Finished: nothing left, so even a stale stamp is not claimable.
	_ = r.svc.BeforeUndone(ctx, left[0].ID, r.user)
	r.exec(`UPDATE ai_connection_requests SET undone_at = ? WHERE id = ?`, time.Now().UTC().Add(-time.Hour), r.id)
	if _, _, err := r.svc.ClaimUndo(ctx, r.camp, r.user, r.id); err == nil {
		t.Error("a finished undo was claimed again")
	}
	sums, _ = r.svc.UndoSummaries(ctx, r.camp, r.user, []int64{r.id})
	if sums[r.id].UndoneAt == nil || len(sums[r.id].Left) != 0 {
		t.Errorf("a finished undo reads as %+v", sums[r.id])
	}
}
