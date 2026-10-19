package app

import (
	"context"
	"log/slog"
	"slices"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// panelTypeLister is what the panel resolver needs: the page types the owner
// listed as NPCs, and the campaign's creature types.
type panelTypeLister interface {
	NPCTypeIDs(ctx context.Context, campaignID string) ([]int, error)
	CreatureTypeIDs(ctx context.Context, campaignID string) ([]int, error)
}

// newSystemPanelResolver builds the resolver the entity show page consults: it
// picks the campaign's enabled system and keeps the manifest panels whose
// audience matches the page. "npc" follows the rule shared with the NPC gallery
// (the owner's NPC page types); "creature" follows the page's type, so a
// bestiary page gets its stat block without being listed as an NPC. A page a
// player has claimed is their character, never an NPC or creature, even when
// its type also holds those. A widget aimed at both audiences mounts once. The
// entities plugin only sees the finished list, so it never learns about
// systems or manifests.
func newSystemPanelResolver(sys enabledSystemResolver, types panelTypeLister) entities.SystemPanelResolver {
	return func(ctx context.Context, campaignID string, et *entities.EntityType, claimed bool) []entities.SystemPanel {
		if et == nil || claimed {
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
		// Each audience is looked up at most once, and only if a panel needs it.
		matched := map[string]bool{}
		matches := func(audience string) (bool, error) {
			if m, ok := matched[audience]; ok {
				return m, nil
			}
			var ids []int
			var err error
			switch audience {
			case systems.EntityPanelAppliesNPC:
				ids, err = types.NPCTypeIDs(ctx, campaignID)
			case systems.EntityPanelAppliesCreature:
				ids, err = types.CreatureTypeIDs(ctx, campaignID)
			default:
				return false, nil
			}
			if err != nil {
				return false, err
			}
			matched[audience] = slices.Contains(ids, et.ID)
			return matched[audience], nil
		}
		var panels []entities.SystemPanel
		for _, p := range manifest.EntityPanels {
			ok, err := matches(p.AppliesTo)
			if err != nil {
				// A page view must not fail over an optional panel.
				slog.Warn("system panels: entity types unavailable",
					slog.String("campaign_id", campaignID), slog.Any("error", err))
				return nil
			}
			panel := entities.SystemPanel{Widget: p.Widget, SystemID: manifest.ID}
			if ok && !slices.Contains(panels, panel) {
				panels = append(panels, panel)
			}
		}
		return panels
	}
}
