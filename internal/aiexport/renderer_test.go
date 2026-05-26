// renderer_test.go covers per-category renderers + the egress-
// sanitization structural pin (PR-A's load-bearing invariant).
//
// SEC-6-AMENDED inheritance: every renderer that emits user HTML
// MUST funnel through htmlToMarkdown, which is the only allowed
// path into the html-to-markdown converter (sanitize.HTMLPtr runs
// BEFORE the converter). The structural pin walks each Render*
// function's AST + asserts it contains `htmlToMarkdown(` (i.e. it
// can't bypass via direct converter access). A future refactor
// that adds a renderer without funneling through fails pinpointed.
package aiexport

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// polluted is the SEC-6-AMENDED test fixture — script tag + onclick
// handler + javascript: URL. After renderer runs, none of these
// substrings may appear in the output markdown.
const polluted = `<p>hello</p>` +
	`<script>alert('xss')</script>` +
	`<a href="javascript:alert(1)" onclick="alert(2)">click</a>`

func sp(s string) *string { return &s }

func assertClean(t *testing.T, label, got string) {
	t.Helper()
	low := strings.ToLower(got)
	for _, bad := range []string{"<script", "onclick=", "javascript:"} {
		if strings.Contains(low, bad) {
			t.Errorf("%s: output contains %q after sanitize. body:\n%s", label, bad, got)
		}
	}
}

// ---------------------------------------------------------------------------
// AST structural pin — the load-bearing invariant
// ---------------------------------------------------------------------------

// TestRenderers_FunnelThroughHtmlToMarkdown asserts every per-category
// renderer + the renderEntity / renderSession / etc helpers funnel
// user-HTML conversions through htmlToMarkdown (which applies
// sanitize.HTMLPtr per SEC-6-AMENDED). If a future refactor adds a
// direct call to the markdown library, the pin fails pinpointed.
func TestRenderers_FunnelThroughHtmlToMarkdown(t *testing.T) {
	src, err := os.ReadFile("renderer.go")
	if err != nil {
		t.Fatalf("read renderer.go: %v", err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "renderer.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse renderer.go: %v", err)
	}

	// Functions that consume HTML fields and must funnel through
	// htmlToMarkdown. If a new Render* lands, add it here.
	required := []string{
		"renderEntity",
		"renderNoteTree",
		"renderCalendarEvent",
		"renderSession",
		"renderTimeline",
	}

	bodies := map[string]string{}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		start := fset.Position(fd.Body.Pos()).Offset
		end := fset.Position(fd.Body.End()).Offset
		bodies[fd.Name.Name] = string(src[start:end])
	}

	for _, name := range required {
		t.Run(name, func(t *testing.T) {
			body, ok := bodies[name]
			if !ok {
				t.Fatalf("function %s missing from renderer.go", name)
			}
			if !strings.Contains(body, "htmlToMarkdown(") {
				t.Errorf("%s does not call htmlToMarkdown(...) — "+
					"SEC-6-AMENDED egress invariant: every user-HTML "+
					"emit MUST funnel through htmlToMarkdown so "+
					"sanitize.HTMLPtr runs first. Body:\n%s", name, body)
			}
			// Forbid direct converter access — if you see this fail,
			// a future refactor leaked the converter; route through
			// htmlToMarkdown instead.
			if strings.Contains(body, "getConverter(") {
				t.Errorf("%s bypasses htmlToMarkdown by calling "+
					"getConverter directly. Only htmlToMarkdown may "+
					"reach the converter — it's the single point "+
					"that applies sanitize.HTMLPtr first.", name)
			}
		})
	}
}

// TestHtmlToMarkdown_StripsScriptViaSanitizeHTMLPtr proves the
// SEC-6-AMENDED invariant lands at the converter level: a malicious
// HTML pointer through htmlToMarkdown yields markdown with no script
// / onclick / javascript: artifact.
func TestHtmlToMarkdown_StripsScriptViaSanitizeHTMLPtr(t *testing.T) {
	got, err := htmlToMarkdown(sp(polluted))
	if err != nil {
		t.Fatalf("htmlToMarkdown: %v", err)
	}
	assertClean(t, "htmlToMarkdown(polluted)", got)
}

