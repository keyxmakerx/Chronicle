package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// system_panels_repair.go is the one-time reconciler for categories whose
// layout was saved as only the Game System Panels block. That happened when
// the panels were placed into a category that stored no rows (meaning "use the
// default layout"), which left its pages with no title, entry or attributes.
// Only that exact shape is repaired; see entities.repairSystemPanelsOnlyLayout.

// systemPanelsRepairKey is the site setting that records the reconciler ran.
const systemPanelsRepairKey = "reconcile.system_panels_only_layout"

// systemPanelsRepairEntities is the slice of the entity service it needs.
type systemPanelsRepairEntities interface {
	RepairSystemPanelsLayouts(ctx context.Context, campaignID string) (int, error)
}

// repairSystemPanelsLayoutsOnce repairs every campaign unless that already
// happened. The setting is written only after every campaign was attempted, so
// a crash part-way repeats the sweep, which is harmless: a repaired layout no
// longer matches.
func repairSystemPanelsLayoutsOnce(ctx context.Context, st pageExtrasSettings, camps pageExtrasCampaigns, ents systemPanelsRepairEntities) (int, error) {
	if v, err := st.Get(ctx, systemPanelsRepairKey); err == nil && v == "1" {
		return 0, nil
	} else if err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != 404 {
			return 0, fmt.Errorf("reading %s: %w", systemPanelsRepairKey, err)
		}
	}

	changed := 0
	for page := 1; ; page++ {
		list, total, err := camps.ListAll(ctx, campaigns.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return changed, fmt.Errorf("listing campaigns: %w", err)
		}
		for _, c := range list {
			n, err := ents.RepairSystemPanelsLayouts(ctx, c.ID)
			if err != nil {
				slog.Warn("system panels repair failed",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
				continue
			}
			changed += n
		}
		if len(list) == 0 || page*100 >= total {
			break
		}
	}
	if err := st.Set(ctx, systemPanelsRepairKey, "1"); err != nil {
		return changed, fmt.Errorf("recording %s: %w", systemPanelsRepairKey, err)
	}
	return changed, nil
}
