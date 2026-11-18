// desk_provider.go tells the owner's desk where their AI connection stands and
// how many of its requests wait. It lives here so campaigns never imports this
// plugin.

package ai_workspace

import (
	"context"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// DeskProvider fills the desk's Your AI line.
type DeskProvider struct{ svc *connector.Service }

// NewDeskProvider returns the desk provider for the owner's AI connection.
func NewDeskProvider(svc *connector.Service) *DeskProvider { return &DeskProvider{svc: svc} }

// Slot is the one desk line this provider fills.
func (p *DeskProvider) Slot() campaigns.DeskSlot { return campaigns.DeskAI }

// Facts reads the connection the request's owner made. It asks for the owner's
// own connection only, never a player's key, so no other person's AI shows on
// the desk.
func (p *DeskProvider) Facts(ctx context.Context, req campaigns.DeskRequest) (campaigns.DeskFacts, error) {
	conn, err := p.svc.Current(ctx, req.CampaignID, req.OwnerID)
	if err != nil {
		return campaigns.DeskFacts{}, err
	}
	f := &campaigns.DeskAIFacts{State: deskAIState(conn, req.Now)}
	if conn != nil {
		f.Client = conn.ClientName
		f.CanAsk = conn.CanAsk
	}
	if f.State == campaigns.DeskAIConnected || f.State == campaigns.DeskAIPaused {
		waiting, err := p.svc.Waiting(ctx, req.CampaignID, req.OwnerID)
		if err != nil {
			return campaigns.DeskFacts{}, err
		}
		f.Waiting = len(waiting)
	}
	return campaigns.DeskFacts{AI: f}, nil
}

// deskAIState maps a key to the desk's four words. A revoked or run-out key is
// "ended": the page keeps saying so until the owner makes a new one.
func deskAIState(c *connector.Connection, now time.Time) campaigns.DeskAIState {
	switch {
	case c == nil:
		return campaigns.DeskAINone
	case !c.Live(now):
		return campaigns.DeskAIEnded
	case c.PausedAt != nil:
		return campaigns.DeskAIPaused
	}
	return campaigns.DeskAIConnected
}
