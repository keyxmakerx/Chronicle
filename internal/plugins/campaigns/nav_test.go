package campaigns

import (
	"reflect"
	"testing"
)

// The fixture apps use neutral slugs: the plugin-isolation guard refuses real
// plugin names as string literals outside their own plugin.
func navTestApps() []NavApp {
	return []NavApp{
		{Slug: "notes", Label: "Journal", URL: "/c/journal", Enabled: true, Access: NavAccessMember, DefaultPinned: true},
		{Slug: "dates", Label: "Dates", URL: "/c/dates", Enabled: true, Access: NavAccessSignedIn, DefaultPinned: true},
		{Slug: "maps", Label: "Maps", URL: "/c/maps", Enabled: true, Access: NavAccessAnyone},
		{Slug: "characters", Label: "Characters", URL: "/c/characters", Caption: "Party & NPCs", Enabled: true, Access: NavAccessMember},
		{Slug: "forge", Label: "Forge", URL: "/c/forge", Enabled: false, Access: NavAccessAnyone},
	}
}

func navTestCats() []NavCategory {
	return []NavCategory{
		{TypeID: 1, Label: "Locations", URL: "/c/locations", Count: 64, Subs: []NavSubcategory{{TypeID: 11, Label: "Cities", URL: "/c/cities", Count: 14}}},
		{TypeID: 2, Label: "Factions", URL: "/c/factions", Count: 18},
		{TypeID: 3, Label: "Items", URL: "/c/items", Count: 53},
	}
}

// layoutKeys flattens a layout to section id → item keys, for comparisons.
func layoutKeys(l NavLayout) map[string][]string {
	out := map[string][]string{}
	for _, s := range l.Sections {
		keys := []string{}
		for _, it := range s.Items {
			keys = append(keys, it.Key)
		}
		out[s.ID] = keys
	}
	return out
}

// viewKeys flattens a viewer's sections to section id → row keys.
func viewKeys(secs []NavSection) map[string][]string {
	out := map[string][]string{}
	for _, s := range secs {
		keys := []string{}
		for _, r := range s.Rows {
			keys = append(keys, r.Key)
		}
		out[s.ID] = keys
	}
	return out
}

