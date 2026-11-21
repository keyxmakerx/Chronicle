package app

import (
	"context"
	"log/slog"
)

// starterFeatureSlugs is the usual set a campaign made now starts with, in the
// order they are turned on: the calendar comes before Game nights, which needs
// it. Armory, Crafting, Transformations and the Sync API stay off because each
// needs further setup or is wanted by only some tables. Campaigns that exist
// are never touched by this list.
var starterFeatureSlugs = []string{
	"calendar", "sessions", "notes", "jot-notes", "maps", "timeline",
	"media-gallery", "npcs", "player-character-claiming", "attributes",
	"player-notes",
}

// featureEnabler is the slice of the addons service this needs, so every
// enable runs the same checks and enable effects as the owner's own switch.
type featureEnabler interface {
	EnableForCampaignBySlug(ctx context.Context, campaignID, addonSlug, userID string) error
}

// starterFeatures turns on a new campaign's usual features. It implements
// campaigns.FeatureEnabler, so the campaigns plugin never names an add-on.
type starterFeatures struct{ addons featureEnabler }

// EnableStarterFeatures skips and logs a feature that cannot be turned on: the
// campaign exists already and the owner can switch it on in Game & features.
func (s starterFeatures) EnableStarterFeatures(ctx context.Context, campaignID, userID string) {
	for _, slug := range starterFeatureSlugs {
		if err := s.addons.EnableForCampaignBySlug(ctx, campaignID, slug, userID); err != nil {
			slog.Warn("failed to turn on a feature for the new campaign",
				slog.String("campaign_id", campaignID),
				slog.String("addon_slug", slug),
				slog.Any("error", err),
			)
		}
	}
}
