package sessions

import (
	"context"
	"testing"
	"time"
)

func TestOpenPollsForTheCalendar(t *testing.T) {
	// 01:30 UTC on Oct 17 is still the evening of Oct 16 in Chicago.
	start := time.Date(2026, 10, 17, 1, 30, 0, 0, time.UTC)
	repo := &mockSessionRepo{
		listProposalsFn: func(_ context.Context, _ string) ([]SlotProposal, error) {
			return []SlotProposal{
				{ID: "p1", CampaignID: "c1", CreatedBy: "dm", Title: "Next arc", Status: ProposalOpen},
				{ID: "p2", CampaignID: "c1", CreatedBy: "dm", Title: "Done", Status: ProposalClosed},
			}, nil
		},
		getProposalFn: func(_ context.Context, _, pid string) (*SlotProposal, []SlotProposalOption, error) {
			if pid != "p1" {
				t.Fatalf("a closed poll was read: %s", pid)
			}
			return &SlotProposal{ID: "p1", CampaignID: "c1", CreatedBy: "dm", Title: "Next arc", Status: ProposalOpen},
				[]SlotProposalOption{{ID: "o1", ProposalID: "p1", StartsAtUTC: start, EndsAtUTC: start.Add(4 * time.Hour), Ordinal: 1}}, nil
		},
		listProposalResponsesFn: func(_ context.Context, _ string) ([]SlotProposalResponse, error) {
			return []SlotProposalResponse{
				{OptionID: "o1", UserID: "u1", Response: ResponseYes},
				{OptionID: "o1", UserID: "u2", Response: ResponseMaybe},
			}, nil
		},
	}
	views, err := newTestSessionService(repo).ListOpenProposalViews(context.Background(), "c1", "u2", "America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	polls := calendarPollsFrom(views, map[string]string{"dm": "Echo"}, 4, true)
	if len(polls) != 1 {
		t.Fatalf("got %d polls, want only the open one", len(polls))
	}
	p := polls[0]
	if p.AskedBy != "Echo" || !p.CanPick || p.Members != 4 || p.TZ != "America/Chicago" {
		t.Fatalf("poll = %+v", p)
	}
	o := p.Options[0]
	if o.Date != "2026-10-16" || o.TimeLabel != "8:30 PM – 12:30 AM (Sat, Oct 17)" {
		t.Fatalf("option lands on %s %q, want the evening it starts in Chicago", o.Date, o.TimeLabel)
	}
	if o.Yes != 1 || o.Maybe != 1 || o.No != 0 || o.Mine != ResponseMaybe {
		t.Fatalf("option = %+v", o)
	}
	if got := calendarPollsFrom(views, nil, 0, false)[0].AskedBy; got != "Someone" {
		t.Fatalf("a poll whose asker left reads %q", got)
	}
}

func TestPollLandingDate(t *testing.T) {
	opt := func(date string, win bool) ProposalOptionView {
		return ProposalOptionView{Option: SlotProposalOption{IsWinner: win}, Local: LocalSlot{Date: date}}
	}
	tests := []struct {
		name string
		opts []ProposalOptionView
		want string
	}{
		{"the picked date", []ProposalOptionView{opt("2026-10-17", false), opt("2026-10-24", true)}, "2026-10-24"},
		{"the next date it suggests", []ProposalOptionView{opt("2026-10-10", false), opt("2026-10-17", false)}, "2026-10-17"},
		{"its first, once all have passed", []ProposalOptionView{opt("2026-10-03", false), opt("2026-10-10", false)}, "2026-10-03"},
		{"nothing to open", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pollLandingDate(&ProposalView{Options: tt.opts}, "2026-10-12"); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPollTarget(t *testing.T) {
	if got := pollTarget("c1", "k1", "p1", "2026-10-17"); got != "/campaigns/c1/calendars/k1/view?date=2026-10-17&poll=p1" {
		t.Fatalf("got %q", got)
	}
	if got := pollTarget("c1", "k1", "p1", "soon"); got != "/campaigns/c1/calendars/k1/view?poll=p1" {
		t.Fatalf("a bad date is dropped, got %q", got)
	}
}
