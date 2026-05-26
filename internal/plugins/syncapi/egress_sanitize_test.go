// Tests for the egress sanitize helpers. Each helper takes the
// model the corresponding /api/v1/* GET handler returns, with a
// polluted HTML field, and is expected to strip dangerous content
// before serialization. Mirrors the dispatch's per-handler
// polluted-DB-row test: load malicious HTML into the model, run the
// helper, assert no script tag remains. Encoded via json.Marshal to
// match the on-the-wire shape clients see.
package syncapi

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// polluted is a representative malicious HTML payload. Combines a
// classic <script>alert</script>, an inline event handler, and a
// javascript: URL — all three should be stripped by bluemonday's
// UGC policy that internal/sanitize wraps.
const polluted = `<p>hello</p>` +
	`<script>alert('xss')</script>` +
	`<a href="javascript:alert(1)" onclick="alert(2)">click</a>`

// assertCleaned encodes v to JSON (the on-the-wire shape) and
// asserts no obviously-dangerous markers survived sanitization.
// Doesn't assert specific allowed content — that's bluemonday's
// concern and is pinned at the sanitize-package level.
func assertCleaned(t *testing.T, label string, v interface{}) {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: marshal: %v", label, err)
	}
	got := string(body)
	for _, bad := range []string{"<script", "onclick=", "javascript:"} {
		if strings.Contains(strings.ToLower(got), bad) {
			t.Errorf("%s: response body contains %q after sanitize; body = %s",
				label, bad, got)
		}
	}
}

// sptr returns &s. Local to this test file because the package already
// has a `strPtr` helper with a different (nil-on-empty) shape.
func sptr(s string) *string {
	return &s
}

// --- entity ---

// TestSanitizeEntityHTMLForEgress_StripsScript covers GetEntity's
// per-entity case. Both EntryHTML and PlayerNotesHTML must be
// scrubbed; either left raw is a defense-in-depth gap.
func TestSanitizeEntityHTMLForEgress_StripsScript(t *testing.T) {
	e := &entities.Entity{
		ID:              "ent-1",
		EntryHTML:       sptr(polluted),
		PlayerNotesHTML: sptr(polluted),
	}
	sanitizeEntityHTMLForEgress(e)
	assertCleaned(t, "entity.EntryHTML",       *e.EntryHTML)
	assertCleaned(t, "entity.PlayerNotesHTML", *e.PlayerNotesHTML)
	// Full payload check — catches a future field that surfaces with
	// HTML but isn't yet listed in the helper.
	assertCleaned(t, "entity marshaled", e)
}

// TestSanitizeEntitiesHTMLForEgress_StripsScript covers ListEntities.
// Each element of the slice must be sanitized — a forgotten loop
// would leak the polluted element while others looked fine.
func TestSanitizeEntitiesHTMLForEgress_StripsScript(t *testing.T) {
	es := []entities.Entity{
		{ID: "a", EntryHTML: sptr(polluted), PlayerNotesHTML: sptr(polluted)},
		{ID: "b", EntryHTML: sptr(polluted)},
	}
	sanitizeEntitiesHTMLForEgress(es)
	assertCleaned(t, "entities[0]", es[0])
	assertCleaned(t, "entities[1]", es[1])
}

// TestSanitizeEntityHTMLForEgress_NilSafe asserts the helper is
// safe on nil — handlers may pass through nil from an error
// branch, and nil-deref-panic-at-egress is worse than no sanitize.
func TestSanitizeEntityHTMLForEgress_NilSafe(t *testing.T) {
	sanitizeEntityHTMLForEgress(nil) // must not panic

	// Nil HTML pointers: helper must leave them nil, not crash or
	// substitute empty string.
	e := &entities.Entity{ID: "ent-1"}
	sanitizeEntityHTMLForEgress(e)
	if e.EntryHTML != nil {
		t.Errorf("nil EntryHTML mutated to %v", e.EntryHTML)
	}
	if e.PlayerNotesHTML != nil {
		t.Errorf("nil PlayerNotesHTML mutated to %v", e.PlayerNotesHTML)
	}
}

// TestSanitizeEntityHTMLForEgress_DoesNotMutateOriginalString proves
// the sanitize.HTMLPtr contract: the original *string's referent is
// not edited. The egress helper hands back a fresh pointer to a
// fresh string. Matters because the DB-fresh model could be re-
// referenced upstream (caching, downstream handler chaining); only
// the response copy should change.
func TestSanitizeEntityHTMLForEgress_DoesNotMutateOriginalString(t *testing.T) {
	original := polluted
	e := &entities.Entity{EntryHTML: &original}
	sanitizeEntityHTMLForEgress(e)
	if original != polluted {
		t.Errorf("source string mutated; got %q, want polluted", original)
	}
}

// --- note ---

// TestSanitizeNoteHTMLForEgress_StripsScript covers GetNote.
func TestSanitizeNoteHTMLForEgress_StripsScript(t *testing.T) {
	n := &notes.Note{
		ID:        "note-1",
		EntryHTML: sptr(polluted),
	}
	sanitizeNoteHTMLForEgress(n)
	assertCleaned(t, "note.EntryHTML", *n.EntryHTML)
	assertCleaned(t, "note marshaled", n)
}

