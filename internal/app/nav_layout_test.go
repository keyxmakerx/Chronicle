package app

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// nav_layout_test.go pins the sidebar pipeline end to end: which apps a
// campaign offers, the categories with their sub-category rows, and above all
// that a row hidden from players never reaches a player — not in the sidebar
// HTML, not in the command palette's data.

func navTestTypes() []layouts.SidebarEntityType {
	parent := 1
	return []layouts.SidebarEntityType{
		{ID: 1, Slug: "locations", Name: "Location", NamePlural: "Locations", Icon: "fa-map-pin", Color: "#f87171"},
		{ID: 11, Slug: "cities", Name: "City", NamePlural: "Cities", Color: "#f87171", SortOrder: 1, ParentTypeID: &parent},
		{ID: 12, Slug: "regions", Name: "Region", NamePlural: "Regions", Color: "#f87171", SortOrder: 0, ParentTypeID: &parent},
		{ID: 2, Slug: "factions", Name: "Faction", NamePlural: "Factions", Icon: `fa-x" onmouseover="alert(1)`, Color: "#fbbf24"},
	}
}

func navTestEnabled() map[string]bool {
	return map[string]bool{"notes": true, calendar.PluginSlug: true, "maps": true, "npcs": true}
}

var navTestSystem = layouts.EnabledSystem{Slug: "drawsteel", Name: "Draw Steel", Icon: "fa-dragon"}

func navTestContext(items []campaigns.SidebarItem, role campaigns.Role, member, anonymous bool) *campaigns.CampaignContext {
	raw, _ := json.Marshal(campaigns.SidebarConfig{Items: items})
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1", SidebarConfig: string(raw)},
		MemberRole:  role,
		IsMember:    member,
		IsAnonymous: anonymous,
	}
}

// hiddenTestItems hides Maps, the Factions category and a link in the owner's
// own section from players.
func hiddenTestItems() []campaigns.SidebarItem {
	return []campaigns.SidebarItem{
		{Type: "app", Slug: "maps", Section: "apps", Visible: false},
		{Type: "category", TypeID: 2, Visible: false},
		{Type: "section", ID: "sec_gm", Label: "Prep", Visible: true},
		{Type: "link", ID: "lnk_1", Label: "Secret Tunnel", URL: "/campaigns/camp-1/tunnel", Section: "sec_gm", Visible: false},
	}
}

func rowsByKey(secs []layouts.NavSectionView) map[string]layouts.NavRowView {
	out := map[string]layouts.NavRowView{}
	for _, s := range secs {
		for _, r := range s.Rows {
			out[r.Key] = r
		}
	}
	return out
}

func TestNavAppsFor(t *testing.T) {
	apps := navAppsFor("camp-1", navTestEnabled(), navTestSystem)
	got := map[string]campaigns.NavApp{}
	for _, a := range apps {
		got[a.Slug] = a
	}
	// Every other app in the catalog has none of its addons on, so it is off.
	wantOn := map[string]bool{
		"notes": true, calendar.PluginSlug: true, "sessions": true, "maps": true,
		"characters": true, "rulebook": true,
	}
	for _, a := range apps {
		if a.Enabled != wantOn[a.Slug] {
			t.Errorf("%s enabled = %v, want %v", a.Slug, a.Enabled, wantOn[a.Slug])
		}
	}
	if got["characters"].Caption != "Party & NPCs" {
		t.Errorf("Characters caption = %q", got["characters"].Caption)
	}
	rb := got["rulebook"]
	if rb.URL != "/campaigns/camp-1/systems/drawsteel" || rb.Label != "Draw Steel" || rb.Icon != "fa-dragon" {
		t.Errorf("rulebook = %+v", rb)
	}
	for _, a := range apps {
		if strings.HasSuffix(a.URL, "/npcs") {
			t.Errorf("no app may link to the NPC gallery; Characters lists NPCs: %+v", a)
		}
	}
	badIcon := navAppsFor("camp-1", nil, layouts.EnabledSystem{Slug: "x", Icon: `"><b>`})
	for _, a := range badIcon {
		if a.Slug == "rulebook" && a.Icon != "fa-book" {
			t.Errorf("a system icon that is not a Font Awesome name must fall back, got %q", a.Icon)
		}
	}
}

