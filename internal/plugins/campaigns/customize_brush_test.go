package campaigns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

// The brush's first row is the place's own part of Customize, decided from
// the page's address: a page's category layout, a category's home, the
// campaign home, a plugin's own pages; anywhere else none.
func TestBrushRowFor(t *testing.T) {
	place := brushPlace{
		Base: "/campaigns/c1/customize",
		Cats: []CustomizeCategory{
			{ID: 1, Slug: "character", Name: "Character", NamePlural: "Characters"},
			{ID: 9, Slug: "player-character", Name: "Player Character", NamePlural: "Player Characters"},
		},
		Tabs: []CustomizeTab{
			{ID: "maps", Label: "Maps", Paths: []string{"/maps"}, Here: "The frame around maps"},
			{ID: "dice", Label: "Dice"},
		},
		pageCategory: func(eid string) (int, bool) {
			switch eid {
			case "tessaly":
				return 1, true
			case "pip":
				return 9, true
			}
			return 0, false
		},
	}
	tests := []struct {
		name, rel string
		want      *brushRow
	}{
		{"a character's page", "/entities/tessaly", &brushRow{"How Character pages are laid out", "Customize › Categories › Characters", "/campaigns/c1/customize/categories/1/layout"}},
		{"a kind's page uses the kind", "/entities/pip", &brushRow{"How Player Character pages are laid out", "Customize › Categories › Player Characters", "/campaigns/c1/customize/categories/9/layout"}},
		{"a page that isn't this campaign's", "/entities/someone-else", nil},
		{"a page's editor is not the page", "/entities/tessaly/edit", nil},
		{"the new page form", "/entities/new", nil},
		{"a category's home", "/character", &brushRow{"How the Characters home is laid out", "Customize › Categories › Characters", "/campaigns/c1/customize/categories/1/home"}},
		{"the campaign home", "", &brushRow{"How the campaign home is laid out", "Customize › Home pages", "/campaigns/c1/customize/home"}},
		{"the campaign home with a slash", "/", &brushRow{"How the campaign home is laid out", "Customize › Home pages", "/campaigns/c1/customize/home"}},
		{"the maps list", "/maps", &brushRow{"The frame around maps", "Customize › Maps", "/campaigns/c1/customize/x/maps"}},
		{"a map", "/maps/abc", &brushRow{"The frame around maps", "Customize › Maps", "/campaigns/c1/customize/x/maps"}},
		{"a look-alike path is not the plugin's", "/mapsfoo", nil},
		{"anywhere else", "/journal", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := brushRowFor(tt.rel, place)
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// The endpoint answers only for a path inside this campaign.
func TestCustomizeHere_OtherCampaignHasNoRow(t *testing.T) {
	h := &Handler{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/customize/here?path=/campaigns/c2", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyCampaign, &CampaignContext{Campaign: &Campaign{ID: "c1"}, MemberRole: RoleOwner})
	if err := h.CustomizeHere(c); err != nil {
		t.Fatal(err)
	}
	if rec.Body.String() != "{\"row\":null}\n" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

// A tab's summary is the plugin's own words; a panicking one reads as none,
// and a tab whose add-on is off is left out of the block and the brush.
func TestCustomizeTabs_SummaryAndAddon(t *testing.T) {
	h := &Handler{}
	h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
		return CustomizeTab{ID: "maps", Label: "Maps", Content: tabBody("m"), Addon: "maps",
			Summary: func(context.Context) CustomizeSummary { return CustomizeSummary{Text: "Atlas frame"} }}
	})
	h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
		return CustomizeTab{ID: "dice", Label: "Dice", Content: tabBody("d"), SortOrder: 20,
			Summary: func(context.Context) CustomizeSummary { panic("boom") }}
	})
	cc := ctxWithRole(RoleOwner)
	on := h.customizeTabsOn(cc, map[string]bool{"maps": true})
	if len(on) != 2 || on[0].summary(context.Background()).Text != "Atlas frame" || on[1].summary(context.Background()) != (CustomizeSummary{}) {
		t.Fatalf("tabs = %+v", on)
	}
	if off := h.customizeTabsOn(cc, map[string]bool{}); len(off) != 1 || off[0].ID != "dice" {
		t.Errorf("a tab whose add-on is off still shows: %+v", off)
	}
}
