package syncapi

// The history list is hand-written SQL with a filter that must match a
// catch-up run by any of its steps; this runs it against a migrated
// scratch schema. Skips when no server is reachable.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/syncapi/ -run TestSyncHistoryIntegration

import (
	"context"
	"testing"
	"time"
)

func TestSyncHistoryIntegration_ListFiltersAndSteps(t *testing.T) {
	db := newSyncStatsScratchDB(t)
	ctx := context.Background()
	uid, cid, other := "u-hist-00000000000000000000000000001", "c-hist-00000000000000000000000000001", "c-hist-00000000000000000000000000002"
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, []any{uid, "hist@example.test", "Ren", "x"}},
		{`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, []any{cid, "Synced", "synced", uid}},
		{`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, []any{other, "Other", "other", uid}},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewSyncHistoryRepository(db)
	at := time.Now().UTC().Add(-time.Minute)
	u := uid
	insert := func(campaign string, ev SyncEvent) int64 {
		t.Helper()
		ev.OccurredAt, ev.ReportedBy = at, reportedByChronicle
		id, err := repo.Insert(ctx, campaign, &ev)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	edit := insert(cid, SyncEvent{Direction: DirToChronicle, ResourceName: "Harbour", Action: "page updated", Status: "200", OK: true, UserID: &u})
	run := insert(cid, SyncEvent{Direction: DirLink, Action: "catch-up", Status: "ok", OK: true, Children: []SyncEvent{
		{Direction: DirToFoundry, ResourceName: "Lighthouse", Action: "page updated", Status: "ok", OK: true},
		{Direction: DirToFoundry, ResourceName: "50%_off", Action: "page updated", Status: "failed", OK: false, Message: "folder missing"},
	}})
	insert(other, SyncEvent{Direction: DirToChronicle, ResourceName: "Elsewhere", Action: "page updated", Status: "200", OK: true})

	ids := func(evs []SyncEvent) []int64 {
		out := []int64{}
		for _, e := range evs {
			out = append(out, e.ID)
		}
		return out
	}
	cases := []struct {
		name string
		f    SyncHistoryFilter
		want []int64
	}{
		{"newest first, this campaign only", SyncHistoryFilter{}, []int64{run, edit}},
		{"failure inside a run surfaces the run", SyncHistoryFilter{FailedOnly: true}, []int64{run}},
		{"direction matches a step", SyncHistoryFilter{Direction: DirToFoundry}, []int64{run}},
		{"search by person", SyncHistoryFilter{Query: "ren"}, []int64{edit}},
		{"search is literal, not a pattern", SyncHistoryFilter{Query: "50%_"}, []int64{run}},
		{"after cursor", SyncHistoryFilter{After: edit}, []int64{run}},
		{"before cursor", SyncHistoryFilter{Before: run}, []int64{edit}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.List(ctx, cid, tc.f)
			if err != nil {
				t.Fatal(err)
			}
			if g := ids(got); len(g) != len(tc.want) || (len(g) > 0 && g[0] != tc.want[0]) || (len(g) > 1 && g[1] != tc.want[1]) {
				t.Fatalf("got %v, want %v", g, tc.want)
			}
		})
	}

	all, _ := repo.List(ctx, cid, SyncHistoryFilter{})
	if len(all[0].Children) != 2 || all[0].Children[1].Message != "folder missing" || all[1].UserName != "Ren" {
		t.Fatalf("steps or names not loaded: %+v", all)
	}

	// The call flow's reads: one row, a window with steps, the latest
	// (failure), and the same failure lately.
	refused := insert(cid, SyncEvent{Direction: DirToChronicle, Kind: historyKindCalendar, ResourceName: "Ches 8, 1492", Was: "Ches 7, 1492",
		Action: "date set", Call: "PUT /calendar/date", Status: "403", OK: false, Message: "owner role required"})
	got, err := repo.Get(ctx, cid, refused)
	if err != nil || got.Was != "Ches 7, 1492" || got.ResourceName != "Ches 8, 1492" {
		t.Fatalf("get: %+v, %v", got, err)
	}
	if _, err := repo.Get(ctx, other, refused); err == nil {
		t.Fatal("read another campaign's row")
	}
	win, err := repo.Window(ctx, cid, at.Add(-time.Second), at.Add(time.Second), 100)
	if err != nil || len(win) != 5 {
		t.Fatalf("window: %d rows (want the 3 rows and 2 steps), %v", len(win), err)
	}
	last, err := repo.Latest(ctx, cid, at.Add(-time.Hour), true)
	if err != nil || last == nil || last.ID != refused {
		t.Fatalf("latest failure: %+v, %v", last, err)
	}
	if none, err := repo.Latest(ctx, other, at.Add(time.Hour), false); err != nil || none != nil {
		t.Fatalf("latest with nothing: %+v, %v", none, err)
	}
	reps, err := repo.Failures(ctx, cid, "PUT /calendar/date", "403", at.Add(-time.Hour), 10)
	if err != nil || len(reps) != 1 || reps[0].ID != refused {
		t.Fatalf("failures: %+v, %v", reps, err)
	}

	if n, err := repo.PruneOlderThan(ctx, time.Now().UTC().Add(time.Hour)); err != nil || n != 6 {
		t.Fatalf("prune removed %d rows (err %v), want 6", n, err)
	}
}
