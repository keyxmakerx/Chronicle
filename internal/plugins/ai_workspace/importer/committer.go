// committer.go is the entity-creation orchestrator. It takes the
// parser+classifier output plus the per-row operator decisions from
// the review screen, then creates entity types (for new categories)
// and entities (for each included row).
//
// Per-row autonomy: row N failure does NOT abort N+1..M. Each row's
// outcome lives independently on RowOutcome; the operator sees the
// per-row result in the import_result templ.
//
// SEC-6: every entity body is routed through MarkdownToHTML
// (markdown_html.go) → htmlconv.Convert before being handed to the
// entity service. The AST structural pin in committer_sanitize_test.go
// fails if a future refactor adds a code path that calls
// EntityCreator.UpdateEntry / Create / Update without that funnel.

package importer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
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
	Delete(ctx context.Context, entityID string) error
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

	// Visibility is the enum value: "private" | "dm_only" | "public", or
	// "" for no per-row/bulk override. It maps to IsPrivate at commit
	// time (private / dm_only → private; public → public) and to
	// NOTHING ELSE — Entity.Visibility is a separate default/custom
	// MODE switch this plugin never touches, so dm_only and private are
	// indistinguishable once committed. On a CREATE, "" defers to
	// CommitInput.CampaignDefaultPrivate (issue #729) — the review
	// screen's "Campaign default" option, or a row whose front matter
	// carried no `visibility:` key. On an UPDATE this value is only
	// honored when the markdown carried an explicit `visibility:` key
	// — see commitUpdate.
	Visibility string

	// ConflictMode is "skip" | "rename" | "update". Honored only when
	// the action is "create" (or empty) AND the corresponding entity
	// slug exists in the campaign. Ignored for action=update /
	// action=delete (those actions encode operator intent directly).
	// "overwrite" is accepted as a backward-compat alias for "update".
	ConflictMode string

	// Action is the operator-confirmed action verb for this row,
	// normally inherited from the AI's front-matter `action:` field
	// but operator-overridable in the review screen. Empty defaults
	// to "create". Values: "create" | "update" | "delete".
	Action string

	// DeleteConfirmed is the per-row confirmation flag for
	// Action=delete rows. The review screen gates Submit on this
	// being true for every Delete row; the committer treats a Delete
	// row without it as StatusFailed (belt-and-suspenders against a
	// client-side bypass of the gate).
	DeleteConfirmed bool
}

// CommitInput bundles the per-request commit payload.
type CommitInput struct {
	OwnerID   string
	Pages     []ParsedPage
	Decisions []RowDecision

	// CampaignDefaultPrivate is the campaign's
	// CampaignSettings.DefaultsToPrivate() value, resolved by the
	// handler before calling Commit. A create row with no per-row/bulk
	// visibility override (RowDecision.Visibility == "") follows this,
	// matching every other entity-creation path
	// (CampaignSettings.ResolveNewEntityPrivacy) instead of the
	// importer's own hardcoded choice (#729).
	CampaignDefaultPrivate bool
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
	StatusCreated RowStatus = "created"
	StatusRenamed RowStatus = "renamed"
	StatusUpdated RowStatus = "updated"
	StatusDeleted RowStatus = "deleted"
	StatusSkipped RowStatus = "skipped"
	StatusFailed  RowStatus = "failed"
)

// CommitResult is the aggregate returned to the handler. Audit-log
// payload keys mirror these field names verbatim.
type CommitResult struct {
	Rows                 []RowOutcome
	Created              int
	Renamed              int
	Updated              int
	Deleted              int
	Skipped              int
	Failed               int
	NewCategoriesCreated []string // slugs successfully created this batch
	NewCategoriesFailed  []string // slugs that failed to create
}