func TestNavCategoriesFor(t *testing.T) {
	cats := navCategoriesFor("camp-1", navTestTypes(), map[int]int{1: 9, 11: 4, 12: 5, 2: 3})
	if len(cats) != 2 {
		t.Fatalf("top-level categories = %d, want 2 (sub-categories nest under their parent)", len(cats))
	}
	loc := cats[0]
	if loc.URL != "/campaigns/camp-1/locations" || loc.Count != 9 {
		t.Errorf("Locations = %+v", loc)
	}
	if len(loc.Subs) != 2 || loc.Subs[0].Label != "Regions" || loc.Subs[1].Label != "Cities" {
		t.Errorf("sub-categories must follow their own order: %+v", loc.Subs)
	}
	if loc.Subs[1].URL != "/campaigns/camp-1/cities" || loc.Subs[1].Count != 4 {
		t.Errorf("Cities = %+v", loc.Subs[1])
	}
	if cats[1].Icon != "" {
		t.Errorf("an icon that is not a Font Awesome name must be dropped, got %q", cats[1].Icon)
	}
}

func TestBuildNavSections_HiddenRowsReachOnlyTheOwner(t *testing.T) {
	hiddenKeys := []string{"app:maps", "cat:2", "link:lnk_1"}
	tests := []struct {
		name       string
		role       campaigns.Role
		owner      bool // effective: false for an owner viewing as a player
		wantHidden bool
	}{
		{name: "the owner sees hidden rows, marked", role: campaigns.RoleOwner, owner: true, wantHidden: true},
		{name: "a player never gets them", role: campaigns.RolePlayer, owner: false},
		{name: "a scribe never gets them", role: campaigns.RoleScribe, owner: false},
		{name: "an owner viewing as a player does not get them", role: campaigns.RoleOwner, owner: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := navTestContext(hiddenTestItems(), tt.role, true, false)
			rows := rowsByKey(buildNavSections(cc, tt.owner, nil, navTestTypes(), nil, navTestEnabled(), navTestSystem))
			for _, key := range hiddenKeys {
				row, shown := rows[key]
				if tt.wantHidden && (!shown || !row.Hidden) {
					t.Errorf("%s: shown=%v hidden=%v, want it shown and marked", key, shown, row.Hidden)
				}
				if !tt.wantHidden && shown {
					t.Errorf("%s reached a viewer it is hidden from", key)
				}
			}
		})
	}
}

func TestBuildNavSections_AppsFollowAccess(t *testing.T) {
	tests := []struct {
		name string
		cc   *campaigns.CampaignContext
		want map[string]bool
	}{
		{
			name: "a public visitor",
			cc:   navTestContext(nil, campaigns.RoleNone, false, true),
			want: map[string]bool{"app:notes": false, "app:" + calendar.PluginSlug: false, "app:maps": true, "app:characters": false, "app:rulebook": false, "app:sessions": true},
		},
		{
			name: "a signed-in visitor who is not a member",
			cc:   navTestContext(nil, campaigns.RoleNone, false, false),
			want: map[string]bool{"app:notes": false, "app:" + calendar.PluginSlug: true, "app:maps": true, "app:characters": false, "app:rulebook": true},
		},
		{
			name: "a player",
			cc:   navTestContext(nil, campaigns.RolePlayer, true, false),
			want: map[string]bool{"app:notes": true, "app:" + calendar.PluginSlug: true, "app:maps": true, "app:characters": true, "app:rulebook": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := rowsByKey(buildNavSections(tt.cc, false, nil, navTestTypes(), nil, navTestEnabled(), navTestSystem))
			for key, want := range tt.want {
				if _, got := rows[key]; got != want {
					t.Errorf("%s shown = %v, want %v", key, got, want)
				}
			}
		})
	}
}

func TestBuildNavSections_DefaultsForANewCampaign(t *testing.T) {
	cc := navTestContext(nil, campaigns.RolePlayer, true, false)
	secs := buildNavSections(cc, false, nil, navTestTypes(), nil, navTestEnabled(), navTestSystem)
	keys := map[string][]string{}
	for _, s := range secs {
		for _, r := range s.Rows {
			keys[s.ID] = append(keys[s.ID], r.Key)
		}
	}
	if got := strings.Join(keys["pinned"], ","); got != "app:notes,app:"+calendar.PluginSlug {
		t.Errorf("pinned = %s", got)
	}
	if got := strings.Join(keys["apps"], ","); got != "app:sessions,app:maps,app:characters,app:rulebook" {
		t.Errorf("apps = %s", got)
	}
	if got := strings.Join(keys["categories"], ","); got != "cat:1,cat:2" {
		t.Errorf("categories = %s", got)
	}
}

