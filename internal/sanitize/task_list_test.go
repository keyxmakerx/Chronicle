package sanitize

import (
	"strings"
	"testing"
)

// TestHTML_KeepsChecklistState: the editor's checklist survives sanitizing
// as a checklist, ticked items included, while the checkbox control itself
// and any other data-type or data-checked value is dropped.
func TestHTML_KeepsChecklistState(t *testing.T) {
	in := `<ul data-type="taskList"><li data-checked="true" data-type="taskItem"><label><input type="checkbox" checked="checked"><span></span></label><div><p>Rusted key</p></div></li>` +
		`<li data-checked="false" data-type="taskItem"><label><input type="checkbox"><span></span></label><div><p>Signet ring</p></div></li></ul>`
	got := HTML(in)
	for _, want := range []string{`<ul data-type="taskList">`, `data-checked="true"`, `data-checked="false"`, `data-type="taskItem"`, "Rusted key", "Signet ring"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in %s", want, got)
		}
	}
	if strings.Contains(got, "<input") {
		t.Errorf("no form control may be stored: %s", got)
	}

	got = HTML(`<ul data-type="x onclick"><li data-checked="yes" data-type="other">a</li></ul><ol data-type="taskList"><li>b</li></ol>`)
	if strings.Contains(got, "data-checked") || strings.Contains(got, `data-type="x`) || strings.Contains(got, `data-type="other"`) || strings.Contains(got, `<ol data-type`) {
		t.Errorf("only the checklist values on ul/li may pass: %s", got)
	}
}
