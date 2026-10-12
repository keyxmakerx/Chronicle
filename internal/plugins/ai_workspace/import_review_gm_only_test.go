package ai_workspace

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
)

// TestImportReview_GMOnlyTick pins the callout and the unticked, optional
// checkbox on a row that would replace a page with GM-only parts, and that a
// page without any renders neither.
func TestImportReview_GMOnlyTick(t *testing.T) {
	d := representativeReviewData()
	d.Classes[0].GMOnly = importer.GMOnlyParts{Passages: 2, Pictures: 1}
	out := renderReview(t, d)
	for _, want := range []string{
		`name="page_0_replace_gm_confirmed"`,
		"2 GM-only passages and 1 GM-only picture.",
		"Replace the text anyway",
		"will be visible to players",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("review is missing %q", want)
		}
	}
	i := strings.Index(out, `name="page_0_replace_gm_confirmed"`)
	if tag := out[i : i+strings.Index(out[i:], ">")]; strings.Contains(tag, "required") || strings.Contains(tag, "checked") {
		t.Errorf("the tick must be optional and unticked: %s", tag)
	}

	plain := renderReview(t, representativeReviewData())
	if strings.Contains(plain, "replace_gm_confirmed") {
		t.Errorf("a page without GM-only parts should show no tick")
	}
}
