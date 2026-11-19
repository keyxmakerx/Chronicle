// desk_provider.go tells the owner's desk what waits in the campaign Trash.
// It lives here so campaigns never imports this plugin.

package entities

import (
	"context"

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
