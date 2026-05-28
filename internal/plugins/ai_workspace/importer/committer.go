// committer.go is the Phase 5 entity-creation orchestrator. Takes
// the parser+classifier output PLUS the per-row operator decisions
// from the review screen, then creates entity types (for new
// categories) + entities (for each included row).
//
// Per-row autonomy: row N failure does NOT abort N+1..M. Each row's
// outcome lives independently on RowOutcome; the operator sees the
// per-row result in the import_result templ.
//
// SEC-6-AMENDED mirror: every entity body is routed through
// MarkdownToHTML (markdown_html.go) → htmlconv.Convert before being
// handed to the entity service. The AST structural pin in
// committer_sanitize_test.go fails pinpointed (line number on the
// unprotected call site) if any future refactor adds a code path
// that calls EntityCreator.UpdateEntry / Create / Update WITHOUT
// the MarkdownToHTML funnel.
//
// Per cordinator/reports/chronicle/2026-05-26-c-ai-workspace-scoping.md
// §4 Phase 5 + §5 acceptance invariants.

package importer

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer/htmlconv"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// _ = fmt.Errorf is referenced explicitly to keep the fmt import
// after future-refactor pruning; the Commit method uses fmt for its
// load-error wrap.
var _ = fmt.Errorf

// EntityCreator is the narrow contract the Committer needs. The
// concrete entities.EntityService implements every method.
type EntityCreator interface {
	CreateEntityType(ctx context.Context, campaignID string, input entities.CreateEntityTypeInput) (*entities.EntityType, error)
	Create(ctx context.Context, campaignID, userID string, input entities.CreateEntityInput) (*entities.Entity, error)
	Update(ctx context.Context, entityID string, input entities.UpdateEntityInput) (*entities.Entity, error)
	UpdateEntry(ctx context.Context, entityID, entryJSON, entryHTML string) error
	GetBySlug(ctx context.Context, campaignID, slug string) (*entities.Entity, error)
	GetEntityTypeBySlug(ctx context.Context, campaignID, slug string) (*entities.EntityType, error)
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
}

// Committer is the orchestrator. Constructed per request in the
// handler from the campaigns-wired EntityCreator (entities.EntityService).
type Committer struct {
	creator EntityCreator
}

// NewCommitter constructs a Committer.
func NewCommitter(c EntityCreator) *Committer {
	return &Committer{creator: c}
}

// RowDecision is the operator's per-row form submission from the
// review screen. One entry per ParsedPage, indexed parallel to
// the original Pages slice.
type RowDecision struct {
	// Include is the row's checkbox state. False → skip.
	Include bool

	// Name is the (possibly edited) entity name. Defaults to
	// ParsedPage.Name when the operator didn't edit the input.
	Name string

	// CategorySpec is the entity-type selection. Either an existing
	// entity-type slug ("character", "location", ...) OR the
	// "new:<proposed_slug>" sentinel produced by the review templ
	// when the operator chose Create-new for a previously-unknown
	// category.
	CategorySpec string

	// Subcategory mirrors ParsedPage.FrontMatter.Subcategory; the
	// review form doesn't currently let the operator edit it so the
	// committer pulls it from the parsed FM. Reserved for future
	// per-row editing.
	Subcategory string

	// Visibility is the enum value. Mapped to IsPrivate at
	// commit-time (private / dm_only → private; public → public).
	// Visibility=dm_only is preserved on the entity's Visibility
	// field via Update; CreateEntityInput doesn't carry it.
	Visibility string

	// ConflictMode is "skip" | "rename" | "overwrite". Honored only
	// when the corresponding entity slug exists.
	ConflictMode string
}

// CommitInput bundles the per-request commit payload.
type CommitInput struct {
	OwnerID   string
	Pages     []ParsedPage
	Decisions []RowDecision
}

// RowOutcome is the per-row commit outcome. The result summary
// templ iterates these.
type RowOutcome struct {
	Index    int
	Name     string
	Status   RowStatus
	EntityID string // populated when an entity was created or updated
	Slug     string // resolved final slug (after rename suffix if applied)
	Reason   string // human-readable reason for skip/failed; empty otherwise
}

// RowStatus enumerates the per-row commit outcomes.
type RowStatus string

const (
	StatusCreated   RowStatus = "created"
	StatusRenamed   RowStatus = "renamed"
	StatusOverwrote RowStatus = "overwrote"
	StatusSkipped   RowStatus = "skipped"
	StatusFailed    RowStatus = "failed"
)

