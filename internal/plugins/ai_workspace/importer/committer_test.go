// committer_test.go is the behavioral-test counterpart to the
// structural AST pin in committer_sanitize_test.go. Drives the
// Committer through every per-row outcome:
//
//   - Created (new page, no conflict)
//   - Renamed (slug conflict → "(Imported)" suffix)
//   - Overwrote (slug conflict → operator picked Overwrite)
//   - Skipped (Include=false; parse-error row)
//   - Failed (entity-type creation failed; row referenced it)
//   - New category creation (slug→ID map; dedup across rows)
//   - Per-row autonomy (row N failure doesn't abort N+1)

package importer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type fakeCreator struct {
	types         []entities.EntityType
	existing      map[string]*entities.Entity // slug → entity
	createCalls   []entities.CreateEntityInput
	updateCalls   []entities.UpdateEntityInput
	updateEntries []updateEntryCall
	// V1.5 / C-AI-WORKSPACE-V1-G: capture for Delete dispatch tests.
	deleteCalls   []string
	createTypeFn  func(input entities.CreateEntityTypeInput) (*entities.EntityType, error)
	createFn      func(input entities.CreateEntityInput) (*entities.Entity, error)
	updateFn      func(id string, input entities.UpdateEntityInput) (*entities.Entity, error)
	updateEntryFn func(id, entryJSON, entryHTML string) error
	deleteFn      func(id string) error
}

type updateEntryCall struct {
	EntityID  string
	EntryJSON string
	EntryHTML string
}

func (f *fakeCreator) GetEntityTypes(_ context.Context, _ string) ([]entities.EntityType, error) {
	return f.types, nil
}

func (f *fakeCreator) GetBySlug(_ context.Context, _, slug string) (*entities.Entity, error) {
	return f.existing[slug], nil
}

func (f *fakeCreator) GetEntityTypeBySlug(_ context.Context, _, slug string) (*entities.EntityType, error) {
	for i, t := range f.types {
		if strings.EqualFold(t.Slug, slug) {
			return &f.types[i], nil
		}
	}
	return nil, nil
}

func (f *fakeCreator) CreateEntityType(_ context.Context, _ string, input entities.CreateEntityTypeInput) (*entities.EntityType, error) {
	if f.createTypeFn != nil {
		return f.createTypeFn(input)
	}
	newID := 100 + len(f.types)
	et := entities.EntityType{
		ID: newID, Name: input.Name, NamePlural: input.NamePlural, Slug: strings.ToLower(input.Name), Enabled: true,
	}
	f.types = append(f.types, et)
	return &et, nil
}

func (f *fakeCreator) Create(_ context.Context, _, _ string, input entities.CreateEntityInput) (*entities.Entity, error) {
	f.createCalls = append(f.createCalls, input)
	if f.createFn != nil {
		return f.createFn(input)
	}
	ent := &entities.Entity{
		ID: "ent-" + strings.ReplaceAll(strings.ToLower(input.Name), " ", "-"),
		Name: input.Name,
		Slug: entities.Slugify(input.Name),
	}
	if f.existing == nil {
		f.existing = map[string]*entities.Entity{}
	}
	f.existing[ent.Slug] = ent
	return ent, nil
}

func (f *fakeCreator) Update(_ context.Context, id string, input entities.UpdateEntityInput) (*entities.Entity, error) {
	f.updateCalls = append(f.updateCalls, input)
	if f.updateFn != nil {
		return f.updateFn(id, input)
	}
	return &entities.Entity{ID: id, Name: input.Name}, nil
}

func (f *fakeCreator) UpdateEntry(_ context.Context, id, entryJSON, entryHTML string) error {
	f.updateEntries = append(f.updateEntries, updateEntryCall{EntityID: id, EntryJSON: entryJSON, EntryHTML: entryHTML})
	if f.updateEntryFn != nil {
		return f.updateEntryFn(id, entryJSON, entryHTML)
	}
	return nil
}

func (f *fakeCreator) Delete(_ context.Context, id string) error {
	f.deleteCalls = append(f.deleteCalls, id)
	if f.deleteFn != nil {
		return f.deleteFn(id)
	}
	return nil
}

// page builds a ParsedPage with sensible defaults.
func page(name, typeSlug, body string) ParsedPage {
	return ParsedPage{
		Name:           name,
		FrontMatter:    FrontMatter{Name: name, Type: typeSlug},
		HasFrontMatter: true,
		Body:           body,
		Status:         StatusNew,
	}
}

