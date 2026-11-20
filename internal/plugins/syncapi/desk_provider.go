// desk_provider.go tells the owner's desk how many API keys the campaign has.
// It lives here so campaigns never imports this plugin.

package syncapi

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// DeskProvider fills the desk's Keys row.
type DeskProvider struct{ svc SyncAPIService }

// NewDeskProvider returns the desk provider for the campaign's API keys.
func NewDeskProvider(svc SyncAPIService) *DeskProvider { return &DeskProvider{svc: svc} }

// Slot is the one desk row this provider fills.
func (p *DeskProvider) Slot() campaigns.DeskSlot { return campaigns.DeskKeys }

// Facts counts the keys in use, which is what the keys page counts as active:
// a revoked or expired key is no longer a way in. It reads through the
// per-campaign index, and only the campaign the owner-only request names.
func (p *DeskProvider) Facts(ctx context.Context, req campaigns.DeskRequest) (campaigns.DeskFacts, error) {
	keys, err := p.svc.ListKeysByCampaign(ctx, req.CampaignID)
	if err != nil {
		return campaigns.DeskFacts{}, err
	}
	n := 0
	for i := range keys {
		if keys[i].IsActive && !keys[i].IsExpired() {
			n++
		}
	}
	return campaigns.DeskFacts{Keys: &campaigns.DeskKeysFacts{Count: n}}, nil
}
