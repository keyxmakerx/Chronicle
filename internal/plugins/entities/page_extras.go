package entities

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// page_extras.go covers the page pieces that are blocks the owner places
// rather than sections drawn under every layout: sub-pages, writing prompts,
// backlinks, the game system's panels and the items-and-money panel (posts
// was already a block). Nothing shows on a page unless its layout places it;
// the one default is the items-and-money panel on player-character pages.
// The page a game-system renderer owns has no layout, so it still draws them.

// Block types for the pieces above.
const (
	BlockSubPages        = "children"
	BlockPosts           = "posts"
	BlockWritingPrompts  = "writing_prompts"
	BlockBacklinks       = "backlinks"
	BlockSystemPanels    = "system_panels"
	BlockCharacterItems  = "character_items"
	blockInventoryLegacy = "inventory"
)

type pageChildrenKey struct{}

// withPageChildren carries the page's sub-pages to the Sub-pages block.
func withPageChildren(ctx context.Context, children []Entity) context.Context {
	return context.WithValue(ctx, pageChildrenKey{}, children)
}

// pageChildren returns the sub-pages the show handler loaded, or nil.
func pageChildren(ctx context.Context) []Entity {
	c, _ := ctx.Value(pageChildrenKey{}).([]Entity)
	return c
}

type autoPanelKey struct{}

// withAutoPagePanel marks that the items-and-money panel goes under the page
// without the layout placing it.
func withAutoPagePanel(ctx context.Context) context.Context {
	return context.WithValue(ctx, autoPanelKey{}, true)
}

// autoPagePanel reports whether the show page draws the panel on its own.
func autoPagePanel(ctx context.Context) bool {
	v, _ := ctx.Value(autoPanelKey{}).(bool)
	return v
}

// isPlayerCharacterPageType is the exception to "nothing is on a page unless
// placed": the types players claim as their characters.
func isPlayerCharacterPageType(et *EntityType) bool {
	if et == nil {
		return false
	}
	return isClaimableType(et) || isPlayerCharacterType(derefStr(et.PresetCategory), et.Slug)
}

// autoCharacterPanel reports whether a page shows the items-and-money panel
// although its layout does not place it: player-character pages only, and
// only when the layout has neither that block nor the Inventory block, so a
// page never shows two inventories.
func autoCharacterPanel(et *EntityType) bool {
	if !isPlayerCharacterPageType(et) {
		return false
	}
	return !layoutPlacesBlock(et.Layout, BlockCharacterItems) &&
		!layoutPlacesBlock(et.Layout, blockInventoryLegacy)
}

// PageExtrasPlan says which panels a type's pages showed outside its layout,
// so placing them as blocks keeps those pages as they were.
type PageExtrasPlan struct {
	SystemPanels   bool // game-system panels under the title (NPC pages)
	CharacterItems bool // the items-and-money panel after the layout
}

// placePageExtras adds the blocks that used to be drawn outside a layout and
// reports whether anything changed. Each block is added only when the layout
// does not already place it, so running it again changes nothing.
func placePageExtras(layout EntityTypeLayout, plan PageExtrasPlan) (EntityTypeLayout, bool) {
	changed := false
	if plan.SystemPanels && !layoutPlacesBlock(layout, BlockSystemPanels) {
		layout = insertAfterTitle(layout, TemplateBlock{ID: "blk-system-panels", Type: BlockSystemPanels})
		changed = true
	}
	if plan.CharacterItems && !layoutPlacesBlock(layout, BlockCharacterItems) &&
		!layoutPlacesBlock(layout, blockInventoryLegacy) {
		layout.Rows = append(layout.Rows, fullWidthRow("row-character-items", TemplateBlock{ID: "blk-character-items", Type: BlockCharacterItems}))
		changed = true
	}
	// The order these were drawn in under the layout.
	var below []TemplateBlock
	for _, t := range []string{BlockSubPages, BlockPosts, BlockWritingPrompts, BlockBacklinks} {
		if !layoutPlacesBlock(layout, t) {
			below = append(below, TemplateBlock{ID: "blk-" + t, Type: t})
		}
	}
	if len(below) > 0 {
		layout.Rows = append(layout.Rows, fullWidthRow("row-page-extras", below...))
		changed = true
	}
	return layout, changed
}

// insertAfterTitle puts a block right after the first top-level title block,
// where the panels used to sit, or in a row of its own at the top.
func insertAfterTitle(layout EntityTypeLayout, b TemplateBlock) EntityTypeLayout {
	for r := range layout.Rows {
		for c := range layout.Rows[r].Columns {
			blocks := layout.Rows[r].Columns[c].Blocks
			for i := range blocks {
				if blocks[i].Type == "title" {
					out := make([]TemplateBlock, 0, len(blocks)+1)
					out = append(out, blocks[:i+1]...)
					out = append(out, b)
					out = append(out, blocks[i+1:]...)
					layout.Rows[r].Columns[c].Blocks = out
					return layout
				}
			}
		}
	}
	layout.Rows = append([]TemplateRow{fullWidthRow("row-system-panels", b)}, layout.Rows...)
	return layout
}

// fullWidthRow builds a one-column row holding the given blocks.
func fullWidthRow(id string, blocks ...TemplateBlock) TemplateRow {
	return TemplateRow{ID: id, Columns: []TemplateColumn{{ID: id + "-col", Width: 12, Blocks: blocks}}}
}

// PlacePageExtras places the page pieces that used to be drawn outside the
// layout into every entity type of a campaign, so its pages look as they did
// before those pieces became blocks. plans is keyed by entity type id. Types
// whose layout already places everything are left untouched. Returns the
// number of layouts changed.
func (s *entityService) PlacePageExtras(ctx context.Context, campaignID string, plans map[int]PageExtrasPlan) (int, error) {
	types, err := s.types.ListByCampaign(ctx, campaignID)
	if err != nil {
		return 0, fmt.Errorf("listing entity types: %w", err)
	}
	changed := 0
	for i := range types {
		layout, ok := placePageExtras(types[i].Layout, plans[types[i].ID])
		if !ok {
			continue
		}
		// Written as is rather than through UpdateEntityTypeLayout: a stored
		// layout may hold a retired block type that validation would refuse,
		// and this only adds registered blocks to what is already there.
		raw, err := json.Marshal(layout)
		if err == nil {
			err = s.types.UpdateLayout(ctx, types[i].ID, string(raw))
		}
		if err != nil {
			// One bad layout must not stop the rest of the campaign.
			slog.Warn("placing page extras failed",
				slog.String("campaign_id", campaignID), slog.Int("entity_type_id", types[i].ID), slog.Any("error", err))
			continue
		}
		changed++
	}
	return changed, nil
}
