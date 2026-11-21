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

// A card with no summary says what its tab holds, not that a feature adds it.
func TestCustomizeOverview_PluginCardSaysWhatIsInside(t *testing.T) {
	areas := []CustomizeArea{{ID: "overview", Label: "Overview"}, {ID: "x-dice", Label: "Dice", Href: "/d", Blurb: "The dice everyone rolls."}, {ID: "x-bare", Label: "Bare", Href: "/b"}}
	var sb strings.Builder
	if err := customizeOverview(areas, map[string]CustomizeSummary{}).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, "The dice everyone rolls.") {
		t.Errorf("the Dice card does not say what is inside it")
	}
	if strings.Contains(out, "feature adds") {
		t.Errorf("a card still says a feature adds its settings")
	}
}