func decision(include bool, name, category, visibility, conflict string) RowDecision {
	return RowDecision{
		Include:      include,
		Name:         name,
		CategorySpec: category,
		Visibility:   visibility,
		ConflictMode: conflict,
	}
}

// TestCommit_CreatesNewEntity is the happy path. One page, no
// conflict, existing category → StatusCreated.
func TestCommit_CreatesNewEntity(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
	}
	c := NewCommitter(f)
	res, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages:   []ParsedPage{page("Lyra Vance", "character", "# Lyra\n\nBody.")},
		Decisions: []RowDecision{
			decision(true, "Lyra Vance", "character", "private", "rename"),
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Created != 1 || res.Rows[0].Status != StatusCreated {
		t.Errorf("expected 1 created; got %+v", res)
	}
	if len(f.createCalls) != 1 {
		t.Fatalf("expected 1 create call; got %d", len(f.createCalls))
	}
	if f.createCalls[0].EntityTypeID != 1 {
		t.Errorf("create used type %d; want 1", f.createCalls[0].EntityTypeID)
	}
	if len(f.updateEntries) != 1 {
		t.Fatalf("expected 1 UpdateEntry call; got %d", len(f.updateEntries))
	}
	if !strings.Contains(f.updateEntries[0].EntryHTML, "<p>Body") {
		t.Errorf("EntryHTML didn't contain expected body: %q", f.updateEntries[0].EntryHTML)
	}
}

// TestCommit_StripsScriptFromBody is the SEC-6 ingress behaviour
// pin (separate from the AST structural pin). Polluted markdown
// body must NOT survive into the UpdateEntry call's entryHTML.
func TestCommit_StripsScriptFromBody(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
	}
	c := NewCommitter(f)
	body := "Body with <script>alert(1)</script> and [click](javascript:alert(1))"
	_, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages:   []ParsedPage{page("X", "character", body)},
		Decisions: []RowDecision{
			decision(true, "X", "character", "private", "rename"),
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if len(f.updateEntries) != 1 {
		t.Fatalf("expected 1 UpdateEntry; got %d", len(f.updateEntries))
	}
	got := strings.ToLower(f.updateEntries[0].EntryHTML)
	for _, bad := range []string{"<script", "javascript:"} {
		if strings.Contains(got, bad) {
			t.Errorf("entryHTML leaked %q: %s", bad, f.updateEntries[0].EntryHTML)
		}
	}
}

// TestCommit_RenameOnConflict appends "(Imported)" + slug-dedup
// loop. Mirrors entities.Clone's "(Copy)" pattern.
func TestCommit_RenameOnConflict(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		existing: map[string]*entities.Entity{
			"lyra-vance": {ID: "ent-old", Name: "Lyra Vance", Slug: "lyra-vance"},
		},
	}
	c := NewCommitter(f)
	res, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages:   []ParsedPage{page("Lyra Vance", "character", "# Lyra\n\nBody.")},
		Decisions: []RowDecision{
			decision(true, "Lyra Vance", "character", "private", "rename"),
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Renamed != 1 || res.Rows[0].Status != StatusRenamed {
		t.Errorf("expected 1 renamed; got %+v", res)
	}
	if !strings.Contains(f.createCalls[0].Name, "(Imported)") {
		t.Errorf("expected (Imported) suffix; got %q", f.createCalls[0].Name)
	}
}

// TestCommit_RenameWithCollisionLoop — when "(Imported)" is also
// taken, the loop tries "(Imported 2)", "(Imported 3)", etc.
func TestCommit_RenameWithCollisionLoop(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		existing: map[string]*entities.Entity{
			"lyra-vance":            {ID: "ent-a", Name: "Lyra Vance", Slug: "lyra-vance"},
			"lyra-vance-imported":   {ID: "ent-b", Name: "Lyra Vance (Imported)", Slug: "lyra-vance-imported"},
		},
	}
	c := NewCommitter(f)
	res, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID:   "u-1",
		Pages:     []ParsedPage{page("Lyra Vance", "character", "body")},
		Decisions: []RowDecision{decision(true, "Lyra Vance", "character", "private", "rename")},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Renamed != 1 {
		t.Errorf("expected renamed; got %+v", res)
	}
	if !strings.Contains(f.createCalls[0].Name, "Imported 2") {
		t.Errorf("expected (Imported 2) on second collision; got %q", f.createCalls[0].Name)
	}
}

