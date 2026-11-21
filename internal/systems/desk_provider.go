// desk_provider.go tells the owner's Campaign settings how many own rules
// entries the campaign has and how many players' drafts wait for a Director.
// It lives here so campaigns never imports the entry service's internals.

package systems

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EntriesDeskProvider fills the desk's Own entries line.
type EntriesDeskProvider struct {
	svc  SystemEntryService
	asks AskCounter
}

// AskCounter lists the Ask-first picks waiting on a Director.
type AskCounter interface {
	WaitingRequests(ctx context.Context, campaignID string, actor EntryActor) ([]WaitingChoiceRequest, error)
}

// NewEntriesDeskProvider returns the desk provider for Own entries.
// asks may be nil, which leaves Ask-first picks out of the figures.
func NewEntriesDeskProvider(svc SystemEntryService, asks AskCounter) *EntriesDeskProvider {
	return &EntriesDeskProvider{svc: svc, asks: asks}
}

// Slot is the one desk line this provider fills.
func (p *EntriesDeskProvider) Slot() campaigns.DeskSlot { return campaigns.DeskEntries }

// Facts reads the entries as a Director, the same list the page draws, so the
// row and the page agree. A draft a player is still writing is not counted:
// only a handed-in draft waits on the owner, and so does an Ask-first pick.
func (p *EntriesDeskProvider) Facts(ctx context.Context, req campaigns.DeskRequest) (campaigns.DeskFacts, error) {
	actor := EntryActor{UserID: req.OwnerID, IsDirector: true}
	list, err := p.svc.List(ctx, req.CampaignID, "", actor)
	if err != nil {
		return campaigns.DeskFacts{}, err
	}
	var f campaigns.DeskEntriesFacts
	for _, e := range list {
		switch e.Status {
		case EntryStatusSent:
			f.Drafts++
		case EntryStatusAccepted:
			f.Entries++
		}
	}
	if p.asks != nil {
		asks, err := p.asks.WaitingRequests(ctx, req.CampaignID, actor)
		if err != nil {
			return campaigns.DeskFacts{}, err
		}
		f.Asks = len(asks)
	}
	return campaigns.DeskFacts{Entries: &f}, nil
}
