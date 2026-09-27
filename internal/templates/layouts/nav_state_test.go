package layouts

import (
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
	role   int
	player bool // an owner or scribe viewing as a player
	path   string
	folds  NavFolds
	hint   *NavHint
}

func (c navStateCase) ctx() context.Context {
	ctx := context.Background()
	ctx = SetIsAuthenticated(ctx, true)
	ctx = SetCampaignID(ctx, "c1")
	ctx = SetCampaignName(ctx, "Saltmarsh")
	ctx = SetCampaignRole(ctx, c.role)
	ctx = SetViewingAsPlayer(ctx, c.player)
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
		{"a player has no Manage rows", navStateCase{role: 1, path: "/campaigns/c1/settings"}, "", ""},
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
