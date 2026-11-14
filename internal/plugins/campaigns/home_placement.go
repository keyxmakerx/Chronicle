package campaigns

import (
	"context"
	"net/url"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// HomeHasBlock reports whether a home layout places blockType in a column or
// a tile section. Home blocks that a visitor may see only where the owner
// chose to show them ask it of the Guests home, so placement is the consent:
// the fragment routes are reachable on any public campaign, and the layout is
// the only thing that says the owner wanted the block there.
func HomeHasBlock(l *DashboardLayout, blockType string) bool {
	if l == nil {
		return false
	}
	for _, r := range l.Rows {
		for _, col := range r.Columns {
			for _, b := range col.Blocks {
				if b.Type == blockType {
					return true
				}
			}
		}
		for _, t := range r.Tiles {
			if t.Type == blockType {
				return true
			}
		}
	}
	return false
}

// GuestHomeHasBlock reports whether the campaign's Guests home places
// blockType.
func GuestHomeHasBlock(c *Campaign, blockType string) bool {
	home, _ := c.HomeLayout(HomeGuest)
	return HomeHasBlock(home, blockType)
}

// BlockPlacementChecker answers whether the owner placed a block on a page a
// guest may open. The campaigns package cannot see entity layouts, so the
// entities plugin implements it and the app wires it into the handlers that
// draw the fragments.
type BlockPlacementChecker interface {
	// GuestMaySee reports whether the page onEntityID exists in the campaign,
	// the viewer may open it, and its layout, as that page wears it, draws
	// blockType for a viewer without the Director's role. Any doubt is false.
	GuestMaySee(ctx context.Context, campaignID, blockType, onEntityID, userID string, visibilityRole int) bool
}

// BlockVisibleTo is the one rule every placeable fragment asks before it
// answers, so a block carries its own check rather than trusting where it was
// placed:
//   - a member always passes (each block still filters its content by viewer);
//   - since_last_here is a member's own and never passes a guest;
//   - any other block passes a guest only where the owner placed it: on the
//     Guests home, or on the page named by onEntityID after the checker has
//     verified that page. onEntityID alone is never trusted.
//
// checker may be nil (nothing is placed on pages then).
func BlockVisibleTo(ctx context.Context, cc *CampaignContext, checker BlockPlacementChecker, blockType, onEntityID, userID string) bool {
	if cc == nil || cc.Campaign == nil {
		return false
	}
	if cc.IsMember {
		return true
	}
	if blockType == BlockSinceLastHere {
		return false
	}
	if GuestHomeHasBlock(cc.Campaign, blockType) {
		return true
	}
	if checker == nil || onEntityID == "" {
		return false
	}
	return checker.GuestMaySee(ctx, cc.Campaign.ID, blockType, onEntityID, userID, cc.VisibilityRole())
}

// blockFragment is how a placeable block's fragment is reached and how its
// loading bar looks.
type blockFragment struct {
	Type     string
	Path     string // path segment under /campaigns/:id, also the block's data-block name
	Skeleton string // height class of the loading bar
}

// placeableBlocks are the home blocks another plugin draws that can also sit
// on an ordinary page. The entities plugin cannot import those plugins, so
// their fragment routes are named here once.
var placeableBlocks = []blockFragment{
	{BlockWhenWeCouldPlay, "when-we-could-play", "h-24"},
	{BlockNextGame, "next-game", "h-24"},
	{BlockTodayInWorld, "today-in-world", "h-24"},
	{BlockOpenQuests, "open-quests", "h-24"},
	{BlockWorldClocks, "world-clocks", "h-20"},
	{BlockSinceLastHere, "since-last-here", "h-24"},
}

// PlaceableBlockTypes lists the block types that may sit on any page, in the
// order a palette offers them.
func PlaceableBlockTypes() []string {
	out := make([]string, 0, len(placeableBlocks))
	for _, b := range placeableBlocks {
		out = append(out, b.Type)
	}
	return out
}

func placeableBlock(blockType string) (blockFragment, bool) {
	for _, b := range placeableBlocks {
		if b.Type == blockType {
			return b, true
		}
	}
	return blockFragment{}, false
}

// BlockFragmentURL is the address a placeable block loads its content from.
// A block on a page names that page in "on" so the fragment can check the
// placement; a block on a home sends none.
func BlockFragmentURL(campaignID, blockType, onEntityID string) (string, bool) {
	b, ok := placeableBlock(blockType)
	if !ok {
		return "", false
	}
	href := "/campaigns/" + url.PathEscape(campaignID) + "/" + b.Path
	if onEntityID != "" {
		href += "?on=" + url.QueryEscape(onEntityID)
	}
	return href, true
}

// OwnerCardVisible reports whether a card that reads the owner's own routes
// (the activity log, the Foundry sync status) is drawn for this viewer. Those
// routes answer only the owner, so anyone else is shown nothing rather than a
// card that can only fail to load, and an owner looking as a player sees what
// a player sees. The block asks this itself, so it holds on any page it is
// placed on, not only on the owner's home.
func OwnerCardVisible(ctx context.Context, cc *CampaignContext) bool {
	return cc != nil && cc.MemberRole >= RoleOwner && !layouts.IsViewingAsPlayer(ctx)
}
