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
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
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

// --- inline-secret redaction (P0: DM-secret egress) ---

// secretHTML is a representative rendered-HTML payload carrying an
// inline GM secret span alongside player-visible prose. StripSecretsHTML
// must drop the whole <span data-secret> element.
const secretHTML = `<p>Public intro.</p>` +
	`<p>The vault code is <span data-secret="true">42-42-42</span>.</p>`

// secretJSON is a representative ProseMirror document carrying a
// "secret"-marked text node. StripSecretsJSON must drop that node.
const secretJSON = `{"type":"doc","content":[` +
	`{"type":"paragraph","content":[` +
	`{"type":"text","text":"Public. "},` +
	`{"type":"text","text":"42-42-42","marks":[{"type":"secret"}]}` +
	`]}]}`

// secretToken is the GM-only substring that must never appear in a
// below-Scribe response, in either HTML or JSON representation.
const secretToken = "42-42-42"

// assertNoSecret encodes v to JSON (the on-the-wire shape) and asserts
// the GM-only token did not survive redaction.
func assertNoSecret(t *testing.T, label string, v interface{}) {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: marshal: %v", label, err)
	}
	if strings.Contains(string(body), secretToken) {
		t.Errorf("%s: response body still contains GM secret %q; body = %s",
			label, secretToken, string(body))
	}
}

// assertHasSecret is the inverse — Owner/Scribe responses MUST retain
// the secret content (they see it with a client-side indicator).
func assertHasSecret(t *testing.T, label string, v interface{}) {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: marshal: %v", label, err)
	}
	if !strings.Contains(string(body), secretToken) {
		t.Errorf("%s: response body dropped secret %q for an above-bar role; body = %s",
			label, secretToken, string(body))
	}
}

// newSecretEntity builds an entity whose four secret-bearing fields all
// carry the GM token, in both HTML and ProseMirror-JSON form.
func newSecretEntity() *entities.Entity {
	return &entities.Entity{
		ID:              "ent-secret",
		Entry:           sptr(secretJSON),
		EntryHTML:       sptr(secretHTML),
		PlayerNotes:     sptr(secretJSON),
		PlayerNotesHTML: sptr(secretHTML),
	}
}

// TestStripEntitySecretsForEgress_ByRole pins the role boundary: a
// Player (below RoleScribe) must NOT receive secret content; a Scribe
// and an Owner (at/above the bar) must. Mirrors the web GetEntry rule
// (entities/handler.go: MemberRole < RoleScribe). Both the HTML and the
// ProseMirror-JSON representations are checked, since a client may read
// either — leaking through one defeats the fix.
func TestStripEntitySecretsForEgress_ByRole(t *testing.T) {
	cases := []struct {
		name      string
		role      int
		wantStrip bool
	}{
		{"none/anonymous below player", int(campaigns.RoleNone), true},
		{"player", int(campaigns.RolePlayer), true},
		{"scribe", int(campaigns.RoleScribe), false},
		{"owner", int(campaigns.RoleOwner), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newSecretEntity()
			stripEntitySecretsForEgress(e, tc.role)
			if tc.wantStrip {
				assertNoSecret(t, "entity", e)
				// Spot-check each field individually so a single
				// un-stripped field can't hide behind another's success.
				assertNoSecret(t, "entity.Entry", derefForTest(e.Entry))
				assertNoSecret(t, "entity.EntryHTML", derefForTest(e.EntryHTML))
				assertNoSecret(t, "entity.PlayerNotes", derefForTest(e.PlayerNotes))
				assertNoSecret(t, "entity.PlayerNotesHTML", derefForTest(e.PlayerNotesHTML))
			} else {
				assertHasSecret(t, "entity", e)
			}
		})
	}
}

