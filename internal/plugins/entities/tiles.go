package entities

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// tileBlock turns a tile into the block the entity-page dispatcher draws, so
// a tile and a column block share visibility, addon and registry handling.
func tileBlock(t tilegrid.Tile) TemplateBlock {
	return TemplateBlock{ID: t.ID, Type: t.Type, Config: t.Config}
}

// resolveTile gives the size a tile is drawn at. A stored size its type no
// longer allows falls back to the type's first allowed size; ok is false when
// the type now allows no size (or is gone), which the caller draws as the
// same not-available notice a misplaced block gets, so the page never fails.
func resolveTile(ctx context.Context, cc *campaigns.CampaignContext, t tilegrid.Tile) (w, h int, ok bool) {
	reg := GetGlobalBlockRegistry()
	if reg == nil || cc == nil || cc.Campaign == nil {
		return 0, 0, false
	}
	_, w, h, ok = tilegrid.Resolve(t.Size, reg.TileSizesFor(ctx, cc.Campaign.ID, t.Type, t.Config))
	return w, h, ok
}

// blockHiddenFromViewer reports whether a block is not drawn for this viewer:
// the attributes block with its addon off, or a DM-only block the viewer
// cannot read. Tile sections check it before opening the tile's cell, so a
// hidden tile leaves no empty card in the grid.
func blockHiddenFromViewer(block TemplateBlock, cc *campaigns.CampaignContext, showAttributes bool) bool {
	if block.Type == "attributes" && !showAttributes {
		return true
	}
	return blockVisibility(block) == "dm_only" && cc.VisibilityRole() < int(campaigns.RoleOwner)
}
