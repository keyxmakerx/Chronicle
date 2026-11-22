package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

func newTemplatesLookup() LayoutsLookup {
	return LayoutsLookup{Svc: newLayoutFake(), Homes: newCatHomes(), Saved: newSavedFake(), Templates: newTemplatesFake()}
}

func TestLayoutsLookup_Parts(t *testing.T) {
	cases := []struct {
		name    string
		a       Actor
		fields  map[string]any
		want    []string
		lacks   []string
		wantErr string
	}{
		{name: "the default is the page layouts and points at the rest", a: layoutOwner,
			want:  []string{"## Page layouts", "### More templates", "`part: category-homes`", "`part: saved-layouts`", "`part: page-templates`"},
			lacks: []string{"## Category homes"}},
		{name: "category homes", a: layoutOwner, fields: map[string]any{"part": "category-homes"},
			want: []string{"## Category homes", "### Locations", "Its own arrangement: category_header, text_block / entity_grid | pinned_pages / [tile section: recent_pages]",
				"kind: category-home\ncategory: \"Location\"", "{type: text_block, text: \"Start here\"}", "### Characters", "Not saved, so it shows Chronicle's built-in landing page",
				"category: \"Character\"", "### Blocks a category home can place", "- `entity_grid` — All pages: Every page"}},
		{name: "category homes of one category", a: layoutOwner, fields: map[string]any{"part": "category_homes", "category": "locations"},
			want: []string{"### Locations"}, lacks: []string{"### Characters"}},
		{name: "saved layouts", a: layoutOwner, fields: map[string]any{"part": "saved-layouts"},
			want: []string{"## Saved layouts", "### Standard", "Built in, so it can't be changed or removed. It holds: title, entry | image.", "### Hero sheet",
				"kind: saved-layout\nname: \"Hero sheet\"", "{type: details, directors_only: true}", "### Blocks a layout can place"},
			lacks: []string{"kind: saved-layout\nname: \"Standard\""}},
		{name: "one saved layout", a: layoutOwner, fields: map[string]any{"part": "saved-layouts", "name": "hero sheet"},
			want: []string{"### Hero sheet"}, lacks: []string{"### Standard"}},
		{name: "unknown saved layout", a: layoutOwner, fields: map[string]any{"part": "saved-layouts", "name": "Villain"}, wantErr: "no saved layout called"},
		{name: "page templates", a: layoutOwner, fields: map[string]any{"part": "page-templates"},
			want: []string{"## Page templates", "### Session Recap", "Shared by every campaign, so it can't be changed or removed.", "### Tavern", "For Locations.",
				"kind: page-template\nname: \"Tavern\"\ncategory: \"Location\"", "## The room", "- Bar", "### Rumours", "For every category.", "Heard **in town**"},
			lacks: []string{"kind: page-template\nname: \"Session Recap\""}},
		{name: "page templates of a category include the ones for every category", a: layoutOwner, fields: map[string]any{"part": "page-templates", "category": "Locations"},
			want: []string{"### Tavern", "### Rumours", "### Session Recap"}},
		{name: "page templates of a category with none", a: layoutOwner, fields: map[string]any{"part": "page-templates", "category": "Characters", "name": "Tavern"},
			wantErr: "no page template called"},
		{name: "one page template", a: layoutOwner, fields: map[string]any{"part": "page-templates", "name": "tavern"},
			want: []string{"### Tavern"}, lacks: []string{"### Rumours"}},
		{name: "unknown part", a: layoutOwner, fields: map[string]any{"part": "moods"}, wantErr: "not a part; use page-layouts, category-homes, saved-layouts, page-templates"},
		{name: "a co-DM is refused in every part", a: layoutCoDM, fields: map[string]any{"part": "page-templates"}, wantErr: "campaign owner's"},
		{name: "a player is refused in every part", a: Actor{UserID: "pl", Role: 1}, fields: map[string]any{"part": "saved-layouts"}, wantErr: "campaign owner's"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Record{Kind: KindLookup, Fields: tc.fields}
			if r.Fields == nil {
				r.Fields = map[string]any{}
			}
			chip, text, err := newTemplatesLookup().Answer(context.Background(), "c", tc.a, r)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || text != "" {
					t.Fatalf("got %q, %v", text, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if chip == "" {
				t.Error("no chip")
			}
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Errorf("missing %q in:\n%s", w, text)
				}
			}
			for _, w := range tc.lacks {
				if strings.Contains(text, w) {
					t.Errorf("unexpected %q in:\n%s", w, text)
				}
			}
		})
	}
}

// A part that is not wired on this server says so, and the default answer does
// not point at it.
func TestLayoutsLookup_PartNotWired(t *testing.T) {
	l := LayoutsLookup{Svc: newLayoutFake()}
	_, text, err := l.Answer(context.Background(), "c", layoutOwner, Record{Kind: KindLookup, Fields: map[string]any{}})
	if err != nil || strings.Contains(text, "More templates") {
		t.Errorf("default: %v\n%s", err, text)
	}
	for _, part := range []string{"category-homes", "saved-layouts", "page-templates"} {
		_, _, err := l.Answer(context.Background(), "c", layoutOwner, Record{Kind: KindLookup, Fields: map[string]any{"part": part}})
		if err == nil || !strings.Contains(err.Error(), "can't be looked up on this server") {
			t.Errorf("%s: %v", part, err)
		}
	}
}

// Every part stays under the size one lookup answer may have.
func TestLayoutsLookup_PartsFitTheAnswerCap(t *testing.T) {
	for _, part := range layoutParts {
		_, text, err := newTemplatesLookup().Answer(context.Background(), "c", layoutOwner, Record{Kind: KindLookup, Fields: map[string]any{"part": part}})
		if err != nil || len(text) > maxAnswerBytes {
			t.Errorf("%s: %d bytes, %v", part, len(text), err)
		}
	}
}

// The lookup is found by its aliases and shows up as the owner-only
// `templates` tool, which takes a part.
func TestLayoutsLookup_ToolInfo(t *testing.T) {
	in := LayoutsLookup{}.LookupInfo()
	if in.What != "layouts" || in.Tool.Name != "templates" || !in.Tool.Owner {
		t.Fatalf("%+v", in)
	}
	var keys []string
	for _, a := range in.Tool.Args {
		keys = append(keys, a.Key)
	}
	if got := strings.Join(keys, " "); got != "part category layout name" {
		t.Errorf("args = %s", got)
	}
	var _ aiplug.Lookup = LayoutsLookup{}
}
