// Package campaigns — import_report.go carries the per-object failure tally
// for a campaign import.
//
// Import is deliberately best-effort: one bad row must not abandon a
// half-created campaign, so every importer catches its own per-row errors and
// moves on. Until sweep R4 stage 16 that was the whole story — each skipped
// row went to slog.Warn and the operator was redirected to their shiny new
// campaign with no indication that anything had been left behind. A restore
// that quietly drops rows is worse than one that fails, because the operator
// stops looking.
//
// Fix id: backend/import-silent-partial-success.
//
// ImportReport keeps best-effort behaviour and adds the missing half: every
// skip is recorded, counted, and shown. The report threads through the
// importer adapters exactly like *IDMap does.
package campaigns

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// maxRecordedImportFailures caps how many individual failures we retain. A
// pathological import (wrong-schema file, dead database) could otherwise
// accumulate one record per row. The COUNT stays exact past the cap; only the
// per-row detail is dropped, because the count is what tells the operator
// whether to trust the restore.
const maxRecordedImportFailures = 200

// Report section and kind labels whose spelling collides with a plugin slug.
//
// These are user-facing report headings ("calendar", "timelines") and singular
// human nouns ("calendar", "timeline"). Nothing resolves a plugin from them —
// they are never compared against a registry, never used to build a route, and
// never round-trip into the export envelope as a plugin identifier. But a bare
// literal at a call site is indistinguishable from a real cross-plugin
// reference to tools/check-plugin-isolation.sh (T-B2 / M-B2.1), and the guard
// is right to be unable to tell the difference: a heading today is a lookup key
// tomorrow.
//
// So the vocabulary lives here, in the file that owns ImportFailure, and the
// call sites name the constant. This is the second remedy the guard documents
// ("route the labels through a constant"). The guard's const_registry_files
// list (amendment R4-S26-A) permits the literals on THESE const lines only —
// any other line of this file, and every call site in every other file, stays
// fully governed. tools/test-plugin-isolation.sh pins that narrowness.
//
// Labels that do not collide with a plugin slug ("notes", "maps", "entities",
// "sessions", …) are deliberately NOT hoisted here: they are ordinary prose and
// hoisting them would imply the guard cares about them.
const (
	// SectionCalendar is the export-envelope section heading for calendar data.
	SectionCalendar = "calendar"
	// SectionTimelines is the export-envelope section heading for timelines.
	SectionTimelines = "timelines"
	// KindCalendar is the singular noun for a calendar in a failure row.
	KindCalendar = "calendar"
	// KindTimeline is the singular noun for a timeline in a failure row.
	KindTimeline = "timeline"
)

// ImportFailure records one object the import could not create.
type ImportFailure struct {
	// Section is the export envelope section the object came from,
	// e.g. "notes", "maps", "entities".
	Section string

	// Kind is a singular human noun for the object, e.g. "note",
	// "map marker". Used to build the summary line.
	Kind string

	// Name is the best identifier available for the object — usually its
	// title. May be empty for objects that have no name.
	Name string

	// Reason is a short, user-safe cause. Never a raw driver error.
	Reason string

	// Count is how many objects this record stands for. Normally 1. A
	// section that loses a whole homogeneous batch — every media file in a
	// zip, say — records it once with the real count rather than emitting
	// hundreds of identical rows, so the tally stays exact without burying
	// the interesting failures.
	Count int
}

// ImportReport accumulates per-object failures during a campaign import.
//
// All methods are safe on a nil receiver so an adapter can be exercised
// without one, and safe under concurrent use so a future parallel importer
// does not have to revisit this.
type ImportReport struct {
	mu        sync.Mutex
	failures  []ImportFailure
	total     int
	truncated int
}

// NewImportReport returns an empty report.
func NewImportReport() *ImportReport { return &ImportReport{} }

// Fail records one failed object. reason should already be user-safe;
// callers holding a raw error should pass apperror.SafeMessage(err).
func (r *ImportReport) Fail(section, kind, name, reason string) {
	r.FailN(section, kind, name, reason, 1)
}

// FailN records n identical failures as a single detail row. Use it when a
// whole homogeneous batch is lost for one reason; use Fail when the objects
// are individually worth naming.
func (r *ImportReport) FailN(section, kind, name, reason string, n int) {
	if r == nil || n <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.total += n
	if len(r.failures) >= maxRecordedImportFailures {
		r.truncated += n
		return
	}
	r.failures = append(r.failures, ImportFailure{
		Section: section,
		Kind:    kind,
		Name:    name,
		Reason:  reason,
		Count:   n,
	})
}

// Count returns the exact number of failed objects, including any beyond
// the detail cap.
func (r *ImportReport) Count() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.total
}

// HasFailures reports whether anything was dropped.
func (r *ImportReport) HasFailures() bool { return r.Count() > 0 }

// Truncated returns how many failures were counted but not retained in
// detail. Non-zero means the detail list is incomplete; the count is not.
func (r *ImportReport) Truncated() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.truncated
}

// Failures returns a copy of the retained failure detail.
func (r *ImportReport) Failures() []ImportFailure {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ImportFailure, len(r.failures))
	copy(out, r.failures)
	return out
}

// KindCount is one row of the summary: how many of a given kind failed.
type KindCount struct {
	Kind  string
	Count int
}

// CountsByKind returns per-kind totals, largest first then alphabetical, so
// the summary line is stable between runs. Failures beyond the detail cap
// are attributed to the kind they were recorded under, which is why the cap
// is generous: the shape of a partial import matters as much as its size.
func (r *ImportReport) CountsByKind() []KindCount {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	byKind := map[string]int{}
	for _, f := range r.failures {
		byKind[f.Kind] += f.Count
	}
	out := make([]KindCount, 0, len(byKind))
	for k, n := range byKind {
		out = append(out, KindCount{Kind: k, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// Summary renders the failure tally as one line naming how many of what
// failed, e.g. "3 notes, 1 map marker". Empty string when nothing failed.
func (r *ImportReport) Summary() string {
	counts := r.CountsByKind()
	if len(counts) == 0 {
		return ""
	}
	parts := make([]string, 0, len(counts))
	for _, c := range counts {
		parts = append(parts, fmt.Sprintf("%d %s", c.Count, pluralize(c.Kind, c.Count)))
	}
	line := strings.Join(parts, ", ")
	if t := r.Truncated(); t > 0 {
		line += fmt.Sprintf(", and %d more", t)
	}
	return line
}

// pluralize adds a naive plural to a kind noun. Kinds are chosen at the call
// sites to be regular ("note", "map marker", "session"), so the naive rule
// holds; the "s"/"x"/"ch"/"sh" branch is there so a future kind like "class"
// does not read as "classs".
func pluralize(kind string, n int) string {
	if n == 1 || kind == "" {
		return kind
	}
	switch {
	case strings.HasSuffix(kind, "s"), strings.HasSuffix(kind, "x"),
		strings.HasSuffix(kind, "ch"), strings.HasSuffix(kind, "sh"):
		return kind + "es"
	case strings.HasSuffix(kind, "y") && len(kind) > 1 && !isVowel(kind[len(kind)-2]):
		return kind[:len(kind)-1] + "ies"
	default:
		return kind + "s"
	}
}

func isVowel(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}
