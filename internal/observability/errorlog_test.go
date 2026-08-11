package observability

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// paths renders an entry slice as just its paths, so a failed ordering
// assertion prints something a human can read at a glance.
func paths(es []Entry) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Path)
	}
	return out
}

// TestShouldRecordPolicy pins the recording policy itself. This is the test
// that matters most: the policy is the reason a 404 storm cannot evict the one
// 500 an operator came to find, and it is exactly the kind of rule a later
// "let's capture everything" change would loosen without noticing.
func TestShouldRecordPolicy(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		unexpected bool
		want       bool
	}{
		{"200 is not an error at all", 200, false, false},
		{"304 is not an error", 304, false, false},
		{"400 bad request is NOT recorded", 400, false, false},
		{"401 unauthorized is NOT recorded", 401, false, false},
		{"403 forbidden is NOT recorded", 403, false, false},
		{"404 is NOT recorded — this is the eviction guard", 404, false, false},
		{"422 is NOT recorded", 422, false, false},
		{"429 is NOT recorded", 429, false, false},
		{"499 is the last non-recorded status", 499, false, false},
		{"500 IS recorded", 500, false, true},
		{"502 IS recorded", 502, false, true},
		{"503 IS recorded", 503, false, true},
		{"a 4xx flagged explicitly unexpected IS recorded", 418, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldRecord(tt.status, tt.unexpected); got != tt.want {
				t.Errorf("ShouldRecord(%d, %v) = %v, want %v", tt.status, tt.unexpected, got, tt.want)
			}
		})
	}
}

// TestRingEvictsOldestFirst is the core ring contract: at capacity the OLDEST
// entry goes, never the newest, and the snapshot stays newest-first across the
// wrap. A ring that dropped writes instead of evicting would look identical
// until the moment it mattered.
func TestRingEvictsOldestFirst(t *testing.T) {
	r := NewRing(3)
	for i := 1; i <= 5; i++ {
		r.Record(Entry{Status: 500, Path: fmt.Sprintf("/e%d", i)})
	}

	s := r.Snapshot(0)
	if s.Capacity != 3 {
		t.Errorf("Capacity = %d, want 3", s.Capacity)
	}
	if s.Held != 3 {
		t.Errorf("Held = %d, want 3 (the ring is full)", s.Held)
	}
	if s.Total != 5 {
		t.Errorf("Total = %d, want 5 — Total counts evicted entries too, and it is the only signal that history was lost", s.Total)
	}
	// Newest first: e5, e4, e3. e1 and e2 were evicted.
	want := []string{"/e5", "/e4", "/e3"}
	if len(s.Entries) != len(want) {
		t.Fatalf("got %d entries, want %d", len(s.Entries), len(want))
	}
	for i := range want {
		if s.Entries[i].Path != want[i] {
			t.Errorf("entry[%d].Path = %q, want %q (full order: %v)", i, s.Entries[i].Path, want[i], paths(s.Entries))
		}
	}
}

// TestRingPartiallyFilled covers the pre-wrap path separately: before the ring
// wraps, Held must be the write count and the snapshot must not surface the
// zero-valued slots that make up the rest of the backing array.
func TestRingPartiallyFilled(t *testing.T) {
	r := NewRing(10)
	r.Record(Entry{Status: 500, Path: "/a"})
	r.Record(Entry{Status: 503, Path: "/b"})

	s := r.Snapshot(0)
	if s.Held != 2 || s.Total != 2 || s.Capacity != 10 {
		t.Fatalf("Held/Total/Capacity = %d/%d/%d, want 2/2/10", s.Held, s.Total, s.Capacity)
	}
	if len(s.Entries) != 2 {
		t.Fatalf("got %d entries, want 2 — empty slots must not be reported as entries", len(s.Entries))
	}
	if s.Entries[0].Path != "/b" {
		t.Errorf("newest entry = %q, want %q", s.Entries[0].Path, "/b")
	}
}

