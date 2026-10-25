package entities

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// pageEditorState is what the Customizer's page layout
// (customize_page_editor.js) starts from: the category's layout as its pages
// show it, whether nothing is saved yet (the page then draws a built-in
// arrangement), and the addresses it reads.
func pageEditorState(ctx context.Context, cc *campaigns.CampaignContext, et *EntityType) string {
	base := fmt.Sprintf("/campaigns/%s", cc.Campaign.ID)
	layout, unsaved := withoutRemovedBlocks(et.Layout), len(et.Layout.Rows) == 0
	if unsaved {
		layout = shownFallbackLayout()
	}
	state := map[string]any{
		"unsaved":     unsaved,
		"campaignId":  cc.Campaign.ID,
		"key":         fmt.Sprintf("%d", et.ID),
		"name":        et.Name,
		"plural":      et.NamePlural,
		"layout":      layout,
		"fields":      len(et.Fields),
		"systemDrawn": hasEntityShowRenderer(ctx, et),
		"blockTypes":  fmt.Sprintf("%s/entity-types/block-types?context=template", base),
		"preview":     fmt.Sprintf("%s/entity-types/%d/layout-preview", base, et.ID),
		"presets":     fmt.Sprintf("%s/layout-presets", base),
		"fieldsUrl":   fmt.Sprintf("%s/customize/categories/%d/fields", base, et.ID),
	}
	if pl, ok := typeLayoutsFrom(ctx); ok {
		state["pageLayouts"] = pageLayoutsEditorState(ctx, et, pl)
	}
	b, _ := json.Marshal(state)
	return string(b)
}

// pageLayoutsEditorState is the layout strip's starting point: the named
// layouts as saved, whether pages of this category can be a player's
// character (only then does "a page a player claims starts on" mean
// anything), and whether a game system has a sheet for a claimed page, so a
// new layout can start with it.
func pageLayoutsEditorState(ctx context.Context, et *EntityType, pl PageLayouts) map[string]any {
	extra := make([]map[string]any, 0, len(pl.Extra))
	for _, l := range pl.Extra {
		extra = append(extra, map[string]any{"key": l.Key, "name": l.Name, "layout": withoutRemovedBlocks(l.Layout)})
	}
	return map[string]any{
		"mainName":    pl.MainName,
		"mainDefault": pl.MainLabel(&EntityType{Name: et.Name}),
		"claimed":     pl.Claimed,
		"extra":       extra,
		"max":         MaxPageLayouts,
		"claimable":   isClaimableType(et),
		"systemSheet": hasEntityShowRenderer(withNPCPage(ctx, false), et),
	}
}

// removedBlockTypes are block types Chronicle no longer has. Older layouts
// may still hold them, since nothing rewrites stored layouts; pages already
// skip them, and the Customizer leaves them out so they don't show as a
// widget an owner could keep, and the next Save drops them for good.
// "permissions" moved into the page's visibility control (ADR-057).
var removedBlockTypes = map[string]bool{"permissions": true}

// withoutRemovedBlocks returns layout minus any removedBlockTypes. A row
// left with nothing in it by that is dropped too, so the old auto-added
// permissions row doesn't linger as an empty section.
func withoutRemovedBlocks(layout EntityTypeLayout) EntityTypeLayout {
	out := EntityTypeLayout{Rows: make([]TemplateRow, 0, len(layout.Rows))}
	for _, row := range layout.Rows {
		removed, kept := 0, 0
		next := row
		if row.Tiles != nil {
			next.Tiles = row.Tiles[:0:0]
			for _, t := range row.Tiles {
				if removedBlockTypes[t.Type] {
					removed++
					continue
				}
				next.Tiles = append(next.Tiles, t)
				kept++
			}
		}
		if row.Columns != nil {
			next.Columns = make([]TemplateColumn, len(row.Columns))
			for i, col := range row.Columns {
				next.Columns[i] = col
				next.Columns[i].Blocks = make([]TemplateBlock, 0, len(col.Blocks))
				for _, b := range col.Blocks {
					if removedBlockTypes[b.Type] {
						removed++
						continue
					}
					next.Columns[i].Blocks = append(next.Columns[i].Blocks, b)
					kept++
				}
			}
		}
		if removed > 0 && kept == 0 {
			continue
		}
		out.Rows = append(out.Rows, next)
	}
	return out
}

// shownFallbackLayout is the arrangement entityPageBody draws for a category
// with no saved layout, as a layout the editor can start from.
func shownFallbackLayout() EntityTypeLayout {
	blocks := func(prefix string, types ...string) []TemplateBlock {
		out := make([]TemplateBlock, len(types))
		for i, t := range types {
			out[i] = TemplateBlock{ID: prefix + t, Type: t}
		}
		return out
	}
	return EntityTypeLayout{Rows: []TemplateRow{
		{
			ID: "row-1",
			Columns: []TemplateColumn{
				{ID: "col-1-1", Width: 8, Blocks: blocks("blk-", "title", "tags", "relations", "entry")},
				{ID: "col-1-2", Width: 4, Blocks: blocks("blk-", "image", "attributes", "details")},
			},
		},
		// The page pieces the fallback draws under the columns; a saved
		// layout shows only what it places, so leaving them out would drop
		// them from every page on the first Save.
		{
			ID:      "row-2",
			Columns: []TemplateColumn{{ID: "col-2-1", Width: 12, Blocks: blocks("blk-", BlockSubPages, BlockPosts, BlockWritingPrompts, BlockBacklinks)}},
		},
	}}
}