// Commit runs the orchestration in two phases: (1) create each unique
// "new:<slug>" entity type once, building a slug → ID map (a failed
// category creation marks all rows referencing it as Failed); (2) for
// each included row, sanitize the body via MarkdownToHTML (see the
// SEC-6 funnel pin in committer_sanitize_test.go), convert to
// ProseMirror JSON, then Create/Update via the entity service.
//
// Per-row autonomy: one row's error doesn't abort the rest. The
// returned error is reserved for fatal infrastructure failures;
// per-row failures surface via Status=StatusFailed + Reason.
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

	// Phase 2: per-row commits. made maps a page's name slug to the entity
	// this commit created or updated for it, so a later row's `parent:`
	// finds a page from the same paste even when it was saved under a
	// renamed slug.
	made := map[string]string{}
	for i, page := range in.Pages {
		dec := RowDecision{}
		if i < len(in.Decisions) {
			dec = in.Decisions[i]
		}
		result.Rows[i] = c.commitRow(ctx, campaignID, in.OwnerID,
			i, page, dec, typesBySlug, newTypeIDs, failedTypes, in.CampaignDefaultPrivate, made)
		if id := result.Rows[i].EntityID; id != "" && (result.Rows[i].Status == StatusCreated ||
			result.Rows[i].Status == StatusRenamed || result.Rows[i].Status == StatusUpdated) {
			made[entities.Slugify(page.Name)] = id
			made[entities.Slugify(result.Rows[i].Name)] = id
		}
		switch result.Rows[i].Status {
		case StatusCreated:
			result.Created++
		case StatusRenamed:
			result.Renamed++
		case StatusUpdated:
			result.Updated++
		case StatusDeleted:
			result.Deleted++
		case StatusSkipped:
			result.Skipped++
		case StatusFailed:
			result.Failed++
		}
	}

	return result, nil
}

