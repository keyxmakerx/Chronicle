package app

import (
	"sort"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// nav_layout.go gathers what a campaign's sidebar is built from (the apps
// turned on, the categories and their counts), runs it through the campaigns
// plugin's NormalizeNav and ViewNav for the viewer, and hands the result to
// the layout as view rows.

// navAppsFor resolves the app catalog for one campaign: every app, with
// Enabled set when one of its addons (or, for the rulebook, a game system) is
// on. Apps that are off stay in the list so their place in the owner's
// arrangement survives until they are turned back on.
func navAppsFor(campaignID string, enabled map[string]bool, sys layouts.EnabledSystem) []campaigns.NavApp {
	base := "/campaigns/" + campaignID
	out := make([]campaigns.NavApp, 0, len(navAppCatalog))
	for _, d := range navAppCatalog {
		app := campaigns.NavApp{
			Slug: d.slug, Label: d.label, Icon: d.icon, Caption: d.caption,
			Access: d.access, DefaultPinned: d.pinned,
		}
		if d.system {
			if sys.Slug != "" {
				app.Enabled = true
				app.URL = base + "/systems/" + sys.Slug
				if sys.Name != "" {
					app.Label = sys.Name
				}
				app.Icon = layouts.SafeNavIcon(sys.Icon, d.icon)
			}
		} else {
			app.URL = base + d.path
			for _, a := range d.addons {
				if enabled[a] {
					app.Enabled = true
					break
				}
			}
		}
		out = append(out, app)
	}
	return out
}

// navCategoriesFor turns the campaign's entity types into top-level categories
// with their sub-categories, which get rows of their own. Counts are the
// viewer's (a parent's count already includes its sub-categories). Sub-
// categories follow their own order among siblings.
func navCategoriesFor(campaignID string, types []layouts.SidebarEntityType, counts map[int]int) []campaigns.NavCategory {
	base := "/campaigns/" + campaignID + "/"
	subs := make(map[int][]layouts.SidebarEntityType)
	for _, t := range types {
		if t.ParentTypeID != nil {
			subs[*t.ParentTypeID] = append(subs[*t.ParentTypeID], t)
		}
	}
	var out []campaigns.NavCategory
	for _, t := range types {
		if t.ParentTypeID != nil {
			continue
		}
		cat := campaigns.NavCategory{
			TypeID: t.ID, Label: t.NamePlural, Icon: layouts.SafeNavIcon(t.Icon, ""),
			Color: t.Color, URL: base + t.Slug, Count: counts[t.ID],
		}
		if cat.Label == "" {
			cat.Label = t.Name
		}
		children := subs[t.ID]
		sort.SliceStable(children, func(i, j int) bool {
			if children[i].SortOrder != children[j].SortOrder {
				return children[i].SortOrder < children[j].SortOrder
			}
			return children[i].Name < children[j].Name
		})
		for _, c := range children {
			label := c.NamePlural
			if label == "" {
				label = c.Name
			}
			cat.Subs = append(cat.Subs, campaigns.NavSubcategory{
				TypeID: c.ID, Label: label, Color: c.Color, URL: base + c.Slug, Count: counts[c.ID],
			})
		}
		out = append(out, cat)
	}
	return out
}

// navAccessFor is what the viewer may open: members get member-only apps,
// any signed-in viewer gets signed-in ones, and a public visitor only the
// pages open to anyone.
func navAccessFor(cc *campaigns.CampaignContext) campaigns.NavAccess {
	switch {
	case cc.IsMember && cc.MemberRole >= campaigns.RolePlayer:
		return campaigns.NavAccessMember
	case !cc.IsAnonymous:
		return campaigns.NavAccessSignedIn
	default:
		return campaigns.NavAccessAnyone
	}
}

// navSectionViews maps the viewer's sections onto the layout's view rows.
func navSectionViews(secs []campaigns.NavSection) []layouts.NavSectionView {
	out := make([]layouts.NavSectionView, 0, len(secs))
	for _, s := range secs {
		v := layouts.NavSectionView{ID: s.ID, Kind: s.Kind, Label: s.Label}
		for _, r := range s.Rows {
			row := layouts.NavRowView{
				Key: r.Key, Kind: r.Kind, Label: r.Label, Icon: r.Icon, Color: r.Color,
				URL: r.URL, Caption: r.Caption, External: r.External, Count: r.Count,
				Hidden: r.Hidden, Personal: r.Personal,
			}
			if r.Kind == campaigns.NavRowCategory {
				row.TypeID = navTypeID(r.Key)
			}
			for _, sub := range r.Subs {
				row.Subs = append(row.Subs, layouts.NavSubRowView{
					Key: campaigns.NavCategoryKey(sub.TypeID), TypeID: sub.TypeID,
					Label: sub.Label, Color: sub.Color, URL: sub.URL, Count: sub.Count,
				})
			}
			v.Rows = append(v.Rows, row)
		}
		out = append(out, v)
	}
	return out
}

// navTypeID reads the entity type id back out of a category key ("cat:12").
func navTypeID(key string) int {
	const prefix = "cat:"
	if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
		return 0
	}
	id, err := strconv.Atoi(key[len(prefix):])
	if err != nil {
		return 0
	}
	return id
}

// buildNavSections is the whole sidebar pipeline for one request: normalize
// the owner's stored items, cut them down for this viewer, map to view rows.
func buildNavSections(cc *campaigns.CampaignContext, owner bool, pins []string,
	types []layouts.SidebarEntityType, counts map[int]int,
	enabled map[string]bool, sys layouts.EnabledSystem) []layouts.NavSectionView {
	apps := navAppsFor(cc.Campaign.ID, enabled, sys)
	cats := navCategoriesFor(cc.Campaign.ID, types, counts)
	layout := campaigns.NormalizeNav(cc.Campaign.ParseSidebarConfig().Items, apps, cats)
	viewer := campaigns.NavViewer{Owner: owner, Access: navAccessFor(cc), Pins: pins}
	return navSectionViews(campaigns.ViewNav(layout, apps, cats, viewer))
}
