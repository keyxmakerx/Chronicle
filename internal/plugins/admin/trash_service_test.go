package admin

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

var trashNow = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func daysAgo(n int) time.Time { return trashNow.Add(-time.Duration(n) * 24 * time.Hour) }

// --- Fakes ---

type fakeTrashCampaigns struct {
	trashed  []campaigns.TrashedCampaign
	restored []string
	purged   []string
	// purgeResult / purgeErr decide PurgeTrashed's answer per id.
	purgeResult map[string]bool
	purgeErr    map[string]error
	restoreErr  error
	dueArgs     []bool // the `all` flag of each ListPurgeDue call
	// retrashed gives a campaign a newer deleted_at at claim time, as if it
	// were undone and trashed again after the list was read.
	retrashed map[string]time.Time
	cutoffs   []time.Time // the olderThan of each PurgeTrashed call
}

func (f *fakeTrashCampaigns) ListTrashed(context.Context) ([]campaigns.TrashedCampaign, error) {
	return f.trashed, nil
}
func (f *fakeTrashCampaigns) RestoreFromTrash(_ context.Context, id string) error {
	if f.restoreErr != nil {
		return f.restoreErr
	}
	f.restored = append(f.restored, id)
	return nil
}

// ListPurgeDue applies the same rule as the SQL: past the cutoff, or already
// started, or everything when all.
func (f *fakeTrashCampaigns) ListPurgeDue(_ context.Context, cutoff time.Time, all bool) ([]string, error) {
	f.dueArgs = append(f.dueArgs, all)
	var ids []string
	for _, c := range f.trashed {
		if all || c.Emptying || c.DeletedAt.Before(cutoff) {
			ids = append(ids, c.ID)
		}
	}
	return ids, nil
}
func (f *fakeTrashCampaigns) PurgeTrashed(_ context.Context, id string, olderThan time.Time) (bool, error) {
	f.cutoffs = append(f.cutoffs, olderThan)
	if err := f.purgeErr[id]; err != nil {
		return false, err
	}
	// The claim re-checks age itself, like the SQL: an unclaimed campaign
	// younger than the cutoff is left alone.
	if !olderThan.IsZero() {
		for _, c := range f.trashed {
			if c.ID != id || c.Emptying {
				continue
			}
			at := c.DeletedAt
			if v, ok := f.retrashed[id]; ok {
				at = v
			}
			if !at.Before(olderThan) {
				return false, nil
			}
		}
	}
	f.purged = append(f.purged, id)
	if v, ok := f.purgeResult[id]; ok {
		return v, nil
	}
	return true, nil
}

type fakeTrashFiles struct {
	// batches maps batch id to the file ids marked into it.
	batches map[string][]string
	sizes   map[string]int64
	purged  []string
	// failPurge makes PurgeTrashedFile fail for these file ids.
	failPurge map[string]bool
}

func (f *fakeTrashFiles) TrashFiles(_ context.Context, batchID string, ids []string) (int, int64, error) {
	if f.batches == nil {
		f.batches = map[string][]string{}
	}
	f.batches[batchID] = append(f.batches[batchID], ids...)
	var size int64
	for _, id := range f.batches[batchID] {
		size += f.sizes[id]
	}
	return len(f.batches[batchID]), size, nil
}
func (f *fakeTrashFiles) RestoreTrashedFiles(_ context.Context, batchID string) (int, error) {
	n := len(f.batches[batchID])
	delete(f.batches, batchID)
	return n, nil
}
func (f *fakeTrashFiles) ListTrashedFileIDs(_ context.Context, batchID string) ([]string, error) {
	return append([]string(nil), f.batches[batchID]...), nil
}
func (f *fakeTrashFiles) PurgeTrashedFile(_ context.Context, batchID, id string) error {
	if f.failPurge[id] {
		return errors.New("disk")
	}
	f.purged = append(f.purged, id)
	return nil
}

type fakeFinder struct{ items []OrphanedMediaItem }

func (f fakeFinder) ScanOrphanedMedia(context.Context) ([]OrphanedMediaItem, error) {
	return f.items, nil
}

type fakeBatchRepo struct{ rows map[string]*TrashBatch }

