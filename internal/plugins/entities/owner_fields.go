package entities

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ownerFieldsFor is what the claimed owner may change from the Attributes box
// on their own page: the same keys the fields door lets them write, so a plain
// page (one with no system sheet) still gives them a way to set their ancestry,
// class or background. nil for anyone else; a Scribe edits every field.
func (h *Handler) ownerFieldsFor(ctx context.Context, cc *campaigns.CampaignContext, entity *Entity, et *EntityType, userID string) []string {
	if !IsClaimedOwnerPlayer(cc.MemberRole, entity, userID) {
		return nil
	}
	typeFields := append([]FieldDefinition{}, et.Fields...)
	if entity.FieldOverrides != nil {
		typeFields = append(typeFields, entity.FieldOverrides.Added...)
	}
	var plan *HeroPlan
	if h.heroPlanner != nil {
		// Without the plan only the identity texts are offered, which the
		// door accepts either way.
		if p, err := h.heroPlanner.HeroPlan(ctx, cc.Campaign.ID, userID, false); err == nil {
			plan = p
		}
	}
	return ownerEditableKeys(typeFields, plan)
}

// ownerEditableKeys lists, in the category's order, the fields a claimed
// owner may write. Lists of names are left out: they are JSON arrays a sheet
// edits, which a plain text box would only garble.
func ownerEditableKeys(typeFields []FieldDefinition, plan *HeroPlan) []string {
	var keys []string
	seen := map[string]bool{}
	for i := range typeFields {
		k := typeFields[i].Key
		if seen[k] {
			continue
		}
		seen[k] = true
		if kind, _ := OwnerFieldKindOf(k, typeFields, plan); kind != "" && kind != OwnerFieldList {
			keys = append(keys, k)
		}
	}
	return keys
}
