package systems

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
)

// SystemSearchAdapter adapts module DataProvider.Search() results to the
// entity handler's SystemSearcher interface. It checks which modules are
// enabled as addons for the campaign before searching.
type SystemSearchAdapter struct {
	addonSvc addons.AddonService
}

// NewSystemSearchAdapter creates an adapter that will check addon
// enablement before searching module content.
func NewSystemSearchAdapter(addonSvc addons.AddonService) *SystemSearchAdapter {
	return &SystemSearchAdapter{addonSvc: addonSvc}
}

// SearchSystemContent searches all enabled modules for the given campaign
// and returns results formatted for the entity search API response.
func (a *SystemSearchAdapter) SearchSystemContent(ctx context.Context, campaignID, query string) ([]map[string]string, error) {
	if query == "" {
		return nil, nil
	}

	var results []map[string]string

	for _, mod := range AllSystems() {
		info := mod.Info()

		// Check if this module's addon is enabled for the campaign.
		enabled, err := a.addonSvc.IsEnabledForCampaign(ctx, campaignID, info.ID)
		if err != nil || !enabled {
			continue
		}

		dp := mod.DataProvider()
		if dp == nil {
			continue
		}

		items, err := dp.Search(query)
		if err != nil {
			continue
		}

		// Format results to match the entity search API shape.
		for _, item := range items {
			results = append(results, map[string]string{
				"id":        item.ID,
				"name":      item.Name,
				"type_name": info.Name + " · " + item.Category,
				"type_icon": info.Icon,
				"type_color": "#6B7280",
				"url":       fmt.Sprintf("/campaigns/%s/systems/%s/%s/%s", campaignID, info.ID, item.Category, item.ID),
			})
		}
	}

	return results, nil
}
