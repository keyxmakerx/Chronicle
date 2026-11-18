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
		{"a when-can-we-play ask is left alone", note(NotifProposalCreated, false, "/campaigns/c1/proposals/p1"), true},
		{"news is left alone", note(NotifProposalConfirmed, false, sessionLink("c1", "played")), true},
		{"an ask with no link is left alone", note(NotifSessionScheduled, false, ""), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.OpenAsks(context.Background(), []Notification{tc.in})
			if (len(got) == 1) != tc.kept {
				t.Errorf("kept = %v, want %v", len(got) == 1, tc.kept)
			}
		})
	}

	t.Run("order is kept and a night is read once however many rows point at it", func(t *testing.T) {
		lookups = 0
		in := []Notification{ask("played"), ask("ahead"), note(NotifProposalCreated, false, "/campaigns/c1/proposals/p1"), ask("ahead"), ask("played")}
		got := svc.OpenAsks(context.Background(), in)
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
		over := svc.OpenAsks(context.Background(), []Notification{ask("played"), ask("completed")})
		if out := BuildCallout(over, true, now); out.Kind != CalloutNone {
			t.Errorf("finished nights raised a banner: %+v", out)
		}
		mixed := svc.OpenAsks(context.Background(), []Notification{ask("played"), ask("ahead"), ask("cancelled")})
		out := BuildCallout(mixed, true, now)
		if out.Kind != CalloutRSVP || out.Count != 1 || out.Link != sessionLink("c1", "ahead") {
			t.Errorf("banner = %+v, want one ask linking to the night ahead", out)
		}
	})
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
