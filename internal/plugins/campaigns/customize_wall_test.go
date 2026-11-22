package campaigns

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func tileKeys(g wallGroup) string {
	var ks []string
	for _, t := range g.Tiles {
		ks = append(ks, t.Key)
	}
	return strings.Join(ks, ",")
}

// The wall's groups: the shared parts always, then a picture for each
// category, each home and each app, and a group with nothing in it is left out.
func TestBuildWallGroups(t *testing.T) {
	cats := []CustomizeCategory{
		{ID: 7, Name: "Location", NamePlural: "Locations", Fields: 3, HomeLayout: true, Sample: "The Drowned Bell"},
		{ID: 9, Name: "Character", NamePlural: "Characters", Fields: 1},
	}
	homes := []wallHome{{Key: HomeDefault, Label: "Everyone"}, {Key: HomePlayer, Label: "Players", Own: true}, {Key: HomeGuest, Label: "Guests"}}
	tests := []struct {
		name   string
		in     wallInput
		groups string // group ids in order
		tiles  map[string]string
	}{
		{"nothing but the look", wallInput{Base: "/c"}, "shared",
			map[string]string{"shared": "frame,colours,page,hover,sheet,email"}},
		{"categories, homes and apps", wallInput{Base: "/c", Cats: cats, Homes: homes, Apps: []wallApp{{ID: "maps", Label: "Maps"}, {ID: "dice", Label: "Dice"}}},
			"shared,kinds,homes,apps",
			map[string]string{"kinds": "cat-7,cat-9", "homes": "home-default,home-player,home-guest", "apps": "app-maps,app-dice"}},
		{"apps only", wallInput{Base: "/c", Apps: []wallApp{{ID: "hero", Label: "Hero creator"}}}, "shared,apps",
			map[string]string{"apps": "app-hero"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := buildWallGroups(tt.in)
			var ids []string
			for _, g := range gs {
				ids = append(ids, g.ID)
				if want, ok := tt.tiles[g.ID]; ok && tileKeys(g) != want {
					t.Errorf("group %s = %s, want %s", g.ID, tileKeys(g), want)
				}
			}
			if strings.Join(ids, ",") != tt.groups {
				t.Errorf("groups = %v, want %s", ids, tt.groups)
			}
		})
	}
}

// Each picture leads to its own place and says what is set now.
func TestBuildWallGroups_LinksAndLines(t *testing.T) {
	// A look that is no starter's: its header follows the page, its menu is
	// charcoal, its accent is Indigo and its text is Inter.
	var d customizeDraft
	d.Colours.Accent, d.Type.Body = "#6366f1", "inter"
	d.Header.Bg, d.Header.Solid, d.Colours.Sidebar = "solid", "page", "charcoal"
	gs := buildWallGroups(wallInput{
		Base: "/c", Look: d,
		Cats:  []CustomizeCategory{{ID: 7, Name: "Location", NamePlural: "Locations", Fields: 3, HomeLayout: true}, {ID: 9, NamePlural: "Characters", Fields: 1}},
		Homes: []wallHome{{Key: HomeDefault, Label: "Everyone"}, {Key: HomePlayer, Label: "Players", Own: true}, {Key: HomeScribe, Label: "Scribes"}},
		Apps:  []wallApp{{ID: "maps", Label: "Maps", Line: "Atlas frame"}},
	})
	by := map[string]wallTile{}
	for _, g := range gs {
		for _, tl := range g.Tiles {
			by[tl.Key] = tl
		}
	}
	tests := []struct{ key, href, line, share string }{
		{"frame", "/c/look#frame", "Page-coloured header · Charcoal menu", "frame"},
		{"colours", "/c/look#style", "Custom · Indigo · Inter", "colours"},
		{"hover", "/c/look#hover", "Paper", ""},
		{"email", "/c/look#email", "Note", ""},
		{"cat-7", "/c/categories/7/layout", "2 layouts · 3 fields", ""},
		{"cat-9", "/c/categories/9/layout", "1 layout · 1 field", ""},
		{"home-default", "/c/home/default", "The campaign home", ""},
		{"home-player", "/c/home/player", "Its own home", ""},
		{"home-scribe", "/c/home/scribe", "Same as the Everyone home", ""},
		{"app-maps", "/c/x/maps", "Atlas frame", ""},
	}
	for _, tt := range tests {
		got, ok := by[tt.key]
		if !ok {
			t.Errorf("no picture %q", tt.key)
			continue
		}
		if got.Href != tt.href || got.Share != tt.share {
			t.Errorf("%s: href %q share %q, want %q %q", tt.key, got.Href, got.Share, tt.href, tt.share)
		}
		if got.Line != tt.line {
			t.Errorf("%s line = %q, want %q", tt.key, got.Line, tt.line)
		}
	}
}