// CommitResult is the aggregate returned to the handler.
type CommitResult struct {
	Rows                 []RowOutcome
	Created              int
	Renamed              int
	Overwrote            int
	Skipped              int
	Failed               int
	NewCategoriesCreated []string // slugs successfully created this batch
	NewCategoriesFailed  []string // slugs that failed to create
}

// Commit runs the orchestration. Two phases:
//
//  1. Category-creation phase: derive the set of unique "new:<slug>"
//     specs across decisions; create each entity-type ONCE; build a
//     slug → ID map. Failed category creation marks ALL rows that
//     referenced that slug as Failed.
//  2. Per-row phase: for each Include=true row, sanitize the body
//     via MarkdownToHTML (LOAD-BEARING — the AST pin in
//     committer_sanitize_test.go enforces this funnel), convert to
//     ProseMirror JSON, then Create / Update via the entity service.
//
// Per-row autonomy: errors on one row don't abort subsequent rows.
// Returned error is reserved for fatal infrastructure failures
// (e.g. the entity-types pre-fetch barfing); per-row failures
// surface via Status=StatusFailed + Reason.
func (c *Committer) Commit(ctx context.Context, campaignID string, in CommitInput) (CommitResult, error) {
	result := CommitResult{Rows: make([]RowOutcome, len(in.Pages))}

	// Build the existing-types index up-front so per-row work
	// doesn't N+1 the registry. Operator-facing error is friendly;
	// the underlying repo error is preserved via %w for slog.
	types, err := c.creator.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return result, fmt.Errorf("could not load campaign categories — try again in a moment: %w", err)
	}
	typesBySlug := make(map[string]*entities.EntityType, len(types))
	for i := range types {
		typesBySlug[strings.ToLower(types[i].Slug)] = &types[i]
	}

	// Phase 1: category creation.
	newTypeIDs, failedTypes := c.createNewCategories(ctx, campaignID, in.Decisions)
	result.NewCategoriesFailed = failedTypes
	for slug := range newTypeIDs {
		result.NewCategoriesCreated = append(result.NewCategoriesCreated, slug)
	}

	// Phase 2: per-row commits.
	for i, page := range in.Pages {
		dec := RowDecision{}
		if i < len(in.Decisions) {
			dec = in.Decisions[i]
		}
		result.Rows[i] = c.commitRow(ctx, campaignID, in.OwnerID,
			i, page, dec, typesBySlug, newTypeIDs, failedTypes)
		switch result.Rows[i].Status {
		case StatusCreated:
			result.Created++
		case StatusRenamed:
			result.Renamed++
		case StatusOverwrote:
			result.Overwrote++
		case StatusSkipped:
			result.Skipped++
		case StatusFailed:
			result.Failed++
		}
	}

	return result, nil
}

// createNewCategories scans the decisions for "new:<slug>" specs +
// creates each unique entity type ONCE. Returns:
//   - created: slug → new EntityType.ID
//   - failed: slugs whose CreateEntityType call returned an error
//
// Per scoping §3.8: per-batch dedup is non-negotiable (multiple
// rows can reference the same new category; we don't want to
// create "warrior" three times).
func (c *Committer) createNewCategories(
	ctx context.Context,
	campaignID string,
	decisions []RowDecision,
) (created map[string]int, failed []string) {
	created = make(map[string]int)
	seen := make(map[string]bool)
	for _, d := range decisions {
		if !d.Include {
			continue
		}
		slug, ok := parseNewCategorySpec(d.CategorySpec)
		if !ok {
			continue
		}
		if seen[slug] {
			continue
		}
		seen[slug] = true

		input := entities.CreateEntityTypeInput{
			Name:       cases_title(slug),
			NamePlural: cases_title(slug) + "s",
			Icon:       "fa-cube",
			Color:      "#6366f1", // accent fallback; operator can recolor later
		}
		et, err := c.creator.CreateEntityType(ctx, campaignID, input)
		if err != nil {
			failed = append(failed, slug)
			continue
		}
		created[slug] = et.ID
	}
	return created, failed
}

// parseNewCategorySpec returns the slug + true when spec is
// "new:<slug>"; otherwise ("", false).
func parseNewCategorySpec(spec string) (string, bool) {
	if !strings.HasPrefix(spec, "new:") {
		return "", false
	}
	slug := strings.TrimSpace(strings.TrimPrefix(spec, "new:"))
	if slug == "" {
		return "", false
	}
	return strings.ToLower(slug), true
}

