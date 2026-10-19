package dmscreen

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// The Adventures tab sits in a panel that HTMX swaps, so its handlers must be
// self-contained inline text: a function expression that fits inside a
// double-quoted attribute and never names a templ script helper.
func TestAdventuresHandlers_AreSwapSafe(t *testing.T) {
	tests := map[string]string{
		"open":   openSheetJS,
		"remove": removeItemJS,
		"note":   addNoteJS,
		"enter":  noteEnterJS,
		"pick":   pickPageJS,
	}
	for name, js := range tests {
		t.Run(name, func(t *testing.T) {
			if !strings.HasPrefix(js, "(function(") {
				t.Errorf("handler must start with an IIFE, got %.20q", js)
			}
			if strings.Contains(js, `"`) {
				t.Error("handler holds a double quote, which would end its attribute")
			}
			if strings.Contains(js, "__templ_") {
				t.Error("handler references a templ script helper")
			}
		})
	}
}

func TestAdventuresPane_Render(t *testing.T) {
	v := &View{CampaignID: "c1", Adventures: &AdventuresView{Items: []AdventureItem{
		{ID: "i1", Kind: BoardKindPage, EntityID: "e1", Kicker: "Adventure", Title: "Lastwell", Subtitle: "Running"},
		{ID: "i2", Kind: BoardKindNote, Text: "Bring the <bell>"},
	}}}
	var buf bytes.Buffer
	if err := Panel(v, "tok").Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`data-dms-tab="adventures"`,
		`data-dms-pane="adventures"`,
		`data-sheet-url="/campaigns/c1/entities/e1/sheet"`,
		`data-url="/campaigns/c1/dm-screen/adventures/items/i1"`,
		"Lastwell",
		"Only the GM team sees this tab.",
		"Bring the &lt;bell&gt;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("panel is missing %q", want)
		}
	}
	if strings.Contains(out, "<bell>") {
		t.Error("a note's text reached the page unescaped")
	}
	if strings.Contains(out, "__templ_") {
		t.Error("the tab uses a templ script helper, which a swapped fragment may not run")
	}

	// With no board the tab is left out entirely.
	buf.Reset()
	if err := Panel(&View{CampaignID: "c1"}, "tok").Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "adventures") {
		t.Error("the Adventures tab shows although no board is wired")
	}
}
