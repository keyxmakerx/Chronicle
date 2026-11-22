package app

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// The AI's other templates: a category's own home, the saved layouts and the
// page templates. Each saves through the service the Customizer's route or
// section uses, and takes the owner rule from that route. They share one rank,
// so they keep this order in the prompt.
func init() {
	addAIKinds(188, func(d *aiDeps) []records.Kind {
		return []records.Kind{records.CategoryHomeKind{Svc: d.categoryHomes()}}
	})
	addAIKinds(188, func(d *aiDeps) []records.Kind {
		if svc := d.savedLayouts(); svc != nil {
			return []records.Kind{records.SavedLayoutKind{Svc: svc}}
		}
		return nil
	})
	addAIKinds(188, func(d *aiDeps) []records.Kind {
		if svc := d.pageTemplates(); svc != nil {
			return []records.Kind{records.PageTemplateKind{Svc: svc}}
		}
		return nil
	})
}

func (d *aiDeps) owners() aiMenuAdapter {
	return aiMenuAdapter{campaigns: d.Campaigns, types: d.Entities}
}

// categoryHomes is a category home's adapter; the owner rule is the one of the
// dashboard-layout routes, which gate on the owner role.
func (d *aiDeps) categoryHomes() records.CategoryHomesAPI {
	return aiCategoryHomeAdapter{owners: d.owners(), ents: d.Entities, addons: d.Addons}
}

// savedLayouts is the saved layouts' adapter, nil while the layouts' tables
// are degraded; the owner rule is the one of the layout-presets write routes.
func (d *aiDeps) savedLayouts() records.SavedLayoutsAPI {
	if d.SavedLayouts == nil {
		return nil
	}
	return aiSavedLayoutAdapter{owners: d.owners(), presets: d.SavedLayouts, check: aiLayoutAdapter{ents: d.Entities, addons: d.Addons}}
}

// pageTemplates is the page templates' adapter, nil while their tables are
// degraded; the owner rule is the one of the content-templates write routes.
func (d *aiDeps) pageTemplates() records.PageTemplatesAPI {
	if d.PageTemplates == nil {
		return nil
	}
	return aiPageTemplateAdapter{owners: d.owners(), templates: d.PageTemplates, ents: d.Entities}
}

// aiCategoryHomeAdapter implements records.CategoryHomesAPI over the entity
// service's category dashboard layout, so a save meets the rules of the
// editor's own.
type aiCategoryHomeAdapter struct {
	owners aiMenuAdapter
	ents   entities.EntityService
	addons aiAddonChecker
}

var _ records.CategoryHomesAPI = aiCategoryHomeAdapter{}

func (h aiCategoryHomeAdapter) IsOwner(ctx context.Context, campaignID, userID string) (bool, error) {
	return h.owners.IsOwner(ctx, campaignID, userID)
}

func (h aiCategoryHomeAdapter) Categories(ctx context.Context, campaignID string) ([]entities.EntityType, error) {
	return h.ents.GetEntityTypes(ctx, campaignID)
}

// BlockTypes are the editor's palette: what the category context offers,
// narrowed to the widgets a category's landing page draws.
func (h aiCategoryHomeAdapter) BlockTypes(ctx context.Context, campaignID string) []records.LayoutBlockInfo {
	offered := map[string]bool{}
	for _, t := range entities.CategoryHomeBlockTypes() {
		offered[t] = true
	}
	reg := entities.GetGlobalBlockRegistry()
	if reg == nil {
		return nil
	}
	var out []records.LayoutBlockInfo
	for _, m := range reg.TypesForCampaignAndContext(ctx, campaignID, h.addons, "category") {
		if m.Retired || !offered[m.Type] {
			continue
		}
		out = append(out, records.LayoutBlockInfo{Type: m.Type, Label: m.Label, Description: m.Description})
	}
	return out
}

func (h aiCategoryHomeAdapter) Check(ctx context.Context, _ string, typeID int, l campaigns.DashboardLayout) error {
	return h.ents.CheckCategoryDashboardLayout(ctx, typeID, l)
}

func (h aiCategoryHomeAdapter) Save(ctx context.Context, _ string, typeID int, l campaigns.DashboardLayout) error {
	b, err := json.Marshal(l)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return h.ents.UpdateCategoryDashboardLayout(ctx, typeID, string(b))
}

func (h aiCategoryHomeAdapter) Reset(ctx context.Context, _ string, typeID int) error {
	return h.ents.ResetCategoryDashboardLayout(ctx, typeID)
}

