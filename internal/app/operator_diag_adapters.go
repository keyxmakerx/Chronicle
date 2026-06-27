package app

import (
	"context"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// operator_diag_adapters.go injects read-only campaign-data windows into the
// operator diagnostics (the AI Workflow Support workspace). The systems package
// must not import entities, so the app layer implements the provider interfaces
// here and wires them at startup — mirroring SetInstalledPackagesProvider.

// entityDiagAdapter implements systems.EntityDiagProvider against the entities
// service: a read-only reader for entity.fields (one entity's stored values) and
// entity.field-coverage (how populated a type's declared fields are). Read-only
// by construction — only Get/List calls, never a mutation.
type entityDiagAdapter struct {
	entities entities.EntityService
}

// EntityFields resolves an entity by ID (then by slug within the campaign) and
// returns its stored field map. Campaign scope is enforced so the diagnostic
// can't read across campaigns.
func (a entityDiagAdapter) EntityFields(ctx context.Context, campaignID, idOrSlug string) (systems.EntityFieldDump, error) {
	ent, err := a.entities.GetByID(ctx, idOrSlug)
	if err != nil || ent == nil || (campaignID != "" && ent.CampaignID != campaignID) {
		ent, err = a.entities.GetBySlug(ctx, campaignID, idOrSlug)
	}
	if err != nil {
		return systems.EntityFieldDump{}, err
	}
	if ent == nil || (campaignID != "" && ent.CampaignID != campaignID) {
		return systems.EntityFieldDump{Found: false}, nil
	}
	return systems.EntityFieldDump{
		Found:    true,
		ID:       ent.ID,
		Name:     ent.Name,
		TypeName: ent.TypeName,
		Fields:   ent.FieldsData,
	}, nil
}

// TypeFieldCoverage counts, for each declared field of a type, how many of its
// entities have a non-empty value. Entities are pulled with a generous cap; role
// 3 (owner) sees all.
func (a entityDiagAdapter) TypeFieldCoverage(ctx context.Context, campaignID, typeRef string) (systems.FieldCoverage, error) {
	et, err := a.resolveType(ctx, campaignID, typeRef)
	if err != nil {
		return systems.FieldCoverage{}, err
	}
	if et == nil {
		return systems.FieldCoverage{Found: false}, nil
	}
	list, _, err := a.entities.List(ctx, campaignID, et.ID, 3, "", entities.ListOptions{Page: 1, PerPage: 1000, Sort: "name"})
	if err != nil {
		return systems.FieldCoverage{}, err
	}
	rows := make([]systems.FieldCoverageRow, 0, len(et.Fields))
	for _, fd := range et.Fields {
		n := 0
		for i := range list {
			if nonEmptyField(list[i].FieldsData, fd.Key) {
				n++
			}
		}
		rows = append(rows, systems.FieldCoverageRow{Key: fd.Key, Label: fd.Label, NonEmpty: n})
	}
	return systems.FieldCoverage{Found: true, TypeName: et.Name, EntityCount: len(list), Declared: rows}, nil
}

// EntityTypes lists a campaign's entity types with per-type entity counts, for
// the entity.types discovery diagnostic.
func (a entityDiagAdapter) EntityTypes(ctx context.Context, campaignID string) ([]systems.EntityTypeInfo, error) {
	types, err := a.entities.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	// role 3 (owner) so counts include all entities.
	counts, err := a.entities.CountByType(ctx, campaignID, 3, "")
	if err != nil {
		counts = map[int]int{} // counts are best-effort; still list the types
	}
	out := make([]systems.EntityTypeInfo, 0, len(types))
	for i := range types {
		preset := ""
		if types[i].PresetCategory != nil {
			preset = *types[i].PresetCategory
		}
		out = append(out, systems.EntityTypeInfo{
			ID:             types[i].ID,
			Name:           types[i].Name,
			Slug:           types[i].Slug,
			PresetCategory: preset,
			Count:          counts[types[i].ID],
		})
	}
	return out, nil
}

// resolveType accepts a numeric type id, a slug, or a (case-insensitive) name.
func (a entityDiagAdapter) resolveType(ctx context.Context, campaignID, ref string) (*entities.EntityType, error) {
	if id, err := strconv.Atoi(ref); err == nil {
		return a.entities.GetEntityTypeByID(ctx, id)
	}
	if et, err := a.entities.GetEntityTypeBySlug(ctx, campaignID, ref); err == nil && et != nil {
		return et, nil
	}
	types, err := a.entities.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	for i := range types {
		if strings.EqualFold(types[i].Name, ref) || strings.EqualFold(types[i].Slug, ref) {
			return &types[i], nil
		}
	}
	return nil, nil
}

// nonEmptyField reports whether fields_data has a meaningful value for key.
func nonEmptyField(fd map[string]any, key string) bool {
	if fd == nil {
		return false
	}
	v, ok := fd[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) != ""
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	default:
		return true
	}
}
