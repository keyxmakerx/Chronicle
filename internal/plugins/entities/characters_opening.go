package entities

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// ownEntriesLinkShown reports whether the Characters page links to Own
// entries: for the campaign's Directors, outside "view as player", when the
// campaign has a game system whose pick lists the page manages.
func ownEntriesLinkShown(ctx context.Context, cc *campaigns.CampaignContext) bool {
	if cc == nil || !cc.CanAuthorDmOnly() || layouts.IsViewingAsPlayer(ctx) {
		return false
	}
	_, ok := layouts.GetEnabledSystem(ctx)
	return ok
}

// charactersOpening is the Characters page's opening. It is always the
// one-line size: the page's bands (Yours, the Party, the NPCs) each carry
// their own empty sentence. "Create hero" is offered only to someone who may
// make one.
func charactersOpening(cc *campaigns.CampaignContext, view CastView) layouts.AppOpeningData {
	d := layouts.AppOpeningData{Name: "Characters"}
	if view.CanCreateHero {
		d.Action = &layouts.AppAction{
			Label: "Create hero",
			Icon:  "fa-feather-pointed",
			Href:  fmt.Sprintf("/campaigns/%s/characters/new", cc.Campaign.ID),
			Attrs: map[string]any{"data-create-hero": "true"},
		}
	}
	return d
}