func newFakeBatchRepo(bs ...TrashBatch) *fakeBatchRepo {
	r := &fakeBatchRepo{rows: map[string]*TrashBatch{}}
	for _, b := range bs {
		b := b
		if b.State == "" {
			b.State = batchTrashed
		}
		r.rows[b.ID] = &b
	}
	return r
}
func (r *fakeBatchRepo) Create(_ context.Context, b *TrashBatch) error {
	c := *b
	c.State = batchTrashed
	r.rows[b.ID] = &c
	return nil
}
func (r *fakeBatchRepo) SetTotals(_ context.Context, id string, items int, bytes int64) error {
	r.rows[id].Items, r.rows[id].Bytes = items, bytes
	return nil
}
func (r *fakeBatchRepo) Get(_ context.Context, id string) (*TrashBatch, error) {
	if b, ok := r.rows[id]; ok {
		c := *b
		return &c, nil
	}
	return nil, nil
}
func (r *fakeBatchRepo) List(context.Context) ([]TrashBatch, error) {
	var out []TrashBatch
	for _, b := range r.rows {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r *fakeBatchRepo) Claim(_ context.Context, id, from, to string, olderThan time.Time) (bool, error) {
	if b, ok := r.rows[id]; ok && b.State == from && (olderThan.IsZero() || b.CreatedAt.Before(olderThan)) {
		b.State = to
		return true, nil
	}
	return false, nil
}
func (r *fakeBatchRepo) Delete(_ context.Context, id string) error {
	delete(r.rows, id)
	return nil
}

type fixedRetention int

func (f fixedRetention) SiteTrashRetentionDays(context.Context) (int, error) { return int(f), nil }

// failingRetention is a setting that cannot be read right now.
type failingRetention struct{}

func (failingRetention) SiteTrashRetentionDays(context.Context) (int, error) {
	return 0, errors.New("settings read failed")
}

func newTestTrash(c *fakeTrashCampaigns, f *fakeTrashFiles, finder UnusedUploadFinder, b *fakeBatchRepo, days int) *trashService {
	s := NewTrashService(c, f, finder, b, fixedRetention(days)).(*trashService)
	s.now = func() time.Time { return trashNow }
	return s
}

// --- Wording ---

func TestAgoLabel(t *testing.T) {
	tests := []struct {
		name string
		age  time.Duration
		want string
	}{
		{"seconds", 20 * time.Second, "just now"},
		{"one minute", time.Minute, "1 minute ago"},
		{"minutes", 59 * time.Minute, "59 minutes ago"},
		{"one hour", time.Hour, "1 hour ago"},
		{"hours", 23 * time.Hour, "23 hours ago"},
		{"yesterday", 30 * time.Hour, "yesterday"},
		{"three days", 3*24*time.Hour + time.Hour, "3 days ago"},
		{"future clock skew", -time.Minute, "just now"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := agoLabel(trashNow, trashNow.Add(-tc.age)); got != tc.want {
				t.Errorf("agoLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEmptiesLabel(t *testing.T) {
	tests := []struct {
		name      string
		trashed   time.Time
		retention int
		want      string
	}{
		{"three days in, thirty kept", daysAgo(3), 30, "empties in 27 days"},
		{"yesterday, thirty kept", daysAgo(1), 30, "empties in 29 days"},
		{"just now", trashNow, 30, "empties in 30 days"},
		{"one day left rounds to one day", daysAgo(29), 30, "empties in 1 day"},
		{"a day and a half left is two days", trashNow.Add(-(28*24 + 12) * time.Hour), 30, "empties in 2 days"},
		{"under a day left", trashNow.Add(-(29*24 + 6) * time.Hour), 30, "empties today"},
		{"overdue", daysAgo(40), 30, "empties soon"},
		{"shortened retention makes old items due", daysAgo(10), 7, "empties soon"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := emptiesLabel(trashNow, tc.trashed, tc.retention); got != tc.want {
				t.Errorf("emptiesLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBatchSummary(t *testing.T) {
	tests := []struct {
		name string
		b    TrashBatch
		want string
	}{
		{"pictures", TrashBatch{Label: "pictures", Items: 42, Bytes: 180 << 20}, "42 unused pictures, 180.0 MB"},
		{"one picture", TrashBatch{Label: "pictures", Items: 1, Bytes: 2048}, "1 unused picture, 2.0 KB"},
		{"mixed files", TrashBatch{Label: "files", Items: 3, Bytes: 100}, "3 unused files, 100 B"},
		{"no label falls back to files", TrashBatch{Items: 2, Bytes: 0}, "2 unused files, 0 B"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := batchSummary(tc.b); got != tc.want {
				t.Errorf("batchSummary = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- Overview ---

func TestOverview(t *testing.T) {
	by := "u-1"
	camps := &fakeTrashCampaigns{trashed: []campaigns.TrashedCampaign{
		{ID: "c1", Name: "Shattered Coast", DeletedAt: daysAgo(3), DeletedBy: &by, DeletedByName: "alex"},
		{ID: "c2", Name: "Old Name", DeletedAt: daysAgo(2), DeletedBy: &by},
		{ID: "c3", Name: "Sweep", DeletedAt: daysAgo(5)},
		{ID: "c4", Name: "Going", DeletedAt: daysAgo(6), DeletedBy: &by, DeletedByName: "mara", Emptying: true},
	}}
	batches := newFakeBatchRepo(
		TrashBatch{ID: "b1", Label: "pictures", Items: 42, Bytes: 180 << 20, CreatedAt: daysAgo(1)},
		TrashBatch{ID: "b2", Label: "files", Items: 5, CreatedAt: daysAgo(7), State: batchRestoring},
	)
	s := newTestTrash(camps, &fakeTrashFiles{}, fakeFinder{}, batches, 30)

	ov, err := s.Overview(context.Background())
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if ov.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d, want 30", ov.RetentionDays)
	}

	type row struct {
		Kind     TrashKind
		Title    string
		Who      string
		When     string
		Empties  string
		Emptying bool
	}
	var got []row
	for _, e := range ov.Entries {
		got = append(got, row{e.Kind, e.Title, e.Who, e.When, e.Empties, e.Emptying})
	}
	// Newest first; the batch being restored is already back so it is hidden.
	want := []row{
		{TrashFiles, "Files clean-up: 42 unused pictures, 180.0 MB", "", "yesterday", "empties in 29 days", false},
		{TrashCampaign, "Campaign: Old Name", "deleted by a former member", "2 days ago", "empties in 28 days", false},
		{TrashCampaign, "Campaign: Shattered Coast", "deleted by alex", "3 days ago", "empties in 27 days", false},
		{TrashCampaign, "Campaign: Sweep", "deleted", "5 days ago", "empties in 25 days", false},
		{TrashCampaign, "Campaign: Going", "deleted by mara", "6 days ago", "empties in 24 days", true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows =\n%#v\nwant\n%#v", got, want)
	}
}

func TestRetentionDays(t *testing.T) {
	tests := []struct {
		name      string
		retention TrashRetention
		want      int
		wantErr   bool
	}{
		{"nil setting means the default", nil, 30, false},
		{"offered choice", fixedRetention(7), 7, false},
		{"not an offered choice is an error, not a guess", fixedRetention(3), 0, true},
		{"zero is an error", fixedRetention(0), 0, true},
		{"a read failure is an error", failingRetention{}, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewTrashService(&fakeTrashCampaigns{}, &fakeTrashFiles{}, fakeFinder{}, newFakeBatchRepo(), tc.retention)
			got, err := s.RetentionDays(context.Background())
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("RetentionDays = %d, %v; want %d, err %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

// --- Trashing unused uploads ---

func TestTrashUnusedUploads(t *testing.T) {
	pic := func(id string, ref bool) OrphanedMediaItem {
		return OrphanedMediaItem{ID: id, MimeType: "image/png", Referenced: ref}
	}
	tests := []struct {
		name      string
		items     []OrphanedMediaItem
		wantNil   bool
		wantLabel string
		wantIDs   []string
	}{
		{"nothing to move", nil, true, "", nil},
		{"all still referenced", []OrphanedMediaItem{pic("a", true)}, true, "", nil},
		{"only unreferenced move", []OrphanedMediaItem{pic("a", false), pic("b", true), pic("c", false)}, false, "pictures", []string{"a", "c"}},
		{"a non-picture makes them files", []OrphanedMediaItem{pic("a", false), {ID: "d", MimeType: "application/pdf"}}, false, "files", []string{"a", "d"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			files := &fakeTrashFiles{sizes: map[string]int64{"a": 10, "c": 20, "d": 30}}
			repo := newFakeBatchRepo()
			s := newTestTrash(&fakeTrashCampaigns{}, files, fakeFinder{items: tc.items}, repo, 30)

			b, err := s.TrashUnusedUploads(context.Background(), TrashActor{UserID: "u-1", Name: "alex"})
			if err != nil {
				t.Fatalf("TrashUnusedUploads: %v", err)
			}
			if tc.wantNil {
				if b != nil || len(repo.rows) != 0 || len(files.batches) != 0 {
					t.Fatalf("expected no batch, got %v rows=%d", b, len(repo.rows))
				}
				return
			}
			if b.Label != tc.wantLabel || b.Items != len(tc.wantIDs) {
				t.Errorf("batch label=%q items=%d, want %q %d", b.Label, b.Items, tc.wantLabel, len(tc.wantIDs))
			}
			if got := files.batches[b.ID]; !reflect.DeepEqual(got, tc.wantIDs) {
				t.Errorf("marked files = %v, want %v", got, tc.wantIDs)
			}
			stored := repo.rows[b.ID]
			if stored == nil || stored.Items != len(tc.wantIDs) || stored.DeletedByName != "alex" || stored.State != batchTrashed {
				t.Errorf("stored batch = %+v", stored)
			}
			if len(files.purged) != 0 {
				t.Errorf("trashing deleted files: %v", files.purged)
			}
		})
	}
}

// --- Undo ---

func TestUndoBatch(t *testing.T) {
	tests := []struct {
		name         string
		state        string // "" means the batch does not exist
		wantCode     int
		wantRestored bool
	}{
		{"waiting batch comes back", batchTrashed, 0, true},
		{"a half-finished undo is completed", batchRestoring, 0, true},
		{"already being emptied is refused", batchPurging, http.StatusNotFound, false},
		{"missing batch", "", http.StatusNotFound, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeBatchRepo()
			files := &fakeTrashFiles{batches: map[string][]string{"b1": {"x", "y"}}}
			if tc.state != "" {
				repo.rows["b1"] = &TrashBatch{ID: "b1", Items: 2, State: tc.state}
			}
			s := newTestTrash(&fakeTrashCampaigns{}, files, fakeFinder{}, repo, 30)

			b, err := s.UndoBatch(context.Background(), "b1")
			if tc.wantCode != 0 {
				var appErr *apperror.AppError
				if !errors.As(err, &appErr) || appErr.Code != tc.wantCode {
					t.Fatalf("err = %v, want code %d", err, tc.wantCode)
				}
			} else if err != nil || b == nil {
				t.Fatalf("UndoBatch: %v %v", b, err)
			}
			_, stillMarked := files.batches["b1"]
			if tc.wantRestored && (stillMarked || repo.rows["b1"] != nil) {
				t.Errorf("files still marked=%v or batch row left=%v", stillMarked, repo.rows["b1"])
			}
			if !tc.wantRestored && tc.state != "" && (!stillMarked || repo.rows["b1"] == nil) {
				t.Errorf("a refused undo changed the batch: marked=%v row=%v", stillMarked, repo.rows["b1"])
			}
		})
	}
}

func TestUndoCampaign(t *testing.T) {
	camps := &fakeTrashCampaigns{trashed: []campaigns.TrashedCampaign{{ID: "c1", Name: "Shattered Coast"}}}
	s := newTestTrash(camps, &fakeTrashFiles{}, fakeFinder{}, newFakeBatchRepo(), 30)
	name, err := s.UndoCampaign(context.Background(), "c1")
	if err != nil || name != "Shattered Coast" || !reflect.DeepEqual(camps.restored, []string{"c1"}) {
		t.Fatalf("name=%q err=%v restored=%v", name, err, camps.restored)
	}

	camps.restoreErr = apperror.NewNotFound("gone")
	if _, err := s.UndoCampaign(context.Background(), "c1"); !isNotFound(err) {
		t.Errorf("a refused restore must surface as not found, got %v", err)
	}
}

// --- Purging ---

func TestPurgeDue_OnlyAfterRetention(t *testing.T) {
	tests := []struct {
		name         string
		retention    int
		wantCampaign []string
		wantBatches  []string
	}{
		{"thirty days", 30, []string{"old"}, []string{"b-old"}},
		{"shortened to seven takes the middle ones too", 7, []string{"mid", "old"}, []string{"b-mid", "b-old"}},
		{"ninety days keeps all", 90, nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			camps := &fakeTrashCampaigns{trashed: []campaigns.TrashedCampaign{
				{ID: "new", DeletedAt: daysAgo(1)},
				{ID: "mid", DeletedAt: daysAgo(10)},
				{ID: "old", DeletedAt: daysAgo(31)},
			}}
			files := &fakeTrashFiles{batches: map[string][]string{"b-new": {"n"}, "b-mid": {"m"}, "b-old": {"o"}}}
			repo := newFakeBatchRepo(
				TrashBatch{ID: "b-new", Items: 1, CreatedAt: daysAgo(1)},
				TrashBatch{ID: "b-mid", Items: 1, CreatedAt: daysAgo(10)},
				TrashBatch{ID: "b-old", Items: 1, CreatedAt: daysAgo(31)},
			)
			s := newTestTrash(camps, files, fakeFinder{}, repo, tc.retention)

			res, err := s.PurgeDue(context.Background())
			if err != nil {
				t.Fatalf("PurgeDue: %v", err)
			}
			sort.Strings(camps.purged)
			if !reflect.DeepEqual(camps.purged, tc.wantCampaign) {
				t.Errorf("campaigns purged = %v, want %v", camps.purged, tc.wantCampaign)
			}
			var gone []string
			for _, id := range []string{"b-mid", "b-new", "b-old"} {
				if repo.rows[id] == nil {
					gone = append(gone, id)
				}
			}
			if !reflect.DeepEqual(gone, tc.wantBatches) {
				t.Errorf("batches removed = %v, want %v", gone, tc.wantBatches)
			}
			if res.Campaigns != len(tc.wantCampaign) || res.Batches != len(tc.wantBatches) || res.Failed != 0 {
				t.Errorf("result = %+v", res)
			}
			if repo.rows["b-new"] == nil || len(files.batches["b-new"]) != 1 {
				t.Errorf("a young batch was touched")
			}
		})
	}
}

func TestEmptyNow_RemovesEverythingWhateverItsAge(t *testing.T) {
	camps := &fakeTrashCampaigns{trashed: []campaigns.TrashedCampaign{{ID: "c1", DeletedAt: daysAgo(0)}, {ID: "c2", DeletedAt: daysAgo(2)}}}
	files := &fakeTrashFiles{batches: map[string][]string{"b1": {"f1", "f2"}}}
	repo := newFakeBatchRepo(TrashBatch{ID: "b1", Items: 2, CreatedAt: trashNow})
	s := newTestTrash(camps, files, fakeFinder{}, repo, 30)

	res, err := s.EmptyNow(context.Background())
	if err != nil {
		t.Fatalf("EmptyNow: %v", err)
	}
	if res.Campaigns != 2 || res.Batches != 1 || len(repo.rows) != 0 {
		t.Errorf("result = %+v, batches left %d", res, len(repo.rows))
	}
	if !reflect.DeepEqual(files.purged, []string{"f1", "f2"}) {
		t.Errorf("files purged = %v", files.purged)
	}
	if !reflect.DeepEqual(camps.dueArgs, []bool{true}) {
		t.Errorf("ListPurgeDue all flags = %v, want [true]", camps.dueArgs)
	}
}

// TestPurge_SkipsWhatUndoClaimed pins the central safety rule: a batch that an
// Undo has claimed is never purged, and one that a purge has claimed is
// resumed rather than restored.
func TestPurge_SkipsWhatUndoClaimed(t *testing.T) {
	files := &fakeTrashFiles{batches: map[string][]string{"restoring": {"r"}, "purging": {"p"}}}
	repo := newFakeBatchRepo(
		TrashBatch{ID: "restoring", Items: 1, CreatedAt: daysAgo(90), State: batchRestoring},
		TrashBatch{ID: "purging", Items: 1, CreatedAt: daysAgo(1), State: batchPurging},
	)
	s := newTestTrash(&fakeTrashCampaigns{}, files, fakeFinder{}, repo, 30)

	if _, err := s.PurgeDue(context.Background()); err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}
	if !reflect.DeepEqual(files.purged, []string{"p"}) {
		t.Errorf("purged = %v, want only the batch already being purged", files.purged)
	}
	if repo.rows["restoring"] != nil {
		t.Errorf("a half-finished undo must be completed, row left: %+v", repo.rows["restoring"])
	}
	if _, kept := files.batches["restoring"]; kept {
		t.Errorf("the restoring batch's files are still marked")
	}
}

func TestPurge_FailureIsRetriedNotLost(t *testing.T) {
	camps := &fakeTrashCampaigns{
		trashed:  []campaigns.TrashedCampaign{{ID: "bad", DeletedAt: daysAgo(40)}, {ID: "good", DeletedAt: daysAgo(40)}},
		purgeErr: map[string]error{"bad": errors.New("disk full")},
	}
	files := &fakeTrashFiles{batches: map[string][]string{"b1": {"x", "y"}}, failPurge: map[string]bool{"y": true}}
	repo := newFakeBatchRepo(TrashBatch{ID: "b1", Items: 2, CreatedAt: daysAgo(40)})
	s := newTestTrash(camps, files, fakeFinder{}, repo, 30)

	res, err := s.PurgeDue(context.Background())
	if err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}
	if res.Failed != 2 || res.Campaigns != 1 || res.Batches != 0 {
		t.Errorf("result = %+v, want 2 failed, 1 campaign, 0 batches", res)
	}
	b := repo.rows["b1"]
	if b == nil || b.State != batchPurging {
		t.Fatalf("a batch that could not finish must stay claimed for the next run, got %+v", b)
	}

	// The next run finishes it.
	files.failPurge = nil
	if _, err := s.PurgeDue(context.Background()); err != nil {
		t.Fatalf("second PurgeDue: %v", err)
	}
	if repo.rows["b1"] != nil {
		t.Errorf("the retry left the batch behind")
	}
}

func TestPurge_RecountsABatchWhoseTotalsWereNeverSaved(t *testing.T) {
	files := &fakeTrashFiles{batches: map[string][]string{"crashed": {"a", "b"}}, sizes: map[string]int64{"a": 5, "b": 6}}
	repo := newFakeBatchRepo(
		TrashBatch{ID: "crashed", Items: 0, CreatedAt: trashNow.Add(-time.Hour)},
		TrashBatch{ID: "empty", Items: 0, CreatedAt: trashNow.Add(-time.Hour)},
		TrashBatch{ID: "writing", Items: 0, CreatedAt: trashNow.Add(-time.Minute)},
	)
	s := newTestTrash(&fakeTrashCampaigns{}, files, fakeFinder{}, repo, 30)

	if _, err := s.PurgeDue(context.Background()); err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}
	if c := repo.rows["crashed"]; c == nil || c.Items != 2 || c.Bytes != 11 {
		t.Errorf("crashed batch = %+v, want it kept with its real totals", c)
	}
	if repo.rows["empty"] != nil {
		t.Errorf("a batch with no files should be dropped")
	}
	if repo.rows["writing"] == nil {
		t.Errorf("a batch still being written must be left alone")
	}
	if len(files.purged) != 0 {
		t.Errorf("recounting deleted files: %v", files.purged)
	}
}

func TestTrashViewHelpers(t *testing.T) {
	if got := trashMeta(TrashEntry{Who: "deleted by alex", When: "3 days ago", Empties: "empties in 27 days"}); got != "deleted by alex · 3 days ago · empties in 27 days" {
		t.Errorf("trashMeta = %q", got)
	}
	if got := trashMeta(TrashEntry{When: "yesterday", Empties: "empties in 29 days"}); got != "yesterday · empties in 29 days" {
		t.Errorf("trashMeta without Who = %q", got)
	}
	if got := trashUndoURL(TrashEntry{Kind: TrashCampaign, ID: "abc"}); got != "/admin/trash/campaigns/abc/undo" {
		t.Errorf("campaign undo url = %q", got)
	}
	if got := trashUndoURL(TrashEntry{Kind: TrashFiles, ID: "abc"}); got != "/admin/trash/batches/abc/undo" {
		t.Errorf("batch undo url = %q", got)
	}
	if h := trashHeaders(`ab"c`); !strings.Contains(h, `X-CSRF-Token`) || !strings.Contains(h, `\"`) {
		t.Errorf("trashHeaders must be valid JSON with the token escaped, got %s", h)
	}
}
