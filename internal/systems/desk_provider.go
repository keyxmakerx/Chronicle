// desk_provider.go tells the owner's Campaign settings how many own rules
// entries the campaign has and how many players' drafts wait for a Director.
// It lives here so campaigns never imports the entry service's internals.

package systems

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EntriesDeskProvider fills the desk's Own entries line.
type EntriesDeskProvider struct{ svc SystemEntryService }

// NewEntriesDeskProvider returns the desk provider for Own entries.
func NewEntriesDeskProvider(svc SystemEntryService) *EntriesDeskProvider {
	return &EntriesDeskProvider{svc: svc}
}

// Slot is the one desk line this provider fills.
func (p *EntriesDeskProvider) Slot() campaigns.DeskSlot { return campaigns.DeskEntries }

// Facts reads the entries as a Director, the same list the page draws, so the
// row and the page agree. A draft a player is still writing is not counted:
// only a handed-in draft waits on the owner.
func (p *EntriesDeskProvider) Facts(ctx context.Context, req campaigns.DeskRequest) (campaigns.DeskFacts, error) {
	list, err := p.svc.List(ctx, req.CampaignID, "", EntryActor{UserID: req.OwnerID, IsDirector: true})
	if err != nil {
		return campaigns.DeskFacts{}, err
	}
	var f campaigns.DeskEntriesFacts
	for _, e := range list {
		switch e.Status {
		case EntryStatusSent:
			f.Waiting++
		case EntryStatusAccepted:
			f.Entries++
		}
	}
	return campaigns.DeskFacts{Entries: &f}, nil
}
