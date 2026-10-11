package admin

import (
	"context"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestPurgeDue_UnreadableRetentionSkipsTheRun pins that a failed read of the
// setting never falls back to a default: a configured 90 days with a transient
// read error must not purge a 45-day-old item.
func TestPurgeDue_UnreadableRetentionSkipsTheRun(t *testing.T) {
	camps := &fakeTrashCampaigns{trashed: []campaigns.TrashedCampaign{{ID: "c45", DeletedAt: daysAgo(45)}}}
	files := &fakeTrashFiles{batches: map[string][]string{"b45": {"f"}}}
	repo := newFakeBatchRepo(TrashBatch{ID: "b45", Items: 1, CreatedAt: daysAgo(45)})
	s := NewTrashService(camps, files, fakeFinder{}, repo, failingRetention{}).(*trashService)
	s.now = func() time.Time { return trashNow }

	if _, err := s.PurgeDue(context.Background()); err == nil {
		t.Fatal("PurgeDue with an unreadable setting returned no error")
	}
	if len(camps.purged) != 0 || len(camps.dueArgs) != 0 || repo.rows["b45"] == nil || len(files.purged) != 0 {
		t.Errorf("something was purged or listed: campaigns %v, batch kept %v, files %v", camps.purged, repo.rows["b45"] != nil, files.purged)
	}
	if _, err := s.Overview(context.Background()); err == nil {
		t.Error("Overview with an unreadable setting returned no error")
	}
	// Empty now ignores age, so it does not depend on the setting.
	if res, err := s.EmptyNow(context.Background()); err != nil || res.Campaigns != 1 || res.Batches != 1 {
		t.Errorf("EmptyNow = %+v, %v; want it to run", res, err)
	}
}

// TestPurgeDue_ClaimRechecksAge pins that the run's cutoff reaches the claim:
// a campaign listed as due, then undone and trashed again, is not purged.
func TestPurgeDue_ClaimRechecksAge(t *testing.T) {
	camps := &fakeTrashCampaigns{
		trashed:   []campaigns.TrashedCampaign{{ID: "again", DeletedAt: daysAgo(40)}, {ID: "stale", DeletedAt: daysAgo(40)}},
		retrashed: map[string]time.Time{"again": daysAgo(0)},
	}
	s := newTestTrash(camps, &fakeTrashFiles{}, fakeFinder{}, newFakeBatchRepo(), 30)

	res, err := s.PurgeDue(context.Background())
	if err != nil || res.Campaigns != 1 || res.Failed != 0 {
		t.Fatalf("PurgeDue = %+v, %v; want only the stale one purged", res, err)
	}
	for _, c := range camps.cutoffs {
		if !c.Equal(daysAgo(30)) {
			t.Errorf("claim got cutoff %v, want the run's %v", c, daysAgo(30))
		}
	}
	if got := camps.purged; len(got) != 1 || got[0] != "stale" {
		t.Errorf("purged = %v, want [stale]", got)
	}

	// Empty now passes no cutoff.
	camps2 := &fakeTrashCampaigns{trashed: []campaigns.TrashedCampaign{{ID: "x", DeletedAt: daysAgo(0)}}}
	s2 := newTestTrash(camps2, &fakeTrashFiles{}, fakeFinder{}, newFakeBatchRepo(), 30)
	if _, err := s2.EmptyNow(context.Background()); err != nil || len(camps2.cutoffs) != 1 || !camps2.cutoffs[0].IsZero() {
		t.Errorf("EmptyNow cutoffs = %v, %v; want one zero cutoff", camps2.cutoffs, err)
	}
}

// TestPurge_BatchStillFillingIsLeftAloneEvenByEmptyNow pins that a clean-up
// whose totals are not written yet and is inside the grace window is skipped,
// while one past it is recounted.
func TestPurge_BatchStillFillingIsLeftAloneEvenByEmptyNow(t *testing.T) {
	files := &fakeTrashFiles{batches: map[string][]string{"fresh": {"a"}, "crashed": {"b"}}}
	repo := newFakeBatchRepo(
		TrashBatch{ID: "fresh", Items: 0, CreatedAt: trashNow.Add(-time.Minute)},
		TrashBatch{ID: "crashed", Items: 0, CreatedAt: trashNow.Add(-time.Hour)},
	)
	s := newTestTrash(&fakeTrashCampaigns{}, files, fakeFinder{}, repo, 30)

	if _, err := s.EmptyNow(context.Background()); err != nil {
		t.Fatalf("EmptyNow: %v", err)
	}
	if repo.rows["fresh"] == nil || len(files.purged) != 0 {
		t.Errorf("a batch still filling was purged: files %v", files.purged)
	}
	if repo.rows["crashed"] == nil || repo.rows["crashed"].Items != 1 {
		t.Errorf("a crashed batch was not recounted: %+v", repo.rows["crashed"])
	}
}
