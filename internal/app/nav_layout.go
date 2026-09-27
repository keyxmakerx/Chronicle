package app

import (
	"context"
	"sort"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
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

// navAccessFor is what the viewer may open: members get member-only apps, a
// site admin looking in gets the pages RequireCampaignAccess lets them open,
// and anyone else (a public visitor, signed in or not) only the pages open to
// anyone.
func navAccessFor(cc *campaigns.CampaignContext) campaigns.NavAccess {
	switch {
	case cc.IsMember && cc.MemberRole >= campaigns.RolePlayer:
		return campaigns.NavAccessMember
	case cc.IsSiteAdmin && !cc.IsAnonymous:
		return campaigns.NavAccessMemberOrAdmin
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
	kind, ref, ok := campaigns.NavKeyKind(key)
	if !ok || kind != campaigns.NavRowCategory {
		return 0
	}
	id, err := strconv.Atoi(ref)
	if err != nil {
		return 0
	}
	return id
}

// navInputs is what one campaign's sidebar is built from: its apps, its
// categories and the owner's arrangement of them.
type navInputs struct {
	apps   []campaigns.NavApp
	cats   []campaigns.NavCategory
	layout campaigns.NavLayout
}

// navInputsFor resolves the apps and categories and normalizes the owner's
// stored items over them.
func navInputsFor(cc *campaigns.CampaignContext, types []layouts.SidebarEntityType,
	counts map[int]int, enabled map[string]bool, sys layouts.EnabledSystem) navInputs {
	apps := navAppsFor(cc.Campaign.ID, enabled, sys)
	cats := navCategoriesFor(cc.Campaign.ID, types, counts)
	return navInputs{
		apps:   apps,
		cats:   cats,
		layout: campaigns.NormalizeNav(cc.Campaign.ParseSidebarConfig().Items, apps, cats),
	}
}

// viewNavSections cuts the arrangement down for one viewer and maps it to
// view rows.
func viewNavSections(cc *campaigns.CampaignContext, in navInputs, owner bool, pins []string) []layouts.NavSectionView {
	viewer := campaigns.NavViewer{Owner: owner, Access: navAccessFor(cc), Pins: pins}
	return navSectionViews(campaigns.ViewNav(in.layout, in.apps, in.cats, viewer))
}

// buildNavSections is the whole sidebar pipeline for one request: normalize
// the owner's stored items, cut them down for this viewer, map to view rows.
func buildNavSections(cc *campaigns.CampaignContext, owner bool, pins []string,
	types []layouts.SidebarEntityType, counts map[int]int,
	enabled map[string]bool, sys layouts.EnabledSystem) []layouts.NavSectionView {
	return viewNavSections(cc, navInputsFor(cc, types, counts, enabled, sys), owner, pins)
}

// buildNavEdit is the owner's editor model: the whole arrangement, with rows
// hidden from players and turned-off apps kept in place and marked, plus the
// turned-off apps for the editor's tray. Only ever given to the owner.
func buildNavEdit(in navInputs) *layouts.NavEditView {
	appBySlug := make(map[string]campaigns.NavApp, len(in.apps))
	for _, a := range in.apps {
		appBySlug[a.Slug] = a
	}
	catByKey := make(map[string]campaigns.NavCategory, len(in.cats))
	for _, c := range in.cats {
		catByKey[campaigns.NavCategoryKey(c.TypeID)] = c
	}

	out := &layouts.NavEditView{Sections: []layouts.NavEditSection{}, Off: []layouts.NavEditRow{}}
	for _, s := range in.layout.Sections {
		sec := layouts.NavEditSection{ID: s.ID, Kind: s.ID, Label: s.Label, Items: []layouts.NavEditRow{}}
		switch s.ID {
		case campaigns.NavSectionPinned:
			sec.Label = "Pinned"
		case campaigns.NavSectionApps:
			sec.Label = "Apps"
		case campaigns.NavSectionCategories:
			sec.Label = "Categories"
		default:
			sec.Kind = campaigns.NavKindCustom
		}
		for _, it := range s.Items {
			kind, ref, ok := campaigns.NavKeyKind(it.Key)
			if !ok {
				continue
			}
			row := layouts.NavEditRow{Key: it.Key, Kind: kind, Hidden: it.Hidden}
			switch kind {
			case campaigns.NavRowApp:
				a, found := appBySlug[ref]
				if !found {
					continue
				}
				row.Label, row.Icon, row.Off = a.Label, a.Icon, !a.Enabled
			case campaigns.NavRowCategory:
				c, found := catByKey[it.Key]
				if !found {
					continue
				}
				row.Label, row.Icon, row.Color = c.Label, c.Icon, c.Color
			case campaigns.NavRowLink:
				row.Label, row.Icon, row.URL = it.Label, it.Icon, it.URL
			}
			sec.Items = append(sec.Items, row)
		}
		out.Sections = append(out.Sections, sec)
	}
	for _, a := range in.apps {
		if !a.Enabled {
			out.Off = append(out.Off, layouts.NavEditRow{
				Key: campaigns.NavAppKey(a.Slug), Kind: campaigns.NavRowApp, Label: a.Label, Icon: a.Icon, Off: true,
			})
		}
	}
	return out
}

// sidebarTypesFrom maps a campaign's entity types to the layout's.
func sidebarTypesFrom(etypes []entities.EntityType) []layouts.SidebarEntityType {
	out := make([]layouts.SidebarEntityType, len(etypes))
	for i, et := range etypes {
		out[i] = layouts.SidebarEntityType{
			ID:           et.ID,
			Slug:         et.Slug,
			Name:         et.Name,
			NamePlural:   et.NamePlural,
			Icon:         et.Icon,
			Color:        et.Color,
			SortOrder:    et.SortOrder,
			ParentTypeID: et.ParentTypeID,
		}
	}
	return out
}

// navAddonsFrom reads which addons a campaign has on, and the game system
// behind its rulebook. Systems are mutually exclusive, so the first enabled
// one wins.
func navAddonsFrom(list []addons.CampaignAddon) (map[string]bool, layouts.EnabledSystem) {
	enabled := make(map[string]bool)
	var sys layouts.EnabledSystem
	for _, ca := range list {
		if !ca.Enabled {
			continue
		}
		enabled[ca.AddonSlug] = true
		if ca.AddonCategory == addons.CategorySystem && sys.Slug == "" {
			sys = layouts.EnabledSystem{Slug: ca.AddonSlug, Name: ca.AddonName, Icon: ca.AddonIcon}
		}
	}
	return enabled, sys
}

// navSectionsSource draws a member's sidebar, before their own pins, for the
// campaigns plugin's pin check (campaigns.NavSectionsSource). It gathers the
// same inputs the LayoutInjector does, so a member can pin exactly the rows
// their sidebar shows.
type navSectionsSource struct {
	entities interface {
		GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	}
	addons interface {
		ListForCampaign(ctx context.Context, campaignID string) ([]addons.CampaignAddon, error)
	}
}

// NavSectionsFor implements campaigns.NavSectionsSource. The viewer is never
// the owner here: only non-owner members have pins of their own.
func (s *navSectionsSource) NavSectionsFor(ctx context.Context, cc *campaigns.CampaignContext) ([]campaigns.NavSection, error) {
	etypes, err := s.entities.GetEntityTypes(ctx, cc.Campaign.ID)
	if err != nil {
		return nil, err
	}
	list, err := s.addons.ListForCampaign(ctx, cc.Campaign.ID)
	if err != nil {
		return nil, err
	}
	enabled, sys := navAddonsFrom(list)
	in := navInputsFor(cc, sidebarTypesFrom(etypes), nil, enabled, sys)
	viewer := campaigns.NavViewer{Access: navAccessFor(cc)}
	return campaigns.ViewNav(in.layout, in.apps, in.cats, viewer), nil
}
