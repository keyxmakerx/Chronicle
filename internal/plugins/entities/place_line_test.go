// place_line_test.go pins the markup of the "Also listed under" line and of a
// listing row in the sidebar tree: swap-safe inline handlers, escaped names,
// controls only for people who can edit, and no data-entity-id on a listing
// row (the sidebar script keys real pages by it).
package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func placeCC() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe}
}

func TestEntityPlaces_Markup(t *testing.T) {
	links := []PlaceLink{{EntityID: "e1", ParentID: "p1", ParentName: `Guild <b>"Hall"</b>`}}
	tests := []struct {
		name     string
		p        pagePlaces
		want     []string
		wantNot  []string
		wantNone bool
	}{
		{
			name:    "an editor gets the links, the remove button and the picker",
			p:       pagePlaces{Enabled: true, CanEdit: true, Links: links, RealParentID: "home"},
			want:    []string{"Also listed under:", "Add a place", `class="pl-x"`, `data-parent-id="home"`, `data-listed="p1"`, "onclick="},
			wantNot: []string{"<script"},
		},
		{
			name:    "a reader sees the links only",
			p:       pagePlaces{Enabled: true, CanEdit: false, Links: links},
			want:    []string{"Also listed under:", "/campaigns/camp-1/entities/p1"},
			wantNot: []string{"Add a place", "pl-x", "<script"},
		},
		{name: "a reader of a page with no extra places sees nothing", p: pagePlaces{Enabled: true}, wantNone: true},
		{name: "unwired listings draw nothing", p: pagePlaces{CanEdit: true, Links: links}, wantNone: true},
		{
			name:    "an editor of a page with no extra places gets the button without the lead",
			p:       pagePlaces{Enabled: true, CanEdit: true},
			want:    []string{"Add a place"},
			wantNot: []string{"Also listed under:"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := entityPlaces(placeCC(), &Entity{ID: "e1", Name: "Cook"}, tc.p).Render(context.Background(), &buf); err != nil {
				t.Fatal(err)
			}
			html := buf.String()
			if tc.wantNone {
				if strings.TrimSpace(html) != "" {
					t.Fatalf("expected nothing, got %q", html)
				}
				return
			}
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q in %s", w, html)
				}
			}
			for _, w := range tc.wantNot {
				if strings.Contains(html, w) {
					t.Errorf("unexpected %q in %s", w, html)
				}
			}
			if strings.Contains(html, `<b>"Hall"</b>`) {
				t.Errorf("a parent's name must be escaped: %s", html)
			}
		})
	}
}

func TestSidebarEntityList_DrawsListingsAsPlaceRows(t *testing.T) {
	cc := placeCC()
	parent := Entity{ID: "p1", Name: "Guild", EntityTypeID: 1}
	ctx := withTreePlaces(context.Background(), []PlaceLink{
		{EntityID: "e1", EntityName: "Cook", ParentID: "p1", ParentName: "Guild"},
	})
	var buf bytes.Buffer
	if err := SidebarEntityList([]Entity{parent}, nil, 1, 1, cc, nil).Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	at := strings.Index(html, `data-place-key="e1~p1"`)
	row := html[strings.LastIndex(html[:at], "<a "):]
	row = row[:strings.Index(row, "</a>")]
	for _, w := range []string{`data-place-entity="e1"`, `data-parent-id="p1"`, "also here", "/campaigns/camp-1/entities/e1"} {
		if !strings.Contains(row, w) {
			t.Errorf("listing row missing %q: %s", w, row)
		}
	}
	if strings.Contains(row, "data-entity-id") || strings.Contains(row, "draggable") {
		t.Errorf("a listing row must not look like the page's own row: %s", row)
	}
}

func TestBuildEntityTreeWithPlaces(t *testing.T) {
	p1, p2 := "city", "city"
	entities := []Entity{
		{ID: "city", Name: "City"},
		{ID: "guild", Name: "Guild", ParentID: &p1},
		{ID: "inn", Name: "Inn", ParentID: &p2},
	}
	places := []PlaceLink{
		{EntityID: "cook", EntityName: "Cook", ParentID: "guild"},
		{EntityID: "ghost", EntityName: "Elsewhere", ParentID: "not-on-this-page"},
	}
	roots := buildEntityTreeWithPlaces(entities, places)
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want 1", len(roots))
	}
	var guild *EntityTreeNode
	for i := range roots[0].Children {
		if roots[0].Children[i].Entity.ID == "guild" {
			guild = &roots[0].Children[i]
		}
	}
	if guild == nil || len(guild.Children) != 1 || !guild.Children[0].AlsoHere || guild.Children[0].Entity.ID != "cook" {
		t.Fatalf("Cook should sit under Guild as an also-here leaf: %+v", roots)
	}
	// A listing whose parent is not on this page is dropped, never a root.
	for _, r := range roots {
		if r.AlsoHere {
			t.Fatalf("a listing became a root: %+v", r)
		}
	}
}
