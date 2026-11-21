package layouts

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
)

// The sidebar's marks are all one colour unless the campaign picks the
// Category icon colour. By default a category's own colour tints neither its
// icon nor the dot that stands in for a missing one, in the list and in the
// category's panel; the owner's pick is kept and simply not drawn. Under the
// Category theme the colour travels as the --kind-colour custom property and
// the stylesheet decides how to use it; it is never a colour declaration.

// navMarks returns the inside of every icon slot matching class.
func navMarks(html, class string) []string {
	re := regexp.MustCompile(`<span class="` + class + `[^"]*"[^>]*>(.*?)</span>\s*<span class="nav-`)
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
		if !strings.Contains(head, `<span class="nav-drill-ic kind-themed">`) || strings.Contains(head, "style=") {
			t.Errorf("%s: the panel's mark is not drawn plainly: %s", et.NamePlural, head)
		}
		for _, colour := range []string{"#ef4444", "#fbbf24", "#6366f1"} {
			if strings.Contains(html, colour) {
				t.Errorf("%s: the panel carries a category's colour, %s", et.NamePlural, colour)
			}
		}
		if et.ID == parent && !strings.Contains(html, `<span class="nav-drill-sub-ic kind-themed"><span class="nav-dot"></span></span>`) {
			t.Errorf("%s: its sub-category's dot is not drawn plainly: %s", et.NamePlural, html)
		}
	}
}

// categoryThemeCtx is a campaign that colours category icons by category.
func categoryThemeCtx(colour string) context.Context {
	ctx := navStateCase{role: 3, path: "/campaigns/c1/maps"}.ctx()
	return SetAppearance(ctx, &AppearanceData{IconColour: colour})
}

func TestSidebarDrillPanel_CategoryThemePassesTheColourAsAProperty(t *testing.T) {
	parent := 1
	head := SidebarEntityType{ID: 1, Slug: "locations", Name: "Location", NamePlural: "Locations", Icon: "fa-map-pin", Color: "#EF4444"}
	sub := SidebarEntityType{ID: 11, Slug: "cities", Name: "City", NamePlural: "Cities", Icon: "fa-city", Color: "#6366f1", ParentTypeID: &parent}
	hostile := SidebarEntityType{ID: 12, Slug: "ruins", Name: "Ruin", NamePlural: "Ruins", Color: `red;background:url(//x.test/a)`, ParentTypeID: &parent}

	tests := []struct {
		name   string
		colour string
		want   []string
		never  []string
	}{
		{"category", "category", []string{
			`<span class="nav-drill-ic kind-themed" style="--kind-colour:#ef4444;">`,
			`<span class="nav-drill-sub-ic kind-themed" style="--kind-colour:#6366f1;">`,
		}, []string{"url(", "red;", "background"}},
		{"accent carries no colour", "accent", nil, []string{"--kind-colour", "#ef4444", "#6366f1"}},
		{"ink carries no colour", "", nil, []string{"--kind-colour", "#ef4444", "#6366f1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := SetEntityTypes(categoryThemeCtx(tt.colour), []SidebarEntityType{head, sub, hostile})
			var buf bytes.Buffer
			if err := SidebarDrillPanel("c1", head, 7).Render(ctx, &buf); err != nil {
				t.Fatalf("render: %v", err)
			}
			html := buf.String()
			for _, w := range tt.want {
				if !strings.Contains(html, w) {
					t.Errorf("panel is missing %s in %s", w, html)
				}
			}
			for _, n := range tt.never {
				if strings.Contains(html, n) {
					t.Errorf("panel carries %q: %s", n, html)
				}
			}
		})
	}
}

func TestSidebar_CategoryThemePassesTheColourOnCategoryRowsOnly(t *testing.T) {
	sections := []NavSectionView{
		{ID: "apps", Kind: "apps", Label: "Apps", Rows: []NavRowView{
			{Key: "app:maps", Kind: "app", Label: "Maps", Icon: "fa-map", Color: "#112233", URL: "/campaigns/c1/maps"},
		}},
		{ID: "categories", Kind: "categories", Label: "Categories", Rows: []NavRowView{
			{Key: "cat:1", Kind: "category", Label: "Locations", Icon: "fa-map-pin", Color: "#EF4444", URL: "/campaigns/c1/locations", TypeID: 1, Count: 7,
				Subs: []NavSubRowView{{Key: "cat:11", TypeID: 11, Label: "Cities", Color: "#6366f1", URL: "/campaigns/c1/cities", Count: 3}}},
			{Key: "cat:2", Kind: "category", Label: "Factions", Color: "not a colour", URL: "/campaigns/c1/factions", TypeID: 2},
		}},
	}
	ctx := categoryThemeCtx("category")
	ctx = SetNavSections(ctx, sections)
	ctx = SetNavFolds(ctx, NavFolds{"cat:1": false})
	html := renderNavList(t, ResolveNavState(ctx))

	for _, want := range []string{
		`<span class="nav-ic kind-themed" style="--kind-colour:#ef4444;"><i class="fa-solid fa-map-pin"`,
		`<span class="nav-ic kind-themed" style="--kind-colour:#6366f1;"><span class="nav-dot">`,
		`<span class="nav-ic kind-themed"><span class="nav-dot"></span>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("sidebar is missing %s in %s", want, html)
		}
	}
	if strings.Contains(html, "#112233") || strings.Contains(html, "not a colour") {
		t.Errorf("an app's colour, or one that is not #RRGGBB, reached the markup: %s", html)
	}
}

func TestKindIconStyle(t *testing.T) {
	tests := []struct {
		name   string
		a      *AppearanceData
		colour string
		want   string
	}{
		{"no appearance", nil, "#ef4444", ""},
		{"ink", &AppearanceData{}, "#ef4444", ""},
		{"accent", &AppearanceData{IconColour: "accent"}, "#ef4444", ""},
		{"category", &AppearanceData{IconColour: "category"}, "#EF4444", "--kind-colour:#ef4444"},
		{"category, no colour saved", &AppearanceData{IconColour: "category"}, "", ""},
		{"category, short hex", &AppearanceData{IconColour: "category"}, "#f44", ""},
		{"category, injected", &AppearanceData{IconColour: "category"}, "#ef4444;background:red", ""},
		{"category, a function", &AppearanceData{IconColour: "category"}, "rgb(1,2,3)", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.a != nil {
				ctx = SetAppearance(ctx, tt.a)
			}
			if got := KindIconStyle(ctx, tt.colour); got != tt.want {
				t.Errorf("KindIconStyle = %q, want %q", got, tt.want)
			}
			attrs := KindIconAttrs(ctx, tt.colour)
			if (tt.want == "") != (len(attrs) == 0) {
				t.Errorf("KindIconAttrs = %v for style %q", attrs, tt.want)
			}
		})
	}
}