// TestStripEntitiesSecretsForEgress_ByRole covers the slice variant
// (ListEntities / sync-pull). Every element must be redacted for a
// below-bar caller; a forgotten loop iteration would leak one element.
func TestStripEntitiesSecretsForEgress_ByRole(t *testing.T) {
	t.Run("player strips all", func(t *testing.T) {
		es := []entities.Entity{*newSecretEntity(), *newSecretEntity()}
		stripEntitiesSecretsForEgress(es, int(campaigns.RolePlayer))
		assertNoSecret(t, "entities[0]", es[0])
		assertNoSecret(t, "entities[1]", es[1])
	})
	t.Run("owner keeps all", func(t *testing.T) {
		es := []entities.Entity{*newSecretEntity(), *newSecretEntity()}
		stripEntitiesSecretsForEgress(es, int(campaigns.RoleOwner))
		assertHasSecret(t, "entities[0]", es[0])
		assertHasSecret(t, "entities[1]", es[1])
	})
}

// TestStripEntitySecretsForEgress_NilSafe asserts nil-pointer and
// nil-field safety — handlers can pass a nil entity from an error
// branch, and nil fields are the common case (most entities have no
// player notes).
func TestStripEntitySecretsForEgress_NilSafe(t *testing.T) {
	stripEntitySecretsForEgress(nil, int(campaigns.RolePlayer)) // must not panic

	e := &entities.Entity{ID: "ent-1"}
	stripEntitySecretsForEgress(e, int(campaigns.RolePlayer))
	if e.Entry != nil || e.EntryHTML != nil || e.PlayerNotes != nil || e.PlayerNotesHTML != nil {
		t.Errorf("nil secret fields mutated: %+v", e)
	}
}

// TestStripEntitySecretsForEgress_DoesNotMutateSource proves the egress
// transform hands back fresh pointers rather than editing the DB-fresh
// model in place — the source strings must be untouched even when their
// content is stripped from the response.
func TestStripEntitySecretsForEgress_DoesNotMutateSource(t *testing.T) {
	originalHTML := secretHTML
	originalJSON := secretJSON
	e := &entities.Entity{EntryHTML: &originalHTML, Entry: &originalJSON}
	stripEntitySecretsForEgress(e, int(campaigns.RolePlayer))
	if originalHTML != secretHTML {
		t.Errorf("source EntryHTML mutated; got %q", originalHTML)
	}
	if originalJSON != secretJSON {
		t.Errorf("source Entry mutated; got %q", originalJSON)
	}
}

// derefForTest returns the pointed-to string, or "" for nil. Local
// helper so the assert* functions can take a plain string for a field.
func derefForTest(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// TestEntityEgress_SanitizeThenStrip_OrderingSafe pins the load-bearing
// interaction between the two egress transforms the entity handlers run
// in sequence: the role-agnostic XSS sanitize (sanitizeEntityHTMLForEgress)
// FIRST, then the role-aware secret strip (stripEntitySecretsForEgress).
//
// The risk: if bluemonday stripped the data-secret attribute, the
// subsequent StripSecretsHTML regex (which matches <span data-secret>)
// would no longer fire and the secret PROSE would ship. The sanitize
// policy deliberately allow-lists data-secret on <span> exactly so the
// downstream strip can find it. This test runs the handler's two-step
// order and proves a below-bar caller still gets no secret, while the
// surrounding public prose survives.
func TestEntityEgress_SanitizeThenStrip_OrderingSafe(t *testing.T) {
	e := newSecretEntity()
	// Same order the handler applies them.
	sanitizeEntityHTMLForEgress(e)
	stripEntitySecretsForEgress(e, int(campaigns.RolePlayer))

	assertNoSecret(t, "entity after sanitize+strip", e)
	// The public prose must survive both transforms (we didn't nuke the
	// whole field).
	if got := derefForTest(e.EntryHTML); !strings.Contains(got, "Public intro") {
		t.Errorf("public prose lost after sanitize+strip; EntryHTML = %q", got)
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
		// P0 DM-secret egress redaction (C-SYNCAPI-PRELAUNCH-HARDENING):
		// the entity read handlers must ALSO invoke the role-aware
		// secret stripper, or a player-role caller reads raw GM prose.
		{"api_handler.go", "GetEntity", "stripEntitySecretsForEgress"},
		{"api_handler.go", "ListEntities", "stripEntitiesSecretsForEgress"},
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