// navTestLayoutCtx is the template context the LayoutInjector would build for
// this viewer.
func navTestLayoutCtx(secs []layouts.NavSectionView, role int) context.Context {
	ctx := context.Background()
	ctx = layouts.SetIsAuthenticated(ctx, true)
	ctx = layouts.SetCampaignID(ctx, "camp-1")
	ctx = layouts.SetCampaignRole(ctx, role)
	ctx = layouts.SetEntityTypes(ctx, navTestTypes())
	ctx = layouts.SetNavSections(ctx, secs)
	return ctx
}

func TestNavCommandsJSON_OffersOnlyTheViewersRows(t *testing.T) {
	iconRe := regexp.MustCompile(`^fa-[a-z0-9-]{1,40}$`)
	decode := func(raw string) map[string]string {
		var list []struct{ Label, Href, Icon string }
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			t.Fatalf("palette data is not JSON: %v\n%s", err, raw)
		}
		out := map[string]string{}
		for _, c := range list {
			if c.Icon != "" && !iconRe.MatchString(c.Icon) {
				t.Errorf("palette icon %q is not a Font Awesome name", c.Icon)
			}
			out[c.Label] = c.Href
		}
		return out
	}

	player := navTestContext(hiddenTestItems(), campaigns.RolePlayer, true, false)
	playerCmds := decode(layouts.NavCommandsJSON(navTestLayoutCtx(
		buildNavSections(player, false, nil, navTestTypes(), nil, navTestEnabled(), navTestSystem), int(campaigns.RolePlayer))))
	for _, hidden := range []string{"Maps", "Factions", "Secret Tunnel"} {
		if _, ok := playerCmds[hidden]; ok {
			t.Errorf("a player's palette offers %q, which is hidden from players", hidden)
		}
	}
	for _, want := range []string{"Dashboard", "Journal", "Locations", "Cities", "All Pages"} {
		if _, ok := playerCmds[want]; !ok {
			t.Errorf("a player's palette is missing %q: %v", want, playerCmds)
		}
	}
	for _, manage := range []string{"Members", "Customize", "Extensions", "Settings", "Owner dashboard"} {
		if _, ok := playerCmds[manage]; ok {
			t.Errorf("a player's palette offers the owner's %q", manage)
		}
	}

	owner := navTestContext(hiddenTestItems(), campaigns.RoleOwner, true, false)
	ownerCmds := decode(layouts.NavCommandsJSON(navTestLayoutCtx(
		buildNavSections(owner, true, nil, navTestTypes(), nil, navTestEnabled(), navTestSystem), int(campaigns.RoleOwner))))
	for _, want := range []string{"Maps", "Factions", "Secret Tunnel", "Members", "Settings", "Customize", "Owner dashboard"} {
		if _, ok := ownerCmds[want]; !ok {
			t.Errorf("the owner's palette is missing %q", want)
		}
	}
}

func TestSidebar_PlayerHTMLHasNoHiddenRow(t *testing.T) {
	render := func(cc *campaigns.CampaignContext, owner bool, role campaigns.Role) string {
		secs := buildNavSections(cc, owner, nil, navTestTypes(), nil, navTestEnabled(), navTestSystem)
		var buf bytes.Buffer
		if err := layouts.Sidebar().Render(navTestLayoutCtx(secs, int(role)), &buf); err != nil {
			t.Fatalf("render Sidebar: %v", err)
		}
		return buf.String()
	}

	playerHTML := render(navTestContext(hiddenTestItems(), campaigns.RolePlayer, true, false), false, campaigns.RolePlayer)
	for _, leak := range []string{"Secret Tunnel", "/campaigns/camp-1/tunnel", "/campaigns/camp-1/maps", "/campaigns/camp-1/factions", "hidden from players"} {
		if strings.Contains(playerHTML, leak) {
			t.Errorf("a player's sidebar HTML contains %q", leak)
		}
	}

	ownerHTML := render(navTestContext(hiddenTestItems(), campaigns.RoleOwner, true, false), true, campaigns.RoleOwner)
	for _, want := range []string{"Secret Tunnel", "/campaigns/camp-1/maps", "/campaigns/camp-1/factions"} {
		if !strings.Contains(ownerHTML, want) {
			t.Errorf("the owner's sidebar is missing %q", want)
		}
	}
	if got := strings.Count(ownerHTML, "(hidden from players)"); got != 3 {
		t.Errorf("the owner's sidebar marks %d rows hidden, want 3", got)
	}
}

