package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// material_page_look.go is the one-time reconciler that moves every existing
// campaign whose owner never chose a page look to Material. A Standard pick
// is stored by name and skipped. It runs once per site all the same: a
// build that stored Standard as unset may have run before this one.

// materialPageLookKey is the site setting that records the reconciler ran.
const materialPageLookKey = "reconcile.material_page_look"

// materialPageLookCampaigns is the slice of the campaign service it needs.
type materialPageLookCampaigns interface {
	ListAll(ctx context.Context, opts campaigns.ListOptions) ([]campaigns.Campaign, int, error)
	AdoptMaterialPageLook(ctx context.Context, campaignID string) (bool, error)
}

// materialPageLookOnce moves every campaign with no page look to Material
// unless that already happened. The setting is written once every campaign
// was attempted; a crash part-way repeats the sweep at the next boot, which
// runs before the site serves anyone, so no owner can have picked Standard.
func materialPageLookOnce(ctx context.Context, st pageExtrasSettings, camps materialPageLookCampaigns) (int, error) {
	if v, err := st.Get(ctx, materialPageLookKey); err == nil && v == "1" {
		return 0, nil
	} else if err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != 404 {
			return 0, fmt.Errorf("reading %s: %w", materialPageLookKey, err)
		}
	}

	changed, failed := 0, 0
	for page := 1; ; page++ {
		list, total, err := camps.ListAll(ctx, campaigns.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return changed, fmt.Errorf("listing campaigns: %w", err)
		}
		for _, c := range list {
			ok, err := camps.AdoptMaterialPageLook(ctx, c.ID)
			if err != nil {
				slog.Warn("material page look: campaign not moved",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
				failed++
				continue
			}
			if ok {
				changed++
			}
		}
		if len(list) == 0 || page*100 >= total {
			break
		}
	}
	// A campaign that failed is not retried: by the next boot its owner may
	// have picked Standard, which looks the same as unset. It stays in
	// Standard, one switch away from Material.
	if err := st.Set(ctx, materialPageLookKey, "1"); err != nil {
		return changed, fmt.Errorf("recording %s: %w", materialPageLookKey, err)
	}
	if failed > 0 {
		return changed, fmt.Errorf("%d campaigns stayed in the Standard page look", failed)
	}
	return changed, nil
}
