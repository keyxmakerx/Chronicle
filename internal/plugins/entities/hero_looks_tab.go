package entities

import (
	"context"
	"fmt"
	"io"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// HeroLooksTabID is the Customize tab's id (served at /customize/x/hero-creator)
// and HeroLooksSection the draft section its changes live in; the section's
// owner is internal/app/customize_herolooks.go.
const (
	HeroLooksTabID   = "hero-creator"
	HeroLooksSection = "herolooks"
)

// heroLooksTabURL is where the tab is served, for the links in the save bar.
func heroLooksTabURL(campaignID string) string {
	return fmt.Sprintf("/campaigns/%s/customize/x/%s", campaignID, HeroLooksTabID)
}

// heroLooksPlanURL is the owner's own creator plan: the page reads the cards
// as the owner's Director plan serves them, with the game system's look beside
// each, and never previews another player.
func heroLooksPlanURL(campaignID string) string {
	return fmt.Sprintf("/campaigns/%s/characters/new/plan", campaignID)
}

// heroLooksFinds names what the tab holds for Find.
func heroLooksFinds() []campaigns.CustomizeFind {
	return []campaigns.CustomizeFind{
		{Label: "Card borders", Words: "border frame card colour color hero creator"},
	}
}

// CustomizeTabFactory contributes the "Hero creator" tab to the Customize page
// (campaigns.Handler.RegisterCustomizeTab): the creator as players see it, in
// which the owner presses a card to change its border. The campaigns plugin
// only hosts the tab; the choices wait in the draft's "herolooks" section and
// are saved by the section's own Apply.
func CustomizeTabFactory() func(*campaigns.CampaignContext) campaigns.CustomizeTab {
	return func(cc *campaigns.CampaignContext) campaigns.CustomizeTab {
		return campaigns.CustomizeTab{
			ID:        HeroLooksTabID,
			Label:     "Hero creator",
			Icon:      "fa-solid fa-hat-wizard",
			SortOrder: 30,
			Section:   HeroLooksSection,
			Content: templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
				return heroLooksSection(cc.Campaign.ID).Render(ctx, w)
			}),
			// The creator's own page is /characters/new; the brush matches by prefix.
			Paths: []string{"/characters/new"},
			Here:  "The borders of the creator's cards",
			Blurb: "The border each card wears in the hero creator.",
			Finds: heroLooksFinds(),
		}
	}
}