// TestCommit_OverwritePreservesExistingNameAndID — Overwrite mode
// loads the existing entity by slug + runs Update keeping the
// original name. Audit semantics: the operator decided to keep the
// existing entity (not create a new one) so the existing.ID is
// what gets touched.
// TestCommit_UpdatePreservesExistingNameAndID (V1-E test, renamed in
// V1.5 along with the verb-set rename overwrite → update). The
// committer accepts both labels for one release via the backward-
// compat alias at committer.go's conflict-mode switch.
func TestCommit_UpdatePreservesExistingNameAndID(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		existing: map[string]*entities.Entity{
			"lyra-vance": {ID: "ent-original", Name: "Lyra Vance", Slug: "lyra-vance"},
		},
	}
	c := NewCommitter(f)
	res, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID:   "u-1",
		Pages:     []ParsedPage{page("Lyra Vance", "character", "# Lyra\n\nNew body.")},
		// "overwrite" form value still accepted as backward-compat
		// alias for "update"; V2 removes the alias.
		Decisions: []RowDecision{decision(true, "Lyra Vance", "character", "private", "overwrite")},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Updated != 1 || res.Rows[0].Status != StatusUpdated {
		t.Errorf("expected 1 overwrote; got %+v", res)
	}
	if res.Rows[0].EntityID != "ent-original" {
		t.Errorf("Overwrite should target existing.ID; got %q", res.Rows[0].EntityID)
	}
	if len(f.createCalls) != 0 {
		t.Errorf("Overwrite shouldn't Create; got %d Create calls", len(f.createCalls))
	}
	if len(f.updateCalls) != 1 {
		t.Errorf("expected 1 Update; got %d", len(f.updateCalls))
	}
	if len(f.updateEntries) != 1 {
		t.Errorf("expected 1 UpdateEntry; got %d", len(f.updateEntries))
	}
}

// TestCommit_SkipsConflictWhenModeSkip — operator picked skip.
func TestCommit_SkipsConflictWhenModeSkip(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		existing: map[string]*entities.Entity{
			"lyra-vance": {ID: "ent-x", Name: "Lyra Vance", Slug: "lyra-vance"},
		},
	}
	c := NewCommitter(f)
	res, _ := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID:   "u-1",
		Pages:     []ParsedPage{page("Lyra Vance", "character", "body")},
		Decisions: []RowDecision{decision(true, "Lyra Vance", "character", "private", "skip")},
	})
	if res.Skipped != 1 || res.Rows[0].Status != StatusSkipped {
		t.Errorf("expected 1 skipped; got %+v", res)
	}
	if len(f.createCalls) != 0 || len(f.updateCalls) != 0 {
		t.Errorf("skip should be a no-op against the entity service")
	}
}

// TestCommit_SkipsExcludedRow — Include=false.
func TestCommit_SkipsExcludedRow(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
	}
	c := NewCommitter(f)
	res, _ := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID:   "u-1",
		Pages:     []ParsedPage{page("X", "character", "body")},
		Decisions: []RowDecision{decision(false, "X", "character", "private", "rename")},
	})
	if res.Skipped != 1 || res.Rows[0].Reason != "Excluded by operator" {
		t.Errorf("expected explicit excluded reason; got %+v", res.Rows[0])
	}
}

// TestCommit_SkipsParseErrorRow — even if Include=true, parse
// errors carry through as skipped.
func TestCommit_SkipsParseErrorRow(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
	}
	c := NewCommitter(f)
	bad := ParsedPage{Status: StatusParseError, ParseError: "visibility: \"PUBLIC\" is not valid"}
	res, _ := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID:   "u-1",
		Pages:     []ParsedPage{bad},
		Decisions: []RowDecision{decision(true, "X", "character", "private", "rename")},
	})
	if res.Skipped != 1 || !strings.Contains(res.Rows[0].Reason, "PUBLIC") {
		t.Errorf("expected parse-error skipped with reason; got %+v", res.Rows[0])
	}
}

