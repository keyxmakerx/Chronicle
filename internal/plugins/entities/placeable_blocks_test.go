package entities

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

var placeableTypes = []string{"when_we_could_play", "next_game", "today_in_world", "open_quests", "world_clocks", "since_last_here"}

// The six home blocks other plugins draw are offered on ordinary pages and on
// a category's home, and each has a renderer for the page.
func TestPlaceableBlocksOfferedOnPages(t *testing.T) {
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	inTemplate := map[string]bool{}
	for _, m := range reg.TypesForCampaignAndContext(context.Background(), "c1", nil, "template") {
		inTemplate[m.Type] = true
	}
	inCategory := map[string]bool{}
	for _, m := range reg.TypesForCampaignAndContext(context.Background(), "c1", nil, "category") {
		inCategory[m.Type] = true
	}
	for _, bt := range placeableTypes {
		t.Run(bt, func(t *testing.T) {
			if !inTemplate[bt] {
				t.Error("not offered in the template context")
			}
			if !inCategory[bt] {
				t.Error("not offered in the category context")
			}
			if !reg.IsValidForContext(bt, "template") || !reg.IsValidForContext(bt, "dashboard") {
				t.Error("must stay valid for both the page and the home")
			}
			if !reg.HasRenderer(bt) {
				t.Error("no page renderer")
			}
			if !IsExtraCategoryBlock(bt) {
				t.Error("a category home cannot draw it")
			}
		})
	}
	if got, want := len(campaigns.PlaceableBlockTypes()), len(placeableTypes); got != want {
		t.Errorf("campaigns lists %d placeable blocks, the registry test expects %d", got, want)
	}
}

// A block on a page loads the same fragment the home does, naming the page so
// the fragment can check the placement; a category home names none.
func TestPlaceableBlockPlaceholderCarriesThePage(t *testing.T) {
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp/1"}}
	paths := map[string]string{
		"when_we_could_play": "when-we-could-play", "next_game": "next-game", "today_in_world": "today-in-world",
		"open_quests": "open-quests", "world_clocks": "world-clocks", "since_last_here": "since-last-here",
	}
	for _, bt := range placeableTypes {
		t.Run(bt, func(t *testing.T) {
			comp := reg.Render(context.Background(), BlockRenderContext{
				Block: TemplateBlock{ID: "b1", Type: bt}, CC: cc, Entity: &Entity{ID: "page&1", CampaignID: "camp/1"},
			})
			if comp == nil {
				t.Fatal("no component")
			}
			var sb strings.Builder
			if err := comp.Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
			html := sb.String()
			for _, want := range []string{
				`hx-get="/campaigns/camp%2F1/` + paths[bt] + `?on=page%261"`, `hx-trigger="load"`, `hx-swap="outerHTML"`,
				`data-block="` + paths[bt] + `"`,
			} {
				if !strings.Contains(html, want) {
					t.Errorf("placeholder missing %q:\n%s", want, html)
				}
			}
		})
	}
	// The category home draws it with no page to name.
	var sb strings.Builder
	if err := renderExtraCategoryBlock(cc, &EntityType{}, "next_game", nil).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), `hx-get="/campaigns/camp%2F1/next-game"`) {
		t.Errorf("category placeholder wrong:\n%s", sb.String())
	}
}

func layoutWith(blockType string, cfg map[string]any) EntityTypeLayout {
	return EntityTypeLayout{Rows: []TemplateRow{{ID: "r", Columns: []TemplateColumn{{ID: "c", Width: 12, Blocks: []TemplateBlock{
		{ID: "t", Type: "title"}, {ID: "b", Type: blockType, Config: cfg}}}}}}}
}