func TestBuildNavEdit_KeepsTheWholeArrangementForTheOwner(t *testing.T) {
	enabled := navTestEnabled()
	delete(enabled, "maps") // turned off in Extensions, but arranged: it keeps its place
	items := append(hiddenTestItems(), campaigns.SidebarItem{Type: "app", Slug: "notes", Section: "pinned", Visible: true})
	cc := navTestContext(items, campaigns.RoleOwner, true, false)
	edit := buildNavEdit(navInputsFor(cc, navTestTypes(), map[int]int{1: 9}, enabled, navTestSystem))

	rows := map[string]layouts.NavEditRow{}
	kinds := map[string]string{}
	for _, s := range edit.Sections {
		kinds[s.ID] = s.Kind
		for _, r := range s.Items {
			rows[r.Key] = r
		}
	}
	if kinds["pinned"] != "pinned" || kinds["apps"] != "apps" || kinds["categories"] != "categories" || kinds["sec_gm"] != "custom" {
		t.Fatalf("section kinds = %v", kinds)
	}
	if r := rows["app:maps"]; !r.Off || !r.Hidden || r.Label != "Maps" {
		t.Errorf("a turned-off, hidden app must keep its place, marked: %+v", r)
	}
	if r := rows["cat:2"]; !r.Hidden || r.Icon != "" || r.Color != "#fbbf24" {
		t.Errorf("a hidden category keeps its colour and loses a poisoned icon: %+v", r)
	}
	if r := rows["link:lnk_1"]; !r.Hidden || r.URL != "/campaigns/camp-1/tunnel" || r.Kind != "link" {
		t.Errorf("a hidden link keeps its target: %+v", r)
	}
	if _, ok := rows["cat:11"]; ok {
		t.Errorf("sub-categories move with their parent and have no row of their own in the editor")
	}
	off := map[string]bool{}
	for _, r := range edit.Off {
		off[r.Key] = r.Off
	}
	if !off["app:maps"] || off["app:notes"] {
		t.Errorf("the tray lists the apps turned off, and only those: %v", off)
	}
}

func TestSidebar_EditorDataReachesOnlyTheOwner(t *testing.T) {
	cc := navTestContext(hiddenTestItems(), campaigns.RoleOwner, true, false)
	in := navInputsFor(cc, navTestTypes(), nil, navTestEnabled(), navTestSystem)
	edit := buildNavEdit(in)
	render := func(role int, viewingAsPlayer bool) string {
		ctx := navTestLayoutCtx(viewNavSections(cc, in, role >= 3, nil), role)
		ctx = layouts.SetViewingAsPlayer(ctx, viewingAsPlayer)
		ctx = layouts.SetNavEdit(ctx, edit) // even if it were set, only an owner's page carries it
		var buf bytes.Buffer
		if err := layouts.Sidebar().Render(ctx, &buf); err != nil {
			t.Fatalf("render Sidebar: %v", err)
		}
		return buf.String()
	}
	owner := render(int(campaigns.RoleOwner), false)
	if !strings.Contains(owner, "data-nav-edit=") || !strings.Contains(owner, "Secret Tunnel") {
		t.Fatalf("the owner's sidebar must carry the editor's arrangement")
	}
	for name, html := range map[string]string{
		"player":                  render(int(campaigns.RolePlayer), false),
		"scribe":                  render(int(campaigns.RoleScribe), false),
		"owner viewing as player": render(int(campaigns.RolePlayer), true),
	} {
		if strings.Contains(html, "data-nav-edit") || strings.Contains(html, "data-sidebar-edit-toggle") {
			t.Errorf("%s: the sidebar carries the editor or its pencil", name)
		}
		if strings.Contains(html, "Secret Tunnel") || strings.Contains(html, "&#34;hidden&#34;") {
			t.Errorf("%s: the sidebar leaks a row hidden from players", name)
		}
	}
}
