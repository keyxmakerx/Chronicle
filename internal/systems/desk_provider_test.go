package systems

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// listOnlyEntries answers List with a fixed set; any other call panics, which
// pins that the provider reads nothing else.
type listOnlyEntries struct {
	SystemEntryService
	list []SystemEntry
	seen EntryActor
}

func (l *listOnlyEntries) List(_ context.Context, _, _ string, a EntryActor) ([]SystemEntry, error) {
	l.seen = a
	return l.list, nil
}

func TestEntriesDeskProvider_CountsAcceptedAndHandedIn(t *testing.T) {
	svc := &listOnlyEntries{list: []SystemEntry{
		{Status: EntryStatusAccepted}, {Status: EntryStatusAccepted},
		{Status: EntryStatusSent}, {Status: EntryStatusDraft},
	}}
	asks := fakeAsks(2)
	p := NewEntriesDeskProvider(svc, asks)
	if p.Slot() != campaigns.DeskEntries {
		t.Fatalf("slot = %q", p.Slot())
	}
	f, err := p.Facts(context.Background(), campaigns.DeskRequest{CampaignID: "c", OwnerID: "o"})
	if err != nil || f.Entries == nil {
		t.Fatalf("facts = %+v, err = %v", f, err)
	}
	if f.Entries.Entries != 2 || f.Entries.Drafts != 1 || f.Entries.Asks != 2 {
		t.Errorf("got %+v, want 2 entries, 1 draft and 2 asks (a draft still being written is not counted)", *f.Entries)
	}
	if !svc.seen.IsDirector {
		t.Error("entries must be read as a Director so handed-in drafts are visible")
	}
}

type fakeAsks int

func (n fakeAsks) WaitingRequests(context.Context, string, EntryActor) ([]WaitingChoiceRequest, error) {
	return make([]WaitingChoiceRequest, int(n)), nil
}
