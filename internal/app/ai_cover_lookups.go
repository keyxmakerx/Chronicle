package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
)

// coverageLookups are the read-only lookups that belong to a feature's own
// plugin and need an add-on gate: the campaign's settings and its timelines.
func (d *aiDeps) coverageLookups() []aiplug.Lookup {
	var hub campaigns.AddonLister
	if svc, ok := d.Addons.(addons.AddonService); ok {
		hub = &addonListerAdapter{svc: svc}
	}
	out := []aiplug.Lookup{campaigns.NewAISettings(d.Campaigns, hub)}
	if d.Timelines != nil {
		out = append(out, timeline.NewAITimelines(d.Timelines, d.addonOffReason(timeline.PluginSlug, "Timelines")))
	}
	if d.Quests != nil && d.Boards != nil {
		out = append(out, quests.NewAIQuests(d.Quests, d.Boards))
	}
	if d.Homebrew != nil {
		out = append(out, records.RulesLookup{Svc: d.Homebrew})
	}
	return out
}

// addonOffReason is the refusal an add-on's lookup gives while a campaign has
// the add-on switched off, "" while it is on. A failed check counts it as on,
// as the import's feature gate does; the feature's own routes still gate it.
func (d *aiDeps) addonOffReason(slug, name string) func(ctx context.Context, campaignID string) string {
	addons := d.Addons
	return func(ctx context.Context, campaignID string) string {
		if addons == nil {
			return ""
		}
		if on, err := addons.IsEnabledForCampaign(ctx, campaignID, slug); err == nil && !on {
			return name + " is switched off for this campaign, so there is nothing to look at"
		}
		return ""
	}
}
