// import_review_a11y_test.go pins keyboard-only + screen-reader
// operability of the review screen's rendered HTML: the skip-link,
// landmark roles, per-control aria-labels, and that Alpine's @click
// never leaks a legacy onclick= attribute.

package ai_workspace

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// representativeReviewData builds a small ImportReviewData that
// covers every row variant — new, conflict, new-category, parse
// error — so the rendered output exercises every per-row code path.
func representativeReviewData() ImportReviewData {
	types := []entities.EntityType{
		{ID: 1, Name: "Character", Slug: "character"},
		{ID: 2, Name: "Location", Slug: "location"},
	}
	return ImportReviewData{
		CampaignID:     "camp-1",
		MarkdownSource: "---\nname: X\n---\n# X\n",
		SummaryCounts: ReviewSummary{
			Total:         4,
			Selectable:    3,
			Conflicts:     1,
			NewCategories: 1,
			ParseErrors:   1,
		},
		Pages: []importer.ParsedPage{
			{Name: "Maro Halvi", Status: importer.StatusNew, FrontMatter: importer.FrontMatter{Type: "character", Tags: []string{"scholar"}, Subcategory: "scholar"}},
			{Name: "Lyra Vance", Status: importer.StatusNew},
			{Name: "Ash-Wraith", Status: importer.StatusNew},
			{Name: "", Status: importer.StatusParseError, ParseError: "visibility: \"PUBLIC\" is not valid"},
		},
		Classes: []importer.Classification{
			{Status: importer.StatusNew, ExistingType: &types[0], AvailableTypes: types},
			{Status: importer.StatusConflict, ConflictEntity: &entities.Entity{Name: "Lyra Vance"}, AvailableTypes: types},
			{Status: importer.StatusNewCategory, IsNewCategory: true, ProposedTypeSlug: "warrior", AvailableTypes: types},
			{Status: importer.StatusParseError, AvailableTypes: types},
		},
	}
}

// TestImportReview_A11y_SkipLink — a keyboard-only operator must
// be able to jump to Submit without tabbing through every row.
func TestImportReview_A11y_SkipLink(t *testing.T) {
	out := renderReview(t, representativeReviewData())
	if !strings.Contains(out, `href="#ai-import-submit"`) {
		t.Errorf("review screen is missing the skip-link to #ai-import-submit\nout:\n%s",
			truncateA11y(out))
	}
	if !strings.Contains(out, `id="ai-import-submit"`) {
		t.Errorf("review screen is missing the Submit button's id (#ai-import-submit) — skip-link target broken")
	}
}

// TestImportReview_A11y_LandmarkRoles — the bulk-defaults bar, the
// row list, and the form itself surface as region/list landmarks
// for assistive tech navigation.
func TestImportReview_A11y_LandmarkRoles(t *testing.T) {
	out := renderReview(t, representativeReviewData())
	wantSubstrings := []string{
		`role="region"`,
		`aria-labelledby="ai-import-review-heading"`,
		`aria-labelledby="ai-import-bulk-heading"`,
		`role="list"`,
		`role="listitem"`,
		`role="radiogroup"`, // new-category control
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("review screen is missing landmark/aria attribute: %q", want)
		}
	}
}

// TestImportReview_A11y_FormControlsLabelled — every <input> and
// <select> emitted in a representative row carries either an
// aria-label or sits inside a <label> with text content. Checked by
// substring rather than an HTML parse (no x/net/html dependency).
func TestImportReview_A11y_FormControlsLabelled(t *testing.T) {
	out := renderReview(t, representativeReviewData())

	// Every aria-label expected on bulk + row controls.
	wantLabels := []string{
		"Default category to apply",
		"Default visibility for new entities",
		"Default action when a row's name conflicts",
		"name (editable)",         // per-row name input
		"Visibility for page",      // per-row visibility dropdown
		"How to handle the name conflict for page", // per-row conflict dropdown
		"Category for page",                         // existing-category dropdown
		"Create a new",                              // new-category Create radio
		"Map this page to an existing category",     // new-category Map radio
		"Existing category to map this page to",     // map-to-existing dropdown
		"Apply the bulk defaults above to every row",
		"Set every name-conflict row to Skip",
		"Answer Yes to every request except removals",
		"Answer No to every request",
		"Make this change: ", // each row's Yes / No
	}
	for _, want := range wantLabels {
		if !strings.Contains(out, want) {
			t.Errorf("review screen is missing aria-label snippet: %q", want)
		}
	}
}

// TestImportReview_A11y_NoLegacyOnclick — Alpine.js @click compiles
// to event listeners, not to the legacy `onclick=` attribute. Stray
// `onclick=` is almost always copy-paste from a non-Alpine sketch
// + skips keyboard activation; failing fast catches the regression.
func TestImportReview_A11y_NoLegacyOnclick(t *testing.T) {
	out := renderReview(t, representativeReviewData())
	if strings.Contains(out, "onclick=") {
		t.Errorf("review screen emits a legacy onclick attribute; use @click (Alpine) or hx-on:click instead\nout:\n%s",
			truncateA11y(out))
	}
}

// TestImportReview_A11y_TagAndSubcategoryChips pins that front-matter
// `tags: [scholar]` + `subcategory: scholar` surface on the row.
func TestImportReview_A11y_TagAndSubcategoryChips(t *testing.T) {
	out := renderReview(t, representativeReviewData())
	// Distinct chip icons + the chip text value. The representative
	// data sets both tags=[scholar] and subcategory=scholar on the
	// first row; the chip-icon classes (fa-tag, fa-folder-tree) are
	// stable across copy edits and unique to this chip family.
	if !strings.Contains(out, "fa-folder-tree") {
		t.Errorf("subcategory chip icon (fa-folder-tree) is missing — chip surface didn't render")
	}
	if !strings.Contains(out, "fa-tag") {
		t.Errorf("tag chip icon (fa-tag) is missing — chip surface didn't render")
	}
	if !strings.Contains(out, "scholar") {
		t.Errorf("chip body 'scholar' missing — neither subcategory nor tag value rendered\nout:\n%s",
			truncateA11y(out))
	}
}

// TestImportReview_A11y_HxPushUrl pins that the commit form declares
// hx-push-url, so browser Back from the result has a destination.
func TestImportReview_A11y_HxPushUrl(t *testing.T) {
	out := renderReview(t, representativeReviewData())
	if !strings.Contains(out, `hx-push-url="/campaigns/camp-1/settings?tab=ai-workspace"`) {
		t.Errorf("review form is missing hx-push-url (V1-F Bug 2 fix)")
	}
}

// renderReview is the test-local helper that drives the ImportReview
// templ into a string.
func renderReview(t *testing.T, d ImportReviewData) string {
	t.Helper()
	var sb strings.Builder
	if err := ImportReview(d).Render(context.Background(), &sb); err != nil {
		t.Fatalf("ImportReview.Render: %v", err)
	}
	return sb.String()
}

func truncateA11y(s string) string {
	const n = 3000
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n...[truncated]"
}
