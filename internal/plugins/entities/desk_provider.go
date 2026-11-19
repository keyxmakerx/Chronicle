// desk_provider.go tells the owner's desk what waits in the campaign Trash.
// It lives here so campaigns never imports this plugin.

package entities

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// DeskProvider fills the desk's Trash line.
type DeskProvider struct{ svc PageSafetyService }

// NewDeskProvider returns the desk provider for the Trash.
func NewDeskProvider(svc PageSafetyService) *DeskProvider { return &DeskProvider{svc: svc} }

// Slot is the one desk line this provider fills.
func (p *DeskProvider) Slot() campaigns.DeskSlot { return campaigns.DeskTrash }

// Facts counts pages plus the other kinds the Trash holds (pictures, game
// nights, maps), the same list the Trash page draws, so the two agree.
func (p *DeskProvider) Facts(ctx context.Context, req campaigns.DeskRequest) (campaigns.DeskFacts, error) {
	pages, err := p.svc.ListTrash(ctx, req.CampaignID)
	if err != nil {
		return campaigns.DeskFacts{}, err
	}
	others := p.svc.ListOtherTrash(ctx, req.CampaignID)
	return campaigns.DeskFacts{Trash: &campaigns.DeskTrashFacts{
		Items: len(pages) + len(others),
		Days:  p.svc.RetentionDays(ctx),
	}}, nil
}

// Fold lists the most recent things in the Trash, each with the existing
// restore route, so the owner can bring one back without leaving the desk. It
// merges pages with the other kinds the way the Trash page does and leaves
// the rest to "and N more".
func (p *DeskProvider) Fold(ctx context.Context, req campaigns.DeskRequest) (*campaigns.DeskFold, error) {
	pages, err := p.svc.ListTrash(ctx, req.CampaignID)
	if err != nil {
		return nil, err
	}
	others := p.svc.ListOtherTrash(ctx, req.CampaignID)
	days := p.svc.RetentionDays(ctx)
	base := "/campaigns/" + req.CampaignID + "/trash/"

	type item struct {
		at  time.Time
		row campaigns.DeskFoldRow
	}
	var all []item
	for _, pg := range pages {
		meta := daysLeftWords(DaysLeft(pg.DeletedAt, days, req.Now))
		switch {
		case pg.DeletedBy != nil && *pg.DeletedBy == req.OwnerID:
			meta = "removed by you, " + meta
		case pg.DeletedByName != "":
			meta = "removed by " + pg.DeletedByName + ", " + meta
		}
		all = append(all, item{pg.DeletedAt, campaigns.DeskFoldRow{
			Lead: pg.Name, Meta: meta,
			Act: &campaigns.DeskFoldAct{Label: "Bring back", Busy: "Bringing back", URL: base + pg.ID + "/restore", Method: "POST"},
		}})
	}
	for _, o := range others {
		all = append(all, item{o.DeletedAt, campaigns.DeskFoldRow{
			Lead: o.Name, Meta: o.Label + ", " + daysLeftWords(DaysLeft(o.DeletedAt, days, req.Now)),
			Act: &campaigns.DeskFoldAct{Label: "Bring back", Busy: "Bringing back", URL: base + o.Kind + "/" + o.ID + "/restore", Method: "POST"},
		}})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].at.After(all[j].at) })

	f := &campaigns.DeskFold{}
	if len(all) == 0 {
		f.Note = "Trash is empty."
		return f, nil
	}
	for _, it := range all {
		f.Rows = append(f.Rows, it.row)
	}
	return f, nil
}

func daysLeftWords(n int) string {
	if n == 1 {
		return "1 day left"
	}
	return fmt.Sprintf("%d days left", n)
}
