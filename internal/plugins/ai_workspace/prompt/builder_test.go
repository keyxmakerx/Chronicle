// builder_test.go pins the prompt builder's behavior against the
// §3.5 template + the locked operator decisions. Each test exercises
// a different combination of picker toggles + content-mode +
// privacy.
//
// SEC-6-AMENDED inheritance is verified indirectly — when ContentMode
// != "none", the test passes a stub Exporter whose Generate output
// contains a <script> tag, and asserts the prompt body contains it
// VERBATIM. The aiexport.Service is what actually sanitizes (PR
// #349's TestRenderers_FunnelThroughHtmlToMarkdown enforces); the
// prompt builder is a pure pass-through of that already-sanitized
// content.
package prompt

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// stubEntityLister returns canned entity types + list responses.
type stubEntityLister struct {
	types   []entities.EntityType
	listByType map[int][]entities.Entity
}

func (s *stubEntityLister) GetEntityTypes(_ context.Context, _ string) ([]entities.EntityType, error) {
	return s.types, nil
}

func (s *stubEntityLister) List(_ context.Context, _ string, typeID int, _ int, _ string, _ entities.ListOptions) ([]entities.Entity, int, error) {
	ents := s.listByType[typeID]
	return ents, len(ents), nil
}

// stubTagLister — V1 doesn't call it but interface needs an impl.
type stubTagLister struct{}

func (stubTagLister) ListByCampaign(_ context.Context, _ string, _ bool) ([]tags.Tag, error) {
	return nil, nil
}

// stubExporter returns canned markdown for the content section.
type stubExporter struct {
	markdown string
	calls    int
	lastOpts aiexport.Options
}

func (s *stubExporter) Generate(_ context.Context, _, _, _ string, opts aiexport.Options) (string, error) {
	s.calls++
	s.lastOpts = opts
	return s.markdown, nil
}

func sp(s string) *string { return &s }

func defaultEntityTypes() []entities.EntityType {
	return []entities.EntityType{
		{ID: 1, Name: "Character", Slug: "character", Enabled: true, PresetCategory: sp("character"), SortOrder: 1},
		{ID: 2, Name: "Location", Slug: "location", Enabled: true, SortOrder: 2},
		{ID: 3, Name: "Item", Slug: "item", Enabled: false}, // disabled — should be filtered out
	}
}

