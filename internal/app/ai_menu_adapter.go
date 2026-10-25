package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

func init() {
	addAIKinds(185, func(d *aiDeps) []records.Kind {
		return []records.Kind{records.MenuOrderKind{Svc: aiMenuAdapter{campaigns: d.Campaigns, types: d.Entities}}}
	})
}

// aiMenuAdapter implements records.MenuAPI over the campaigns service's
// sidebar config, built from the same inputs the sidebar is drawn from.
type aiMenuAdapter struct {
	campaigns campaigns.CampaignService
	types     interface {
		GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	}
}

var _ records.MenuAPI = aiMenuAdapter{}

func (m aiMenuAdapter) IsOwner(ctx context.Context, campaignID, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	mem, err := m.campaigns.GetMember(ctx, campaignID, userID)
	if err != nil {
		// Not a member reads as not the owner; any other failure is reported.
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return mem.Role >= campaigns.RoleOwner, nil
}

// inputs is what the campaign's menu is built from. Which apps are turned on
// doesn't change where a category is drawn, so the catalog is enough.
func (m aiMenuAdapter) inputs(ctx context.Context, campaignID string) ([]campaigns.SidebarItem, []campaigns.NavApp, []campaigns.NavCategory, map[int]entities.EntityType, error) {
	c, err := m.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	etypes, err := m.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	byID := make(map[int]entities.EntityType, len(etypes))
	for _, t := range etypes {
		byID[t.ID] = t
	}
	apps := navAppsFor(campaignID, nil, layouts.EnabledSystem{})
	cats := navCategoriesFor(campaignID, sidebarTypesFrom(etypes), nil)
	return c.ParseSidebarConfig().Items, apps, cats, byID, nil
}

func (m aiMenuAdapter) MenuCategories(ctx context.Context, campaignID string) ([]records.MenuCategory, error) {
	items, apps, cats, byID, err := m.inputs(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return menuCategories(campaigns.NavCategoryOrder(items, apps, cats), byID), nil
}

func (m aiMenuAdapter) PreviewOrder(ctx context.Context, campaignID string, ids []int) ([]records.MenuCategory, error) {
	items, apps, cats, byID, err := m.inputs(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	next := campaigns.OrderNavCategories(items, apps, cats, ids)
	return menuCategories(campaigns.NavCategoryOrder(next, apps, cats), byID), nil
}

func (m aiMenuAdapter) OrderCategories(ctx context.Context, campaignID string, ids []int) error {
	_, apps, cats, _, err := m.inputs(ctx, campaignID)
	if err != nil {
		return err
	}
	return m.campaigns.OrderNavCategories(ctx, campaignID, apps, cats, ids)
}

func menuCategories(places []campaigns.NavCategoryPlace, byID map[int]entities.EntityType) []records.MenuCategory {
	out := make([]records.MenuCategory, 0, len(places))
	for _, p := range places {
		t := byID[p.TypeID]
		out = append(out, records.MenuCategory{ID: p.TypeID, Name: t.Name, Plural: t.NamePlural, Slug: t.Slug, Section: p.SectionTitle})
	}
	return out
}