func TestHtmlToMarkdown_NilAndEmpty(t *testing.T) {
	got, err := htmlToMarkdown(nil)
	if err != nil || got != "" {
		t.Errorf("nil input → (%q, %v), want (\"\", nil)", got, err)
	}
	empty := ""
	got, err = htmlToMarkdown(&empty)
	if err != nil || got != "" {
		t.Errorf("empty input → (%q, %v), want (\"\", nil)", got, err)
	}
}

// ---------------------------------------------------------------------------
// Per-category integration-light tests
// ---------------------------------------------------------------------------

func TestRenderEntities_StripsScriptAndGroups(t *testing.T) {
	ctx := context.Background()
	ents := []entities.Entity{
		{
			ID: "e1", Name: "Lyra Vance", EntityTypeID: 1,
			TypeName: "Character", TypeLabel: sp("PC · Storm Sorcerer"),
			EntryHTML: sp(polluted),
		},
		{
			ID: "e2", Name: "The Coral Court", EntityTypeID: 2,
			TypeName: "Location",
			EntryHTML: sp("<p>A ruined court submerged in the Cataclysm.</p>"),
		},
	}
	types := []entities.EntityType{
		{ID: 1, Name: "Character", NamePlural: "Characters", SortOrder: 1},
		{ID: 2, Name: "Location", NamePlural: "Locations", SortOrder: 2},
	}
	tagsByEntity := map[string][]tags.Tag{
		"e1": {{ID: 1, Name: "pc"}, {ID: 2, Name: "secret", DmOnly: true}},
	}
	relByEntity := map[string][]relations.Relation{
		"e1": {{
			TargetEntityName: "The Coral Court",
			RelationType:     "haunts",
		}},
	}

	got, err := RenderEntities(ctx, ents, types, tagsByEntity, relByEntity, Options{Privacy: PrivacyModeSafe})
	if err != nil {
		t.Fatalf("RenderEntities: %v", err)
	}
	assertClean(t, "RenderEntities", got)
	// Heading grouping
	for _, want := range []string{"# Entities", "## Characters", "## Locations",
		"### Lyra Vance", "### The Coral Court",
		"**Tags:** pc",                  // dm_only tag dropped in Safe mode
		"haunts [The Coral Court](#",    // relation with wikilink
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want substring %q in output:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secret") {
		t.Errorf("Safe mode leaked dm_only tag 'secret':\n%s", got)
	}
}

func TestRenderEntities_PermittedKeepsDmOnlyTags(t *testing.T) {
	ctx := context.Background()
	ents := []entities.Entity{{
		ID: "e1", Name: "Lyra Vance", EntityTypeID: 1, TypeName: "Character",
		EntryHTML: sp("<p>Body.</p>"),
	}}
	types := []entities.EntityType{{ID: 1, Name: "Character", NamePlural: "Characters"}}
	tagsByEntity := map[string][]tags.Tag{
		"e1": {{ID: 1, Name: "pc"}, {ID: 2, Name: "secret", DmOnly: true}},
	}
	got, err := RenderEntities(ctx, ents, types, tagsByEntity, nil,
		Options{Privacy: PrivacyModePermitted})
	if err != nil {
		t.Fatalf("RenderEntities: %v", err)
	}
	if !strings.Contains(got, "secret") {
		t.Errorf("Permitted mode dropped dm_only tag 'secret':\n%s", got)
	}
}

func TestRenderNotes_FolderHierarchyAndScriptStripped(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	list := []notes.Note{
		{ID: "f1", Title: "Plot Threads", IsFolder: true},
		{ID: "n1", Title: "Hollow Captain's Pact", ParentID: sp("f1"),
			EntryHTML: sp(polluted), Pinned: true, IsShared: true, UpdatedAt: now},
		{ID: "n2", Title: "Top-level note",
			EntryHTML: sp("<p>standalone</p>"), UpdatedAt: now},
	}
	got, err := RenderNotes(ctx, list, Options{Privacy: PrivacyModeSafe})
	if err != nil {
		t.Fatalf("RenderNotes: %v", err)
	}
	assertClean(t, "RenderNotes", got)
	for _, want := range []string{"# Notes",
		"📁 Plot Threads",
		"Hollow Captain's Pact",
		"**Pinned**",
		"_shared with campaign_",
		"Top-level note",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want substring %q in output:\n%s", want, got)
		}
	}
}

