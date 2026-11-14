package entities

import (
	"context"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// placeableBlockRenderer draws a block another plugin owns on an ordinary
// page: the same lazy-loading placeholder the campaign home uses, carrying the
// page's id so the fragment can verify the block is really placed there. The
// fragment decides what the viewer sees; nothing is read here.
func placeableBlockRenderer(blockType string) BlockRenderer {
	return func(ctx BlockRenderContext) templ.Component {
		if ctx.CC == nil || ctx.CC.Campaign == nil {
			return templ.NopComponent
		}
		on := ""
		if ctx.Entity != nil {
			on = ctx.Entity.ID
		}
		return campaigns.PageBlockFragment(blockType, ctx.CC.Campaign.ID, on)
	}
}

// registerPlaceableCategoryBlocks lets the same blocks sit on a category's
// home. That home has no page id to name, so a visitor sees them there only
// where the Guests home places the block.
func registerPlaceableCategoryBlocks() {
	for _, t := range campaigns.PlaceableBlockTypes() {
		blockType := t
		RegisterCategoryBlock(blockType, func(cc *campaigns.CampaignContext, _ *EntityType) templ.Component {
			if cc == nil || cc.Campaign == nil {
				return templ.NopComponent
			}
			return campaigns.PageBlockFragment(blockType, cc.Campaign.ID, "")
		})
	}
}

// layoutDrawsBlockForGuests reports whether layout draws blockType in a column
// or tile for a viewer who is not on the Director team. It mirrors what the
// page draws, and is deliberately stricter than "mentioned in the layout":
// a block marked for the Director team is not drawn for anyone else, and
// container blocks are not drawn by the server at all, so neither counts.
func layoutDrawsBlockForGuests(layout EntityTypeLayout, blockType string) bool {
	for _, row := range layout.Rows {
		for _, col := range row.Columns {
			for _, b := range col.Blocks {
				if b.Type == blockType && blockVisibility(b) == "everyone" {
					return true
				}
			}
		}
		for _, t := range row.Tiles {
			if t.Type == blockType && blockVisibility(TemplateBlock{Config: t.Config}) == "everyone" {
				return true
			}
		}
	}
	return false
}

// BlockPlacementChecker implements campaigns.BlockPlacementChecker: it is how
// a fragment that cannot import this plugin learns whether the owner placed
// its block on a page the guest may open.
type BlockPlacementChecker struct {
	svc     EntityService
	layouts PageLayoutService // nil while named layouts are unavailable
}

// NewBlockPlacementChecker builds the checker. layouts may be nil.
func NewBlockPlacementChecker(svc EntityService, layouts PageLayoutService) *BlockPlacementChecker {
	return &BlockPlacementChecker{svc: svc, layouts: layouts}
}

var _ campaigns.BlockPlacementChecker = (*BlockPlacementChecker)(nil)

// GuestMaySee is true only when every step holds: the page exists in this
// campaign, the viewer may open it (the check a page view makes, at the same
// role), and the layout the page actually wears draws blockType for them.
// A page that is missing, in another campaign or hidden answers the same
// false, so the route cannot be used to probe for pages. Every error is false.
func (p *BlockPlacementChecker) GuestMaySee(ctx context.Context, campaignID, blockType, onEntityID, userID string, visibilityRole int) bool {
	if p == nil || p.svc == nil || onEntityID == "" || campaignID == "" {
		return false
	}
	entity, err := p.svc.GetByID(ctx, onEntityID)
	if err != nil || entity == nil || entity.CampaignID != campaignID {
		return false
	}
	access, err := p.svc.CheckEntityAccess(ctx, entity.ID, visibilityRole, userID)
	if err != nil || access == nil || !access.CanView {
		return false
	}
	et, err := p.svc.GetEntityTypeByID(ctx, entity.EntityTypeID)
	if err != nil || et == nil {
		return false
	}
	// Same choice wornLayout makes for a page view: the category's main layout
	// unless the page wears one of its named layouts.
	layout := et.Layout
	if p.layouts != nil {
		if pl, key := p.layouts.Worn(ctx, entity, et); !pl.Empty() && key != MainLayoutKey {
			layout = pl.LayoutFor(et, key)
		}
	}
	return layoutDrawsBlockForGuests(layout, blockType)
}
