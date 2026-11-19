package updatehealth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMatch(t *testing.T) {
	r := NewRecorder([]Feature{
		{ID: "a", Routes: []string{"/campaigns/:id/crafting*"}},
		{ID: "b", Routes: []string{"POST /campaigns/:id/shop/buy", "post /campaigns/:id/shop/sell"}},
		{ID: "c", Routes: []string{"/admin/x", "/admin/x*"}},
	})
	tests := []struct {
		method, path string
		want         []int
	}{
		{"GET", "/campaigns/:id/crafting", []int{0}},
		{"POST", "/campaigns/:id/crafting/recipes", []int{0}},
		{"GET", "/campaigns/:id/craft", nil},
		{"POST", "/campaigns/:id/shop/buy", []int{1}},
		{"GET", "/campaigns/:id/shop/buy", nil},
		{"POST", "/campaigns/:id/shop/sell", []int{1}},
		{"GET", "/admin/x", []int{2}}, // two rules, counted once
		{"GET", "", nil},
	}
	for _, tt := range tests {
		got := r.Match(tt.method, tt.path)
		if len(got) != len(tt.want) {
			t.Errorf("%s %s = %v, want %v", tt.method, tt.path, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("%s %s = %v, want %v", tt.method, tt.path, got, tt.want)
			}
		}
	}
}

func TestNilRecorderIgnoresEverything(t *testing.T) {
	var r *Recorder
	r.RecordUse("GET", "/x")
	r.RecordError("GET", "/x", "boom", time.Now())
	SetDefault(nil)
	Use("GET", "/x")
	Fail("GET", "/x", "boom")
}

func TestClassify(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	tests := []struct {
		name string
		c    Counts
		want State
	}{
		{"new", Counts{FirstSeen: now.Add(-day)}, Counting},
		{"nine uses", Counts{FirstSeen: now.Add(-day), Uses: 9}, Counting},
		{"ten uses passes", Counts{FirstSeen: now.Add(-day), Uses: 10}, Passed},
		{"an error outranks uses", Counts{FirstSeen: now.Add(-day), Uses: 50, Errors: 1}, Failing},
		{"unused after 14 days", Counts{FirstSeen: now.Add(-14 * day)}, Unused},
		{"few uses after 14 days", Counts{FirstSeen: now.Add(-20 * day), Uses: 3}, Unused},
		{"enough uses after 14 days", Counts{FirstSeen: now.Add(-20 * day), Uses: 10}, Passed},
	}
	for _, tt := range tests {
		if got := Classify(tt.c, now); got != tt.want {
			t.Errorf("%s: got %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestBuildReport(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	fs := []Feature{{ID: "passed"}, {ID: "old-pass"}, {ID: "fails"}, {ID: "unused"}, {ID: "new"}, {ID: "pending-error"}}
	stored := map[string]Counts{
		"passed":        {FirstSeen: now.Add(-3 * day), Uses: 12},
		"old-pass":      {FirstSeen: now.Add(-40 * day), Uses: 99},
		"fails":         {FirstSeen: now.Add(-40 * day), Uses: 99, Errors: 2},
		"unused":        {FirstSeen: now.Add(-15 * day)},
		"pending-error": {FirstSeen: now.Add(-day), Uses: 30},
	}
	pending := map[string]delta{"pending-error": {errors: 1, lastError: now, lastErrorMsg: "GET /x: boom"}}
	got := buildReport(fs, stored, pending, now)
	want := []struct {
		id    string
		state State
	}{{"fails", Failing}, {"pending-error", Failing}, {"unused", Unused}, {"new", Counting}, {"passed", Passed}}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].ID != w.id || got[i].State != w.state {
			t.Errorf("row %d = %s/%s, want %s/%s", i, got[i].ID, got[i].State, w.id, w.state)
		}
	}
	if got[1].LastErrorMsg != "GET /x: boom" || got[1].Errors != 1 {
		t.Errorf("pending error not folded in: %+v", got[1])
	}
}

// memStore is a Store in memory; failAdd makes every Add fail.
type memStore struct {
	rows    map[string]Counts
	failAdd bool
}

func (m *memStore) EnsureSeen(_ context.Context, ids []string, now time.Time) error {
	for _, id := range ids {
		if _, ok := m.rows[id]; !ok {
			m.rows[id] = Counts{FirstSeen: now}
		}
	}
	return nil
}

func (m *memStore) Add(_ context.Context, id string, uses, errs int, at *time.Time, msg string) error {
	if m.failAdd {
		return errors.New("db down")
	}
	c := m.rows[id]
	c.Uses += uses
	c.Errors += errs
	if at != nil {
		c.LastError, c.LastErrorMsg = at, msg
	}
	m.rows[id] = c
	return nil
}

func (m *memStore) All(context.Context) (map[string]Counts, error) { return m.rows, nil }

func TestFlushKeepsCountsWhenTheStoreFails(t *testing.T) {
	rec := NewRecorder([]Feature{{ID: "a", Routes: []string{"/a"}}})
	st := &memStore{rows: map[string]Counts{}, failAdd: true}
	svc := NewService(rec, st)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec.RecordUse("GET", "/a")
	rec.RecordUse("GET", "/a")
	if err := svc.Flush(context.Background()); err == nil {
		t.Fatal("want the store's error")
	}
	rec.RecordUse("GET", "/a")
	st.failAdd = false
	if err := svc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := st.rows["a"].Uses; got != 3 {
		t.Fatalf("uses = %d, want 3", got)
	}
}

func TestChangedEntriesAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range Changed {
		if f.ID == "" || len(f.ID) > 64 || f.Name == "" || len(f.Routes) == 0 || f.Try == "" || f.Try[0] != '/' {
			t.Errorf("incomplete entry: %+v", f)
		}
		if seen[f.ID] {
			t.Errorf("duplicate id %q", f.ID)
		}
		seen[f.ID] = true
	}
}
