package campaigns

import (
	"reflect"
	"testing"
)

// nav_drawer_test.go pins the apps kept in the app drawer only: stored with
// section "drawer", drawn in no sidebar section, never re-added to Apps, and
// still pinnable by a member for themselves.

func drawerTestItems() []SidebarItem {
	return []SidebarItem{
		{Type: "app", Slug: "notes", Section: "pinned", Visible: true},
		{Type: "app", Slug: "maps", Section: "drawer", Visible: true},
		{Type: "app", Slug: "characters", Section: "drawer", Visible: false},
	}
}

func TestNormalizeNav_DrawerAppsStayOutOfTheSidebar(t *testing.T) {
	l := NormalizeNav(drawerTestItems(), navTestApps(), navTestCats())
	got := layoutKeys(l)
	want := map[string][]string{
		"pinned":     {"app:notes"},
		"apps":       {"app:dates"},
		"categories": {"cat:1", "cat:2", "cat:3"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sections = %v, want %v", got, want)
	}
	var drawer []string
	for _, it := range l.Drawer {
		drawer = append(drawer, it.Key)
	}
	if !reflect.DeepEqual(drawer, []string{"app:maps", "app:characters"}) {
		t.Errorf("drawer = %v", drawer)
	}
	if !l.Drawer[1].Hidden {
		t.Errorf("a drawer app keeps its hidden-from-players flag")
	}
}

func TestViewNav_DrawerAppsAreInNoSidebarButAMemberMayPinThem(t *testing.T) {
	l := NormalizeNav(drawerTestItems(), navTestApps(), navTestCats())
	member := NavViewer{Access: NavAccessMember}

	secs := ViewNav(l, navTestApps(), navTestCats(), member)
	for _, s := range secs {
		for _, r := range s.Rows {
			if r.Key == "app:maps" || r.Key == "app:characters" {
				t.Fatalf("%s is kept in the drawer only but is in %s", r.Key, s.ID)
			}
		}
	}
	rows := NavDrawerRows(l, navTestApps(), member)
	if len(rows) != 1 || rows[0].Key != "app:maps" {
		t.Fatalf("a member's drawer rows = %v, want only Maps (Characters is hidden from players)", rows)
	}
	if owner := NavDrawerRows(l, navTestApps(), NavViewer{Owner: true, Access: NavAccessMember}); len(owner) != 2 || !owner[1].Hidden {
		t.Errorf("the owner's drawer rows = %v, want both, Characters marked hidden", owner)
	}

	member.Pins = []string{"app:maps", "app:characters"}
	got := viewKeys(ViewNav(l, navTestApps(), navTestCats(), member))
	if !reflect.DeepEqual(got["pinned"], []string{"app:notes", "app:maps"}) {
		t.Errorf("pinned = %v: Maps comes from the drawer, Characters (hidden) never does", got["pinned"])
	}
	if len(got) != 3 {
		t.Errorf("the drawer must not become a sidebar section: %v", got)
	}
	if rest := NavDrawerRows(l, navTestApps(), member); len(rest) != 0 {
		t.Errorf("a pinned drawer app leaves the drawer list: %v", rest)
	}
}

func TestPinnableNavKeys_CountsTheDrawer(t *testing.T) {
	l := NormalizeNav(drawerTestItems(), navTestApps(), navTestCats())
	viewer := NavViewer{Access: NavAccessMember}
	secs := append(ViewNav(l, navTestApps(), navTestCats(), viewer),
		NavSection{ID: NavSectionDrawer, Rows: NavDrawerRows(l, navTestApps(), viewer)})
	p := PinnableNavKeys(secs)
	if !p["app:maps"] || p["app:characters"] {
		t.Errorf("pinnable = %v: Maps yes, Characters (hidden from players) no", p)
	}
}

func TestValidateSidebarItems_DrawerHoldsOnlyApps(t *testing.T) {
	if _, err := validateSidebarItems(drawerTestItems()); err != nil {
		t.Fatalf("apps in the drawer are valid: %v", err)
	}
	for name, it := range map[string]SidebarItem{
		"a category": {Type: "category", TypeID: 1, Section: "drawer", Visible: true},
		"a link":     {Type: "link", ID: "lnk_1", Label: "Wiki", URL: "https://example.test", Section: "drawer", Visible: true},
	} {
		if _, err := validateSidebarItems([]SidebarItem{it}); err == nil {
			t.Errorf("%s in the drawer must be refused", name)
		}
	}
}
