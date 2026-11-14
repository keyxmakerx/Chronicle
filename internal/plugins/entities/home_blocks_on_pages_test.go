package entities

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// homeBlocksKeptOffPages are the home block types deliberately not made
// placeable on a page, and why: a block added to the home must either be
// offered on pages or named here.
var homeBlocksKeptOffPages = map[string]string{
	"text_block":      "already a page block",
	"session_tracker": "retired; the home draws it as Coming up",
	"relations_graph": "legacy type no palette offers",
}

// The six blocks the last change made placeable are not part of the older set.
var sixPlaceable = map[string]bool{
	"when_we_could_play": true, "next_game": true, "today_in_world": true,
	"open_quests": true, "world_clocks": true, "since_last_here": true,
}

// Every home block is either offered on pages, one of the six placeable
// blocks, or named as left home-only.
func TestEveryHomeBlockIsDecidedForPages(t *testing.T) {
	offered := map[string]bool{}
	for _, bt := range PageHomeBlockTypes() {
		offered[bt] = true
	}
	for _, bt := range campaigns.HomeBlockTypes() {
		if !offered[bt] && !sixPlaceable[bt] && homeBlocksKeptOffPages[bt] == "" {
			t.Errorf("home block %q is neither placeable on pages nor listed as left out", bt)
		}
	}
	for bt := range offered {
		found := false
		for _, h := range campaigns.HomeBlockTypes() {
			found = found || h == bt
		}
		if !found {
			t.Errorf("%q is offered on pages but is not a home block", bt)
		}
	}
}

// The older home blocks are offered in the page context with a renderer, stay
// valid on the home, and keep what they registered.
func TestHomeBlocksOfferedOnPages(t *testing.T) {
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	ctx := context.Background()
	inTemplate, inCategory := map[string]BlockMeta{}, map[string]bool{}
	for _, m := range reg.TypesForCampaignAndContext(ctx, "c1", nil, "template") {
		inTemplate[m.Type] = m
	}
	for _, m := range reg.TypesForCampaignAndContext(ctx, "c1", nil, "category") {
		inCategory[m.Type] = true
	}
	own := map[string]bool{}
	for _, bt := range categoryHomeTypes {
		own[bt] = true
	}
	for _, bt := range PageHomeBlockTypes() {
		t.Run(bt, func(t *testing.T) {
			m, ok := inTemplate[bt]
			if !ok {
				t.Fatal("not offered in the template context")
			}
			if m.Label == "" || m.Icon == "" {
				t.Errorf("lost its label or icon: %+v", m)
			}
			if !reg.IsValidForContext(bt, "dashboard") || !reg.IsValidForContext(bt, "template") {
				t.Error("must be valid on both the home and a page")
			}
			if !reg.HasRenderer(bt) {
				t.Error("no page renderer")
			}
			if !inCategory[bt] {
				t.Error("not offered on a category home")
			}
			if own[bt] {
				if IsExtraCategoryBlock(bt) {
					t.Error("a block the category home draws itself must keep that version")
				}
			} else if !IsExtraCategoryBlock(bt) {
				t.Error("a category home cannot draw it")
			}
			if m.Retired || m.Container || m.Singleton {
				t.Errorf("unexpected flags: %+v", m)
			}
		})
	}
	for bt := range homeBlocksKeptOffPages {
		if bt == "text_block" {
			continue
		}
		if _, ok := inTemplate[bt]; ok {
			t.Errorf("%q must stay off pages", bt)
		}
	}

	// What each block registered survives being offered on pages.
	byType := map[string]BlockMeta{}
	for _, m := range reg.Types() {
		byType[m.Type] = m
	}
	if got := strings.Join(byType["recent_pages"].TileSizes, ","); got != "2x2,4x2" {
		t.Errorf("recent_pages tile sizes = %q", got)
	}
	if got := strings.Join(byType["coming_up"].TileSizes, ","); got != "2x2,2x1" {
		t.Errorf("coming_up tile sizes = %q", got)
	}
	if byType["map_full"].Addon != "maps" || byType["calendar_full"].Addon != "calendar" || byType["sync_status"].Addon != "foundry" {
		t.Error("an addon gate was lost")
	}
	if len(byType["where_we_play"].ConfigFields) != 2 || len(byType["entity_list"].ConfigFields) != 2 || len(byType["start_here"].ConfigFields) != 1 {
		t.Error("a settings field was lost")
	}
	if got := byType["map_full"].Contexts; len(got) != 2 || got[0] != "dashboard" || got[1] != "template" {
		t.Errorf("map_full contexts = %v", got)
	}
}

