package app

import (
	"context"
	"log/slog"
	"slices"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// npcTypeLister is the one capability the panel resolver needs: the page
// types the owner listed as NPCs.
type npcTypeLister interface {
	NPCTypeIDs(ctx context.Context, campaignID string) ([]int, error)
}

// newSystemPanelResolver builds the resolver the entity show page consults: it
// picks the campaign's enabled system, keeps the manifest panels whose audience
// matches the page, and applies the NPC-family rule shared with the NPC gallery
// (the owner's NPC page types). A page a player has claimed is their character, never an NPC,
// even when its type also holds NPCs. The entities plugin only sees the
// finished list, so it never learns about systems or manifests.
func newSystemPanelResolver(sys enabledSystemResolver, types npcTypeLister) entities.SystemPanelResolver {
	return func(ctx context.Context, campaignID string, et *entities.EntityType, claimed bool) []entities.SystemPanel {
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
			if p.AppliesTo != systems.EntityPanelAppliesNPC || claimed {
				continue
			}
			if !npcChecked {
				npcChecked = true
				all, err := types.NPCTypeIDs(ctx, campaignID)
				if err != nil {
					// A page view must not fail over an optional panel.
					slog.Warn("system panels: entity types unavailable",
						slog.String("campaign_id", campaignID), slog.Any("error", err))
					return nil
				}
				npcPage = slices.Contains(all, et.ID)
			}
			if npcPage {
				panels = append(panels, entities.SystemPanel{Widget: p.Widget, SystemID: manifest.ID})
			}
		}
		return panels
	}
}
