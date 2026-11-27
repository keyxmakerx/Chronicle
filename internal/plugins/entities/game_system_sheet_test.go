package entities

import (
	"context"
	"testing"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// A layout's placed blocks decide which tabs a placed game system sheet
// brings along, so a block nested in a container or a tile section counts.
func TestLayoutBlockTypes(t *testing.T) {
	tests := []struct {
		name   string
		layout EntityTypeLayout
		want   []string
		absent []string
	}{
		{
			name:   "empty layout places nothing",
			absent: []string{"trackers"},
		},
		{
			name: "column blocks",
			layout: EntityTypeLayout{Rows: []TemplateRow{{Columns: []TemplateColumn{{Blocks: []TemplateBlock{
				{ID: "b1", Type: BlockGameSystemSheet}, {ID: "b2", Type: "trackers"},
			}}}}}},
			want: []string{BlockGameSystemSheet, "trackers"},
		},
		{
			name: "a block inside a tabs container",
			layout: EntityTypeLayout{Rows: []TemplateRow{{Columns: []TemplateColumn{{Blocks: []TemplateBlock{
				{ID: "b1", Type: "tabs", Config: map[string]any{"tabs": []any{
					map[string]any{"label": "Play", "blocks": []any{map[string]any{"id": "b2", "type": "trackers"}}},
				}}},
			}}}}}},
			want:   []string{"tabs", "trackers"},
			absent: []string{"Play"},
		},
		{
			name: "a tile section",
			layout: EntityTypeLayout{Rows: []TemplateRow{{Kind: "tiles", Tiles: []tilegrid.Tile{
				{ID: "t1", Type: "trackers", Size: "2x2"},
			}}}},
			want: []string{"trackers"},
		},
		{
			name: "a config value named type without an id is not a block",
			layout: EntityTypeLayout{Rows: []TemplateRow{{Columns: []TemplateColumn{{Blocks: []TemplateBlock{
				{ID: "b1", Type: "text_block", Config: map[string]any{"style": map[string]any{"type": "trackers"}}},
			}}}}}},
			want:   []string{"text_block"},
			absent: []string{"trackers"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := layoutBlockTypes(tc.layout)
			for _, w := range tc.want {
				if !got[w] {
					t.Errorf("want %q placed, got %v", w, got)
				}
			}
			for _, a := range tc.absent {
				if got[a] {
					t.Errorf("want %q not placed, got %v", a, got)
				}
			}
		})
	}
}

// A layout that places the game system sheet but not a block offering a page
// tab still gets that tab; one that places the block gets no second copy.
func TestUnplacedPageTabs(t *testing.T) {
	prev := GetGlobalBlockRegistry()
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	reg.Register(BlockMeta{
		Type: "trackers", Label: "Trackers", Contexts: []string{"template"},
		PageTab: &BlockPageTab{Label: "Trackers", State: func(context.Context, BlockRenderContext) (int, bool, error) {
			return 2, false, nil
		}},
	}, func(BlockRenderContext) templ.Component { return templ.Raw(`<div data-test-trackers></div>`) })
	SetGlobalBlockRegistry(reg)
	defer SetGlobalBlockRegistry(prev)

	sheetOnly := EntityTypeLayout{Rows: []TemplateRow{{Columns: []TemplateColumn{{Blocks: []TemplateBlock{
		{ID: "b1", Type: BlockGameSystemSheet},
	}}}}}}
	withTrackers := EntityTypeLayout{Rows: []TemplateRow{{Columns: []TemplateColumn{{Blocks: []TemplateBlock{
		{ID: "b1", Type: BlockGameSystemSheet}, {ID: "b2", Type: "trackers"},
	}}}}}}
	ent := &Entity{ID: "e1", CampaignID: "c1"}
	ctx := layouts.SetUserID(context.Background(), "u1")
	cc := foldTestCC(campaigns.RoleOwner)

	got := unplacedPageTabs(ctx, cc, ent, &EntityType{ID: 1, Layout: sheetOnly}, "tok")
	if len(got) != 1 || got[0].Block != "trackers" {
		t.Fatalf("sheet-only layout: tabs = %+v, want the trackers tab", got)
	}
	if got := unplacedPageTabs(ctx, cc, ent, &EntityType{ID: 1, Layout: withTrackers}, "tok"); len(got) != 0 {
		t.Fatalf("layout placing trackers: tabs = %+v, want none", got)
	}
}