func TestLayoutDrawsBlockForGuests(t *testing.T) {
	tiles := EntityTypeLayout{Rows: []TemplateRow{{ID: "r", Kind: "tiles", Tiles: []tilegrid.Tile{{ID: "t", Type: "next_game", Size: "2x1"}}}}}
	dmTile := EntityTypeLayout{Rows: []TemplateRow{{ID: "r", Kind: "tiles", Tiles: []tilegrid.Tile{{ID: "t", Type: "next_game", Size: "2x1", Config: map[string]any{"visibility": "dm_only"}}}}}}
	// A container is not drawn by the server, so what it holds is not shown.
	nested := layoutWith("two_column", map[string]any{"columns": []any{map[string]any{"blocks": []any{map[string]any{"type": "next_game"}}}}})
	tests := []struct {
		name   string
		layout EntityTypeLayout
		want   bool
	}{
		{"in a column", layoutWith("next_game", nil), true},
		{"everyone is explicit", layoutWith("next_game", map[string]any{"visibility": "everyone"}), true},
		{"in a tile", tiles, true},
		{"another block type only", layoutWith("world_clocks", nil), false},
		{"marked for the Director team", layoutWith("next_game", map[string]any{"visibility": "dm_only"}), false},
		{"an unknown visibility fails closed", layoutWith("next_game", map[string]any{"visibility": "later"}), false},
		{"a Director-only tile", dmTile, false},
		{"inside a container", nested, false},
		{"empty layout", EntityTypeLayout{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := layoutDrawsBlockForGuests(tt.layout, "next_game"); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// placementSvc is the slice of EntityService the checker reads.
type placementSvc struct {
	EntityService
	entities map[string]*Entity
	canView  map[string]bool
	types    map[int]*EntityType
	role     int
	user     string
}

func (s *placementSvc) GetByID(_ context.Context, id string) (*Entity, error) {
	if e, ok := s.entities[id]; ok {
		return e, nil
	}
	return nil, errors.New("not found")
}

func (s *placementSvc) CheckEntityAccess(_ context.Context, id string, role int, user string) (*EffectivePermission, error) {
	s.role, s.user = role, user
	return &EffectivePermission{CanView: s.canView[id]}, nil
}

func (s *placementSvc) GetEntityTypeByID(_ context.Context, id int) (*EntityType, error) {
	if t, ok := s.types[id]; ok {
		return t, nil
	}
	return nil, errors.New("no type")
}

type wornLayouts struct {
	PageLayoutService
	pl  PageLayouts
	key string
}

func (w wornLayouts) Worn(context.Context, *Entity, *EntityType) (PageLayouts, string) {
	return w.pl, w.key
}

// A guest passes only for a page that exists in this campaign, that they may
// open, and whose worn layout draws the block for them.
func TestBlockPlacementCheckerGuestMaySee(t *testing.T) {
	withBlock := &EntityType{ID: 1, Layout: layoutWith("next_game", nil)}
	without := &EntityType{ID: 2, Layout: layoutWith("world_clocks", nil)}
	dmBlock := &EntityType{ID: 3, Layout: layoutWith("next_game", map[string]any{"visibility": "dm_only"})}
	named := PageLayouts{Extra: []PageLayout{{Key: "alt", Name: "Alt", Layout: layoutWith("next_game", nil)}}}
	namedWithout := PageLayouts{Extra: []PageLayout{{Key: "alt", Name: "Alt", Layout: layoutWith("world_clocks", nil)}}}
	svc := func() *placementSvc {
		return &placementSvc{
			entities: map[string]*Entity{
				"public":    {ID: "public", CampaignID: "c1", EntityTypeID: 1},
				"bare":      {ID: "bare", CampaignID: "c1", EntityTypeID: 2},
				"private":   {ID: "private", CampaignID: "c1", EntityTypeID: 1},
				"dmblock":   {ID: "dmblock", CampaignID: "c1", EntityTypeID: 3},
				"elsewhere": {ID: "elsewhere", CampaignID: "c2", EntityTypeID: 1},
				"notype":    {ID: "notype", CampaignID: "c1", EntityTypeID: 99},
			},
			canView: map[string]bool{"public": true, "bare": true, "dmblock": true, "elsewhere": true, "notype": true},
			types:   map[int]*EntityType{1: withBlock, 2: without, 3: dmBlock},
		}
	}
	tests := []struct {
		name    string
		on      string
		block   string
		layouts PageLayoutService
		want    bool
	}{
		{"public page whose layout has the block", "public", "next_game", nil, true},
		{"public page without the block", "bare", "next_game", nil, false},
		{"private page", "private", "next_game", nil, false},
		{"page that does not exist", "nope", "next_game", nil, false},
		{"page of another campaign", "elsewhere", "next_game", nil, false},
		{"block marked for the Director team", "dmblock", "next_game", nil, false},
		{"page whose type cannot be read", "notype", "next_game", nil, false},
		{"no page named", "", "next_game", nil, false},
		{"page wears a named layout that has it", "bare", "next_game", wornLayouts{pl: named, key: "alt"}, true},
		{"page wears a named layout that lacks it", "public", "next_game", wornLayouts{pl: namedWithout, key: "alt"}, false},
		{"page wears the main layout", "public", "next_game", wornLayouts{pl: named, key: MainLayoutKey}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := svc()
			chk := NewBlockPlacementChecker(s, tt.layouts)
			if got := chk.GuestMaySee(context.Background(), "c1", tt.block, tt.on, "visitor", 0); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			if tt.want && (s.role != 0 || s.user != "visitor") {
				t.Errorf("access checked as role %d user %q, want the viewer's own", s.role, s.user)
			}
		})
	}
	var nilChecker *BlockPlacementChecker
	if nilChecker.GuestMaySee(context.Background(), "c1", "next_game", "public", "", 0) {
		t.Error("a missing checker must fail closed")
	}
}
