package sessions

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The banner asks "are you coming?" only about a night that can still be
// answered. A member who never answered must not be sent, days later, to a
// night that has been played, completed, cancelled or removed.
func TestOpenAsks_KeepsOnlyNightsThatStillTakeAnAnswer(t *testing.T) {
	now := time.Date(2026, 11, 18, 18, 0, 0, 0, time.UTC)
	day := func(d string) *string { return &d }
	nights := map[string]*Session{
		"ahead":     {ID: "ahead", Status: StatusPlanned, ScheduledDate: day("2026-11-20")},
		"tonight":   {ID: "tonight", Status: StatusPlanned, ScheduledDate: day("2026-11-18")},
		"played":    {ID: "played", Status: StatusPlanned, ScheduledDate: day("2026-11-13")},
		"completed": {ID: "completed", Status: StatusCompleted, ScheduledDate: day("2026-11-06")},
		"cancelled": {ID: "cancelled", Status: StatusCancelled, ScheduledDate: day("2026-11-27")},
		"undated":   {ID: "undated", Status: StatusPlanned},
		"series":    {ID: "series", Status: StatusPlanned, IsRecurring: true, ScheduledDate: day("2026-10-02")},
	}
	lookups := 0
	repo := &mockSessionRepo{findByIDFn: func(_ context.Context, id string) (*Session, error) {
		lookups++
		if id == "broken" {
			return nil, errors.New("the database is away")
		}
		// A removed night reads as nil, the way the repository hides a deleted row.
		return nights[id], nil
	}, findProposalByIDFn: func(_ context.Context, id string) (*SlotProposal, error) {
		return &SlotProposal{ID: id, Status: ProposalOpen}, nil
	}}
	svc := &sessionService{repo: repo, clock: func() time.Time { return now }}

	ask := func(id string) Notification {
		return note(NotifSessionScheduled, false, sessionLink("c1", id))
	}
	cases := []struct {
		name string
		in   Notification
		kept bool
	}{
		{"a night still ahead", ask("ahead"), true},
		{"tonight's game, through its own evening", ask("tonight"), true},
		{"a night with no date yet", ask("undated"), true},
		{"a repeating night, whose next date is still ahead", ask("series"), true},
		{"a night whose date has passed", ask("played"), false},
		{"a night marked complete", ask("completed"), false},
		{"a cancelled night", ask("cancelled"), false},
		{"a removed night", ask("removed"), false},
		{"a night that cannot be read", ask("broken"), false},
		{"an ask already answered is not this function's to judge", note(NotifSessionScheduled, true, sessionLink("c1", "played")), true},
		{"an open, unanswered date poll is kept", note(NotifProposalCreated, false, "/campaigns/c1/proposals/p1"), true},
		{"news is left alone", note(NotifProposalConfirmed, false, sessionLink("c1", "played")), true},
		{"an ask with no link is left alone", note(NotifSessionScheduled, false, ""), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.OpenAsks(context.Background(), []Notification{tc.in}, nil)
			if (len(got) == 1) != tc.kept {
				t.Errorf("kept = %v, want %v", len(got) == 1, tc.kept)
			}
		})
	}

	t.Run("order is kept and a night is read once however many rows point at it", func(t *testing.T) {
		lookups = 0
		in := []Notification{ask("played"), ask("ahead"), note(NotifProposalCreated, false, "/campaigns/c1/proposals/p1"), ask("ahead"), ask("played")}
		got := svc.OpenAsks(context.Background(), in, nil)
		if len(got) != 3 || sessionIDFromLink(got[0].Link) != "ahead" || got[1].Type != NotifProposalCreated || sessionIDFromLink(got[2].Link) != "ahead" {
			t.Errorf("got %d rows in the wrong shape: %+v", len(got), got)
		}
		if lookups != 2 {
			t.Errorf("read the nights %d times, want 2", lookups)
		}
	})

	// The banner is built from what OpenAsks returns: with only finished
	// nights unanswered it says nothing about them, and with one still ahead
	// it counts and links that one alone.
	t.Run("the banner follows", func(t *testing.T) {
		over := svc.OpenAsks(context.Background(), []Notification{ask("played"), ask("completed")}, nil)
		if out := BuildCallout(over, true, now); out.Kind != CalloutNone {
			t.Errorf("finished nights raised a banner: %+v", out)
		}
		mixed := svc.OpenAsks(context.Background(), []Notification{ask("played"), ask("ahead"), ask("cancelled")}, nil)
		out := BuildCallout(mixed, true, now)
		if out.Kind != CalloutRSVP || out.Count != 1 || out.Link != sessionLink("c1", "ahead") {
			t.Errorf("banner = %+v, want one ask linking to the night ahead", out)
		}
	})
}

