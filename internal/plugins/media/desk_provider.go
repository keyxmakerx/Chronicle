// desk_provider.go tells the owner's desk what the campaign's picture store
// holds. It lives here so campaigns never imports this plugin.

package media

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// DeskProvider fills the desk's Media line.
type DeskProvider struct{ svc MediaService }

// NewDeskProvider returns the desk provider for the media store.
func NewDeskProvider(svc MediaService) *DeskProvider { return &DeskProvider{svc: svc} }

// Slot is the one desk line this provider fills.
func (p *DeskProvider) Slot() campaigns.DeskSlot { return campaigns.DeskMedia }

// Facts reads the campaign's file count and size; only the campaign the
// owner-only request names.
func (p *DeskProvider) Facts(ctx context.Context, req campaigns.DeskRequest) (campaigns.DeskFacts, error) {
	st, err := p.svc.GetCampaignStats(ctx, req.CampaignID)
	if err != nil {
		return campaigns.DeskFacts{}, err
	}
	return campaigns.DeskFacts{Media: &campaigns.DeskMediaFacts{Files: st.TotalFiles, Bytes: st.TotalBytes}}, nil
}
