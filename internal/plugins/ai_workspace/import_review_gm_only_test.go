package ai_workspace

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
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

// TestImportReview_ListAnswers pins the list's first answers and wording:
// a removal starts on No with its "for good" tick not yet required, other
// rows start on Yes, and the AI's reason shows when it gave one.
func TestImportReview_ListAnswers(t *testing.T) {
	d := representativeReviewData()
	d.Pages[0].FrontMatter.Reason = "She runs the map room."
	d.Pages[1].FrontMatter.Action = importer.ActionDelete
	d.Pages[3].SourceIndex = 3
	d.Records = []RecordRow{
		{Index: 0, Label: "Map pin", Action: "create", Name: "The breach", Block: 6, Reason: "Marks where the wall falls."},
		{Index: 1, Label: "Calendar", Action: "delete", Name: "Old Reckoning", Block: 7},
		{Index: 2, Label: "Calendar event", Action: "create", Name: "Feast", Block: 8, Plan: records.Plan{Error: "there's no month called Hamer"}},
	}
	out := renderReview(t, d)
	for _, want := range []string{
		`name="page_0_include" value="on" checked`,
		`name="page_1_include" value="off" checked`,
		`name="rec_0_include" value="on" checked`,
		`name="rec_1_include" value="off" checked`,
		"AI's reason: <i>She runs the map room.</i>",
		"AI's reason: <i>Marks where the wall falls.</i>",
		"Block 4", "Block 8", "Feast: there&#39;s no month called Hamer",
		"Remove the page <b>Lyra Vance</b>",
		`data-air-verb="Remove"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("review is missing %q", want)
		}
	}
	for _, name := range []string{"page_1_delete_confirmed", "rec_1_delete_confirmed"} {
		i := strings.Index(out, `name="`+name+`"`)
		if i < 0 {
			t.Fatalf("missing %s", name)
		}
		if tag := out[i : i+strings.Index(out[i:], ">")]; strings.Contains(tag, "required") {
			t.Errorf("%s must start not required (its removal starts on No): %s", name, tag)
		}
	}
	if strings.Contains(out, `name="rec_2_include"`) {
		t.Errorf("a refused record must not be offered as a row")
	}
}

// TestImportReview_SheetFields pins the row's sheet line, and that a page
// whose fields don't fit sits in the problems box instead of the list.
func TestImportReview_SheetFields(t *testing.T) {
	d := representativeReviewData()
	d.Classes[0].FieldsSummary = "Sheet: Level 3, Stamina 30"
	d.Pages[2].SourceIndex = 2
	d.Classes[2].Status = importer.StatusFieldProblem
	d.Classes[2].Reason = `These sheet fields don't fit the Creature category: "hp" is not one of this category's fields.`
	out := renderReview(t, d)
	for _, want := range []string{
		"Sheet: Level 3, Stamina 30",
		"Block 3", "Ash-Wraith: These sheet fields don&#39;t fit",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("review is missing %q", want)
		}
	}
	if strings.Contains(out, `name="page_2_include"`) {
		t.Errorf("a page whose fields don't fit must not be offered as a row")
	}
}
