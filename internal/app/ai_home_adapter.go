package app

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func init() {
	addAIKinds(186, func(d *aiDeps) []records.Kind {
		return []records.Kind{records.HomeKind{Svc: aiHomeAdapter{campaigns: d.Campaigns}}}
	})
}

// aiHomeAdapter implements records.HomeAPI over the campaigns service, saving
// a home the way the Customizer does: the same layout rules, merged into the
// role-keyed campaign home.
type aiHomeAdapter struct {
	campaigns campaigns.CampaignService
}

var _ records.HomeAPI = aiHomeAdapter{}

func (h aiHomeAdapter) IsOwner(ctx context.Context, campaignID, userID string) (bool, error) {
	return aiMenuAdapter{campaigns: h.campaigns}.IsOwner(ctx, campaignID, userID)
}

func (h aiHomeAdapter) BlockTypes() []string { return campaigns.HomeBlockTypes() }

func (h aiHomeAdapter) Home(ctx context.Context, campaignID, audience string) (records.HomeLayout, bool, error) {
	c, err := h.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return records.HomeLayout{}, false, err
	}
	l, saved := c.HomeLayout(audience)
	return homeFromLayout(l), saved, nil
}

func (h aiHomeAdapter) Check(ctx context.Context, campaignID string, layout records.HomeLayout) error {
	return campaigns.ValidateHomeLayout(ctx, campaignID, homeToLayout(layout))
}

func (h aiHomeAdapter) Save(ctx context.Context, campaignID, audience string, layout records.HomeLayout) error {
	if !campaigns.ValidHomeAudience(audience) {
		return apperror.NewBadRequest("unknown home: " + audience)
	}
	l := homeToLayout(layout)
	if err := campaigns.ValidateHomeLayout(ctx, campaignID, l); err != nil {
		return err
	}
	return h.store(ctx, campaignID, audience, l)
}

func (h aiHomeAdapter) Reset(ctx context.Context, campaignID, audience string) error {
	if !campaigns.ValidHomeAudience(audience) {
		return apperror.NewBadRequest("unknown home: " + audience)
	}
	return h.store(ctx, campaignID, audience, nil)
}

func (h aiHomeAdapter) store(ctx context.Context, campaignID, audience string, l *campaigns.DashboardLayout) error {
	c, err := h.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return err
	}
	full, err := c.SetRoleDashboardJSON(audience, l)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling role layout: %w", err))
	}
	return h.campaigns.UpdateDashboardLayoutRaw(ctx, campaignID, full)
}

func homeFromLayout(l *campaigns.DashboardLayout) records.HomeLayout {
	var out records.HomeLayout
	for _, row := range l.Rows {
		if row.Kind != "" || len(row.Tiles) > 0 {
			out.Tiles = true
			continue
		}
		r := records.HomeRow{ID: row.ID}
		for _, c := range row.Columns {
			col := records.HomeColumn{ID: c.ID, Width: c.Width}
			for _, b := range c.Blocks {
				col.Blocks = append(col.Blocks, records.HomeBlock{ID: b.ID, Type: b.Type, Config: b.Config})
			}
			r.Columns = append(r.Columns, col)
		}
		out.Rows = append(out.Rows, r)
	}
	return out
}

func homeToLayout(l records.HomeLayout) *campaigns.DashboardLayout {
	out := &campaigns.DashboardLayout{}
	for _, row := range l.Rows {
		r := campaigns.DashboardRow{ID: row.ID}
		for _, c := range row.Columns {
			col := campaigns.DashboardColumn{ID: c.ID, Width: c.Width, Blocks: []campaigns.DashboardBlock{}}
			for _, b := range c.Blocks {
				col.Blocks = append(col.Blocks, campaigns.DashboardBlock{ID: b.ID, Type: b.Type, Config: b.Config})
			}
			r.Columns = append(r.Columns, col)
		}
		out.Rows = append(out.Rows, r)
	}
	return out
}