// A date poll asks only while it is open and the member has answered none of
// its dates: once they answer, or the Director picks a date and closes it,
// the banner stops sending them to it, though the notification row itself is
// never marked read.
func TestOpenAsks_KeepsOnlyDatePollsStillWaitingOnTheMember(t *testing.T) {
	polls := map[string]*SlotProposal{
		"open":     {ID: "open", Status: ProposalOpen},
		"answered": {ID: "answered", Status: ProposalOpen},
		"others":   {ID: "others", Status: ProposalOpen},
		"closed":   {ID: "closed", Status: ProposalClosed},
	}
	responses := map[string][]SlotProposalResponse{
		"answered": {{OptionID: "o1", UserID: "pip", Response: ResponseYes}},
		"others":   {{OptionID: "o1", UserID: "quinn", Response: ResponseNo}},
		"closed":   {{OptionID: "o1", UserID: "pip", Response: ResponseYes}},
	}
	reads := 0
	repo := &mockSessionRepo{
		findProposalByIDFn: func(_ context.Context, id string) (*SlotProposal, error) {
			reads++
			if id == "broken" {
				return nil, errors.New("the database is away")
			}
			return polls[id], nil
		},
		listProposalResponsesFn: func(_ context.Context, id string) ([]SlotProposalResponse, error) {
			return responses[id], nil
		},
	}
	svc := &sessionService{repo: repo, clock: time.Now}
	poll := func(id string) Notification {
		n := note(NotifProposalCreated, false, proposalLink("c1", id))
		n.UserID = "pip"
		return n
	}

	cases := []struct {
		name string
		in   Notification
		kept bool
	}{
		{"an open poll the member has not answered", poll("open"), true},
		{"an open poll only others have answered", poll("others"), true},
		{"an open poll the member answered", poll("answered"), false},
		{"a poll the Director closed by picking a date", poll("closed"), false},
		{"a removed poll", poll("removed"), false},
		{"a poll that cannot be read", poll("broken"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.OpenAsks(context.Background(), []Notification{tc.in}, nil)
			if (len(got) == 1) != tc.kept {
				t.Errorf("kept = %v, want %v", len(got) == 1, tc.kept)
			}
		})
	}

	t.Run("the banner follows, and a poll is read once", func(t *testing.T) {
		reads = 0
		settled := svc.OpenAsks(context.Background(), []Notification{poll("answered"), poll("closed"), poll("closed")}, nil)
		if out := BuildCallout(settled, true, time.Now()); out.Kind != CalloutNone {
			t.Errorf("settled polls raised a banner: %+v", out)
		}
		if reads != 2 {
			t.Errorf("read the polls %d times, want 2", reads)
		}
		waiting := svc.OpenAsks(context.Background(), []Notification{poll("closed"), poll("open")}, nil)
		if out := BuildCallout(waiting, true, time.Now()); out.Kind != CalloutRSVP || out.Count != 1 || out.Link != proposalLink("c1", "open") {
			t.Errorf("banner = %+v, want one ask linking to the open poll", out)
		}
	})
}

