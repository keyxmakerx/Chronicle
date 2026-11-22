package campaigns

import (
	"context"
	"strings"
	"testing"
)

// A plugin's area is listed only while its add-on is on, and the things inside
// it are findable by name; a failed read of the add-ons keeps every area.
func TestCustomizeAreas_AddonAndFinds(t *testing.T) {
	register := func(h *Handler) {
		h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
			return CustomizeTab{ID: "maps", Label: "Maps", Content: tabBody("m"), Addon: "maps",
				Blurb: "The frame around every map.", Finds: []CustomizeFind{{Label: "Map frame", Words: "border"}}}
		})
		h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
			return CustomizeTab{ID: "dice", Label: "Dice", Content: tabBody("d"), SortOrder: 20, Addon: "dice",
				Finds: []CustomizeFind{{Label: "Crit success", Words: "crit"}}}
		})
	}
	cc := ctxWithRole(RoleOwner)
	cases := []struct {
		name      string
		lister    AddonLister
		wantAreas []string
		wantFinds []string
	}{
		{"both on", hubAddonLister{[]PluginHubAddon{{Slug: "maps", Enabled: true}, {Slug: "dice", Enabled: true}}},
			[]string{"x-maps", "x-dice"}, []string{"Map frame", "Crit success"}},
		{"maps off", hubAddonLister{[]PluginHubAddon{{Slug: "maps", Enabled: false}, {Slug: "dice", Enabled: true}}},
			[]string{"x-dice"}, []string{"Crit success"}},
		{"no lister keeps every area", nil, []string{"x-maps", "x-dice"}, []string{"Map frame", "Crit success"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := &Handler{}
			register(h)
			if c.lister != nil {
				h.SetAddonLister(c.lister)
			}
			areas := h.customizeAreas(context.Background(), cc)
			var got []string
			for _, a := range areas {
				if strings.HasPrefix(a.ID, "x-") {
					got = append(got, a.ID)
				}
			}
			if strings.Join(got, ",") != strings.Join(c.wantAreas, ",") {
				t.Errorf("plugin areas = %v, want %v", got, c.wantAreas)
			}
			found := map[string]string{}
			for _, e := range h.customizeFindEntries(cc, areas, nil) {
				found[e.Label] = e.Href
			}
			for _, label := range c.wantFinds {
				if href := found[label]; !strings.Contains(href, "/customize/x/") {
					t.Errorf("Find has no entry %q leading into its tab: %q", label, href)
				}
			}
			if len(c.wantFinds) == 1 && found["Map frame"] != "" {
				t.Errorf("Find offers the map frame while the Maps add-on is off")
			}
		})
	}
}

// A picture with no summary says what its tab holds, not that a feature adds it.
func TestCustomizeWall_PluginPictureSaysWhatIsInside(t *testing.T) {
	groups := buildWallGroups(wallInput{Base: "/c", Apps: []wallApp{
		{ID: "dice", Label: "Dice", Line: "The dice everyone rolls."},
		{ID: "bare", Label: "Bare"},
	}})
	var sb strings.Builder
	if err := customizeWall("Ashfall", groups, wallSkin{Campaign: "Ashfall"}).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, "The dice everyone rolls.") {
		t.Errorf("the Dice picture does not say what is inside it")
	}
	if strings.Contains(out, "feature adds") {
		t.Errorf("a picture still says a feature adds its settings")
	}
}

// A tab's Section reaches its area (that is what gives the area its dot and
// its label in the save bar), and a tab with a Section is drawn as a workshop,
// without the "saves by itself" note a self-saving tab carries.
func TestCustomizeAreas_TabSectionReachesArea(t *testing.T) {
	h := &Handler{}
	h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
		return CustomizeTab{ID: "heroes", Label: "Hero creator", Content: tabBody("workshop"), Section: "herolooks"}
	})
	h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
		return CustomizeTab{ID: "dice", Label: "Dice", Content: tabBody("dice"), SortOrder: 20}
	})
	sections := map[string]string{}
	for _, a := range h.customizeAreas(context.Background(), ctxWithRole(RoleOwner)) {
		sections[a.ID] = a.Section
	}
	if sections["x-heroes"] != "herolooks" {
		t.Errorf("the tab's section did not reach its area: %q", sections["x-heroes"])
	}
	if sections["x-dice"] != "" {
		t.Errorf("a tab with no section got one: %q", sections["x-dice"])
	}
	for id, wantNote := range map[string]bool{"heroes": false, "dice": true} {
		var sb strings.Builder
		for _, tab := range h.customizeTabs(ctxWithRole(RoleOwner)) {
			if tab.ID != id {
				continue
			}
			if err := customizePluginArea(tab).Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
		}
		if got := strings.Contains(sb.String(), "saves each change"); got != wantNote {
			t.Errorf("%s: self-saving note shown = %v, want %v", id, got, wantNote)
		}
	}
}