// TestSanitizeNotesHTMLForEgress_StripsScript covers ListNotes.
func TestSanitizeNotesHTMLForEgress_StripsScript(t *testing.T) {
	ns := []notes.Note{
		{ID: "a", EntryHTML: sptr(polluted)},
		{ID: "b", EntryHTML: sptr(polluted)},
	}
	sanitizeNotesHTMLForEgress(ns)
	assertCleaned(t, "notes[0]", ns[0])
	assertCleaned(t, "notes[1]", ns[1])
}

func TestSanitizeNoteHTMLForEgress_NilSafe(t *testing.T) {
	sanitizeNoteHTMLForEgress(nil) // must not panic

	n := &notes.Note{ID: "note-1"}
	sanitizeNoteHTMLForEgress(n)
	if n.EntryHTML != nil {
		t.Errorf("nil EntryHTML mutated to %v", n.EntryHTML)
	}
}

// --- calendar event ---

// TestSanitizeCalendarEventHTMLForEgress_StripsScript covers GetEvent.
func TestSanitizeCalendarEventHTMLForEgress_StripsScript(t *testing.T) {
	e := &calendar.Event{
		ID:              "evt-1",
		DescriptionHTML: sptr(polluted),
	}
	sanitizeCalendarEventHTMLForEgress(e)
	assertCleaned(t, "event.DescriptionHTML", *e.DescriptionHTML)
	assertCleaned(t, "event marshaled", e)
}

// TestSanitizeCalendarEventsHTMLForEgress_StripsScript covers
// ListEvents.
func TestSanitizeCalendarEventsHTMLForEgress_StripsScript(t *testing.T) {
	es := []calendar.Event{
		{ID: "a", DescriptionHTML: sptr(polluted)},
		{ID: "b", DescriptionHTML: sptr(polluted)},
	}
	sanitizeCalendarEventsHTMLForEgress(es)
	assertCleaned(t, "events[0]", es[0])
	assertCleaned(t, "events[1]", es[1])
}

func TestSanitizeCalendarEventHTMLForEgress_NilSafe(t *testing.T) {
	sanitizeCalendarEventHTMLForEgress(nil) // must not panic

	e := &calendar.Event{ID: "evt-1"}
	sanitizeCalendarEventHTMLForEgress(e)
	if e.DescriptionHTML != nil {
		t.Errorf("nil DescriptionHTML mutated to %v", e.DescriptionHTML)
	}
}

// TestEgressSanitize_HandlersInvokeHelpers is the load-bearing
// structural pin: every /api/v1/* GET handler that emits HTML must
// call the corresponding sanitize-for-egress helper inside its
// function body. Walks the handler file's AST to find the named
// handler method and asserts the helper identifier appears in its
// body.
//
// Why a structural test instead of a wired-up integration test:
// stubbing the full EntityService / NoteService / CalendarService
// interface surface (30+ methods each) for one egress assertion is
// disproportionate. The egress helper itself is unit-tested above;
// this pins the wiring at the handler.
//
// Failure mode this catches: a future refactor that drops the
// helper call from a handler (or adds a new HTML-emitting handler
// without wiring the helper). Update the case table below when a
// new HTML-emitting GET handler lands on /api/v1/*.
func TestEgressSanitize_HandlersInvokeHelpers(t *testing.T) {
	cases := []struct {
		file       string
		fn         string
		mustCall   string
	}{
		{"api_handler.go", "GetEntity", "sanitizeEntityHTMLForEgress"},
		{"api_handler.go", "ListEntities", "sanitizeEntitiesHTMLForEgress"},
		{"note_api_handler.go", "GetNote", "sanitizeNoteHTMLForEgress"},
		{"note_api_handler.go", "ListNotes", "sanitizeNotesHTMLForEgress"},
		{"calendar_api_handler.go", "GetEvent", "sanitizeCalendarEventHTMLForEgress"},
		{"calendar_api_handler.go", "ListEvents", "sanitizeCalendarEventsHTMLForEgress"},
	}
	for _, tc := range cases {
		t.Run(tc.fn, func(t *testing.T) {
			body := readHandlerBody(t, tc.file, tc.fn)
			if !strings.Contains(body, tc.mustCall+"(") {
				t.Errorf("%s::%s does not call %s — egress sanitization wiring missing.\n"+
					"Per cordinator/dispatches/chronicle/C-SEC-CHUNK-6-AMENDED.md, every /api/v1/* "+
					"GET handler that emits HTML must invoke its egress-sanitize helper before "+
					"c.JSON. Body:\n%s",
					tc.file, tc.fn, tc.mustCall, body)
			}
		})
	}
}

// readHandlerBody parses file, locates the FuncDecl whose name is fn,
// and returns its body's source text. Failure to find the function is
// a test failure — drift between the case table and the handler file
// names should surface loudly.
func readHandlerBody(t *testing.T, file, fn string) string {
	t.Helper()
	src, err := readSourceFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != fn || fd.Body == nil {
			continue
		}
		start := fset.Position(fd.Body.Pos()).Offset
		end := fset.Position(fd.Body.End()).Offset
		return string(src[start:end])
	}
	t.Fatalf("function %s not found in %s", fn, file)
	return ""
}

func readSourceFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}