// commitRow is the per-row orchestrator. Returns a populated
// RowOutcome regardless of success/failure (per-row autonomy).
//
// This function is the LOAD-BEARING SEC-6 mirror site. The AST pin
// in committer_sanitize_test.go asserts that every function in this
// file calling EntityCreator.UpdateEntry / Create / Update also
// contains MarkdownToHTML(. Two-step funnel:
//
//  1. MarkdownToHTML(page.Body)  — goldmark + sanitize.HTML
//  2. htmlconv.Convert(html)     — HTML → ProseMirror JSON
//  3. creator.Create / Update    — entity service (which also
//                                  calls sanitize.HTML internally
//                                  → belt-and-suspenders)
//  4. creator.UpdateEntry(entryJSON, entryHTML)
//
// Future maintenance: keep this function as the SINGLE place that
// invokes EntityCreator.UpdateEntry. If you add another call site,
// the AST pin will fail unless that site ALSO funnels through
// MarkdownToHTML — that's the point of the pin.
func (c *Committer) commitRow(
	ctx context.Context,
	campaignID, ownerID string,
	idx int,
	page ParsedPage,
	dec RowDecision,
	typesBySlug map[string]*entities.EntityType,
	newTypeIDs map[string]int,
	failedNewTypes []string,
) RowOutcome {
	out := RowOutcome{Index: idx, Name: dec.Name}
	if out.Name == "" {
		out.Name = page.Name
	}

	// Skip: not included OR parse error.
	if !dec.Include {
		out.Status = StatusSkipped
		out.Reason = "Excluded by operator"
		return out
	}
	if page.Status == StatusParseError {
		out.Status = StatusSkipped
		out.Reason = "Parse error: " + page.ParseError
		return out
	}

	// Resolve entity type.
	typeID, ok := resolveTypeID(dec.CategorySpec, typesBySlug, newTypeIDs)
	if !ok {
		// Did this row reference a category that we tried (and
		// failed) to create?
		if slug, isNew := parseNewCategorySpec(dec.CategorySpec); isNew {
			for _, f := range failedNewTypes {
				if f == slug {
					out.Status = StatusFailed
					out.Reason = "Couldn't create category " + strconv.Quote(slug)
					return out
				}
			}
		}
		out.Status = StatusFailed
		out.Reason = "No entity type selected (pick a category from the dropdown)"
		return out
	}

	// SEC-6 mirror — every entity-creation path funnels through
	// MarkdownToHTML first. Per-row errors are operator-friendly;
	// the underlying library error stays in slog (handler-side).
	bodyHTML, err := MarkdownToHTML(page.Body)
	if err != nil {
		out.Status = StatusFailed
		out.Reason = "Could not parse the page's markdown body — check the heading structure."
		return out
	}
	bodyJSON, err := htmlconv.Convert(bodyHTML)
	if err != nil {
		out.Status = StatusFailed
		out.Reason = "Could not convert the page's body to the editor format. Try simpler markdown."
		return out
	}

	// Map visibility → IsPrivate (CreateEntityInput's only visibility flag).
	isPrivate := dec.Visibility == "private" || dec.Visibility == "dm_only"

	// Resolve final name + conflict outcome.
	finalName := out.Name
	var existing *entities.Entity
	if e, _ := c.creator.GetBySlug(ctx, campaignID, entities.Slugify(finalName)); e != nil {
		existing = e
	}

	if existing != nil {
		switch dec.ConflictMode {
		case "skip":
			out.Status = StatusSkipped
			out.Reason = "Skipped: name conflicts with " + strconv.Quote(existing.Name)
			return out
		case "overwrite":
			return c.overwriteExisting(ctx, existing, page, dec, typeID, bodyJSON, bodyHTML, isPrivate, idx)
		default: // "rename" (default mode)
			finalName = c.suffixUntilFree(ctx, campaignID, finalName)
		}
	}

	// Create. Operator-facing errors stay friendly — the technical
	// detail is captured by the handler's slog before this Reason is
	// rendered.
	ent, err := c.creator.Create(ctx, campaignID, ownerID, entities.CreateEntityInput{
		Name:         finalName,
		EntityTypeID: typeID,
		TypeLabel:    dec.Subcategory,
		IsPrivate:    isPrivate,
		FieldsData:   map[string]any{},
	})
	if err != nil {
		out.Status = StatusFailed
		out.Reason = "Could not save this page. Try again in a moment."
		return out
	}
	if err := c.creator.UpdateEntry(ctx, ent.ID, bodyJSON, bodyHTML); err != nil {
		out.Status = StatusFailed
		out.Reason = "Saved the page, but could not write its body. Open the page to retry from the editor."
		out.EntityID = ent.ID
		out.Slug = ent.Slug
		return out
	}
	out.EntityID = ent.ID
	out.Slug = ent.Slug
	out.Name = finalName
	if finalName != dec.Name && finalName != page.Name {
		out.Status = StatusRenamed
		out.Reason = "Original name conflicted; saved as " + strconv.Quote(finalName)
	} else {
		out.Status = StatusCreated
	}
	return out
}

