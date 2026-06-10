package calendar

// calendar_v2_quickedit_test.go — the event quick-edit card (C-CAL-QUICKEDIT):
// the editing affordances must be SERVER-GATED to Scribes (markup-level, not
// CSS) — players receive a read-only card with no inputs or buttons.

import (
	"context"
	"strings"
	"testing"
)

func renderQuickEdit(t *testing.T, isScribe bool) string {
	t.Helper()
	var sb strings.Builder
	data := CalendarV2ViewData{IsScribe: isScribe}
	if err := eventQuickEditV2(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render quick-edit: %v", err)
	}
	return sb.String()
}

// TestEventQuickEditV2_ScribeGetsEditor: Scribes get the editable card —
// name input, description textarea, Save + Full-editor buttons.
func TestEventQuickEditV2_ScribeGetsEditor(t *testing.T) {
	html := renderQuickEdit(t, true)
	for _, want := range []string{
		`id="cal-v2-event-quickedit"`,
		"data-qe-name", "data-qe-desc", "data-qe-save", "data-qe-expand",
		"data-qe-close", "data-qe-meta", "data-qe-vis",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("scribe quick-edit card missing %q", want)
		}
	}
}

// TestEventQuickEditV2_PlayersReadOnly: players get plain text — no inputs,
// no Save, no Full-editor (the gate is the markup, not a CSS hide).
func TestEventQuickEditV2_PlayersReadOnly(t *testing.T) {
	html := renderQuickEdit(t, false)
	for _, want := range []string{"data-qe-name-ro", "data-qe-desc-ro", "data-qe-close"} {
		if !strings.Contains(html, want) {
			t.Errorf("player quick-edit card missing read-only %q", want)
		}
	}
	for _, gone := range []string{"data-qe-save", "data-qe-expand", "<input", "<textarea"} {
		if strings.Contains(html, gone) {
			t.Errorf("player quick-edit card must not contain %q", gone)
		}
	}
}
