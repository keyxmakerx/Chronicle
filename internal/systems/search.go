package systems

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
)

// SystemSearchAdapter adapts the rules search to the entity handler's
// SystemSearcher interface. It checks which systems are enabled as addons
// for the campaign before searching, and searches each through RulesPages,
// so global search finds exactly what the reader's own list holds.
type SystemSearchAdapter struct {
	addonSvc addons.AddonService
	rules    *RulesPages
}

// NewSystemSearchAdapter creates an adapter that will check addon
// enablement before searching module content.
func NewSystemSearchAdapter(addonSvc addons.AddonService) *SystemSearchAdapter {
	return &SystemSearchAdapter{addonSvc: addonSvc}
}

// SetRulesPages gives the adapter the per-reader catalog. Without it every
// system answers from its package alone.
func (a *SystemSearchAdapter) SetRulesPages(p *RulesPages) { a.rules = p }

// SearchSystemContent searches all enabled systems for the given campaign
// and returns results formatted for the entity search API response.
// Director-only categories and entries are left out unless director is set.
func (a *SystemSearchAdapter) SearchSystemContent(ctx context.Context, campaignID, query string, director bool, userID, campaignName string) ([]map[string]string, error) {
	if query == "" {
		return nil, nil
	}
	rules := a.rules
	if rules == nil {
		rules = NewRulesPages(nil, nil, nil)
	}
	reader := RulesReader{CampaignID: campaignID, CampaignName: campaignName,
		Actor: EntryActor{UserID: userID, IsDirector: director}}

	var results []map[string]string
	for _, mod := range AllSystems() {
		info := mod.Info()
		enabled, err := a.addonSvc.IsEnabledForCampaign(ctx, campaignID, info.ID)
		if err != nil || !enabled {
			continue
		}
		hits, err := rules.Search(ctx, mod, query, reader)
		if err != nil {
			continue
		}
		for _, h := range hits {
			typeName := info.Name + " · " + h.Category
			switch h.Kind {
			case "trait":
				typeName = "Trait of " + h.EntryName + " · " + info.Name
			case "left_out":
				typeName = h.Summary
			}
			results = append(results, map[string]string{
				"id":         hitID(h),
				"name":       h.Name,
				"type_name":  typeName,
				"type_icon":  info.Icon,
				"type_color": "#6B7280",
				"url":        h.URL,
			})
		}
	}
	return results, nil
}

// hitID keeps a trait's hit apart from its entry's.
func hitID(h RulesHit) string {
	if h.Kind == "trait" {
		return h.Slug + "#" + slugFromName(h.Name)
	}
	return h.Slug
}
