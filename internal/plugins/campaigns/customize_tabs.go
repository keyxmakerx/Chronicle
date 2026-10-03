// customize_tabs.go — registry that lets other plugins add a tab to the
// Customize page without the campaigns plugin knowing they exist.
//
// Mirrors RegisterSettingsTab (settings_tabs.go): the campaigns plugin owns
// only the registry and the tab chrome, each plugin owns its own tab body.
// That is how a look setting that belongs to another plugin (the map frame)
// sits on the Customize page while its storage, validation and routes stay
// in that plugin (T-B2).

package campaigns

import (
	"sort"

	"github.com/a-h/templ"
)

// CustomizeTab is the declarative description of one plugin-contributed tab on
// the Customize page.
//
//   - ID: stable slug used as the Alpine tab key; URL-safe and constant.
//   - Label/Icon: button text / FontAwesome classes.
//   - SortOrder: order among contributed tabs (they all render after the
//     built-in Layouts / Content Templates / Appearance tabs).
//   - Content: the tab body; the factory builds it per request so it can read
//     live state for cc.Campaign.
//
// The Customize page is Owner-only, so a contributed tab needs no role of its
// own: every viewer of it is already an Owner.
type CustomizeTab struct {
	ID        string
	Label     string
	Icon      string
	SortOrder int
	Content   templ.Component
}

// RegisterCustomizeTab appends a tab factory, called by other plugins at
// startup. A nil factory is ignored so a plugin that conditionally exposes a
// tab can pass its own nil through.
func (h *Handler) RegisterCustomizeTab(factory func(*CampaignContext) CustomizeTab) {
	if factory == nil {
		return
	}
	h.extraCustomizeTabs = append(h.extraCustomizeTabs, factory)
}

// customizeTabs resolves the contributed tabs for one request, in a stable
// order (SortOrder, then registration order). Empty IDs are skipped: an
// unnamed tab would collide with the page's own tab keys.
func (h *Handler) customizeTabs(cc *CampaignContext) []CustomizeTab {
	out := make([]CustomizeTab, 0, len(h.extraCustomizeTabs))
	for _, factory := range h.extraCustomizeTabs {
		t := factory(cc)
		if t.ID == "" || t.Content == nil {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out
}
