package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakePageMaps keeps one map per page; a page missing from blocks has no
// Page map block in its layout.
type fakePageMaps struct {
	blocks map[string]string
	text   map[string]string
	saved  map[string]string
}

func (f *fakePageMaps) FirstBlock(_ context.Context, _, pageID string) (string, bool, error) {
	b, ok := f.blocks[pageID]
	return b, ok, nil
}
func (f *fakePageMaps) Text(_ context.Context, _, pageID, _ string, _ Actor) (string, error) {
	return f.text[pageID], nil
}
func (f *fakePageMaps) Save(_ context.Context, _, pageID, _ string, _ Actor, text string) error {
	f.saved[pageID] = text
	return nil
}

func pageMaps() (*fakePageMaps, fakeEntities) {
	f := &fakePageMaps{
		blocks: map[string]string{"hm": "b1", "keep": "b2"},
		text:   map[string]string{"hm": "room \"Old keep\" 1,1 to 4,4"},
		saved:  map[string]string{},
	}
	pages := fakeEntities{}
	for _, e := range []entities.Entity{page("hm", "Harrowmere", "Place", false), page("keep", "The Keep", "Place", false), page("inn", "The Inn", "Place", false)} {
		e := e
		pages[e.Slug] = &e
	}
	return f, pages
}

func mapRec(action, pg, body string) Record {
	return Record{Kind: "page-map", Action: action, Fields: map[string]any{"page": pg}, Body: body}
}

const twoThings = "background hex\nroom \"The keep\" 18,11 to 22,15\npin \"The gate\" 20,22"

func TestPageMapKind_Plan(t *testing.T) {
	made := mapRec(ActionCreate, "New Town", twoThings)
	made.Made = map[string]bool{"new-town": true}
	tests := []struct {
		name    string
		r       Record
		a       Actor
		summary string
		err     string
	}{
		{"draw on an empty map", mapRec(ActionCreate, "The Keep", twoThings), owner, "1 marker, 1 shape, hex background", ""},
		{"replace a map", mapRec(ActionUpdate, "Harrowmere", twoThings), owner, "replaces the map: 1 marker", ""},
		{"create over a map", mapRec(ActionCreate, "Harrowmere", twoThings), owner, "", "already has a map; use action: update"},
		{"no block in the layout", mapRec(ActionUpdate, "The Inn", twoThings), owner, "", "Add a Page map block"},
		{"draws nothing", mapRec(ActionUpdate, "Harrowmere", "background hex"), owner, "", "the map draws nothing"},
		{"no such page", mapRec(ActionUpdate, "Nowhere", twoThings), owner, "", "no page called"},
		{"a page made in this paste", made, owner, "1 marker", ""},
		{"no page named", mapRec(ActionUpdate, "", twoThings), owner, "", "needs `page`"},
		{"remove refused", mapRec(ActionDelete, "Harrowmere", twoThings), owner, "", "can't be removed from here"},
		{"players can't", mapRec(ActionUpdate, "Harrowmere", twoThings), player, "", "only the owner or a co-DM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, pages := pageMaps()
			p := PageMapKind{Svc: f, Entities: pages}.Plan(context.Background(), camp, tt.a, tt.r)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tt.summary) {
				t.Fatalf("plan = %+v, want summary containing %q", p, tt.summary)
			}
		})
	}
}

// A line the parser can't read is a warning on the row, not a refusal.
func TestPageMapKind_PlanWarnsOnBadLines(t *testing.T) {
	f, pages := pageMaps()
	p := PageMapKind{Svc: f, Entities: pages}.Plan(context.Background(), camp, owner, mapRec(ActionUpdate, "Harrowmere", twoThings+"\nzeppelin over the keep"))
	if p.Error != "" || len(p.Warnings) == 0 {
		t.Fatalf("plan = %+v, want a warning and no error", p)
	}
}

func TestPageMapKind_Apply(t *testing.T) {
	f, pages := pageMaps()
	k := PageMapKind{Svc: f, Entities: pages}
	if err := k.Apply(context.Background(), camp, owner, mapRec(ActionUpdate, "Harrowmere", twoThings)); err != nil {
		t.Fatal(err)
	}
	if f.saved["hm"] != twoThings {
		t.Errorf("saved = %q, want the whole body", f.saved["hm"])
	}
	if err := k.Apply(context.Background(), camp, owner, mapRec(ActionUpdate, "The Inn", twoThings)); err == nil {
		t.Error("a page with no map block must fail")
	}
}

// The page lookup ends with the map's lines, ready to send back.
func TestLookups_PageMap(t *testing.T) {
	f, _ := pageMaps()
	hm := page("hm", "Harrowmere", "Place", false)
	inn := page("inn", "The Inn", "Place", false)
	l := world()
	l.Pages = &fakePages{all: []entities.Entity{hm, inn}}
	l.PageMaps = f
	ans := l.Answer(context.Background(), camp, owner, []Record{lookup("page", map[string]any{"name": "Harrowmere"})})
	if ans[0].Error != "" || !strings.Contains(ans[0].Text, "### Map") || !strings.Contains(ans[0].Text, "room \"Old keep\" 1,1 to 4,4") {
		t.Fatalf("answer lacks the map:\n%s %s", ans[0].Text, ans[0].Error)
	}
	ans = l.Answer(context.Background(), camp, owner, []Record{lookup("page", map[string]any{"name": "The Inn"})})
	if strings.Contains(ans[0].Text, "### Map") {
		t.Errorf("a page with no map shows a Map section:\n%s", ans[0].Text)
	}
}
