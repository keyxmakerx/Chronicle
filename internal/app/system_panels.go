package app

import (
	"context"
	"log/slog"
	"slices"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// entityTypeLister is the one entities capability the panel resolver needs.
type entityTypeLister interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
}

// newSystemPanelResolver builds the resolver the entity show page consults: it
// picks the campaign's enabled system, keeps the manifest panels whose audience
// matches the page, and applies the NPC-family rule shared with the NPC gallery
// (npcTypeIDs). The entities plugin only sees the finished list, so it never
// learns about systems or manifests.
func newSystemPanelResolver(sys enabledSystemResolver, types entityTypeLister) entities.SystemPanelResolver {
	return func(ctx context.Context, campaignID string, et *entities.EntityType) []entities.SystemPanel {
		if et == nil {
			return nil
		}
		active := sys.EnabledSystem(ctx, campaignID)
		if active == nil {
			return nil
		}
		manifest := active.Info()
		// Cheap exit before the entity-type query: most systems ship no panels.
		if manifest == nil || len(manifest.EntityPanels) == 0 {
			return nil
		}
		var panels []entities.SystemPanel
		npcPage := false
		npcChecked := false
		for _, p := range manifest.EntityPanels {
			if p.AppliesTo != systems.EntityPanelAppliesNPC {
				continue
			}
			if !npcChecked {
				npcChecked = true
				all, err := types.GetEntityTypes(ctx, campaignID)
				if err != nil {
					// A page view must not fail over an optional panel.
					slog.Warn("system panels: entity types unavailable",
						slog.String("campaign_id", campaignID), slog.Any("error", err))
					return nil
				}
				npcPage = slices.Contains(npcTypeIDs(all), et.ID)
			}
			if npcPage {
				panels = append(panels, entities.SystemPanel{Widget: p.Widget, SystemID: manifest.ID})
			}
		}
		return panels
	}
}
