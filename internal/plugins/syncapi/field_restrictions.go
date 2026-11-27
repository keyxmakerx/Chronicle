package syncapi

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// restrictToCampaign applies the campaign's own GM-only and owner-only marks
// to a preset's Foundry field export. A Director can restrict a field on the
// campaign's category that the manifest leaves open, and the Foundry module
// must then treat it the same way. Every category made from the preset counts
// (matched by slug or preset category, and their sub-categories), and a mark
// on any of them restricts the field: more restriction is the safe side.
// Fails closed: without the categories nothing says which fields are hidden.
func (h *APIHandler) restrictToCampaign(ctx context.Context, campaignID, category string, resp *systems.CharacterFieldsResponse) error {
	if resp == nil || len(resp.Fields) == 0 {
		return nil
	}
	types, err := h.entitySvc.GetEntityTypes(ctx, campaignID)
	if err != nil {
		slog.Error("api: field export could not load the campaign's categories",
			slog.String("campaign_id", campaignID), slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to load field definitions"))
	}
	gmOnly, ownerOnly := presetRestrictions(types, resp.PresetSlug, category)
	resp.Restrict(gmOnly, ownerOnly)
	return nil
}

// presetRestrictions collects the restricted field keys of every category
// made from one preset, and of their sub-categories.
func presetRestrictions(types []entities.EntityType, presetSlug, category string) (gmOnly, ownerOnly map[string]bool) {
	gmOnly, ownerOnly = map[string]bool{}, map[string]bool{}
	from := map[int]bool{}
	for _, t := range types {
		if (presetSlug != "" && t.Slug == presetSlug) ||
			(category != "" && t.PresetCategory != nil && *t.PresetCategory == category) {
			from[t.ID] = true
		}
	}
	for _, t := range types {
		if !from[t.ID] && (t.ParentTypeID == nil || !from[*t.ParentTypeID]) {
			continue
		}
		for _, f := range t.Fields {
			if f.GMOnly {
				gmOnly[f.Key] = true
			}
			if f.OwnerOnly {
				ownerOnly[f.Key] = true
			}
		}
	}
	return gmOnly, ownerOnly
}