// aiSavedLayoutAdapter implements records.SavedLayoutsAPI over the layout
// preset service. The service does not look at whose preset an id is or
// whether it is built in (the route does), so the adapter does, and it checks
// a layout against the block palette, which the service skips.
type aiSavedLayoutAdapter struct {
	owners  aiMenuAdapter
	presets entities.LayoutPresetService
	check   aiLayoutAdapter
}

var _ records.SavedLayoutsAPI = aiSavedLayoutAdapter{}

func (s aiSavedLayoutAdapter) IsOwner(ctx context.Context, campaignID, userID string) (bool, error) {
	return s.owners.IsOwner(ctx, campaignID, userID)
}

func (s aiSavedLayoutAdapter) List(ctx context.Context, campaignID string) ([]entities.LayoutPreset, error) {
	return s.presets.ListForCampaign(ctx, campaignID)
}

func (s aiSavedLayoutAdapter) BlockTypes(ctx context.Context, campaignID string) []records.LayoutBlockInfo {
	return s.check.BlockTypes(ctx, campaignID)
}

func (s aiSavedLayoutAdapter) Check(ctx context.Context, campaignID string, l entities.EntityTypeLayout) error {
	return s.check.Check(ctx, campaignID, 0, l)
}

func (s aiSavedLayoutAdapter) Create(ctx context.Context, campaignID string, in entities.CreateLayoutPresetInput) (*entities.LayoutPreset, error) {
	return s.presets.Create(ctx, campaignID, in)
}

// own finds the campaign's own, editable saved layout.
func (s aiSavedLayoutAdapter) own(ctx context.Context, campaignID string, id int) error {
	p, err := s.presets.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p.CampaignID != campaignID {
		return apperror.NewNotFound("layout preset not found")
	}
	if p.IsBuiltin {
		return apperror.NewForbidden("cannot change built-in presets")
	}
	return nil
}

func (s aiSavedLayoutAdapter) Update(ctx context.Context, campaignID string, id int, in entities.UpdateLayoutPresetInput) error {
	if err := s.own(ctx, campaignID, id); err != nil {
		return err
	}
	_, err := s.presets.Update(ctx, id, in)
	return err
}

func (s aiSavedLayoutAdapter) Delete(ctx context.Context, campaignID string, id int) error {
	if err := s.own(ctx, campaignID, id); err != nil {
		return err
	}
	return s.presets.Delete(ctx, id)
}

// aiPageTemplateAdapter implements records.PageTemplatesAPI over the content
// template service. As with presets, the route and the Customizer's section
// refuse another campaign's and a shared template, so the adapter does.
type aiPageTemplateAdapter struct {
	owners    aiMenuAdapter
	templates entities.ContentTemplateService
	ents      entities.EntityService
}

var _ records.PageTemplatesAPI = aiPageTemplateAdapter{}

func (t aiPageTemplateAdapter) IsOwner(ctx context.Context, campaignID, userID string) (bool, error) {
	return t.owners.IsOwner(ctx, campaignID, userID)
}

func (t aiPageTemplateAdapter) Categories(ctx context.Context, campaignID string) ([]entities.EntityType, error) {
	return t.ents.GetEntityTypes(ctx, campaignID)
}

func (t aiPageTemplateAdapter) List(ctx context.Context, campaignID string) ([]entities.ContentTemplate, error) {
	return t.templates.ListForCampaign(ctx, campaignID)
}

func (t aiPageTemplateAdapter) Create(ctx context.Context, campaignID string, in entities.CreateContentTemplateInput) (*entities.ContentTemplate, error) {
	return t.templates.Create(ctx, campaignID, in)
}

// own finds the campaign's own, editable template.
func (t aiPageTemplateAdapter) own(ctx context.Context, campaignID string, id int) error {
	x, err := t.templates.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if x.CampaignID == nil || *x.CampaignID != campaignID || x.IsGlobal {
		return apperror.NewNotFound("content template not found")
	}
	return nil
}

func (t aiPageTemplateAdapter) Update(ctx context.Context, campaignID string, id int, in entities.UpdateContentTemplateInput) error {
	if err := t.own(ctx, campaignID, id); err != nil {
		return err
	}
	_, err := t.templates.Update(ctx, id, in)
	return err
}

func (t aiPageTemplateAdapter) Delete(ctx context.Context, campaignID string, id int) error {
	if err := t.own(ctx, campaignID, id); err != nil {
		return err
	}
	return t.templates.Delete(ctx, id)
}