// TestSnapshotLimit pins that a limit pages the NEWEST entries, not the oldest,
// and that limit <= 0 means everything.
func TestSnapshotLimit(t *testing.T) {
	r := NewRing(10)
	for i := 1; i <= 6; i++ {
		r.Record(Entry{Status: 500, Path: fmt.Sprintf("/e%d", i)})
	}
	s := r.Snapshot(2)
	if len(s.Entries) != 2 {
		t.Fatalf("limit 2 returned %d entries", len(s.Entries))
	}
	if s.Entries[0].Path != "/e6" || s.Entries[1].Path != "/e5" {
		t.Errorf("limit returned %v, want the two NEWEST (/e6, /e5)", paths(s.Entries))
	}
	if s.Held != 6 {
		t.Errorf("Held = %d, want 6 — a limit pages the view, it must not change the reported count", s.Held)
	}
	if n := len(r.Snapshot(0).Entries); n != 6 {
		t.Errorf("Snapshot(0) returned %d entries, want all 6", n)
	}
	if n := len(r.Snapshot(99).Entries); n != 6 {
		t.Errorf("Snapshot(99) returned %d entries, want all 6 (an over-large limit must clamp, not pad)", n)
	}
}

// TestRingEmptySnapshot pins the "wired but nothing has gone wrong" state,
// which the diagnostic must be able to tell apart from "never wired".
func TestRingEmptySnapshot(t *testing.T) {
	s := NewRing(8).Snapshot(0)
	if s.Capacity != 8 || s.Held != 0 || s.Total != 0 || len(s.Entries) != 0 {
		t.Errorf("empty ring snapshot = %+v, want capacity 8 and everything else zero", s)
	}
}

// TestNewRingCoercesBadCapacity: a zero-length ring would accept every write
// and keep none, i.e. it would look wired and record nothing — the exact state
// these diagnostics exist to make impossible.
func TestNewRingCoercesBadCapacity(t *testing.T) {
	for _, c := range []int{0, -1} {
		r := NewRing(c)
		r.Record(Entry{Status: 500, Path: "/x"})
		s := r.Snapshot(0)
		if s.Capacity != DefaultCapacity {
			t.Errorf("NewRing(%d).Capacity = %d, want DefaultCapacity %d", c, s.Capacity, DefaultCapacity)
		}
		if s.Held != 1 {
			t.Errorf("NewRing(%d) held %d entries after one Record, want 1", c, s.Held)
		}
	}
}

// TestNilRingIsSafe: both methods are nil-guarded, mirroring the systems
// EventLog, so a binary that never initialises a ring degrades to a no-op
// instead of panicking on its first error — which would turn an ordinary 500
// into a crash loop.
func TestNilRingIsSafe(t *testing.T) {
	var r *Ring
	r.Record(Entry{Status: 500, Path: "/x"}) // must not panic
	if s := r.Snapshot(5); s.Held != 0 || s.Capacity != 0 || len(s.Entries) != 0 {
		t.Errorf("nil ring snapshot = %+v, want the zero value", s)
	}
}

// TestRecordFillsTimeAndTruncates covers the two normalisations Record applies.
func TestRecordFillsTimeAndTruncates(t *testing.T) {
	r := NewRing(4)
	before := time.Now()
	r.Record(Entry{Status: 500, Path: "/a"})
	got := r.Snapshot(1).Entries[0]
	if got.Time.Before(before) {
		t.Errorf("Record left Time = %v, want it filled with roughly now (%v)", got.Time, before)
	}

	fixed := time.Date(2026, 8, 11, 2, 0, 0, 0, time.UTC)
	r.Record(Entry{Status: 500, Path: "/b", Time: fixed})
	if got := r.Snapshot(1).Entries[0]; !got.Time.Equal(fixed) {
		t.Errorf("Record overwrote an explicit Time: got %v, want %v", got.Time, fixed)
	}

	long := strings.Repeat("x", maxErrLen+200)
	r.Record(Entry{Status: 500, Path: "/c", Err: long})
	stored := r.Snapshot(1).Entries[0].Err
	if len(stored) >= len(long) {
		t.Errorf("error string was not truncated: stored %d bytes of %d", len(stored), len(long))
	}
	if !strings.Contains(stored, "truncated") {
		t.Errorf("truncation was not marked; stored tail = %q", stored[max(0, len(stored)-30):])
	}
}

