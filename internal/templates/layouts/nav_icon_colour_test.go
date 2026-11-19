package layouts

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
)

// The sidebar's marks are all one colour. A category's own colour tints
// neither its icon nor the dot that stands in for a missing one, in the list
// and in the category's panel; the owner's pick is kept and simply not drawn
// here.

// navMarks returns the inside of every icon slot matching class.
func navMarks(html, class string) []string {
	re := regexp.MustCompile(`<span class="` + class + `"[^>]*>(.*?)</span>\s*<span class="nav-`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		out = append(out, m[0])
	}
	return out
}

func TestSidebar_CategoryMarksCarryNoColourOfTheirOwn(t *testing.T) {
	sections := []NavSectionView{
		{ID: "apps", Kind: "apps", Label: "Apps", Rows: []NavRowView{
			{Key: "app:maps", Kind: "app", Label: "Maps", Icon: "fa-map", URL: "/campaigns/c1/maps"},
		}},
		{ID: "categories", Kind: "categories", Label: "Categories", Rows: []NavRowView{
			{Key: "cat:1", Kind: "category", Label: "Locations", Icon: "fa-map-pin", URL: "/campaigns/c1/locations", TypeID: 1, Count: 7,
				Subs: []NavSubRowView{{Key: "cat:11", TypeID: 11, Label: "Cities", URL: "/campaigns/c1/cities", Count: 3}}},
			{Key: "cat:2", Kind: "category", Label: "Factions", URL: "/campaigns/c1/factions", TypeID: 2},
		}},
	}
	ctx := context.Background()
	ctx = SetIsAuthenticated(ctx, true)
	ctx = SetCampaignID(ctx, "c1")
	ctx = SetCampaignRole(ctx, 3)
	ctx = SetActivePath(ctx, "/campaigns/c1")
	ctx = SetNavSections(ctx, sections)
	// Unfold Locations so its sub-category's row is drawn.
	ctx = SetNavFolds(ctx, NavFolds{"cat:1": false})
	html := renderNavList(t, ResolveNavState(ctx))

	for _, want := range []string{
		`<i class="fa-solid fa-map-pin" aria-hidden="true"></i>`,
		`<span class="nav-dot"></span>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("sidebar is missing %s", want)
		}
	}
	marks := navMarks(html, "nav-ic")
	if len(marks) < 3 {
		t.Fatalf("found %d icon slots, want the app's and both categories': %s", len(marks), html)
	}
	for _, m := range marks {
		if strings.Contains(m, "style=") {
			t.Errorf("a sidebar mark carries a colour of its own: %s", m)
		}
	}
}

func TestSidebarDrillPanel_HeadAndSubMarksCarryNoColourOfTheirOwn(t *testing.T) {
	parent := 1
	withIcon := SidebarEntityType{ID: 1, Slug: "locations", Name: "Location", NamePlural: "Locations", Icon: "fa-map-pin", Color: "#ef4444"}
	noIcon := SidebarEntityType{ID: 2, Slug: "factions", Name: "Faction", NamePlural: "Factions", Color: "#fbbf24"}
	sub := SidebarEntityType{ID: 11, Slug: "cities", Name: "City", NamePlural: "Cities", Color: "#6366f1", ParentTypeID: &parent}
	for _, et := range []SidebarEntityType{withIcon, noIcon} {
		ctx := navStateCase{role: 3, path: "/campaigns/c1/maps"}.ctx()
		ctx = SetEntityTypes(ctx, []SidebarEntityType{withIcon, noIcon, sub})
		var buf bytes.Buffer
		if err := SidebarDrillPanel("c1", et, 7).Render(ctx, &buf); err != nil {
			t.Fatalf("render: %v", err)
		}
		html := buf.String()
		head := html[strings.Index(html, `class="nav-drill-head"`):]
		head = head[:strings.Index(head, `class="nav-drill-name"`)]
		if !strings.Contains(head, `<span class="nav-drill-ic">`) || strings.Contains(head, "style=") {
			t.Errorf("%s: the panel's mark is not drawn plainly: %s", et.NamePlural, head)
		}
		for _, colour := range []string{"#ef4444", "#fbbf24", "#6366f1"} {
			if strings.Contains(html, colour) {
				t.Errorf("%s: the panel carries a category's colour, %s", et.NamePlural, colour)
			}
		}
		if et.ID == parent && !strings.Contains(html, `<span class="nav-drill-sub-ic"><span class="nav-dot"></span></span>`) {
			t.Errorf("%s: its sub-category's dot is not drawn plainly: %s", et.NamePlural, html)
		}
	}
}
