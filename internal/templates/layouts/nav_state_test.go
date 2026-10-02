package layouts

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
)

// nav_state_test.go pins where the sidebar says the viewer is: the current
// row, the one living ring, the folds a viewer's cookie keeps, and that the
// server paints all of it so a full page load needs no script to look right.

func navStateTestSections() []NavSectionView {
	return []NavSectionView{
		{ID: "pinned", Kind: "pinned", Label: "Pinned", Rows: []NavRowView{
			{Key: "app:notes", Kind: "app", Label: "Journal", Icon: "fa-book", URL: "/campaigns/c1/journal"},
		}},
		{ID: "apps", Kind: "apps", Label: "Apps", Rows: []NavRowView{
			{Key: "app:maps", Kind: "app", Label: "Maps", Icon: "fa-map", URL: "/campaigns/c1/maps"},
		}},
		{ID: "categories", Kind: "categories", Label: "Categories", Rows: []NavRowView{
			{Key: "cat:1", Kind: "category", Label: "Locations", Color: "#f87171", URL: "/campaigns/c1/locations", TypeID: 1, Count: 7,
				Subs: []NavSubRowView{{Key: "cat:11", TypeID: 11, Label: "Cities", Color: "#f87171", URL: "/campaigns/c1/cities", Count: 3}}},
			{Key: "cat:2", Kind: "category", Label: "Factions", Color: "#fbbf24", URL: "/campaigns/c1/factions", TypeID: 2},
		}},
		{ID: "sec_prep", Kind: "custom", Label: "Prep", Rows: []NavRowView{
			{Key: "link:l1", Kind: "link", Label: "Tunnel map", URL: "/campaigns/c1/maps/tunnel"},
		}},
	}
}

type navStateCase struct {
	role     int
	player   bool // an owner or scribe viewing as a player
	archived bool
	path     string
	folds    NavFolds
	hint     *NavHint
}

func (c navStateCase) ctx() context.Context {
	ctx := context.Background()
	ctx = SetIsAuthenticated(ctx, true)
	ctx = SetCampaignID(ctx, "c1")
	ctx = SetCampaignName(ctx, "Saltmarsh")
	ctx = SetCampaignRole(ctx, c.role)
	ctx = SetViewingAsPlayer(ctx, c.player)
	ctx = SetCampaignArchived(ctx, c.archived)
	ctx = SetActivePath(ctx, c.path)
	ctx = SetNavSections(ctx, navStateTestSections())
	if c.folds != nil {
		ctx = SetNavFolds(ctx, c.folds)
	}
	if c.hint != nil {
		ctx = WithNavHint(ctx, *c.hint)
	}
	return ResolveNavState(ctx)
}