// createNewCategories scans the decisions for "new:<slug>" specs and
// creates each unique entity type ONCE per batch (multiple rows can
// reference the same new category). Returns:
//   - created: slug → new EntityType.ID
//   - failed: slugs whose CreateEntityType call returned an error
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
// LOAD-BEARING: committer_sanitize_test.go's AST pin asserts every
// function in this file calling EntityCreator.UpdateEntry / Create /
// Update also calls MarkdownToHTML first (goldmark + sanitize.HTML),
// then htmlconv.Convert to ProseMirror JSON, before handing off to
// the entity service. Any new call site must follow the same funnel
// or the pin fails.
func (c *Committer) commitRow(
	ctx context.Context,
	campaignID, ownerID string,
	idx int,
	page ParsedPage,
	dec RowDecision,
	typesBySlug map[string]*entities.EntityType,
	newTypeIDs map[string]int,
	failedNewTypes []string,
	campaignDefaultPrivate bool,
	made map[string]string,
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

	// Action dispatch; empty Action defaults to "create".
	switch dec.Action {
	case ActionDelete:
		return c.commitDelete(ctx, campaignID, page, dec, idx)
	case ActionUpdate:
		return c.commitUpdateExplicit(ctx, campaignID, page, dec, typesBySlug, newTypeIDs, failedNewTypes, idx, made)
	}
	// Fall-through: ActionCreate (or empty).

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
	// "" (no per-row/bulk override) follows the campaign default, same as
	// every other creation path's ResolveNewEntityPrivacy merge (#729) —
	// this importer must not hardcode a choice the operator never made.
	var isPrivate bool
	switch dec.Visibility {
	case "private", "dm_only":
		isPrivate = true
	case "public":
		isPrivate = false
	default:
		isPrivate = campaignDefaultPrivate
	}

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
		case "update", "overwrite": // "overwrite" is a back-compat alias for "update"
			parent, _, reason := c.resolveParent(ctx, campaignID, page, made)
			if reason != "" {
				out.Status = StatusFailed
				out.Reason = reason
				return out
			}
			return c.commitUpdate(ctx, existing, page, dec, typeID, bodyJSON, bodyHTML, isPrivate, parent, idx)
		default: // "rename" (default mode)
			finalName = c.suffixUntilFree(ctx, campaignID, finalName)
		}
	}

	_, parentID, reason := c.resolveParent(ctx, campaignID, page, made)
	if reason != "" {
		out.Status = StatusFailed
		out.Reason = reason
		return out
	}

	// Create. Operator-facing errors stay friendly — the technical
	// detail is captured by the handler's slog before this Reason is
	// rendered.
	ent, err := c.creator.Create(ctx, campaignID, ownerID, entities.CreateEntityInput{
		Name:         finalName,
		EntityTypeID: typeID,
		TypeLabel:    dec.Subcategory,
		IsPrivate:    isPrivate,
		ParentID:     parentID,
		FieldsData:   map[string]any{},
	})
	if err != nil {
		out.Status = StatusFailed
		out.Reason = "Could not save this page. Try again in a moment."
		if parentID != "" {
			out.Reason = parentReason(err, out.Reason)
		}
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

// commitUpdate handles the Update conflict mode + the action=update
// path: load the existing entity by slug, run Update with the new
// metadata, then UpdateEntry with the new body. Body must already be
// sanitized by the caller — committer_sanitize_test.go's AST pin
// exempts this function on that basis.
func (c *Committer) commitUpdate(
	ctx context.Context,
	existing *entities.Entity,
	page ParsedPage,
	dec RowDecision,
	typeID int,
	bodyJSON, bodyHTML string,
	isPrivate bool,
	parent patch.Field[string],
	idx int,
) RowOutcome {
	out := RowOutcome{Index: idx, Name: existing.Name, Slug: existing.Slug, EntityID: existing.ID}

	// ParentID, IsPrivate, TypeLabel and FieldsData are absent unless
	// explicitly set, meaning "preserve": an update must not flatten
	// an existing page out of the hierarchy or reset its state. The
	// review row's Visibility dropdown is pre-filled from the
	// INCOMING front matter, not the entity, so leaving it alone is
	// not consent to change it — only an explicit `visibility:` key
	// in the markdown changes IsPrivate. The markdown here is pasted
	// back from an external AI tool that read campaign text a player
	// may have written; re-deciding privacy from that text is how a
	// hidden page gets published by an unrelated commit.
	var isPrivateField *bool
	if page.FrontMatter.Visibility != "" {
		isPrivateField = &isPrivate
	}

	// TypeLabel: absent preserves, present-but-empty CLEARS. patch.Of("") is
	// the clear, so a page whose front matter omits `subcategory:` used to
	// erase the descriptor off the entity it was updating.
	typeLabel := patch.Absent[string]()
	if dec.Subcategory != "" {
		typeLabel = patch.Of(dec.Subcategory)
	}

	if _, err := c.creator.Update(ctx, existing.ID, entities.UpdateEntityInput{
		Name:      patch.Of(existing.Name), // update keeps the existing name
		TypeLabel: typeLabel,
		IsPrivate: isPrivateField,
		// Absent keeps the page where it is; the service validates a
		// present parent (same campaign, no cycle).
		ParentID: parent,
		// FieldsData is ABSENT (nil), never an empty map: the service
		// replaces all fields on any non-nil map, and the importer has
		// no source of structured field data to replace them with.
	}); err != nil {
		out.Status = StatusFailed
		out.Reason = parentReason(err, "Could not update the existing page's settings. The original page is unchanged.")
		return out
	}
	if err := c.creator.UpdateEntry(ctx, existing.ID, bodyJSON, bodyHTML); err != nil {
		out.Status = StatusFailed
		out.Reason = "Updated the page's settings, but could not write the new body."
		return out
	}
	out.Status = StatusUpdated
	return out
}

// commitUpdateExplicit handles action=update — the AI explicitly
// requested an update via front-matter. The target must exist; the
// classifier already filters ActionMismatch rows, but this re-checks
// defensively against a concurrent delete. Sanitizes via
// MarkdownToHTML before delegating to commitUpdate, satisfying the
// SEC-6 funnel pin in committer_sanitize_test.go.
func (c *Committer) commitUpdateExplicit(
	ctx context.Context,
	campaignID string,
	page ParsedPage,
	dec RowDecision,
	typesBySlug map[string]*entities.EntityType,
	newTypeIDs map[string]int,
	failedNewTypes []string,
	idx int,
	made map[string]string,
) RowOutcome {
	out := RowOutcome{Index: idx, Name: dec.Name}
	if out.Name == "" {
		out.Name = page.Name
	}

	// Find the target. Classifier already validated this for the
	// AI-suggested action=update path; the re-check handles the
	// (rare) case of concurrent deletion between classify + commit.
	existing, _ := c.creator.GetBySlug(ctx, campaignID, entities.Slugify(out.Name))
	if existing == nil {
		out.Status = StatusFailed
		out.Reason = "Cannot update: no existing entity named " + strconv.Quote(out.Name) + " in this campaign."
		return out
	}

	// Type resolution — operator may have changed the category on a
	// per-row override; falls back to existing type if empty.
	typeID, ok := resolveTypeID(dec.CategorySpec, typesBySlug, newTypeIDs)
	if !ok {
		if slug, isNew := parseNewCategorySpec(dec.CategorySpec); isNew {
			for _, f := range failedNewTypes {
				if f == slug {
					out.Status = StatusFailed
					out.Reason = "Couldn't create category " + strconv.Quote(slug)
					return out
				}
			}
		}
		// For action=update the operator may legitimately leave
		// CategorySpec empty (no change to type); preserve existing.
		typeID = 0
	}

	// SEC-6 funnel — sanitize body markdown BEFORE handing to
	// commitUpdate. The pin verifies this funnel exists in every
	// non-exempt function calling creator.Update / UpdateEntry.
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

	parent, _, reason := c.resolveParent(ctx, campaignID, page, made)
	if reason != "" {
		out.Status = StatusFailed
		out.Reason = reason
		return out
	}
	isPrivate := dec.Visibility == "private" || dec.Visibility == "dm_only"
	return c.commitUpdate(ctx, existing, page, dec, typeID, bodyJSON, bodyHTML, isPrivate, parent, idx)
}

// commitDelete handles action=delete: load the existing entity by
// slug, verify the DeleteConfirmed gate (belt-and-suspenders against
// a client-side bypass of the review-screen confirmation checkbox),
// and call entities.EntityService.Delete. No body sanitization is
// needed since Delete isn't in the SEC-6 pin's mutators map.
//
// Hard- vs soft-delete is delegated to the entities plugin.
func (c *Committer) commitDelete(
	ctx context.Context,
	campaignID string,
	page ParsedPage,
	dec RowDecision,
	idx int,
) RowOutcome {
	out := RowOutcome{Index: idx, Name: dec.Name}
	if out.Name == "" {
		out.Name = page.Name
	}

	if !dec.DeleteConfirmed {
		out.Status = StatusFailed
		out.Reason = "Delete not confirmed — operator must check the per-row confirmation in the review screen."
		return out
	}

	existing, _ := c.creator.GetBySlug(ctx, campaignID, entities.Slugify(out.Name))
	if existing == nil {
		out.Status = StatusFailed
		out.Reason = "Cannot delete: no existing entity named " + strconv.Quote(out.Name) + " in this campaign."
		return out
	}
	out.EntityID = existing.ID
	out.Slug = existing.Slug

	if err := c.creator.Delete(ctx, existing.ID); err != nil {
		out.Status = StatusFailed
		out.Reason = "Could not delete the page. Try again in a moment."
		return out
	}
	out.Status = StatusDeleted
	return out
}

// resolveParent turns the row's `parent:` into a patch for updates and an
// id for creates. Absent stays absent so an update never moves a page
// unasked; "none" is the explicit lift to the top level. A name is looked
// up among pages this commit already saved before the campaign, because
// the same-paste page may sit under a renamed slug. The entities service
// still owns the real checks (same campaign, no cycle) when the id is used.
func (c *Committer) resolveParent(ctx context.Context, campaignID string, page ParsedPage, made map[string]string) (patch.Field[string], string, string) {
	if page.FrontMatter.LiftsParent() {
		return patch.Null[string](), "", ""
	}
	key, ok := page.FrontMatter.ParentKey()
	if !ok {
		return patch.Absent[string](), "", ""
	}
	if key == "" || key == entities.Slugify(page.Name) {
		return patch.Absent[string](), "", "A page cannot be its own parent, and the parent needs a usable name."
	}
	if id, ok := made[key]; ok {
		return patch.Of(id), id, ""
	}
	if e, _ := c.creator.GetBySlug(ctx, campaignID, key); e != nil {
		return patch.Of(e.ID), e.ID, ""
	}
	return patch.Absent[string](), "", "The parent " + strconv.Quote(strings.TrimSpace(page.FrontMatter.Parent)) +
		" was not found or not saved, so this page was left alone. Include the parent above it, or fix the name."
}

// parentReason shows the entities service's plain refusal (circular
// parent, wrong campaign) instead of a generic failure; anything else
// keeps the fallback so no raw error reaches the review screen.
func parentReason(err error, fallback string) string {
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code == 400 && ae.Message != "" {
		return "Could not set the parent: " + ae.Message + "."
	}
	return fallback
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

// cases_title is a lowercase-first-letter capitalize helper, standing
// in for the deprecated strings.Title. Input is always a lowercase
// ASCII entity-type slug, so no unicode handling is needed.
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