// fakeRecent records how the Recent Pages block asked for its pages.
type fakeRecent struct {
	role  int
	user  string
	limit int
	pages []campaigns.RecentEntity
}

func (f *fakeRecent) ListRecentForDashboard(_ context.Context, _ string, role int, userID string, limit int) ([]campaigns.RecentEntity, error) {
	f.role, f.user, f.limit = role, userID, limit
	return f.pages, nil
}

type pageViewer struct {
	name string
	cc   *campaigns.CampaignContext
	user string
}

func pageViewers() []pageViewer {
	camp := &campaigns.Campaign{ID: "camp-1", Name: "Therin", IsPublic: true}
	return []pageViewer{
		{"owner", &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RoleOwner, IsMember: true}, "u-owner"},
		{"player", &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RolePlayer, IsMember: true}, "u-player"},
		{"guest", &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RoleNone}, ""},
	}
}

func renderOnPage(t *testing.T, reg *BlockRegistry, ctx context.Context, v pageViewer, bt string, cfg map[string]any) string {
	t.Helper()
	comp := reg.Render(ctx, BlockRenderContext{
		Block: TemplateBlock{ID: "b1", Type: bt, Config: cfg}, CC: v.cc, UserID: v.user,
		Entity: &Entity{ID: "page-1", CampaignID: "camp-1"},
	})
	if comp == nil {
		t.Fatalf("%s: no component", bt)
	}
	var sb strings.Builder
	if err := comp.Render(ctx, &sb); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

// Each block drawn on a page, as the owner, a player and a guest: who gets
// anything at all follows the block's own rule, and nothing the Director's
// side holds reaches a player or a guest.
func TestHomeBlocksOnAPageShowEachViewerOnlyTheirOwn(t *testing.T) {
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	reg.SetRecentPagesLister(&fakeRecent{pages: []campaigns.RecentEntity{{ID: "p1", Name: "Harbour Gate"}}})
	ctx := layouts.SetUpcomingEventsAvailable(context.Background(), true)
	ctx = layouts.SetEntityTypes(ctx, []layouts.SidebarEntityType{{ID: 1, Slug: "c", NamePlural: "Characters"}, {ID: 2, Slug: "s", NamePlural: "Secret Vaults"}})
	ctx = layouts.SetEntityCounts(ctx, map[int]int{1: 4, 2: 0})

	const (
		everyone = "everyone"
		members  = "members"
		owner    = "owner"
	)
	// directorMarkers are what only the Director's side may ever show.
	directorMarkers := []string{
		"data-dm-only", "/activity", "dashboard-sync-block", "/settings", "/members", "/entity-types",
		"/customize", "Tell your players", "Pick a category", "Empty text block",
	}
	tests := []struct {
		block string
		cfg   map[string]any
		who   string
		// owner sees this and player and guest never do.
		ownerSees string
		// every viewer sees this.
		allSee string
	}{
		{block: "welcome_banner", who: everyone, allSee: "Therin", ownerSees: "/settings"},
		{block: "quick_actions", who: everyone, allSee: "All Pages", ownerSees: "/entity-types"},
		{block: "category_grid", who: everyone, allSee: "Characters", ownerSees: "Secret Vaults"},
		{block: "recent_pages", who: everyone, allSee: "Harbour Gate"},
		{block: "entity_list", cfg: map[string]any{"entity_type_id": float64(7), "limit": float64(5)}, who: everyone, allSee: "/campaigns/camp-1/entity-list?type=7&amp;limit=5"},
		{block: "entity_list", who: owner, ownerSees: "entity-list?type=0"},
		{block: "pinned_pages", who: everyone, allSee: "/campaigns/camp-1/pinned-pages"},
		{block: "calendar_preview", cfg: map[string]any{"limit": float64(3)}, who: everyone, allSee: "calendars/upcoming?limit=3"},
		{block: "timeline_preview", who: everyone, allSee: "timelines/preview"},
		{block: "calendar_full", who: everyone, allSee: "calendars/upcoming?limit=10"},
		{block: "timeline_full", cfg: map[string]any{"timeline_id": "tl-9"}, who: everyone, allSee: "timelines/embed?timeline_id=tl-9"},
		{block: "relations_graph_full", who: everyone, allSee: `data-api-url="/campaigns/camp-1/relations-graph"`},
		{block: "map_full", cfg: map[string]any{"map_id": "map-3"}, who: everyone, allSee: `data-map-id="map-3"`},
		{block: "coming_up", who: everyone, allSee: "/campaigns/camp-1/coming-up"},
		{block: "start_here", cfg: map[string]any{"entity_id": "page-77"}, who: everyone, allSee: "start-here?page=page-77"},
		{block: "at_a_glance", who: everyone, allSee: "/campaigns/camp-1/at-a-glance"},
		{block: "party", who: everyone, allSee: "/campaigns/camp-1/party"},
		{block: "your_hero", who: members, allSee: "/campaigns/camp-1/your-hero"},
		{block: "getting_started", who: members, allSee: "/campaigns/camp-1/getting-started"},
		{block: "when_we_play", who: members, allSee: "/campaigns/camp-1/when-we-play"},
		{block: "where_we_play", cfg: map[string]any{"foundry_url": "https://play.example.com", "voice": "Discord"}, who: everyone, allSee: "play.example.com"},
		{block: "where_we_play", who: owner, ownerSees: "Tell your players"},
		{block: "house_rules", who: members, allSee: "/campaigns/camp-1/house-rules"},
		{block: "last_time", who: members, allSee: "/campaigns/camp-1/last-time"},
		{block: "activity_feed", who: owner, ownerSees: "/campaigns/camp-1/activity/embed"},
		{block: "sync_status", who: owner, ownerSees: "/campaigns/camp-1/foundry-vtt/dashboard-sync-block"},
	}
	for _, tt := range tests {
		for _, v := range pageViewers() {
			name := tt.block + "/" + v.name
			t.Run(name, func(t *testing.T) {
				html := renderOnPage(t, reg, ctx, v, tt.block, tt.cfg)
				isOwner := v.name == "owner"
				isMember := v.name != "guest"
				drawsAnything := tt.who == everyone || (tt.who == members && isMember) || (tt.who == owner && isOwner)
				if !drawsAnything {
					if strings.TrimSpace(html) != "" {
						t.Fatalf("drew something for a viewer it is not meant for:\n%s", html)
					}
					return
				}
				if strings.TrimSpace(html) == "" {
					t.Fatal("drew nothing")
				}
				if tt.allSee != "" && !strings.Contains(html, tt.allSee) {
					t.Errorf("missing %q:\n%s", tt.allSee, html)
				}
				if tt.ownerSees != "" && strings.Contains(html, tt.ownerSees) != isOwner {
					t.Errorf("%q shown = %v for the %s", tt.ownerSees, !isOwner, v.name)
				}
				if !isOwner {
					for _, m := range directorMarkers {
						if strings.Contains(html, m) {
							t.Errorf("the Director's %q reached the %s:\n%s", m, v.name, html)
						}
					}
				}
			})
		}
	}
}

// An owner looking as a player is shown what a player is: no owner-only card.
func TestOwnerCardsFollowViewAsPlayerOnAPage(t *testing.T) {
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	owner := pageViewers()[0]
	for _, bt := range []string{"activity_feed", "sync_status"} {
		plain := renderOnPage(t, reg, context.Background(), owner, bt, nil)
		if strings.TrimSpace(plain) == "" || !strings.Contains(plain, "data-dm-only") {
			t.Errorf("%s: the owner should see a card marked for the Director team:\n%s", bt, plain)
		}
		asPlayer := renderOnPage(t, reg, layouts.SetViewingAsPlayer(context.Background(), true), owner, bt, nil)
		if strings.TrimSpace(asPlayer) != "" {
			t.Errorf("%s: an owner looking as a player was shown it:\n%s", bt, asPlayer)
		}
	}
}

// Recent Pages on a page asks for pages as the viewer, not as the owner, and a
// page from the editor never widens what is read.
func TestRecentPagesOnAPageReadsForTheViewer(t *testing.T) {
	tests := []struct {
		name     string
		viewer   int
		asPlayer bool
		cfg      map[string]any
		wantRole int
		wantUser string
		wantMax  int
	}{
		{"owner", 0, false, nil, int(campaigns.RoleOwner), "u-owner", 8},
		{"owner as a player", 0, true, nil, int(campaigns.RolePlayer), "u-owner", 8},
		{"player", 1, false, nil, int(campaigns.RolePlayer), "u-player", 8},
		{"guest", 2, false, nil, int(campaigns.RoleNone), "", 8},
		{"a limit past the cap", 1, false, map[string]any{"limit": float64(500)}, int(campaigns.RolePlayer), "u-player", 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewBlockRegistry()
			RegisterCoreBlocks(reg)
			f := &fakeRecent{pages: []campaigns.RecentEntity{{ID: "p1", Name: "Harbour Gate"}}}
			reg.SetRecentPagesLister(f)
			ctx := context.Background()
			if tt.asPlayer {
				ctx = layouts.SetViewingAsPlayer(ctx, true)
			}
			renderOnPage(t, reg, ctx, pageViewers()[tt.viewer], "recent_pages", tt.cfg)
			if f.role != tt.wantRole || f.user != tt.wantUser || f.limit != tt.wantMax {
				t.Errorf("read as role %d user %q limit %d; want %d %q %d", f.role, f.user, f.limit, tt.wantRole, tt.wantUser, tt.wantMax)
			}
		})
	}
	// With no source wired the block draws nothing rather than failing the page.
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	if out := renderOnPage(t, reg, context.Background(), pageViewers()[0], "recent_pages", nil); strings.TrimSpace(out) != "" {
		t.Errorf("drew without a source:\n%s", out)
	}
}

// A category home draws a block with the settings saved on that block, and the
// pinned, recent, calendar and timeline blocks keep the category's own.
func TestHomeBlocksOnACategoryHomeReadTheirSavedSettings(t *testing.T) {
	cc := pageViewers()[1].cc
	var sb strings.Builder
	if err := renderExtraCategoryBlock(cc, &EntityType{}, "map_full", map[string]any{"map_id": "map-3"}).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), `data-map-id="map-3"`) {
		t.Errorf("config not used:\n%s", sb.String())
	}
	for _, bt := range []string{"pinned_pages", "recent_pages", "calendar_preview", "timeline_preview"} {
		if IsExtraCategoryBlock(bt) {
			t.Errorf("%s: the category's own version would be shadowed", bt)
		}
	}
	// A guest on a category home gets nothing from a members' block.
	sb.Reset()
	guest := pageViewers()[2].cc
	if err := renderExtraCategoryBlock(guest, &EntityType{}, "house_rules", nil).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(sb.String()) != "" {
		t.Errorf("a guest was shown a members' block:\n%s", sb.String())
	}
}