func TestParseNavFolds(t *testing.T) {
	many := make([]string, 70)
	for i := range many {
		many[i] = "sub-" + strings.Repeat("1", i%9+1) + ":1"
	}
	tests := []struct {
		name string
		raw  string
		want NavFolds
	}{
		{"empty", "", NavFolds{}},
		{"sections and sub-categories", "apps:0|manage:1|sub-12:1", NavFolds{"apps": false, "manage": true, "sub-12": true}},
		{"malformed parts are skipped", "apps:2|:1|bad id:1|x|sec_a:0", NavFolds{"sec_a": false}},
		{"markup is not an id", `<b>:1|apps:0`, NavFolds{"apps": false}},
		{"oversized cookie is ignored", strings.Repeat("a", 2049), NavFolds{}},
		{"reads a bounded number of parts", strings.Join(many, "|"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseNavFolds(tt.raw)
			if tt.want == nil {
				if len(got) == 0 || len(got) > 64 {
					t.Fatalf("read %d folds, want 1..64", len(got))
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseNavFolds(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestResolveNavState_CurrentRow(t *testing.T) {
	tests := []struct {
		name     string
		c        navStateCase
		wantKey  string
		wantPage string
	}{
		{"the campaign's front page is Dashboard", navStateCase{role: 1, path: "/campaigns/c1"}, "dashboard", ""},
		{"Dashboard matches only exactly", navStateCase{role: 1, path: "/campaigns/c1/unknown"}, "", ""},
		{"an app's own sub-pages", navStateCase{role: 1, path: "/campaigns/c1/maps/abc"}, "app:maps", ""},
		{"a sub-category's page is its own row", navStateCase{role: 1, path: "/campaigns/c1/cities"}, "cat:11", ""},
		{"an entity page follows its hint", navStateCase{role: 1, path: "/campaigns/c1/cities",
			hint: &NavHint{TypeID: 11, PageName: "Waterdeep"}}, "cat:11", "Waterdeep"},
		{"a hint for a category without a row falls back to the path", navStateCase{role: 1, path: "/campaigns/c1/entities/42",
			hint: &NavHint{TypeID: 99, PageName: "Lost"}}, "all", ""},
		{"a link never takes the ring", navStateCase{role: 1, path: "/campaigns/c1/maps/tunnel"}, "app:maps", ""},
		{"a player's own characters", navStateCase{role: 1, path: "/campaigns/c1/me"}, "me", ""},
		{"an owner has no My Characters row", navStateCase{role: 3, path: "/campaigns/c1/me"}, "", ""},
		{"an owner viewing as a player does", navStateCase{role: 3, player: true, path: "/campaigns/c1/me"}, "me", ""},
		{"the owner's Manage pages", navStateCase{role: 3, path: "/campaigns/c1/settings/general"}, "manage:settings", ""},
		{"a player has none of the owner's Manage rows", navStateCase{role: 1, path: "/campaigns/c1/settings"}, "", ""},
		{"the owner's People page", navStateCase{role: 3, path: "/campaigns/c1/members"}, "manage:members", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, page := NavCurrent(tt.c.ctx())
			if key != tt.wantKey || page != tt.wantPage {
				t.Fatalf("current = (%q, %q), want (%q, %q)", key, page, tt.wantKey, tt.wantPage)
			}
		})
	}
}

func TestResolveNavState_RingAndFolds(t *testing.T) {
	tests := []struct {
		name     string
		c        navStateCase
		wantRing string
		open     map[string]bool
	}{
		{"a current sub-category opens its parent's fold", navStateCase{role: 1, path: "/campaigns/c1/cities"},
			"cat:11", map[string]bool{"sub-1": true, "categories": true, "manage": false}},
		{"folded away by the viewer, the ring waits on the parent", navStateCase{role: 1, path: "/campaigns/c1/cities",
			folds: NavFolds{"sub-1": false}}, "cat:1", map[string]bool{"sub-1": false}},
		{"a current category opens its own sub-categories", navStateCase{role: 1, path: "/campaigns/c1/locations"},
			"cat:1", map[string]bool{"sub-1": true}},
		{"elsewhere, sub-categories start folded", navStateCase{role: 1, path: "/campaigns/c1/maps"},
			"app:maps", map[string]bool{"sub-1": false, "apps": true}},
		{"the viewer's own folds win", navStateCase{role: 3, path: "/campaigns/c1/maps",
			folds: NavFolds{"apps": false, "manage": true}}, "app:maps", map[string]bool{"apps": false, "manage": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.c.ctx()
			if !NavHasRing(ctx, tt.wantRing) {
				t.Errorf("ring is not on %q (state %+v)", tt.wantRing, navStateOf(ctx))
			}
			for id, want := range tt.open {
				if got := NavFoldOpen(ctx, id); got != want {
					t.Errorf("fold %q open = %v, want %v", id, got, want)
				}
			}
		})
	}
}

// renderNavList renders both halves of the sidebar (campaignNavTop and
// campaignNavCategories) and concatenates them, standing in for the single
// list the pre-split sidebar rendered — every existing assertion below cares
// about a row or heading existing SOMEWHERE in the sidebar, not which half.
// TestCampaignNav_TopAndCategoriesAreSeparate below pins the split itself.
func renderNavList(t *testing.T, ctx context.Context) string {
	t.Helper()
	var buf bytes.Buffer
	if err := campaignNavTop().Render(ctx, &buf); err != nil {
		t.Fatalf("render campaignNavTop: %v", err)
	}
	if err := campaignNavCategories().Render(ctx, &buf); err != nil {
		t.Fatalf("render campaignNavCategories: %v", err)
	}
	return buf.String()
}

func renderNavFooter(t *testing.T, ctx context.Context) string {
	t.Helper()
	var buf bytes.Buffer
	if err := campaignNavFooter().Render(ctx, &buf); err != nil {
		t.Fatalf("render campaignNavFooter: %v", err)
	}
	return buf.String()
}

// TestCampaignNav_TopAndCategoriesAreSeparate pins the two-part split itself:
// Dashboard/Pinned/Apps are in the fixed top block and never in the
// categories area, Categories/Manage are the other way around, and a
// category row opens the drill panel instead of navigating away.
func TestCampaignNav_TopAndCategoriesAreSeparate(t *testing.T) {
	// Entity types resolve a category button's drill URL (navDrillURL); the
	// shared fixture's cat:1/cat:2 need types 1/2 present to resolve one.
	ctx := SetEntityTypes(navStateCase{role: 3, path: "/campaigns/c1"}.ctx(), []SidebarEntityType{
		{ID: 1, Slug: "locations"}, {ID: 2, Slug: "factions"},
	})
	var top, cats bytes.Buffer
	if err := campaignNavTop().Render(ctx, &top); err != nil {
		t.Fatalf("render campaignNavTop: %v", err)
	}
	if err := campaignNavCategories().Render(ctx, &cats); err != nil {
		t.Fatalf("render campaignNavCategories: %v", err)
	}
	topHTML, catHTML := top.String(), cats.String()

	for _, want := range []string{`data-nav-key="dashboard"`, `data-nav-key="app:notes"`, `data-nav-key="app:maps"`} {
		if !strings.Contains(topHTML, want) {
			t.Errorf("campaignNavTop is missing %q", want)
		}
		if strings.Contains(catHTML, want) {
			t.Errorf("campaignNavCategories must not have %q — that belongs in the fixed top", want)
		}
	}
	for _, want := range []string{`data-nav-section="categories"`, `data-nav-section="manage"`, `data-nav-key="cat:1"`} {
		if !strings.Contains(catHTML, want) {
			t.Errorf("campaignNavCategories is missing %q", want)
		}
		if strings.Contains(topHTML, want) {
			t.Errorf("campaignNavTop must not have %q — that belongs in the scrolling categories area", want)
		}
	}
	// A category is a button that opens the drill panel, never a link.
	cat := catHTML[strings.Index(catHTML, `data-nav-key="cat:1"`)-40:]
	cat = cat[:strings.Index(cat, "</button>")+len("</button>")]
	if !strings.Contains(cat, "<button") || !strings.Contains(cat, "data-drill-open=\"/campaigns/c1/sidebar/drill/") {
		t.Errorf("a category row must be a button that opens the drill panel: %s", cat)
	}
}

func TestCampaignNavList_PaintsWhereTheViewerIs(t *testing.T) {
	html := renderNavList(t, navStateCase{role: 1, path: "/campaigns/c1/cities",
		hint: &NavHint{TypeID: 11, PageName: "Waterdeep"}}.ctx())

	if got := strings.Count(html, `class="nav-ring"`); got != 1 {
		t.Fatalf("sidebar has %d living rings, want exactly 1", got)
	}
	if got := strings.Count(html, `aria-current="page"`); got != 1 {
		t.Fatalf("sidebar marks %d rows current, want 1", got)
	}
	cities := html[strings.Index(html, `data-nav-key="cat:11"`):]
	cities = cities[:strings.Index(cities, "</a>")]
	for _, want := range []string{`aria-current="page"`, `class="nav-ring"`, "Waterdeep"} {
		if !strings.Contains(cities, want) {
			t.Errorf("the Cities row is missing %q: %s", want, cities)
		}
	}
	if strings.Contains(html, `id="nav-b-sub-1" hidden`) {
		t.Errorf("the open page's sub-categories must be painted open")
	}
}

func TestCampaignNavList_FoldedSectionNamesTheCurrentPage(t *testing.T) {
	html := renderNavList(t, navStateCase{role: 1, path: "/campaigns/c1/factions",
		folds: NavFolds{"categories": false}}.ctx())
	if !strings.Contains(html, `id="nav-b-categories" hidden`) {
		t.Errorf("a section the viewer folded must be painted folded")
	}
	head := html[strings.Index(html, `data-nav-fold="categories"`)-80:]
	head = head[:strings.Index(head, "</button>")]
	if !strings.Contains(head, "has-cur") || !strings.Contains(head, `<span class="nav-gh-cur">Factions</span>`) {
		t.Errorf("a folded heading must name the current page inside it: %s", head)
	}
	if !strings.Contains(head, `aria-expanded="false"`) {
		t.Errorf("a folded heading must say it is collapsed: %s", head)
	}
}

func TestCampaignNavList_ManageAndEditingAreTheOwners(t *testing.T) {
	player := renderNavList(t, navStateCase{role: 1, path: "/campaigns/c1"}.ctx())
	for _, owners := range []string{"/campaigns/c1/settings", "/campaigns/c1/customize", "/campaigns/c1/dashboard", "data-nav-edit", "Add category"} {
		if strings.Contains(player, owners) {
			t.Errorf("a player's sidebar contains the owner's %q", owners)
		}
	}
	// Only the owner manages; a player gets no Manage section or Members row.
	if strings.Contains(player, `data-nav-section="manage"`) || strings.Contains(player, `href="/campaigns/c1/members"`) {
		t.Errorf("a player's sidebar must have no Manage section")
	}
	scribe := renderNavList(t, navStateCase{role: 2, path: "/campaigns/c1"}.ctx())
	if strings.Contains(scribe, `data-nav-section="manage"`) {
		t.Errorf("a scribe's sidebar must have no Manage section")
	}
	visitor := renderNavList(t, navStateCase{role: 0, path: "/campaigns/c1"}.ctx())
	if strings.Contains(visitor, `data-nav-section="manage"`) {
		t.Errorf("a visitor who is not a member gets no Manage")
	}
	if !strings.Contains(player, `data-nav-key="me"`) {
		t.Errorf("a player's sidebar is missing My Characters")
	}

	ownerCtx := SetNavEdit(navStateCase{role: 3, path: "/campaigns/c1"}.ctx(), &NavEditView{})
	owner := renderNavList(t, ownerCtx)
	for _, want := range []string{`data-nav-section="manage"`, `id="nav-b-manage" hidden`, "/campaigns/c1/settings", "data-nav-edit"} {
		if !strings.Contains(owner, want) {
			t.Errorf("the owner's sidebar is missing %q", want)
		}
	}
	if strings.Contains(owner, `data-nav-key="me"`) {
		t.Errorf("the owner's sidebar must not have My Characters")
	}

	var brand bytes.Buffer
	if err := campaignNavBrand().Render(navStateCase{role: 1, path: "/campaigns/c1"}.ctx(), &brand); err != nil {
		t.Fatalf("render brand: %v", err)
	}
	if strings.Contains(brand.String(), "data-sidebar-edit-toggle") {
		t.Errorf("the brand row must never carry the edit toggle — it moved to the footer")
	}
	if strings.Contains(renderNavFooter(t, navStateCase{role: 1, path: "/campaigns/c1"}.ctx()), "data-sidebar-edit-toggle") {
		t.Errorf("a player must never get the footer's labelled Edit button")
	}
}

// TestCampaignNavBrand_NeverHasThePencil pins that the brand row is the
// campaign's logo and name only, for every role — editing moved to the
// footer (campaignNavFooter), never back to the brand row.
func TestCampaignNavBrand_NeverHasThePencil(t *testing.T) {
	for _, role := range []int{1, 2, 3} {
		var brand bytes.Buffer
		if err := campaignNavBrand().Render(navStateCase{role: role, path: "/campaigns/c1"}.ctx(), &brand); err != nil {
			t.Fatalf("render brand: %v", err)
		}
		if strings.Contains(brand.String(), "data-sidebar-edit-toggle") || strings.Contains(brand.String(), "fa-pencil") {
			t.Errorf("role %d: the brand row has a pencil; it must be the campaign's logo and name only", role)
		}
	}
}

// TestCampaignNavFooter_EditIsTheOwners pins the footer's labelled Edit
// button: only the owner gets it, and it still wires to the same toggle
// sidebar_editor.js listens for.
func TestCampaignNavFooter_EditIsTheOwners(t *testing.T) {
	owner := renderNavFooter(t, navStateCase{role: 3, path: "/campaigns/c1"}.ctx())
	for _, want := range []string{"data-sidebar-edit-toggle", "chronicle:toggle-sidebar-editor", "Edit", "Campaigns", "Discover"} {
		if !strings.Contains(owner, want) {
			t.Errorf("the owner's footer is missing %q: %s", want, owner)
		}
	}
	player := renderNavFooter(t, navStateCase{role: 1, path: "/campaigns/c1"}.ctx())
	if strings.Contains(player, "data-sidebar-edit-toggle") {
		t.Errorf("a player's footer must not have the Edit button")
	}
}

func TestApp_MarksTheCurrentRowForBoostedNavigation(t *testing.T) {
	ctx := navStateCase{role: 1, path: "/campaigns/c1/cities", hint: &NavHint{TypeID: 11, PageName: "Water<deep>"}}.ctx()
	var buf bytes.Buffer
	if err := App("Waterdeep").Render(ctx, &buf); err != nil {
		t.Fatalf("render App: %v", err)
	}
	html := buf.String()
	main := html[strings.Index(html, `id="main-content"`):]
	if !strings.Contains(main, `data-nav-current="cat:11" data-nav-page="Water&lt;deep&gt;"`) {
		t.Errorf("#main-content must carry the current row and the page's escaped name: %.300s", main)
	}
}

func TestCampaignNavList_ArchivedCampaignDrawsNoWriteControls(t *testing.T) {
	player := renderNavList(t, navStateCase{role: 1, path: "/campaigns/c1"}.ctx())
	if !strings.Contains(player, "data-nav-pin=") {
		t.Fatalf("a player on a live campaign gets their own pins")
	}
	archivedPlayer := renderNavList(t, navStateCase{role: 1, archived: true, path: "/campaigns/c1"}.ctx())
	if strings.Contains(archivedPlayer, "data-nav-pin=") {
		t.Errorf("an archived campaign refuses pins, so none may be drawn")
	}

	ownerCtx := SetNavEdit(navStateCase{role: 3, archived: true, path: "/campaigns/c1"}.ctx(), &NavEditView{})
	owner := renderNavList(t, ownerCtx)
	if strings.Contains(owner, "data-nav-edit") || strings.Contains(renderNavFooter(t, ownerCtx), "data-sidebar-edit-toggle") {
		t.Errorf("an archived campaign refuses sidebar saves, so the owner gets no editor")
	}
}