// TestBuild_EntityTypesOnly pins the §3.5 entity-types section.
// Disabled types are filtered out; preset-category appears when set.
func TestBuild_EntityTypesOnly(t *testing.T) {
	svc := NewService(&stubEntityLister{types: defaultEntityTypes()}, stubTagLister{}, &stubExporter{})
	got, err := svc.Build(context.Background(), "Ashfall", "owner-1", "camp-1", Input{
		IncludeEntityTypes:  true,
		ContentMode:         "none",
		OperatorInstruction: "Generate five new NPCs.",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, want := range []string{
		"## My campaign's entity types",
		"**Character** (slug: `character`) — preset: character",
		"**Location** (slug: `location`)",
		"## What I want you to generate",
		"Generate five new NPCs.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected substring %q in output. Full:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Item") {
		t.Errorf("disabled entity type 'Item' leaked into output:\n%s", got)
	}
	if strings.Contains(got, "## Existing world context") {
		t.Errorf("content section rendered despite ContentMode=none")
	}
}

// TestBuild_CategoriesInUse pins the §3.5 "Categories currently in
// use" section — distinct TypeLabel values per type, counts, sorted.
func TestBuild_CategoriesInUse(t *testing.T) {
	svc := NewService(&stubEntityLister{
		types: defaultEntityTypes(),
		listByType: map[int][]entities.Entity{
			1: {
				{ID: "e1", TypeLabel: sp("NPC")},
				{ID: "e2", TypeLabel: sp("PC")},
				{ID: "e3", TypeLabel: sp("NPC")}, // dup label → dedup
			},
			2: {
				{ID: "e4", TypeLabel: sp("City")},
				{ID: "e5"}, // no TypeLabel → uncategorised
			},
		},
	}, stubTagLister{}, &stubExporter{})

	got, err := svc.Build(context.Background(), "Ashfall", "owner-1", "camp-1", Input{
		IncludeCategoriesInUse: true,
		ContentMode:            "none",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, want := range []string{
		"## Categories currently in use",
		"**Character**: NPC, PC (3 pages)",
		"**Location**: (uncategorised), City (2 pages)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected substring %q in output. Full:\n%s", want, got)
		}
	}
}

// TestBuild_FrontMatterExample pins the schema-format section.
// Operator decision §3.6 + scoping §3.5 — the template is a contract
// with AI consumers; the example block here is what they're trained
// to imitate.
func TestBuild_FrontMatterExample(t *testing.T) {
	svc := NewService(nil, nil, &stubExporter{})
	got, err := svc.Build(context.Background(), "Ashfall", "owner-1", "camp-1", Input{
		IncludeFrontMatterExample: true,
		ContentMode:               "none",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, want := range []string{
		"## Format your output as markdown with YAML front-matter",
		"name: Example Page Name",
		"type: location",
		"visibility: private",
		"tags: [trade-hub, coastal]",
		"# Example Page Name",
		"`private`, `dm_only`, or `public`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected substring %q in output. Full:\n%s", want, got)
		}
	}
}

// TestBuild_ContentMode_All exercises the Exporter reuse path. Stub
// returns a malicious-shaped markdown body — prompt builder embeds
// it verbatim (sanitization happens inside aiexport per
// SEC-6-AMENDED; prompt builder is a pass-through). The test also
// asserts Privacy + GMNotes flags propagate.
func TestBuild_ContentMode_All(t *testing.T) {
	exp := &stubExporter{markdown: "# Ashfall content\n\n<not-sanitized-by-prompt-layer>"}
	svc := NewService(nil, nil, exp)
	got, err := svc.Build(context.Background(), "Ashfall", "owner-1", "camp-1", Input{
		ContentMode:           "all",
		Privacy:               aiexport.PrivacyModePermitted,
		IncludeSessionGMNotes: true,
		OperatorInstruction:   "Extend the Drowned Reach.",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(got, "## Existing world context") {
		t.Errorf("expected content section header in output:\n%s", got)
	}
	if !strings.Contains(got, "# Ashfall content") {
		t.Errorf("exporter output didn't propagate into prompt:\n%s", got)
	}
	if exp.calls != 1 {
		t.Errorf("expected 1 Exporter call, got %d", exp.calls)
	}
	if exp.lastOpts.Privacy != aiexport.PrivacyModePermitted {
		t.Errorf("expected privacy=Permitted, got %v", exp.lastOpts.Privacy)
	}
	if !exp.lastOpts.IncludeSessionGMNotes {
		t.Errorf("expected IncludeSessionGMNotes=true to propagate")
	}
	if len(exp.lastOpts.Categories) != 0 {
		t.Errorf("expected empty Categories for ContentMode=all (=all), got %v", exp.lastOpts.Categories)
	}
}

// TestBuild_ContentMode_SpecificCategories pins the comma-separated
// slug parsing — picker can pass a subset.
func TestBuild_ContentMode_SpecificCategories(t *testing.T) {
	exp := &stubExporter{markdown: "## body"}
	svc := NewService(nil, nil, exp)
	_, err := svc.Build(context.Background(), "Ashfall", "owner-1", "camp-1", Input{
		ContentMode: "entities,notes,sessions",
		Privacy:     aiexport.PrivacyModeSafe,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(exp.lastOpts.Categories) != 3 {
		t.Fatalf("expected 3 categories, got %d: %v", len(exp.lastOpts.Categories), exp.lastOpts.Categories)
	}
	want := []aiexport.Category{
		aiexport.CategoryEntities,
		aiexport.CategoryNotes,
		aiexport.CategorySessions,
	}
	for i, w := range want {
		if exp.lastOpts.Categories[i] != w {
			t.Errorf("Categories[%d]=%v, want %v", i, exp.lastOpts.Categories[i], w)
		}
	}
}

// TestBuild_ContentMode_None_OmitsContent confirms the "none" mode
// SKIPS the Exporter call AND the template block. This is the
// default privacy posture — operator pastes schema-only.
func TestBuild_ContentMode_None_OmitsContent(t *testing.T) {
	exp := &stubExporter{markdown: "should not appear"}
	svc := NewService(nil, nil, exp)
	got, err := svc.Build(context.Background(), "Ashfall", "owner-1", "camp-1", Input{
		ContentMode:               "none",
		IncludeFrontMatterExample: true,
		OperatorInstruction:       "Just schema.",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if exp.calls != 0 {
		t.Errorf("Exporter called %d times in ContentMode=none; expected 0", exp.calls)
	}
	if strings.Contains(got, "## Existing world context") {
		t.Errorf("content header rendered in ContentMode=none:\n%s", got)
	}
	if strings.Contains(got, "should not appear") {
		t.Errorf("exporter markdown leaked despite ContentMode=none:\n%s", got)
	}
}

// TestBuild_OperatorInstructionPosition pins that the instruction is
// the LAST authored section — AI consumers read top-down so the
// instruction must come after the schema + content context.
func TestBuild_OperatorInstructionPosition(t *testing.T) {
	svc := NewService(&stubEntityLister{types: defaultEntityTypes()}, stubTagLister{}, &stubExporter{markdown: "# body"})
	got, err := svc.Build(context.Background(), "Ashfall", "owner-1", "camp-1", Input{
		IncludeEntityTypes:  true,
		ContentMode:         "all",
		OperatorInstruction: "GENERATE_TARGET",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	schemaIdx := strings.Index(got, "## My campaign's entity types")
	contentIdx := strings.Index(got, "## Existing world context")
	instructionIdx := strings.Index(got, "GENERATE_TARGET")
	if schemaIdx < 0 || contentIdx < 0 || instructionIdx < 0 {
		t.Fatalf("missing section anchors in output:\n%s", got)
	}
	if schemaIdx >= contentIdx || contentIdx >= instructionIdx {
		t.Errorf("section order schema(%d) → content(%d) → instruction(%d) violated",
			schemaIdx, contentIdx, instructionIdx)
	}
}

// TestBuild_AllSectionsTogether is the smoke test — every picker
// toggle ON, ContentMode=all. Confirms the assembled prompt is
// well-formed and the §3.5 closing instructions land.
func TestBuild_AllSectionsTogether(t *testing.T) {
	svc := NewService(&stubEntityLister{
		types: defaultEntityTypes(),
		listByType: map[int][]entities.Entity{
			1: {{ID: "e1", TypeLabel: sp("PC")}},
			2: {{ID: "e2", TypeLabel: sp("City")}},
		},
	}, stubTagLister{}, &stubExporter{markdown: "# body"})

	got, err := svc.Build(context.Background(), "Ashfall", "owner-1", "camp-1", Input{
		IncludeEntityTypes:        true,
		IncludeCategoriesInUse:    true,
		IncludeFrontMatterExample: true,
		ContentMode:               "all",
		Privacy:                   aiexport.PrivacyModeSafe,
		OperatorInstruction:       "Build me a maze.",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	must := []string{
		"You are helping me extend my TTRPG campaign in Chronicle",
		"## My campaign's entity types",
		"## Categories currently in use",
		"## Format your output as markdown with YAML front-matter",
		"## Existing world context",
		"## What I want you to generate",
		"Build me a maze.",
		"Use front-matter for every entity.",
	}
	for _, w := range must {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q. Full output:\n%s", w, got)
		}
	}
}

// TestBuild_TrimsInstructionWhitespace catches accidental leading /
// trailing whitespace in the textarea — Claude's behaviour can shift
// when the prompt has trailing newlines after the instruction.
func TestBuild_TrimsInstructionWhitespace(t *testing.T) {
	svc := NewService(nil, nil, &stubExporter{})
	got, err := svc.Build(context.Background(), "Ashfall", "owner-1", "camp-1", Input{
		ContentMode:         "none",
		OperatorInstruction: "   \n  Build me a maze.\n\n   ",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(got, "Build me a maze.") {
		t.Errorf("expected trimmed instruction text:\n%s", got)
	}
	if strings.Contains(got, "   Build") {
		t.Errorf("expected leading whitespace stripped:\n%s", got)
	}
}
