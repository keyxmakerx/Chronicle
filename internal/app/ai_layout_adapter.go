package app

import (
	"context"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func init() {
	addAIKinds(187, func(d *aiDeps) []records.Kind {
		return []records.Kind{records.LayoutKind{Svc: d.layouts()}}
	})
}

// layouts is the category page layouts' adapter; the owner rule is the
// Customizer's, which every categories route gates on the owner role.
func (d *aiDeps) layouts() records.LayoutsAPI {
	return aiLayoutAdapter{
		owners: aiMenuAdapter{campaigns: d.Campaigns, types: d.Entities},
		ents:   d.Entities, named: d.PageLayouts, addons: d.Addons,
	}
}

// aiLayoutAdapter implements records.LayoutsAPI over the entity service,
// saving a layout the way the Customizer's categories section does, so the
// same block, width, singleton and size rules apply.
type aiLayoutAdapter struct {
	owners aiMenuAdapter
	ents   entities.EntityService
	named  entities.PageLayoutService // nil while its tables are degraded
	addons aiAddonChecker
}

var _ records.LayoutsAPI = aiLayoutAdapter{}

func (l aiLayoutAdapter) IsOwner(ctx context.Context, campaignID, userID string) (bool, error) {
	return l.owners.IsOwner(ctx, campaignID, userID)
}

func (l aiLayoutAdapter) Categories(ctx context.Context, campaignID string) ([]entities.EntityType, error) {
	return l.ents.GetEntityTypes(ctx, campaignID)
}

func (l aiLayoutAdapter) Named(ctx context.Context, et *entities.EntityType) (entities.PageLayouts, bool) {
	if l.named == nil {
		return entities.PageLayouts{}, false
	}
	return l.named.ForType(ctx, et), true
}

func (l aiLayoutAdapter) BlockTypes(ctx context.Context, campaignID string) []records.LayoutBlockInfo {
	reg := entities.GetGlobalBlockRegistry()
	if reg == nil {
		return nil
	}
	var out []records.LayoutBlockInfo
	for _, m := range reg.TypesForCampaignAndContext(ctx, campaignID, l.addons, "template") {
		if m.Retired {
			continue
		}
		out = append(out, records.LayoutBlockInfo{Type: m.Type, Label: m.Label, Description: m.Description})
	}
	return out
}

func (l aiLayoutAdapter) Check(ctx context.Context, campaignID string, _ int, layout entities.EntityTypeLayout) error {
	reg := entities.GetGlobalBlockRegistry()
	if reg == nil {
		return entities.ValidateLayout(layout, nil, nil, nil)
	}
	// Kept tile sections are checked against the sizes their blocks declare.
	sizes := func(blockType string, config map[string]any) []string {
		return reg.TileSizesFor(ctx, campaignID, blockType, config)
	}
	return entities.ValidateLayout(layout, reg.IsValid, reg.IsSingleton, sizes)
}

func (l aiLayoutAdapter) SaveMain(ctx context.Context, campaignID string, typeID int, layout entities.EntityTypeLayout) error {
	key := strconv.Itoa(typeID)
	_, err := l.ents.ApplyCustomizeCategories(ctx, campaignID, entities.CustomizeCategoriesPatch{
		Items: map[string]entities.CustomizeCategoryItem{key: {Layout: &layout}},
	})
	return err
}

func (l aiLayoutAdapter) SaveNamed(ctx context.Context, campaignID string, typeID int, p entities.PageLayouts) error {
	key := strconv.Itoa(typeID)
	_, err := l.ents.ApplyCustomizeCategories(ctx, campaignID, entities.CustomizeCategoriesPatch{
		Items: map[string]entities.CustomizeCategoryItem{key: {PageLayouts: &p}},
	})
	return err
}
