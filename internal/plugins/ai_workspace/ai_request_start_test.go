package ai_workspace

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

// A change a connected AI asked for to something that already exists starts
// on No, so text it read can't steer an edit through a hurried Apply. The
// same block pasted by the owner keeps starting on Yes; new things start on
// Yes either way, and removals on No either way.
func TestStartsYes_AIChangesStartOnNo(t *testing.T) {
	page := func(action string) importer.ParsedPage {
		return importer.ParsedPage{FrontMatter: importer.FrontMatter{Action: action}}
	}
	ok := importer.Classification{Status: importer.StatusNew}
	cases := []struct {
		name   string
		action string
		fromAI bool
		want   bool
	}{
		{"pasted change", importer.ActionUpdate, false, true},
		{"AI change", importer.ActionUpdate, true, false},
		{"AI new", importer.ActionCreate, true, true},
		{"AI new, no action", "", true, true},
		{"AI removal", importer.ActionDelete, true, false},
		{"pasted removal", importer.ActionDelete, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pageStartsYes(page(tc.action), ok, tc.fromAI); got != tc.want {
				t.Errorf("page: %v, want %v", got, tc.want)
			}
			if got := recordStartsYes(RecordRow{Action: tc.action}, tc.fromAI); got != tc.want {
				t.Errorf("record: %v, want %v", got, tc.want)
			}
		})
	}

	// The save bar's first count agrees with the rows.
	d := ImportReviewData{
		Pages:   []importer.ParsedPage{page(importer.ActionUpdate), page(importer.ActionCreate)},
		Classes: []importer.Classification{ok, ok},
		Records: []RecordRow{{Index: 0, Label: "Table", Action: records.ActionUpdate}},
	}
	if got := startingYesCount(d); got != 3 {
		t.Errorf("pasted: %d start on Yes, want 3", got)
	}
	d.RequestID = 7
	if got := startingYesCount(d); got != 1 {
		t.Errorf("AI request: %d start on Yes, want 1", got)
	}
}

// The record row draws the answer it was given.
func TestReviewRecordRow_AIChangeRendersNo(t *testing.T) {
	r := RecordRow{Index: 0, Label: "Table", Action: records.ActionUpdate, Name: "Tavern names"}
	for _, tc := range []struct {
		fromAI bool
		want   string
	}{{false, `value="on" checked`}, {true, `value="off" checked`}} {
		var buf bytes.Buffer
		if err := reviewRecordRow("camp-1", r, tc.fromAI).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), tc.want) {
			t.Errorf("fromAI=%v: want %q in\n%s", tc.fromAI, tc.want, buf.String())
		}
	}
}
