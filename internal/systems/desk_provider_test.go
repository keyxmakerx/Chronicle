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
	p := NewEntriesDeskProvider(svc)
	if p.Slot() != campaigns.DeskEntries {
		t.Fatalf("slot = %q", p.Slot())
	}
	f, err := p.Facts(context.Background(), campaigns.DeskRequest{CampaignID: "c", OwnerID: "o"})
	if err != nil || f.Entries == nil {
		t.Fatalf("facts = %+v, err = %v", f, err)
	}
	if f.Entries.Entries != 2 || f.Entries.Waiting != 1 {
		t.Errorf("got %+v, want 2 entries and 1 waiting (a draft still being written is not counted)", *f.Entries)
	}
	if !svc.seen.IsDirector {
		t.Error("entries must be read as a Director so handed-in drafts are visible")
	}
}
