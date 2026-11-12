package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// panel_only_layouts.go is the one-time reconciler for categories whose stored
// layout is just the Game System Panels block. Placing the block on an empty
// layout used to leave exactly that, which hid the title, picture,
// description and attributes. Each such layout goes back to the default page
// with the block under the title. A layout with anything else in it is left
// alone.

// panelOnlyLayoutsKey is the site setting that records the reconciler ran.
const panelOnlyLayoutsKey = "reconcile.panel_only_layouts"

// panelOnlyLayoutsEntities is the slice of the entity service it needs.
type panelOnlyLayoutsEntities interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	UpdateEntityTypeLayout(ctx context.Context, id int, layout entities.EntityTypeLayout) error
}

// repairPanelOnlyLayoutsOnce repairs every campaign's panel-only layouts
// unless that already happened. The setting is written only after every
// campaign was attempted; a repeat finds nothing left to change.
func repairPanelOnlyLayoutsOnce(ctx context.Context, st pageExtrasSettings, camps pageExtrasCampaigns, ents panelOnlyLayoutsEntities) (int, error) {
	if v, err := st.Get(ctx, panelOnlyLayoutsKey); err == nil && v == "1" {
		return 0, nil
	} else if err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != 404 {
			return 0, fmt.Errorf("reading %s: %w", panelOnlyLayoutsKey, err)
		}
	}

	changed := 0
	for page := 1; ; page++ {
		list, total, err := camps.ListAll(ctx, campaigns.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return changed, fmt.Errorf("listing campaigns: %w", err)
		}
		for _, c := range list {
			types, err := ents.GetEntityTypes(ctx, c.ID)
			if err != nil {
				slog.Warn("panel-only layouts: entity types unavailable",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
				continue
			}
			for _, et := range types {
				layout, ok := entities.RepairPanelOnlyLayout(et.Layout)
				if !ok {
					continue
				}
				if err := ents.UpdateEntityTypeLayout(ctx, et.ID, layout); err != nil {
					slog.Warn("panel-only layouts: repair failed",
						slog.String("campaign_id", c.ID), slog.Int("entity_type_id", et.ID), slog.Any("error", err))
					continue
				}
				changed++
			}
		}
		if len(list) == 0 || page*100 >= total {
			break
		}
	}
	if err := st.Set(ctx, panelOnlyLayoutsKey, "1"); err != nil {
		return changed, fmt.Errorf("recording %s: %w", panelOnlyLayoutsKey, err)
	}
	return changed, nil
}