// TestPathForPrefersTemplate pins the privacy decision. Chronicle really does
// route `/rsvp/:token` and `/join/:code`, so a concrete path stored for one of
// those would put a live credential into a buffer whose whole purpose is to be
// pasted into a chat window.
func TestPathForPrefersTemplate(t *testing.T) {
	tests := []struct {
		name          string
		template, raw string
		wantPath      string
		wantTemplated bool
	}{
		{"a matched route stores the template", "/rsvp/:token", "/rsvp/9f3c-live-secret", "/rsvp/:token", true},
		{"campaign ids are not stored either", "/campaigns/:id/entities/:slug", "/campaigns/abc/entities/xyz", "/campaigns/:id/entities/:slug", true},
		{"a blank template falls back to the raw path", "", "/healthz", "/healthz", false},
		{"a whitespace template is still blank", "   ", "/healthz", "/healthz", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, templated := PathFor(tt.template, tt.raw)
			if p != tt.wantPath || templated != tt.wantTemplated {
				t.Errorf("PathFor(%q, %q) = (%q, %v), want (%q, %v)", tt.template, tt.raw, p, templated, tt.wantPath, tt.wantTemplated)
			}
		})
	}
}

// TestRecordHTTPErrorAppliesPolicy exercises the process-wide entry point:
// a 4xx is dropped, a 5xx is stored, and the token in the raw path never
// reaches the ring.
func TestRecordHTTPErrorAppliesPolicy(t *testing.T) {
	before := Recent(0).Total

	if RecordHTTPError(404, "GET", "", "/wp-admin", KindHTTP, errors.New("not found")) {
		t.Error("RecordHTTPError stored a 404 — the default policy must drop it")
	}
	if !RecordHTTPError(500, "POST", "/rsvp/:token", "/rsvp/live-secret-value", KindRaw, errors.New("boom")) {
		t.Fatal("RecordHTTPError dropped a 500")
	}

	s := Recent(1)
	if s.Total != before+1 {
		t.Errorf("Total went %d -> %d, want exactly one new entry", before, s.Total)
	}
	e := s.Entries[0]
	if e.Path != "/rsvp/:token" || !e.PathIsTemplate {
		t.Errorf("stored path = %q (templated=%v), want the template", e.Path, e.PathIsTemplate)
	}
	if strings.Contains(e.Path, "live-secret-value") {
		t.Errorf("the concrete path leaked into the ring: %q", e.Path)
	}
	if e.Kind != KindRaw || e.Status != 500 || e.Method != "POST" || e.Err != "boom" {
		t.Errorf("stored entry = %+v, want kind=raw status=500 method=POST err=boom", e)
	}
}

// TestRecordHTTPErrorNilError guards the path where the handler has a status
// but no error value; it must record rather than dereference nil.
func TestRecordHTTPErrorNilError(t *testing.T) {
	if !RecordHTTPError(503, "GET", "/healthz", "/healthz", KindApp, nil) {
		t.Fatal("a 5xx with a nil error was dropped")
	}
	if got := Recent(1).Entries[0].Err; got != "" {
		t.Errorf("nil error stored as %q, want empty", got)
	}
}