func TestRenderCalendarEvents_MonthNamesAndSafeFilter(t *testing.T) {
	ctx := context.Background()
	cal := &calendar.Calendar{
		ID: "cal1", Name: "Ashfall Reckoning", EpochName: sp("Coral Age"),
		Months: []calendar.Month{
			{Name: "Highsummer", Days: 30, SortOrder: 1},
			{Name: "Stormfall", Days: 30, SortOrder: 2},
		},
		Eras: []calendar.Era{
			{Name: "AR", StartYear: 1, EndYear: nil},
		},
	}
	events := []calendar.Event{
		{ID: "e1", Name: "The Glasswater Tide", Year: 1247, Month: 1, Day: 4,
			DescriptionHTML: sp(polluted), Visibility: "everyone"},
		{ID: "e2", Name: "Hidden Council", Year: 1247, Month: 1, Day: 10,
			DescriptionHTML: sp("<p>Secret meeting.</p>"), Visibility: "dm_only"},
	}
	got, err := RenderCalendarEvents(ctx, cal, events, Options{Privacy: PrivacyModeSafe})
	if err != nil {
		t.Fatalf("RenderCalendarEvents: %v", err)
	}
	assertClean(t, "RenderCalendarEvents", got)
	if !strings.Contains(got, "Highsummer 1247 AR") {
		t.Errorf("expected human-readable month/year/era heading:\n%s", got)
	}
	if !strings.Contains(got, "The Glasswater Tide") {
		t.Errorf("expected event title in output:\n%s", got)
	}
	if strings.Contains(got, "Hidden Council") {
		t.Errorf("Safe mode leaked dm_only event:\n%s", got)
	}
}

func TestRenderSessions_GMNotesGated(t *testing.T) {
	ctx := context.Background()
	list := []sessions.Session{
		{ID: "s1", Name: "Into the Glasswater Maze", Status: sessions.StatusPlanned,
			Summary:   sp("Cross into the maze."),
			RecapHTML: sp("<p>Recap body</p>"),
			NotesHTML: sp(polluted),
			ScheduledDate: sp("2026-06-12"),
		},
	}
	attendees := map[string][]sessions.Attendee{
		"s1": {{UserID: "u1", DisplayName: "Anna", Status: "accepted"}},
	}
	linked := map[string][]sessions.SessionEntity{
		"s1": {{EntityID: "e1", EntityName: "Lyra Vance", Role: "key"}},
	}

	t.Run("Safe mode excludes GM notes", func(t *testing.T) {
		got, err := RenderSessions(ctx, list, attendees, linked,
			Options{Privacy: PrivacyModeSafe, IncludeSessionGMNotes: true})
		if err != nil {
			t.Fatalf("RenderSessions: %v", err)
		}
		assertClean(t, "RenderSessions/safe", got)
		if strings.Contains(got, "GM notes") {
			t.Errorf("Safe mode included GM notes header:\n%s", got)
		}
		if !strings.Contains(got, "[Lyra Vance](#lyra-vance)") {
			t.Errorf("expected linked-entity wikilink:\n%s", got)
		}
	})

	t.Run("Permitted + opt-in renders GM notes (scripts stripped)", func(t *testing.T) {
		got, err := RenderSessions(ctx, list, attendees, linked,
			Options{Privacy: PrivacyModePermitted, IncludeSessionGMNotes: true})
		if err != nil {
			t.Fatalf("RenderSessions: %v", err)
		}
		assertClean(t, "RenderSessions/permitted", got)
		if !strings.Contains(got, "GM notes") {
			t.Errorf("Permitted+opt-in dropped GM notes header:\n%s", got)
		}
	})
}

