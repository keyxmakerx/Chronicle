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
	"context"
	"sort"
	"strings"

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
//   - Summary: optional. What is chosen now, in a few words ("Atlas frame"),
//     for the tab's row in the Customize block of Campaign settings and its
//     card on the Overview. The plugin reads its own storage here, so the
//     campaigns plugin never reads another plugin's tables. Nil shows the
//     tab's name alone.
//   - Addon: optional. The add-on the tab belongs to; while it is off for the
//     campaign the Customize block and the header's brush leave the tab out.
//   - Paths: optional. Campaign-relative path prefixes of the plugin's own
//     pages ("/maps"); on them the header's brush offers this tab first.
//   - Here: the words of that brush row ("The frame around maps"); empty
//     falls back to the tab's label.
//   - Blurb: optional. What is inside the tab, for its card on the Overview
//     when it has no summary to show ("The dice everyone rolls, ...").
//   - Finds: optional. The named things inside the tab, so Find can open the
//     tab by what it holds and not only by its name.
//
// The Customize page is Owner-only, so a contributed tab needs no role of its
// own: every viewer of it is already an Owner.
type CustomizeTab struct {
	ID        string
	Label     string
	Icon      string
	SortOrder int
	Content   templ.Component
	Summary   func(ctx context.Context) CustomizeSummary
	Addon     string
	Paths     []string
	Here      string
	Blurb     string
	Finds     []CustomizeFind
}

// CustomizeFind is one thing inside a contributed tab that Find should know:
// the name shown, and other words someone might type for it.
type CustomizeFind struct {
	Label, Words string
}

// CustomizeSummary is a tab's "what is chosen now": a figure and an optional
// muted second part ("Jewelled, onyx" and "players pick their own").
type CustomizeSummary struct {
	Text, Sub string
}

// summary calls the tab's summary, or returns nothing when it has none. A
// panicking summary reads as none: a plugin's words must never break the
// settings page.
func (t CustomizeTab) summary(ctx context.Context) (s CustomizeSummary) {
	if t.Summary == nil {
		return CustomizeSummary{}
	}
	defer func() {
		if recover() != nil {
			s = CustomizeSummary{}
		}
	}()
	return t.Summary(ctx)
}

// owns reports whether a campaign-relative path is one of the tab's pages.
func (t CustomizeTab) owns(rel string) bool {
	for _, p := range t.Paths {
		p = strings.TrimRight(p, "/")
		if p != "" && (rel == p || strings.HasPrefix(rel, p+"/")) {
			return true
		}
	}
	return false
}

// customizeTabsOn is customizeTabs without the tabs whose add-on is off.
func (h *Handler) customizeTabsOn(cc *CampaignContext, addons map[string]bool) []CustomizeTab {
	return tabsWithAddon(h.customizeTabs(cc), addons)
}

// shownCustomizeTabs is the tabs Customize lists for this campaign: those whose
// add-on is off are left out, as in the header's brush. When the add-ons
// cannot be read every tab stays, since hiding a working tab on a failed read
// is worse than showing one the campaign switched off.
func (h *Handler) shownCustomizeTabs(ctx context.Context, cc *CampaignContext) []CustomizeTab {
	all := h.customizeTabs(cc)
	if h.addonLister == nil {
		return all
	}
	list, err := h.addonLister.ListForPluginHub(ctx, cc.Campaign.ID)
	if err != nil {
		return all
	}
	addons := make(map[string]bool, len(list))
	for _, a := range list {
		addons[a.Slug] = a.Enabled
	}
	return tabsWithAddon(all, addons)
}

// tabsWithAddon drops the tabs whose add-on is not on in addons.
func tabsWithAddon(all []CustomizeTab, addons map[string]bool) []CustomizeTab {
	out := all[:0:0]
	for _, t := range all {
		if t.Addon == "" || addons[t.Addon] {
			out = append(out, t)
		}
	}
	return out
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
