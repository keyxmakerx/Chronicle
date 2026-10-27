package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

type waitingAsksStub struct {
	waiting, decided []connector.Request
	err              error
}

func (s waitingAsksStub) Waiting(context.Context, string) ([]connector.Request, error) {
	return s.waiting, s.err
}
func (s waitingAsksStub) Decided(context.Context, string, int) ([]connector.Request, error) {
	return s.decided, s.err
}

type waitingEntriesStub struct {
	list []systems.SystemEntry
	err  error
	seen *systems.EntryActor
}

func (s waitingEntriesStub) List(_ context.Context, _, _ string, a systems.EntryActor) ([]systems.SystemEntry, error) {
	if s.seen != nil {
		*s.seen = a
	}
	return s.list, s.err
}

type waitingMembersStub struct{}

func (waitingMembersStub) ListMembers(context.Context, string) ([]campaigns.CampaignMember, error) {
	return []campaigns.CampaignMember{{UserID: "u-sam", DisplayName: "Sam"}}, nil
}

type waitingRequestsStub struct {
	reqs []dmscreen.Request
	ok   bool
	err  error
}

func (s waitingRequestsStub) WaitingRequests(context.Context, string, dmscreen.Viewer) ([]dmscreen.Request, bool, error) {
	return s.reqs, s.ok, s.err
}

func TestAIWaitingAdapter(t *testing.T) {
	t0 := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	decided := t0.Add(time.Hour)
	owner := records.Actor{UserID: "u-owner", Role: int(campaigns.RoleOwner)}
	entries := []systems.SystemEntry{
		{Name: "Dwarf", FieldKey: "ancestry", Status: systems.EntryStatusAccepted},
		{Name: "Gnome", FieldKey: "ancestry", Status: systems.EntryStatusDraft, CreatedBy: "u-sam", ReviewNote: "Wings cost 3"},
		{Name: "Fairy", FieldKey: "ancestry", Status: systems.EntryStatusSent, CreatedBy: "u-sam", ForName: "Ash"},
	}
	boom := errors.New("boom")

	tests := []struct {
		name        string
		w           aiWaitingAdapter
		wantAsks    int
		wantDecided int
		wantEntries []string
		wantReqs    []string
		wantMissing []string
	}{
		{
			name: "every queue",
			w: aiWaitingAdapter{
				asks: waitingAsksStub{
					waiting: []connector.Request{{Summary: "Five characters", CreatedAt: t0}},
					decided: []connector.Request{{Summary: "Drop the moon", Status: connector.RequestTurnedDown, CreatedAt: t0, DecidedAt: &decided}},
				},
				entries:  waitingEntriesStub{list: entries},
				members:  waitingMembersStub{},
				requests: waitingRequestsStub{ok: true, reqs: []dmscreen.Request{{Text: "later", CreatedAt: t0.Add(time.Hour)}, {Text: "earlier", CreatedAt: t0}}},
			},
			wantAsks: 1, wantDecided: 1,
			wantEntries: []string{"Fairy", "Gnome"},
			wantReqs:    []string{"earlier", "later"},
		},
		{
			name: "no connector and no armory",
			w:    aiWaitingAdapter{entries: waitingEntriesStub{}, requests: waitingRequestsStub{ok: false}},
		},
		{
			name: "failing queues are named, the rest still answer",
			w: aiWaitingAdapter{
				asks:     waitingAsksStub{err: boom},
				entries:  waitingEntriesStub{err: boom},
				requests: waitingRequestsStub{ok: true, reqs: []dmscreen.Request{{Text: "earlier", CreatedAt: t0}}, err: nil},
			},
			wantReqs:    []string{"earlier"},
			wantMissing: []string{"the AI's requests", "players' entries"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.w.Waiting(context.Background(), "c1", owner)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Asks) != tt.wantAsks || len(got.Decided) != tt.wantDecided {
				t.Errorf("asks %d decided %d, want %d %d", len(got.Asks), len(got.Decided), tt.wantAsks, tt.wantDecided)
			}
			if tt.wantDecided > 0 && (!got.Decided[0].TurnedDown || !got.Decided[0].At.Equal(decided)) {
				t.Errorf("decided = %+v", got.Decided[0])
			}
			var names []string
			for _, e := range got.Entries {
				names = append(names, e.Name)
			}
			if !equalStrings(names, tt.wantEntries) {
				t.Errorf("entries %v, want %v", names, tt.wantEntries)
			}
			if len(got.Entries) == 2 {
				if f := got.Entries[0]; !f.HandedIn || f.Writer != "Sam" || f.For != "Ash" {
					t.Errorf("handed-in entry = %+v", f)
				}
				if g := got.Entries[1]; g.HandedIn || g.Note != "Wings cost 3" {
					t.Errorf("draft entry = %+v", g)
				}
			}
			var reqs []string
			for _, r := range got.Requests {
				reqs = append(reqs, r.Text)
			}
			if !equalStrings(reqs, tt.wantReqs) {
				t.Errorf("requests %v, want %v", reqs, tt.wantReqs)
			}
			if !equalStrings(got.Missing, tt.wantMissing) {
				t.Errorf("missing %v, want %v", got.Missing, tt.wantMissing)
			}
		})
	}

	// The entries are read as a Director, so players' drafts are included.
	var seen systems.EntryActor
	_, _ = aiWaitingAdapter{entries: waitingEntriesStub{seen: &seen}}.Waiting(context.Background(), "c1", owner)
	if !seen.IsDirector || seen.UserID != "u-owner" {
		t.Errorf("entries read as %+v", seen)
	}
}
