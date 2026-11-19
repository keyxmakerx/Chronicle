// desk_provider.go tells the owner's desk how much players changed lately. It
// lives here, not in campaigns, so campaigns never imports this plugin: the
// app registers it on the desk's provider seam.

package playerchanges

import (
	"context"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// deskWindow is how far back "lately" reaches on the desk.
const deskWindow = 7 * 24 * time.Hour

// DeskProvider fills the desk's "what players changed" line.
type DeskProvider struct{ svc Service }

// NewDeskProvider returns the desk provider for the changes log.
func NewDeskProvider(svc Service) *DeskProvider { return &DeskProvider{svc: svc} }

// Slot is the one desk line this provider fills.
func (p *DeskProvider) Slot() campaigns.DeskSlot { return campaigns.DeskChanges }

// Facts counts the last week's changes. The desk route is owner-only and this
// reads only the one campaign the request names.
func (p *DeskProvider) Facts(ctx context.Context, req campaigns.DeskRequest) (campaigns.DeskFacts, error) {
	changes, people, err := p.svc.CountSince(ctx, req.CampaignID, req.Now.Add(-deskWindow))
	if err != nil {
		return campaigns.DeskFacts{}, err
	}
	return campaigns.DeskFacts{Changes: &campaigns.DeskChangesFacts{Changes: changes, Players: people}}, nil
}
