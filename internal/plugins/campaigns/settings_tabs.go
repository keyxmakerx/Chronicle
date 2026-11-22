// settings_tabs.go is the registry other plugins add their parts of
// Campaign settings' Data tab through (Import), and the map from the old
// Details and data tabs' addresses to where each lives now.

package campaigns

import (
	"sort"

	"github.com/a-h/templ"
)

// SettingsTab is one part another plugin adds to the Data tab.
//
//   - ID: stable slug, the part's anchor on Data (`/data#<id>`) and the old
//     `/settings?tab=<id>` address that redirects there; keep it URL-safe
//     and stable (bookmarks rely on it).
//   - Label/Icon: the part's heading, and the icon Find shows beside it.
//   - MinRole gates the part; a viewer below it does not get it.
//   - SortOrder orders the parts (multiples of 10) so a plugin can place its
//     part between others.
//   - Content is the part's body, built per request by the plugin's factory.
type SettingsTab struct {
	ID        string
	Label     string
	Icon      string
	MinRole   Role
	SortOrder int
	Content   templ.Component
}

// RegisterSettingsTab adds a part-factory to the Data tab. Called by other
// plugins at startup; the factory runs per request with the current
// CampaignContext so the part's Content can capture per-request state.
func (h *Handler) RegisterSettingsTab(factory func(*CampaignContext) SettingsTab) {
	h.extraSettingsTabs = append(h.extraSettingsTabs, factory)
}

// Settings tab IDs that other code points at. The old Details and data tabs
// kept these ids, and their links still arrive (settingsTabRedirect).
const (
	settingsTabAPIKeys  = "api-keys"
	settingsTabData     = "data"
	settingsTabActivity = "activity"
	// settingsTabImport is the id the vault_import plugin registers its part
	// under; campaigns names it only to link there and cannot import the plugin.
	settingsTabImport = "import"
)

// visibleSettingsTabs returns the parts other plugins add to Data that this
// viewer may see, sorted by SortOrder (stable, so two parts at the same order
// keep the order they were registered in).
func (h *Handler) visibleSettingsTabs(cc *CampaignContext) []SettingsTab {
	all := make([]SettingsTab, 0, len(h.extraSettingsTabs))
	for _, factory := range h.extraSettingsTabs {
		all = append(all, factory(cc))
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].SortOrder < all[j].SortOrder
	})
	out := all[:0]
	for _, t := range all {
		if t.ID == "" || cc.MemberRole < t.MinRole {
			continue
		}
		out = append(out, t)
	}
	return out
}

// settingsTabRedirect is where an old Details and data tab lives now, or ""
// when the address is the Campaign tab itself. Keys moved to Connections;
// the export, the activity log and every plugin's part (Import) to Data,
// each under its own anchor; People, Foundry and the AI tools became pages of
// their own earlier.
func settingsTabRedirect(campaignID, tab string, extra map[string]bool) string {
	base := "/campaigns/" + campaignID
	switch tab {
	case "", "general":
		return ""
	case "people":
		return base + "/members"
	case "integrations":
		return base + "/foundry"
	case "ai-workspace":
		return base + "/ai"
	case settingsTabAPIKeys:
		return base + "/connections#keys"
	case settingsTabData:
		return base + "/data#export"
	case settingsTabActivity:
		return base + "/data#activity"
	}
	if extra[tab] {
		return base + "/data#" + tab
	}
	return ""
}

// extraSettingsTabIDs are the ids of the parts other plugins add, which Data
// carries under anchors of the same name.
func (h *Handler) extraSettingsTabIDs(cc *CampaignContext) map[string]bool {
	out := map[string]bool{}
	for _, t := range h.visibleSettingsTabs(cc) {
		out[t.ID] = true
	}
	return out
}