// TestCommit_NewCategoryCreatedOnceForDuplicateRows — per scoping
// §3.8 (per-category create-once decision). Two rows both reference
// new category "warrior" → CreateEntityType called once, both rows
// land successfully.
func TestCommit_NewCategoryCreatedOnceForDuplicateRows(t *testing.T) {
	createCalls := 0
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		createTypeFn: func(input entities.CreateEntityTypeInput) (*entities.EntityType, error) {
			createCalls++
			return &entities.EntityType{ID: 99, Name: input.Name, Slug: strings.ToLower(input.Name), Enabled: true}, nil
		},
	}
	c := NewCommitter(f)
	res, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages: []ParsedPage{
			page("Ash-Wraith", "warrior", "body1"),
			page("Bone-Wraith", "warrior", "body2"),
		},
		Decisions: []RowDecision{
			decision(true, "Ash-Wraith", "new:warrior", "private", "rename"),
			decision(true, "Bone-Wraith", "new:warrior", "private", "rename"),
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if createCalls != 1 {
		t.Errorf("CreateEntityType called %d times; want 1 (dedup)", createCalls)
	}
	if res.Created != 2 {
		t.Errorf("expected 2 entities created; got %d (%+v)", res.Created, res)
	}
	if len(res.NewCategoriesCreated) != 1 || res.NewCategoriesCreated[0] != "warrior" {
		t.Errorf("NewCategoriesCreated = %v; want [warrior]", res.NewCategoriesCreated)
	}
}

// TestCommit_FailedNewCategoryMarksReferencingRowsFailed — when
// CreateEntityType errors, every row referencing that new slug is
// marked Failed (not skipped); other rows proceed.
func TestCommit_FailedNewCategoryMarksReferencingRowsFailed(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		createTypeFn: func(_ entities.CreateEntityTypeInput) (*entities.EntityType, error) {
			return nil, errors.New("simulated DB error")
		},
	}
	c := NewCommitter(f)
	res, _ := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages: []ParsedPage{
			page("Lyra Vance", "character", "body"),  // OK
			page("Ash-Wraith", "warrior", "body"),    // needs new:warrior → fails
			page("Bone-Wraith", "warrior", "body"),   // same — also fails
		},
		Decisions: []RowDecision{
			decision(true, "Lyra Vance", "character", "private", "rename"),
			decision(true, "Ash-Wraith", "new:warrior", "private", "rename"),
			decision(true, "Bone-Wraith", "new:warrior", "private", "rename"),
		},
	})
	if res.Created != 1 {
		t.Errorf("expected 1 created (character row); got %d", res.Created)
	}
	if res.Failed != 2 {
		t.Errorf("expected 2 failed (warrior rows); got %d", res.Failed)
	}
	if len(res.NewCategoriesFailed) != 1 || res.NewCategoriesFailed[0] != "warrior" {
		t.Errorf("NewCategoriesFailed = %v; want [warrior]", res.NewCategoriesFailed)
	}
	// Per-row autonomy check: the character row landed despite the
	// warrior rows failing.
	if res.Rows[0].Status != StatusCreated {
		t.Errorf("autonomy violation: row 0 status = %q; want created", res.Rows[0].Status)
	}
}

// TestCommit_PerRowAutonomy — row 1's entity-create failure must
// NOT abort row 2 + row 3.
func TestCommit_PerRowAutonomy(t *testing.T) {
	createCount := 0
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		createFn: func(input entities.CreateEntityInput) (*entities.Entity, error) {
			createCount++
			if createCount == 1 {
				return nil, errors.New("DB barfed on row 0")
			}
			return &entities.Entity{ID: "ent-" + input.Name, Name: input.Name, Slug: entities.Slugify(input.Name)}, nil
		},
	}
	c := NewCommitter(f)
	res, _ := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages: []ParsedPage{
			page("Page A", "character", "a"),
			page("Page B", "character", "b"),
			page("Page C", "character", "c"),
		},
		Decisions: []RowDecision{
			decision(true, "Page A", "character", "private", "rename"),
			decision(true, "Page B", "character", "private", "rename"),
			decision(true, "Page C", "character", "private", "rename"),
		},
	})
	if res.Failed != 1 || res.Rows[0].Status != StatusFailed {
		t.Errorf("expected row 0 failed; got %+v", res.Rows[0])
	}
	if res.Created != 2 {
		t.Errorf("expected 2 created (rows 1+2 survive); got %d", res.Created)
	}
}

// TestCommit_UnknownCategoryButFM — page's FM type matches an
// existing slug → use it directly (operator didn't have to touch
// the category dropdown).
func TestCommit_UnknownCategoryDecisionMarksFailed(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
	}
	c := NewCommitter(f)
	res, _ := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID:   "u-1",
		Pages:     []ParsedPage{page("X", "", "body")},
		Decisions: []RowDecision{decision(true, "X", "", "private", "rename")}, // no category
	})
	if res.Failed != 1 || !strings.Contains(res.Rows[0].Reason, "category") {
		t.Errorf("expected failed-with-category-reason; got %+v", res.Rows[0])
	}
}