// The apps are the tabs plugins registered and the campaign shows: a new tab
// adds a picture, and one whose add-on is off adds none.
func TestCustomizeWall_AppsFromRegisteredTabs(t *testing.T) {
	register := func(h *Handler) {
		h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
			return CustomizeTab{ID: "maps", Label: "Maps", Content: tabBody("m"), Addon: "maps", Blurb: "The frame around every map."}
		})
		h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
			return CustomizeTab{ID: "dice", Label: "Dice", Content: tabBody("d"), SortOrder: 20, Addon: "dice",
				Summary: func(context.Context) CustomizeSummary { return CustomizeSummary{Text: "Bone", Sub: "players pick"} }}
		})
		h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
			return CustomizeTab{ID: "heroes", Label: "Hero creator", Content: tabBody("h"), SortOrder: 30}
		})
	}
	tests := []struct {
		name   string
		lister AddonLister
		want   string
	}{
		{"all on", hubAddonLister{[]PluginHubAddon{{Slug: "maps", Enabled: true}, {Slug: "dice", Enabled: true}}}, "app-maps,app-dice,app-heroes"},
		{"maps off", hubAddonLister{[]PluginHubAddon{{Slug: "maps", Enabled: false}, {Slug: "dice", Enabled: true}}}, "app-dice,app-heroes"},
		{"add-ons unreadable keeps every tab", nil, "app-maps,app-dice,app-heroes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{}
			register(h)
			if tt.lister != nil {
				h.SetAddonLister(tt.lister)
			}
			r := getPage(t, h, h.Customize, "/campaigns/camp-1/customize")
			if r.code != http.StatusOK {
				t.Fatalf("code = %d", r.code)
			}
			var got []string
			for _, k := range []string{"app-maps", "app-dice", "app-heroes"} {
				if strings.Contains(r.body, `data-czw-key="`+k+`"`) {
					got = append(got, k)
				}
			}
			if strings.Join(got, ",") != tt.want {
				t.Errorf("apps = %v, want %s", got, tt.want)
			}
			if tt.want != "" && !strings.Contains(tt.want, "app-dice") != !strings.Contains(r.body, "Bone") {
				t.Errorf("the Dice summary shows only with its picture")
			}
		})
	}
}

// A draft the owner has not saved shows in the pictures, and a colour that is
// not a plain hex never reaches the style attribute.
func TestWallSkin(t *testing.T) {
	var d customizeDraft
	d.Colours.Accent = "red;background:url(x)"
	d.Colours.Page = "warm"
	d.Header.Bg, d.Header.Solid = "solid", "#112233"
	s := skinOf(d, "Ashfall", []CustomizeCategory{{NamePlural: "NPCs"}})
	css := string(s.Style)
	if strings.Contains(css, "url(") || !strings.Contains(css, "--x-acc:"+classicAccent) {
		t.Errorf("a bad accent reached the style: %s", css)
	}
	if !strings.Contains(css, "--xl-page:#faf8f5") || !strings.Contains(css, "--xd-hd:#112233") {
		t.Errorf("tone or header missing: %s", css)
	}
	if s.Initial != "A" || len(s.Nav) != 1 {
		t.Errorf("initial %q nav %v", s.Initial, s.Nav)
	}

	var base customizeDraft
	base.Colours.Accent = "#6366f1"
	overlayLookDraft(&base, []byte(`{"look":{"draft":{"colours":{"accent":"#9a4a26"}}}}`))
	if base.Colours.Accent != "#9a4a26" {
		t.Errorf("the draft's accent = %q", base.Colours.Accent)
	}
	overlayLookDraft(&base, []byte(`not json`))
	if base.Colours.Accent != "#9a4a26" {
		t.Errorf("a bad draft changed the look: %q", base.Colours.Accent)
	}
}