// TestRecordPanicIsAlwaysRecorded: a recovered panic never reaches the central
// error handler (recovery.go writes its own 500 and returns nil), so this is
// the only path that captures the most valuable error class there is.
func TestRecordPanicIsAlwaysRecorded(t *testing.T) {
	RecordPanic("GET", "/campaigns/:id", "/campaigns/abc", "runtime error: index out of range [3]")
	e := Recent(1).Entries[0]
	if e.Kind != KindPanic {
		t.Errorf("Kind = %q, want %q", e.Kind, KindPanic)
	}
	if e.Status != 500 {
		t.Errorf("Status = %d, want 500 (what recovery.go actually sends)", e.Status)
	}
	if !strings.Contains(e.Err, "index out of range") {
		t.Errorf("Err = %q, want it to carry the panic value", e.Err)
	}
	if strings.Contains(e.Err, "goroutine") {
		t.Errorf("a stack trace was stored: %q — the stack belongs in the log, not in a 256-slot ring", e.Err)
	}
}

// TestConcurrentRecordAndSnapshot is the race test. Run under -race it is the
// only thing that proves the mutex actually covers every field it needs to:
// buf, next, full and total are all mutated together and read together.
func TestConcurrentRecordAndSnapshot(t *testing.T) {
	const writers, reads, perWriter = 8, 200, 250
	r := NewRing(64) // deliberately smaller than the write count, so it wraps hard

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				r.Record(Entry{
					Status: 500,
					Method: "GET",
					Path:   fmt.Sprintf("/w%d", w),
					Kind:   KindRaw,
					Err:    fmt.Sprintf("writer %d error %d", w, i),
				})
			}
		}(w)
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < reads; j++ {
				s := r.Snapshot(25)
				// Touch the returned data: a snapshot that handed back
				// aliased backing memory would race HERE, not at the copy.
				for k := range s.Entries {
					_ = s.Entries[k].Path + s.Entries[k].Err
				}
			}
		}()
	}
	wg.Wait()

	s := r.Snapshot(0)
	if s.Total != uint64(writers*perWriter) {
		t.Errorf("Total = %d, want %d — a lost update means the counter is outside the lock", s.Total, writers*perWriter)
	}
	if s.Held != 64 {
		t.Errorf("Held = %d, want 64 (capacity)", s.Held)
	}
	for i, e := range s.Entries {
		if e.Status != 500 || e.Path == "" {
			t.Fatalf("entry %d is torn or empty: %+v", i, e)
		}
	}
}

// TestRecordBoundsPath is the regression guard for an unbounded field that hid
// behind a usually-short one.
//
// Path is normally a route template this codebase wrote, so it looked harmless
// and went unbounded while its sibling Err was truncated from the start. But
// PathFor's fallback branch stores raw request bytes, that branch IS reachable
// (a panic in global middleware on an unmatched route records unconditionally,
// because RecordPanic never consults ShouldRecord), and net/http accepts a
// request line up to ~1 MiB. Measured before the fix: a 1 MiB GET put 349,526
// bytes into ONE entry, so a full ring retained ~90 MB — in a buffer whose only
// purpose is to be small enough to paste into a chat window.
func TestRecordBoundsPath(t *testing.T) {
	r := NewRing(4)
	long := "/" + strings.Repeat("a", maxPathLen*50)
	r.Record(Entry{Status: 500, Path: long})

	stored := r.Snapshot(1).Entries[0].Path
	if len(stored) >= len(long) {
		t.Errorf("path was not truncated: stored %d bytes of %d", len(stored), len(long))
	}
	// The bound has to be the bound, not merely "shorter". truncate appends a
	// marker, so allow for it.
	if len(stored) > maxPathLen+len("…[truncated]") {
		t.Errorf("path exceeded the bound: stored %d bytes, cap is %d", len(stored), maxPathLen)
	}
	if !strings.Contains(stored, "truncated") {
		t.Errorf("truncation was not marked, so a clipped path reads as a whole one: %q", stored)
	}

	// A real route template must survive untouched — a bound that mangles
	// ordinary output would be traded for a bound nobody trusts.
	const real = "/campaigns/:id/entities/:slug"
	r.Record(Entry{Status: 500, Path: real})
	if got := r.Snapshot(1).Entries[0].Path; got != real {
		t.Errorf("an ordinary route template was altered: got %q, want %q", got, real)
	}
}