func TestRenderTimelines_GroupsAndFilters(t *testing.T) {
	ctx := context.Background()
	tls := []timeline.Timeline{
		{ID: "tl1", Name: "Coral Court — Rise & Fall", DescriptionHTML: sp(polluted)},
		{ID: "tl2", Name: "DM-only Timeline", Visibility: "dm_only"},
	}
	eventsByTimeline := map[string][]timeline.EventLink{
		"tl1": {
			{ID: 1, TimelineID: "tl1", EventName: "Coral Crowning",
				EventYear: 1102, EventMonth: 1, EventDay: 1,
				EventDescription: sp("First crowning.")},
		},
		"tl2": {
			{ID: 2, TimelineID: "tl2", EventName: "Secret",
				EventYear: 1, EventMonth: 1, EventDay: 1},
		},
	}
	got, err := RenderTimelines(ctx, tls, eventsByTimeline, Options{Privacy: PrivacyModeSafe})
	if err != nil {
		t.Fatalf("RenderTimelines: %v", err)
	}
	assertClean(t, "RenderTimelines", got)
	if !strings.Contains(got, "Coral Court — Rise & Fall") {
		t.Errorf("expected timeline heading:\n%s", got)
	}
	if !strings.Contains(got, "Coral Crowning") {
		t.Errorf("expected event under timeline:\n%s", got)
	}
	if strings.Contains(got, "DM-only Timeline") {
		t.Errorf("Safe mode leaked dm_only timeline:\n%s", got)
	}
}

func TestRenderHeader_ContainsTokenPlaceholderAndPrivacy(t *testing.T) {
	got := RenderHeader("Ashfall", time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		Options{Privacy: PrivacyModeSafe})
	for _, want := range []string{
		"# Ashfall — AI Export",
		"Privacy mode: safe",
		"**Estimated tokens:** ~__TOKEN_COUNT__",
		"intentionally lossy markdown export",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderHeader missing %q:\n%s", want, got)
		}
	}
}

func TestSubstituteTokenCount(t *testing.T) {
	in := "**Estimated tokens:** ~__TOKEN_COUNT__\n"
	got := substituteTokenCount(in, 12400)
	want := "**Estimated tokens:** ~12,400\n"
	if got != want {
		t.Errorf("substituteTokenCount = %q, want %q", got, want)
	}
}

func TestFormatTokens(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"}, {1, "1"}, {999, "999"}, {1000, "1,000"},
		{12400, "12,400"}, {1234567, "1,234,567"},
	}
	for _, c := range cases {
		if got := formatTokens(c.in); got != c.want {
			t.Errorf("formatTokens(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSlugify(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Lyra Vance", "lyra-vance"},
		{"The Captain's Pact", "the-captains-pact"},
		{"Coral Court — Rise & Fall", "coral-court-rise-fall"},
		{"  spaces  ", "spaces"},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestOptions_EnabledCategoriesDedupAndOrder pins the orchestrator's
// guarantee: enabling [notes, entities, notes] yields the canonical
// order [entities, notes] (entities first so the wikilink resolver
// can be referenced by later sections in v2).
func TestOptions_EnabledCategoriesDedupAndOrder(t *testing.T) {
	opts := Options{Categories: []Category{CategoryNotes, CategoryEntities, CategoryNotes}}
	got := opts.EnabledCategories()
	want := []Category{CategoryEntities, CategoryNotes}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i], want[i])
		}
	}
}

// TestEstimateTokens_RoughHeuristic verifies the 4 chars/token
// approximation doesn't drift wildly. Exact value isn't load-bearing;
// "approximately right" is what the UI needs.
func TestEstimateTokens_RoughHeuristic(t *testing.T) {
	if estimateTokens("") != 0 {
		t.Errorf("empty → expected 0")
	}
	// "hello world" is 11 bytes → ceil(11/4) = 3
	if got := estimateTokens("hello world"); got != 3 {
		t.Errorf("11-byte input → got %d, want 3", got)
	}
}

// jsonRoundtrip is a smoke helper — confirms the rendered markdown
// survives JSON encoding (the PR-B handler returns the markdown in
// a JSON response payload so the modal can paste it).
func TestRenderEntities_MarkdownJSONSafe(t *testing.T) {
	got, err := RenderEntities(context.Background(),
		[]entities.Entity{{ID: "e1", Name: "X", EntityTypeID: 1, EntryHTML: sp("<p>hi</p>")}},
		[]entities.EntityType{{ID: 1, NamePlural: "Things"}},
		nil, nil, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	body, err := json.Marshal(map[string]string{"markdown": got})
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	// Round-trip
	var out map[string]string
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if out["markdown"] != got {
		t.Errorf("JSON round-trip drifted")
	}
}
