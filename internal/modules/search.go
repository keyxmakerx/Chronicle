package modules

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
)

// ModuleSearchAdapter adapts module DataProvider.Search() results to the
// entity handler's ModuleSearcher interface. It checks which modules are
// enabled as addons for the campaign before searching.
type ModuleSearchAdapter struct {
	addonSvc addons.AddonService
}

// NewModuleSearchAdapter creates an adapter that will check addon
// enablement before searching module content.
func NewModuleSearchAdapter(addonSvc addons.AddonService) *ModuleSearchAdapter {
	return &ModuleSearchAdapter{addonSvc: addonSvc}
}

// SearchModuleContent searches all enabled modules for the given campaign
// and returns results formatted for the entity search API response.
func (a *ModuleSearchAdapter) SearchModuleContent(ctx context.Context, campaignID, query string) ([]map[string]string, error) {
	if query == "" {
		return nil, nil
	}

	var results []map[string]string

	for _, mod := range AllModules() {
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
				"url":       fmt.Sprintf("/campaigns/%s/modules/%s/%s/%s", campaignID, info.ID, item.Category, item.ID),
			})
		}
	}

	return results, nil
}