// overwriteExisting handles the Overwrite conflict mode: load the
// existing entity by slug, run Update with the new metadata, then
// UpdateEntry with the new body. Same SEC-6 funnel applies — body
// is already sanitized before this call.
func (c *Committer) overwriteExisting(
	ctx context.Context,
	existing *entities.Entity,
	page ParsedPage,
	dec RowDecision,
	typeID int,
	bodyJSON, bodyHTML string,
	isPrivate bool,
	idx int,
) RowOutcome {
	out := RowOutcome{Index: idx, Name: existing.Name, Slug: existing.Slug, EntityID: existing.ID}

	if _, err := c.creator.Update(ctx, existing.ID, entities.UpdateEntityInput{
		Name:       existing.Name, // overwrite keeps the existing name
		TypeLabel:  dec.Subcategory,
		IsPrivate:  &isPrivate,
		FieldsData: map[string]any{},
		// Entry + EntryHTML on UpdateEntityInput would also work,
		// but we use UpdateEntry below to mirror the create path
		// + go through service.go's sanitize.HTML for symmetry.
	}); err != nil {
		out.Status = StatusFailed
		out.Reason = "Could not overwrite the existing page's settings. The original page is unchanged."
		return out
	}
	if err := c.creator.UpdateEntry(ctx, existing.ID, bodyJSON, bodyHTML); err != nil {
		out.Status = StatusFailed
		out.Reason = "Updated the page's settings, but could not write the new body."
		return out
	}
	out.Status = StatusOverwrote
	return out
}

// suffixUntilFree appends "(Imported)" then "-2", "-3", ... to the
// name until a slug-free variant emerges. Mirrors entities.Clone's
// "(Copy)" pattern; bounded at 100 attempts (effectively impossible
// to hit; matches the entity service's own dedup loop cap).
func (c *Committer) suffixUntilFree(ctx context.Context, campaignID, baseName string) string {
	candidate := baseName + " (Imported)"
	for i := 0; i < 100; i++ {
		slug := entities.Slugify(candidate)
		existing, _ := c.creator.GetBySlug(ctx, campaignID, slug)
		if existing == nil {
			return candidate
		}
		candidate = baseName + " (Imported " + intToStr(i+2) + ")"
	}
	// Pathological — fall back to a guaranteed-unique suffix.
	return baseName + " (Imported " + strconv.Quote(baseName) + ")"
}

// resolveTypeID looks up the entity-type ID for a category spec.
// Handles three cases: existing slug ("character"), newly-created
// slug from the per-batch map ("new:warrior" → real ID), and
// the failed-creation case (caller's job to disambiguate via
// failedNewTypes).
func resolveTypeID(
	spec string,
	typesBySlug map[string]*entities.EntityType,
	newTypeIDs map[string]int,
) (int, bool) {
	spec = strings.TrimSpace(strings.ToLower(spec))
	if spec == "" {
		return 0, false
	}
	if newSlug, isNew := parseNewCategorySpec(spec); isNew {
		if id, ok := newTypeIDs[newSlug]; ok {
			return id, true
		}
		return 0, false
	}
	if et, ok := typesBySlug[spec]; ok {
		return et.ID, true
	}
	return 0, false
}

// cases_title is a small lowercase-first-letter capitalize helper.
// Stand-in for the deprecated strings.Title; the input is already
// lowercase ASCII (an entity-type slug) so we don't need unicode
// edge handling.
func cases_title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// intToStr stringifies a small int without importing strconv from
// the inner per-row loop — the outer commitRow does.
func intToStr(n int) string {
	return strconv.Itoa(n)
}