// The reader's own day decides, campaign by campaign: it is the day the
// night's page marks it Played by, so the banner and the bell never send
// anyone to a page that will not take their answer. The campaign asked about
// is the night's own, never the one a notification claims.
func TestOpenAsks_JudgesANightOnTheReadersOwnDay(t *testing.T) {
	day := func(d string) *string { return &d }
	nights := map[string]*Session{
		"last-night": {ID: "last-night", CampaignID: "east", Status: StatusPlanned, ScheduledDate: day("2026-11-17")},
		"same-date":  {ID: "same-date", CampaignID: "west", Status: StatusPlanned, ScheduledDate: day("2026-11-17")},
	}
	repo := &mockSessionRepo{findByIDFn: func(_ context.Context, id string) (*Session, error) { return nights[id], nil }}
	// 10:00 UTC on the 18th: the 17th has not ended in every zone yet, so the
	// service's own day still calls both nights open.
	svc := &sessionService{repo: repo, clock: func() time.Time { return time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC) }}
	asks := []Notification{
		note(NotifSessionScheduled, false, sessionLink("somewhere-else", "last-night")),
		note(NotifSessionScheduled, false, sessionLink("somewhere-else", "same-date")),
	}

	if got := svc.OpenAsks(context.Background(), asks, nil); len(got) != 2 {
		t.Fatalf("with no reader's day, %d asks are open, want both", len(got))
	}

	var asked []string
	readersDay := func(campaignID string) string {
		asked = append(asked, campaignID)
		// The reader set an eastern zone in one campaign, where it is already
		// the 18th, and a western one in the other, where it is still the 17th.
		if campaignID == "east" {
			return "2026-11-18"
		}
		return "2026-11-17"
	}
	got := svc.OpenAsks(context.Background(), asks, readersDay)
	if len(got) != 1 || sessionIDFromLink(got[0].Link) != "same-date" {
		t.Errorf("open asks = %+v, want only the night whose date is still today for the reader", got)
	}
	if len(asked) != 2 || asked[0] != "east" || asked[1] != "west" {
		t.Errorf("asked for the day in %v, want the nights' own campaigns east and west", asked)
	}
}

func TestTakesAnswers(t *testing.T) {
	day := func(d string) *string { return &d }
	tests := []struct {
		name  string
		night *Session
		want  bool
	}{
		{"a night still ahead", &Session{Status: StatusPlanned, ScheduledDate: day("2026-11-20")}, true},
		{"tonight's game", &Session{Status: StatusPlanned, ScheduledDate: day("2026-11-18")}, true},
		{"a night with no date", &Session{Status: StatusPlanned}, true},
		{"a repeating night that began long ago", &Session{Status: StatusPlanned, IsRecurring: true, ScheduledDate: day("2026-10-02")}, true},
		{"a night whose date is behind today", &Session{Status: StatusPlanned, ScheduledDate: day("2026-11-17")}, false},
		{"a completed night", &Session{Status: StatusCompleted, ScheduledDate: day("2026-11-20")}, false},
		{"a cancelled night", &Session{Status: StatusCancelled, ScheduledDate: day("2026-11-20")}, false},
		{"no night at all", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := takesAnswers(tt.night, "2026-11-18"); got != tt.want {
				t.Errorf("takesAnswers = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionIDFromLink(t *testing.T) {
	s := func(v string) *string { return &v }
	cases := []struct {
		link *string
		want string
	}{
		{s(sessionLink("c1", "s1")), "s1"},
		{s(" /campaigns/c1/sessions/s1/ "), "s1"},
		{s("/campaigns/c1/proposals/p1"), ""},
		{s("/campaigns/c1/sessions/s1/recap"), ""},
		{s("https://elsewhere.example/campaigns/c1/sessions/s1"), ""},
		{s(""), ""},
		{nil, ""},
	}
	for _, tc := range cases {
		if got := sessionIDFromLink(tc.link); got != tc.want {
			v := "<nil>"
			if tc.link != nil {
				v = *tc.link
			}
			t.Errorf("sessionIDFromLink(%q) = %q, want %q", v, got, tc.want)
		}
	}
}
