package decks

import (
	"context"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// BlockType is the layout block that shows a hero's cards.
const BlockType = "hero_cards"

// ViewerOf is the viewer a page's block reads decks as.
func ViewerOf(cc *campaigns.CampaignContext, userID string) Viewer {
	return Viewer{
		UserID:         userID,
		MemberRole:     int(cc.MemberRole),
		VisibilityRole: cc.VisibilityRole(),
		Director:       cc.CanAuthorDmOnly(),
	}
}

// HeroBlock is the Cards part of a hero's page. It reads through the same
// service call as the strip's own route, so the part shows exactly what that
// route would answer; a page whose hand the viewer may not read draws
// nothing, as the route answers 404.
func HeroBlock(ctx context.Context, svc Service, cc *campaigns.CampaignContext, heroID, userID string) (templ.Component, error) {
	view, err := svc.HeroHand(ctx, cc.Campaign.ID, heroID, ViewerOf(cc, userID))
	if err != nil {
		return templ.NopComponent, err
	}
	return HeroStrip(stripPage{CampaignID: cc.Campaign.ID, View: view}), nil
}