func TestNormalizeNav(t *testing.T) {
	tests := []struct {
		name  string
		items []SidebarItem
		want  map[string][]string
	}{
		{
			name:  "a campaign that never arranged its sidebar gets the defaults",
			items: nil,
			want: map[string][]string{
				"pinned":     {"app:notes", "app:dates"},
				"apps":       {"app:maps", "app:characters"},
				"categories": {"cat:1", "cat:2", "cat:3"},
			},
		},
		{
			name: "items saved before sections land where the old sidebar drew them",
			items: []SidebarItem{
				{Type: "dashboard", Visible: true},
				{Type: "addon", Slug: "notes", Visible: true},
				{Type: "category", TypeID: 2, Visible: true},
				{Type: "section", ID: "sec_a", Label: "Lore", Visible: true},
				{Type: "link", ID: "lnk_1", Label: "Wiki", URL: "https://example.test", Visible: true},
				{Type: "addon", Slug: "maps", Visible: true},
				{Type: "category", TypeID: 1, Visible: true},
				{Type: "all_pages", Visible: true},
			},
			want: map[string][]string{
				"pinned":     {"app:notes"},
				"apps":       {"app:dates", "app:characters"},
				"categories": {"cat:2", "cat:1", "cat:3"},
				"sec_a":      {"link:lnk_1", "app:maps"},
			},
		},
		{
			name: "stored sections are kept, and a section that cannot hold a row sends it home",
			items: []SidebarItem{
				{Type: "app", Slug: "maps", Section: "pinned", Visible: true},
				{Type: "category", TypeID: 3, Section: "pinned", Visible: true},
				{Type: "app", Slug: "notes", Section: "apps", Visible: true},
				{Type: "category", TypeID: 2, Section: "apps", Visible: true},
				{Type: "app", Slug: "dates", Section: "gone", Visible: true},
				{Type: "section", ID: "sec_b", Label: "At the table", Visible: true},
				{Type: "category", TypeID: 1, Section: "sec_b", Visible: true},
				{Type: "link", ID: "lnk_2", Label: "Rules", URL: "/rules", Section: "sec_b", Visible: true},
			},
			want: map[string][]string{
				"pinned":     {"app:maps", "cat:3"},
				"apps":       {"app:notes", "app:dates", "app:characters"},
				"categories": {"cat:2"},
				"sec_b":      {"cat:1", "link:lnk_2"},
			},
		},
		{
			name: "unknown, deleted and repeated items are dropped",
			items: []SidebarItem{
				{Type: "addon", Slug: "npcs", Visible: true},
				{Type: "category", TypeID: 99, Visible: true},
				{Type: "category", TypeID: 1, Visible: true},
				{Type: "category", TypeID: 1, Section: "pinned", Visible: true},
				{Type: "section", ID: "pinned", Label: "Imposter", Visible: true},
				{Type: "section", ID: "bad id!", Label: "Broken", Visible: true},
				{Type: "mystery", Visible: true},
			},
			want: map[string][]string{
				"pinned":     {},
				"apps":       {"app:notes", "app:dates", "app:maps", "app:characters"},
				"categories": {"cat:1", "cat:2", "cat:3"},
			},
		},
		{
			name: "an app turned off keeps its place but is not added when missing",
			items: []SidebarItem{
				{Type: "app", Slug: "forge", Section: "pinned", Visible: true},
			},
			want: map[string][]string{
				"pinned":     {"app:forge"},
				"apps":       {"app:notes", "app:dates", "app:maps", "app:characters"},
				"categories": {"cat:1", "cat:2", "cat:3"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := layoutKeys(NormalizeNav(tt.items, navTestApps(), navTestCats()))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("layout =\n  %v\nwant\n  %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeNav_SectionOrderAndLabels(t *testing.T) {
	l := NormalizeNav([]SidebarItem{
		{Type: "section", ID: "sec_b", Label: "  Downtime  ", Visible: true},
		{Type: "section", ID: "sec_a", Label: "", Visible: true},
		{Type: "section", ID: "sec_b", Label: "Repeat", Visible: true},
	}, navTestApps(), navTestCats())
	var ids, labels []string
	for _, s := range l.Sections {
		ids = append(ids, s.ID)
		labels = append(labels, s.Label)
	}
	if want := []string{"pinned", "apps", "categories", "sec_b", "sec_a"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("section order = %v, want %v", ids, want)
	}
	if labels[3] != "Downtime" {
		t.Errorf("custom section label = %q, want it trimmed to %q", labels[3], "Downtime")
	}
}

func TestNormalizeNav_KeepsHiddenFlagAndSanitizesLinkIcons(t *testing.T) {
	l := NormalizeNav([]SidebarItem{
		{Type: "category", TypeID: 2, Visible: false},
		{Type: "link", ID: "lnk_1", Label: "Wiki", URL: "/wiki", Icon: "fixed inset-0 z-50", Visible: true},
		{Type: "link", ID: "lnk_2", Label: "Map", URL: "/map", Icon: "fa-globe", Visible: true},
	}, navTestApps(), navTestCats())
	byKey := map[string]NavLayoutItem{}
	for _, s := range l.Sections {
		for _, it := range s.Items {
			byKey[it.Key] = it
		}
	}
	if !byKey["cat:2"].Hidden {
		t.Errorf("a stored visible:false must read as hidden from players")
	}
	if byKey["cat:1"].Hidden {
		t.Errorf("an item added because it was missing must not be hidden")
	}
	if got := byKey["link:lnk_1"].Icon; got != "" {
		t.Errorf("a non-Font-Awesome icon must be dropped, got %q", got)
	}
	if got := byKey["link:lnk_2"].Icon; got != "fa-globe" {
		t.Errorf("icon = %q, want fa-globe", got)
	}
}

func TestViewNav_HiddenRowsReachOnlyTheOwner(t *testing.T) {
	layout := NormalizeNav([]SidebarItem{
		{Type: "app", Slug: "maps", Section: "apps", Visible: false},
		{Type: "category", TypeID: 2, Visible: false},
		{Type: "section", ID: "sec_x", Label: "Secrets", Visible: true},
		{Type: "link", ID: "lnk_1", Label: "GM notes", URL: "/gm", Section: "sec_x", Visible: false},
	}, navTestApps(), navTestCats())

	owner := ViewNav(layout, navTestApps(), navTestCats(), NavViewer{Owner: true, Access: NavAccessMember})
	player := ViewNav(layout, navTestApps(), navTestCats(), NavViewer{Owner: false, Access: NavAccessMember})

	hiddenFor := func(secs []NavSection) map[string]bool {
		out := map[string]bool{}
		for _, s := range secs {
			for _, r := range s.Rows {
				out[r.Key] = r.Hidden
			}
		}
		return out
	}
	ownerRows := hiddenFor(owner)
	for _, key := range []string{"app:maps", "cat:2", "link:lnk_1"} {
		hidden, shown := ownerRows[key]
		if !shown || !hidden {
			t.Errorf("owner must see %s marked hidden (shown=%v hidden=%v)", key, shown, hidden)
		}
	}
	playerRows := hiddenFor(player)
	for _, key := range []string{"app:maps", "cat:2", "link:lnk_1"} {
		if _, shown := playerRows[key]; shown {
			t.Errorf("a player must never be given %s, which is hidden from players", key)
		}
	}
	for key, hidden := range playerRows {
		if hidden {
			t.Errorf("a player was given a row marked hidden: %s", key)
		}
	}
	// The custom section still exists for the player but is empty, so the
	// sidebar leaves it out.
	if got := viewKeys(player)["sec_x"]; len(got) != 0 {
		t.Errorf("player's Secrets section = %v, want empty", got)
	}
}

func TestViewNav_AppsNeedAccessAndMustBeOn(t *testing.T) {
	layout := NormalizeNav(nil, navTestApps(), navTestCats())
	tests := []struct {
		name   string
		access NavAccess
		want   []string
	}{
		{name: "a public visitor gets only public apps", access: NavAccessAnyone, want: []string{"app:maps"}},
		{name: "a signed-in visitor also gets signed-in apps", access: NavAccessSignedIn, want: []string{"app:dates", "app:maps"}},
		{name: "a member gets every app that is on", access: NavAccessMember, want: []string{"app:notes", "app:dates", "app:maps", "app:characters"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secs := ViewNav(layout, navTestApps(), navTestCats(), NavViewer{Access: tt.access})
			var got []string
			for _, s := range secs {
				for _, r := range s.Rows {
					if r.Kind == NavRowApp {
						got = append(got, r.Key)
					}
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("apps = %v, want %v", got, tt.want)
			}
		})
	}
	// Turned off in Extensions: never shown, even to the owner.
	forge := NormalizeNav([]SidebarItem{{Type: "app", Slug: "forge", Section: "pinned", Visible: true}}, navTestApps(), navTestCats())
	for _, s := range ViewNav(forge, navTestApps(), navTestCats(), NavViewer{Owner: true, Access: NavAccessMember}) {
		for _, r := range s.Rows {
			if r.Key == "app:forge" {
				t.Errorf("an app that is turned off must not be shown")
			}
		}
	}
}

func TestViewNav_PersonalPins(t *testing.T) {
	layout := NormalizeNav(nil, navTestApps(), navTestCats())
	hiddenLayout := NormalizeNav([]SidebarItem{{Type: "category", TypeID: 3, Visible: false}}, navTestApps(), navTestCats())

	tests := []struct {
		name   string
		layout NavLayout
		viewer NavViewer
		want   map[string][]string
	}{
		{
			name:   "a player's pins follow the campaign's, in the order pinned",
			layout: layout,
			viewer: NavViewer{Access: NavAccessMember, Pins: []string{"cat:2", "app:maps", "cat:99", "app:notes"}},
			want: map[string][]string{
				"pinned":     {"app:notes", "app:dates", "cat:2", "app:maps"},
				"apps":       {"app:characters"},
				"categories": {"cat:1", "cat:3"},
			},
		},
		{
			name:   "the owner pins for everyone, so personal pins are ignored",
			layout: layout,
			viewer: NavViewer{Owner: true, Access: NavAccessMember, Pins: []string{"cat:2"}},
			want: map[string][]string{
				"pinned":     {"app:notes", "app:dates"},
				"apps":       {"app:maps", "app:characters"},
				"categories": {"cat:1", "cat:2", "cat:3"},
			},
		},
		{
			name:   "a pin cannot bring back a row hidden from players",
			layout: hiddenLayout,
			viewer: NavViewer{Access: NavAccessMember, Pins: []string{"cat:3"}},
			want: map[string][]string{
				"pinned":     {},
				"apps":       {"app:notes", "app:dates", "app:maps", "app:characters"},
				"categories": {"cat:1", "cat:2"},
			},
		},
		{
			name:   "a pin cannot bring in an app the viewer may not open",
			layout: layout,
			viewer: NavViewer{Access: NavAccessAnyone, Pins: []string{"app:characters", "app:maps"}},
			want: map[string][]string{
				"pinned":     {"app:maps"},
				"apps":       {},
				"categories": {"cat:1", "cat:2", "cat:3"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secs := ViewNav(tt.layout, navTestApps(), navTestCats(), tt.viewer)
			got := viewKeys(secs)
			for id, want := range tt.want {
				if g := got[id]; !reflect.DeepEqual(append([]string{}, g...), want) {
					t.Errorf("%s = %v, want %v", id, g, want)
				}
			}
			seen := map[string]bool{}
			for _, s := range secs {
				for _, r := range s.Rows {
					if seen[r.Key] {
						t.Errorf("row %s is shown twice", r.Key)
					}
					seen[r.Key] = true
					personal := s.ID == "pinned" && r.Personal
					if r.Personal && !personal {
						t.Errorf("row %s is marked personal outside Pinned", r.Key)
					}
				}
			}
		})
	}
}

func TestViewNav_Rows(t *testing.T) {
	layout := NormalizeNav([]SidebarItem{
		{Type: "section", ID: "sec_l", Label: "", Visible: true},
		{Type: "link", ID: "lnk_js", Label: "Bad", URL: "javascript:alert(1)", Section: "sec_l", Visible: true},
		{Type: "link", ID: "lnk_rel", Label: "Rules", URL: "/rules", Section: "sec_l", Visible: true},
		{Type: "link", ID: "lnk_ext", Label: "Wiki", URL: "https://example.test/wiki", Icon: "fa-globe", Section: "sec_l", Visible: true},
	}, navTestApps(), navTestCats())
	secs := ViewNav(layout, navTestApps(), navTestCats(), NavViewer{Owner: true, Access: NavAccessMember})

	var custom NavSection
	for _, s := range secs {
		if s.ID == "sec_l" {
			custom = s
		}
	}
	if custom.Kind != NavKindCustom || custom.Label != "Untitled" {
		t.Errorf("custom section kind/label = %q/%q, want %q/%q", custom.Kind, custom.Label, NavKindCustom, "Untitled")
	}
	if len(custom.Rows) != 2 {
		t.Fatalf("links = %+v, want the unsafe one dropped", custom.Rows)
	}
	if custom.Rows[0].External || !custom.Rows[1].External || custom.Rows[1].Icon != "fa-globe" {
		t.Errorf("external flags/icon wrong: %+v", custom.Rows)
	}

	var locations NavRow
	for _, s := range secs {
		for _, r := range s.Rows {
			if r.Key == "cat:1" {
				locations = r
			}
		}
	}
	if locations.Count != 64 || len(locations.Subs) != 1 || locations.Subs[0].Label != "Cities" {
		t.Errorf("category row lost its count or sub-categories: %+v", locations)
	}
	for _, s := range secs {
		for _, r := range s.Rows {
			if r.Key == "app:characters" && r.Caption != "Party & NPCs" {
				t.Errorf("Characters caption = %q", r.Caption)
			}
		}
	}
}
